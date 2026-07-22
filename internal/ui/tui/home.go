package tui

import (
	"fmt"
	"strings"

	"github.com/Gelmezon/grok-switch/internal/models"
	"github.com/Gelmezon/grok-switch/internal/profiles"
	"github.com/Gelmezon/grok-switch/internal/switcher"
	"github.com/Gelmezon/grok-switch/internal/ui"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Focus panels on the home screen (Tab cycles).
const (
	focusList = iota
	focusDetail
	focusOps
	focusCount
)

// listItem is either the built-in official entry or a stored provider.
type listItem struct {
	official bool
	profile  profiles.Profile
}

// HomeModel is the main provider browser.
type HomeModel struct {
	version       string
	configPath    string
	list          []profiles.Profile
	status        switcher.Status
	cursor        int
	focus         int
	filter        string
	filtering     bool
	filterBuf     string
	width         int
	height        int
	toast         string
	showBanner    bool
	latestVersion string
}

func NewHome(version, configPath string, list []profiles.Profile, st switcher.Status) *HomeModel {
	m := &HomeModel{
		version:    version,
		configPath: configPath,
		list:       list,
		status:     st,
		width:      100,
		height:     30,
		showBanner: true,
		focus:      focusList,
	}
	// Default cursor: official only when the on-disk config is official.
	if st.Mode == switcher.StatusOfficial {
		m.cursor = 0
	} else if st.Profile != nil {
		items := m.items()
		for i, it := range items {
			if !it.official && it.profile.IsActive {
				m.cursor = i
				break
			}
		}
	}
	return m
}

func (m *HomeModel) Init() tea.Cmd { return nil }

// items returns official first, then filtered providers.
func (m *HomeModel) items() []listItem {
	out := []listItem{{official: true}}
	q := strings.ToLower(m.filter)
	for _, p := range m.list {
		if q != "" {
			if !strings.Contains(strings.ToLower(p.Name), q) &&
				!strings.Contains(strings.ToLower(p.ID), q) &&
				!strings.Contains(strings.ToLower(p.DefaultModel), q) {
				continue
			}
		}
		out = append(out, listItem{profile: p})
	}
	// When filtering, hide official unless query matches
	if q != "" && !strings.Contains("官方 official grok", q) {
		out = out[1:]
	}
	return out
}

func (m *HomeModel) selected() *listItem {
	items := m.items()
	if len(items) == 0 || m.cursor < 0 || m.cursor >= len(items) {
		return nil
	}
	it := items[m.cursor]
	return &it
}

func (m *HomeModel) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items())-1 {
				m.cursor++
			}
		case "enter":
			sel := m.selected()
			if sel == nil {
				return m, nil
			}
			if sel.official {
				if m.status.Mode == switcher.StatusOfficial {
					m.toast = "当前已是官方配置"
					return m, nil
				}
				return m, func() tea.Msg { return requestOfficialMsg{} }
			}
			return m, func() tea.Msg { return requestUseMsg{ID: sel.profile.ID, Name: sel.profile.Name} }
		case "a":
			return m, func() tea.Msg { return requestAddMsg{} }
		case "e":
			sel := m.selected()
			if sel == nil || sel.official {
				if sel != nil && sel.official {
					m.toast = "官方配置无需编辑"
				}
				return m, nil
			}
			return m, func() tea.Msg { return requestEditMsg{Profile: sel.profile} }
		case "d":
			sel := m.selected()
			if sel == nil || sel.official {
				if sel != nil && sel.official {
					m.toast = "官方配置无法删除"
				}
				return m, nil
			}
			return m, func() tea.Msg { return requestDeleteMsg{Profile: sel.profile} }
		case "o":
			return m, func() tea.Msg { return requestOfficialMsg{} }
		case "b":
			return m, func() tea.Msg { return requestBackupMsg{} }
		case "s":
			return m, func() tea.Msg { return requestStatusMsg{} }
		case "t":
			sel := m.selected()
			if sel == nil || sel.official {
				if sel != nil && sel.official {
					m.toast = "官方配置无需连通性测试"
				}
				return m, nil
			}
			return m, func() tea.Msg { return requestTestMsg{Profile: sel.profile} }
		case "/":
			m.filtering = true
			m.filterBuf = m.filter
			m.focus = focusList
		case "r":
			return m, func() tea.Msg { return RefreshMsg{} }
		case "U":
			return m, func() tea.Msg { return requestUpdateMsg{} }
		case "esc":
			if m.filter != "" {
				m.filter = ""
				m.cursor = 0
			}
		}
	}
	return m, nil
}

type (
	pushHelpMsg        struct{}
	requestUseMsg      struct{ ID, Name string }
	requestAddMsg      struct{}
	requestEditMsg     struct{ Profile profiles.Profile }
	requestDeleteMsg   struct{ Profile profiles.Profile }
	requestOfficialMsg struct{}
	requestBackupMsg   struct{}
	requestStatusMsg   struct{}
	requestTestMsg     struct{ Profile profiles.Profile }
)

func (m *HomeModel) SetData(list []profiles.Profile, st switcher.Status) {
	m.list = list
	m.status = st
	if m.cursor >= len(m.items()) {
		m.cursor = max(0, len(m.items())-1)
	}
}

func (m *HomeModel) SetAvailableUpdate(version string) {
	m.latestVersion = version
}

func (m *HomeModel) View() string {
	var parts []string

	if m.showBanner && m.height >= 24 {
		parts = append(parts, ui.Banner(m.version))
	}

	// Top status bar
	activeName := models.OfficialName
	activeModel := "Grok 官方认证"
	if m.status.Profile != nil {
		activeName = m.status.Profile.Name
		activeModel = m.status.Profile.DefaultModel
	} else if m.status.Mode == switcher.StatusUnmanagedOrUnknown {
		activeName = "未托管或未知"
		activeModel = ""
	}
	topLeft := theme.Title.Render(fmt.Sprintf(" grok-switch  %s ", ui.VersionLabel(m.version)))
	topRight := theme.Success.Render(theme.SymActive + " " + activeName)
	if activeModel != "" {
		topRight += theme.Muted.Render("  " + activeModel)
	}
	topRight += theme.Muted.Render("   " + ui.Truncate(m.configPath, 36))
	if m.latestVersion != "" {
		topRight += theme.Warning.Render("   ↑ " + m.latestVersion)
	}
	barInner := lipgloss.JoinHorizontal(lipgloss.Top,
		topLeft,
		lipgloss.NewStyle().Width(max(1, m.width-6-lipgloss.Width(topLeft)-lipgloss.Width(topRight))).Render(""),
		topRight,
	)
	top := theme.BoxActive.Width(max(20, m.width-2)).Render(barInner)
	parts = append(parts, top)

	items := m.items()
	leftW := max(28, m.width/3)
	rightW := max(30, m.width-leftW-8)
	contentH := max(10, m.height-12)
	if m.showBanner && m.height >= 24 {
		contentH = max(8, m.height-18)
	}

	// Left list
	var left strings.Builder
	providerCount := len(m.list)
	left.WriteString(theme.Bold.Render(fmt.Sprintf("供应商  [%d]", providerCount)) + "\n\n")
	if m.filtering {
		left.WriteString(theme.Accent.Render("/ "+m.filterBuf+"_") + "\n\n")
	} else if m.filter != "" {
		left.WriteString(theme.Muted.Render("过滤: "+m.filter) + "\n\n")
	}
	if len(items) == 0 {
		left.WriteString(theme.Muted.Render("  （无匹配项）") + "\n")
	}
	for i, it := range items {
		var label string
		var isActive bool
		if it.official {
			isActive = m.status.Mode == switcher.StatusOfficial
			label = fmt.Sprintf("%s %s  官方认证", theme.SymActive, models.OfficialName)
			if !isActive {
				label = fmt.Sprintf("%s %s  官方认证", theme.SymInactive, models.OfficialName)
			}
		} else {
			isActive = it.profile.IsActive
			mark := theme.SymInactive
			if isActive {
				mark = theme.SymActive
			}
			label = fmt.Sprintf("%s %s  %s", mark, it.profile.Name, ui.Truncate(it.profile.DefaultModel, 12))
		}
		var line string
		if i == m.cursor {
			line = theme.Selected.Render(theme.SymCursor + " " + label)
		} else if isActive {
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
	right.WriteString(theme.Bold.Render("供应商详情") + "\n\n")
	sel := m.selected()
	if sel == nil {
		right.WriteString(theme.Muted.Render("选择一个供应商查看详情") + "\n")
	} else if sel.official {
		right.WriteString(theme.Field("名称", models.OfficialName) + "\n")
		right.WriteString(theme.Field("类型", "Grok 内置官方认证") + "\n")
		right.WriteString(theme.Field("说明", "不写入中间站 endpoints/model") + "\n\n")
		if m.status.Mode == switcher.StatusOfficial {
			right.WriteString(theme.Field("状态", theme.Success.Render(theme.SymActive+" 当前活动（默认）")) + "\n")
		} else if m.status.Mode == switcher.StatusUnmanagedOrUnknown && m.status.Profile == nil {
			right.WriteString(theme.Field("状态", theme.Warning.Render(theme.SymWarn+" 磁盘配置未托管或未知")) + "\n")
		} else {
			right.WriteString(theme.Field("状态", theme.Muted.Render(theme.SymInactive+" 非活动")) + "\n")
		}
		right.WriteString(theme.Field("配置文件", ui.Truncate(m.configPath, max(12, rightW-16))) + "\n")
	} else {
		p := sel.profile
		right.WriteString(theme.Field("名称", p.Name) + "\n")
		right.WriteString(theme.Field("Base URL", ui.Truncate(p.BaseURL, max(12, rightW-16))) + "\n")
		right.WriteString(theme.Field("推理等级", p.DefaultReasoningEffort) + "\n")
		privacy := theme.Warning.Render(theme.SymWarn + " 已允许")
		if p.CodebaseUploadDisabled() {
			privacy = theme.Success.Render(theme.SymOK + " 已禁止")
		}
		right.WriteString(theme.Field("源码上传", privacy) + "\n")
		right.WriteString(theme.Field("搜索模型", p.WebSearchModel) + "\n")
		right.WriteString(theme.Field("Explore", p.SubagentsModels.Explore) + "\n")
		right.WriteString(theme.Field("Plan", p.SubagentsModels.Plan) + "\n")
		right.WriteString(theme.Field("ID", p.ID) + "\n\n")
		if p.IsActive {
			right.WriteString(theme.Field("状态", theme.Success.Render(theme.SymActive+" 活动")) + "\n")
			if m.status.Mode == switcher.StatusManagedRelay && m.status.Profile != nil && m.status.Profile.ID == p.ID {
				right.WriteString(theme.Field("磁盘配置", theme.Success.Render(theme.SymOK+" 与供应商一致")) + "\n")
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

	opsStyle := theme.BoxNormal
	if m.focus == focusOps {
		opsStyle = theme.BoxActive
	}
	ops := opsStyle.Width(rightW).Render(
		theme.Bold.Render("操作") + "\n" +
			"Enter 切换  a 添加  e 编辑\n" +
			"d 删除      t 测试  o 官方\n" +
			"b 备份      s 状态  / 搜索\n" +
			"U 检查/更新 r 刷新",
	)

	rightCol := lipgloss.JoinVertical(lipgloss.Left, detailBox, ops)
	mid := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, "  ", rightCol)
	parts = append(parts, mid)

	help := theme.Help.Render("  ↑↓ 选择  Enter 切换  Tab 面板  t 测试模型  a 添加  U 检查/更新  ? 帮助  q 退出")
	if m.toast != "" {
		if strings.HasPrefix(m.toast, theme.SymFail) {
			help = theme.Error.Render("  "+m.toast) + "\n" + help
		} else {
			help = theme.Success.Render("  "+theme.SymOK+"  "+m.toast) + "\n" + help
		}
	}
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
