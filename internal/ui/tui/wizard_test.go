package tui

import (
	"path/filepath"
	"testing"

	"github.com/Gelmezon/grok-switch/internal/models"
	"github.com/Gelmezon/grok-switch/internal/profiles"
	tea "github.com/charmbracelet/bubbletea"
)

func TestWizardValidateURL(t *testing.T) {
	m := NewWizard("add", profiles.Profile{}, func(p profiles.Profile) tea.Cmd { return nil })
	m.inputs[1].SetValue("not-a-url")
	if err := m.validate(); err == nil {
		t.Fatal("expected invalid URL error")
	}
	m.inputs[0].SetValue("test")
	m.inputs[1].SetValue("https://relay.example.com/v1")
	m.inputs[2].SetValue("sk-test-key")
	if err := m.validate(); err != nil {
		t.Fatalf("valid URL: %v", err)
	}
}

func TestWizardValidateName(t *testing.T) {
	m := NewWizard("add", profiles.Profile{}, func(p profiles.Profile) tea.Cmd { return nil })
	m.inputs[0].SetValue("bad name!")
	if err := m.validate(); err == nil {
		t.Fatal("expected invalid name error")
	}
	m.inputs[0].SetValue("relay_a-1")
	m.inputs[1].SetValue("https://relay.example.com/v1")
	m.inputs[2].SetValue("sk-test-key")
	if err := m.validate(); err != nil {
		t.Fatalf("valid name: %v", err)
	}
}

func TestWizardLiveValidateURL(t *testing.T) {
	m := NewWizard("add", profiles.Profile{}, func(p profiles.Profile) tea.Cmd { return nil })
	m.inputs[1].SetValue("http://")
	m.inputs[0].SetValue("test")
	m.inputs[2].SetValue("sk-test-key")
	m.validate()
	if m.errMsg == "" {
		t.Fatal("expected live URL error for incomplete host")
	}
	m.inputs[1].SetValue("https://ok.example.com")
	m.validate()
	if m.errMsg != "" {
		t.Fatalf("expected clear err, got %q", m.errMsg)
	}
}

func TestWizardAddSingleStep(t *testing.T) {
	m := NewWizard("add", profiles.Profile{}, func(p profiles.Profile) tea.Cmd { return nil })
	// Empty fields should error (as per new single-step validation)
	if err := m.validate(); err == nil {
		t.Fatal("validate should fail for empty fields initially")
	}
}

func TestWizardAddBuildCanBeSaved(t *testing.T) {
	m := NewWizard("add", profiles.Profile{}, func(p profiles.Profile) tea.Cmd { return nil })
	m.inputs[0].SetValue("relay-a")
	m.inputs[1].SetValue("https://relay.example.com/v1")
	m.inputs[2].SetValue("sk-test-key")

	dir := t.TempDir()
	store := profiles.NewStore(
		filepath.Join(dir, "profiles.json"),
		filepath.Join(dir, "grok-switch.lock"),
	)
	created, err := store.Create(m.build())
	if err != nil {
		t.Fatalf("save wizard profile: %v", err)
	}
	if created.DefaultModel != models.DefaultModel {
		t.Fatalf("default model = %q, want %q", created.DefaultModel, models.DefaultModel)
	}
	if !created.CodebaseUploadDisabled() {
		t.Fatal("new wizard profile should disable codebase upload by default")
	}
}

func TestWizardCanAllowCodebaseUpload(t *testing.T) {
	m := NewWizard("add", profiles.Profile{}, func(p profiles.Profile) tea.Cmd { return nil })
	for range 3 {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	if !m.privacyFocused {
		t.Fatal("privacy toggle should be focused after tabbing past inputs")
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if m.build().CodebaseUploadDisabled() {
		t.Fatal("space should turn off codebase upload protection")
	}
}

func TestWizardEditPreservesExplicitUploadChoice(t *testing.T) {
	p := profiles.Profile{}
	p.SetCodebaseUploadDisabled(false)
	m := NewWizard("edit", p, func(p profiles.Profile) tea.Cmd { return nil })
	if m.disableCodebaseUpload || m.build().CodebaseUploadDisabled() {
		t.Fatal("edit wizard should preserve explicit allow choice")
	}
}
