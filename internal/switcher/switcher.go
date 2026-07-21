package switcher

import (
	"fmt"
	"os"

	"github.com/Gelmezon/grok-switch/internal/config"
	"github.com/Gelmezon/grok-switch/internal/exitcodes"
	"github.com/Gelmezon/grok-switch/internal/profiles"
)

// Status describes the current active profile vs disk config.
type Status struct {
	HasActive   bool
	Profile     *profiles.Profile
	DiskMatches bool
	ConfigPath  string
}

// ActivateResult is returned on successful switch.
type ActivateResult struct {
	Profile     profiles.Profile
	BackupName  string
	ConfigPath  string
}

// Activate switches config.toml to the given profile under a single lock.
//
// Order: read+validate profile → parse current TOML → backup → write TOML →
// verify → set active. Failures leave active state unchanged when possible.
func Activate(profileID string, store profiles.ProfileStore, configPath, backupsDir string) (ActivateResult, error) {
	var result ActivateResult
	result.ConfigPath = configPath

	err := profiles.WithLock(store, func(tx *profiles.Transaction) error {
		p, err := tx.Get(profileID)
		if err != nil {
			return err
		}
		if err := profiles.Validate(p); err != nil {
			return fmt.Errorf("Profile 校验失败: %w", err)
		}

		// Parse current config — fail before any write.
		if _, err := config.Load(configPath); err != nil {
			return err
		}

		// Backup first.
		bak, err := CreateBackup(configPath, backupsDir)
		if err != nil {
			return err
		}
		result.BackupName = bak

		// Apply profile to config.
		if err := config.Apply(configPath, p); err != nil {
			return fmt.Errorf("写入配置失败: %w", err)
		}

		// Read-back already done inside Apply; double-check Match-critical fields.
		if _, err := config.Load(configPath); err != nil {
			// Try restore backup.
			_ = restoreFile(join(backupsDir, bak), configPath)
			return fmt.Errorf("写后校验失败，已尝试恢复备份: %w", err)
		}

		// Update active state last.
		if err := tx.SetActive(profileID); err != nil {
			// Severe: config written but profile state failed — try restore config.
			_ = restoreFile(join(backupsDir, bak), configPath)
			return fmt.Errorf("严重错误: 配置已写入但无法更新 Profile 状态，已尝试恢复配置: %w", err)
		}

		result.Profile = p
		return nil
	})
	return result, err
}

// ActivateOfficial removes mid-station config and clears active profile.
func ActivateOfficial(store profiles.ProfileStore, configPath, backupsDir string) (backupName string, err error) {
	err = profiles.WithLock(store, func(tx *profiles.Transaction) error {
		if _, err := config.Load(configPath); err != nil {
			return err
		}
		bak, err := CreateBackup(configPath, backupsDir)
		if err != nil {
			return err
		}
		backupName = bak
		if err := config.ApplyOfficial(configPath); err != nil {
			return fmt.Errorf("切回官方配置失败: %w", err)
		}
		tx.ClearActive()
		return nil
	})
	return backupName, err
}

// ActiveStatus returns current active profile and whether disk matches.
// Exit-code semantics are applied by the CLI layer via MismatchError.
func ActiveStatus(store profiles.ProfileStore, configPath string) (Status, error) {
	st := Status{ConfigPath: configPath}

	list, err := store.List()
	if err != nil {
		return st, err
	}
	var active *profiles.Profile
	for i := range list {
		if list[i].IsActive {
			p := list[i]
			active = &p
			break
		}
	}
	if active == nil {
		st.HasActive = false
		st.DiskMatches = true // no active → treat as match (exit 0)
		return st, nil
	}
	st.HasActive = true
	st.Profile = active

	// Check if config is readable.
	if _, err := config.Load(configPath); err != nil {
		return st, err
	}
	ok, err := config.Match(configPath, *active)
	if err != nil {
		return st, err
	}
	st.DiskMatches = ok
	return st, nil
}

// Restore restores a backup and clears active profile status.
func Restore(store profiles.ProfileStore, backupsDir, name, configPath string) (newBackup string, err error) {
	err = profiles.WithLock(store, func(tx *profiles.Transaction) error {
		bak, err := RestoreBackup(backupsDir, name, configPath)
		if err != nil {
			return err
		}
		newBackup = bak
		tx.ClearActive()
		return nil
	})
	return newBackup, err
}

func join(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + string(os.PathSeparator) + name
}

// Ensure types used by exit codes remain referenced for callers.
var _ = exitcodes.OK
