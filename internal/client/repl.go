package client

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"clipboard/internal/model"
)

type CmdKind int

const (
	Noop CmdKind = iota
	Send
	Copy
	Delete
	Quit
	Help
	Mouse
	Invalid
)

type Command struct {
	Kind CmdKind
	N    int    // entry number for Copy/Delete (0 = the latest, Copy only)
	Text string // text to send, or a usage message for Invalid
}

// ParseCommand turns a submitted line into a command. Anything that is not
// exactly a command is text to send; a leading backslash escapes a command word.
func ParseCommand(in string) Command {
	t := strings.TrimSpace(in)
	if t == "" {
		return Command{Kind: Noop}
	}
	if strings.Contains(t, "\n") {
		return Command{Kind: Send, Text: t}
	}
	if strings.HasPrefix(t, `\`) {
		return Command{Kind: Send, Text: t[1:]}
	}
	f := strings.Fields(t)
	num := func() (int, bool) {
		n, err := strconv.Atoi(f[1])
		return n, err == nil && n > 0
	}
	switch f[0] {
	case "c":
		switch {
		case len(f) == 1:
			return Command{Kind: Copy}
		case len(f) == 2:
			if n, ok := num(); ok {
				return Command{Kind: Copy, N: n}
			}
			return Command{Kind: Invalid, Text: "uso: c [número]"}
		}
	case "d":
		switch {
		case len(f) == 1:
			return Command{Kind: Invalid, Text: "uso: d número"}
		case len(f) == 2:
			if n, ok := num(); ok {
				return Command{Kind: Delete, N: n}
			}
			return Command{Kind: Invalid, Text: "uso: d número"}
		}
	case "q", "quit", "exit":
		if len(f) == 1 {
			return Command{Kind: Quit}
		}
	case "?", "help":
		if len(f) == 1 {
			return Command{Kind: Help}
		}
	case "m":
		if len(f) == 1 {
			return Command{Kind: Mouse}
		}
	}
	return Command{Kind: Send, Text: t}
}

// ParseInput is ParseCommand for a submitted line. Text that was pasted is
// never a command and is sent exactly as it came, minus trailing newlines.
func ParseInput(line string, pasted bool) Command {
	if !pasted {
		return ParseCommand(line)
	}
	text := strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(text) == "" {
		return Command{Kind: Noop}
	}
	return Command{Kind: Send, Text: text}
}

// visible turns a character that a terminal would act on (escape codes, carriage
// return, bell...) into a harmless printable stand-in. Newlines are left alone.
func visible(r rune) rune {
	switch {
	case r == '\n':
		return r
	case r == '\t':
		return ' '
	case r < 0x20:
		return 0x2400 + r // ␛ ␍ ␇ ...
	case r == 0x7f:
		return 0x2421
	case r >= 0x80 && r <= 0x9f:
		return '·'
	}
	return r
}

func sanitize(s string) string {
	return strings.Map(visible, s)
}

// Numbering gives each entry a number that stays the same for the whole session
// and is never reused, so "c 3" always means the same thing.
type Numbering struct {
	next    int
	byID    map[string]int
	byN     map[int]string
	removed map[string]bool
	order   []string
}

func NewNumbering() *Numbering {
	return &Numbering{next: 1, byID: map[string]int{}, byN: map[int]string{}, removed: map[string]bool{}}
}

func (n *Numbering) Assign(id string) int {
	if v, ok := n.byID[id]; ok {
		return v
	}
	v := n.next
	n.next++
	n.byID[id], n.byN[v] = v, id
	n.order = append(n.order, id)
	return v
}

func (n *Numbering) Remove(id string) { n.removed[id] = true }

func (n *Numbering) ID(num int) (string, bool) {
	id, ok := n.byN[num]
	if !ok || n.removed[id] {
		return "", false
	}
	return id, true
}

func (n *Numbering) Last() (string, bool) {
	for i := len(n.order) - 1; i >= 0; i-- {
		if !n.removed[n.order[i]] {
			return n.order[i], true
		}
	}
	return "", false
}

func ago(t, now time.Time) string {
	s := now.Sub(t).Seconds()
	if s < 45 {
		return "ahora"
	}
	m := int(s/60 + 0.5)
	if m < 60 {
		return fmt.Sprintf("hace %d min", m)
	}
	return fmt.Sprintf("hace %d h", int(float64(m)/60+0.5))
}

func fmtSize(n int) string {
	if n >= 1<<20 {
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	k := (n + 512) / 1024
	if k < 1 {
		k = 1
	}
	return fmt.Sprintf("%d KB", k)
}

// FormatEntry renders one entry for the terminal; multi-line text goes under
// its header, indented.
func FormatEntry(n int, e model.Entry, now time.Time) string {
	head := fmt.Sprintf("[%d] %s · %s:", n, strings.ReplaceAll(sanitize(e.Device), "\n", " "), ago(e.CreatedAt, now))
	if e.Kind == "image" {
		return fmt.Sprintf("%s [imagen %s]", head, fmtSize(e.Size))
	}
	text := sanitize(e.Text)
	if !strings.Contains(text, "\n") {
		return head + " " + text
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	return head + "\n    " + strings.Join(lines, "\n    ")
}
