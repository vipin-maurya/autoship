package artifacts

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/vipinm/autoship/internal/release"
)

func TestAssemble(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()

	bundle := filepath.Join(src, "app-release.aab")
	want := []byte("pretend this is a bundle")
	if err := os.WriteFile(bundle, want, 0o644); err != nil {
		t.Fatal(err)
	}
	shotsFrom := filepath.Join(src, "screenshots")
	if err := os.MkdirAll(shotsFrom, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"screenshot-01-home.png", "screenshot-02-merchant-alias-chaining.png", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(shotsFrom, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rel := release.Release{Name: "1.0.5", Code: 8, Kind: release.Patch}
	f, err := Assemble(root, rel, bundle, shotsFrom)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	if f.Dir != filepath.Join(root, "v1.0.5") {
		t.Errorf("Dir = %q, want %q", f.Dir, filepath.Join(root, "v1.0.5"))
	}
	if _, err := os.Stat(f.ScreenshotsDir()); err != nil {
		t.Errorf("screenshots dir missing: %v", err)
	}
	got, err := os.ReadFile(f.Bundle)
	if err != nil {
		t.Fatalf("read copied bundle: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("copied bundle = %q, want identical bytes", got)
	}
	if filepath.Base(f.Bundle) != BundleName {
		t.Errorf("bundle name = %q, want %q", filepath.Base(f.Bundle), BundleName)
	}
	if len(f.Screenshots) != 2 {
		t.Fatalf("copied %d screenshots, want 2 (the .txt is not an image): %v", len(f.Screenshots), f.Screenshots)
	}
	for _, s := range f.Screenshots {
		if _, err := os.Stat(s); err != nil {
			t.Errorf("screenshot %q not on disk: %v", s, err)
		}
	}
}

func TestAssemble_MissingScreenshotsDirIsFine(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(t.TempDir(), "app-release.aab")
	if err := os.WriteFile(bundle, []byte("aab"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := Assemble(root, release.Release{Name: "1.0.5"}, bundle, filepath.Join(root, "nope"))
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if len(f.Screenshots) != 0 {
		t.Errorf("Screenshots = %v, want none", f.Screenshots)
	}
}

func TestAssemble_MissingBundleIsAnError(t *testing.T) {
	root := t.TempDir()
	if _, err := Assemble(root, release.Release{Name: "1.0.5"}, filepath.Join(root, "gone.aab"), ""); err == nil {
		t.Fatal("Assemble with a missing bundle = nil error, want one")
	}
}

func TestFolder_WriteFile(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(t.TempDir(), "app-release.aab")
	if err := os.WriteFile(bundle, []byte("aab"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Assemble(root, release.Release{Name: "1.0.5"}, bundle, "")
	if err != nil {
		t.Fatal(err)
	}
	path, err := f.WriteFile("release-notes-technical.md", "# notes\n")
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "# notes\n" {
		t.Errorf("written file = (%q, %v)", raw, err)
	}
}
