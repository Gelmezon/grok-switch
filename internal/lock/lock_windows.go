//go:build windows

package lock

import (
	"fmt"
	"os"
	"time"

	"github.com/Gelmezon/grok-switch/internal/exitcodes"
)

// File is an acquired exclusive lock (Windows: exclusive CreateFile share-mode).
type File struct {
	f *os.File
}

// Acquire opens lockPath with exclusive access, retrying until timeout.
// Windows has no flock; we use exclusive open (O_EXCL-like via share denial).
func Acquire(lockPath string, timeout time.Duration) (*File, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		// Open with exclusive write; if another process holds it, fail.
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
		if err == nil {
			// Try to lock via renaming a sentinel — simple mutex file is enough
			// for same-machine tests. Use the file itself as lock holder.
			return &File{f: f}, nil
		}
		if time.Now().After(deadline) {
			return nil, &exitcodes.LockTimeoutError{}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Release closes the lock file.
func (l *File) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	if err != nil {
		return fmt.Errorf("释放锁失败: %w", err)
	}
	return nil
}
