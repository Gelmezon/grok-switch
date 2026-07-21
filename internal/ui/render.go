package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/Gelmezon/grok-switch/internal/profiles"
	"github.com/Gelmezon/grok-switch/internal/secret"
	"github.com/Gelmezon/grok-switch/internal/switcher"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
)

// RenderProfileList formats profiles for `list` command.
func RenderProfileList(list []profiles.Profile, version string) string {
	var b strings.Builder
	b.WriteString(HeaderLine("Profiles", version) + "\n\n")

	if len(list) == 0 {
		b.WriteString(theme.Muted.Render("  （无 Profile）") + "\n")
		return b.String()
	}

	hi := -1
	rows := make([][]string, 0, len(list))
	activeCount := 0
	for i, p := range list {
		active := ""
		if p.IsActive {
			active = theme.Success.Render(theme.SymActive)
			hi = i
			activeCount++
		}
		// Pass full URL; Table truncates by terminal width (wide ≥120 keeps more).
		rows = append(rows, []string{
			active,
			p.Name,
			p.DefaultModel,
			p.BaseURL,
			p.ID,
		})
	}

	tbl := Table{
		Columns: []Column{
			{Title: "ACTIVE", Width: 6, Priority: 10},
			{Title: "NAME", Width: 12, Priority: 9},
			{Title: "MODEL", Width: 14, Priority: 7},
			{Title: "BASE URL", Width: 36, Priority: 5},
			{Title: "ID", Width: 16, HideNarrow: true, Priority: 1},
		},
		Rows:         rows,
		HighlightRow: hi,
		Plain:        !IsTTY(),
	}
	b.WriteString(tbl.Render())
	b.WriteString("\n")
	summary := fmt.Sprintf("  共 %d 个 Profile，%d 个活动中", len(list), activeCount)
	b.WriteString(theme.Muted.Render(summary) + "\n")
	return b.String()
}

// RenderStatus formats status command output.
func RenderStatus(st switcher.Status, version string) string {
	var b strings.Builder
	b.WriteString(HeaderLine("当前状态", version) + "\n\n")

	var body string
	switch {
	case !st.HasActive:
		body = joinFields(
			theme.Muted.Render(theme.SymInactive)+" 无活动 Profile",
			"",
			theme.Field("配置文件", st.ConfigPath),
			theme.Field("当前配置", "使用 Grok 官方认证"),
		)
		body = box(body, "normal")
	case st.DiskMatches:
		p := st.Profile
		body = joinFields(
			theme.Success.Render(theme.SymActive)+" 活动 Profile: "+theme.Bold.Render(p.Name),
			"",
			theme.Field("配置文件", st.ConfigPath),
			theme.Field("Base URL", p.BaseURL),
			theme.Field("默认模型", p.DefaultModel),
			theme.Field("推理等级", p.DefaultReasoningEffort),
			"",
			theme.Field("磁盘配置", theme.Success.Render(theme.SymOK+" 与 Profile 一致")),
		)
		body = box(body, "success")
	default:
		p := st.Profile
		body = joinFields(
			theme.Warning.Render(theme.SymWarn)+" 活动 Profile: "+p.Name+"（配置不一致）",
			"",
			theme.Field("配置文件", st.ConfigPath),
			theme.Field("Profile 期望", p.BaseURL),
			theme.Field("默认模型", p.DefaultModel),
			"",
			theme.Field("磁盘配置", theme.Error.Render(theme.SymFail+" 与 Profile 不一致")),
			theme.Field("建议运行", "grok-switch use "+p.Name+"  重新同步"),
		)
		body = box(body, "warning")
	}
	b.WriteString("  " + indent(body, "  ") + "\n")
	return b.String()
}

// RenderSwitchSuccess formats a successful use result.
func RenderSwitchSuccess(name, baseURL, model, backup string) string {
	body := joinFields(
		theme.Success.Render(theme.SymOK)+"  已切换到 "+theme.Bold.Render(name),
		"",
		theme.Field("Base URL", baseURL),
		theme.Field("模型", model),
		theme.Field("备份", backup),
		"",
		theme.Warning.Render(theme.SymWarn)+"  已运行的 Grok 会话不会自动重载，请新开会话",
	)
	return box(body, "success") + "\n"
}

// RenderOfficialSuccess formats official mode result.
func RenderOfficialSuccess(backup string) string {
	body := joinFields(
		theme.Success.Render(theme.SymOK)+"  已切回 Grok 官方认证配置",
		"",
		theme.Field("备份", backup),
	)
	return box(body, "success") + "\n"
}

// RenderShow formats profile details.
func RenderShow(p profiles.Profile, showKey bool) string {
	key := secret.MaskSecret(p.APIKey)
	if showKey {
		key = p.APIKey
	}
	active := theme.Muted.Render(theme.SymInactive + " 非活动")
	if p.IsActive {
		active = theme.Success.Render(theme.SymActive + " 活动")
	}
	body := joinFields(
		theme.Field("ID", p.ID),
		theme.Field("名称", p.Name),
		theme.Field("Base URL", p.BaseURL),
		theme.Field("默认模型", p.DefaultModel),
		theme.Field("搜索模型", p.WebSearchModel),
		theme.Field("Explore", p.SubagentsModels.Explore),
		theme.Field("Plan", p.SubagentsModels.Plan),
		theme.Field("推理等级", p.DefaultReasoningEffort),
		theme.Field("API Key", key),
		theme.Field("状态", active),
	)
	return box(body, "normal") + "\n"
}

// RenderBackups formats backup list.
func RenderBackups(list []switcher.BackupInfo, backupsDir string, keep int) string {
	var b strings.Builder
	b.WriteString(HeaderLine("备份列表", "") + "\n\n")

	if len(list) == 0 {
		b.WriteString(theme.Muted.Render("  （无备份）") + "\n")
		return b.String()
	}
	rows := make([][]string, 0, len(list))
	for i, bk := range list {
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			bk.Name,
			bk.ModTime.Local().Format("2006-01-02 15:04:05"),
		})
	}
	tbl := Table{
		Columns: []Column{
			{Title: "#", Width: 3},
			{Title: "文件名", Width: 44},
			{Title: "时间", Width: 19},
		},
		Rows:         rows,
		HighlightRow: 0,
		Plain:        !IsTTY(),
	}
	b.WriteString(tbl.Render())
	b.WriteString("\n")
	b.WriteString(theme.Muted.Render(fmt.Sprintf("  共 %d 个备份，保留最近 %d 个", len(list), keep)) + "\n")
	b.WriteString(theme.Muted.Render("  备份目录: "+backupsDir) + "\n")
	return b.String()
}

// RenderBackupsWithSize includes file sizes.
func RenderBackupsWithSize(rows [][]string, backupsDir string, keep int) string {
	var b strings.Builder
	b.WriteString(HeaderLine("备份列表", "") + "\n\n")
	if len(rows) == 0 {
		b.WriteString(theme.Muted.Render("  （无备份）") + "\n")
		return b.String()
	}
	tbl := Table{
		Columns: []Column{
			{Title: "#", Width: 3},
			{Title: "文件名", Width: 44},
			{Title: "大小", Width: 8},
			{Title: "时间", Width: 19},
		},
		Rows:         rows,
		HighlightRow: 0,
		Plain:        !IsTTY(),
	}
	b.WriteString(tbl.Render())
	b.WriteString("\n")
	b.WriteString(theme.Muted.Render(fmt.Sprintf("  共 %d 个备份，保留最近 %d 个", len(rows), keep)) + "\n")
	b.WriteString(theme.Muted.Render("  备份目录: "+backupsDir) + "\n")
	return b.String()
}

func box(content, kind string) string {
	if !IsTTY() || !theme.Enabled() {
		// plain indent
		lines := strings.Split(content, "\n")
		for i, l := range lines {
			lines[i] = "  " + l
		}
		return strings.Join(lines, "\n")
	}
	var style lipgloss.Style
	switch kind {
	case "success":
		style = theme.BoxSuccess
	case "error":
		style = theme.BoxError
	case "warning":
		style = theme.BoxWarning
	case "active":
		style = theme.BoxActive
	default:
		style = theme.BoxNormal
	}
	return style.Render(content)
}

func joinFields(lines ...string) string {
	return strings.Join(lines, "\n")
}

func indent(s, prefix string) string {
	// lipgloss boxes already have padding; for plain mode we indent.
	if IsTTY() && theme.Enabled() {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
