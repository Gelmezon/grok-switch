package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/Gelmezon/grok-switch/internal/models"
	"github.com/Gelmezon/grok-switch/internal/probe"
	"github.com/Gelmezon/grok-switch/internal/profiles"
	"github.com/Gelmezon/grok-switch/internal/secret"
	"github.com/Gelmezon/grok-switch/internal/switcher"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
)

// RenderProfileList formats providers for `list` command.
// Always shows built-in official first.
func RenderProfileList(list []profiles.Profile, version string) string {
	var b strings.Builder
	b.WriteString(HeaderLine("供应商", version) + "\n\n")

	hi := -1
	rows := make([][]string, 0, len(list)+1)
	activeCount := 0

	// Official row
	officialActive := true
	for _, p := range list {
		if p.IsActive {
			officialActive = false
			break
		}
	}
	offMark := ""
	if officialActive {
		offMark = theme.Success.Render(theme.SymActive)
		hi = 0
		activeCount++
	}
	rows = append(rows, []string{
		offMark,
		models.OfficialName,
		"官方认证",
		"—",
		models.OfficialID,
	})

	for _, p := range list {
		active := ""
		if p.IsActive {
			active = theme.Success.Render(theme.SymActive)
			hi = len(rows)
			activeCount++
		}
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
	summary := fmt.Sprintf("  共 %d 个中间站供应商 + 官方配置，%d 个活动中", len(list), activeCount)
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
			theme.Success.Render(theme.SymActive)+" 当前配置: "+theme.Bold.Render(models.OfficialName+"（默认）"),
			"",
			theme.Field("配置文件", st.ConfigPath),
			theme.Field("说明", "使用 Grok 官方认证，无活动中间站供应商"),
		)
		body = box(body, "success")
	case st.DiskMatches:
		p := st.Profile
		body = joinFields(
			theme.Success.Render(theme.SymActive)+" 活动供应商: "+theme.Bold.Render(p.Name),
			"",
			theme.Field("配置文件", st.ConfigPath),
			theme.Field("Base URL", p.BaseURL),
			theme.Field("默认模型", p.DefaultModel),
			theme.Field("推理等级", p.DefaultReasoningEffort),
			"",
			theme.Field("磁盘配置", theme.Success.Render(theme.SymOK+" 与供应商一致")),
		)
		body = box(body, "success")
	default:
		p := st.Profile
		body = joinFields(
			theme.Warning.Render(theme.SymWarn)+" 活动供应商: "+p.Name+"（配置不一致）",
			"",
			theme.Field("配置文件", st.ConfigPath),
			theme.Field("供应商期望", p.BaseURL),
			theme.Field("默认模型", p.DefaultModel),
			"",
			theme.Field("磁盘配置", theme.Error.Render(theme.SymFail+" 与供应商不一致")),
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
		theme.Success.Render(theme.SymOK)+"  已切换到 Grok 官方配置（默认）",
		"",
		theme.Field("备份", backup),
	)
	return box(body, "success") + "\n"
}

// RenderShow formats provider details.
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

// RenderTestResult formats a probe result.
func RenderTestResult(name string, r probe.Result) string {
	var body string
	if r.OK {
		body = joinFields(
			theme.Success.Render(theme.SymOK)+"  连通性测试通过",
			"",
			theme.Field("供应商", name),
			theme.Field("端点", r.Endpoint),
			theme.Field("结果", r.Message),
			theme.Field("耗时", fmt.Sprintf("%d ms", r.Latency.Milliseconds())),
		)
		return box(body, "success") + "\n"
	}
	body = joinFields(
		theme.Error.Render(theme.SymFail)+"  连通性测试失败",
		"",
		theme.Field("供应商", name),
		theme.Field("端点", r.Endpoint),
		theme.Field("HTTP", fmt.Sprintf("%d", r.StatusCode)),
		theme.Field("原因", r.Message),
		theme.Field("耗时", fmt.Sprintf("%d ms", r.Latency.Milliseconds())),
	)
	return box(body, "error") + "\n"
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
	if IsTTY() && theme.Enabled() {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
