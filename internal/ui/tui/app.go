package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/Gelmezon/grok-switch/internal/paths"
	"github.com/Gelmezon/grok-switch/internal/probe"
	"github.com/Gelmezon/grok-switch/internal/profiles"
	"github.com/Gelmezon/grok-switch/internal/switcher"
	"github.com/Gelmezon/grok-switch/internal/ui"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
	"github.com/Gelmezon/grok-switch/internal/updater"
	tea "github.com/charmbracelet/bubbletea"
)

// App is the root Bubbletea model with a screen stack.
type App struct {
	version string
	paths   paths.Paths
	store   profiles.ProfileStore
	stack   []Screen
	width   int
	height  int
	updates *updater.Service
	update  *updater.Info
}

// Config for launching the TUI.
type Config struct {
	Version string
	Paths   paths.Paths
	Store   profiles.ProfileStore
}

// Run starts the full-screen TUI.
func Run(cfg Config) error {
	ui.InitTheme()
	home, err := loadHome(cfg)
	if err != nil {
		return err
	}
	app := &App{
		version: cfg.Version,
		paths:   cfg.Paths,
		store:   cfg.Store,
		stack:   []Screen{home},
		updates: updater.NewService(cfg.Version, cfg.Paths.UpdateStateFile),
	}
	p := tea.NewProgram(app, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func loadHome(cfg Config) (*HomeModel, error) {
	list, err := cfg.Store.List()
	if err != nil {
		return nil, err
	}
	st, err := switcher.ActiveStatus(cfg.Store, cfg.Paths.GrokConfig)
	if err != nil {
		st = switcher.Status{ConfigPath: cfg.Paths.GrokConfig}
	}
	return NewHome(cfg.Version, cfg.Paths.GrokConfig, list, st), nil
}

func (a *App) Init() tea.Cmd {
	var cmds []tea.Cmd
	if len(a.stack) > 0 {
		cmds = append(cmds, a.stack[len(a.stack)-1].Init())
	}
	if cmd := a.checkUpdateCmd(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

func (a *App) push(s Screen) tea.Cmd {
	a.stack = append(a.stack, s)
	return s.Init()
}

func (a *App) pop() {
	if len(a.stack) > 1 {
		a.stack = a.stack[:len(a.stack)-1]
	}
}

func (a *App) top() Screen {
	if len(a.stack) == 0 {
		return nil
	}
	return a.stack[len(a.stack)-1]
}

func (a *App) refreshHome() {
	list, err := a.store.List()
	if err != nil {
		return
	}
	st, _ := switcher.ActiveStatus(a.store, a.paths.GrokConfig)
	for i := range a.stack {
		if h, ok := a.stack[i].(*HomeModel); ok {
			h.SetData(list, st)
		}
	}
}

func (a *App) setToast(text string) {
	if h, ok := a.top().(*HomeModel); ok {
		h.toast = text
	}
	if b, ok := a.top().(*BackupModel); ok {
		b.status = text
	}
}

func (a *App) setAvailableUpdate(version string) {
	for i := range a.stack {
		if h, ok := a.stack[i].(*HomeModel); ok {
			h.SetAvailableUpdate(version)
		}
	}
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

	case QuitMsg:
		return a, tea.Quit

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}

	case DoneMsg:
		a.pop()
		a.refreshHome()
		switch payload := msg.Payload.(type) {
		case InfoMsg:
			a.setToast(payload.Text)
		case updateInstalledMsg:
			a.update = nil
			a.setAvailableUpdate("")
			a.setToast(fmt.Sprintf("已更新到 %s，退出并重新启动后生效", payload.Result.Version))
		case updateInstallFailedMsg:
			a.setToast(theme.SymFail + "  " + payload.Err.Error())
		}
		return a, nil

	case RefreshMsg:
		a.refreshHome()
		return a, nil

	case ErrMsg:
		a.setToast(theme.SymFail + "  " + msg.Err.Error())
		return a, nil

	case InfoMsg:
		a.setToast(msg.Text)
		// also refresh backup list if on backup screen
		if b, ok := a.top().(*BackupModel); ok {
			list, _ := switcher.ListBackups(a.paths.BackupsDir)
			b.list = list
			b.status = msg.Text
		}
		return a, nil

	case updateAvailableMsg:
		a.update = &msg.Info
		a.setAvailableUpdate(msg.Info.LatestVersion)
		if msg.AutoErr != nil {
			a.setToast(fmt.Sprintf("发现 %s；自动更新失败：%v", msg.Info.LatestVersion, msg.AutoErr))
		} else {
			a.setToast(fmt.Sprintf("发现新版本 %s，按 U 更新", msg.Info.LatestVersion))
		}
		return a, nil

	case updateInstalledMsg:
		a.update = nil
		a.setAvailableUpdate("")
		a.setToast(fmt.Sprintf("已更新到 %s，退出并重新启动后生效", msg.Result.Version))
		return a, nil

	case updateManualCheckMsg:
		if msg.Err != nil {
			a.setToast(theme.SymFail + "  检查更新失败：" + msg.Err.Error())
			return a, nil
		}
		if !msg.Info.Available {
			a.setToast(fmt.Sprintf("当前已是最新版本 %s", msg.Info.CurrentVersion))
			return a, nil
		}
		a.update = &msg.Info
		a.setAvailableUpdate(msg.Info.LatestVersion)
		return a, a.push(a.newUpdateConfirm(msg.Info))

	case pushHelpMsg:
		return a, a.push(NewHelp())

	case requestStatusMsg:
		st, err := switcher.ActiveStatus(a.store, a.paths.GrokConfig)
		if err != nil {
			st = switcher.Status{ConfigPath: a.paths.GrokConfig}
		}
		return a, a.push(NewStatusScreen(st))

	case requestAddMsg:
		return a, a.push(NewWizard("add", profiles.Profile{}, a.doCreate))

	case requestEditMsg:
		return a, a.push(NewWizard("edit", msg.Profile, a.doUpdate(msg.Profile.ID)))

	case requestDeleteMsg:
		p := msg.Profile
		if p.IsActive {
			a.setToast(fmt.Sprintf("%s 处于活动状态，无法删除", p.Name))
			return a, nil
		}
		body := fmt.Sprintf("确定要删除供应商 %s 吗？此操作无法撤销。\n（config.toml 不会被修改）", p.Name)
		return a, a.push(NewTypeConfirm("删除供应商", body, p.Name, func() tea.Cmd {
			return func() tea.Msg {
				if err := a.store.Delete(p.ID); err != nil {
					return ErrMsg{Err: err}
				}
				return DoneMsg{Payload: InfoMsg{Text: "已删除供应商 " + p.Name}}
			}
		}))

	case requestUseMsg:
		p, err := a.store.Get(msg.ID)
		if err != nil {
			return a, func() tea.Msg { return ErrMsg{Err: err} }
		}
		cur := theme.SymActive + " 官方"
		st, _ := switcher.ActiveStatus(a.store, a.paths.GrokConfig)
		if st.Profile != nil {
			cur = theme.SymActive + " " + st.Profile.Name + "  " + st.Profile.DefaultModel
		} else if st.Mode == switcher.StatusUnmanagedOrUnknown {
			cur = theme.SymWarn + " 未托管或未知配置"
		}
		body := fmt.Sprintf("从  %s\n到  %s %s  %s\n\n切换时将自动同步中间站 /models。\n备份将自动保存到 %s",
			cur, theme.SymInactive, p.Name, p.DefaultModel, a.paths.BackupsDir)
		return a, a.push(NewConfirm("切换供应商", body, "确认切换", true, func() tea.Cmd {
			return func() tea.Msg {
				p, err := probe.DiscoverProfileModels(p, 20*time.Second)
				if err != nil {
					return ErrMsg{Err: err}
				}
				if _, err := a.store.Update(p.ID, p); err != nil {
					return ErrMsg{Err: fmt.Errorf("保存模型列表失败: %w", err)}
				}
				res, err := switcher.Activate(p.ID, a.store, a.paths.GrokConfig, a.paths.BackupsDir)
				if err != nil {
					return ErrMsg{Err: err}
				}
				return DoneMsg{Payload: InfoMsg{Text: fmt.Sprintf("已切换到 %s（备份: %s）", res.Profile.Name, res.BackupName)}}
			}
		}))

	case requestOfficialMsg:
		body := "将移除中间站相关配置段，保留其他 TOML 段，\n并恢复为 Grok 官方认证（默认配置）。\n\n备份将自动保存。"
		return a, a.push(NewConfirm("切换到官方配置", body, "确认", true, func() tea.Cmd {
			return func() tea.Msg {
				bak, err := switcher.ActivateOfficial(a.store, a.paths.GrokConfig, a.paths.BackupsDir)
				if err != nil {
					return ErrMsg{Err: err}
				}
				return DoneMsg{Payload: InfoMsg{Text: "已切换到官方配置（备份: " + bak + ")"}}
			}
		}))

	case requestTestMsg:
		p := msg.Profile
		a.setToast("正在测试 " + p.Name + " …")
		return a, func() tea.Msg {
			r := probe.TestProfile(p, 20*time.Second)
			if r.OK {
				return InfoMsg{Text: fmt.Sprintf("测试通过 %s · %s · %dms", p.Name, r.Message, r.Latency.Milliseconds())}
			}
			return ErrMsg{Err: fmt.Errorf("测试失败 %s: %s", p.Name, r.Message)}
		}

	case requestUpdateMsg:
		if a.update == nil {
			a.setToast("正在检查更新…")
			return a, a.checkUpdateNowCmd()
		}
		info := *a.update
		return a, a.push(a.newUpdateConfirm(info))

	case requestBackupMsg:
		return a, a.push(NewBackupScreen(a.paths.BackupsDir, a.paths.GrokConfig, a.doRestore, a.doPrune))

	case backupRestoreRequest:
		name := msg.Name
		body := fmt.Sprintf("恢复文件  %s\n\n%s  恢复操作将：\n   1. 先备份当前 config.toml\n   2. 用选定备份覆盖 config.toml\n   3. 清除活动供应商标记（回到官方）",
			name, theme.SymWarn)
		return a, a.push(NewConfirm("恢复备份", body, "确认恢复", true, func() tea.Cmd {
			return a.doRestore(name)
		}))
	}

	top := a.top()
	if top == nil {
		return a, tea.Quit
	}
	next, cmd := top.Update(msg)
	a.stack[len(a.stack)-1] = next
	return a, cmd
}

type updateAvailableMsg struct {
	Info    updater.Info
	AutoErr error
}

type updateInstalledMsg struct {
	Result updater.InstallResult
}

type updateInstallFailedMsg struct {
	Err error
}

type updateManualCheckMsg struct {
	Info updater.Info
	Err  error
}

func (a *App) checkUpdateCmd() tea.Cmd {
	mode := updater.ModeFromEnv()
	if mode == updater.ModeOff || a.updates == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		info, err := a.updates.Check(ctx, true)
		cancel()
		if err != nil || !info.Available {
			// Startup checks are best-effort and must never disrupt the TUI.
			return nil
		}
		if mode != updater.ModeAuto {
			return updateAvailableMsg{Info: info}
		}
		ctx, cancel = context.WithTimeout(context.Background(), 3*time.Minute)
		result, err := a.updates.Apply(ctx, info)
		cancel()
		if err != nil {
			return updateAvailableMsg{Info: info, AutoErr: err}
		}
		return updateInstalledMsg{Result: result}
	}
}

func (a *App) checkUpdateNowCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		info, err := a.updates.Check(ctx, false)
		return updateManualCheckMsg{Info: info, Err: err}
	}
}

func (a *App) newUpdateConfirm(info updater.Info) Screen {
	body := fmt.Sprintf("当前版本  %s\n最新版本  %s\n\n将下载、校验并原子替换当前程序。\n上一版本会保留用于回退。",
		info.CurrentVersion, info.LatestVersion)
	return NewConfirm("更新 grok-switch", body, "确认更新", true, func() tea.Cmd {
		return a.installUpdateCmd(info)
	})
}

func (a *App) installUpdateCmd(info updater.Info) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		result, err := a.updates.Apply(ctx, info)
		if err != nil {
			return DoneMsg{Payload: updateInstallFailedMsg{Err: err}}
		}
		return DoneMsg{Payload: updateInstalledMsg{Result: result}}
	}
}

func (a *App) doCreate(p profiles.Profile) tea.Cmd {
	return func() tea.Msg {
		p, err := probe.DiscoverProfileModels(p, 20*time.Second)
		if err != nil {
			return ErrMsg{Err: err}
		}
		created, err := a.store.Create(p)
		if err != nil {
			return ErrMsg{Err: err}
		}

		// Auto-activate the newly created profile to update config.toml
		res, err := switcher.Activate(created.ID, a.store, a.paths.GrokConfig, a.paths.BackupsDir)
		if err != nil {
			return ErrMsg{Err: fmt.Errorf("profile created but activation failed: %w", err)}
		}

		return DoneMsg{Payload: InfoMsg{Text: fmt.Sprintf("已添加并切换到供应商 %s (备份: %s)", res.Profile.Name, res.BackupName)}}
	}
}

func (a *App) doUpdate(id string) func(profiles.Profile) tea.Cmd {
	return func(p profiles.Profile) tea.Cmd {
		return func() tea.Msg {
			p, err := probe.DiscoverProfileModels(p, 20*time.Second)
			if err != nil {
				return ErrMsg{Err: err}
			}
			_, err = a.store.Update(id, p)
			if err != nil {
				return ErrMsg{Err: err}
			}

			// Auto-activate the updated profile
			res, err := switcher.Activate(id, a.store, a.paths.GrokConfig, a.paths.BackupsDir)
			if err != nil {
				return ErrMsg{Err: fmt.Errorf("profile updated but activation failed: %w", err)}
			}

			return DoneMsg{Payload: InfoMsg{Text: fmt.Sprintf("已更新并切换到供应商 %s (备份: %s)", res.Profile.Name, res.BackupName)}}
		}
	}
}

func (a *App) doRestore(name string) tea.Cmd {
	return func() tea.Msg {
		bak, err := switcher.Restore(a.store, a.paths.BackupsDir, name, a.paths.GrokConfig)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return DoneMsg{Payload: InfoMsg{Text: fmt.Sprintf("已恢复 %s（当前已备份为 %s）", name, bak)}}
	}
}

func (a *App) doPrune(keep int) tea.Cmd {
	return func() tea.Msg {
		n, err := switcher.PruneBackups(a.paths.BackupsDir, keep)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return InfoMsg{Text: fmt.Sprintf("已清理 %d 个旧备份", n)}
	}
}

func (a *App) View() string {
	top := a.top()
	if top == nil {
		return ""
	}
	return top.View()
}
