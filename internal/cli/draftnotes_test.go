package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (f *fixture) draftNotes(extra ...string) (int, string, string) {
	f.t.Helper()
	var out, errOut bytes.Buffer
	args := append([]string{"--config", f.ConfigPath}, extra...)
	code := draftNotesCmd(args, &out, &errOut, f.deps())
	return code, out.String(), errOut.String()
}

func TestDraftNotes_PrintsADraft(t *testing.T) {
	fx := newFixture(t)
	writeFile(t, filepath.Join(fx.Clone, "app", "src", "settings.kt"), "// settings\n")
	git(t, fx.Clone, "add", ".")
	git(t, fx.Clone, "commit", "-m", "feat: add settings screen")

	code, stdout, stderr := fx.draftNotes()
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, "add settings screen") {
		t.Errorf("draft = %q, want the commit subject", stdout)
	}
	if !strings.Contains(stdout, "1.0.5") {
		t.Errorf("draft = %q, want the version under development", stdout)
	}
}

func TestDraftNotes_WritesToTheConfiguredPath(t *testing.T) {
	fx := newFixture(t)
	// The notes file exists in the fixture, so --write needs --force.
	git(t, fx.Clone, "commit", "--allow-empty", "-m", "feat: add settings screen")

	target := filepath.Join(fx.Clone, "docs", "release", "notes", "1.0.5.txt")
	if code, _, stderr := fx.draftNotes("--write"); code == 0 {
		t.Fatalf("--write over an existing file should fail, got 0 (stderr: %s)", stderr)
	}
	code, stdout, stderr := fx.draftNotes("--write", "--force")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stdout, target) {
		t.Errorf("output = %q, want it to name %q", stdout, target)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "add settings screen") {
		t.Errorf("written draft = %q", raw)
	}
}

func TestDraftNotes_NothingToDraft(t *testing.T) {
	fx := newFixture(t)
	// Only housekeeping since the start of history.
	git(t, fx.Clone, "commit", "--allow-empty", "-m", "chore: bump deps")
	git(t, fx.Clone, "tag", "-a", "v1.0.4", "-m", "release 1.0.4")
	git(t, fx.Clone, "commit", "--allow-empty", "-m", "ci: cache gradle")

	code, _, stderr := fx.draftNotes()
	if code == 0 {
		t.Fatal("exit code = 0, want a failure when there is nothing to draft")
	}
	if !strings.Contains(stderr, "nothing to draft") {
		t.Errorf("stderr = %q", stderr)
	}
}
