package tui

import tea "github.com/charmbracelet/bubbletea"

// Screen is a stackable TUI view.
type Screen interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Screen, tea.Cmd)
	View() string
}

// result messages used between screens
type (
	// DoneMsg pops current screen (optional payload).
	DoneMsg struct{ Payload interface{} }
	// QuitMsg requests full app exit.
	QuitMsg struct{}
	// RefreshMsg asks home to reload data.
	RefreshMsg struct{}
	// ErrMsg displays an error toast then continues.
	ErrMsg struct{ Err error }
	// InfoMsg is a transient success/info banner.
	InfoMsg struct{ Text string }
)
