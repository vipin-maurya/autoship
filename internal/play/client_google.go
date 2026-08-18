package play

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	androidpublisher "google.golang.org/api/androidpublisher/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// googleClient implements EditClient over the official Play Developer API.
// Every call goes through withRetry, so the retry policy lives in one place
// rather than being sprinkled through the publish flow.
type googleClient struct {
	svc *androidpublisher.Service
	pkg string
}

// NewClient builds an EditClient authenticated with a Play service-account
// JSON key. The account should hold only "Release to testing tracks", so it is
// incapable of touching production even if the key leaks (spec §8).
func NewClient(ctx context.Context, serviceAccountJSON []byte, packageName string) (EditClient, error) {
	if len(serviceAccountJSON) == 0 {
		return nil, fmt.Errorf("play service account credentials are empty")
	}
	if packageName == "" {
		return nil, fmt.Errorf("app.package is required to publish")
	}
	svc, err := androidpublisher.NewService(ctx,
		option.WithCredentialsJSON(serviceAccountJSON),
		option.WithScopes(androidpublisher.AndroidpublisherScope),
	)
	if err != nil {
		return nil, fmt.Errorf("create play client: %w", err)
	}
	return &googleClient{svc: svc, pkg: packageName}, nil
}

func (c *googleClient) Insert(ctx context.Context) (string, error) {
	var id string
	err := withRetry(ctx, "insert edit", func() error {
		edit, err := c.svc.Edits.Insert(c.pkg, &androidpublisher.AppEdit{}).Context(ctx).Do()
		if err != nil {
			return err
		}
		id = edit.Id
		return nil
	})
	return id, err
}

func (c *googleClient) UploadBundle(ctx context.Context, editID, path string) (int64, error) {
	var code int64
	err := withRetry(ctx, "upload bundle", func() error {
		// The file is reopened per attempt: a retried upload cannot reuse a
		// reader that the failed attempt already consumed.
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		bundle, err := c.svc.Edits.Bundles.Upload(c.pkg, editID).Media(f, googleapi.ContentType("application/octet-stream")).Context(ctx).Do()
		if err != nil {
			return err
		}
		code = bundle.VersionCode
		return nil
	})
	return code, err
}

var langTagPattern = regexp.MustCompile(`(?s)^\s*<([a-zA-Z]{2,3}(?:-[a-zA-Z0-9]+)*)>\s*(.*?)\s*</\1>\s*$`)

func cleanNotes(text string) string {
	if m := langTagPattern.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[2])
	}
	return strings.TrimSpace(text)
}

func (c *googleClient) TrackUpdate(ctx context.Context, editID string, t Track) error {
	notes := make([]*androidpublisher.LocalizedText, 0, len(t.ReleaseNotes))
	for lang, text := range t.ReleaseNotes {
		notes = append(notes, &androidpublisher.LocalizedText{Language: lang, Text: cleanNotes(text)})
	}
	payload := &androidpublisher.Track{
		Track: t.Name,
		Releases: []*androidpublisher.TrackRelease{{
			Status:       t.Status,
			VersionCodes: t.VersionCodes,
			ReleaseNotes: notes,
		}},
	}
	return withRetry(ctx, "update track", func() error {
		_, err := c.svc.Edits.Tracks.Update(c.pkg, editID, t.Name, payload).Context(ctx).Do()
		return err
	})
}

func (c *googleClient) PatchListing(ctx context.Context, editID string, l Listing) error {
	lang := l.Language
	if lang == "" {
		lang = DefaultLocale
	}
	payload := &androidpublisher.Listing{
		Language:         lang,
		Title:            l.Title,
		ShortDescription: l.ShortDescription,
		FullDescription:  l.FullDescription,
	}
	return withRetry(ctx, "update listing", func() error {
		_, err := c.svc.Edits.Listings.Update(c.pkg, editID, lang, payload).Context(ctx).Do()
		return err
	})
}

func (c *googleClient) UploadImage(ctx context.Context, editID, language, imageType, path string) error {
	return withRetry(ctx, "upload image", func() error {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = c.svc.Edits.Images.Upload(c.pkg, editID, language, imageType).Media(f).Context(ctx).Do()
		return err
	})
}

func (c *googleClient) Commit(ctx context.Context, editID string) error {
	return withRetry(ctx, "commit edit", func() error {
		_, err := c.svc.Edits.Commit(c.pkg, editID).Context(ctx).Do()
		return err
	})
}

func (c *googleClient) Delete(ctx context.Context, editID string) error {
	// Abandoning is cleanup, and cleanup that retries forever is worse than
	// cleanup that fails loudly, so this uses the same bounded policy.
	return withRetry(ctx, "delete edit", func() error {
		return c.svc.Edits.Delete(c.pkg, editID).Context(ctx).Do()
	})
}
