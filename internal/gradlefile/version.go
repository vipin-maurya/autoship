// Package gradlefile reads and writes the versionName/versionCode pair in an
// Android module's build.gradle.kts. The repo is the source of truth for
// versions; this package never invents one (spec R6).
package gradlefile

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// snapshotSuffix marks a version that is in development, not released.
const snapshotSuffix = "-SNAPSHOT"

var (
	nameRe = regexp.MustCompile(`versionName\s*=\s*"([^"]+)"`)
	codeRe = regexp.MustCompile(`versionCode\s*=\s*(\d+)`)
)

// Version is the pair Android identifies a build by.
type Version struct {
	Code int
	Name string
}

func (v Version) String() string { return fmt.Sprintf("%s (%d)", v.Name, v.Code) }

// ReleaseName is the version name with the development suffix removed:
// "1.0.5-SNAPSHOT" ships as "1.0.5". Any other suffix is left alone, since only
// -SNAPSHOT means "not yet released".
func (v Version) ReleaseName() string {
	return strings.TrimSuffix(v.Name, snapshotSuffix)
}

// IsSnapshot reports whether the version is still a development snapshot.
func (v Version) IsSnapshot() bool { return strings.HasSuffix(v.Name, snapshotSuffix) }

// Parse reads the first versionName and versionCode declared in a Gradle file.
func Parse(path string) (Version, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Version{}, fmt.Errorf("read gradle file: %w", err)
	}
	return parseBytes(raw, path)
}

func parseBytes(raw []byte, path string) (Version, error) {
	var v Version

	m := nameRe.FindSubmatch(raw)
	if m == nil {
		return Version{}, fmt.Errorf("%s: no versionName found", path)
	}
	v.Name = string(m[1])

	m = codeRe.FindSubmatch(raw)
	if m == nil {
		return Version{}, fmt.Errorf("%s: no versionCode found", path)
	}
	code, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return Version{}, fmt.Errorf("%s: versionCode %q is not a number: %w", path, m[1], err)
	}
	v.Code = code

	return v, nil
}
