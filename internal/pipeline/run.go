package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vipinm/autoship/internal/artifacts"
	"github.com/vipinm/autoship/internal/build"
	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/gitrepo"
	"github.com/vipinm/autoship/internal/notes"
	"github.com/vipinm/autoship/internal/play"
	"github.com/vipinm/autoship/internal/release"
	"github.com/vipinm/autoship/internal/runner"
	"github.com/vipinm/autoship/internal/state"
)

// Stage labels for the stages that live in this package.
const (
	StageGate     = "S0"
	StageAssemble = "S4"
)

// Names of the files written into the release folder, matching what the manual
// releasing-app skill produces.
const (
	CustomerNotesFile  = "release-notes-customer.txt"
	TechnicalNotesFile = "release-notes-technical.md"
)

// Deps are everything a run needs from the outside world. The seams that
// matter — the command runner and the Play client — are interfaces, so a test
// can prove what did and did not happen without a JVM or a network.
type Deps struct {
	Cfg   *config.Config
	Store state.Store
	Log   *slog.Logger
	// LogPath is recorded in a halt so a human can find the run's log.
	LogPath string
	Runner  runner.Runner
	// PlayClient builds the publisher client. It is only called when a run
	// actually reaches S5.
	PlayClient func(ctx context.Context) (play.EditClient, error)
	DryRun     bool
	// Now defaults to time.Now.
	Now func() time.Time
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d Deps) log() *slog.Logger {
	if d.Log != nil {
		return d.Log
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Result reports what a run did, for the CLI to print and for tests to assert.
type Result struct {
	Decision  Decision
	Head      string
	Release   release.Release
	Folder    artifacts.Folder
	Published bool
	Post      PostReleaseResult
	Halt      *state.Halt
}

// Execute runs one tick: the S0 gate, and — only if main actually moved — the
// full S1–S6 pipeline.
func Execute(ctx context.Context, d Deps) (Result, error) {
	log := d.log()
	st, err := d.Store.Load()
	if err != nil {
		return Result{}, fmt.Errorf("%s: load state: %w", StageGate, err)
	}

	head, headErr := gitrepo.ResolveRemoteHead(d.Cfg.Repo.Path, d.Cfg.Repo.Remote, d.Cfg.Repo.Branch)
	decision := Decide(st, head, headErr)
	res := Result{Decision: decision, Head: head}

	if decision != Proceed {
		// This is the path ~99% of invocations take, and nothing below it may
		// start a JVM, touch Gradle, or allocate meaningfully (spec §3).
		switch decision {
		case SkipHalted:
			log.Info("halted; waiting for a new commit or `autoship resume`",
				"stage", haltStage(st), "reason", haltReason(st))
			res.Halt = st.Halted
		case SkipTransient:
			log.Info("remote unreachable; retrying on the next tick", "error", headErr)
		case SkipError:
			log.Error("gate failed", "error", headErr)
		default:
			log.Info("no change", "head", short(head))
		}
		st.LastRunAt = d.now()
		if err := d.Store.Save(st); err != nil {
			return res, fmt.Errorf("%s: save state: %w", StageGate, err)
		}
		if decision == SkipError {
			return res, fmt.Errorf("%s: %w", StageGate, headErr)
		}
		return res, nil
	}

	log.Info("main moved; starting a release run", "head", short(head), "dryRun", d.DryRun)
	st.Status = state.StatusRunning
	state.ClearHalt(&st)
	st.LastRunAt = d.now()
	if err := d.Store.Save(st); err != nil {
		return res, fmt.Errorf("%s: save state: %w", StageGate, err)
	}

	runErr := d.release(ctx, &st, &res)
	if runErr != nil {
		var he *HaltError
		if errors.As(runErr, &he) {
			state.SetHalt(&st, he.Stage, he.Error(), head, d.LogPath, d.now())
			res.Halt = st.Halted
			log.Error("halted", "stage", he.Stage, "reason", he.Error())
		} else {
			state.SetHalt(&st, StageGate, runErr.Error(), head, d.LogPath, d.now())
			res.Halt = st.Halted
			log.Error("halted", "error", runErr)
		}
		if err := d.Store.Save(st); err != nil {
			return res, errors.Join(runErr, fmt.Errorf("save state: %w", err))
		}
		return res, runErr
	}

	st.Status = state.StatusIdle
	state.ClearHalt(&st)
	if d.DryRun {
		// A dry run publishes nothing, so the commit it examined is still
		// releasable. Recording it would make the real run that follows report
		// "no change" and skip the release entirely.
		st.LastRunAt = d.now()
		if err := d.Store.Save(st); err != nil {
			return res, fmt.Errorf("save state: %w", err)
		}
		log.Info("dry run complete; commit left unprocessed",
			"version", res.Release.Name, "versionCode", res.Release.Code,
			"kind", res.Release.Kind.String())
		return res, nil
	}

	st.LastProcessedSHA = head
	if res.Post.Pushed && res.Post.CommitSHA != "" {
		// S6 pushed autoship's own bump commit, so that commit is now the tip
		// of the release branch. Recording it stops the next tick from treating
		// the tool's own housekeeping as releasable work — which would halt
		// immediately, since no one has written notes for the next version.
		st.LastProcessedSHA = res.Post.CommitSHA
	}
	st.LastPublishedVersionCode = res.Release.Code
	st.LastPublishedVersionName = res.Release.Name
	st.LastRunAt = d.now()
	if err := d.Store.Save(st); err != nil {
		return res, fmt.Errorf("save state: %w", err)
	}
	log.Info("release run complete",
		"version", res.Release.Name, "versionCode", res.Release.Code,
		"kind", res.Release.Kind.String(), "dryRun", d.DryRun)
	return res, nil
}

// release runs S1–S6. Every failure it returns is a *HaltError, so the caller
// records exactly one halt with the stage that produced it.
func (d Deps) release(ctx context.Context, st *state.State, res *Result) error {
	log := d.log()
	cfg := d.Cfg

	// Build from exactly what the gate saw.
	if err := gitrepo.Checkout(cfg.Repo.Path, cfg.Repo.Remote, cfg.Repo.Branch); err != nil {
		return Halt(StageGate, "cannot fast-forward the working tree to the release branch", err)
	}

	rel, err := Preflight(cfg, *st)
	if err != nil {
		return err
	}
	rel.SHA = res.Head
	res.Release = rel
	log.Info("preflight passed", "version", rel.Name, "versionCode", rel.Code, "kind", rel.Kind.String())

	// Take the -SNAPSHOT off the Gradle file before anything builds, so the
	// bundle carries the version being released rather than the one being
	// developed.
	pinned, err := d.pinVersion(rel)
	if pinned && d.DryRun {
		defer d.unpinVersion()
	}
	if err != nil {
		return err
	}

	// S2 — tests, lint, bundle.
	gradle := build.GradleStage{Runner: d.Runner, Dir: cfg.Repo.Path, Cfg: cfg.Gradle}
	if err := gradle.Execute(ctx); err != nil {
		return Halt(build.StageBuild, "build failed", err)
	}
	log.Info("build passed", "bundle", cfg.Gradle.Bundle)

	// S3 — UI validation.
	ui := build.UIValidateStage{Runner: d.Runner, Dir: cfg.Repo.Path, Cfg: cfg.UIValidation}
	if err := ui.Execute(ctx); err != nil {
		return Halt(build.StageUIValidation, "ui validation failed", err)
	}

	// S4 — assemble the artefact folder and collect the notes.
	folder, customerNotes, err := d.assemble(rel)
	if err != nil {
		return err
	}
	res.Folder = folder
	log.Info("artifacts assembled", "dir", folder.Dir, "screenshots", len(folder.Screenshots))

	// S5 — publish.
	if err := d.publish(ctx, rel, folder, customerNotes); err != nil {
		return err
	}
	res.Published = !d.DryRun

	// S6 — tag, bump, push.
	post, err := PostRelease(cfg, rel, d.DryRun, log)
	res.Post = post
	return err
}

// assemble builds the release folder and resolves the customer-facing notes.
func (d Deps) assemble(rel release.Release) (artifacts.Folder, string, error) {
	cfg := d.Cfg
	moduleDir := build.ModuleDir(cfg.Repo.Path, cfg.App.Module)
	bundlePath, err := build.FindBundle(moduleDir)
	if err != nil {
		return artifacts.Folder{}, "", Halt(StageAssemble, "cannot find the release bundle", err)
	}

	customer, err := d.customerNotes(rel)
	if err != nil {
		return artifacts.Folder{}, "", err
	}

	shots := cfg.Artifacts.ScreenshotsFrom
	if shots != "" && !filepath.IsAbs(shots) {
		shots = filepath.Join(cfg.Repo.Path, filepath.FromSlash(shots))
	}
	folder, err := artifacts.Assemble(cfg.Artifacts.Root, rel, bundlePath, shots)
	if err != nil {
		return artifacts.Folder{}, "", Halt(StageAssemble, "cannot assemble the release folder", err)
	}

	if customer != "" {
		if _, err := folder.WriteFile(CustomerNotesFile, customer+"\n"); err != nil {
			return artifacts.Folder{}, "", Halt(StageAssemble, "cannot write the customer release notes", err)
		}
	}
	technical := notes.Technical(rel, d.buildResult(rel, folder, bundlePath))
	if _, err := folder.WriteFile(TechnicalNotesFile, technical); err != nil {
		return artifacts.Folder{}, "", Halt(StageAssemble, "cannot write the technical release notes", err)
	}
	return folder, customer, nil
}

// customerNotes resolves the provider chain and enforces the Play limit.
func (d Deps) customerNotes(rel release.Release) (string, error) {
	chain, err := notes.BuildChain(d.Cfg.Notes, notes.Deps{RepoPath: d.Cfg.Repo.Path})
	if err != nil {
		return "", Halt(StageAssemble, "notes configuration is invalid", err)
	}
	text, err := chain.Notes(context.Background(), rel)
	if err != nil {
		if errors.Is(err, notes.ErrNoNotes) {
			if d.Cfg.Notes.OnExhausted == config.OnExhaustedSkip {
				// Configured to ship without copy rather than stop. Loud, but
				// not fatal — the tester-facing cost is an empty "What's new".
				d.log().Warn("no release notes available; publishing without them", "error", err)
				return "", nil
			}
			return "", Halt(StageAssemble,
				fmt.Sprintf("no release notes for %s; write them and push (chain: %s)",
					rel.Name, strings.Join(d.Cfg.Notes.Source, ", ")), err)
		}
		return "", Halt(StageAssemble, "cannot resolve the release notes", err)
	}
	if err := notes.Validate(text); err != nil {
		return "", Halt(StageAssemble, "release notes are not publishable", err)
	}
	return text, nil
}

func (d Deps) buildResult(rel release.Release, folder artifacts.Folder, bundlePath string) notes.BuildResult {
	res := notes.BuildResult{
		SHA:         rel.SHA,
		Tests:       outcome(d.Cfg.Gradle.UnitTests),
		Lint:        outcome(d.Cfg.Gradle.Lint),
		UIValidated: uiOutcome(d.Cfg.UIValidation),
		BundlePath:  folder.Bundle,
		Track:       d.Cfg.Play.Track,
		At:          d.now(),
	}
	if info, err := os.Stat(bundlePath); err == nil {
		res.BundleBytes = info.Size()
	}
	if rel.PreviousName != "" {
		res.CommitRange = "v" + rel.PreviousName + "..HEAD"
	}
	if subjects, err := gitrepo.Subjects(d.Cfg.Repo.Path, res.CommitRange); err == nil {
		res.Commits = subjects
	}
	return res
}

func outcome(task string) string {
	if strings.TrimSpace(task) == "" {
		return "not configured"
	}
	return "passed (" + task + ")"
}

func uiOutcome(cfg config.UIValidation) string {
	if cfg.Mode == "" || cfg.Mode == config.UIModeNone {
		return "not configured"
	}
	return "passed (" + cfg.Mode + ")"
}

// publish runs S5, including reading the store listing when the release kind
// calls for it.
func (d Deps) publish(ctx context.Context, rel release.Release, folder artifacts.Folder, customerNotes string) error {
	client, err := d.PlayClient(ctx)
	if err != nil {
		return Halt(play.Stage, "cannot create the play client", err)
	}

	req := play.Request{
		Release:    rel,
		BundlePath: folder.Bundle,
		Notes:      customerNotes,
	}
	if play.ShouldUpdateListing(d.Cfg.Play.UpdateListingOn, rel.Kind) {
		req.Screenshots = folder.Screenshots
		listingPath := d.Cfg.Play.ListingFile
		if listingPath != "" && !filepath.IsAbs(listingPath) {
			listingPath = filepath.Join(d.Cfg.Repo.Path, filepath.FromSlash(listingPath))
		}
		listing, err := play.ParseListingFile(listingPath)
		if err != nil {
			// A non-patch release is supposed to refresh the listing; failing
			// to read it silently would ship a stale store page.
			return Halt(play.Stage, "cannot read the store listing", err)
		}
		req.Listing = &listing
	}

	pub := play.Publisher{Client: client, Cfg: d.Cfg.Play, DryRun: d.DryRun, Log: d.log()}
	if err := pub.Publish(ctx, req); err != nil {
		return Halt(play.Stage, "publish failed", err)
	}
	return nil
}

func haltStage(st state.State) string {
	if st.Halted == nil {
		return ""
	}
	return st.Halted.Stage
}

func haltReason(st state.State) string {
	if st.Halted == nil {
		return ""
	}
	return st.Halted.Reason
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
