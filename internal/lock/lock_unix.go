//go:build unix

package lock

import (
	"fmt"
	"os"
	"time"

	"github.com/Gelmezon/grok-switch/internal/exitcodes"
	"golang.org/x/sys/unix"
)

// File is an acquired exclusive flock.
type File struct {
	f *os.File
}

// Acquire opens lockPath and takes an exclusive non-blocking flock,
// retrying until timeout (default 5s). Returns LockTimeoutError on timeout.
func Acquire(lockPath string, timeout time.Duration) (*File, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("打开锁文件失败: %w", err)
	}
	deadline := time.Now().Add(timeout)
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return &File{f: f}, nil
		}
		if time.Now().After(deadline) {
			_ = f.Close()
			return nil, &exitcodes.LockTimeoutError{}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Release unlocks and closes the lock file.
func (l *File) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = unix.Flock(int(l.f.Fd()), unix.LOCK_UN)
	err := l.f.Close()
	l.f = nil
	return err
}
