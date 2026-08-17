package gradlefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copyFixture puts a writable copy of the fixture in a temp dir.
func copyFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "build.gradle.kts")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBump(t *testing.T) {
	path := copyFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	cur, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	next := NextSnapshot(cur, "1.0.6")
	if next.Code != 8 || next.Name != "1.0.6-SNAPSHOT" {
		t.Fatalf("NextSnapshot = %+v, want {8 1.0.6-SNAPSHOT}", next)
	}
	if err := Bump(path, next); err != nil {
		t.Fatalf("Bump: %v", err)
	}

	got, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse after Bump: %v", err)
	}
	if got != next {
		t.Errorf("after Bump = %+v, want %+v", got, next)
	}

	// Everything except the two version lines must be byte-identical.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeLines := strings.Split(string(before), "\n")
	afterLines := strings.Split(string(after), "\n")
	if len(beforeLines) != len(afterLines) {
		t.Fatalf("line count changed: %d -> %d", len(beforeLines), len(afterLines))
	}
	for i := range beforeLines {
		if beforeLines[i] == afterLines[i] {
			continue
		}
		if !strings.Contains(afterLines[i], "versionCode") && !strings.Contains(afterLines[i], "versionName") {
			t.Errorf("line %d changed but is not a version line:\n  before: %q\n  after:  %q", i+1, beforeLines[i], afterLines[i])
		}
	}
	if !strings.Contains(string(after), "versionCode = 8") {
		t.Error("want versionCode = 8 in the rewritten file")
	}
	if !strings.Contains(string(after), `versionName = "1.0.6-SNAPSHOT"`) {
		t.Error(`want versionName = "1.0.6-SNAPSHOT" in the rewritten file`)
	}
	// Indentation is part of "every other line byte-identical".
	if !strings.Contains(string(after), "        versionCode = 8") {
		t.Error("want the original indentation preserved")
	}
}

func TestBump_RejectsFileWithoutVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "build.gradle.kts")
	if err := os.WriteFile(path, []byte("android {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Bump(path, Version{Code: 2, Name: "1.0.0"}); err == nil {
		t.Fatal("Bump on a file with no version = nil error, want one")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "android {}\n" {
		t.Errorf("file was modified despite the error: %q", raw)
	}
}
