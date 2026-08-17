package notes

import (
	"strings"
	"testing"
	"time"

	"github.com/vipinm/autoship/internal/release"
)

func TestTechnicalNotes(t *testing.T) {
	rel := release.Release{Name: "1.0.5", Code: 8, Kind: release.Patch, PreviousName: "1.0.4", PreviousCode: 7}
	res := BuildResult{
		SHA:         "a1b2c3d",
		CommitRange: "v1.0.4..a1b2c3d",
		Commits:     []string{"feat: add settings screen", "fix: filter persistence"},
		Tests:       "passed (:app:testDebugUnitTest)",
		Lint:        "passed (:app:lintRelease)",
		UIValidated: "passed (jvm)",
		BundlePath:  `C:\artifacts\v1.0.5\app-release.aab`,
		BundleBytes: 12 * 1024 * 1024,
		Track:       "alpha",
		At:          time.Date(2026, 8, 17, 9, 14, 2, 0, time.UTC),
	}

	got := Technical(rel, res)
	for _, want := range []string{
		"1.0.5", "8", "1.0.4", "v1.0.4..a1b2c3d",
		"passed (:app:testDebugUnitTest)", "passed (:app:lintRelease)", "passed (jvm)",
		"app-release.aab", "12.0 MB", "alpha", "2026-08-17T09:14:02Z",
		"feat: add settings screen", "fix: filter persistence",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("technical notes missing %q:\n%s", want, got)
		}
	}
}

func TestTechnicalNotes_FirstRelease(t *testing.T) {
	got := Technical(release.Release{Name: "1.0.0", Code: 1, Kind: release.Major}, BuildResult{})
	if !strings.Contains(got, "none") {
		t.Errorf("notes = %q, want the previous release reported as none", got)
	}
	if !strings.Contains(got, "no commit subjects recorded") {
		t.Errorf("notes = %q, want the empty commit list handled", got)
	}
}
