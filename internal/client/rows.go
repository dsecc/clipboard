package client

import "strings"

const maxRows = 4000

// RowMap remembers which entry each printed screen row belongs to, so a mouse
// click on the terminal can be traced back to an entry.
type RowMap struct {
	rows []string // oldest first; "" is a row that belongs to no entry
}

func (m *RowMap) Len() int { return len(m.rows) }

// Add records the rows that text takes in a terminal width columns wide.
func (m *RowMap) Add(id, text string, width int) {
	if width < 1 {
		width = 1
	}
	for _, line := range strings.Split(text, "\n") {
		n := (runesWidth([]rune(line)) + width - 1) / width
		if n < 1 {
			n = 1
		}
		for i := 0; i < n; i++ {
			m.rows = append(m.rows, id)
		}
	}
	if len(m.rows) > maxRows {
		m.rows = append([]string(nil), m.rows[len(m.rows)-maxRows:]...)
	}
}

// Lookup returns the entry shown on screen row clickRow, given that the prompt
// is on row promptRow (rows count from 1 at the top). "" means no entry there.
func (m *RowMap) Lookup(promptRow, clickRow int) string {
	above := promptRow - clickRow // 1 = the row right above the prompt
	if clickRow < 1 || above < 1 || above > len(m.rows) {
		return ""
	}
	return m.rows[len(m.rows)-above]
}
