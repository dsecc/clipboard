package client

// Editor is a single-line text buffer with a cursor. Pasted newlines are kept
// in the text but drawn as a marker so the prompt stays on one terminal line.
type Editor struct {
	buf []rune
	pos int
}

func (e *Editor) Insert(s string) {
	r := []rune(s)
	e.buf = append(e.buf[:e.pos], append(r, e.buf[e.pos:]...)...)
	e.pos += len(r)
}

func (e *Editor) Backspace() {
	if e.pos > 0 {
		e.buf = append(e.buf[:e.pos-1], e.buf[e.pos:]...)
		e.pos--
	}
}

func (e *Editor) Delete() {
	if e.pos < len(e.buf) {
		e.buf = append(e.buf[:e.pos], e.buf[e.pos+1:]...)
	}
}

func (e *Editor) Left() {
	if e.pos > 0 {
		e.pos--
	}
}

func (e *Editor) Right() {
	if e.pos < len(e.buf) {
		e.pos++
	}
}

func (e *Editor) Home()        { e.pos = 0 }
func (e *Editor) End()         { e.pos = len(e.buf) }
func (e *Editor) KillToEnd()   { e.buf = e.buf[:e.pos] }
func (e *Editor) KillToStart() { e.buf = append([]rune(nil), e.buf[e.pos:]...); e.pos = 0 }

func (e *Editor) KillWord() {
	i := e.pos
	for i > 0 && e.buf[i-1] == ' ' {
		i--
	}
	for i > 0 && e.buf[i-1] != ' ' {
		i--
	}
	e.buf = append(e.buf[:i], e.buf[e.pos:]...)
	e.pos = i
}

func (e *Editor) Text() string { return string(e.buf) }
func (e *Editor) Reset()       { e.buf, e.pos = nil, 0 }
func (e *Editor) SetText(s string) {
	e.buf = []rune(s)
	e.pos = len(e.buf)
}

func runeWidth(r rune) int {
	switch {
	case r == 0x200d || r == 0xfe0f || (r >= 0x300 && r <= 0x36f):
		return 0
	case r >= 0x1100 && r <= 0x115f, r >= 0x2e80 && r <= 0xa4cf, r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff, r >= 0xfe30 && r <= 0xfe6f, r >= 0xff00 && r <= 0xff60,
		r >= 0x1f300 && r <= 0x1faff, r >= 0x20000:
		return 2
	}
	return 1
}

func runesWidth(r []rune) int {
	w := 0
	for _, c := range r {
		w += runeWidth(c)
	}
	return w
}

// View returns the part of the line that fits in width columns (one column is
// left free) and the cursor's column inside it.
func (e *Editor) View(width int) (string, int) {
	disp := make([]rune, len(e.buf))
	for i, r := range e.buf {
		if r == '\n' || r == '\r' {
			disp[i] = '↵'
		} else {
			disp[i] = visible(r)
		}
	}
	avail := width - 1
	if avail < 1 {
		avail = 1
	}
	// right context: up to a quarter of the width, never more than there is
	end := e.pos
	rw := 0
	for end < len(disp) && rw+runeWidth(disp[end]) <= avail/4 {
		rw += runeWidth(disp[end])
		end++
	}
	// left context fills the rest
	start := e.pos
	lw := 0
	for start > 0 && lw+runeWidth(disp[start-1])+rw <= avail {
		lw += runeWidth(disp[start-1])
		start--
	}
	return string(disp[start:end]), lw
}

// History remembers sent lines; Prev/Next walk through them like a shell.
type History struct {
	items []string
	idx   int // len(items) means "the draft"
	draft string
}

func (h *History) Push(s string) {
	if s != "" && (len(h.items) == 0 || h.items[len(h.items)-1] != s) {
		h.items = append(h.items, s)
	}
	h.idx = len(h.items)
}

func (h *History) Prev(current string) (string, bool) {
	if len(h.items) == 0 {
		return "", false
	}
	if h.idx == len(h.items) {
		h.draft = current
	}
	if h.idx > 0 {
		h.idx--
	}
	return h.items[h.idx], true
}

func (h *History) Next() (string, bool) {
	if h.idx >= len(h.items) {
		return "", false
	}
	h.idx++
	if h.idx == len(h.items) {
		return h.draft, true
	}
	return h.items[h.idx], true
}
