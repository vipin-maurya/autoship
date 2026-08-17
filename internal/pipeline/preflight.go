package pipeline

import (
	"errors"
	"path/filepath"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/gradlefile"
	"github.com/vipinm/autoship/internal/release"
	"github.com/vipinm/autoship/internal/state"
)

// StagePreflight is the S1 label used in halt records.
const StagePreflight = "S1"

// Preflight reads the version the repo declares and checks it can legally be
// published on top of what was published last. It never invents a version:
// the repo is the source of truth (spec R6).
func Preflight(cfg *config.Config, st state.State) (release.Release, error) {
	gradlePath := filepath.Join(cfg.Repo.Path, filepath.FromSlash(cfg.App.GradleFile))
	v, err := gradlefile.Parse(gradlePath)
	if err != nil {
		return release.Release{}, Halt(StagePreflight, "cannot read the version from "+cfg.App.GradleFile, err)
	}

	name := v.ReleaseName()
	if name == "" {
		return release.Release{}, Haltf(StagePreflight, "versionName in %s is empty", cfg.App.GradleFile)
	}

	// A versionCode that does not advance is rejected by Play itself, and the
	// R6 contract says only a human can decide what the right fix is.
	if v.Code <= st.LastPublishedVersionCode {
		return release.Release{}, Haltf(StagePreflight,
			"versionCode %d is not greater than the last published versionCode %d",
			v.Code, st.LastPublishedVersionCode)
	}

	kind, err := release.Classify(st.LastPublishedVersionName, name)
	if err != nil {
		switch {
		case errors.Is(err, release.ErrNoVersionBump):
			return release.Release{}, Haltf(StagePreflight,
				"versionName %s has already been published; nothing to release", name)
		case errors.Is(err, release.ErrVersionWentBackwards):
			return release.Release{}, Haltf(StagePreflight,
				"versionName %s is lower than the last published %s", name, st.LastPublishedVersionName)
		default:
			return release.Release{}, Halt(StagePreflight, "cannot classify the release", err)
		}
	}

	return release.Release{
		Name:         name,
		Code:         v.Code,
		Kind:         kind,
		PreviousName: st.LastPublishedVersionName,
		PreviousCode: st.LastPublishedVersionCode,
	}, nil
}
