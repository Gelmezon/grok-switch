package ui

import (
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	raw := "\x1b[32m●\x1b[0m name"
	if stripANSI(raw) != "● name" {
		t.Fatalf("got %q", stripANSI(raw))
	}
}

func TestDisplayWidthIgnoresANSI(t *testing.T) {
	plain := "hello"
	colored := "\x1b[32mhello\x1b[0m"
	if displayWidth(plain) != displayWidth(colored) {
		t.Fatalf("%d vs %d", displayWidth(plain), displayWidth(colored))
	}
}

func TestTableHideNarrow(t *testing.T) {
	tbl := Table{
		Columns: []Column{
			{Title: "A", Width: 4},
			{Title: "B", Width: 4, HideNarrow: true},
		},
		Rows:     [][]string{{"1", "2"}},
		MaxWidth: 60,
		Plain:    true,
	}
	out := tbl.Render()
	if strings.Contains(out, "B") {
		t.Fatalf("expected column B hidden on narrow: %q", out)
	}
	if !strings.Contains(out, "A") {
		t.Fatalf("expected column A: %q", out)
	}
}

func TestTablePadAlignment(t *testing.T) {
	tbl := Table{
		Columns: []Column{
			{Title: "NAME", Width: 8},
			{Title: "MODEL", Width: 8},
		},
		Rows: [][]string{
			{"relay-a", "grok-4"},
			{"b", "x"},
		},
		MaxWidth: 120,
		Plain:    false,
	}
	// Force pretty path without TTY by temporarily not using Plain and checking pad helper.
	line := pad("ab", 5)
	if line != "ab   " {
		t.Fatalf("pad = %q", line)
	}
	_ = tbl
}
