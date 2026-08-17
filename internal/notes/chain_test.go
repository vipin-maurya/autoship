package notes

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/release"
)

// gitRepoWithHistory builds a repo whose log carries one shippable commit, so
// the commits provider has something real to render.
func gitRepoWithHistory(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	steps := [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "autoship@example.test"},
		{"config", "user.name", "autoship test"},
		{"config", "commit.gpgsign", "false"},
		{"commit", "--allow-empty", "-m", "feat: add settings screen"},
		{"commit", "--allow-empty", "-m", "chore: bump deps"},
	}
	for _, args := range steps {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	return dir
}

// snapshot records every file under dir, so a test can prove the pipeline
// wrote nothing it should not have.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[path] = string(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestChain_FileOnlyExhausts(t *testing.T) {
	root := repoWithNotes(t, "", "")
	cfg := config.Notes{Source: []string{config.NotesSourceFile}, FilePath: notesTemplate}

	chain, err := BuildChain(cfg, Deps{RepoPath: root})
	if err != nil {
		t.Fatalf("BuildChain: %v", err)
	}
	if _, err := chain.Notes(context.Background(), rel105); !errors.Is(err, ErrNoNotes) {
		t.Fatalf("Notes = %v, want ErrNoNotes", err)
	}
}

func TestChain_FileWins(t *testing.T) {
	root := repoWithNotes(t, "1.0.5", "Human-written copy.")
	cfg := config.Notes{
		Source:   []string{config.NotesSourceFile, config.NotesSourceCommits},
		FilePath: notesTemplate,
	}
	chain, err := BuildChain(cfg, Deps{RepoPath: root})
	if err != nil {
		t.Fatalf("BuildChain: %v", err)
	}
	got, err := chain.Notes(context.Background(), rel105)
	if err != nil {
		t.Fatalf("Notes: %v", err)
	}
	if got != "Human-written copy." {
		t.Errorf("Notes = %q, want the file provider to win", got)
	}
}

// TestChain_FallsThroughToCommits is the executable proof that the notes
// policy is swappable by config alone (spec §7.1): the same code path that
// halts on a missing file instead generates copy when the config says so.
func TestChain_FallsThroughToCommits(t *testing.T) {
	root := gitRepoWithHistory(t)
	before := snapshot(t, root)

	cfg := config.Notes{
		Source:   []string{config.NotesSourceFile, config.NotesSourceCommits},
		FilePath: notesTemplate,
	}
	chain, err := BuildChain(cfg, Deps{RepoPath: root})
	if err != nil {
		t.Fatalf("BuildChain: %v", err)
	}
	got, err := chain.Notes(context.Background(), rel105)
	if err != nil {
		t.Fatalf("Notes: %v", err)
	}
	if !strings.Contains(got, "add settings screen") {
		t.Errorf("Notes = %q, want commit-derived copy", got)
	}
	if strings.Contains(got, "bump deps") {
		t.Errorf("Notes = %q, want housekeeping commits omitted", got)
	}

	after := snapshot(t, root)
	if len(after) != len(before) {
		t.Errorf("the repo gained or lost files: %d -> %d", len(before), len(after))
	}
	for path, content := range before {
		if after[path] != content {
			t.Errorf("file %q was modified while generating notes", path)
		}
	}
}

func TestChain_UnknownProvider(t *testing.T) {
	_, err := BuildChain(config.Notes{Source: []string{"tarot"}}, Deps{})
	if err == nil {
		t.Fatal("BuildChain = nil error, want one")
	}
	if !strings.Contains(err.Error(), "tarot") {
		t.Errorf("error = %v, want it to name the bad value", err)
	}
}

func TestChain_EmptySource(t *testing.T) {
	if _, err := BuildChain(config.Notes{}, Deps{}); err == nil {
		t.Fatal("BuildChain with no sources = nil error, want one")
	}
}

func TestChain_RealFailureStopsTheChain(t *testing.T) {
	boom := errors.New("disk on fire")
	chain := Chain{
		Providers: []NotesProvider{
			ProviderFunc(func(context.Context, release.Release) (string, error) { return "", boom }),
		},
		Names: []string{"broken"},
	}
	_, err := chain.Notes(context.Background(), rel105)
	if !errors.Is(err, boom) {
		t.Fatalf("Notes = %v, want the underlying failure", err)
	}
}
