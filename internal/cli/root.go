package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/Gelmezon/grok-switch/internal/exitcodes"
	"github.com/Gelmezon/grok-switch/internal/ui"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
	"github.com/Gelmezon/grok-switch/internal/ui/tui"
)

// Version is set via -ldflags at build time.

var Version = "0.1.0"

// Run is the grok-switch entrypoint.
func Run(args []string) error {
	ui.InitTheme()

	// Global flags
	args, noInteractive := stripGlobalFlags(args)
	if noInteractive {
		os.Setenv("GROK_SWITCH_NO_TUI", "1")
		os.Setenv("GROK_SWITCH_NO_INTERACTIVE", "1")
	}

	if len(args) == 0 {
		return runDefault()
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "tui":
		return runTUI()
	case "version", "--version", "-V":
		fmt.Println(Version)
		return nil
	case "help", "--help", "-h":
		if len(rest) > 0 {
			return helpCommand(rest[0])
		}
		printHelp(os.Stdout)
		return nil
	case "completion":
		return runCompletion(rest)
	case "add":
		return runAdd(rest)
	case "edit":
		return runEdit(rest)
	case "delete", "rm", "remove":
		return runDelete(rest)
	case "show":
		return runShow(rest)
	case "list", "ls":
		return runList(rest)
	case "use":
		return runUse(rest)
	case "status":
		return runStatus(rest)
	case "official":
		return runOfficial(rest)
	case "test":
		return runTest(rest)
	case "import-current":
		return runImportCurrent(rest)
	case "backup":
		return runBackup(rest)
	default:
		fmt.Fprintln(os.Stderr, theme.Fail(fmt.Sprintf("未知子命令 %q", cmd)))
		printHelp(os.Stderr)
		return &exitcodes.UsageError{Msg: fmt.Sprintf("未知子命令: %s", cmd)}
	}
}

func stripGlobalFlags(args []string) ([]string, bool) {
	noInter := false
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--no-interactive", "--no-tui":
			noInter = true
		case "--version":
			out = append(out, "version")
		case "--help", "-h":
			out = append(out, "help")
		default:
			out = append(out, args[i])
		}
	}
	return out, noInter
}

func runDefault() error {
	if !ui.TUIEnabled() {
		printHelp(os.Stdout)
		return &exitcodes.UsageError{Msg: "非 TTY 环境：请指定子命令（如 list、status）"}
	}
	return runTUI()
}

func runTUI() error {
	if !ui.TUIEnabled() {
		return &exitcodes.UsageError{Msg: "当前环境不支持交互式 TUI（非 TTY 或已禁用）"}
	}
	ctx, err := setup()
	if err != nil {
		return err
	}
	// Print banner once before alt screen (optional — alt screen clears it)
	// Start TUI directly.
	return tui.Run(tui.Config{
		Version: Version,
		Paths:   ctx.Paths,
		Store:   ctx.Store,
	})
}

func printHelp(w io.Writer) {
	bin := "grok-switch"
	fmt.Fprintf(w, `%s — Grok Build 中间站切换工具（终端 UI）

用法:
  %s                     启动交互式 TUI
  %s tui                 显式启动 TUI
  %s <命令> [参数]

供应商管理:
  add [name]              添加中间站供应商（交互向导）
  edit <name-or-id>       编辑供应商（含高级模型）
  delete <name-or-id>     删除供应商
  show <name-or-id>       显示供应商详情
  list                    列出供应商（含默认官方）
  test <name-or-id>       测试模型连通性

切换:
  use <name-or-id>        切换到指定供应商
  status                  查看当前配置状态
  official                切回 Grok 官方配置（默认）
  import-current <name>   从当前 config.toml 导入供应商

备份:
  backup list             查看备份
  backup restore <文件名> 恢复备份
  backup prune [--keep N] 清理旧备份（默认保留 10 个）

其他:
  version                 打印版本号
  help [命令]             显示帮助
  completion <shell>      生成 shell 补全 (bash|zsh|fish)

全局标志:
  --no-interactive        强制纯 CLI 模式（禁用 TUI/向导）

环境变量:
  GROK_HOME / GROK_CONFIG / GROK_SWITCH_HOME
  GROK_SWITCH_API_KEY     非交互 API Key
  NO_COLOR=1              禁用颜色
  GROK_SWITCH_NO_TUI=1      禁用全屏 TUI

`, bin, bin, bin, bin)
}

func helpCommand(name string) error {
	// Reuse printHelp for simplicity; sub-helps can expand later.
	printHelp(os.Stdout)
	_ = name
	return nil
}

func runCompletion(args []string) error {
	if len(args) < 1 {
		return &exitcodes.UsageError{Msg: "用法: grok-switch completion <bash|zsh|fish>"}
	}
	switch args[0] {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	default:
		return &exitcodes.UsageError{Msg: "支持的 shell: bash, zsh, fish"}
	}
	return nil
}

const bashCompletion = `# bash completion for grok-switch
_grok_switch() {
  local cur cmds
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  cmds="tui add edit delete show list use status official test import-current backup version help completion"
  if [[ ${COMP_CWORD} -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "${cmds}" -- ${cur}) )
  elif [[ ${COMP_WORDS[1]} == "backup" && ${COMP_CWORD} -eq 2 ]]; then
    COMPREPLY=( $(compgen -W "list restore prune" -- ${cur}) )
  fi
}
complete -F _grok_switch grok-switch
`

const zshCompletion = `#compdef grok-switch
_arguments '1: :->cmds' '*: :->args'
case $state in
  cmds)
    _values 'command' tui add edit delete show list use status official test import-current backup version help completion
    ;;
  args)
    if [[ ${words[2]} == backup ]]; then
      _values 'backup' list restore prune
    fi
    ;;
esac
`

const fishCompletion = `complete -c grok-switch -f
complete -c grok-switch -n __fish_use_subcommand -a "tui add edit delete show list use status official test import-current backup version help completion"
complete -c grok-switch -n "__fish_seen_subcommand_from backup" -a "list restore prune"
`
