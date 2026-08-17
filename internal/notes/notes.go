// Package notes produces the two sets of release notes: the customer-facing
// copy Play shows testers, and the technical notes kept with the artefacts.
//
// Customer copy is a writing task, so the default policy is that a human wrote
// it and the pipeline only collects it. That policy is expressed as a provider
// chain rather than an `if fileMissing { halt }`, so changing it is a config
// edit rather than a code change (spec §7.1).
package notes

import (
	"context"
	"errors"

	"github.com/vipinm/autoship/internal/release"
)

// ErrNoNotes reports that a provider has nothing to offer for this release.
// It is not a failure: the chain decides what an exhausted chain means.
var ErrNoNotes = errors.New("no release notes from this provider")

// NotesProvider yields customer-facing copy for a release.
type NotesProvider interface {
	// Notes returns the customer-facing copy for a release, or ErrNoNotes
	// if this provider has nothing to offer for it.
	Notes(ctx context.Context, rel release.Release) (string, error)
}

// ProviderFunc adapts a function to NotesProvider.
type ProviderFunc func(ctx context.Context, rel release.Release) (string, error)

// Notes implements NotesProvider.
func (f ProviderFunc) Notes(ctx context.Context, rel release.Release) (string, error) {
	return f(ctx, rel)
}
