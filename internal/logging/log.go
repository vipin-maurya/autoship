// Package logging opens the per-run log file that halts point at.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunID returns the identifier for a run started at t, usable as a filename on
// Windows (where colons are not).
func RunID(t time.Time) string {
	return "run-" + strings.ReplaceAll(t.UTC().Format(time.RFC3339), ":", "-")
}

// Log is a run's logger plus the path of the file it writes to, which is what
// a halt record stores so a human can find it later.
type Log struct {
	*slog.Logger
	Path  string
	w     io.Writer
	close func() error
}

// Writer returns the raw sink behind the logger, for streaming a subprocess's
// output into the same file the structured entries go to.
func (l *Log) Writer() io.Writer {
	if l == nil || l.w == nil {
		return io.Discard
	}
	return l.w
}

// Close flushes and closes the log file.
func (l *Log) Close() error {
	if l == nil || l.close == nil {
		return nil
	}
	err := l.close()
	l.close = nil
	return err
}

// New opens dir/<runID>.log and returns a logger writing to both it and tee
// (typically stdout, so an interactive run shows its work).
func New(dir, runID string, tee io.Writer) (*Log, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	path := filepath.Join(dir, runID+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	var w io.Writer = f
	if tee != nil {
		w = io.MultiWriter(f, tee)
	}
	logger := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
	return &Log{Logger: logger, Path: path, w: w, close: f.Close}, nil
}

// Discard returns a logger that writes nowhere, for tests and for the paths
// that run before a log file exists.
func Discard() *Log {
	return &Log{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), w: io.Discard}
}
