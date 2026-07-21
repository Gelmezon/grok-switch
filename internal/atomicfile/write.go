package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Write writes data to path atomically: temp file → Sync → Chmod 0600 → Rename.
// On Windows, an existing destination is removed before rename (no atomic replace).
func Write(path string, data []byte) error {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(append([]byte{}, data...), '\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	tmp := fmt.Sprintf("%s.tmp-%d", path, os.Getpid())
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("同步临时文件失败: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("设置权限失败: %w", err)
	}
	if err := replace(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("替换文件失败: %w", err)
	}
	return nil
}

func replace(tmp, path string) error {
	if runtime.GOOS == "windows" {
		// Windows rename cannot overwrite existing files.
		if _, err := os.Stat(path); err == nil {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return os.Rename(tmp, path)
}
