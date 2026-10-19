package client

import (
	"strings"
	"testing"
	"time"

	"clipboard/internal/model"
)

func TestParseCommand(t *testing.T) {
	cases := []struct {
		in   string
		want Command
	}{
		{"c", Command{Kind: Copy}},
		{"c 3", Command{Kind: Copy, N: 3}},
		{"  c   3  ", Command{Kind: Copy, N: 3}},
		{"d 3", Command{Kind: Delete, N: 3}},
		{"c x", Command{Kind: Invalid, Text: "uso: c [número]"}},
		{"d", Command{Kind: Invalid, Text: "uso: d número"}},
		{"d 0", Command{Kind: Invalid, Text: "uso: d número"}},
		{"m", Command{Kind: Mouse}},
		{"q", Command{Kind: Quit}},
		{"exit", Command{Kind: Quit}},
		{"?", Command{Kind: Help}},
		{"help", Command{Kind: Help}},
		{"hola mundo", Command{Kind: Send, Text: "hola mundo"}},
		{`\c`, Command{Kind: Send, Text: "c"}},
		{`\d 3`, Command{Kind: Send, Text: "d 3"}},
		{"c 3 patatas", Command{Kind: Send, Text: "c 3 patatas"}},
		{"", Command{Kind: Noop}},
		{"   ", Command{Kind: Noop}},
		{"línea 1\nc", Command{Kind: Send, Text: "línea 1\nc"}},
	}
	for _, c := range cases {
		if got := ParseCommand(c.in); got != c.want {
			t.Errorf("ParseCommand(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestNumberingStableAfterDelete(t *testing.T) {
	n := NewNumbering()
	a, b, c := n.Assign("A"), n.Assign("B"), n.Assign("C")
	if a != 1 || b != 2 || c != 3 {
		t.Fatalf("numbers %d %d %d", a, b, c)
	}
	if n.Assign("B") != 2 {
		t.Fatal("re-assigning a known id must keep its number")
	}
	n.Remove("B")
	if _, ok := n.ID(2); ok {
		t.Fatal("removed entry still resolvable")
	}
	if d := n.Assign("D"); d != 4 {
		t.Fatalf("number reused: %d", d)
	}
	if id, ok := n.ID(3); !ok || id != "C" {
		t.Fatalf("ID(3) = %q %v", id, ok)
	}
	if id, ok := n.Last(); !ok || id != "D" {
		t.Fatalf("Last = %q %v", id, ok)
	}
	n.Remove("D")
	if id, _ := n.Last(); id != "C" {
		t.Fatalf("Last after removing newest = %q", id)
	}
}

func TestFormatEntry(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	text := model.Entry{ID: "a", Kind: "text", Text: "hola", Device: "netbook", CreatedAt: now.Add(-2 * time.Minute)}
	if got, want := FormatEntry(3, text, now), "[3] netbook · hace 2 min: hola"; got != want {
		t.Errorf("text: %q want %q", got, want)
	}
	multi := model.Entry{ID: "b", Kind: "text", Text: "uno\ndos", Device: "pc", CreatedAt: now.Add(-10 * time.Second)}
	if got, want := FormatEntry(4, multi, now), "[4] pc · ahora:\n    uno\n    dos"; got != want {
		t.Errorf("multi: %q want %q", got, want)
	}
	img := model.Entry{ID: "c", Kind: "image", Size: 120 * 1024, Device: "iPhone", CreatedAt: now.Add(-3 * time.Hour)}
	got := FormatEntry(5, img, now)
	if !strings.Contains(got, ": [imagen 120 KB]") || !strings.HasPrefix(got, "[5] iPhone · hace 3 h:") {
		t.Errorf("image: %q", got)
	}
}
