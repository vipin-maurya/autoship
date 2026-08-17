package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vipinm/autoship/internal/gradlefile"
	"github.com/vipinm/autoship/internal/notes"
	"github.com/vipinm/autoship/internal/release"
)

func init() {
	register("draft-notes", func(args []string, stdout, stderr io.Writer) int {
		return draftNotesCmd(args, stdout, stderr, defaultDeps)
	})
}

// draftNotesCmd generates a first draft of the customer copy from the commit
// log for a human to edit. It is deliberately a separate, human-invoked
// command: generation stays out of the automated path (spec §7.1).
func draftNotesCmd(args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("draft-notes", flag.ContinueOnError)
	write := fs.Bool("write", false, "write the draft to the configured notes path instead of stdout")
	force := fs.Bool("force", false, "with --write, overwrite an existing notes file")

	cfg, _, err := loadConfig(fs, args, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "autoship draft-notes: %v\n", err)
		return exitUsage
	}

	gradlePath := filepath.Join(cfg.Repo.Path, filepath.FromSlash(cfg.App.GradleFile))
	v, err := gradlefile.Parse(gradlePath)
	if err != nil {
		fmt.Fprintf(stderr, "autoship draft-notes: %v\n", err)
		return exitHalt
	}
	rel := release.Release{Name: v.ReleaseName(), Code: v.Code}

	provider := notes.CommitsProvider{
		RepoPath:     cfg.Repo.Path,
		TemplatePath: cfg.Notes.CommitTemplate,
	}
	draft, err := provider.Notes(context.Background(), rel)
	if err != nil {
		if errors.Is(err, notes.ErrNoNotes) {
			fmt.Fprintf(stderr, "autoship draft-notes: nothing to draft: %v\n", err)
			return exitHalt
		}
		fmt.Fprintf(stderr, "autoship draft-notes: %v\n", err)
		return exitHalt
	}
	if err := notes.Validate(draft); err != nil {
		// Still worth showing: the developer is about to edit it anyway.
		fmt.Fprintf(stderr, "warning: %v\n", err)
	}

	if !*write {
		fmt.Fprintln(stdout, draft)
		return exitOK
	}

	target := notes.FileProvider{RepoPath: cfg.Repo.Path, PathTemplate: cfg.Notes.FilePath}.Path(rel)
	if strings.TrimSpace(cfg.Notes.FilePath) == "" {
		fmt.Fprintln(stderr, "autoship draft-notes: notes.file_path is not configured")
		return exitHalt
	}
	if _, err := os.Stat(target); err == nil && !*force {
		fmt.Fprintf(stderr, "autoship draft-notes: %s already exists (use --force to overwrite)\n", target)
		return exitHalt
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fmt.Fprintf(stderr, "autoship draft-notes: %v\n", err)
		return exitHalt
	}
	if err := os.WriteFile(target, []byte(draft+"\n"), 0o644); err != nil {
		fmt.Fprintf(stderr, "autoship draft-notes: %v\n", err)
		return exitHalt
	}
	fmt.Fprintf(stdout, "wrote a draft to %s — edit it before pushing\n", target)
	return exitOK
}
