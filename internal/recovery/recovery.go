package recovery

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CorruptJSON renames a corrupt JSON file to a timestamped .corrupt-*.bak
// and writes a warning to stderr. Callers should then continue with empty data.
func CorruptJSON(path string) (bakPath string, err error) {
	ts := time.Now().Format("20060102-150405")
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	bakName := fmt.Sprintf("%s.corrupt-%s.bak", base, ts)
	bakPath = filepath.Join(dir, bakName)

	if err := os.Rename(path, bakPath); err != nil {
		return "", fmt.Errorf("无法备份损坏文件 %s: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "警告: %s 已损坏，已重命名为 %s，将以空列表继续运行。\n", path, bakPath)
	return bakPath, nil
}
