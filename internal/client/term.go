package client

import (
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/term"
)

var ErrInterrupt = errors.New("interrumpido")

const prompt = "› "

// Term is the interactive terminal: a one-line editor at the bottom, with
// incoming lines printed above it without disturbing what is being typed.
type Term struct {
	mu     sync.Mutex
	out    io.Writer
	ed     Editor
	hist   History
	parser Parser
	keys   []Key // decoded but not yet handled
	// pasteOnly: the buffer holds one untouched bracketed paste
	pasteOnly bool
	rows      RowMap // which entry each printed row belongs to
	mouse     bool
	clicks    []int // rows of clicks waiting for the cursor position report
	// OnClick is called (in its own goroutine) with the entry a left click landed on.
	OnClick func(id string)
	input   chan []byte
	state   *term.State
	restore func()
}

func NewTerm() (*Term, error) {
	t := &Term{out: os.Stdout, input: make(chan []byte, 8)}
	st, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return nil, err
	}
	t.state = st
	t.restore = enableVT()
	t.write("\x1b[?2004h") // bracketed paste: a pasted block arrives as one piece
	t.setMouse(true)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				t.input <- append([]byte(nil), buf[:n]...)
			}
			if err != nil {
				close(t.input)
				return
			}
		}
	}()
	return t, nil
}

func (t *Term) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.setMouse(false)
	t.write("\r\x1b[2K\x1b[?2004l")
	t.restore()
	_ = term.Restore(int(os.Stdin.Fd()), t.state)
}

func (t *Term) write(s string) { _, _ = io.WriteString(t.out, s) }

func (t *Term) width() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 10 {
		return w
	}
	return 80
}

func (t *Term) draw() {
	pw := runesWidth([]rune(prompt))
	view, col := t.ed.View(t.width() - pw)
	var b strings.Builder
	b.WriteString("\r\x1b[2K" + prompt + view + "\r")
	if n := pw + col; n > 0 {
		b.WriteString("\x1b[" + strconv.Itoa(n) + "C")
	}
	t.write(b.String())
}

// setMouse turns click reporting on or off (the caller holds mu, or is single-threaded).
func (t *Term) setMouse(on bool) {
	t.mouse = on
	if on {
		t.write("\x1b[?1000h\x1b[?1006h")
	} else {
		t.write("\x1b[?1006l\x1b[?1000l")
	}
}

// ToggleMouse switches click-to-copy and reports the new state. While it is on,
// selecting text with the mouse needs Shift held down.
func (t *Term) ToggleMouse() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.setMouse(!t.mouse)
	return t.mouse
}

// Print writes s above the prompt and redraws what the user was typing.
func (t *Term) Print(s string) { t.PrintEntry("", s) }

// PrintEntry is Print for the text of entry id, so a click on it can be traced back.
func (t *Term) PrintEntry(id, s string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rows.Add(id, s, t.width())
	t.write("\r\x1b[2K" + strings.ReplaceAll(s, "\n", "\r\n") + "\r\n")
	t.draw()
}

// ReadLine blocks until the user presses Enter. Ctrl+D on an empty line gives
// io.EOF; Ctrl+C gives ErrInterrupt.
//
// The bool says the whole line came from one bracketed paste and was not edited,
// so the caller can send it verbatim instead of reading it as a command.
func (t *Term) ReadLine() (string, bool, error) {
	t.mu.Lock()
	t.draw()
	t.mu.Unlock()
	for {
		if len(t.keys) == 0 {
			chunk, ok := <-t.input
			if !ok {
				return "", false, io.EOF
			}
			t.keys = t.parser.Feed(chunk)
		}
		t.mu.Lock()
		for len(t.keys) > 0 {
			k := t.keys[0]
			t.keys = t.keys[1:]
			switch k.Kind {
			case KRune, KBackspace, KDelete, KCtrlK, KCtrlU, KCtrlW, KCtrlD, KUp, KDown:
				t.pasteOnly = false
			}
			switch k.Kind {
			case KRune:
				t.ed.Insert(string(k.R))
			case KPaste:
				t.pasteOnly = t.ed.Text() == ""
				t.ed.Insert(k.Text)
			case KBackspace:
				t.ed.Backspace()
			case KDelete:
				t.ed.Delete()
			case KLeft:
				t.ed.Left()
			case KRight:
				t.ed.Right()
			case KHome, KCtrlA:
				t.ed.Home()
			case KEnd, KCtrlE:
				t.ed.End()
			case KCtrlK:
				t.ed.KillToEnd()
			case KCtrlU:
				t.ed.KillToStart()
			case KCtrlW:
				t.ed.KillWord()
			case KUp:
				if s, ok := t.hist.Prev(t.ed.Text()); ok {
					t.ed.SetText(s)
				}
			case KDown:
				if s, ok := t.hist.Next(); ok {
					t.ed.SetText(s)
				}
			case KMouse:
				if t.mouse && k.Press && k.Btn == 0 {
					// ask where the prompt is now; the answer tells us which entry row was hit
					t.clicks = append(t.clicks, k.Y)
					t.write("\x1b[6n")
				}
			case KCursor:
				if len(t.clicks) > 0 {
					y := t.clicks[0]
					t.clicks = t.clicks[1:]
					if id := t.rows.Lookup(k.Y, y); id != "" && t.OnClick != nil {
						go t.OnClick(id)
					}
				}
			case KCtrlC:
				t.mu.Unlock()
				return "", false, ErrInterrupt
			case KCtrlD:
				if t.ed.Text() == "" {
					t.mu.Unlock()
					return "", false, io.EOF
				}
				t.ed.Delete()
			case KEnter:
				line, pasted := t.ed.Text(), t.pasteOnly
				t.hist.Push(strings.TrimSpace(line))
				t.ed.Reset()
				t.pasteOnly = false
				t.write("\r\x1b[2K") // the submitted line is not echoed; its result is printed instead
				t.mu.Unlock()
				return line, pasted, nil
			}
		}
		t.draw()
		t.mu.Unlock()
	}
}
