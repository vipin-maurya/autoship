package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BundleGlob is where the Android Gradle plugin writes release bundles,
// relative to the module directory.
const BundleGlob = "build/outputs/bundle/release/*.aab"

// ModuleDir turns a Gradle module path (":app", ":features:core") into the
// directory it lives in, under repoDir.
func ModuleDir(repoDir, module string) string {
	rel := strings.ReplaceAll(strings.TrimPrefix(module, ":"), ":", string(filepath.Separator))
	if rel == "" {
		return repoDir
	}
	return filepath.Join(repoDir, rel)
}

// FindBundle returns the release .aab in moduleDir, choosing the most recently
// modified one when a stale bundle from an earlier build is still present.
func FindBundle(moduleDir string) (string, error) {
	pattern := filepath.Join(moduleDir, filepath.FromSlash(BundleGlob))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("search for the release bundle: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no .aab found in %s", filepath.Dir(pattern))
	}

	newest := ""
	var newestMod int64
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if mod := info.ModTime().UnixNano(); newest == "" || mod > newestMod {
			newest, newestMod = m, mod
		}
	}
	if newest == "" {
		return "", fmt.Errorf("no readable .aab found in %s", filepath.Dir(pattern))
	}
	return newest, nil
}
