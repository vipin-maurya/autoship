// Package config loads and validates autoship.yaml.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Repo locates the Android repository and the branch that triggers releases.
type Repo struct {
	Path   string `yaml:"path"`
	Branch string `yaml:"branch"`
	Remote string `yaml:"remote"`
}

// App identifies the Gradle module being released.
type App struct {
	Module     string `yaml:"module"`
	Package    string `yaml:"package"`
	GradleFile string `yaml:"gradle_file"`
}

// Gradle names the tasks each build stage invokes.
type Gradle struct {
	UnitTests string `yaml:"unit_tests"`
	Lint      string `yaml:"lint"`
	Bundle    string `yaml:"bundle"`
	APK       string `yaml:"apk"`
}

// UI validation modes (spec §6).
const (
	UIModeJVM      = "jvm"
	UIModeEmulator = "emulator"
	UIModeNone     = "none"
)

// UIValidation configures the S3 gate.
type UIValidation struct {
	Mode string `yaml:"mode"`
	Task string `yaml:"task"`
}

// Play rollout values (spec §13).
const (
	RolloutDraft     = "draft"
	RolloutCompleted = "completed"
)

// Listing update policies (spec §7.3).
const (
	ListingNever = "never"
	ListingMinor = "minor"
	ListingAny   = "any"
)

// Play configures the closed-testing publish.
type Play struct {
	Track           string `yaml:"track"`
	Rollout         string `yaml:"rollout"`
	UpdateListingOn string `yaml:"update_listing_on"`
	ListingFile     string `yaml:"listing_file"`
}

// Artifacts locates the release folder tree and the screenshots to upload.
type Artifacts struct {
	Root            string `yaml:"root"`
	ScreenshotsFrom string `yaml:"screenshots_from"`
}

// Notes provider names (spec §7.1).
const (
	NotesSourceFile    = "file"
	NotesSourceCommits = "commits"
)

// Behaviour when no notes provider yields (spec §4).
const (
	OnExhaustedHalt = "halt"
	OnExhaustedSkip = "skip"
)

// Notes configures the customer-facing release-notes provider chain.
type Notes struct {
	Source         []string `yaml:"source"`
	FilePath       string   `yaml:"file_path"`
	CommitTemplate string   `yaml:"commit_template"`
	OnExhausted    string   `yaml:"on_exhausted"`
}

// Notify configures the optional webhook ping (spec §10).
type Notify struct {
	URL string `yaml:"url"`
}

// Config is the whole of autoship.yaml.
type Config struct {
	Repo         Repo          `yaml:"repo"`
	App          App           `yaml:"app"`
	Gradle       Gradle        `yaml:"gradle"`
	UIValidation UIValidation  `yaml:"ui_validation"`
	Play         Play          `yaml:"play"`
	Artifacts    Artifacts     `yaml:"artifacts"`
	Notes        Notes         `yaml:"notes"`
	Notify       Notify        `yaml:"notify"`
	MaxRunDur    time.Duration `yaml:"max_run_duration"`
}

// DefaultMaxRunDuration bounds how long a lock is trusted before it is treated
// as stale (spec §5).
const DefaultMaxRunDuration = 45 * time.Minute

// Load reads, env-expands, defaults and validates a config file.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.expand()
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &cfg, nil
}

// expand substitutes ${VAR} / $VAR from the environment in path-shaped fields.
func (c *Config) expand() {
	c.Repo.Path = os.ExpandEnv(c.Repo.Path)
	c.App.GradleFile = os.ExpandEnv(c.App.GradleFile)
	c.Artifacts.Root = os.ExpandEnv(c.Artifacts.Root)
	c.Artifacts.ScreenshotsFrom = os.ExpandEnv(c.Artifacts.ScreenshotsFrom)
	c.Notes.CommitTemplate = os.ExpandEnv(c.Notes.CommitTemplate)
	c.Play.ListingFile = os.ExpandEnv(c.Play.ListingFile)
	// Notes.FilePath keeps ${version}, which is substituted per release, so only
	// expand the parts the environment actually owns.
	c.Notes.FilePath = expandExcept(c.Notes.FilePath, "version")
}

// expandExcept expands environment variables but leaves the named placeholders
// (written as ${name}) untouched.
func expandExcept(s string, keep ...string) string {
	return os.Expand(s, func(name string) string {
		for _, k := range keep {
			if name == k {
				return "${" + name + "}"
			}
		}
		return os.Getenv(name)
	})
}

func (c *Config) applyDefaults() {
	if c.Repo.Branch == "" {
		c.Repo.Branch = "main"
	}
	if c.Repo.Remote == "" {
		c.Repo.Remote = "origin"
	}
	if c.UIValidation.Mode == "" {
		c.UIValidation.Mode = UIModeNone
	}
	if c.Play.Rollout == "" {
		c.Play.Rollout = RolloutDraft
	}
	if c.Play.UpdateListingOn == "" {
		c.Play.UpdateListingOn = ListingMinor
	}
	if c.Play.ListingFile == "" {
		c.Play.ListingFile = "docs/release/play_store_listing.md"
	}
	if len(c.Notes.Source) == 0 {
		c.Notes.Source = []string{NotesSourceFile}
	}
	if c.Notes.OnExhausted == "" {
		c.Notes.OnExhausted = OnExhaustedHalt
	}
	if c.MaxRunDur == 0 {
		c.MaxRunDur = DefaultMaxRunDuration
	}
}

// Validate reports the first required-but-empty or out-of-range field, named so
// the message points at the line to fix.
func (c *Config) Validate() error {
	required := []struct {
		field string
		value string
	}{
		{"repo.path", c.Repo.Path},
		{"app.module", c.App.Module},
		{"app.package", c.App.Package},
		{"app.gradle_file", c.App.GradleFile},
		{"gradle.unit_tests", c.Gradle.UnitTests},
		{"gradle.bundle", c.Gradle.Bundle},
		{"play.track", c.Play.Track},
		{"artifacts.root", c.Artifacts.Root},
	}
	for _, r := range required {
		if strings.TrimSpace(r.value) == "" {
			return fmt.Errorf("%s is required", r.field)
		}
	}
	if err := oneOf("ui_validation.mode", c.UIValidation.Mode, UIModeJVM, UIModeEmulator, UIModeNone); err != nil {
		return err
	}
	if err := oneOf("play.rollout", c.Play.Rollout, RolloutDraft, RolloutCompleted); err != nil {
		return err
	}
	if err := oneOf("play.update_listing_on", c.Play.UpdateListingOn, ListingNever, ListingMinor, ListingAny); err != nil {
		return err
	}
	if err := oneOf("notes.on_exhausted", c.Notes.OnExhausted, OnExhaustedHalt, OnExhaustedSkip); err != nil {
		return err
	}
	for _, s := range c.Notes.Source {
		if err := oneOf("notes.source", s, NotesSourceFile, NotesSourceCommits); err != nil {
			return err
		}
	}
	if c.UIValidation.Mode == UIModeJVM && strings.TrimSpace(c.UIValidation.Task) == "" {
		return fmt.Errorf("ui_validation.task is required when ui_validation.mode is %q", UIModeJVM)
	}
	return nil
}

func oneOf(field, value string, allowed ...string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("%s: %q is not one of %s", field, value, strings.Join(allowed, ", "))
}
