package notes

import (
	"context"
	"errors"
	"fmt"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/release"
)

// Deps are what providers need from the rest of the system.
type Deps struct {
	RepoPath string
}

// Chain tries each provider in order and returns the first that yields notes.
type Chain struct {
	Providers []NotesProvider
	Names     []string
}

// Notes returns the first provider's output, or ErrNoNotes once the chain is
// exhausted.
func (c Chain) Notes(ctx context.Context, rel release.Release) (string, error) {
	var reasons []error
	for _, p := range c.Providers {
		text, err := p.Notes(ctx, rel)
		if err == nil {
			return text, nil
		}
		if errors.Is(err, ErrNoNotes) {
			reasons = append(reasons, err)
			continue
		}
		// A real failure (unreadable file, broken template) is not the same as
		// "this provider has nothing", and must not be silently fallen through.
		return "", err
	}
	return "", fmt.Errorf("%w: chain %v exhausted: %w", ErrNoNotes, c.Names, errors.Join(reasons...))
}

// BuildChain resolves the configured source list into providers. Adding a
// strategy later means writing one type and registering it here, not
// restructuring S4 (spec §7.1).
func BuildChain(cfg config.Notes, deps Deps) (Chain, error) {
	if len(cfg.Source) == 0 {
		return Chain{}, errors.New("notes.source is empty")
	}
	var chain Chain
	for _, name := range cfg.Source {
		switch name {
		case config.NotesSourceFile:
			chain.Providers = append(chain.Providers, FileProvider{
				RepoPath:     deps.RepoPath,
				PathTemplate: cfg.FilePath,
			})
		case config.NotesSourceCommits:
			chain.Providers = append(chain.Providers, CommitsProvider{
				RepoPath:     deps.RepoPath,
				TemplatePath: cfg.CommitTemplate,
			})
		default:
			return Chain{}, fmt.Errorf("notes.source: unknown provider %q", name)
		}
		chain.Names = append(chain.Names, name)
	}
	return chain, nil
}
