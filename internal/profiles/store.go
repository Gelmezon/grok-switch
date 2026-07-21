package profiles

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Gelmezon/grok-switch/internal/atomicfile"
	"github.com/Gelmezon/grok-switch/internal/exitcodes"
	"github.com/Gelmezon/grok-switch/internal/lock"
	"github.com/Gelmezon/grok-switch/internal/recovery"
)

// ProfileStore is the persistence interface for profiles.
type ProfileStore interface {
	List() ([]Profile, error)
	Get(id string) (Profile, error)
	GetByName(name string) ([]Profile, error)
	Create(profile Profile) (Profile, error)
	Update(id string, profile Profile) (Profile, error)
	Delete(id string) error
	SetActive(id string) error
	ClearActive() error
}

// fileStore implements ProfileStore with atomic JSON writes and flock.
type fileStore struct {
	path     string
	lockPath string
	mu       sync.Mutex
	timeout  time.Duration
}

// NewStore creates a ProfileStore backed by path (profiles.json).
// lockPath is the flock file (typically ~/.grok_switch/grok-switch.lock).
func NewStore(path, lockPath string) ProfileStore {
	return &fileStore{
		path:     path,
		lockPath: lockPath,
		timeout:  5 * time.Second,
	}
}

// NewStoreWithTimeout is like NewStore but allows custom lock timeout (tests).
func NewStoreWithTimeout(path, lockPath string, timeout time.Duration) ProfileStore {
	return &fileStore{
		path:     path,
		lockPath: lockPath,
		timeout:  timeout,
	}
}

type storeData struct {
	Profiles []Profile `json:"profiles"`
}

func (s *fileStore) withLock(fn func(data *storeData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	lf, err := lock.Acquire(s.lockPath, s.timeout)
	if err != nil {
		return err
	}
	defer func() { _ = lf.Release() }()

	data, err := s.readUnlocked()
	if err != nil {
		return err
	}
	if err := fn(data); err != nil {
		return err
	}
	return s.writeUnlocked(data)
}

func (s *fileStore) withLockRead(fn func(data *storeData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	lf, err := lock.Acquire(s.lockPath, s.timeout)
	if err != nil {
		return err
	}
	defer func() { _ = lf.Release() }()

	data, err := s.readUnlocked()
	if err != nil {
		return err
	}
	return fn(data)
}

func (s *fileStore) readUnlocked() (*storeData, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &storeData{Profiles: []Profile{}}, nil
		}
		return nil, fmt.Errorf("读取 profiles 失败: %w", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return &storeData{Profiles: []Profile{}}, nil
	}
	var data storeData
	if err := json.Unmarshal(raw, &data); err != nil {
		// Corrupt recovery.
		if _, rerr := recovery.CorruptJSON(s.path); rerr != nil {
			return nil, fmt.Errorf("profiles.json 损坏且无法恢复: %v; %w", err, rerr)
		}
		return &storeData{Profiles: []Profile{}}, nil
	}
	if data.Profiles == nil {
		data.Profiles = []Profile{}
	}
	return &data, nil
}

func (s *fileStore) writeUnlocked(data *storeData) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 profiles 失败: %w", err)
	}
	if err := atomicfile.Write(s.path, raw); err != nil {
		return fmt.Errorf("写入 profiles 失败: %w", err)
	}
	return nil
}

func (s *fileStore) List() ([]Profile, error) {
	var out []Profile
	err := s.withLockRead(func(data *storeData) error {
		out = append([]Profile(nil), data.Profiles...)
		return nil
	})
	return out, err
}

func (s *fileStore) Get(id string) (Profile, error) {
	var found Profile
	err := s.withLockRead(func(data *storeData) error {
		for _, p := range data.Profiles {
			if p.ID == id {
				found = p
				return nil
			}
		}
		return &exitcodes.NotFoundError{Name: id}
	})
	return found, err
}

func (s *fileStore) GetByName(name string) ([]Profile, error) {
	name = strings.TrimSpace(name)
	var matches []Profile
	err := s.withLockRead(func(data *storeData) error {
		for _, p := range data.Profiles {
			if p.Name == name {
				matches = append(matches, p)
			}
		}
		return nil
	})
	return matches, err
}

func (s *fileStore) Create(profile Profile) (Profile, error) {
	Normalize(&profile)
	if err := Validate(profile); err != nil {
		return Profile{}, err
	}
	now := time.Now().UTC()
	profile.ID = newID()
	profile.CreatedAt = now
	profile.UpdatedAt = now
	profile.IsActive = false

	err := s.withLock(func(data *storeData) error {
		data.Profiles = append(data.Profiles, profile)
		return nil
	})
	if err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func (s *fileStore) Update(id string, profile Profile) (Profile, error) {
	Normalize(&profile)
	if err := Validate(profile); err != nil {
		return Profile{}, err
	}
	var updated Profile
	err := s.withLock(func(data *storeData) error {
		for i, p := range data.Profiles {
			if p.ID == id {
				profile.ID = p.ID
				profile.CreatedAt = p.CreatedAt
				profile.UpdatedAt = time.Now().UTC()
				profile.IsActive = p.IsActive
				data.Profiles[i] = profile
				updated = profile
				return nil
			}
		}
		return &exitcodes.NotFoundError{Name: id}
	})
	return updated, err
}

func (s *fileStore) Delete(id string) error {
	return s.withLock(func(data *storeData) error {
		for i, p := range data.Profiles {
			if p.ID == id {
				data.Profiles = append(data.Profiles[:i], data.Profiles[i+1:]...)
				return nil
			}
		}
		return &exitcodes.NotFoundError{Name: id}
	})
}

func (s *fileStore) SetActive(id string) error {
	return s.withLock(func(data *storeData) error {
		found := false
		for i := range data.Profiles {
			if data.Profiles[i].ID == id {
				data.Profiles[i].IsActive = true
				data.Profiles[i].UpdatedAt = time.Now().UTC()
				found = true
			} else {
				data.Profiles[i].IsActive = false
			}
		}
		if !found {
			return &exitcodes.NotFoundError{Name: id}
		}
		return nil
	})
}

func (s *fileStore) ClearActive() error {
	return s.withLock(func(data *storeData) error {
		for i := range data.Profiles {
			data.Profiles[i].IsActive = false
		}
		return nil
	})
}

// WithLock runs fn while holding both the mutex and flock.
// Useful for switcher transactions that need exclusive multi-step access.
// fn receives a Transaction that can read/write profiles without re-locking.
func WithLock(s ProfileStore, fn func(tx *Transaction) error) error {
	fs, ok := s.(*fileStore)
	if !ok {
		return fmt.Errorf("WithLock 仅支持文件存储实现")
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()

	lf, err := lock.Acquire(fs.lockPath, fs.timeout)
	if err != nil {
		return err
	}
	defer func() { _ = lf.Release() }()

	data, err := fs.readUnlocked()
	if err != nil {
		return err
	}
	tx := &Transaction{store: fs, data: data, dirty: false}
	if err := fn(tx); err != nil {
		return err
	}
	if tx.dirty {
		return fs.writeUnlocked(tx.data)
	}
	return nil
}

// Transaction provides locked access to profile data.
type Transaction struct {
	store *fileStore
	data  *storeData
	dirty bool
}

// List returns a copy of all profiles.
func (tx *Transaction) List() []Profile {
	return append([]Profile(nil), tx.data.Profiles...)
}

// Get returns a profile by ID.
func (tx *Transaction) Get(id string) (Profile, error) {
	for _, p := range tx.data.Profiles {
		if p.ID == id {
			return p, nil
		}
	}
	return Profile{}, &exitcodes.NotFoundError{Name: id}
}

// SetActive marks id as the sole active profile (in memory).
func (tx *Transaction) SetActive(id string) error {
	found := false
	for i := range tx.data.Profiles {
		if tx.data.Profiles[i].ID == id {
			tx.data.Profiles[i].IsActive = true
			tx.data.Profiles[i].UpdatedAt = time.Now().UTC()
			found = true
		} else {
			tx.data.Profiles[i].IsActive = false
		}
	}
	if !found {
		return &exitcodes.NotFoundError{Name: id}
	}
	tx.dirty = true
	return nil
}

// ClearActive clears all active flags (in memory).
func (tx *Transaction) ClearActive() {
	for i := range tx.data.Profiles {
		tx.data.Profiles[i].IsActive = false
	}
	tx.dirty = true
}

// Active returns the active profile if any.
func (tx *Transaction) Active() *Profile {
	for i := range tx.data.Profiles {
		if tx.data.Profiles[i].IsActive {
			p := tx.data.Profiles[i]
			return &p
		}
	}
	return nil
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Extremely unlikely; fall back to time-based hex.
		return hex.EncodeToString([]byte(fmt.Sprintf("%016x", time.Now().UnixNano())))[:16]
	}
	return hex.EncodeToString(b)
}
