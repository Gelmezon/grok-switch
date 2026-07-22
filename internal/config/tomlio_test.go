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

func TestMatchChecksEveryManagedField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	p := sampleProfile()
	p.WebSearchModel = "grok-search"
	p.SubagentsModels.Explore = "grok-explore"
	p.SubagentsModels.Plan = "grok-plan"
	p.Models = []profiles.ModelDef{{
		ID:                      "grok-extra",
		Model:                   "grok-extra-upstream",
		APIKey:                  "sk-extra",
		SupportsReasoningEffort: true,
		ReasoningEffort:         "medium",
		ReasoningEfforts:        []string{"low", "medium"},
	}}
	if err := Apply(path, p); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(map[string]interface{})
	}{
		{"web search model", func(d map[string]interface{}) { d["models"].(map[string]interface{})["web_search"] = "wrong" }},
		{"explore model", func(d map[string]interface{}) {
			d["subagents"].(map[string]interface{})["models"].(map[string]interface{})["explore"] = "wrong"
		}},
		{"plan model", func(d map[string]interface{}) {
			d["subagents"].(map[string]interface{})["models"].(map[string]interface{})["plan"] = "wrong"
		}},
		{"reasoning effort", func(d map[string]interface{}) {
			d["model"].(map[string]interface{})[p.DefaultModel].(map[string]interface{})["reasoning_effort"] = "low"
		}},
		{"extra model name", func(d map[string]interface{}) {
			d["model"].(map[string]interface{})["grok-extra"].(map[string]interface{})["model"] = "wrong"
		}},
		{"extra model api key", func(d map[string]interface{}) {
			d["model"].(map[string]interface{})["grok-extra"].(map[string]interface{})["api_key"] = "wrong"
		}},
		{"reasoning support", func(d map[string]interface{}) {
			d["model"].(map[string]interface{})["grok-extra"].(map[string]interface{})["supports_reasoning_effort"] = false
		}},
		{"reasoning choices", func(d map[string]interface{}) {
			d["model"].(map[string]interface{})["grok-extra"].(map[string]interface{})["reasoning_efforts"] = []interface{}{"high"}
		}},
		{"unexpected managed model", func(d map[string]interface{}) {
			d["model"].(map[string]interface{})["stale"] = map[string]interface{}{"model": "stale", "api_key": "sk-stale"}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			tt.mutate(data)
			mutated := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "-")+".toml")
			if err := atomicWriteTOML(mutated, data); err != nil {
				t.Fatal(err)
			}
			ok, err := Match(mutated, p)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				t.Fatal("expected mismatch")
			}
		})
	}
}

func TestApplyPreservesSourceAndUnknownManagedFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	initial := `# file comment
[endpoints]
# endpoint comment
api_url = "https://old.example.com/v1" # inline endpoint comment
future_endpoint = "keep"

[models]
default = "old"
web_search = "old-search"
future_models = 42

[subagents.models]
explore = "old-explore"
plan = "old-plan"
future_subagent = true

[model.old]
model = "old"
api_key = "old-key"
supports_reasoning_effort = false
reasoning_effort = "low"
reasoning_efforts = ["low"]
future_model_field = "keep"

[custom]
z = 1
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Apply(path, sampleProfile()); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	text := string(raw)
	for _, want := range []string{
		"# file comment",
		"# endpoint comment",
		"# inline endpoint comment",
		`future_endpoint = "keep"`,
		"future_models = 42",
		"future_subagent = true",
		`future_model_field = "keep"`,
		"[custom]\nz = 1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("preserved source fragment missing %q:\n%s", want, text)
		}
	}
	data, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	old := data["model"].(map[string]interface{})["old"].(map[string]interface{})
	for key := range managedModelKeys {
		if _, exists := old[key]; exists {
			t.Fatalf("stale managed key %s was retained", key)
		}
	}
	ok, err := Match(path, sampleProfile())
	if err != nil || !ok {
		t.Fatalf("rewritten config should fully match: ok=%v err=%v", ok, err)
	}
}

func TestApplyOfficialPreservesUnknownManagedSectionFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	initial := `# keep comment
[endpoints]
api_url = "https://relay.example.com/v1"
future_endpoint = "keep"

[models]
default = "grok-4"
web_search = "grok-4"
future_models = 42

[subagents.models]
explore = "grok-4"
plan = "grok-4"
future_subagent = true

[model.grok-4]
model = "grok-4"
api_key = "sk-key"
supports_reasoning_effort = true
reasoning_effort = "high"
reasoning_efforts = ["low", "medium", "high"]
future_model_field = "keep"
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyOfficial(path); err != nil {
		t.Fatal(err)
	}
	official, err := IsOfficial(path)
	if err != nil || !official {
		t.Fatalf("want official config: %v %v", official, err)
	}
	raw, _ := os.ReadFile(path)
	for _, want := range []string{"# keep comment", `future_endpoint = "keep"`, "future_models = 42", "future_subagent = true", `future_model_field = "keep"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("unknown field or comment missing %q:\n%s", want, raw)
		}
	}
}

func TestManagedRemovalPreservesMultilineCommentsAndCRLF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	initial := "[model.old] # model header comment\r\n" +
		"model = \"old\"\r\n" +
		"api_key = \"key#inside-string\" # key comment\r\n" +
		"reasoning_efforts = [\r\n" +
		"  \"low\", # low comment\r\n" +
		"  \"high\",\r\n" +
		"] # array comment\r\n"
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyOfficial(path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	text := string(raw)
	for _, comment := range []string{"# model header comment", "# key comment", "# low comment", "# array comment"} {
		if !strings.Contains(text, comment) {
			t.Fatalf("comment %q was lost:\n%s", comment, text)
		}
	}
	if strings.Contains(text, "#inside-string") {
		t.Fatalf("string contents were incorrectly retained as a comment:\n%s", text)
	}
	withoutCRLF := strings.ReplaceAll(text, "\r\n", "")
	if strings.Contains(withoutCRLF, "\n") || strings.Contains(withoutCRLF, "\r") {
		t.Fatalf("line ending style changed:\n%q", text)
	}
}

func TestApplyOfficialRejectsInlineManagedTablesWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	initial := `endpoints = { api_url = "https://relay.example.com/v1", future = "keep" }
models = { default = "grok-4", web_search = "grok-4" }
model = { grok-4 = { model = "grok-4", api_key = "sk-key" } }
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyOfficial(path); err == nil {
		t.Fatal("expected unsupported inline-table error")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != initial {
		t.Fatal("unsupported inline-table config should not be modified")
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
