package pipeline

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/release"
)

func quietLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io_Discard{}, nil))
}

type io_Discard struct{}

func (io_Discard) Write(b []byte) (int, error) { return len(b), nil }

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v: %s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func configureRepo(t *testing.T, dir string) {
	t.Helper()
	git(t, dir, "config", "user.email", "autoship@example.test")
	git(t, dir, "config", "user.name", "autoship test")
	git(t, dir, "config", "commit.gpgsign", "false")
	git(t, dir, "config", "receive.denyCurrentBranch", "updateInstead")
}

// releaseFixture builds an origin plus a clone whose app/build.gradle.kts
// declares the version under release, and a config pointing at the clone.
func releaseFixture(t *testing.T) (cfg *config.Config, origin, clone string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	origin = filepath.Join(root, "origin")
	clone = filepath.Join(root, "clone")
	if err := os.MkdirAll(filepath.Join(origin, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	gradle := "android {\n    defaultConfig {\n        versionCode = 8\n        versionName = \"1.0.5-SNAPSHOT\"\n    }\n}\n"
	if err := os.WriteFile(filepath.Join(origin, "app", "build.gradle.kts"), []byte(gradle), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "--initial-branch=main", origin)
	configureRepo(t, origin)
	git(t, origin, "add", ".")
	git(t, origin, "commit", "-m", "feat: initial")

	git(t, root, "clone", origin, clone)
	configureRepo(t, clone)

	cfg = &config.Config{}
	cfg.Repo.Path = clone
	cfg.Repo.Branch = "main"
	cfg.Repo.Remote = "origin"
	cfg.App.Module = ":app"
	cfg.App.GradleFile = "app/build.gradle.kts"
	return cfg, origin, clone
}

var rel105 = release.Release{Name: "1.0.5", Code: 8, Kind: release.Patch, PreviousName: "1.0.4", PreviousCode: 7}

func TestPostRelease(t *testing.T) {
	cfg, origin, clone := releaseFixture(t)

	res, err := PostRelease(cfg, rel105, false, quietLog())
	if err != nil {
		t.Fatalf("PostRelease: %v", err)
	}
	if !res.Pushed {
		t.Error("Pushed = false, want true")
	}
	if res.Tag != "v1.0.5" || res.NextName != "1.0.6-SNAPSHOT" || res.NextCode != 9 {
		t.Errorf("result = %+v, want tag v1.0.5 bumping to 1.0.6-SNAPSHOT (9)", res)
	}

	// The tag and the bump commit both reached origin.
	if tags := git(t, origin, "tag", "--list"); !strings.Contains(tags, "v1.0.5") {
		t.Errorf("origin tags = %q, want v1.0.5", tags)
	}
	if msg := git(t, origin, "log", "-1", "--pretty=%s"); !strings.Contains(msg, "1.0.6") {
		t.Errorf("origin head message = %q, want the chore bump", msg)
	}
	raw, err := os.ReadFile(filepath.Join(clone, "app", "build.gradle.kts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `versionName = "1.0.6-SNAPSHOT"`) || !strings.Contains(string(raw), "versionCode = 9") {
		t.Errorf("gradle file after bump:\n%s", raw)
	}
}

func TestPostRelease_DryRunTouchesNothing(t *testing.T) {
	cfg, origin, clone := releaseFixture(t)
	before := git(t, clone, "rev-parse", "HEAD")

	res, err := PostRelease(cfg, rel105, true, quietLog())
	if err != nil {
		t.Fatalf("PostRelease: %v", err)
	}
	if res.Pushed {
		t.Error("dry run reported a push")
	}
	if res.NextName != "1.0.6-SNAPSHOT" {
		t.Errorf("NextName = %q, want the version it would have written", res.NextName)
	}
	if after := git(t, clone, "rev-parse", "HEAD"); after != before {
		t.Error("dry run created a commit")
	}
	if tags := git(t, origin, "tag", "--list"); strings.TrimSpace(tags) != "" {
		t.Errorf("dry run pushed tags: %q", tags)
	}
}

func TestPostRelease_RebasesOnRejectedPush(t *testing.T) {
	cfg, origin, _ := releaseFixture(t)

	// A second clone moves main while the release is in flight.
	other := filepath.Join(t.TempDir(), "other")
	git(t, filepath.Dir(other), "clone", origin, other)
	configureRepo(t, other)
	if err := os.WriteFile(filepath.Join(other, "NOTES.md"), []byte("moved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, other, "add", ".")
	git(t, other, "commit", "-m", "docs: main moves during the release")
	git(t, other, "push", "origin", "main")

	res, err := PostRelease(cfg, rel105, false, quietLog())
	if err != nil {
		t.Fatalf("PostRelease: %v", err)
	}
	if !res.Rebased {
		t.Error("Rebased = false, want the rejected push to have been rebased")
	}
	if !res.Pushed {
		t.Error("Pushed = false, want the retry to have succeeded")
	}
	if msg := git(t, origin, "log", "-1", "--pretty=%s"); !strings.Contains(msg, "1.0.6") {
		t.Errorf("origin head = %q, want the bump commit on top", msg)
	}
}

func TestPostRelease_HaltsWithBothIdentifiers(t *testing.T) {
	cfg, origin, clone := releaseFixture(t)

	// Origin moves with a conflicting change to the same file, so the rebase
	// cannot resolve itself.
	other := filepath.Join(t.TempDir(), "other")
	git(t, filepath.Dir(other), "clone", origin, other)
	configureRepo(t, other)
	conflicting := "android {\n    defaultConfig {\n        versionCode = 42\n        versionName = \"9.9.9-SNAPSHOT\"\n    }\n}\n"
	if err := os.WriteFile(filepath.Join(other, "app", "build.gradle.kts"), []byte(conflicting), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, other, "add", ".")
	git(t, other, "commit", "-m", "chore: conflicting version change")
	git(t, other, "push", "origin", "main")

	_, err := PostRelease(cfg, rel105, false, quietLog())
	if err == nil {
		t.Fatal("PostRelease = nil error, want a halt")
	}
	var he *HaltError
	if !errors.As(err, &he) {
		t.Fatalf("error = %T (%v), want *HaltError", err, err)
	}
	if he.Stage != StagePostRelease {
		t.Errorf("Stage = %q, want %q", he.Stage, StagePostRelease)
	}
	// Both identifiers: what is already live, and what has not been pushed.
	if !strings.Contains(he.Reason, "8") {
		t.Errorf("Reason = %q, want the published versionCode", he.Reason)
	}
	localSHA := git(t, clone, "rev-parse", "HEAD")
	if !strings.Contains(he.Reason, localSHA[:12]) {
		t.Errorf("Reason = %q, want the unpushed commit %s", he.Reason, localSHA[:12])
	}
}
