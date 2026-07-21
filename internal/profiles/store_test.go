package profiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) (ProfileStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	lockPath := filepath.Join(dir, "grok-switch.lock")
	return NewStore(path, lockPath), path
}

func sample(name string) Profile {
	return Profile{
		Name:         name,
		BaseURL:      "https://relay.example.com/v1",
		APIKey:       "sk-test-key-12345678",
		DefaultModel: "grok-4",
	}
}

func TestNormalize(t *testing.T) {
	p := sample("a")
	p.BaseURL = "https://relay.example.com/v1///"
	Normalize(&p)
	if p.UpstreamFormat != "openai_chat" {
		t.Errorf("UpstreamFormat = %q", p.UpstreamFormat)
	}
	if p.DefaultReasoningEffort != "high" {
		t.Errorf("DefaultReasoningEffort = %q", p.DefaultReasoningEffort)
	}
	if p.WebSearchModel != "grok-4" {
		t.Errorf("WebSearchModel = %q", p.WebSearchModel)
	}
	if p.SubagentsModels.Explore != "grok-4" || p.SubagentsModels.Plan != "grok-4" {
		t.Errorf("SubagentsModels = %+v", p.SubagentsModels)
	}
	if p.BaseURL != "https://relay.example.com/v1" {
		t.Errorf("BaseURL = %q (should strip trailing /)", p.BaseURL)
	}
}

func TestValidate(t *testing.T) {
	if err := Validate(sample("ok")); err != nil {
		t.Fatal(err)
	}
	bad := sample("")
	if err := Validate(bad); err == nil {
		t.Fatal("expected empty name error")
	}
	bad = sample("x")
	bad.BaseURL = "ftp://bad"
	if err := Validate(bad); err == nil {
		t.Fatal("expected bad URL error")
	}
	bad = sample("x")
	bad.APIKey = ""
	if err := Validate(bad); err == nil {
		t.Fatal("expected empty key error")
	}
	bad = sample("x")
	bad.DefaultModel = ""
	if err := Validate(bad); err == nil {
		t.Fatal("expected empty model error")
	}
}

func TestCRUD(t *testing.T) {
	s, path := testStore(t)
	a, err := s.Create(sample("relay-a"))
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == "" || len(a.ID) != 16 {
		t.Fatalf("bad id: %q", a.ID)
	}
	b, err := s.Create(sample("relay-b"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %v len=%d", err, len(list))
	}
	got, err := s.Get(a.ID)
	if err != nil || got.Name != "relay-a" {
		t.Fatalf("get: %+v %v", got, err)
	}
	matches, err := s.GetByName("relay-a")
	if err != nil || len(matches) != 1 {
		t.Fatalf("by name: %v", matches)
	}
	// Same name create
	_, err = s.Create(sample("relay-a"))
	if err != nil {
		t.Fatal(err)
	}
	matches, _ = s.GetByName("relay-a")
	if len(matches) != 2 {
		t.Fatalf("want 2 same-name, got %d", len(matches))
	}

	a.DefaultModel = "grok-5"
	updated, err := s.Update(a.ID, a)
	if err != nil || updated.DefaultModel != "grok-5" {
		t.Fatalf("update: %+v %v", updated, err)
	}

	if err := s.SetActive(a.ID); err != nil {
		t.Fatal(err)
	}
	list, _ = s.List()
	activeCount := 0
	for _, p := range list {
		if p.IsActive {
			activeCount++
			if p.ID != a.ID {
				t.Fatal("wrong active")
			}
		}
	}
	if activeCount != 1 {
		t.Fatalf("activeCount=%d", activeCount)
	}
	if err := s.SetActive(b.ID); err != nil {
		t.Fatal(err)
	}
	list, _ = s.List()
	for _, p := range list {
		if p.IsActive && p.ID != b.ID {
			t.Fatal("SetActive should clear others")
		}
	}
	if err := s.ClearActive(); err != nil {
		t.Fatal(err)
	}
	list, _ = s.List()
	for _, p := range list {
		if p.IsActive {
			t.Fatal("ClearActive failed")
		}
	}
	if err := s.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(a.ID); err == nil {
		t.Fatal("expected not found after delete")
	}

	// Permission 0600 (best-effort on Windows).
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = info.Mode().Perm()
}

func TestCorruptJSONRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	lockPath := filepath.Join(dir, "grok-switch.lock")
	if err := os.WriteFile(path, []byte("{not valid json!!!"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewStore(path, lockPath)
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("want empty after corrupt, got %d", len(list))
	}
	// Corrupt backup should exist.
	entries, _ := os.ReadDir(dir)
	found := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "corrupt") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected .corrupt-*.bak file")
	}
	// Further create should work.
	if _, err := s.Create(sample("ok")); err != nil {
		t.Fatal(err)
	}
}

func TestIDsUnique(t *testing.T) {
	s, _ := testStore(t)
	ids := map[string]bool{}
	for i := 0; i < 20; i++ {
		p, err := s.Create(sample("n"))
		if err != nil {
			t.Fatal(err)
		}
		if ids[p.ID] {
			t.Fatalf("duplicate id %s", p.ID)
		}
		ids[p.ID] = true
	}
}

func TestWriteIsValidJSON(t *testing.T) {
	s, path := testStore(t)
	if _, err := s.Create(sample("a")); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data storeData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Profiles) != 1 {
		t.Fatal(data)
	}
}

func TestPublicOmitsAPIKey(t *testing.T) {
	p := sample("a")
	p.ID = "abc"
	p.CreatedAt = time.Now()
	pub := p.Public()
	if _, ok := pub["api_key"]; ok {
		t.Fatal("public should not include api_key")
	}
}
