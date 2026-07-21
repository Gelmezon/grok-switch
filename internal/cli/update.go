package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Gelmezon/grok-switch/internal/exitcodes"
	"github.com/Gelmezon/grok-switch/internal/paths"
	"github.com/Gelmezon/grok-switch/internal/ui"
	"github.com/Gelmezon/grok-switch/internal/ui/theme"
	"github.com/Gelmezon/grok-switch/internal/updater"
)

func runUpdate(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "check":
			return runUpdateCheck(args[1:])
		case "rollback":
			return runUpdateRollback(args[1:])
		case "help", "--help", "-h":
			printUpdateHelp(os.Stdout)
			return nil
		}
	}

	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	yes := fs.Bool("yes", false, "")
	if err := fs.Parse(args); err != nil {
		return &exitcodes.UsageError{Msg: err.Error()}
	}
	if fs.NArg() != 0 {
		return &exitcodes.UsageError{Msg: "用法: grok-switch update [--yes]"}
	}

	svc, err := updateService()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var info updater.Info
	err = ui.WithSpinner("正在检查更新...", func() error {
		var checkErr error
		info, checkErr = svc.Check(ctx, false)
		return checkErr
	})
	if err != nil {
		return err
	}
	if !info.Available {
		fmt.Println(theme.OK(fmt.Sprintf("当前已是最新版本 %s", info.CurrentVersion)))
		return nil
	}

	fmt.Printf("发现新版本: %s → %s\n", info.CurrentVersion, info.LatestVersion)
	if info.ReleaseURL != "" {
		fmt.Println(theme.Muted.Render("Release: " + info.ReleaseURL))
	}
	if !*yes {
		if !ui.Interactive() {
			return &exitcodes.UsageError{Msg: "非交互模式请使用 grok-switch update --yes"}
		}
		fmt.Print("确认下载并更新？ [y/N] ")
		if !confirmYN(false) {
			fmt.Println(theme.Muted.Render("已取消"))
			return nil
		}
	}

	var result updater.InstallResult
	err = ui.WithSpinner("正在下载、校验并安装...", func() error {
		var applyErr error
		result, applyErr = svc.Apply(ctx, info)
		return applyErr
	})
	if err != nil {
		return err
	}
	fmt.Println(theme.OK("已更新到 " + result.Version))
	fmt.Println(theme.Muted.Render("  程序: " + result.Path))
	fmt.Println(theme.Muted.Render("  上一版本: " + result.PreviousPath))
	fmt.Println(theme.Warning.Render("  当前进程仍是旧版本，请重新启动 grok-switch"))
	return nil
}

func runUpdateCheck(args []string) error {
	fs := flag.NewFlagSet("update check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOut := fs.Bool("json", false, "")
	if err := fs.Parse(args); err != nil {
		return &exitcodes.UsageError{Msg: err.Error()}
	}
	if fs.NArg() != 0 {
		return &exitcodes.UsageError{Msg: "用法: grok-switch update check [--json]"}
	}
	svc, err := updateService()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := svc.Check(ctx, false)
	if err != nil {
		return err
	}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}
	if info.Available {
		fmt.Printf("有可用更新: %s → %s\n", info.CurrentVersion, info.LatestVersion)
		if info.ReleaseURL != "" {
			fmt.Println(theme.Muted.Render("Release: " + info.ReleaseURL))
		}
		return nil
	}
	fmt.Println(theme.OK(fmt.Sprintf("当前已是最新版本 %s", info.CurrentVersion)))
	return nil
}

func runUpdateRollback(args []string) error {
	fs := flag.NewFlagSet("update rollback", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	yes := fs.Bool("yes", false, "")
	if err := fs.Parse(args); err != nil {
		return &exitcodes.UsageError{Msg: err.Error()}
	}
	if fs.NArg() != 0 {
		return &exitcodes.UsageError{Msg: "用法: grok-switch update rollback [--yes]"}
	}
	if !*yes {
		if !ui.Interactive() {
			return &exitcodes.UsageError{Msg: "非交互模式请使用 grok-switch update rollback --yes"}
		}
		fmt.Print("确认回退到上一版本？ [y/N] ")
		if !confirmYN(false) {
			fmt.Println(theme.Muted.Render("已取消"))
			return nil
		}
	}

	svc, err := updateService()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := svc.Rollback(ctx)
	if err != nil {
		return err
	}
	fmt.Println(theme.OK("已回退到 " + result.Version))
	fmt.Println(theme.Muted.Render("  程序: " + result.Path))
	fmt.Println(theme.Warning.Render("  请重新启动 grok-switch"))
	return nil
}

func updateService() (*updater.Service, error) {
	p, err := paths.Resolve()
	if err != nil {
		return nil, err
	}
	return updater.NewService(Version, p.UpdateStateFile), nil
}

func printUpdateHelp(w io.Writer) {
	fmt.Fprint(w, `用法:
  grok-switch update [--yes]           检查并安装最新稳定版
  grok-switch update check [--json]    仅检查更新
  grok-switch update rollback [--yes]  回退到上一版本

环境变量:
  GROK_SWITCH_UPDATE_MODE=notify|auto|off
  GROK_SWITCH_NO_UPDATE_CHECK=1
`)
}
