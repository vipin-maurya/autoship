package play

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/release"
)

// ShouldUpdateListing reports whether a release of this kind refreshes the
// store listing and screenshots. A patch is an AAB plus notes and nothing else
// (spec §7.3, R5).
func ShouldUpdateListing(policy string, kind release.Kind) bool {
	switch policy {
	case config.ListingNever:
		return false
	case config.ListingAny:
		return true
	case config.ListingMinor, "":
		return kind != release.Patch
	default:
		return false
	}
}

// applyListing patches the listing and uploads screenshots, when the release
// kind and the configured policy both call for it.
func (p Publisher) applyListing(ctx context.Context, editID, locale string, req Request) error {
	if !ShouldUpdateListing(p.Cfg.UpdateListingOn, req.Release.Kind) {
		p.log().Info("skipping listing update",
			"kind", req.Release.Kind.String(), "policy", p.Cfg.UpdateListingOn)
		return nil
	}
	if req.Listing != nil {
		l := *req.Listing
		if l.Language == "" {
			l.Language = locale
		}
		if err := p.Client.PatchListing(ctx, editID, l); err != nil {
			return fmt.Errorf("%s: patch listing: %w", Stage, err)
		}
		p.log().Info("listing updated", "edit", editID, "locale", l.Language)
	}
	for _, shot := range req.Screenshots {
		if err := p.Client.UploadImage(ctx, editID, locale, ImageTypePhoneScreenshots, shot); err != nil {
			return fmt.Errorf("%s: upload screenshot %s: %w", Stage, shot, err)
		}
	}
	if len(req.Screenshots) > 0 {
		p.log().Info("screenshots uploaded", "edit", editID, "count", len(req.Screenshots))
	}
	return nil
}

// labelled matches the "- **Short Description**: text" shape the releasing-app
// skill writes into docs/release/play_store_listing.md.
var labelled = regexp.MustCompile(`^\s*[-*]?\s*\*\*([^*]+)\*\*\s*:\s*(.*)$`)

// ParseListingFile reads a play_store_listing.md and extracts the fields Play
// accepts. Missing fields are left empty rather than being invented.
func ParseListingFile(path string) (Listing, error) {
	f, err := os.Open(path)
	if err != nil {
		return Listing{}, fmt.Errorf("read store listing: %w", err)
	}
	defer f.Close()

	l := Listing{Language: DefaultLocale}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		m := labelled.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		value := strings.TrimSpace(m[2])
		if value == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(m[1])) {
		case "app name", "title", "app title":
			if l.Title == "" {
				l.Title = value
			}
		case "short description":
			if l.ShortDescription == "" {
				l.ShortDescription = value
			}
		case "full description", "description":
			if l.FullDescription == "" {
				l.FullDescription = value
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Listing{}, fmt.Errorf("read store listing: %w", err)
	}
	if l.Title == "" && l.ShortDescription == "" && l.FullDescription == "" {
		return Listing{}, fmt.Errorf("%s contains no listing fields", path)
	}
	return l, nil
}
