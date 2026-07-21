package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
)

// ConfirmModel is a reusable yes/no dialog.
type ConfirmModel struct {
	title   string
	body    string
	okLabel string
	cancel  string
	focusOK bool // true = confirm focused
	// allowY enables y shortcut for confirm
	allowY bool
	// typeName requires typing this string to confirm (delete protection)
	typeName string
	input    string
	width    int
	onOK     func() tea.Cmd
}

// NewConfirm creates a standard confirm dialog.
func NewConfirm(title, body, okLabel string, allowY bool, onOK func() tea.Cmd) *ConfirmModel {
	return &ConfirmModel{
		title:   title,
		body:    body,
		okLabel: okLabel,
		cancel:  "取消",
		focusOK: true,
		allowY:  allowY,
		width:   56,
		onOK:    onOK,
	}
}

// NewTypeConfirm requires typing name to confirm (for delete).
func NewTypeConfirm(title, body, name string, onOK func() tea.Cmd) *ConfirmModel {
	return &ConfirmModel{
		title:    title,
		body:     body,
		okLabel:  "确认删除",
		cancel:   "取消",
		focusOK:  false,
		allowY:   false,
		typeName: name,
		width:    56,
		onOK:     onOK,
	}
}

func (m *ConfirmModel) Init() tea.Cmd { return nil }

func (m *ConfirmModel) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 20 {
			m.width = min(msg.Width-4, 72)
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "n", "ctrl+c":
			return m, func() tea.Msg { return DoneMsg{} }
		case "y":
			if m.allowY && m.typeName == "" {
				return m, m.onOK()
			}
		case "tab", "left", "right", "h", "l":
			if m.typeName == "" {
				m.focusOK = !m.focusOK
			}
		case "enter":
			if m.typeName != "" {
				if m.input == m.typeName {
					return m, m.onOK()
				}
				return m, nil
			}
			if m.focusOK {
				return m, m.onOK()
			}
			return m, func() tea.Msg { return DoneMsg{} }
		case "backspace":
			if m.typeName != "" && len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		default:
			if m.typeName != "" && len(msg.String()) == 1 {
				m.input += msg.String()
			}
		}
	}
	return m, nil
}

func (m *ConfirmModel) View() string {
	var b strings.Builder
	b.WriteString(theme.Warning.Render(theme.SymWarn+"  "+m.title) + "\n\n")
	b.WriteString(m.body + "\n\n")
	if m.typeName != "" {
		b.WriteString("输入 Profile 名称确认：> " + m.input + "_\n\n")
		ok := theme.Muted.Render("[ "+m.okLabel+" ]")
		if m.input == m.typeName {
			ok = theme.Error.Render("[ "+m.okLabel+" ]")
		}
		cancel := theme.Accent.Render("[ "+m.cancel+" ]")
		b.WriteString(ok + "   " + cancel + "\n")
	} else {
		ok := "[ " + m.okLabel + " ]"
		cancel := "[ " + m.cancel + " ]"
		if m.focusOK {
			ok = theme.Selected.Render(ok)
			cancel = theme.Muted.Render(cancel)
		} else {
			ok = theme.Muted.Render(ok)
			cancel = theme.Selected.Render(cancel)
		}
		b.WriteString(ok + "   " + cancel + "\n")
	}
	content := b.String()
	return theme.BoxWarning.Width(m.width).Render(content)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
