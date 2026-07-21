package ui

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/Gelmezon/grok-switch/internal/ui/theme"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// Column describes a table column.
type Column struct {
	Title    string
	Width    int  // preferred min width; 0 = auto from content
	Priority int  // higher = keep when narrow (default 0)
	HideNarrow bool // hide when terminal < 80
}

// Table renders a simple aligned table.
type Table struct {
	Columns []Column
	Rows    [][]string
	// HighlightRow marks a row index as active (bold + success). -1 = none.
	HighlightRow int
	// Plain forces tab-separated no-border output.
	Plain bool
	// MaxWidth overrides auto terminal width detection (0 = detect).
	MaxWidth int
}

// Render returns the formatted table string.
func (t Table) Render() string {
	if t.Plain || !IsTTY() {
		return t.renderPlain()
	}
	return t.renderPretty()
}

func (t Table) renderPlain() string {
	cols := t.visibleColumns()
	var b strings.Builder
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = c.col.Title
	}
	b.WriteString(strings.Join(headers, "\t"))
	b.WriteByte('\n')
	for _, row := range t.Rows {
		cells := make([]string, len(cols))
		for i, c := range cols {
			if c.idx < len(row) {
				cells[i] = stripANSI(row[c.idx])
			}
		}
		b.WriteString(strings.Join(cells, "\t"))
		b.WriteByte('\n')
	}
	return b.String()
}

type colRef struct {
	idx int
	col Column
}

func (t Table) visibleColumns() []colRef {
	termW := t.MaxWidth
	if termW <= 0 {
		termW = detectWidth()
	}
	narrow := termW > 0 && termW < 80
	var out []colRef
	for i, c := range t.Columns {
		if narrow && c.HideNarrow {
			continue
		}
		out = append(out, colRef{idx: i, col: c})
	}
	if len(out) == 0 {
		for i, c := range t.Columns {
			out = append(out, colRef{idx: i, col: c})
		}
	}
	return out
}

func (t Table) renderPretty() string {
	cols := t.visibleColumns()
	termW := t.MaxWidth
	if termW <= 0 {
		termW = detectWidth()
	}
	wide := termW >= 120

	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = c.col.Width
		if widths[i] == 0 {
			widths[i] = displayWidth(c.col.Title)
		}
		if wide && c.col.Width > 0 {
			// give URL-like columns more room
			if strings.Contains(strings.ToUpper(c.col.Title), "URL") {
				widths[i] = maxInt(widths[i], 56)
			}
		}
	}
	for _, row := range t.Rows {
		for i, c := range cols {
			if c.idx < len(row) {
				w := displayWidth(row[c.idx])
				if w > widths[i] {
					widths[i] = w
				}
			}
		}
	}

	// Cap total width if needed
	if termW > 20 {
		total := 2 // leading spaces
		for _, w := range widths {
			total += w + 2
		}
		if total > termW {
			// shrink largest column
			for total > termW {
				mi := 0
				for i := 1; i < len(widths); i++ {
					if widths[i] > widths[mi] {
						mi = i
					}
				}
				if widths[mi] <= 8 {
					break
				}
				widths[mi]--
				total--
			}
		}
	}

	var b strings.Builder
	headers := make([]string, len(cols))
	rules := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = pad(c.col.Title, widths[i])
		rules[i] = strings.Repeat("─", widths[i])
	}
	headerLine := "  " + strings.Join(headers, "  ")
	ruleLine := "  " + strings.Join(rules, "  ")
	if theme.Enabled() {
		b.WriteString(theme.Muted.Render(headerLine) + "\n")
		b.WriteString(theme.Muted.Render(ruleLine) + "\n")
	} else {
		b.WriteString(headerLine + "\n" + ruleLine + "\n")
	}

	for ri, row := range t.Rows {
		cells := make([]string, len(cols))
		for i, c := range cols {
			val := ""
			if c.idx < len(row) {
				val = row[c.idx]
			}
			cells[i] = padColored(val, widths[i])
		}
		line := "  " + strings.Join(cells, "  ")
		switch {
		case ri == t.HighlightRow && theme.Enabled():
			// re-render highlight without embedded colors to avoid double-style mess
			plainCells := make([]string, len(cols))
			for i, c := range cols {
				val := ""
				if c.idx < len(row) {
					val = stripANSI(row[c.idx])
				}
				plainCells[i] = pad(val, widths[i])
			}
			b.WriteString(theme.ActiveRow.Render("  "+strings.Join(plainCells, "  ")) + "\n")
		default:
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func displayWidth(s string) int {
	return runewidth.StringWidth(stripANSI(s))
}

func pad(s string, width int) string {
	s = stripANSI(s)
	w := runewidth.StringWidth(s)
	if w > width {
		return runewidth.Truncate(s, width, "…")
	}
	return s + strings.Repeat(" ", width-w)
}

// padColored pads a possibly-colored string to display width.
func padColored(s string, width int) string {
	w := displayWidth(s)
	if w > width {
		// truncate plain then lose color for safety
		return pad(s, width)
	}
	return s + strings.Repeat(" ", width-w)
}

// Truncate shortens s to max display width with ellipsis.
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= max {
		return s
	}
	if max <= 2 {
		return runewidth.Truncate(s, max, "")
	}
	return runewidth.Truncate(s, max, "…")
}

// HumanSize formats byte size.
func HumanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func detectWidth() int {
	if !IsTTY() {
		return 100
	}
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		// COLUMNS env fallback
		if c := os.Getenv("COLUMNS"); c != "" {
			if n, err := strconv.Atoi(c); err == nil && n > 0 {
				return n
			}
		}
		return 100
	}
	return w
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
