package tui

import (
	"strings"

	"github.com/Gelmezon/grok-switch/internal/ui/theme"
	tea "github.com/charmbracelet/bubbletea"
)

// HelpModel is an overlay help panel.
type HelpModel struct {
	width int
}

func NewHelp() *HelpModel {
	return &HelpModel{width: 68}
}

func (m *HelpModel) Init() tea.Cmd { return nil }

func (m *HelpModel) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 20 {
			m.width = min(msg.Width-4, 78)
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "?", "enter":
			return m, func() tea.Msg { return DoneMsg{} }
		}
	}
	return m, nil
}

func (m *HelpModel) View() string {
	lines := []string{
		theme.Title.Render("grok-switch 命令帮助"),
		"",
		theme.Accent.Render("供应商管理"),
		theme.Muted.Render("─────────────────────────────────────────────────────────"),
		"  add [name]        添加中间站供应商（交互向导，无高级步骤）",
		"  list              列出供应商（含默认官方配置）",
		"  show <name|id>    查看供应商详情",
		"  edit <name|id>    编辑供应商（含高级模型）",
		"  delete <name|id>  删除供应商",
		"  test <name|id>    测试模型连通性",
		"",
		theme.Accent.Render("切换操作"),
		theme.Muted.Render("─────────────────────────────────────────────────────────"),
		"  use <name|id>     切换到指定供应商",
		"  status            查看当前状态（退出码 3=配置不一致）",
		"  official          切回 Grok 官方配置（默认）",
		"",
		theme.Accent.Render("备份管理"),
		theme.Muted.Render("─────────────────────────────────────────────────────────"),
		"  backup list / restore / prune",
		"",
		theme.Accent.Render("主界面快捷键"),
		theme.Muted.Render("─────────────────────────────────────────────────────────"),
		"  ↑↓/jk 选择  Enter 切换  Tab 面板  a 添加  e 编辑  d 删除",
		"  t 测试模型  o 官方  b 备份  s 状态  / 搜索  r 刷新  U 检查/更新  ? 帮助  q 退出",
		"",
		theme.Help.Render("q / Esc 关闭帮助"),
	}
	return theme.BoxNormal.Width(m.width).Render(strings.Join(lines, "\n"))
}
