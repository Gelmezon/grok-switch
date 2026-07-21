package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Gelmezon/grok-switch/internal/paths"
	"github.com/Gelmezon/grok-switch/internal/profiles"
	"github.com/Gelmezon/grok-switch/internal/switcher"
	"github.com/Gelmezon/grok-switch/internal/ui"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
)

// App is the root Bubbletea model with a screen stack.
type App struct {
	version string
	paths   paths.Paths
	store   profiles.ProfileStore
	stack   []Screen
	width   int
	height  int
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
	if len(a.stack) > 0 {
		return a.stack[len(a.stack)-1].Init()
	}
	return nil
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
		if info, ok := msg.Payload.(InfoMsg); ok {
			a.setToast(info.Text)
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

	case pushHelpMsg:
		return a, a.push(NewHelp())

	case requestStatusMsg:
		st, err := switcher.ActiveStatus(a.store, a.paths.GrokConfig)
		if err != nil {
			st = switcher.Status{ConfigPath: a.paths.GrokConfig}
		}
		return a, a.push(NewStatusScreen(st))

	case requestAddMsg:
		return a, a.push(NewAddWizard("", a.doCreate))

	case requestEditMsg:
		return a, a.push(NewEditWizard(msg.Profile, a.doUpdate(msg.Profile.ID)))

	case requestDeleteMsg:
		p := msg.Profile
		if p.IsActive {
			a.setToast(fmt.Sprintf("%s 处于活动状态，无法删除", p.Name))
			return a, nil
		}
		body := fmt.Sprintf("确定要删除 %s 吗？此操作无法撤销。\n（config.toml 不会被修改）", p.Name)
		return a, a.push(NewTypeConfirm("删除 Profile", body, p.Name, func() tea.Cmd {
			return func() tea.Msg {
				if err := a.store.Delete(p.ID); err != nil {
					return ErrMsg{Err: err}
				}
				return DoneMsg{Payload: InfoMsg{Text: "已删除 " + p.Name}}
			}
		}))

	case requestUseMsg:
		p, err := a.store.Get(msg.ID)
		if err != nil {
			return a, func() tea.Msg { return ErrMsg{Err: err} }
		}
		cur := "（无）"
		st, _ := switcher.ActiveStatus(a.store, a.paths.GrokConfig)
		if st.HasActive && st.Profile != nil {
			cur = theme.SymActive + " " + st.Profile.Name + "  " + st.Profile.DefaultModel
		}
		body := fmt.Sprintf("从  %s\n到  %s %s  %s\n\n备份将自动保存到 %s",
			cur, theme.SymInactive, p.Name, p.DefaultModel, a.paths.BackupsDir)
		return a, a.push(NewConfirm("切换 Profile", body, "确认切换", true, func() tea.Cmd {
			return func() tea.Msg {
				res, err := switcher.Activate(p.ID, a.store, a.paths.GrokConfig, a.paths.BackupsDir)
				if err != nil {
					return ErrMsg{Err: err}
				}
				return DoneMsg{Payload: InfoMsg{Text: fmt.Sprintf("已切换到 %s（备份: %s）", res.Profile.Name, res.BackupName)}}
			}
		}))

	case requestOfficialMsg:
		body := "将移除中间站相关配置段，保留其他 TOML 段，\n并清除活动 Profile 标记。\n\n备份将自动保存。"
		return a, a.push(NewConfirm("切回官方认证", body, "确认", true, func() tea.Cmd {
			return func() tea.Msg {
				bak, err := switcher.ActivateOfficial(a.store, a.paths.GrokConfig, a.paths.BackupsDir)
				if err != nil {
					return ErrMsg{Err: err}
				}
				return DoneMsg{Payload: InfoMsg{Text: "已切回官方（备份: " + bak + ")"}}
			}
		}))

	case requestBackupMsg:
		return a, a.push(NewBackupScreen(a.paths.BackupsDir, a.paths.GrokConfig, a.doRestore, a.doPrune))

	case backupRestoreRequest:
		name := msg.Name
		body := fmt.Sprintf("恢复文件  %s\n\n%s  恢复操作将：\n   1. 先备份当前 config.toml\n   2. 用选定备份覆盖 config.toml\n   3. 清除活动 Profile 标记",
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

func (a *App) doCreate(p profiles.Profile) tea.Cmd {
	return func() tea.Msg {
		created, err := a.store.Create(p)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return DoneMsg{Payload: InfoMsg{Text: "已添加 " + created.Name}}
	}
}

func (a *App) doUpdate(id string) func(profiles.Profile) tea.Cmd {
	return func(p profiles.Profile) tea.Cmd {
		return func() tea.Msg {
			_, err := a.store.Update(id, p)
			if err != nil {
				return ErrMsg{Err: err}
			}
			return DoneMsg{Payload: InfoMsg{Text: "已更新 " + p.Name}}
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
