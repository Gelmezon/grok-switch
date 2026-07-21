package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Gelmezon/grok-switch/internal/switcher"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
)

// StatusModel is a full-screen status card (TUI overlay).
type StatusModel struct {
	status switcher.Status
	width  int
}

// NewStatusScreen creates a status overlay.
func NewStatusScreen(st switcher.Status) *StatusModel {
	return &StatusModel{status: st, width: 60}
}

func (m *StatusModel) Init() tea.Cmd { return nil }

func (m *StatusModel) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 20 {
			m.width = min(msg.Width-4, 72)
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "enter", "s":
			return m, func() tea.Msg { return DoneMsg{} }
		}
	}
	return m, nil
}

func (m *StatusModel) View() string {
	st := m.status
	var body strings.Builder
	body.WriteString(theme.Title.Render("grok-switch  当前状态") + "\n\n")

	switch {
	case !st.HasActive:
		body.WriteString(theme.Muted.Render(theme.SymInactive+" 无活动 Profile") + "\n\n")
		body.WriteString(theme.Field("配置文件", st.ConfigPath) + "\n")
		body.WriteString(theme.Field("当前配置", "使用 Grok 官方认证") + "\n")
		return theme.BoxNormal.Width(m.width).Render(body.String() + "\n" + theme.Help.Render("Esc 返回"))
	case st.DiskMatches:
		p := st.Profile
		body.WriteString(theme.Success.Render(theme.SymActive+" 活动 Profile: "+p.Name) + "\n\n")
		body.WriteString(theme.Field("配置文件", st.ConfigPath) + "\n")
		body.WriteString(theme.Field("Base URL", p.BaseURL) + "\n")
		body.WriteString(theme.Field("默认模型", p.DefaultModel) + "\n")
		body.WriteString(theme.Field("推理等级", p.DefaultReasoningEffort) + "\n\n")
		body.WriteString(theme.Field("磁盘配置", theme.Success.Render(theme.SymOK+" 与 Profile 一致")) + "\n")
		return theme.BoxSuccess.Width(m.width).Render(body.String() + "\n" + theme.Help.Render("Esc 返回"))
	default:
		p := st.Profile
		body.WriteString(theme.Warning.Render(theme.SymWarn+" 活动 Profile: "+p.Name+"（配置不一致）") + "\n\n")
		body.WriteString(theme.Field("配置文件", st.ConfigPath) + "\n")
		body.WriteString(theme.Field("Profile 期望", p.BaseURL) + "\n")
		body.WriteString(theme.Field("默认模型", p.DefaultModel) + "\n\n")
		body.WriteString(theme.Field("磁盘配置", theme.Error.Render(theme.SymFail+" 与 Profile 不一致")) + "\n")
		body.WriteString(theme.Field("建议运行", fmt.Sprintf("grok-switch use %s", p.Name)) + "\n")
		return theme.BoxWarning.Width(m.width).Render(body.String() + "\n" + theme.Help.Render("Esc 返回"))
	}
}
