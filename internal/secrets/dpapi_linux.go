//go:build linux

package secrets

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
)

// Linux has no single built-in secret store, so this shells out to
// `secret-tool` (libsecret), the standard CLI for the Secret Service D-Bus
// API that gnome-keyring and kwallet both implement. Like the macOS backend,
// only a small master key is kept there; the actual secret is AES-256-GCM
// sealed with it and written to the .bin file (plan A4).
const (
	secretToolService = "autoship"
	secretToolKeyAttr = "master-key"
)

func protect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("refusing to store an empty secret")
	}
	key, err := secretServiceMasterKey()
	if err != nil {
		return nil, err
	}
	return sealWithKey(key, data)
}

func unprotect(blob []byte) ([]byte, error) {
	if len(blob) == 0 {
		return nil, fmt.Errorf("stored secret is empty")
	}
	key, err := secretServiceMasterKey()
	if err != nil {
		return nil, err
	}
	return openWithKey(key, blob)
}

// secretServiceMasterKey fetches the AES key from the Secret Service,
// creating one on first use.
func secretServiceMasterKey() ([]byte, error) {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return nil, fmt.Errorf("%w: secret-tool not found — install libsecret-tools "+
			"(Debian/Ubuntu: apt install libsecret-tools; Fedora: dnf install libsecret; "+
			"Arch: pacman -S libsecret) and make sure a Secret Service provider "+
			"(gnome-keyring or kwallet) is running", ErrUnsupported)
	}

	lookup := exec.Command("secret-tool", "lookup",
		"service", secretToolService, "key", secretToolKeyAttr)
	if out, err := lookup.Output(); err == nil {
		key, decodeErr := hex.DecodeString(strings.TrimSpace(string(out)))
		if decodeErr != nil {
			return nil, fmt.Errorf("stored master key is corrupt: %w", decodeErr)
		}
		return key, nil
	}

	key, err := newMasterKey()
	if err != nil {
		return nil, err
	}
	store := exec.Command("secret-tool", "store", "--label=autoship master key",
		"service", secretToolService, "key", secretToolKeyAttr)
	store.Stdin = bytes.NewReader([]byte(hex.EncodeToString(key)))
	if out, err := store.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("store master key (is a Secret Service provider running — "+
			"gnome-keyring-daemon or kwallet?): %w: %s", err, bytes.TrimSpace(out))
	}
	return key, nil
}
