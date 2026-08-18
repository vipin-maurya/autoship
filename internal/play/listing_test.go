package play

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vipinm/autoship/internal/config"
	"github.com/vipinm/autoship/internal/release"
)

func listingRequest(kind release.Kind, shots ...string) Request {
	req := patchRequest()
	req.Release.Kind = kind
	req.Listing = &Listing{
		Title:            "MyAndroidApp",
		ShortDescription: "Modern, privacy-first local expense tracking.",
		FullDescription:  "Comprehensive personal finance app operating 100% on-device.",
	}
	req.Screenshots = shots
	return req
}

func TestPublish_PatchSkipsListing(t *testing.T) {
	f := newFake()
	req := listingRequest(release.Patch, "screenshot-01-home.png", "screenshot-02-alias.png")

	if err := (Publisher{Client: f, Cfg: draftCfg()}).Publish(context.Background(), req); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if f.did("PatchListing") {
		t.Error("a patch release must not touch the store listing")
	}
	if f.did("UploadImage") {
		t.Error("a patch release must not upload screenshots")
	}
}

func TestPublish_MinorUpdatesListing(t *testing.T) {
	f := newFake()
	shots := []string{"screenshot-01-home.png", "screenshot-02-alias.png", "screenshot-03-bills.png"}
	req := listingRequest(release.Minor, shots...)

	if err := (Publisher{Client: f, Cfg: draftCfg()}).Publish(context.Background(), req); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if f.countOf("PatchListing") != 1 {
		t.Errorf("PatchListing called %d times, want 1", f.countOf("PatchListing"))
	}
	if got := f.countOf("UploadImage"); got != len(shots) {
		t.Errorf("uploaded %d images, want %d", got, len(shots))
	}
	if len(f.lists) != 1 || f.lists[0].Title != "MyAndroidApp" {
		t.Errorf("listing = %+v, want the request's listing", f.lists)
	}
	if f.lists[0].Language != DefaultLocale {
		t.Errorf("listing language = %q, want %q", f.lists[0].Language, DefaultLocale)
	}
	for _, img := range f.images {
		if len(img) < len(ImageTypePhoneScreenshots) || img[:len(ImageTypePhoneScreenshots)] != ImageTypePhoneScreenshots {
			t.Errorf("image %q uploaded with the wrong type", img)
		}
	}
}

func TestShouldUpdateListing(t *testing.T) {
	tests := []struct {
		policy string
		kind   release.Kind
		want   bool
	}{
		{config.ListingMinor, release.Patch, false},
		{config.ListingMinor, release.Minor, true},
		{config.ListingMinor, release.Major, true},
		{config.ListingNever, release.Major, false},
		{config.ListingAny, release.Patch, true},
		{"", release.Patch, false},
		{"", release.Minor, true},
	}
	for _, tc := range tests {
		if got := ShouldUpdateListing(tc.policy, tc.kind); got != tc.want {
			t.Errorf("ShouldUpdateListing(%q, %v) = %v, want %v", tc.policy, tc.kind, got, tc.want)
		}
	}
}

func TestParseListingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "play_store_listing.md")
	content := `# Play Store Listing

## Store Listing Details
- **App Name**: MyAndroidApp
- **Short Description**: Modern, privacy-first local expense tracking and automated SMS analytics.
- **Full Description**: Comprehensive personal finance app operating 100% on-device...

## What's New
- **Version**: 1.0.5
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := ParseListingFile(path)
	if err != nil {
		t.Fatalf("ParseListingFile: %v", err)
	}
	if l.Title != "MyAndroidApp" {
		t.Errorf("Title = %q", l.Title)
	}
	if l.ShortDescription != "Modern, privacy-first local expense tracking and automated SMS analytics." {
		t.Errorf("ShortDescription = %q", l.ShortDescription)
	}
	if l.FullDescription != "Comprehensive personal finance app operating 100% on-device..." {
		t.Errorf("FullDescription = %q", l.FullDescription)
	}
	if l.Language != DefaultLocale {
		t.Errorf("Language = %q, want %q", l.Language, DefaultLocale)
	}
}

func TestParseListingFile_NoFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "play_store_listing.md")
	if err := os.WriteFile(path, []byte("# Nothing useful here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseListingFile(path); err == nil {
		t.Fatal("ParseListingFile = nil error, want one")
	}
}

func TestParseListingFile_Missing(t *testing.T) {
	if _, err := ParseListingFile(filepath.Join(t.TempDir(), "gone.md")); err == nil {
		t.Fatal("ParseListingFile on a missing file = nil error, want one")
	}
}
