// Package state persists autoship's run state, halt record and run lock
// outside the repository being released (spec §5).
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Run statuses (spec §5).
const (
	StatusIdle    = "idle"
	StatusRunning = "running"
	StatusHalted  = "halted"
)

// Halt records why the pipeline stopped and where to look.
type Halt struct {
	Stage  string    `json:"stage"`
	Reason string    `json:"reason"`
	SHA    string    `json:"sha"`
	Log    string    `json:"log"`
	At     time.Time `json:"at"`
}

// State is the whole of state.json.
type State struct {
	LastProcessedSHA         string    `json:"last_processed_sha"`
	LastPublishedVersionCode int       `json:"last_published_version_code"`
	LastPublishedVersionName string    `json:"last_published_version_name"`
	Status                   string    `json:"status"`
	Halted                   *Halt     `json:"halted,omitempty"`
	LastRunAt                time.Time `json:"last_run_at"`
}

// Store reads and writes state.json in Dir.
type Store struct {
	Dir string
}

// DirFor returns the per-repo state directory under root, keyed by a short hash
// of the repo path so two repos never share state.
func DirFor(root, repoPath string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(repoPath))))
	return filepath.Join(root, hex.EncodeToString(sum[:])[:12])
}

func (s Store) path() string { return filepath.Join(s.Dir, "state.json") }

// Load returns the persisted state, or a zero State if none exists yet.
func (s Store) Load() (State, error) {
	raw, err := os.ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return State{Status: StatusIdle}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read state: %w", err)
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return State{}, fmt.Errorf("parse state: %w", err)
	}
	if st.Status == "" {
		st.Status = StatusIdle
	}
	return st, nil
}

// Save writes state.json atomically: a sibling temp file, then a rename, so a
// crash mid-write can never leave a truncated state behind.
func (s Store) Save(st State) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	raw = append(raw, '\n')
	tmp := s.path() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := os.Rename(tmp, s.path()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit state: %w", err)
	}
	return nil
}

// LogsDir is where per-run logs are written for this store.
func (s Store) LogsDir() string { return filepath.Join(s.Dir, "logs") }
