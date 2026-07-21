package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"time"

	"github.com/Gelmezon/grok-switch/internal/config"
	"github.com/Gelmezon/grok-switch/internal/exitcodes"
	"github.com/Gelmezon/grok-switch/internal/paths"
	"github.com/Gelmezon/grok-switch/internal/probe"
	"github.com/Gelmezon/grok-switch/internal/profiles"
	"github.com/Gelmezon/grok-switch/internal/secret"
	"github.com/Gelmezon/grok-switch/internal/switcher"
	"github.com/Gelmezon/grok-switch/internal/ui"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
	"github.com/Gelmezon/grok-switch/internal/ui/tui"
	"golang.org/x/term"
)

type appContext struct {
	Paths paths.Paths
	Store profiles.ProfileStore
}

func setup() (*appContext, error) {
	p, err := paths.Resolve()
	if err != nil {
		return nil, err
	}
	if err := paths.InitDirs(p); err != nil {
		return nil, err
	}
	return &appContext{
		Paths: p,
		Store: profiles.NewStore(p.ProfilesFile, p.LockFile),
	}, nil
}

func resolveProfile(store profiles.ProfileStore, nameOrID string) (profiles.Profile, error) {
	nameOrID = strings.TrimSpace(nameOrID)
	if nameOrID == "" {
		return profiles.Profile{}, &exitcodes.UsageError{Msg: "请指定供应商名称或 ID"}
	}
	if p, err := store.Get(nameOrID); err == nil {
		return p, nil
	}
	matches, err := store.GetByName(nameOrID)
	if err != nil {
		return profiles.Profile{}, err
	}
	if len(matches) == 0 {
		return profiles.Profile{}, &exitcodes.NotFoundError{Name: nameOrID}
	}
	if len(matches) > 1 {
		var ids []string
		for _, m := range matches {
			ids = append(ids, m.ID)
		}
		return profiles.Profile{}, &exitcodes.UsageError{
			Msg: fmt.Sprintf("存在多个名为 %q 的供应商，请改用 ID: %s", nameOrID, strings.Join(ids, ", ")),
		}
	}
	return matches[0], nil
}

func flagsAfterPositional(args []string) []string {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return args
	}
	return append(append([]string{}, args[1:]...), args[0])
}

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOut := fs.Bool("json", false, "")
	if err := fs.Parse(args); err != nil {
		return &exitcodes.UsageError{Msg: err.Error()}
	}
	ctx, err := setup()
	if err != nil {
		return err
	}
	list, err := ctx.Store.List()
	if err != nil {
		return err
	}
	if *jsonOut {
		pubs := make([]map[string]interface{}, 0, len(list))
		for _, p := range list {
			pubs = append(pubs, p.Public())
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pubs)
	}
	fmt.Print(ui.RenderProfileList(list, Version))
	return nil
}

func runStatus(args []string) error {
	_ = args
	ctx, err := setup()
	if err != nil {
		return err
	}
	st, err := switcher.ActiveStatus(ctx.Store, ctx.Paths.GrokConfig)
	if err != nil {
		return err
	}
	fmt.Print(ui.RenderStatus(st, Version))
	if st.HasActive && !st.DiskMatches {
		return &exitcodes.MismatchError{Msg: "磁盘配置与活动供应商不匹配"}
	}
	return nil
}

func runTest(args []string) error {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	timeout := fs.Duration("timeout", 20*time.Second, "")
	if err := fs.Parse(flagsAfterPositional(args)); err != nil {
		return &exitcodes.UsageError{Msg: err.Error()}
	}
	pos := fs.Args()
	if len(pos) < 1 {
		return &exitcodes.UsageError{Msg: "用法: grok-switch test <name-or-id>"}
	}
	ctx, err := setup()
	if err != nil {
		return err
	}
	p, err := resolveProfile(ctx.Store, pos[0])
	if err != nil {
		return err
	}
	var result probe.Result
	err = ui.WithSpinner(fmt.Sprintf("正在测试 %s / %s ...", p.Name, p.DefaultModel), func() error {
		result = probe.TestModel(p.BaseURL, p.APIKey, p.DefaultModel, *timeout)
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Print(ui.RenderTestResult(p.Name, result))
	if !result.OK {
		return fmt.Errorf("%s", result.Message)
	}
	return nil
}

func runShow(args []string) error {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	showKey := fs.Bool("show-key", false, "")
	jsonOut := fs.Bool("json", false, "")
	if err := fs.Parse(flagsAfterPositional(args)); err != nil {
		return &exitcodes.UsageError{Msg: err.Error()}
	}
	pos := fs.Args()
	if len(pos) < 1 {
		return &exitcodes.UsageError{Msg: "用法: grok-switch show <name-or-id>"}
	}
	ctx, err := setup()
	if err != nil {
		return err
	}
	p, err := resolveProfile(ctx.Store, pos[0])
	if err != nil {
		return err
	}
	if *jsonOut {
		pub := p.Public()
		if *showKey {
			pub["api_key"] = p.APIKey
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(pub)
	}
	fmt.Print(ui.RenderShow(p, *showKey))
	return nil
}

func runUse(args []string) error {
	fs := flag.NewFlagSet("use", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	yes := fs.Bool("yes", false, "Skip confirmation")
	if err := fs.Parse(flagsAfterPositional(args)); err != nil {
		return &exitcodes.UsageError{Msg: err.Error()}
	}
	pos := fs.Args()
	if len(pos) < 1 {
		return &exitcodes.UsageError{Msg: "用法: grok-switch use <name-or-id>"}
	}
	ctx, err := setup()
	if err != nil {
		return err
	}
	p, err := resolveProfile(ctx.Store, pos[0])
	if err != nil {
		return err
	}

	if ui.Interactive() && !*yes {
		st, _ := switcher.ActiveStatus(ctx.Store, ctx.Paths.GrokConfig)
		from := "（无）"
		if st.HasActive && st.Profile != nil {
			from = st.Profile.Name + "  " + st.Profile.DefaultModel
		}
		fmt.Println(theme.BoxWarning.Render(fmt.Sprintf(
			"%s  切换供应商\n\n从  %s\n到  %s  %s\n\n备份将自动保存到 %s\n\n确认？ [y/N] ",
			theme.SymWarn, from, p.Name, p.DefaultModel, ctx.Paths.BackupsDir,
		)))
		if !confirmYN(false) {
			fmt.Println(theme.Muted.Render("已取消"))
			return nil
		}
	}

	var res switcher.ActivateResult
	err = ui.WithSpinner("正在切换配置...", func() error {
		var e error
		res, e = switcher.Activate(p.ID, ctx.Store, ctx.Paths.GrokConfig, ctx.Paths.BackupsDir)
		return e
	})
	if err != nil {
		return err
	}
	fmt.Print(ui.RenderSwitchSuccess(res.Profile.Name, res.Profile.BaseURL, res.Profile.DefaultModel, res.BackupName))
	return nil
}

func runOfficial(args []string) error {
	fs := flag.NewFlagSet("official", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	yes := fs.Bool("yes", false, "")
	_ = fs.Parse(args)

	ctx, err := setup()
	if err != nil {
		return err
	}
	if ui.Interactive() && !*yes {
		fmt.Println(theme.BoxWarning.Render(fmt.Sprintf(
			"%s  切换到官方配置\n\n将移除中间站相关配置并恢复 Grok 官方认证（默认）。\n确认？ [y/N]",
			theme.SymWarn,
		)))
		if !confirmYN(false) {
			fmt.Println(theme.Muted.Render("已取消"))
			return nil
		}
	}
	var bak string
	err = ui.WithSpinner("正在切回官方配置...", func() error {
		var e error
		bak, e = switcher.ActivateOfficial(ctx.Store, ctx.Paths.GrokConfig, ctx.Paths.BackupsDir)
		return e
	})
	if err != nil {
		return err
	}
	fmt.Print(ui.RenderOfficialSuccess(bak))
	return nil
}

func runAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	baseURL := fs.String("base-url", "", "")
	model := fs.String("model", "", "")
	webSearch := fs.String("web-search-model", "", "")
	explore := fs.String("explore-model", "", "")
	plan := fs.String("plan-model", "", "")
	apiKeyStdin := fs.Bool("api-key-stdin", false, "")
	reasoning := fs.String("reasoning", "", "")
	if err := fs.Parse(flagsAfterPositional(args)); err != nil {
		return &exitcodes.UsageError{Msg: err.Error()}
	}
	pos := fs.Args()
	name := ""
	if len(pos) > 0 {
		name = pos[0]
	}

	ctx, err := setup()
	if err != nil {
		return err
	}

	nonInteractive := *baseURL != "" || *model != "" || *apiKeyStdin || os.Getenv("GROK_SWITCH_API_KEY") != "" || !ui.Interactive()

	var p profiles.Profile
	if nonInteractive {
		if name == "" {
			return &exitcodes.UsageError{Msg: "非交互模式需要名称: grok-switch add <name> --base-url ... --model ..."}
		}
		p.Name = name
		p.BaseURL = *baseURL
		p.DefaultModel = *model
		p.WebSearchModel = *webSearch
		p.SubagentsModels.Explore = *explore
		p.SubagentsModels.Plan = *plan
		if *reasoning != "" {
			p.DefaultReasoningEffort = *reasoning
		}
		key, err := readAPIKey(*apiKeyStdin)
		if err != nil {
			return err
		}
		p.APIKey = key
		if p.BaseURL == "" || p.DefaultModel == "" {
			return &exitcodes.UsageError{Msg: "非交互模式需要 --base-url 和 --model"}
		}
	} else {
		// Prefer bubbletea wizard when TTY available
		if ui.TUIEnabled() {
			return runAddWizard(ctx, name)
		}
		// Fallback line prompts
		return runAddPrompts(ctx, name)
	}

	created, err := ctx.Store.Create(p)
	if err != nil {
		return err
	}
	fmt.Println(theme.OK("已添加供应商 " + created.Name + " (id=" + created.ID + ")"))
	return nil
}

func runAddWizard(ctx *appContext, prefill string) error {
	done := make(chan error, 1)
	wiz := tui.NewAddWizard(prefill, func(p profiles.Profile) tea.Cmd {
		return func() tea.Msg {
			_, err := ctx.Store.Create(p)
			if err != nil {
				return tui.ErrMsg{Err: err}
			}
			done <- nil
			return tui.DoneMsg{}
		}
	})
	return runScreen(wiz, done)
}

func runAddPrompts(ctx *appContext, name string) error {
	in := bufio.NewReader(os.Stdin)
	var p profiles.Profile
	var err error
	if name == "" {
		name, err = promptLine(in, "Profile 名称: ", false)
		if err != nil {
			return err
		}
	}
	p.Name = name
	p.BaseURL, err = promptLine(in, "Base URL: ", false)
	if err != nil {
		return err
	}
	p.APIKey, err = readAPIKey(false)
	if err != nil {
		return err
	}
	p.DefaultModel, err = promptLine(in, "Default model: ", false)
	if err != nil {
		return err
	}
	ws, _ := promptLine(in, "Web search model [同默认]: ", true)
	if ws != "" {
		p.WebSearchModel = ws
	}
	ex, _ := promptLine(in, "Explore model [同默认]: ", true)
	if ex != "" {
		p.SubagentsModels.Explore = ex
	}
	pl, _ := promptLine(in, "Plan model [同默认]: ", true)
	if pl != "" {
		p.SubagentsModels.Plan = pl
	}
	created, err := ctx.Store.Create(p)
	if err != nil {
		return err
	}
	fmt.Println(theme.OK("已添加供应商 " + created.Name))
	fmt.Printf("  Base URL: %s\n  模型: %s\n  API Key: %s\n", created.BaseURL, created.DefaultModel, secret.MaskSecret(created.APIKey))
	return nil
}

func runEdit(args []string) error {
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	baseURL := fs.String("base-url", "", "")
	model := fs.String("model", "", "")
	apiKeyStdin := fs.Bool("api-key-stdin", false, "")
	if err := fs.Parse(flagsAfterPositional(args)); err != nil {
		return &exitcodes.UsageError{Msg: err.Error()}
	}
	pos := fs.Args()
	if len(pos) < 1 {
		return &exitcodes.UsageError{Msg: "用法: grok-switch edit <name-or-id>"}
	}
	ctx, err := setup()
	if err != nil {
		return err
	}
	cur, err := resolveProfile(ctx.Store, pos[0])
	if err != nil {
		return err
	}

	nonInteractive := *baseURL != "" || *model != "" || *apiKeyStdin || os.Getenv("GROK_SWITCH_API_KEY") != "" || !ui.Interactive()
	p := cur
	if nonInteractive {
		if *baseURL != "" {
			p.BaseURL = *baseURL
		}
		if *model != "" {
			p.DefaultModel = *model
		}
		if *apiKeyStdin || os.Getenv("GROK_SWITCH_API_KEY") != "" {
			key, err := readAPIKey(*apiKeyStdin)
			if err != nil {
				return err
			}
			if key != "" {
				p.APIKey = key
			}
		}
	} else if ui.TUIEnabled() {
		return runEditWizard(ctx, cur)
	} else {
		in := bufio.NewReader(os.Stdin)
		fmt.Printf("编辑 %s（回车保留当前值）\n", cur.Name)
		if v, _ := promptDefault(in, "Base URL", cur.BaseURL); v != "" {
			p.BaseURL = v
		}
		fmt.Printf("API Key [%s] (回车保留): ", secret.MaskSecret(cur.APIKey))
		if key, err := readPasswordOptional(); err == nil && key != "" {
			p.APIKey = key
		}
		if v, _ := promptDefault(in, "Default model", cur.DefaultModel); v != "" {
			p.DefaultModel = v
		}
	}
	updated, err := ctx.Store.Update(cur.ID, p)
	if err != nil {
		return err
	}
	fmt.Println(theme.OK("已更新供应商 " + updated.Name))
	return nil
}

func runEditWizard(ctx *appContext, cur profiles.Profile) error {
	done := make(chan error, 1)
	wiz := tui.NewEditWizard(cur, func(p profiles.Profile) tea.Cmd {
		return func() tea.Msg {
			_, err := ctx.Store.Update(cur.ID, p)
			if err != nil {
				return tui.ErrMsg{Err: err}
			}
			done <- nil
			return tui.DoneMsg{}
		}
	})
	return runScreen(wiz, done)
}

func runDelete(args []string) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	yes := fs.Bool("yes", false, "")
	if err := fs.Parse(flagsAfterPositional(args)); err != nil {
		return &exitcodes.UsageError{Msg: err.Error()}
	}
	pos := fs.Args()
	if len(pos) < 1 {
		return &exitcodes.UsageError{Msg: "用法: grok-switch delete <name-or-id>"}
	}
	ctx, err := setup()
	if err != nil {
		return err
	}
	p, err := resolveProfile(ctx.Store, pos[0])
	if err != nil {
		return err
	}
	if p.IsActive {
		return fmt.Errorf("%s 当前处于活动状态。\n请先执行 grok-switch official 或切换到其他供应商", p.Name)
	}
	if !*yes {
		fmt.Println(theme.BoxWarning.Render(fmt.Sprintf(
			"%s  删除供应商\n\n确定要删除 %s 吗？此操作无法撤销。\n（config.toml 不会被修改）\n\n输入供应商名称确认：",
			theme.SymWarn, p.Name,
		)))
		in := bufio.NewReader(os.Stdin)
		line, _ := in.ReadString('\n')
		if strings.TrimSpace(line) != p.Name {
			fmt.Println(theme.Muted.Render("已取消（名称不匹配）"))
			return nil
		}
	}
	if err := ctx.Store.Delete(p.ID); err != nil {
		return err
	}
	fmt.Println(theme.OK("已删除供应商 " + p.Name))
	return nil
}

func runImportCurrent(args []string) error {
	fs := flag.NewFlagSet("import-current", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	active := fs.Bool("active", false, "")
	if err := fs.Parse(flagsAfterPositional(args)); err != nil {
		return &exitcodes.UsageError{Msg: err.Error()}
	}
	pos := fs.Args()
	if len(pos) < 1 {
		return &exitcodes.UsageError{Msg: "用法: grok-switch import-current <name> [--active]"}
	}
	ctx, err := setup()
	if err != nil {
		return err
	}
	p, err := config.ImportFromConfig(ctx.Paths.GrokConfig, pos[0])
	if err != nil {
		return err
	}
	created, err := ctx.Store.Create(p)
	if err != nil {
		return err
	}
	fmt.Println(theme.OK("已从当前配置导入供应商 " + created.Name + " (id=" + created.ID + ")"))
	if *active {
		res, err := switcher.Activate(created.ID, ctx.Store, ctx.Paths.GrokConfig, ctx.Paths.BackupsDir)
		if err != nil {
			return fmt.Errorf("导入成功但激活失败: %w", err)
		}
		fmt.Print(ui.RenderSwitchSuccess(res.Profile.Name, res.Profile.BaseURL, res.Profile.DefaultModel, res.BackupName))
	}
	return nil
}

func runBackup(args []string) error {
	if len(args) < 1 {
		return &exitcodes.UsageError{Msg: "用法: grok-switch backup <list|restore|prune>"}
	}
	switch args[0] {
	case "list":
		return runBackupList()
	case "restore":
		return runBackupRestore(args[1:])
	case "prune":
		return runBackupPrune(args[1:])
	default:
		return &exitcodes.UsageError{Msg: "未知 backup 子命令: " + args[0]}
	}
}

func runBackupList() error {
	ctx, err := setup()
	if err != nil {
		return err
	}
	list, err := switcher.ListBackups(ctx.Paths.BackupsDir)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(list))
	for i, b := range list {
		size := int64(0)
		if fi, err := os.Stat(b.Path); err == nil {
			size = fi.Size()
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			b.Name,
			ui.HumanSize(size),
			b.ModTime.Local().Format("2006-01-02 15:04:05"),
		})
	}
	fmt.Print(ui.RenderBackupsWithSize(rows, ctx.Paths.BackupsDir, 10))
	return nil
}

func runBackupRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	yes := fs.Bool("yes", false, "")
	if err := fs.Parse(flagsAfterPositional(args)); err != nil {
		return &exitcodes.UsageError{Msg: err.Error()}
	}
	pos := fs.Args()
	if len(pos) < 1 {
		return &exitcodes.UsageError{Msg: "用法: grok-switch backup restore <文件名>"}
	}
	name := pos[0]
	ctx, err := setup()
	if err != nil {
		return err
	}
	if ui.Interactive() && !*yes {
		fmt.Println(theme.BoxWarning.Render(fmt.Sprintf(
			"%s  恢复备份\n\n恢复文件  %s\n\n将备份当前配置后覆盖，并清除活动 Profile。\n确认？ [y/N]",
			theme.SymWarn, name,
		)))
		if !confirmYN(false) {
			fmt.Println(theme.Muted.Render("已取消"))
			return nil
		}
	}
	var bak string
	err = ui.WithSpinner("正在恢复备份...", func() error {
		var e error
		bak, e = switcher.Restore(ctx.Store, ctx.Paths.BackupsDir, name, ctx.Paths.GrokConfig)
		return e
	})
	if err != nil {
		return err
	}
	fmt.Println(theme.OK("已恢复备份 " + name))
	fmt.Println(theme.Muted.Render("  恢复前配置已备份为: " + bak))
	fmt.Println(theme.Muted.Render("  活动 Profile 状态已清除"))
	return nil
}

func runBackupPrune(args []string) error {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	keep := fs.Int("keep", 10, "")
	if err := fs.Parse(args); err != nil {
		return &exitcodes.UsageError{Msg: err.Error()}
	}
	ctx, err := setup()
	if err != nil {
		return err
	}
	n, err := switcher.PruneBackups(ctx.Paths.BackupsDir, *keep)
	if err != nil {
		return err
	}
	fmt.Println(theme.OK(fmt.Sprintf("已清理 %d 个旧备份，保留最近 %d 个", n, *keep)))
	return nil
}

// --- helpers ---

func promptLine(in *bufio.Reader, label string, allowEmpty bool) (string, error) {
	fmt.Fprint(os.Stdout, label)
	line, err := in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" && !allowEmpty {
		return "", &exitcodes.UsageError{Msg: "输入不能为空"}
	}
	return line, nil
}

func promptDefault(in *bufio.Reader, label, def string) (string, error) {
	fmt.Fprintf(os.Stdout, "%s [%s]: ", label, def)
	line, err := in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

func readAPIKey(fromStdin bool) (string, error) {
	if fromStdin {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		key := strings.TrimSpace(string(raw))
		if key == "" {
			return "", &exitcodes.UsageError{Msg: "从 stdin 读取的 API Key 为空"}
		}
		return key, nil
	}
	if env := os.Getenv("GROK_SWITCH_API_KEY"); env != "" {
		return env, nil
	}
	if !ui.Interactive() {
		return "", &exitcodes.UsageError{Msg: "请设置 GROK_SWITCH_API_KEY 或使用 --api-key-stdin"}
	}
	fmt.Fprint(os.Stdout, "API Key: ")
	return readPasswordOptional()
}

func readPasswordOptional() (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stdout)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	in := bufio.NewReader(os.Stdin)
	line, err := in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func confirmYN(defaultYes bool) bool {
	in := bufio.NewReader(os.Stdin)
	line, _ := in.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defaultYes
	}
	return line == "y" || line == "yes"
}
