package tui

import (
	"testing"
)

func TestWizardValidateURL(t *testing.T) {
	m := NewAddWizard("", nil)
	m.step = 1
	m.inputs[1].SetValue("not-a-url")
	if err := m.validateStep(); err == nil {
		t.Fatal("expected invalid URL error")
	}
	m.inputs[1].SetValue("https://relay.example.com/v1")
	if err := m.validateStep(); err != nil {
		t.Fatalf("valid URL: %v", err)
	}
}

func TestWizardValidateName(t *testing.T) {
	m := NewAddWizard("", nil)
	m.step = 0
	m.inputs[0].SetValue("bad name!")
	if err := m.validateStep(); err == nil {
		t.Fatal("expected invalid name error")
	}
	m.inputs[0].SetValue("relay_a-1")
	if err := m.validateStep(); err != nil {
		t.Fatalf("valid name: %v", err)
	}
}

func TestWizardLiveValidateURL(t *testing.T) {
	m := NewAddWizard("", nil)
	m.step = 1
	m.inputs[1].SetValue("http://")
	m.liveValidate()
	if m.errMsg == "" {
		t.Fatal("expected live URL error for incomplete host")
	}
	m.inputs[1].SetValue("https://ok.example.com")
	m.liveValidate()
	if m.errMsg != "" {
		t.Fatalf("expected clear err, got %q", m.errMsg)
	}
}

func TestWizardAddHasNoAdvancedStep(t *testing.T) {
	m := NewAddWizard("x", nil)
	if m.confirmStep() != 4 {
		t.Fatalf("add confirm step want 4 got %d", m.confirmStep())
	}
	if m.totalSteps() != 4 {
		t.Fatalf("add total steps want 4 got %d", m.totalSteps())
	}
}

func TestWizardModelPick(t *testing.T) {
	m := NewAddWizard("", nil)
	m.step = 3
	m.modelIdx = 0
	if m.selectedModel() == "" {
		t.Fatal("expected catalog model")
	}
}

