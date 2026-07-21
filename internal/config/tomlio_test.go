package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gelmezon/grok-switch/internal/profiles"
	toml "github.com/pelletier/go-toml/v2"
)

func sampleProfile() profiles.Profile {
	p := profiles.Profile{
		Name:                   "relay-a",
		BaseURL:                "https://relay.example.com/v1",
		APIKey:                 "sk-xxx",
		DefaultModel:           "grok-4",
		DefaultReasoningEffort: "high",
		WebSearchModel:         "grok-4",
		SubagentsModels: profiles.SubagentsModels{
			Explore: "grok-4",
			Plan:    "grok-4",
		},
	}
	profiles.Normalize(&p)
	return p
}

func TestApplyEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	p := sampleProfile()
	if err := Apply(path, p); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "api_url") {
		t.Fatalf("missing api_url: %s", raw)
	}
	if !strings.Contains(string(raw), "sk-xxx") {
		t.Fatalf("missing key: %s", raw)
	}
	// Trailing newline.
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		t.Fatal("missing trailing newline")
	}
	// Re-parse.
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
}

func TestPreserveUnknownSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	initial := `
[custom]
foo = "bar"

[other.nested]
x = 1
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Apply(path, sampleProfile()); err != nil {
		t.Fatal(err)
	}
	data, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	custom, ok := data["custom"].(map[string]interface{})
	if !ok || asString(custom["foo"]) != "bar" {
		t.Fatalf("custom section lost: %+v", data["custom"])
	}
}

func TestReplaceOldModelSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	old := sampleProfile()
	old.DefaultModel = "old-model"
	if err := Apply(path, old); err != nil {
		t.Fatal(err)
	}
	p := sampleProfile()
	p.DefaultModel = "grok-4"
	if err := Apply(path, p); err != nil {
		t.Fatal(err)
	}
	data, _ := Load(path)
	models, ok := data["model"].(map[string]interface{})
	if !ok {
		t.Fatal("no model section")
	}
	if _, exists := models["old-model"]; exists {
		t.Fatal("old model section should be removed")
	}
	if _, exists := models["grok-4"]; !exists {
		t.Fatal("new model missing")
	}
}

func TestMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	p := sampleProfile()
	if err := Apply(path, p); err != nil {
		t.Fatal(err)
	}
	ok, err := Match(path, p)
	if err != nil || !ok {
		t.Fatalf("expected match: %v %v", ok, err)
	}
	p2 := p
	p2.APIKey = "other"
	ok, err = Match(path, p2)
	if err != nil || ok {
		t.Fatal("expected mismatch on key")
	}
}

func TestApplyOfficialPreservesOther(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := Apply(path, sampleProfile()); err != nil {
		t.Fatal(err)
	}
	// Inject custom.
	data, _ := Load(path)
	data["custom"] = map[string]interface{}{"keep": true}
	if err := atomicWriteTOML(path, data); err != nil {
		t.Fatal(err)
	}
	if err := ApplyOfficial(path); err != nil {
		t.Fatal(err)
	}
	data, _ = Load(path)
	if _, ok := data["endpoints"]; ok {
		t.Fatal("endpoints should be removed")
	}
	if _, ok := data["models"]; ok {
		t.Fatal("models should be removed")
	}
	if _, ok := data["model"]; ok {
		t.Fatal("model should be removed")
	}
	if custom, ok := data["custom"].(map[string]interface{}); !ok {
		t.Fatal("custom should remain")
	} else if custom["keep"] != true {
		t.Fatal("custom.keep lost")
	}
	// File must not be empty.
	raw, _ := os.ReadFile(path)
	if len(strings.TrimSpace(string(raw))) == 0 {
		t.Fatal("official mode must not wipe file")
	}
}

func TestAPIKeyWithSpecialChars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	p := sampleProfile()
	p.APIKey = `sk-"quote'\backslash`
	if err := Apply(path, p); err != nil {
		t.Fatal(err)
	}
	ok, err := Match(path, p)
	if err != nil || !ok {
		t.Fatalf("special key match failed: %v %v", ok, err)
	}
	// Ensure re-parse works.
	raw, _ := os.ReadFile(path)
	var m map[string]interface{}
	if err := toml.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidTOMLNotOverwrittenOnLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	bad := "this is = not [ valid toml"
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected parse error")
	}
	// Original file unchanged.
	raw, _ := os.ReadFile(path)
	if string(raw) != bad {
		t.Fatal("file was modified on parse error")
	}
}

func TestImportFromConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	p := sampleProfile()
	if err := Apply(path, p); err != nil {
		t.Fatal(err)
	}
	imp, err := ImportFromConfig(path, "imported")
	if err != nil {
		t.Fatal(err)
	}
	if imp.BaseURL != p.BaseURL || imp.DefaultModel != p.DefaultModel || imp.APIKey != p.APIKey {
		t.Fatalf("import mismatch: %+v", imp)
	}
}
