package theme

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Brand / semantic colors (truecolor).
var (
	ColorPrimary = lipgloss.Color("#7C3AED")
	ColorAccent  = lipgloss.Color("#06B6D4")
	ColorSuccess = lipgloss.Color("#10B981")
	ColorWarning = lipgloss.Color("#F59E0B")
	ColorError   = lipgloss.Color("#EF4444")
	ColorInfo    = lipgloss.Color("#3B82F6")
	ColorMuted   = lipgloss.Color("#6B7280")
	ColorBorder  = lipgloss.Color("#374151")
	ColorBg      = lipgloss.Color("#111827")
	ColorText    = lipgloss.Color("#F9FAFB")

	FallbackActive  = lipgloss.Color("2")
	FallbackWarning = lipgloss.Color("3")
	FallbackError   = lipgloss.Color("1")
	FallbackMuted   = lipgloss.Color("8")
)

// Symbols.
const (
	SymActive   = "●"
	SymInactive = "○"
	SymOK       = "✓"
	SymFail     = "✗"
	SymWarn     = "⚠"
	SymCursor   = "→"
	SymInfo     = "◆"
)

// Styles — initialized in Init.
var (
	BoxNormal  lipgloss.Style
	BoxActive  lipgloss.Style
	BoxSuccess lipgloss.Style
	BoxError   lipgloss.Style
	BoxWarning lipgloss.Style

	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Success  lipgloss.Style
	Error    lipgloss.Style
	Warning  lipgloss.Style
	Info     lipgloss.Style
	Muted    lipgloss.Style
	Accent   lipgloss.Style
	Primary  lipgloss.Style
	Bold     lipgloss.Style
	ActiveRow lipgloss.Style
	Selected  lipgloss.Style
	Help     lipgloss.Style
	Label    lipgloss.Style
	Value    lipgloss.Style
)

var enabled bool

// Init configures lipgloss for the current terminal capability.
func Init(colorOn bool) {
	enabled = colorOn
	if !colorOn {
		lipgloss.SetColorProfile(termenv.Ascii)
	} else if os.Getenv("COLORTERM") == "truecolor" || os.Getenv("COLORTERM") == "24bit" {
		lipgloss.SetColorProfile(termenv.TrueColor)
	}

	primary := ColorPrimary
	success := ColorSuccess
	warning := ColorWarning
	errC := ColorError
	info := ColorInfo
	muted := ColorMuted
	border := ColorBorder
	accent := ColorAccent
	text := ColorText

	if colorOn && !trueColor() {
		success = FallbackActive
		warning = FallbackWarning
		errC = FallbackError
		muted = FallbackMuted
	}

	BoxNormal = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1)
	BoxActive = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primary).
		Padding(0, 1)
	BoxSuccess = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(success).
		Padding(0, 1)
	BoxError = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(errC).
		Padding(0, 1)
	BoxWarning = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(warning).
		Padding(0, 1)

	Title = lipgloss.NewStyle().Bold(true).Foreground(primary)
	Subtitle = lipgloss.NewStyle().Foreground(muted)
	Success = lipgloss.NewStyle().Foreground(success)
	Error = lipgloss.NewStyle().Foreground(errC)
	Warning = lipgloss.NewStyle().Foreground(warning)
	Info = lipgloss.NewStyle().Foreground(info)
	Muted = lipgloss.NewStyle().Foreground(muted)
	Accent = lipgloss.NewStyle().Foreground(accent)
	Primary = lipgloss.NewStyle().Foreground(primary)
	Bold = lipgloss.NewStyle().Bold(true).Foreground(text)
	ActiveRow = lipgloss.NewStyle().Bold(true).Foreground(success)
	Selected = lipgloss.NewStyle().Reverse(true)
	Help = lipgloss.NewStyle().Foreground(muted)
	Label = lipgloss.NewStyle().Foreground(muted).Width(12)
	Value = lipgloss.NewStyle().Foreground(text)
}

func trueColor() bool {
	ct := os.Getenv("COLORTERM")
	return ct == "truecolor" || ct == "24bit"
}

// Enabled reports whether color styling is active.
func Enabled() bool { return enabled }

// OK prefixes a success message.
func OK(msg string) string {
	return Success.Render(SymOK+"  ") + msg
}

// Fail prefixes an error message.
func Fail(msg string) string {
	return Error.Render(SymFail+"  ") + msg
}

// WarnMsg prefixes a warning.
func WarnMsg(msg string) string {
	return Warning.Render(SymWarn+"  ") + msg
}

// Field renders "label  value" aligned.
func Field(label, value string) string {
	return Label.Render(label) + "  " + Value.Render(value)
}
