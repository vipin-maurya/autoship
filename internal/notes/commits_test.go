package notes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixedSubjects(subjects ...string) func(string, string) ([]string, error) {
	return func(string, string) ([]string, error) { return subjects, nil }
}

func TestCommitsProvider_RendersSubjects(t *testing.T) {
	p := CommitsProvider{
		RepoPath: `C:\repo`,
		Subjects: fixedSubjects(
			"feat: add settings screen",
			"fix: filter persistence",
			"chore: bump deps",
		),
	}
	got, err := p.Notes(context.Background(), rel105)
	if err != nil {
		t.Fatalf("Notes: %v", err)
	}
	if !strings.Contains(got, "• add settings screen") {
		t.Errorf("Notes = %q, want a bullet for the feat", got)
	}
	if !strings.Contains(got, "• filter persistence") {
		t.Errorf("Notes = %q, want a bullet for the fix", got)
	}
	if strings.Contains(got, "bump deps") {
		t.Errorf("Notes = %q, want the chore commit omitted", got)
	}
	if !strings.Contains(got, "1.0.5") {
		t.Errorf("Notes = %q, want it to name the version", got)
	}
}

func TestCommitsProvider_NoCommits(t *testing.T) {
	p := CommitsProvider{Subjects: fixedSubjects()}
	if _, err := p.Notes(context.Background(), rel105); !errors.Is(err, ErrNoNotes) {
		t.Fatalf("Notes = %v, want ErrNoNotes", err)
	}
}

func TestCommitsProvider_OnlyHousekeepingIsErrNoNotes(t *testing.T) {
	p := CommitsProvider{Subjects: fixedSubjects("chore: bump deps", "ci: cache gradle", "docs: fix typo")}
	if _, err := p.Notes(context.Background(), rel105); !errors.Is(err, ErrNoNotes) {
		t.Fatalf("Notes = %v, want ErrNoNotes", err)
	}
}

func TestCommitsProvider_CustomTemplate(t *testing.T) {
	root := t.TempDir()
	tmpl := filepath.Join(root, "templates", "notes-from-commits.tmpl")
	if err := os.MkdirAll(filepath.Dir(tmpl), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmpl, []byte("v{{.Version}} ({{.Code}}){{range .Items}} / {{.}}{{end}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := CommitsProvider{
		RepoPath:     root,
		TemplatePath: "templates/notes-from-commits.tmpl",
		Subjects:     fixedSubjects("feat: add settings screen"),
	}
	got, err := p.Notes(context.Background(), rel105)
	if err != nil {
		t.Fatalf("Notes: %v", err)
	}
	if got != "v1.0.5 (8) / add settings screen" {
		t.Errorf("Notes = %q", got)
	}
}

func TestCommitsProvider_MissingTemplateIsARealError(t *testing.T) {
	p := CommitsProvider{
		RepoPath:     t.TempDir(),
		TemplatePath: "templates/gone.tmpl",
		Subjects:     fixedSubjects("feat: add settings screen"),
	}
	_, err := p.Notes(context.Background(), rel105)
	if err == nil {
		t.Fatal("Notes = nil error, want one")
	}
	// A broken template is a configuration failure, not "nothing to say".
	if errors.Is(err, ErrNoNotes) {
		t.Errorf("error = %v, want it not to masquerade as ErrNoNotes", err)
	}
}

func TestHighlights(t *testing.T) {
	got := Highlights([]string{
		"feat(ui)!: add settings screen",
		"fix: filter persistence",
		"chore(deps): bump agp",
		"refactor: extract dao",
		"tidy up the readme",
		"fix: filter persistence",
	})
	want := []string{"add settings screen", "filter persistence", "tidy up the readme"}
	if len(got) != len(want) {
		t.Fatalf("Highlights = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Highlights[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
