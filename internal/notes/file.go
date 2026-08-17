package notes

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/vipinm/autoship/internal/release"
)

// FileProvider reads notes a human wrote and committed. Writing that file is
// the developer's explicit "this version is ready to ship" signal, which the
// pipeline otherwise lacks entirely (spec §7.1).
type FileProvider struct {
	// RepoPath is the repository root the template is relative to.
	RepoPath string
	// PathTemplate is a path containing ${version}, e.g.
	// docs/release/notes/${version}.txt
	PathTemplate string
}

// Path returns the file this provider would read for rel.
func (p FileProvider) Path(rel release.Release) string {
	sub := strings.ReplaceAll(p.PathTemplate, "${version}", rel.Name)
	sub = filepath.FromSlash(sub)
	if filepath.IsAbs(sub) || p.RepoPath == "" {
		return sub
	}
	return filepath.Join(p.RepoPath, sub)
}

// Notes returns the contents of the versioned notes file.
func (p FileProvider) Notes(_ context.Context, rel release.Release) (string, error) {
	if strings.TrimSpace(p.PathTemplate) == "" {
		return "", fmt.Errorf("%w: notes.file_path is not configured", ErrNoNotes)
	}
	path := p.Path(rel)
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: %s does not exist", ErrNoNotes, path)
	}
	if err != nil {
		return "", fmt.Errorf("read release notes: %w", err)
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", fmt.Errorf("%w: %s is empty", ErrNoNotes, path)
	}
	return text, nil
}
