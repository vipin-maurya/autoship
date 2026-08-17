// Package play publishes a bundle to a Play closed-testing track.
//
// The edit lifecycle is driven through a narrow interface rather than the
// Google client directly, so the publish logic — and in particular the promise
// that a failure never leaves a dangling edit (spec §10) — is testable
// entirely offline.
package play

import "context"

// Image types Play recognises for a listing.
const (
	ImageTypePhoneScreenshots = "phoneScreenshots"
	// DefaultLocale is the listing language autoship writes.
	DefaultLocale = "en-US"
)

// Track is a track assignment within an edit.
type Track struct {
	Name         string
	VersionCodes []int64
	// Status is "draft" or "completed" (spec §13).
	Status string
	// ReleaseNotes is keyed by locale.
	ReleaseNotes map[string]string
}

// Listing is the store listing text for one locale.
type Listing struct {
	Language         string
	Title            string
	ShortDescription string
	FullDescription  string
}

// EditClient is the slice of the Play Developer API autoship needs.
type EditClient interface {
	// Insert opens an edit and returns its id.
	Insert(ctx context.Context) (editID string, err error)
	// UploadBundle uploads an .aab and returns the versionCode Play assigned.
	UploadBundle(ctx context.Context, editID, path string) (versionCode int64, err error)
	// TrackUpdate assigns version codes to a track, with release notes.
	TrackUpdate(ctx context.Context, editID string, t Track) error
	// PatchListing replaces the store listing text for a locale.
	PatchListing(ctx context.Context, editID string, l Listing) error
	// UploadImage adds one image of the given type to a locale's listing.
	UploadImage(ctx context.Context, editID, language, imageType, path string) error
	// Commit applies the edit. After this the release is live on its track.
	Commit(ctx context.Context, editID string) error
	// Delete abandons the edit, leaving Play untouched.
	Delete(ctx context.Context, editID string) error
}
