package client

import (
	"strings"
	"testing"
)

func TestParserKeys(t *testing.T) {
	cases := []struct {
		in   string
		want Key
	}{
		{"\x1b[D", Key{Kind: KLeft}},
		{"\x1b[C", Key{Kind: KRight}},
		{"\x1b[A", Key{Kind: KUp}},
		{"\x1b[B", Key{Kind: KDown}},
		{"\x1b[3~", Key{Kind: KDelete}},
		{"\x1b[H", Key{Kind: KHome}},
		{"\x1bOH", Key{Kind: KHome}},
		{"\x1b[1~", Key{Kind: KHome}},
		{"\x1b[F", Key{Kind: KEnd}},
		{"\x1b[4~", Key{Kind: KEnd}},
		{"\x7f", Key{Kind: KBackspace}},
		{"\x08", Key{Kind: KBackspace}},
		{"\r", Key{Kind: KEnter}},
		{"\n", Key{Kind: KEnter}},
		{"\x03", Key{Kind: KCtrlC}},
		{"\x04", Key{Kind: KCtrlD}},
		{"\x17", Key{Kind: KCtrlW}},
		{"é", Key{Kind: KRune, R: 'é'}},
	}
	for _, c := range cases {
		var p Parser
		got := p.Feed([]byte(c.in))
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("%q → %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestParserSplitSequences(t *testing.T) {
	var p Parser
	if got := p.Feed([]byte("\xc3")); len(got) != 0 {
		t.Fatalf("half a UTF-8 rune produced %+v", got)
	}
	if got := p.Feed([]byte("\xa9")); len(got) != 1 || got[0].R != 'é' {
		t.Fatalf("got %+v", got)
	}
	p.Feed([]byte("\x1b"))
	if got := p.Feed([]byte("[C")); len(got) != 1 || got[0].Kind != KRight {
		t.Fatalf("split escape: %+v", got)
	}
}

func TestParserBracketedPaste(t *testing.T) {
	var p Parser
	got := p.Feed([]byte("\x1b[200~línea1\r\nlínea2\x1b[201~"))
	if len(got) != 1 || got[0].Kind != KPaste || got[0].Text != "línea1\nlínea2" {
		t.Fatalf("got %+v", got)
	}
	// split across feeds, with the end marker arriving separately
	p = Parser{}
	var all []Key
	for _, part := range []string{"\x1b[200~uno\n", "dos", "\x1b[201", "~\r"} {
		all = append(all, p.Feed([]byte(part))...)
	}
	if len(all) != 2 || all[0].Kind != KPaste || all[0].Text != "uno\ndos" || all[1].Kind != KEnter {
		t.Fatalf("got %+v", all)
	}
}

func TestEditorInsertBackspaceUTF8(t *testing.T) {
	var e Editor
	e.Insert("aé😀")
	e.Backspace()
	if e.Text() != "aé" {
		t.Fatalf("%q", e.Text())
	}
	e.Left()
	e.Insert("X")
	if e.Text() != "aXé" {
		t.Fatalf("%q", e.Text())
	}
	e.Home()
	e.Delete()
	if e.Text() != "Xé" {
		t.Fatalf("%q", e.Text())
	}
	e.Backspace() // at start: no-op
	if e.Text() != "Xé" {
		t.Fatalf("%q", e.Text())
	}
}

func TestEditorKills(t *testing.T) {
	var e Editor
	e.Insert("hola mundo cruel")
	e.KillWord()
	if e.Text() != "hola mundo " {
		t.Fatalf("%q", e.Text())
	}
	e.Home()
	e.Right()
	e.Right()
	e.KillToEnd()
	if e.Text() != "ho" {
		t.Fatalf("%q", e.Text())
	}
	e.Insert("la")
	e.KillToStart()
	if e.Text() != "" {
		t.Fatalf("%q", e.Text())
	}
}

func TestEditorViewKeepsCursorVisible(t *testing.T) {
	var e Editor
	text := strings.Repeat("abcdefghij", 3)
	e.Insert(text)
	for _, pos := range []int{30, 20, 15, 5, 0} {
		e.Home()
		for i := 0; i < pos; i++ {
			e.Right()
		}
		view, col := e.View(20)
		if len([]rune(view)) > 19 || col > 19 {
			t.Errorf("pos %d: view %q col %d exceeds the width", pos, view, col)
		}
		if !strings.HasPrefix(text[pos:], string([]rune(view)[col:])) {
			t.Errorf("pos %d: cursor column %d does not match the text in %q", pos, col, view)
		}
	}
}

func TestEditorViewShowsNewlinesAsMarker(t *testing.T) {
	var e Editor
	e.Insert("uno\ndos")
	view, _ := e.View(40)
	if view != "uno↵dos" || e.Text() != "uno\ndos" {
		t.Fatalf("view %q text %q", view, e.Text())
	}
}

func TestHistory(t *testing.T) {
	var h History
	h.Push("uno")
	h.Push("dos")
	h.Push("dos") // consecutive duplicates collapse
	if s, ok := h.Prev("borrador"); !ok || s != "dos" {
		t.Fatalf("%q %v", s, ok)
	}
	if s, _ := h.Prev("dos"); s != "uno" {
		t.Fatalf("%q", s)
	}
	if s, _ := h.Prev("uno"); s != "uno" {
		t.Fatalf("stays at oldest: %q", s)
	}
	if s, _ := h.Next(); s != "dos" {
		t.Fatalf("%q", s)
	}
	if s, ok := h.Next(); !ok || s != "borrador" {
		t.Fatalf("returns to the draft: %q %v", s, ok)
	}
}
