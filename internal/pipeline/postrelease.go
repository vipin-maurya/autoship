package pipeline

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/gitrepo"
	"github.com/vipinm/autoship/internal/gradlefile"
	"github.com/vipinm/autoship/internal/release"
)

// StagePostRelease is the S6 label used in halt records.
const StagePostRelease = "S6"

// PostReleaseResult reports what S6 did, so a dry run can say what it would
// have done and a halt can name the commit that never landed.
type PostReleaseResult struct {
	Tag        string
	NextName   string
	NextCode   int
	CommitSHA  string
	Pushed     bool
	Rebased    bool
	DryRunOnly bool
}

// PostRelease tags the release, bumps the Gradle file to the next -SNAPSHOT,
// commits and pushes.
//
// This runs after the irreversible publish, so a rejected push is recoverable
// by rebasing onto the moved branch and retrying once. If that still fails the
// halt names both the published versionCode and the unpushed commit, so
// recovery is a one-line manual push rather than a mystery (spec §11).
func PostRelease(cfg *config.Config, rel release.Release, dryRun bool, log *slog.Logger) (PostReleaseResult, error) {
	res := PostReleaseResult{Tag: rel.Tag(), DryRunOnly: dryRun}
	repo := cfg.Repo.Path
	gradlePath := filepath.Join(repo, filepath.FromSlash(cfg.App.GradleFile))

	cur, err := gradlefile.Parse(gradlePath)
	if err != nil {
		return res, Halt(StagePostRelease, "cannot read the version to bump", err)
	}
	nextName, err := release.NextPatch(rel.Name)
	if err != nil {
		return res, Halt(StagePostRelease, "cannot compute the next version", err)
	}
	next := gradlefile.NextSnapshot(cur, nextName)
	res.NextName, res.NextCode = next.Name, next.Code

	if dryRun {
		log.Info("dry run: would tag, bump and push",
			"tag", res.Tag, "nextVersionName", next.Name, "nextVersionCode", next.Code)
		return res, nil
	}

	if err := gitrepo.Tag(repo, rel.Tag(), "release "+rel.Name); err != nil {
		return res, Halt(StagePostRelease, "cannot create tag "+rel.Tag(), err)
	}
	if err := gradlefile.Bump(gradlePath, next); err != nil {
		return res, Halt(StagePostRelease, "cannot bump the version", err)
	}
	msg := fmt.Sprintf("chore(release): %s, bump to %s", rel.Name, next.Name)
	if err := gitrepo.CommitAll(repo, msg); err != nil {
		return res, Halt(StagePostRelease, "cannot commit the version bump", err)
	}
	if sha, err := gitrepo.HeadSHA(repo); err == nil {
		res.CommitSHA = sha
	}

	err = gitrepo.Push(repo, cfg.Repo.Remote, cfg.Repo.Branch, true)
	if err == nil {
		res.Pushed = true
		log.Info("post-release pushed", "tag", res.Tag, "commit", res.CommitSHA)
		return res, nil
	}
	if !errors.Is(err, gitrepo.ErrPushRejected) {
		return res, Halt(StagePostRelease, "cannot push the release commit", err)
	}

	log.Warn("post-release push rejected, rebasing onto the moved branch",
		"remote", cfg.Repo.Remote, "branch", cfg.Repo.Branch)
	if rebaseErr := gitrepo.PullRebase(repo, cfg.Repo.Remote, cfg.Repo.Branch); rebaseErr != nil {
		return res, pushHalt(rel, res, rebaseErr)
	}
	res.Rebased = true
	if sha, err := gitrepo.HeadSHA(repo); err == nil {
		res.CommitSHA = sha
	}
	if pushErr := gitrepo.Push(repo, cfg.Repo.Remote, cfg.Repo.Branch, true); pushErr != nil {
		return res, pushHalt(rel, res, pushErr)
	}
	res.Pushed = true
	log.Info("post-release pushed after rebase", "tag", res.Tag, "commit", res.CommitSHA)
	return res, nil
}

// pushHalt names both identifiers a human needs: what is already live on Play,
// and which local commit still has to reach the remote.
func pushHalt(rel release.Release, res PostReleaseResult, err error) *HaltError {
	return Halt(StagePostRelease, fmt.Sprintf(
		"versionCode %d (%s) is already published to Play, but the bump commit %s could not be pushed; "+
			"push it manually to unblock the next release",
		rel.Code, rel.Name, shortSHA(res.CommitSHA)), err)
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	if sha == "" {
		return "(unknown)"
	}
	return sha
}
