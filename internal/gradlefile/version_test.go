package gradlefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixture = "testdata/build.gradle.kts"

func TestParse(t *testing.T) {
	v, err := Parse(fixture)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if v.Code != 7 {
		t.Errorf("Code = %d, want 7", v.Code)
	}
	if v.Name != "1.0.5-SNAPSHOT" {
		t.Errorf("Name = %q, want 1.0.5-SNAPSHOT", v.Name)
	}
	if !v.IsSnapshot() {
		t.Error("IsSnapshot = false, want true")
	}
}

func TestParse_MissingVersionName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "build.gradle.kts")
	if err := os.WriteFile(path, []byte("android {\n  defaultConfig {\n    versionCode = 3\n  }\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Parse(path)
	if err == nil {
		t.Fatal("Parse = nil error, want one naming versionName")
	}
	if !strings.Contains(err.Error(), "versionName") {
		t.Errorf("error = %v, want it to name versionName", err)
	}
}

func TestParse_MissingVersionCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "build.gradle.kts")
	if err := os.WriteFile(path, []byte(`versionName = "1.0.0"`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Parse(path)
	if err == nil || !strings.Contains(err.Error(), "versionCode") {
		t.Errorf("Parse error = %v, want it to name versionCode", err)
	}
}

func TestReleaseName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"1.0.5-SNAPSHOT", "1.0.5"},
		{"1.0.5", "1.0.5"},
		{"1.0.5-rc1", "1.0.5-rc1"},
		{"1.0.5-SNAPSHOT-SNAPSHOT", "1.0.5-SNAPSHOT"},
	}
	for _, tc := range tests {
		if got := (Version{Name: tc.in}).ReleaseName(); got != tc.want {
			t.Errorf("ReleaseName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
