package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load on empty dir: %v", err)
	}
	if got.LastProcessedSHA != "" || got.LastPublishedVersionCode != 0 {
		t.Errorf("Load on empty dir = %+v, want zero state", got)
	}
	if got.Status != StatusIdle {
		t.Errorf("Status = %q, want %q", got.Status, StatusIdle)
	}

	want := State{
		LastProcessedSHA:         "a1b2c3",
		LastPublishedVersionCode: 7,
		LastPublishedVersionName: "1.0.5",
		Status:                   StatusIdle,
	}
	if err := s.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err = s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.LastProcessedSHA != want.LastProcessedSHA {
		t.Errorf("LastProcessedSHA = %q, want %q", got.LastProcessedSHA, want.LastProcessedSHA)
	}
	if got.LastPublishedVersionCode != want.LastPublishedVersionCode {
		t.Errorf("LastPublishedVersionCode = %d, want %d", got.LastPublishedVersionCode, want.LastPublishedVersionCode)
	}
}

func TestStore_SaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	if err := s.Save(State{Status: StatusIdle}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file %q survived Save", e.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err != nil {
		t.Errorf("state.json missing after Save: %v", err)
	}
}

func TestDirFor_IsStableAndDistinct(t *testing.T) {
	a := DirFor(`C:\root`, `C:\repos\one`)
	b := DirFor(`C:\root`, `C:\repos\two`)
	if a == b {
		t.Errorf("distinct repos share a state dir: %q", a)
	}
	if a != DirFor(`C:\root`, `C:\repos\one`) {
		t.Error("DirFor is not stable across calls")
	}
}
