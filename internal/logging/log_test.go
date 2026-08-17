package logging

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNew_TeesToFileAndWriter(t *testing.T) {
	dir := t.TempDir()
	var tee bytes.Buffer

	lg, err := New(dir, RunID(time.Now()), &tee)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	lg.Info("build started", "stage", "S2")
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := os.ReadFile(lg.Path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(raw), "build started") {
		t.Errorf("log file = %q, want it to contain the message", raw)
	}
	if !strings.Contains(tee.String(), "build started") {
		t.Errorf("tee = %q, want it to contain the message", tee.String())
	}
	if !strings.Contains(string(raw), "S2") {
		t.Errorf("log file = %q, want it to contain the attribute", raw)
	}
}

func TestRunID_IsAFilename(t *testing.T) {
	id := RunID(time.Date(2026, 8, 17, 9, 14, 2, 0, time.UTC))
	if strings.ContainsAny(id, `:\/*?"<>|`) {
		t.Errorf("RunID = %q, want no characters Windows rejects in filenames", id)
	}
	if !strings.HasPrefix(id, "run-") {
		t.Errorf("RunID = %q, want a run- prefix", id)
	}
}

func TestDiscard_IsUsable(t *testing.T) {
	lg := Discard()
	lg.Info("no destination")
	if err := lg.Close(); err != nil {
		t.Errorf("Close on a discard log = %v, want nil", err)
	}
}
