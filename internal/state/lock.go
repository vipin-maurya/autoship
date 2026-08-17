package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// ErrLocked reports that another autoship run holds the lock and is still
// alive. Overlapping invocations are expected, not exceptional (spec §5).
var ErrLocked = errors.New("another autoship run is in progress")

type lockFile struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
}

// Lock is a held run lock. Release it when the run finishes.
type Lock struct {
	path string
}

func lockPath(dir string) string { return filepath.Join(dir, "lock") }

// Acquire takes the run lock in dir. A lock held by a live PID yields
// ErrLocked. A lock whose PID is dead, or which is older than maxRunDuration,
// is stale and is reclaimed.
func (s Store) Acquire(maxRunDuration time.Duration) (*Lock, error) {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	path := lockPath(s.Dir)

	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		var held lockFile
		if jsonErr := json.Unmarshal(raw, &held); jsonErr != nil {
			// An unreadable lock is not a reason to wedge forever.
			break
		}
		fresh := time.Since(held.StartedAt) < maxRunDuration
		if fresh && processAlive(held.PID) {
			return nil, fmt.Errorf("%w (pid %d, started %s)", ErrLocked, held.PID, held.StartedAt.Format(time.RFC3339))
		}
	case errors.Is(err, fs.ErrNotExist):
		// No lock held.
	default:
		return nil, fmt.Errorf("read lock: %w", err)
	}

	mine, err := json.Marshal(lockFile{PID: os.Getpid(), StartedAt: time.Now()})
	if err != nil {
		return nil, fmt.Errorf("encode lock: %w", err)
	}
	if err := os.WriteFile(path, mine, 0o600); err != nil {
		return nil, fmt.Errorf("write lock: %w", err)
	}
	return &Lock{path: path}, nil
}

// Release drops the lock. It is safe to call more than once.
func (l *Lock) Release() error {
	if l == nil || l.path == "" {
		return nil
	}
	err := os.Remove(l.path)
	l.path = ""
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("release lock: %w", err)
	}
	return nil
}
