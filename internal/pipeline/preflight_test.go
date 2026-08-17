package pipeline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/release"
	"github.com/vipinm/autoship/internal/state"
)

// gradleRepo writes a minimal repo containing a Gradle file with the given
// version pair and returns a config pointing at it.
func gradleRepo(t *testing.T, versionCode int, versionName string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	appDir := filepath.Join(dir, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf("android {\n    defaultConfig {\n        versionCode = %d\n        versionName = %q\n    }\n}\n",
		versionCode, versionName)
	if err := os.WriteFile(filepath.Join(appDir, "build.gradle.kts"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Repo.Path = dir
	cfg.App.Module = ":app"
	cfg.App.GradleFile = "app/build.gradle.kts"
	return cfg
}

func TestPreflight_ReturnsRelease(t *testing.T) {
	cfg := gradleRepo(t, 8, "1.0.5-SNAPSHOT")
	st := state.State{LastPublishedVersionCode: 7, LastPublishedVersionName: "1.0.4"}

	rel, err := Preflight(cfg, st)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	want := release.Release{Name: "1.0.5", Code: 8, Kind: release.Patch, PreviousName: "1.0.4", PreviousCode: 7}
	if rel != want {
		t.Errorf("Preflight = %+v, want %+v", rel, want)
	}
}

func TestPreflight_FirstReleaseIsMajor(t *testing.T) {
	cfg := gradleRepo(t, 1, "1.0.0-SNAPSHOT")
	rel, err := Preflight(cfg, state.State{})
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if rel.Kind != release.Major {
		t.Errorf("Kind = %v, want %v", rel.Kind, release.Major)
	}
}

func TestPreflight_HaltsWhenVersionCodeNotGreater(t *testing.T) {
	cfg := gradleRepo(t, 7, "1.0.6-SNAPSHOT")
	st := state.State{LastPublishedVersionCode: 7, LastPublishedVersionName: "1.0.5"}

	_, err := Preflight(cfg, st)
	if err == nil {
		t.Fatal("Preflight = nil error, want a halt")
	}
	var he *HaltError
	if !errors.As(err, &he) {
		t.Fatalf("error = %T (%v), want *HaltError", err, err)
	}
	if he.Stage != StagePreflight {
		t.Errorf("Stage = %q, want %q", he.Stage, StagePreflight)
	}
	if !strings.Contains(he.Reason, "7") || !strings.Contains(he.Reason, "versionCode") {
		t.Errorf("Reason = %q, want it to name versionCode and 7", he.Reason)
	}
}

func TestPreflight_HaltsWhenVersionAlreadyPublished(t *testing.T) {
	cfg := gradleRepo(t, 8, "1.0.5-SNAPSHOT")
	st := state.State{LastPublishedVersionCode: 7, LastPublishedVersionName: "1.0.5"}

	_, err := Preflight(cfg, st)
	var he *HaltError
	if !errors.As(err, &he) {
		t.Fatalf("error = %v, want a halt", err)
	}
	if !strings.Contains(he.Reason, "1.0.5") {
		t.Errorf("Reason = %q, want it to name the version", he.Reason)
	}
}

func TestPreflight_HaltsWhenVersionWentBackwards(t *testing.T) {
	cfg := gradleRepo(t, 9, "1.0.4-SNAPSHOT")
	st := state.State{LastPublishedVersionCode: 8, LastPublishedVersionName: "1.0.5"}

	_, err := Preflight(cfg, st)
	var he *HaltError
	if !errors.As(err, &he) {
		t.Fatalf("error = %v, want a halt", err)
	}
	if !strings.Contains(he.Reason, "lower") {
		t.Errorf("Reason = %q, want it to explain the ordering", he.Reason)
	}
}

func TestPreflight_HaltsWhenGradleFileMissing(t *testing.T) {
	cfg := gradleRepo(t, 8, "1.0.5-SNAPSHOT")
	cfg.App.GradleFile = "app/does-not-exist.gradle.kts"

	_, err := Preflight(cfg, state.State{})
	var he *HaltError
	if !errors.As(err, &he) {
		t.Fatalf("error = %v, want a halt", err)
	}
	if !strings.Contains(he.Reason, "does-not-exist") {
		t.Errorf("Reason = %q, want it to name the file", he.Reason)
	}
}
