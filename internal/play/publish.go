package play

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/release"
)

// Stage is the S5 label used in halt records.
const Stage = "S5"

// Request is everything one publish needs.
type Request struct {
	Release release.Release
	// BundlePath is the .aab to upload.
	BundlePath string
	// Notes is the customer-facing copy, already validated against the Play
	// character limit.
	Notes string
	// Locale defaults to DefaultLocale when empty.
	Locale string
	// Listing is uploaded only on non-patch releases; nil means none is
	// available.
	Listing *Listing
	// Screenshots are uploaded alongside a listing.
	Screenshots []string
}

// Publisher runs the edit lifecycle for one release.
type Publisher struct {
	Client EditClient
	Cfg    config.Play
	// DryRun performs everything except the commit, then abandons the edit, so
	// a soak run exercises the real API without shipping (spec §13).
	DryRun bool
	Log    *slog.Logger
}

func (p Publisher) log() *slog.Logger {
	if p.Log != nil {
		return p.Log
	}
	return slog.New(slog.NewTextHandler(discard{}, nil))
}

type discard struct{}

func (discard) Write(b []byte) (int, error) { return len(b), nil }

// Publish uploads the bundle, assigns it to the configured track, optionally
// refreshes the listing, and commits. Any failure abandons the edit rather
// than leaving one dangling on the Play account (spec §10).
func (p Publisher) Publish(ctx context.Context, req Request) (err error) {
	locale := req.Locale
	if locale == "" {
		locale = DefaultLocale
	}

	editID, err := p.Client.Insert(ctx)
	if err != nil {
		return fmt.Errorf("%s: open edit: %w", Stage, err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		// Abandoning is best-effort cleanup: it must not mask the real error,
		// but a silent failure here would leave an edit blocking later runs.
		if delErr := p.Client.Delete(ctx, editID); delErr != nil {
			p.log().Warn("could not abandon the play edit", "edit", editID, "error", delErr)
			if err == nil {
				err = fmt.Errorf("%s: abandon edit: %w", Stage, delErr)
			}
			return
		}
		p.log().Info("play edit abandoned", "edit", editID)
	}()

	code, err := p.Client.UploadBundle(ctx, editID, req.BundlePath)
	if err != nil {
		return fmt.Errorf("%s: upload bundle: %w", Stage, err)
	}
	if want := int64(req.Release.Code); code != 0 && code != want {
		return fmt.Errorf("%s: play assigned versionCode %d but the repo declared %d", Stage, code, want)
	}
	if code == 0 {
		code = int64(req.Release.Code)
	}
	p.log().Info("bundle uploaded", "edit", editID, "versionCode", code)

	if err := p.applyListing(ctx, editID, locale, req); err != nil {
		return err
	}

	if err := p.assignTrack(ctx, editID, locale, code, req); err != nil {
		return err
	}

	if p.DryRun {
		p.log().Info("dry run: edit prepared but not committed", "edit", editID, "track", p.Cfg.Track)
		return nil
	}
	if err := p.Client.Commit(ctx, editID); err != nil {
		return fmt.Errorf("%s: commit edit: %w", Stage, err)
	}
	committed = true
	p.log().Info("play edit committed", "edit", editID, "track", p.Cfg.Track, "status", p.Cfg.Rollout)
	return nil
}

// assignTrack builds the track payload from config and the release.
func (p Publisher) assignTrack(ctx context.Context, editID, locale string, code int64, req Request) error {
	t := Track{
		Name:         p.Cfg.Track,
		VersionCodes: []int64{code},
		Status:       p.Cfg.Rollout,
		ReleaseNotes: map[string]string{locale: req.Notes},
	}
	if err := p.Client.TrackUpdate(ctx, editID, t); err != nil {
		return fmt.Errorf("%s: assign track %s: %w", Stage, p.Cfg.Track, err)
	}
	return nil
}
