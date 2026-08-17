package notes

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxLen is the Play Console limit on a release-notes body, in characters.
const MaxLen = 500

// Validate rejects copy Play would refuse, counting characters the way Play
// does — runes, not bytes.
func Validate(s string) error {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return fmt.Errorf("release notes are empty")
	}
	if n := utf8.RuneCountInString(trimmed); n > MaxLen {
		return fmt.Errorf("release notes are %d characters, over the Play limit of %d", n, MaxLen)
	}
	return nil
}
