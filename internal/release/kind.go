// Package release models the version being shipped and how it differs from the
// last one published.
package release

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Kind is the semver delta between the last published version and this one.
// It decides whether the store listing and screenshots are uploaded (spec §7.3).
type Kind int

const (
	// Patch is 1.0.4 → 1.0.5: bundle and customer notes only.
	Patch Kind = iota
	// Minor is 1.0.5 → 1.1.0.
	Minor
	// Major is 1.1.0 → 2.0.0, and also the first ever release.
	Major
)

func (k Kind) String() string {
	switch k {
	case Patch:
		return "patch"
	case Minor:
		return "minor"
	case Major:
		return "major"
	default:
		return "unknown"
	}
}

// Errors returned by Classify. Both are halt conditions: the R6 contract says
// the repo is the source of truth for versions, so only a human can decide
// what a non-advancing version means (spec §10).
var (
	ErrNoVersionBump        = errors.New("version has not changed since the last published release")
	ErrVersionWentBackwards = errors.New("version is lower than the last published release")
)

// Release is the version being shipped.
type Release struct {
	Name         string // e.g. "1.0.5", never carrying -SNAPSHOT
	Code         int    // versionCode from the Gradle file
	Kind         Kind
	PreviousName string
	PreviousCode int
	SHA          string
}

func (r Release) String() string {
	return fmt.Sprintf("%s (%d, %s)", r.Name, r.Code, r.Kind)
}

// Tag is the git tag name for this release.
func (r Release) Tag() string { return "v" + r.Name }

type semver struct{ major, minor, patch int }

func parse(s string) (semver, error) {
	parts := strings.SplitN(strings.TrimSpace(s), ".", 3)
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("version %q is not major.minor.patch", s)
	}
	var v semver
	for i, dst := range []*int{&v.major, &v.minor, &v.patch} {
		// Tolerate a trailing pre-release suffix on the patch component.
		field := parts[i]
		if i == 2 {
			if cut := strings.IndexAny(field, "-+"); cut >= 0 {
				field = field[:cut]
			}
		}
		n, err := strconv.Atoi(field)
		if err != nil {
			return semver{}, fmt.Errorf("version %q: component %q is not a number", s, parts[i])
		}
		if n < 0 {
			return semver{}, fmt.Errorf("version %q: negative component", s)
		}
		*dst = n
	}
	return v, nil
}

// Classify reports how next differs from prev. An empty prev means nothing has
// been published yet, which counts as a major release.
func Classify(prev, next string) (Kind, error) {
	n, err := parse(next)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(prev) == "" {
		return Major, nil
	}
	p, err := parse(prev)
	if err != nil {
		return 0, err
	}
	switch {
	case n.major > p.major:
		return Major, nil
	case n.major < p.major:
		return 0, fmt.Errorf("%w: %s < %s", ErrVersionWentBackwards, next, prev)
	case n.minor > p.minor:
		return Minor, nil
	case n.minor < p.minor:
		return 0, fmt.Errorf("%w: %s < %s", ErrVersionWentBackwards, next, prev)
	case n.patch > p.patch:
		return Patch, nil
	case n.patch < p.patch:
		return 0, fmt.Errorf("%w: %s < %s", ErrVersionWentBackwards, next, prev)
	default:
		return 0, fmt.Errorf("%w: %s", ErrNoVersionBump, next)
	}
}

// NextPatch returns the version that follows name in patch order, which is what
// the post-release bump writes back as the next -SNAPSHOT.
func NextPatch(name string) (string, error) {
	v, err := parse(name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch+1), nil
}
