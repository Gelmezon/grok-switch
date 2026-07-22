package tui

import (
	"fmt"
	"strings"

	"github.com/Gelmezon/grok-switch/internal/models"
	"github.com/Gelmezon/grok-switch/internal/switcher"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
	tea "github.com/charmbracelet/bubbletea"
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

	switch st.Mode {
	case switcher.StatusOfficial:
		body.WriteString(theme.Success.Render(theme.SymActive+" 当前: "+models.OfficialName+"（默认官方配置）") + "\n\n")
		body.WriteString(theme.Field("配置文件", st.ConfigPath) + "\n")
		body.WriteString(theme.Field("说明", "使用 Grok 官方认证") + "\n")
		return theme.BoxSuccess.Width(m.width).Render(body.String() + "\n" + theme.Help.Render("Esc 返回"))
	case switcher.StatusManagedRelay:
		p := st.Profile
		body.WriteString(theme.Success.Render(theme.SymActive+" 活动供应商: "+p.Name) + "\n\n")
		body.WriteString(theme.Field("配置文件", st.ConfigPath) + "\n")
		body.WriteString(theme.Field("Base URL", p.BaseURL) + "\n")
		body.WriteString(theme.Field("默认模型", p.DefaultModel) + "\n")
		body.WriteString(theme.Field("推理等级", p.DefaultReasoningEffort) + "\n")
		privacy := theme.Warning.Render(theme.SymWarn + " 已允许")
		if p.CodebaseUploadDisabled() {
			privacy = theme.Success.Render(theme.SymOK + " 已禁止")
		}
		body.WriteString(theme.Field("源码上传", privacy) + "\n\n")
		body.WriteString(theme.Field("磁盘配置", theme.Success.Render(theme.SymOK+" 与供应商一致")) + "\n")
		return theme.BoxSuccess.Width(m.width).Render(body.String() + "\n" + theme.Help.Render("Esc 返回"))
	default:
		if st.Profile != nil {
			p := st.Profile
			body.WriteString(theme.Warning.Render(theme.SymWarn+" 活动供应商: "+p.Name+"（配置不一致）") + "\n\n")
			body.WriteString(theme.Field("配置文件", st.ConfigPath) + "\n")
			body.WriteString(theme.Field("供应商期望", p.BaseURL) + "\n")
			body.WriteString(theme.Field("默认模型", p.DefaultModel) + "\n\n")
			body.WriteString(theme.Field("磁盘配置", theme.Error.Render(theme.SymFail+" 与供应商不一致")) + "\n")
			body.WriteString(theme.Field("建议运行", fmt.Sprintf("grok-switch use %s", p.Name)) + "\n")
		} else {
			body.WriteString(theme.Warning.Render(theme.SymWarn+" 当前: 未托管或未知配置") + "\n\n")
			body.WriteString(theme.Field("配置文件", st.ConfigPath) + "\n")
			body.WriteString(theme.Field("说明", "检测到中间站字段，但没有活动供应商记录") + "\n")
			body.WriteString(theme.Field("建议运行", "grok-switch use <name> 或 grok-switch official") + "\n")
		}
		return theme.BoxWarning.Width(m.width).Render(body.String() + "\n" + theme.Help.Render("Esc 返回"))
	}
}
