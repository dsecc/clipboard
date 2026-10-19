package client

import (
	"strings"
	"testing"
	"time"

	"clipboard/internal/model"
)

func noControls(s string) bool {
	for _, r := range s {
		if r == '\n' {
			continue
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return false
		}
	}
	return true
}

func TestFormatEntryNeutralizesControlCodes(t *testing.T) {
	now := time.Now()
	e := model.Entry{ID: "a", Kind: "text", Device: "pc\x1b[2J", CreatedAt: now,
		Text: "\x1b[31mrojo\x1b[0m\rsobrescribe\n\x1b]52;c;QUJD\x07fin"}
	got := FormatEntry(1, e, now)
	if !noControls(got) {
		t.Fatalf("raw control characters reach the terminal: %q", got)
	}
	for _, want := range []string{"rojo", "sobrescribe", "fin"} {
		if !strings.Contains(got, want) {
			t.Errorf("visible text %q lost in %q", want, got)
		}
	}
	if !strings.Contains(got, "\n") {
		t.Error("newlines must survive so multi-line entries keep their shape")
	}
}

func TestEditorViewNeutralizesControlCodes(t *testing.T) {
	var e Editor
	e.Insert("a\x1b[31mb\x07c")
	view, _ := e.View(60)
	if !noControls(view) || strings.Contains(view, "\n") {
		t.Fatalf("view has raw control characters: %q", view)
	}
	if e.Text() != "a\x1b[31mb\x07c" {
		t.Fatal("the text itself must stay untouched")
	}
}

func TestParseInputPastedIsVerbatim(t *testing.T) {
	cases := []struct{ in, want string }{
		{"q", "q"}, // pasted text that looks like a command is still text
		{"c 2", "c 2"},
		{`\\servidor\carpeta`, `\\servidor\carpeta`}, // no backslash stripped
		{"    sangría\n  def f():\n    pass\n", "    sangría\n  def f():\n    pass"},
	}
	for _, c := range cases {
		got := ParseInput(c.in, true)
		if got.Kind != Send || got.Text != c.want {
			t.Errorf("pasted %q → %+v, want Send %q", c.in, got, c.want)
		}
	}
	if got := ParseInput(" \n\n", true); got.Kind != Noop {
		t.Errorf("blank paste: %+v", got)
	}
	if got := ParseInput("q", false); got.Kind != Quit {
		t.Errorf("typed q must still quit: %+v", got)
	}
}
