package exitcodes

import (
	"errors"
	"fmt"
)

// Exit codes as defined in project.md section 五.
const (
	OK             = 0
	General        = 1
	Usage          = 2
	Mismatch       = 3
	NotFound       = 4
	LockTimeout    = 5
	ConfigParse    = 6
	BackupRestore  = 7
)

// Typed errors for exit-code mapping.
var (
	ErrUsage        = errors.New("usage error")
	ErrMismatch     = errors.New("status mismatch")
	ErrNotFound     = errors.New("profile not found")
	ErrLockTimeout  = errors.New("lock timeout")
	ErrConfigParse  = errors.New("config parse error")
	ErrBackup       = errors.New("backup or restore failed")
)

// UsageError wraps a usage/argument error with a message.
type UsageError struct {
	Msg string
}

func (e *UsageError) Error() string { return e.Msg }
func (e *UsageError) Is(target error) bool { return target == ErrUsage }

// NotFoundError indicates a missing profile.
type NotFoundError struct {
	Name string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("Profile 不存在: %s", e.Name)
}
func (e *NotFoundError) Is(target error) bool { return target == ErrNotFound }

// LockTimeoutError indicates flock wait timed out.
type LockTimeoutError struct{}

func (e *LockTimeoutError) Error() string {
	return "另一个 grok-switch 操作正在执行（等待超时）"
}
func (e *LockTimeoutError) Is(target error) bool { return target == ErrLockTimeout }

// ConfigParseError indicates TOML parse failure.
type ConfigParseError struct {
	Path string
	Err  error
}

func (e *ConfigParseError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("无法解析 %s：%v", e.Path, e.Err)
	}
	return fmt.Sprintf("无法解析 %s", e.Path)
}
func (e *ConfigParseError) Unwrap() error { return e.Err }
func (e *ConfigParseError) Is(target error) bool { return target == ErrConfigParse }

// BackupError indicates backup/restore failure.
type BackupError struct {
	Msg string
	Err error
}

func (e *BackupError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Err)
	}
	return e.Msg
}
func (e *BackupError) Unwrap() error { return e.Err }
func (e *BackupError) Is(target error) bool { return target == ErrBackup }

// MismatchError indicates status disk/profile mismatch (exit 3).
type MismatchError struct {
	Msg string
}

func (e *MismatchError) Error() string { return e.Msg }
func (e *MismatchError) Is(target error) bool { return target == ErrMismatch }

// Code maps an error to a process exit code.
func Code(err error) int {
	if err == nil {
		return OK
	}
	switch {
	case errors.Is(err, ErrUsage):
		return Usage
	case errors.Is(err, ErrMismatch):
		return Mismatch
	case errors.Is(err, ErrNotFound):
		return NotFound
	case errors.Is(err, ErrLockTimeout):
		return LockTimeout
	case errors.Is(err, ErrConfigParse):
		return ConfigParse
	case errors.Is(err, ErrBackup):
		return BackupRestore
	default:
		return General
	}
}

// Format returns a user-facing error string with the unified prefix.
func Format(err error) string {
	if err == nil {
		return ""
	}
	return "错误: " + err.Error()
}
