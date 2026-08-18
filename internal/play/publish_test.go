package play

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/release"
)

func patchRequest() Request {
	return Request{
		Release:    release.Release{Name: "1.0.5", Code: 8, Kind: release.Patch, PreviousName: "1.0.4"},
		BundlePath: `C:\artifacts\v1.0.5\app-release.aab`,
		Notes:      "Faster search and fewer duplicate merchants.",
	}
}

func draftCfg() config.Play {
	return config.Play{Track: "alpha", Rollout: config.RolloutDraft, UpdateListingOn: config.ListingMinor}
}

func TestPublish_FollowsEditLifecycle(t *testing.T) {
	f := newFake()
	p := Publisher{Client: f, Cfg: draftCfg()}

	if err := p.Publish(context.Background(), patchRequest()); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if got, want := f.order(), "Insert,UploadBundle,TrackUpdate,Commit"; got != want {
		t.Errorf("call order = %q, want %q", got, want)
	}
	if f.did("Delete") {
		t.Error("a successful publish must not abandon the edit")
	}
}

func TestTrackUpdate_SetsTrackAndNotes(t *testing.T) {
	f := newFake()
	p := Publisher{Client: f, Cfg: draftCfg()}
	req := patchRequest()

	if err := p.Publish(context.Background(), req); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(f.tracks) != 1 {
		t.Fatalf("recorded %d track updates, want 1", len(f.tracks))
	}
	got := f.tracks[0]
	if got.Name != "alpha" {
		t.Errorf("track = %q, want alpha", got.Name)
	}
	if len(got.VersionCodes) != 1 || got.VersionCodes[0] != 8 {
		t.Errorf("versionCodes = %v, want [8]", got.VersionCodes)
	}
	if got.Status != config.RolloutDraft {
		t.Errorf("status = %q, want %q", got.Status, config.RolloutDraft)
	}
	if got.ReleaseNotes[DefaultLocale] != req.Notes {
		t.Errorf("notes[%s] = %q, want %q", DefaultLocale, got.ReleaseNotes[DefaultLocale], req.Notes)
	}
}

func TestTrackUpdate_HonoursCompletedRollout(t *testing.T) {
	f := newFake()
	cfg := draftCfg()
	cfg.Rollout = config.RolloutCompleted
	if err := (Publisher{Client: f, Cfg: cfg}).Publish(context.Background(), patchRequest()); err != nil {
		t.Fatal(err)
	}
	if f.tracks[0].Status != config.RolloutCompleted {
		t.Errorf("status = %q, want %q", f.tracks[0].Status, config.RolloutCompleted)
	}
}

func TestPublish_RejectsVersionCodeMismatch(t *testing.T) {
	f := newFake()
	f.versionCode = 99 // Play saw something other than what the repo declared.
	err := (Publisher{Client: f, Cfg: draftCfg()}).Publish(context.Background(), patchRequest())
	if err == nil {
		t.Fatal("Publish = nil error, want a mismatch error")
	}
	if !strings.Contains(err.Error(), "99") || !strings.Contains(err.Error(), "8") {
		t.Errorf("error = %v, want it to name both version codes", err)
	}
	if f.did("Commit") {
		t.Error("must not commit an edit whose bundle does not match the release")
	}
	if !f.did("Delete") {
		t.Error("must abandon the edit")
	}
}

func TestPublish_DeletesEditOnFailure(t *testing.T) {
	for _, failing := range []string{"UploadBundle", "TrackUpdate", "PatchListing", "UploadImage"} {
		t.Run(failing, func(t *testing.T) {
			f := newFake()
			f.failOn[failing] = errors.New("play said no")
			cfg := draftCfg()
			cfg.UpdateListingOn = config.ListingAny

			req := patchRequest()
			req.Listing = &Listing{Title: "MyAndroidApp"}
			req.Screenshots = []string{"a.png"}

			err := (Publisher{Client: f, Cfg: cfg}).Publish(context.Background(), req)
			if err == nil {
				t.Fatalf("Publish = nil error, want the %s failure", failing)
			}
			if !f.did("Delete") {
				t.Errorf("edit not abandoned after %s failed: %s", failing, f.order())
			}
			if f.did("Commit") {
				t.Errorf("edit committed despite %s failing: %s", failing, f.order())
			}
		})
	}
}

func TestPublish_ReportsAbandonFailure(t *testing.T) {
	f := newFake()
	f.failOn["Commit"] = errors.New("play said no")
	f.failOn["Delete"] = errors.New("and no again")

	err := (Publisher{Client: f, Cfg: draftCfg()}).Publish(context.Background(), patchRequest())
	if err == nil {
		t.Fatal("Publish = nil error, want the commit failure")
	}
	// The original failure is what a human needs to see first.
	if !strings.Contains(err.Error(), "commit edit") {
		t.Errorf("error = %v, want the commit failure preserved", err)
	}
}

func TestPublish_DryRunPreparesButDoesNotCommit(t *testing.T) {
	f := newFake()
	p := Publisher{Client: f, Cfg: draftCfg(), DryRun: true}

	if err := p.Publish(context.Background(), patchRequest()); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if f.did("Commit") {
		t.Errorf("dry run committed the edit: %s", f.order())
	}
	if !f.did("UploadBundle") {
		t.Errorf("dry run skipped the upload: %s", f.order())
	}
	if !f.did("Delete") {
		t.Errorf("dry run left the edit dangling: %s", f.order())
	}
}

func TestPublish_InsertFailureIsReported(t *testing.T) {
	f := newFake()
	f.failOn["Insert"] = errors.New("no credentials")
	err := (Publisher{Client: f, Cfg: draftCfg()}).Publish(context.Background(), patchRequest())
	if err == nil || !strings.Contains(err.Error(), Stage) {
		t.Fatalf("Publish = %v, want an S5 error", err)
	}
	if f.did("Delete") {
		t.Error("nothing to abandon when the edit was never opened")
	}
}
