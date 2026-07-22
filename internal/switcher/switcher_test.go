package switcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Gelmezon/grok-switch/internal/config"
	"github.com/Gelmezon/grok-switch/internal/profiles"
)

func env(t *testing.T) (store profiles.ProfileStore, configPath, backupsDir string) {
	t.Helper()
	dir := t.TempDir()
	configPath = filepath.Join(dir, "config.toml")
	switchHome := filepath.Join(dir, ".grok_switch")
	backupsDir = filepath.Join(switchHome, "backups")
	_ = os.MkdirAll(backupsDir, 0o700)
	profilesPath := filepath.Join(switchHome, "profiles.json")
	lockPath := filepath.Join(switchHome, "grok-switch.lock")
	store = profiles.NewStore(profilesPath, lockPath)
	// Seed a minimal valid config.
	_ = os.WriteFile(configPath, []byte("# initial\n[custom]\nx = 1\n"), 0o600)
	return store, configPath, backupsDir
}

func makeProfile(t *testing.T, s profiles.ProfileStore, name, model string) profiles.Profile {
	t.Helper()
	p, err := s.Create(profiles.Profile{
		Name:         name,
		BaseURL:      "https://" + name + ".example.com/v1",
		APIKey:       "sk-" + name + "-key-0001",
		DefaultModel: model,
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestActivateSuccess(t *testing.T) {
	s, cfg, bakDir := env(t)
	a := makeProfile(t, s, "relay-a", "grok-4")
	res, err := Activate(a.ID, s, cfg, bakDir)
	if err != nil {
		t.Fatal(err)
	}
	if res.BackupName == "" {
		t.Fatal("expected backup name")
	}
	ok, err := config.Match(cfg, a)
	if err != nil || !ok {
		t.Fatalf("disk should match: %v %v", ok, err)
	}
	// Active flag.
	got, _ := s.Get(a.ID)
	if !got.IsActive {
		t.Fatal("should be active")
	}
	// Custom section preserved.
	data, _ := config.Load(cfg)
	if _, ok := data["custom"]; !ok {
		t.Fatal("custom section lost")
	}
}

func TestBackupBeforeWrite(t *testing.T) {
	s, cfg, bakDir := env(t)
	a := makeProfile(t, s, "relay-a", "grok-4")
	// Put known content.
	_ = os.WriteFile(cfg, []byte("[keep]\nv = \"before\"\n"), 0o600)
	res, err := Activate(a.ID, s, cfg, bakDir)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(bakDir, res.BackupName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "before") {
		t.Fatalf("backup should contain pre-switch content: %s", raw)
	}
}

func TestActivateBadTOMLNoWrite(t *testing.T) {
	s, cfg, bakDir := env(t)
	a := makeProfile(t, s, "relay-a", "grok-4")
	_ = os.WriteFile(cfg, []byte("{{{not toml"), 0o600)
	before, _ := os.ReadFile(cfg)
	_, err := Activate(a.ID, s, cfg, bakDir)
	if err == nil {
		t.Fatal("expected error")
	}
	after, _ := os.ReadFile(cfg)
	if string(before) != string(after) {
		t.Fatal("config should not be modified on parse error")
	}
	got, _ := s.Get(a.ID)
	if got.IsActive {
		t.Fatal("should not be active")
	}
}

func TestSwitchAtoB(t *testing.T) {
	s, cfg, bakDir := env(t)
	a := makeProfile(t, s, "relay-a", "grok-4")
	b := makeProfile(t, s, "relay-b", "grok-4-fast")
	if _, err := Activate(a.ID, s, cfg, bakDir); err != nil {
		t.Fatal(err)
	}
	if _, err := Activate(b.ID, s, cfg, bakDir); err != nil {
		t.Fatal(err)
	}
	ok, _ := config.Match(cfg, b)
	if !ok {
		t.Fatal("should match B")
	}
	list, _ := s.List()
	for _, p := range list {
		if p.ID == a.ID && p.IsActive {
			t.Fatal("A should not be active")
		}
		if p.ID == b.ID && !p.IsActive {
			t.Fatal("B should be active")
		}
	}
	// At least one backup.
	backs, _ := ListBackups(bakDir)
	if len(backs) < 2 {
		t.Fatalf("want >=2 backups, got %d", len(backs))
	}
}

func TestOfficial(t *testing.T) {
	s, cfg, bakDir := env(t)
	a := makeProfile(t, s, "relay-a", "grok-4")
	if _, err := Activate(a.ID, s, cfg, bakDir); err != nil {
		t.Fatal(err)
	}
	bak, err := ActivateOfficial(s, cfg, bakDir)
	if err != nil {
		t.Fatal(err)
	}
	if bak == "" {
		t.Fatal("expected backup")
	}
	data, _ := config.Load(cfg)
	if _, ok := data["endpoints"]; ok {
		t.Fatal("endpoints should be gone")
	}
	features, ok := data["features"].(map[string]interface{})
	if !ok || features["telemetry"] != false || features["codebase_indexing"] != false {
		t.Fatalf("official mode should preserve privacy features: %#v", data["features"])
	}
	harness, ok := data["harness"].(map[string]interface{})
	if !ok || harness["disable_codebase_upload"] != true {
		t.Fatalf("official mode should preserve codebase upload protection: %#v", data["harness"])
	}
	got, _ := s.Get(a.ID)
	if got.IsActive {
		t.Fatal("should clear active")
	}
}

func TestStatusMatchMismatch(t *testing.T) {
	s, cfg, bakDir := env(t)
	a := makeProfile(t, s, "relay-a", "grok-4")
	st, err := ActiveStatus(s, cfg)
	if err != nil || st.Mode != StatusOfficial || st.Profile != nil {
		t.Fatalf("no active: %+v %v", st, err)
	}
	if _, err := Activate(a.ID, s, cfg, bakDir); err != nil {
		t.Fatal(err)
	}
	st, err = ActiveStatus(s, cfg)
	if err != nil || st.Mode != StatusManagedRelay || st.Profile == nil {
		t.Fatalf("want match: %+v %v", st, err)
	}
	// Corrupt match by editing key.
	data, _ := config.Load(cfg)
	data["endpoints"] = map[string]interface{}{"api_url": "https://other.example.com/v1"}
	_ = config.Apply(cfg, profiles.Profile{
		BaseURL: "https://other.example.com/v1", APIKey: "x", DefaultModel: "m",
	})
	// Manual overwrite of only endpoints after re-apply with wrong profile fields
	// Use write of mismatched content:
	_ = os.WriteFile(cfg, []byte(`[endpoints]
api_url = "https://other.example.com/v1"
[models]
default = "grok-4"
[model.grok-4]
model = "grok-4"
api_key = "sk-relay-a-key-0001"
`), 0o600)
	st, err = ActiveStatus(s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode != StatusUnmanagedOrUnknown || st.Profile == nil {
		t.Fatal("expected mismatch")
	}
}

func TestStatusRelayWithoutActiveProfileIsUnknown(t *testing.T) {
	s, cfg, _ := env(t)
	a := makeProfile(t, s, "relay-a", "grok-4")
	if err := config.Apply(cfg, a); err != nil {
		t.Fatal(err)
	}
	st, err := ActiveStatus(s, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode != StatusUnmanagedOrUnknown || st.Profile != nil {
		t.Fatalf("want unmanaged-or-unknown without profile, got %+v", st)
	}
}

func TestActivateRollsBackWhenProfilesCommitFails(t *testing.T) {
	s, cfg, bakDir := env(t)
	a := makeProfile(t, s, "relay-a", "grok-4")
	before, _ := os.ReadFile(cfg)
	failNextProfilesWrite(t, bakDir)

	_, err := Activate(a.ID, s, cfg, bakDir)
	if err == nil || !strings.Contains(err.Error(), "已从备份恢复配置") {
		t.Fatalf("expected committed-state failure with rollback, got %v", err)
	}
	after, _ := os.ReadFile(cfg)
	if string(after) != string(before) {
		t.Fatalf("config was not rolled back\nwant: %s\ngot: %s", before, after)
	}
	got, getErr := s.Get(a.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if got.IsActive {
		t.Fatal("profile state should remain unchanged")
	}
}

func TestActivateOfficialRollsBackWhenProfilesCommitFails(t *testing.T) {
	s, cfg, bakDir := env(t)
	a := makeProfile(t, s, "relay-a", "grok-4")
	if _, err := Activate(a.ID, s, cfg, bakDir); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(cfg)
	failNextProfilesWrite(t, bakDir)

	_, err := ActivateOfficial(s, cfg, bakDir)
	if err == nil || !strings.Contains(err.Error(), "已从备份恢复配置") {
		t.Fatalf("expected committed-state failure with rollback, got %v", err)
	}
	after, _ := os.ReadFile(cfg)
	if string(after) != string(before) {
		t.Fatal("relay config was not restored after official commit failed")
	}
	got, getErr := s.Get(a.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if !got.IsActive {
		t.Fatal("active profile should remain unchanged")
	}
}

func TestRestoreRollsBackWhenProfilesCommitFails(t *testing.T) {
	s, cfg, bakDir := env(t)
	a := makeProfile(t, s, "relay-a", "grok-4")
	res, err := Activate(a.ID, s, cfg, bakDir)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(cfg)
	failNextProfilesWrite(t, bakDir)

	_, err = Restore(s, bakDir, res.BackupName, cfg)
	if err == nil || !strings.Contains(err.Error(), "已从备份恢复配置") {
		t.Fatalf("expected committed-state failure with rollback, got %v", err)
	}
	after, _ := os.ReadFile(cfg)
	if string(after) != string(before) {
		t.Fatal("pre-restore config was not restored after profile commit failed")
	}
	got, getErr := s.Get(a.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if !got.IsActive {
		t.Fatal("active profile should remain unchanged")
	}
}

func failNextProfilesWrite(t *testing.T, backupsDir string) {
	t.Helper()
	profilesPath := filepath.Join(filepath.Dir(backupsDir), "profiles.json")
	tempPath := profilesPath + ".tmp-" + strconv.Itoa(os.Getpid())
	if err := os.Mkdir(tempPath, 0o700); err != nil {
		t.Fatalf("create profiles write blocker: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove profiles write blocker %s: %v", fmt.Sprintf("%q", tempPath), err)
		}
	})
}

func TestRestorePathTraversal(t *testing.T) {
	s, cfg, bakDir := env(t)
	_, err := Restore(s, bakDir, "../evil.toml", cfg)
	if err == nil {
		t.Fatal("expected rejection")
	}
	_, err = Restore(s, bakDir, "not-toml.txt", cfg)
	if err == nil {
		t.Fatal("expected rejection")
	}
}

func TestRestoreAndPrune(t *testing.T) {
	s, cfg, bakDir := env(t)
	a := makeProfile(t, s, "relay-a", "grok-4")
	res, err := Activate(a.ID, s, cfg, bakDir)
	if err != nil {
		t.Fatal(err)
	}
	// Switch official to change content.
	if _, err := ActivateOfficial(s, cfg, bakDir); err != nil {
		t.Fatal(err)
	}
	// Restore first backup (pre-activate or post?).
	// List and restore the one that has relay content.
	backs, _ := ListBackups(bakDir)
	if len(backs) == 0 {
		t.Fatal("no backups")
	}
	// Restore res.BackupName which was taken before first activate — may be initial.
	// Use a backup that contains the mid-station after activate: create another.
	if _, err := Activate(a.ID, s, cfg, bakDir); err != nil {
		t.Fatal(err)
	}
	backs, _ = ListBackups(bakDir)
	// Pick newest that is not empty marker — just restore res.BackupName for safety chain.
	_, err = Restore(s, bakDir, res.BackupName, cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(a.ID)
	if got.IsActive {
		t.Fatal("restore should clear active")
	}

	// Create many backups then prune.
	for i := 0; i < 5; i++ {
		if _, err := CreateBackup(cfg, bakDir); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	del, err := PruneBackups(bakDir, 3)
	if err != nil {
		t.Fatal(err)
	}
	if del < 1 {
		t.Fatalf("expected some deletions, del=%d", del)
	}
	backs, _ = ListBackups(bakDir)
	if len(backs) > 3 {
		t.Fatalf("want <=3, got %d", len(backs))
	}
}

func TestSameSecondBackupsUnique(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "c.toml")
	_ = os.WriteFile(cfg, []byte("x=1\n"), 0o600)
	names := map[string]bool{}
	for i := 0; i < 10; i++ {
		n, err := CreateBackup(cfg, dir)
		if err != nil {
			t.Fatal(err)
		}
		if names[n] {
			t.Fatalf("duplicate backup name %s", n)
		}
		names[n] = true
	}
}

func TestValidateBackupName(t *testing.T) {
	if err := validateBackupName("ok.toml"); err != nil {
		t.Fatal(err)
	}
	if err := validateBackupName("../x.toml"); err == nil {
		t.Fatal("expected error")
	}
	if err := validateBackupName("x.txt"); err == nil {
		t.Fatal("expected error")
	}
}
