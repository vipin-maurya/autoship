package gitrepo

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Fixture builds an origin repo with one commit plus a clone of it.
type Fixture struct {
	Origin string
	Clone  string
}

// NewFixture creates origin+clone under t.TempDir(). It skips the test when
// git is not installed, since every test in this package drives the real one.
func NewFixture(t *testing.T) Fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	origin := filepath.Join(root, "origin")
	clone := filepath.Join(root, "clone")
	if err := os.MkdirAll(origin, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, origin, "init", "--initial-branch=main")
	configure(t, origin)
	WriteFile(t, origin, "README.md", "hello\n")
	mustGit(t, origin, "add", ".")
	mustGit(t, origin, "commit", "-m", "feat: initial commit")

	mustGit(t, root, "clone", origin, clone)
	configure(t, clone)
	return Fixture{Origin: origin, Clone: clone}
}

func configure(t *testing.T, dir string) {
	t.Helper()
	mustGit(t, dir, "config", "user.email", "autoship@example.test")
	mustGit(t, dir, "config", "user.name", "autoship test")
	mustGit(t, dir, "config", "commit.gpgsign", "false")
	// Pushing to the checked-out branch of a non-bare origin is exactly what
	// the post-release stage does against a workstation clone.
	mustGit(t, dir, "config", "receive.denyCurrentBranch", "updateInstead")
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := run(dir, args...)
	if err != nil {
		t.Fatalf("git %s in %s: %v", strings.Join(args, " "), dir, err)
	}
	return out
}

// WriteFile writes a file inside a repo and is shared by tests in this package.
func WriteFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveRemoteHead(t *testing.T) {
	fx := NewFixture(t)
	want := mustGit(t, fx.Origin, "rev-parse", "HEAD")

	got, err := ResolveRemoteHead(fx.Clone, "origin", "main")
	if err != nil {
		t.Fatalf("ResolveRemoteHead: %v", err)
	}
	if got != want {
		t.Errorf("ResolveRemoteHead = %q, want %q", got, want)
	}
}

func TestResolveRemoteHead_SeesNewCommits(t *testing.T) {
	fx := NewFixture(t)
	before, err := ResolveRemoteHead(fx.Clone, "origin", "main")
	if err != nil {
		t.Fatal(err)
	}

	WriteFile(t, fx.Origin, "NEW.md", "new\n")
	mustGit(t, fx.Origin, "add", ".")
	mustGit(t, fx.Origin, "commit", "-m", "feat: second commit")

	after, err := ResolveRemoteHead(fx.Clone, "origin", "main")
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Errorf("head did not move after a new origin commit: %q", after)
	}
}

func TestResolveRemoteHead_UnreachableRemoteIsTransient(t *testing.T) {
	fx := NewFixture(t)
	missing := filepath.Join(t.TempDir(), "gone")
	mustGit(t, fx.Clone, "remote", "set-url", "origin", missing)

	_, err := ResolveRemoteHead(fx.Clone, "origin", "main")
	if err == nil {
		t.Fatal("ResolveRemoteHead against a missing remote = nil error, want ErrTransient")
	}
	if !errors.Is(err, ErrTransient) {
		t.Errorf("error = %v, want it to wrap ErrTransient", err)
	}
}

func TestResolveRemoteHead_UnknownBranchIsNotTransient(t *testing.T) {
	fx := NewFixture(t)
	_, err := ResolveRemoteHead(fx.Clone, "origin", "no-such-branch")
	if err == nil {
		t.Fatal("want an error for an unknown branch")
	}
	// A branch that does not exist will not appear on the next tick, so this
	// must not be swallowed as transient — that would make every tick a silent
	// no-op forever.
	if errors.Is(err, ErrTransient) {
		t.Errorf("unknown branch reported as transient: %v", err)
	}
	if !strings.Contains(err.Error(), "no-such-branch") {
		t.Errorf("error = %v, want it to name the missing branch", err)
	}
}

func TestLastTagAndSubjects(t *testing.T) {
	fx := NewFixture(t)
	if tag, err := LastTag(fx.Clone); err != nil || tag != "" {
		t.Fatalf("LastTag on an untagged repo = (%q, %v), want (\"\", nil)", tag, err)
	}
	if err := Tag(fx.Clone, "v1.0.4", "release 1.0.4"); err != nil {
		t.Fatal(err)
	}
	WriteFile(t, fx.Clone, "a.txt", "a\n")
	mustGit(t, fx.Clone, "add", ".")
	mustGit(t, fx.Clone, "commit", "-m", "fix: filter persistence")

	tag, err := LastTag(fx.Clone)
	if err != nil || tag != "v1.0.4" {
		t.Fatalf("LastTag = (%q, %v), want v1.0.4", tag, err)
	}
	subs, err := Subjects(fx.Clone, "v1.0.4..HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 1 || subs[0] != "fix: filter persistence" {
		t.Errorf("Subjects = %v, want [fix: filter persistence]", subs)
	}
}

func TestPush_RejectedWhenRemoteMoved(t *testing.T) {
	fx := NewFixture(t)
	// Move origin ahead of the clone.
	WriteFile(t, fx.Origin, "b.txt", "b\n")
	mustGit(t, fx.Origin, "add", ".")
	mustGit(t, fx.Origin, "commit", "-m", "chore: origin moves")

	WriteFile(t, fx.Clone, "c.txt", "c\n")
	mustGit(t, fx.Clone, "add", ".")
	mustGit(t, fx.Clone, "commit", "-m", "chore: clone moves")

	err := Push(fx.Clone, "origin", "main", true)
	if !errors.Is(err, ErrPushRejected) {
		t.Fatalf("Push = %v, want ErrPushRejected", err)
	}

	if err := PullRebase(fx.Clone, "origin", "main"); err != nil {
		t.Fatalf("PullRebase: %v", err)
	}
	if err := Push(fx.Clone, "origin", "main", true); err != nil {
		t.Fatalf("Push after rebase: %v", err)
	}
}
