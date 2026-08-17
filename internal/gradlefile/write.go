package gradlefile

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
)

// Bump rewrites the versionName and versionCode in place, touching nothing
// else in the file — the rest of build.gradle.kts is the app's, not ours.
func Bump(path string, next Version) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat gradle file: %w", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read gradle file: %w", err)
	}
	if !nameRe.Match(raw) {
		return fmt.Errorf("%s: no versionName found", path)
	}
	if !codeRe.Match(raw) {
		return fmt.Errorf("%s: no versionCode found", path)
	}

	out := replaceFirst(raw, nameRe, []byte(`versionName = "`+next.Name+`"`))
	out = replaceFirst(out, codeRe, []byte("versionCode = "+strconv.Itoa(next.Code)))

	if err := os.WriteFile(path, out, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write gradle file: %w", err)
	}
	return nil
}

// NextSnapshot returns the version to develop against once the current one has
// shipped: the next version name carrying -SNAPSHOT, and the next versionCode.
func NextSnapshot(current Version, nextName string) Version {
	return Version{Code: current.Code + 1, Name: nextName + snapshotSuffix}
}

// replaceFirst substitutes only the first match, leaving later occurrences
// (build variants, comments) untouched.
func replaceFirst(src []byte, re *regexp.Regexp, repl []byte) []byte {
	loc := re.FindIndex(src)
	if loc == nil {
		return src
	}
	out := make([]byte, 0, len(src)-(loc[1]-loc[0])+len(repl))
	out = append(out, src[:loc[0]]...)
	out = append(out, repl...)
	out = append(out, src[loc[1]:]...)
	return out
}
