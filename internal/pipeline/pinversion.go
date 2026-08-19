package pipeline

import (
	"fmt"
	"path/filepath"

	"github.com/vipinm/autoship/internal/gitrepo"
	"github.com/vipinm/autoship/internal/gradlefile"
	"github.com/vipinm/autoship/internal/release"
)

// pinVersion strips the -SNAPSHOT off the Gradle file before the bundle is
// built. Stripping it only in the release record is not enough: Gradle stamps
// whatever versionName the file declares into the manifest, so a snapshot left
// in place ships to Play as "1.0.6-SNAPSHOT".
//
// On a real run the pin is committed, which also gives S6 a commit to tag whose
// checkout reproduces exactly what shipped. On a dry run the file is written
// but not committed, so the build is representative; the caller restores it.
//
// It reports whether the file was written, so a dry run knows there is
// something to restore even when the commit that follows fails.
func (d Deps) pinVersion(rel release.Release) (bool, error) {
	path := d.gradlePath()
	cur, err := gradlefile.Parse(path)
	if err != nil {
		return false, Halt(StagePreflight, "cannot read the version to pin", err)
	}
	if cur.Name == rel.Name {
		// Already released-shaped — a repo that does not use -SNAPSHOT at all,
		// or a retry after a halt that had pinned it once.
		return false, nil
	}

	if err := gradlefile.Bump(path, gradlefile.Version{Code: rel.Code, Name: rel.Name}); err != nil {
		return false, Halt(StagePreflight, "cannot pin the release version", err)
	}
	if d.DryRun {
		d.log().Info("dry run: version pinned for the build only",
			"from", cur.Name, "to", rel.Name)
		return true, nil
	}
	if err := gitrepo.CommitAll(d.Cfg.Repo.Path, fmt.Sprintf("chore(release): %s", rel.Name)); err != nil {
		return true, Halt(StagePreflight, "cannot commit the release version", err)
	}
	d.log().Info("release version pinned", "from", cur.Name, "to", rel.Name, "versionCode", rel.Code)
	return true, nil
}

// unpinVersion puts a dry run's pinned Gradle file back the way it found it.
func (d Deps) unpinVersion() {
	if err := gitrepo.RestoreFile(d.Cfg.Repo.Path, d.Cfg.App.GradleFile); err != nil {
		d.log().Warn("dry run: could not restore the pinned version",
			"file", d.Cfg.App.GradleFile, "error", err)
	}
}

func (d Deps) gradlePath() string {
	return filepath.Join(d.Cfg.Repo.Path, filepath.FromSlash(d.Cfg.App.GradleFile))
}
