package cli

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/Gelmezon/grok-switch/internal/ui/tui"
)

// screenProg wraps a single Screen as a tea.Model for standalone wizards.
type screenProg struct {
	screen tui.Screen
	done   chan error
	result error
	quit   bool
}

func (m *screenProg) Init() tea.Cmd { return m.screen.Init() }

func (m *screenProg) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quit = true
			m.result = nil
			return m, tea.Quit
		}
	case tui.QuitMsg:
		m.quit = true
		return m, tea.Quit
	case tui.DoneMsg:
		// Wizard finished (create/update already ran in onDone, or cancel).
		m.quit = true
		if m.done != nil {
			select {
			case err := <-m.done:
				m.result = err
			default:
			}
		}
		return m, tea.Quit
	case tui.ErrMsg:
		m.result = msg.Err
		return m, tea.Quit
	}
	next, cmd := m.screen.Update(msg)
	m.screen = next
	return m, cmd
}

func (m *screenProg) View() string {
	return m.screen.View()
}

// runScreen runs a TUI screen until Done/Quit.
func runScreen(s tui.Screen, done chan error) error {
	m := &screenProg{screen: s, done: done}
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return err
	}
	if sp, ok := final.(*screenProg); ok && sp.result != nil {
		return sp.result
	}
	// drain done channel if still pending success
	if done != nil {
		select {
		case err := <-done:
			return err
		default:
		}
	}
	return nil
}
