package ui

import (
	"os"
	"strings"

	"github.com/Gelmezon/grok-switch/internal/ui/theme"
	"golang.org/x/term"
)

// IsTTY reports whether stdout is an interactive terminal.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// IsStdinTTY reports whether stdin is a terminal.
func IsStdinTTY() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// ColorEnabled respects NO_COLOR / TERM=dumb and TTY state.
func ColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return IsTTY()
}

// TUIEnabled is true when interactive full-screen TUI may start.
func TUIEnabled() bool {
	if os.Getenv("GROK_SWITCH_NO_TUI") != "" {
		return false
	}
	if forceNoInteractive() {
		return false
	}
	return IsTTY() && IsStdinTTY()
}

// Interactive is true when prompts/wizards may run.
func Interactive() bool {
	if forceNoInteractive() {
		return false
	}
	return IsTTY() && IsStdinTTY()
}

func forceNoInteractive() bool {
	v := os.Getenv("GROK_SWITCH_NO_INTERACTIVE")
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// InitTheme initializes the color system based on terminal capability.
func InitTheme() {
	theme.Init(ColorEnabled())
}
