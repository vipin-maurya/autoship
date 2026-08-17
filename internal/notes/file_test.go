package notes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vipinm/autoship/internal/release"
)

const notesTemplate = "docs/release/notes/${version}.txt"

var rel105 = release.Release{Name: "1.0.5", Code: 8, Kind: release.Patch}

// repoWithNotes creates a repo root, optionally containing a notes file.
func repoWithNotes(t *testing.T, version, content string) string {
	t.Helper()
	root := t.TempDir()
	if version == "" {
		return root
	}
	path := filepath.Join(root, filepath.FromSlash("docs/release/notes"), version+".txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestFileProvider_ReadsVersionedFile(t *testing.T) {
	root := repoWithNotes(t, "1.0.5", "Faster search and fewer duplicate merchants.\n")
	p := FileProvider{RepoPath: root, PathTemplate: notesTemplate}

	got, err := p.Notes(context.Background(), rel105)
	if err != nil {
		t.Fatalf("Notes: %v", err)
	}
	if got != "Faster search and fewer duplicate merchants." {
		t.Errorf("Notes = %q", got)
	}
}

func TestFileProvider_MissingIsErrNoNotes(t *testing.T) {
	root := repoWithNotes(t, "", "")
	p := FileProvider{RepoPath: root, PathTemplate: notesTemplate}

	_, err := p.Notes(context.Background(), rel105)
	if !errors.Is(err, ErrNoNotes) {
		t.Fatalf("Notes = %v, want ErrNoNotes", err)
	}
}

func TestFileProvider_EmptyFileIsErrNoNotes(t *testing.T) {
	root := repoWithNotes(t, "1.0.5", "   \n")
	p := FileProvider{RepoPath: root, PathTemplate: notesTemplate}

	if _, err := p.Notes(context.Background(), rel105); !errors.Is(err, ErrNoNotes) {
		t.Fatalf("Notes = %v, want ErrNoNotes", err)
	}
}

func TestFileProvider_PathSubstitutesVersion(t *testing.T) {
	p := FileProvider{RepoPath: `C:\repo`, PathTemplate: notesTemplate}
	want := filepath.Join(`C:\repo`, filepath.FromSlash("docs/release/notes/1.0.5.txt"))
	if got := p.Path(rel105); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}
