// Package artifacts assembles the versioned release folder that the manual
// releasing-app skill produces by hand.
package artifacts

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vipinm/autoship/internal/release"
)

// BundleName is what the copied .aab is called inside the release folder.
const BundleName = "app-release.aab"

// Folder is an assembled release folder.
type Folder struct {
	// Dir is <root>/v<version>.
	Dir string
	// Bundle is the copied .aab inside Dir.
	Bundle string
	// Screenshots are the copied screenshot files, sorted by name.
	Screenshots []string
}

// ScreenshotsDir is where uploaded images live inside the folder.
func (f Folder) ScreenshotsDir() string { return filepath.Join(f.Dir, "screenshots") }

// Assemble creates <root>/v<version>/ with the bundle and a screenshots
// subdirectory, copying in whatever screenshotsFrom holds. Screenshots are
// consumed, never generated (spec §7.2); an empty or missing source directory
// is not an error, it just means nothing to upload.
func Assemble(root string, rel release.Release, bundlePath, screenshotsFrom string) (Folder, error) {
	dir := filepath.Join(root, "v"+rel.Name)
	shots := filepath.Join(dir, "screenshots")
	if err := os.MkdirAll(shots, 0o755); err != nil {
		return Folder{}, fmt.Errorf("create release folder: %w", err)
	}

	f := Folder{Dir: dir, Bundle: filepath.Join(dir, BundleName)}
	if err := copyFile(bundlePath, f.Bundle); err != nil {
		return Folder{}, fmt.Errorf("copy bundle: %w", err)
	}

	copied, err := copyScreenshots(screenshotsFrom, shots)
	if err != nil {
		return Folder{}, err
	}
	f.Screenshots = copied
	return f, nil
}

// WriteFile writes a text file into the release folder, e.g. the two sets of
// release notes.
func (f Folder) WriteFile(name, content string) (string, error) {
	path := filepath.Join(f.Dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", name, err)
	}
	return path, nil
}

var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}

func copyScreenshots(from, to string) ([]string, error) {
	if strings.TrimSpace(from) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(from)
	if errors.Is(err, fs.ErrNotExist) {
		// Screenshots are optional input, so their absence is a fact about
		// this release, not a failure.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read screenshots dir: %w", err)
	}

	var copied []string
	for _, e := range entries {
		if e.IsDir() || !imageExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		dst := filepath.Join(to, e.Name())
		if err := copyFile(filepath.Join(from, e.Name()), dst); err != nil {
			return nil, fmt.Errorf("copy screenshot %s: %w", e.Name(), err)
		}
		copied = append(copied, dst)
	}
	sort.Strings(copied)
	return copied, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
