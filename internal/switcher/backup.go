package switcher

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Gelmezon/grok-switch/internal/config"
	"github.com/Gelmezon/grok-switch/internal/exitcodes"
)

// BackupInfo describes a single backup file.
type BackupInfo struct {
	Name    string
	Path    string
	ModTime time.Time
}

// validateBackupName rejects path traversal and non-toml names.
func validateBackupName(name string) error {
	if filepath.Base(name) != name {
		return fmt.Errorf("非法文件名：不允许路径分隔符")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("非法文件名：不允许路径分隔符")
	}
	if !strings.HasSuffix(name, ".toml") {
		return fmt.Errorf("非法文件名：只接受 .toml 文件")
	}
	return nil
}

// CreateBackup copies configPath into backupsDir with a unique name.
// Returns the backup filename (not full path).
// If configPath does not exist, creates an empty marker backup of empty content.
func CreateBackup(configPath, backupsDir string) (string, error) {
	if err := os.MkdirAll(backupsDir, 0o700); err != nil {
		return "", &exitcodes.BackupError{Msg: "创建备份目录失败", Err: err}
	}
	_ = os.Chmod(backupsDir, 0o700)

	name := backupFileName(time.Now())
	dest := filepath.Join(backupsDir, name)

	var content []byte
	raw, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", &exitcodes.BackupError{Msg: "读取配置以备份失败", Err: err}
		}
		content = []byte("# empty backup — source config did not exist\n")
	} else {
		content = raw
	}

	if err := config.AtomicWriteFile(dest, content); err != nil {
		return "", &exitcodes.BackupError{Msg: "写入备份失败", Err: err}
	}
	return name, nil
}

func backupFileName(t time.Time) string {
	// config-20260721-153012.482-a1b2.toml
	ms := t.Nanosecond() / 1e6
	randPart := randomHex(2) // 4 hex chars
	return fmt.Sprintf("config-%s.%03d-%s.toml",
		t.Format("20060102-150405"),
		ms,
		randPart,
	)
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%04x", time.Now().UnixNano()&0xffff)
	}
	return hex.EncodeToString(b)
}

// ListBackups returns backups newest first.
func ListBackups(backupsDir string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []BackupInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".toml") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupInfo{
			Name:    name,
			Path:    filepath.Join(backupsDir, name),
			ModTime: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime.After(out[j].ModTime)
	})
	return out, nil
}

// PruneBackups keeps the newest keepCount backups; deletes the rest.
func PruneBackups(backupsDir string, keepCount int) (deleted int, err error) {
	if keepCount < 0 {
		keepCount = 0
	}
	list, err := ListBackups(backupsDir)
	if err != nil {
		return 0, err
	}
	if len(list) <= keepCount {
		return 0, nil
	}
	for _, b := range list[keepCount:] {
		if err := os.Remove(b.Path); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// RestoreBackup validates name, backs up current config, writes backup to configPath.
func RestoreBackup(backupsDir, name, configPath string) (newBackup string, err error) {
	if err := validateBackupName(name); err != nil {
		return "", &exitcodes.BackupError{Msg: err.Error()}
	}
	src := filepath.Join(backupsDir, name)
	// Ensure resolved path is still under backupsDir.
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return "", &exitcodes.BackupError{Msg: "解析备份路径失败", Err: err}
	}
	absDir, err := filepath.Abs(backupsDir)
	if err != nil {
		return "", &exitcodes.BackupError{Msg: "解析备份目录失败", Err: err}
	}
	if !strings.HasPrefix(absSrc, absDir+string(os.PathSeparator)) && absSrc != absDir {
		return "", &exitcodes.BackupError{Msg: "非法文件名：路径越界"}
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return "", &exitcodes.BackupError{Msg: fmt.Sprintf("备份不存在: %s", name)}
		}
		return "", &exitcodes.BackupError{Msg: "读取备份失败", Err: err}
	}

	// Backup current config before restore.
	bak, err := CreateBackup(configPath, backupsDir)
	if err != nil {
		return "", err
	}

	if err := config.AtomicWriteFile(configPath, raw); err != nil {
		return "", &exitcodes.BackupError{Msg: "恢复备份失败", Err: err}
	}
	// Verify parseable.
	if _, err := config.Load(configPath); err != nil {
		// Try to restore the just-created backup of previous state.
		_ = restoreFile(filepath.Join(backupsDir, bak), configPath)
		return "", &exitcodes.BackupError{Msg: "恢复后配置无法解析", Err: err}
	}
	return bak, nil
}

func restoreFile(src, dest string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return config.AtomicWriteFile(dest, raw)
}
