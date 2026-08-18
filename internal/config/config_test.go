package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_ValidFile(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "valid.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.Repo.Branch, "main"; got != want {
		t.Errorf("repo.branch = %q, want %q", got, want)
	}
	if got, want := cfg.Gradle.Bundle, ":app:bundleRelease"; got != want {
		t.Errorf("gradle.bundle = %q, want %q", got, want)
	}
	if got, want := cfg.Play.Track, "alpha"; got != want {
		t.Errorf("play.track = %q, want %q", got, want)
	}
	if got, want := strings.Join(cfg.Notes.Source, ","), "file"; got != want {
		t.Errorf("notes.source = %v, want [file]", cfg.Notes.Source)
	}
	if got, want := cfg.App.Package, "com.example.myapp"; got != want {
		t.Errorf("app.package = %q, want %q", got, want)
	}
}

func TestLoad_ExpandsEnv(t *testing.T) {
	t.Setenv("USERPROFILE", `C:\Users\test`)
	cfg, err := Load(filepath.Join("testdata", "valid.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if strings.Contains(cfg.Artifacts.Root, "${") {
		t.Errorf("artifacts.root = %q, want it expanded", cfg.Artifacts.Root)
	}
	if !strings.HasPrefix(cfg.Artifacts.Root, `C:\Users\test`) {
		t.Errorf("artifacts.root = %q, want it to start with the expanded USERPROFILE", cfg.Artifacts.Root)
	}
	// ${version} is a per-release placeholder, not an environment variable.
	if !strings.Contains(cfg.Notes.FilePath, "${version}") {
		t.Errorf("notes.file_path = %q, want ${version} preserved", cfg.Notes.FilePath)
	}
}

func TestValidate_RejectsMissingFields(t *testing.T) {
	base := func() *Config {
		c := &Config{}
		c.Repo.Path = `C:\repo`
		c.App.Module = ":app"
		c.App.Package = "com.example"
		c.App.GradleFile = "app/build.gradle.kts"
		c.Gradle.UnitTests = ":app:testDebugUnitTest"
		c.Gradle.Bundle = ":app:bundleRelease"
		c.Play.Track = "alpha"
		c.Artifacts.Root = `C:\artifacts`
		c.applyDefaults()
		return c
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("baseline config should be valid, got %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{"missing repo.path", func(c *Config) { c.Repo.Path = "" }, "repo.path"},
		{"missing play.track", func(c *Config) { c.Play.Track = "" }, "play.track"},
		{"missing artifacts.root", func(c *Config) { c.Artifacts.Root = "" }, "artifacts.root"},
		{"bad ui mode", func(c *Config) { c.UIValidation.Mode = "hologram" }, "ui_validation.mode"},
		{"bad rollout", func(c *Config) { c.Play.Rollout = "yolo" }, "play.rollout"},
		{"bad notes source", func(c *Config) { c.Notes.Source = []string{"tarot"} }, "notes.source"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mutate(c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want an error naming %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("Validate() = %v, want the message to name %q", err, tc.wantSub)
			}
		})
	}
}

func TestLoad_MissingFileIsAnError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("Load on a missing file = nil error, want one")
	}
}

func TestLoad_AppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "autoship.yaml")
	minimal := `repo:
  path: C:\repo
app:
  module: ":app"
  package: com.example
  gradle_file: app/build.gradle.kts
gradle:
  unit_tests: ":app:testDebugUnitTest"
  bundle: ":app:bundleRelease"
play:
  track: alpha
artifacts:
  root: C:\artifacts
`
	if err := os.WriteFile(path, []byte(minimal), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Repo.Branch != "main" || cfg.Repo.Remote != "origin" {
		t.Errorf("branch/remote = %q/%q, want main/origin", cfg.Repo.Branch, cfg.Repo.Remote)
	}
	if cfg.Play.Rollout != RolloutDraft {
		t.Errorf("play.rollout = %q, want %q", cfg.Play.Rollout, RolloutDraft)
	}
	if cfg.MaxRunDur != DefaultMaxRunDuration {
		t.Errorf("max_run_duration = %v, want %v", cfg.MaxRunDur, DefaultMaxRunDuration)
	}
}
