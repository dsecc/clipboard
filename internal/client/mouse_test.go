package client

import "testing"

func TestParserMouseAndCursorReports(t *testing.T) {
	cases := []struct {
		in   string
		want Key
	}{
		{"\x1b[<0;12;7M", Key{Kind: KMouse, X: 12, Y: 7, Btn: 0, Press: true}},
		{"\x1b[<0;12;7m", Key{Kind: KMouse, X: 12, Y: 7, Btn: 0, Press: false}},
		{"\x1b[<64;3;4M", Key{Kind: KMouse, X: 3, Y: 4, Btn: 64, Press: true}},
		{"\x1b[24;1R", Key{Kind: KCursor, Y: 24, X: 1}},
		{"\x1b[1;5C", Key{Kind: KRight}}, // Ctrl+Right is still "right", not stray characters
		{"\x1b[1;2D", Key{Kind: KLeft}},
	}
	for _, c := range cases {
		var p Parser
		got := p.Feed([]byte(c.in))
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("%q → %+v, want %+v", c.in, got, c.want)
		}
	}
	var p Parser
	if got := p.Feed([]byte("\x1b[<0;1")); len(got) != 0 {
		t.Fatalf("incomplete mouse report produced %+v", got)
	}
	if got := p.Feed([]byte("2;7M")); len(got) != 1 || got[0].X != 12 || got[0].Y != 7 {
		t.Fatalf("split mouse report: %+v", got)
	}
	// unknown sequences are swallowed whole instead of leaking characters into the line
	if got := p.Feed([]byte("\x1b[15~x")); len(got) != 1 || got[0].R != 'x' {
		t.Fatalf("F5 leaked: %+v", got)
	}
}

func TestRowMapFindsTheEntryUnderAClick(t *testing.T) {
	var m RowMap
	m.Add("a", "12345678901234567890x", 20) // 21 columns wide: wraps into 2 rows
	m.Add("b", "corto\nsegunda línea", 20)  // 2 rows
	m.Add("", "aviso", 20)                  // a notice: no entry
	// screen rows above the prompt (prompt on row 10): 9 notice, 8 b, 7 b, 6 a, 5 a, 4 nothing
	cases := []struct {
		click int
		want  string
	}{{9, ""}, {8, "b"}, {7, "b"}, {6, "a"}, {5, "a"}, {4, ""}, {10, ""}, {11, ""}, {0, ""}}
	for _, c := range cases {
		if got := m.Lookup(10, c.click); got != c.want {
			t.Errorf("click on row %d → %q, want %q", c.click, got, c.want)
		}
	}
}

func TestRowMapCountsWideCharactersAndLongLines(t *testing.T) {
	var m RowMap
	m.Add("w", "😀😀😀", 4) // 6 columns in a 4-column terminal: 2 rows
	m.Add("l", "x", 4)
	if m.Lookup(5, 4) != "l" || m.Lookup(5, 3) != "w" || m.Lookup(5, 2) != "w" || m.Lookup(5, 1) != "" {
		t.Fatalf("rows: %q %q %q %q", m.Lookup(5, 4), m.Lookup(5, 3), m.Lookup(5, 2), m.Lookup(5, 1))
	}
}

func TestRowMapForgetsOldRows(t *testing.T) {
	var m RowMap
	for i := 0; i < 3*maxRows; i++ {
		m.Add("x", "linea", 80)
	}
	if m.Len() > maxRows {
		t.Fatalf("kept %d rows, limit is %d", m.Len(), maxRows)
	}
}
