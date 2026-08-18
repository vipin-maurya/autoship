//go:build darwin

package secrets

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
)

// The macOS login Keychain holds the master key, scoped to the current user
// exactly like DPAPI is: it unlocks automatically when the user logs in and
// stays unavailable otherwise, which matches the interactive-session
// scheduling model autoship already needs for Gradle and the Android SDK
// cache (plan A4).
const (
	keychainService = "autoship"
	keychainAccount = "master-key"
)

func protect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("refusing to store an empty secret")
	}
	key, err := macKeychainMasterKey()
	if err != nil {
		return nil, err
	}
	return sealWithKey(key, data)
}

func unprotect(blob []byte) ([]byte, error) {
	if len(blob) == 0 {
		return nil, fmt.Errorf("stored secret is empty")
	}
	key, err := macKeychainMasterKey()
	if err != nil {
		return nil, err
	}
	return openWithKey(key, blob)
}

// macKeychainMasterKey fetches the AES key from the login Keychain, creating
// one on first use via the `security` CLI (built into every macOS install,
// so this adds no dependency).
func macKeychainMasterKey() ([]byte, error) {
	find := exec.Command("security", "find-generic-password",
		"-s", keychainService, "-a", keychainAccount, "-w")
	if out, err := find.Output(); err == nil {
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
	add := exec.Command("security", "add-generic-password",
		"-s", keychainService, "-a", keychainAccount,
		"-w", hex.EncodeToString(key), "-U")
	if out, err := add.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("create keychain entry for the master key (is the login keychain unlocked?): %w: %s",
			err, bytes.TrimSpace(out))
	}
	return key, nil
}
