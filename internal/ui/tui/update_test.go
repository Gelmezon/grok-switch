package tui

import (
	"testing"

	"github.com/Gelmezon/grok-switch/internal/switcher"
	"github.com/Gelmezon/grok-switch/internal/updater"
	tea "github.com/charmbracelet/bubbletea"
)

func TestHomeUpdateKeyRequestsUpdate(t *testing.T) {
	home := NewHome("v1.0.0", "/tmp/config.toml", nil, switcher.Status{})
	_, cmd := home.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
	if cmd == nil {
		t.Fatal("U key returned no command")
	}
	msg := cmd()
	if _, ok := msg.(requestUpdateMsg); !ok {
		t.Fatalf("U key message = %T, want requestUpdateMsg", msg)
	}
}

func TestManualUpdateCheckOpensConfirmation(t *testing.T) {
	home := NewHome("v1.0.0", "/tmp/config.toml", nil, switcher.Status{})
	app := &App{stack: []Screen{home}}
	info := updater.Info{CurrentVersion: "v1.0.0", LatestVersion: "v1.1.0", Available: true}

	model, _ := app.Update(updateManualCheckMsg{Info: info})
	got := model.(*App)
	if len(got.stack) != 2 {
		t.Fatalf("stack length = %d, want 2", len(got.stack))
	}
	if _, ok := got.top().(*ConfirmModel); !ok {
		t.Fatalf("top screen = %T, want *ConfirmModel", got.top())
	}
	if got.update == nil || got.update.LatestVersion != "v1.1.0" {
		t.Fatalf("available update = %+v", got.update)
	}
}

func TestManualUpdateCheckShowsUpToDate(t *testing.T) {
	home := NewHome("v1.0.0", "/tmp/config.toml", nil, switcher.Status{})
	app := &App{stack: []Screen{home}}

	model, _ := app.Update(updateManualCheckMsg{Info: updater.Info{CurrentVersion: "v1.0.0"}})
	got := model.(*App)
	if home.toast != "当前已是最新版本 v1.0.0" {
		t.Fatalf("toast = %q", home.toast)
	}
	if len(got.stack) != 1 {
		t.Fatalf("stack length = %d, want 1", len(got.stack))
	}
}
