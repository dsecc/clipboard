package client

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"
)

type KeyKind int

const (
	KRune KeyKind = iota
	KEnter
	KBackspace
	KDelete
	KLeft
	KRight
	KUp
	KDown
	KHome
	KEnd
	KCtrlC
	KCtrlD
	KCtrlA
	KCtrlE
	KCtrlK
	KCtrlU
	KCtrlW
	KPaste
	KMouse  // a mouse report (X, Y, Btn, Press)
	KCursor // the terminal's answer to "where is the cursor" (Y = row, X = column)
)

type Key struct {
	Kind  KeyKind
	R     rune   // KRune
	Text  string // KPaste
	X, Y  int    // KMouse, KCursor (1-based screen coordinates)
	Btn   int    // KMouse: 0 = left button, 64/65 = wheel
	Press bool   // KMouse: true on press, false on release
}

var (
	pasteStart = []byte("\x1b[200~")
	pasteEnd   = []byte("\x1b[201~")
)

// Parser turns raw terminal bytes into keys. It keeps incomplete sequences
// (half a UTF-8 rune, half an escape code, an unfinished paste) until the rest arrives.
type Parser struct {
	buf     []byte
	inPaste bool
	paste   []byte
}

func (p *Parser) Feed(b []byte) []Key {
	p.buf = append(p.buf, b...)
	var out []Key
	for len(p.buf) > 0 {
		if p.inPaste {
			i := bytes.Index(p.buf, pasteEnd)
			if i < 0 {
				// keep a possible partial end marker in buf, move the rest to the paste
				keep := partialSuffix(p.buf, pasteEnd)
				p.paste = append(p.paste, p.buf[:len(p.buf)-keep]...)
				p.buf = append([]byte(nil), p.buf[len(p.buf)-keep:]...)
				return out
			}
			p.paste = append(p.paste, p.buf[:i]...)
			p.buf = p.buf[i+len(pasteEnd):]
			text := string(bytes.ReplaceAll(bytes.ReplaceAll(p.paste, []byte("\r\n"), []byte("\n")), []byte("\r"), []byte("\n")))
			out = append(out, Key{Kind: KPaste, Text: text})
			p.inPaste, p.paste = false, nil
			continue
		}
		k, n, ok := p.next()
		if !ok {
			return out
		}
		p.buf = p.buf[n:]
		if k == nil {
			continue
		}
		out = append(out, *k)
	}
	return out
}

func partialSuffix(b, marker []byte) int {
	for n := len(marker) - 1; n > 0; n-- {
		if len(b) >= n && bytes.Equal(b[len(b)-n:], marker[:n]) {
			return n
		}
	}
	return 0
}

// csi decodes "ESC [ params final" at the front of b. n is the length used;
// complete is false when more bytes are needed.
func csi(b []byte) (params string, final byte, n int, complete bool) {
	i := 2
	for i < len(b) && b[i] >= 0x20 && b[i] <= 0x3f {
		i++
	}
	if i >= len(b) {
		return "", 0, 0, false
	}
	return string(b[2:i]), b[i], i + 1, true
}

func ints(s string) []int {
	var out []int
	for _, f := range strings.Split(s, ";") {
		n, _ := strconv.Atoi(f)
		out = append(out, n)
	}
	return out
}

// next decodes one key from the front of buf: (key or nil to skip, bytes used, complete?).
func (p *Parser) next() (*Key, int, bool) {
	b := p.buf
	if bytes.HasPrefix(b, pasteStart) {
		p.inPaste = true
		return nil, len(pasteStart), true
	}
	one := func(k KeyKind, n int) (*Key, int, bool) { return &Key{Kind: k}, n, true }
	switch c := b[0]; {
	case c == 0x1b:
		if len(b) < 2 {
			return nil, 0, false
		}
		if b[1] == 'O' { // ESC O x: application-mode arrows and Home/End
			if len(b) < 3 {
				return nil, 0, false
			}
			switch b[2] {
			case 'A':
				return one(KUp, 3)
			case 'B':
				return one(KDown, 3)
			case 'C':
				return one(KRight, 3)
			case 'D':
				return one(KLeft, 3)
			case 'H':
				return one(KHome, 3)
			case 'F':
				return one(KEnd, 3)
			}
			return nil, 3, true
		}
		if b[1] != '[' {
			return nil, 1, true // lone ESC: ignore
		}
		params, final, n, ok := csi(b)
		if !ok {
			return nil, 0, false
		}
		switch final {
		case 'A':
			return one(KUp, n)
		case 'B':
			return one(KDown, n)
		case 'C':
			return one(KRight, n)
		case 'D':
			return one(KLeft, n)
		case 'H':
			return one(KHome, n)
		case 'F':
			return one(KEnd, n)
		case '~':
			switch ints(params)[0] {
			case 1, 7:
				return one(KHome, n)
			case 4, 8:
				return one(KEnd, n)
			case 3:
				return one(KDelete, n)
			}
		case 'M', 'm':
			if strings.HasPrefix(params, "<") { // SGR mouse: ESC [ < btn ; x ; y M (press) / m (release)
				v := ints(params[1:])
				if len(v) == 3 {
					return &Key{Kind: KMouse, Btn: v[0], X: v[1], Y: v[2], Press: final == 'M'}, n, true
				}
			}
		case 'R': // cursor position report: ESC [ row ; col R
			if v := ints(params); len(v) == 2 {
				return &Key{Kind: KCursor, Y: v[0], X: v[1]}, n, true
			}
		}
		return nil, n, true // anything else is swallowed whole
	case c == '\r' || c == '\n':
		return one(KEnter, 1)
	case c == 0x7f || c == 0x08:
		return one(KBackspace, 1)
	case c == 0x03:
		return one(KCtrlC, 1)
	case c == 0x04:
		return one(KCtrlD, 1)
	case c == 0x01:
		return one(KCtrlA, 1)
	case c == 0x05:
		return one(KCtrlE, 1)
	case c == 0x0b:
		return one(KCtrlK, 1)
	case c == 0x15:
		return one(KCtrlU, 1)
	case c == 0x17:
		return one(KCtrlW, 1)
	case c < 0x20:
		return nil, 1, true
	}
	if !utf8.FullRune(b) {
		return nil, 0, false
	}
	r, n := utf8.DecodeRune(b)
	return &Key{Kind: KRune, R: r}, n, true
}
