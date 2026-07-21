package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Gelmezon/grok-switch/internal/profiles"
	"github.com/Gelmezon/grok-switch/internal/switcher"
	"github.com/Gelmezon/grok-switch/internal/ui"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
)

// Focus panels on the home screen (Tab cycles).
const (
	focusList = iota
	focusDetail
	focusOps
	focusCount
)

// HomeModel is the main profile browser.
type HomeModel struct {
	version    string
	configPath string
	list       []profiles.Profile
	status     switcher.Status
	cursor     int
	focus      int // focusList | focusDetail | focusOps
	filter     string
	filtering  bool
	filterBuf  string
	width      int
	height     int
	toast      string
	showBanner bool
}

func NewHome(version, configPath string, list []profiles.Profile, st switcher.Status) *HomeModel {
	return &HomeModel{
		version:    version,
		configPath: configPath,
		list:       list,
		status:     st,
		width:      100,
		height:     30,
		showBanner: true,
		focus:      focusList,
	}
}

func (m *HomeModel) Init() tea.Cmd { return nil }

func (m *HomeModel) filtered() []profiles.Profile {
	if m.filter == "" {
		return m.list
	}
	var out []profiles.Profile
	q := strings.ToLower(m.filter)
	for _, p := range m.list {
		if strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.ID), q) ||
			strings.Contains(strings.ToLower(p.DefaultModel), q) {
			out = append(out, p)
		}
	}
	return out
}

func (m *HomeModel) selected() *profiles.Profile {
	items := m.filtered()
	if len(items) == 0 || m.cursor < 0 || m.cursor >= len(items) {
		return nil
	}
	p := items[m.cursor]
	return &p
}

func (m *HomeModel) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Hide full ASCII banner on short terminals to save vertical space.
		m.showBanner = msg.Height >= 24
	case InfoMsg:
		m.toast = msg.Text
	case tea.KeyMsg:
		if m.filtering {
			switch msg.String() {
			case "esc":
				m.filtering = false
				m.filterBuf = ""
			case "enter":
				m.filter = m.filterBuf
				m.filtering = false
				m.cursor = 0
			case "backspace":
				if len(m.filterBuf) > 0 {
					m.filterBuf = m.filterBuf[:len(m.filterBuf)-1]
				}
			default:
				if len(msg.String()) == 1 {
					m.filterBuf += msg.String()
				}
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, func() tea.Msg { return QuitMsg{} }
		case "?":
			return m, func() tea.Msg { return pushHelpMsg{} }
		case "tab":
			m.focus = (m.focus + 1) % focusCount
			return m, nil
		case "shift+tab":
			m.focus = (m.focus - 1 + focusCount) % focusCount
			return m, nil
		case "up", "k":
			// Navigation always moves list cursor (detail/ops are read-only panels).
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.filtered())-1 {
				m.cursor++
			}
		case "enter":
			sel := m.selected()
			if sel == nil {
				return m, nil
			}
			return m, func() tea.Msg { return requestUseMsg{ID: sel.ID, Name: sel.Name} }
		case "a":
			return m, func() tea.Msg { return requestAddMsg{} }
		case "e":
			sel := m.selected()
			if sel == nil {
				return m, nil
			}
			return m, func() tea.Msg { return requestEditMsg{Profile: *sel} }
		case "d":
			sel := m.selected()
			if sel == nil {
				return m, nil
			}
			return m, func() tea.Msg { return requestDeleteMsg{Profile: *sel} }
		case "o":
			return m, func() tea.Msg { return requestOfficialMsg{} }
		case "b":
			return m, func() tea.Msg { return requestBackupMsg{} }
		case "s":
			return m, func() tea.Msg { return requestStatusMsg{} }
		case "/":
			m.filtering = true
			m.filterBuf = m.filter
			m.focus = focusList
		case "r":
			return m, func() tea.Msg { return RefreshMsg{} }
		case "esc":
			if m.filter != "" {
				m.filter = ""
				m.cursor = 0
			}
		}
	}
	return m, nil
}

// messages for app router
type (
	pushHelpMsg        struct{}
	requestUseMsg      struct{ ID, Name string }
	requestAddMsg      struct{}
	requestEditMsg     struct{ Profile profiles.Profile }
	requestDeleteMsg   struct{ Profile profiles.Profile }
	requestOfficialMsg struct{}
	requestBackupMsg   struct{}
	requestStatusMsg   struct{}
)

func (m *HomeModel) SetData(list []profiles.Profile, st switcher.Status) {
	m.list = list
	m.status = st
	if m.cursor >= len(m.filtered()) {
		m.cursor = max(0, len(m.filtered())-1)
	}
}

func (m *HomeModel) View() string {
	var parts []string

	if m.showBanner && m.height >= 24 {
		parts = append(parts, ui.Banner(m.version))
	}

	// Top status bar
	activeName := "无活动"
	activeModel := ""
	if m.status.HasActive && m.status.Profile != nil {
		activeName = m.status.Profile.Name
		activeModel = m.status.Profile.DefaultModel
	}
	topLeft := theme.Title.Render(fmt.Sprintf(" grok-switch  v%s ", m.version))
	topRight := theme.Success.Render(theme.SymActive + " " + activeName)
	if activeModel != "" {
		topRight += theme.Muted.Render("  " + activeModel)
	}
	topRight += theme.Muted.Render("   " + ui.Truncate(m.configPath, 36))
	barInner := lipgloss.JoinHorizontal(lipgloss.Top,
		topLeft,
		lipgloss.NewStyle().Width(max(1, m.width-6-lipgloss.Width(topLeft)-lipgloss.Width(topRight))).Render(""),
		topRight,
	)
	top := theme.BoxActive.Width(max(20, m.width-2)).Render(barInner)
	parts = append(parts, top)

	items := m.filtered()
	leftW := max(28, m.width/3)
	rightW := max(30, m.width-leftW-8)
	contentH := max(10, m.height-12)
	if m.showBanner && m.height >= 24 {
		contentH = max(8, m.height-18)
	}

	// Left list
	var left strings.Builder
	left.WriteString(theme.Bold.Render(fmt.Sprintf("Profiles  [%d]", len(items))) + "\n\n")
	if m.filtering {
		left.WriteString(theme.Accent.Render("/ "+m.filterBuf+"_") + "\n\n")
	} else if m.filter != "" {
		left.WriteString(theme.Muted.Render("过滤: "+m.filter) + "\n\n")
	}
	if len(items) == 0 {
		left.WriteString(theme.Muted.Render("  （无 Profile）") + "\n")
	}
	for i, p := range items {
		mark := theme.SymInactive
		if p.IsActive {
			mark = theme.SymActive
		}
		label := fmt.Sprintf("%s %s  %s", mark, p.Name, ui.Truncate(p.DefaultModel, 12))
		var line string
		if i == m.cursor {
			line = theme.Selected.Render(theme.SymCursor + " " + label)
		} else if p.IsActive {
			line = "  " + theme.ActiveRow.Render(label)
		} else {
			line = "  " + theme.Muted.Render(label)
		}
		left.WriteString(line + "\n")
	}
	leftBoxStyle := theme.BoxNormal
	if m.focus == focusList {
		leftBoxStyle = theme.BoxActive
	}
	leftBox := leftBoxStyle.Width(leftW).Height(contentH).Render(left.String())

	// Right detail
	var right strings.Builder
	right.WriteString(theme.Bold.Render("Profile 详情") + "\n\n")
	sel := m.selected()
	if sel == nil {
		right.WriteString(theme.Muted.Render("选择一个 Profile 查看详情") + "\n")
	} else {
		right.WriteString(theme.Field("名称", sel.Name) + "\n")
		right.WriteString(theme.Field("Base URL", ui.Truncate(sel.BaseURL, max(12, rightW-16))) + "\n")
		right.WriteString(theme.Field("模型", sel.DefaultModel) + "\n")
		right.WriteString(theme.Field("推理等级", sel.DefaultReasoningEffort) + "\n")
		right.WriteString(theme.Field("搜索模型", sel.WebSearchModel) + "\n")
		right.WriteString(theme.Field("Explore", sel.SubagentsModels.Explore) + "\n")
		right.WriteString(theme.Field("Plan", sel.SubagentsModels.Plan) + "\n")
		right.WriteString(theme.Field("ID", sel.ID) + "\n\n")
		if sel.IsActive {
			right.WriteString(theme.Field("状态", theme.Success.Render(theme.SymActive+" 活动")) + "\n")
			if m.status.DiskMatches {
				right.WriteString(theme.Field("磁盘配置", theme.Success.Render(theme.SymOK+" 与 Profile 一致")) + "\n")
			} else {
				right.WriteString(theme.Field("磁盘配置", theme.Warning.Render(theme.SymWarn+" 不一致")) + "\n")
			}
		} else {
			right.WriteString(theme.Field("状态", theme.Muted.Render(theme.SymInactive+" 非活动")) + "\n")
		}
	}
	detailStyle := theme.BoxNormal
	if m.focus == focusDetail {
		detailStyle = theme.BoxActive
	}
	detailBox := detailStyle.Width(rightW).Render(right.String())

	// Ops box
	opsStyle := theme.BoxNormal
	if m.focus == focusOps {
		opsStyle = theme.BoxActive
	}
	ops := opsStyle.Width(rightW).Render(
		theme.Bold.Render("操作") + "\n" +
			"Enter 切换  a 添加  e 编辑\n" +
			"d 删除      o 官方  b 备份\n" +
			"s 状态      / 搜索  r 刷新",
	)

	rightCol := lipgloss.JoinVertical(lipgloss.Left, detailBox, ops)
	mid := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, "  ", rightCol)
	parts = append(parts, mid)

	help := theme.Help.Render("  ↑↓ 选择  Enter 切换  Tab 切换面板  / 搜索  s 状态  ? 帮助  q 退出")
	if m.toast != "" {
		help = theme.Success.Render("  "+theme.SymOK+"  "+m.toast) + "\n" + help
	}
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
