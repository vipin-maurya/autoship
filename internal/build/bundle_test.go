package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeBundle(t *testing.T, moduleDir, name string, modTime time.Time) string {
	t.Helper()
	dir := filepath.Join(moduleDir, filepath.FromSlash("build/outputs/bundle/release"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("aab"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFindBundle(t *testing.T) {
	moduleDir := t.TempDir()
	old := time.Now().Add(-2 * time.Hour)
	writeBundle(t, moduleDir, "app-release-old.aab", old)
	newest := writeBundle(t, moduleDir, "app-release.aab", time.Now())

	got, err := FindBundle(moduleDir)
	if err != nil {
		t.Fatalf("FindBundle: %v", err)
	}
	if got != newest {
		t.Errorf("FindBundle = %q, want the most recently modified %q", got, newest)
	}
}

func TestFindBundle_None(t *testing.T) {
	moduleDir := t.TempDir()
	_, err := FindBundle(moduleDir)
	if err == nil {
		t.Fatal("FindBundle = nil error, want one")
	}
	if !strings.Contains(filepath.ToSlash(err.Error()), "build/outputs/bundle/release") {
		t.Errorf("error = %v, want it to name the searched directory", err)
	}
}

func TestModuleDir(t *testing.T) {
	tests := []struct {
		module string
		want   string
	}{
		{":app", filepath.Join(`C:\repo`, "app")},
		{"app", filepath.Join(`C:\repo`, "app")},
		{":features:core", filepath.Join(`C:\repo`, "features", "core")},
		{"", `C:\repo`},
	}
	for _, tc := range tests {
		if got := ModuleDir(`C:\repo`, tc.module); got != tc.want {
			t.Errorf("ModuleDir(%q) = %q, want %q", tc.module, got, tc.want)
		}
	}
}
