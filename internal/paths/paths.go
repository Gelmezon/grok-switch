package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// Paths holds resolved filesystem locations for the switcher.
type Paths struct {
	GrokHome        string
	GrokConfig      string
	SwitchHome      string
	ProfilesFile    string
	BackupsDir      string
	LockFile        string
	UpdateStateFile string
}

// Resolve returns paths based on environment variables and defaults.
//
// Priority:
//
//	GROK_HOME         → ~/.grok
//	GROK_CONFIG       → $GROK_HOME/config.toml
//	GROK_SWITCH_HOME  → ~/.grok_switch
func Resolve() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("无法获取用户主目录: %w", err)
	}

	grokHome := envOr("GROK_HOME", filepath.Join(home, ".grok"))
	grokConfig := envOr("GROK_CONFIG", filepath.Join(grokHome, "config.toml"))
	switchHome := envOr("GROK_SWITCH_HOME", filepath.Join(home, ".grok_switch"))

	return Paths{
		GrokHome:        grokHome,
		GrokConfig:      grokConfig,
		SwitchHome:      switchHome,
		ProfilesFile:    filepath.Join(switchHome, "profiles.json"),
		BackupsDir:      filepath.Join(switchHome, "backups"),
		LockFile:        filepath.Join(switchHome, "grok-switch.lock"),
		UpdateStateFile: filepath.Join(switchHome, "update-state.json"),
	}, nil
}

// InitDirs creates required directories with 0700 permissions.
// Existing directories are explicitly chmod'd to 0700.
func InitDirs(p Paths) error {
	dirs := []string{p.SwitchHome, p.BackupsDir, p.GrokHome}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("创建目录失败 %s: %w", d, err)
		}
		if err := os.Chmod(d, 0o700); err != nil {
			return fmt.Errorf("设置目录权限失败 %s: %w", d, err)
		}
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
