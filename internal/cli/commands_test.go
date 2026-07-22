package cli

import (
	"errors"
	"testing"

	"github.com/Gelmezon/grok-switch/internal/exitcodes"
	"github.com/Gelmezon/grok-switch/internal/profiles"
)

func TestApplyCodebaseUploadOption(t *testing.T) {
	var p profiles.Profile
	if err := applyCodebaseUploadOption(&p, ""); err != nil {
		t.Fatal(err)
	}
	if !p.CodebaseUploadDisabled() {
		t.Fatal("empty option should retain secure default")
	}
	if err := applyCodebaseUploadOption(&p, "allow"); err != nil {
		t.Fatal(err)
	}
	if p.CodebaseUploadDisabled() {
		t.Fatal("allow should turn protection off")
	}
	if err := applyCodebaseUploadOption(&p, "DENY"); err != nil {
		t.Fatal(err)
	}
	if !p.CodebaseUploadDisabled() {
		t.Fatal("deny should turn protection on")
	}
}

func TestApplyCodebaseUploadOptionRejectsInvalidValue(t *testing.T) {
	var p profiles.Profile
	err := applyCodebaseUploadOption(&p, "maybe")
	if err == nil || !errors.Is(err, exitcodes.ErrUsage) {
		t.Fatalf("expected usage error, got %v", err)
	}
}
