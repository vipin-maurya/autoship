package play

import (
	"context"
	"fmt"
	"strings"
)

// fakeClient records the edit lifecycle instead of performing it.
type fakeClient struct {
	calls  []string
	tracks []Track
	lists  []Listing
	images []string

	// failOn makes the named method fail.
	failOn map[string]error
	// versionCode is what UploadBundle reports back.
	versionCode int64
	edits       int
}

func newFake() *fakeClient {
	return &fakeClient{failOn: map[string]error{}}
}

func (f *fakeClient) record(name string) error {
	f.calls = append(f.calls, name)
	if err, ok := f.failOn[name]; ok {
		return err
	}
	return nil
}

func (f *fakeClient) Insert(context.Context) (string, error) {
	if err := f.record("Insert"); err != nil {
		return "", err
	}
	f.edits++
	return fmt.Sprintf("edit-%d", f.edits), nil
}

func (f *fakeClient) UploadBundle(_ context.Context, _, _ string) (int64, error) {
	if err := f.record("UploadBundle"); err != nil {
		return 0, err
	}
	return f.versionCode, nil
}

func (f *fakeClient) TrackUpdate(_ context.Context, _ string, t Track) error {
	if err := f.record("TrackUpdate"); err != nil {
		return err
	}
	f.tracks = append(f.tracks, t)
	return nil
}

func (f *fakeClient) PatchListing(_ context.Context, _ string, l Listing) error {
	if err := f.record("PatchListing"); err != nil {
		return err
	}
	f.lists = append(f.lists, l)
	return nil
}

func (f *fakeClient) UploadImage(_ context.Context, _, _, imageType, path string) error {
	if err := f.record("UploadImage"); err != nil {
		return err
	}
	f.images = append(f.images, imageType+":"+path)
	return nil
}

func (f *fakeClient) Commit(_ context.Context, _ string) error { return f.record("Commit") }
func (f *fakeClient) Delete(_ context.Context, _ string) error { return f.record("Delete") }

// did reports whether the named method was called.
func (f *fakeClient) did(name string) bool {
	for _, c := range f.calls {
		if c == name {
			return true
		}
	}
	return false
}

func (f *fakeClient) countOf(name string) int {
	n := 0
	for _, c := range f.calls {
		if c == name {
			n++
		}
	}
	return n
}

func (f *fakeClient) order() string { return strings.Join(f.calls, ",") }
