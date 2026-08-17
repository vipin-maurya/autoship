// Package secrets stores credentials encrypted at rest, scoped to the current
// Windows user. A scheduled task runs non-interactively with a stored
// credential, so neither the Play service-account key nor the keystore
// passwords may sit in plaintext anywhere (spec §8).
package secrets

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Well-known secret names.
const (
	// PlayServiceAccount is the Play publisher service-account JSON key.
	PlayServiceAccount = "play_sa"
	// KeystorePassword is the release keystore password.
	KeystorePassword = "keystore_password"
	// KeyAliasPassword is the signing key alias password.
	KeyAliasPassword = "key_alias_password"
)

// ErrUnsupported reports that this platform has no user-scoped encryption
// available.
var ErrUnsupported = errors.New("encrypted secret storage is only available on windows")

// ErrNotFound reports that no secret is stored under that name.
var ErrNotFound = errors.New("secret not found")

// Store keeps encrypted blobs in Dir, one file per secret.
type Store struct {
	Dir string
}

func (s Store) path(name string) (string, error) {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return "", errors.New("secret name is empty")
	}
	if strings.ContainsAny(clean, `\/:*?"<>|`) {
		return "", fmt.Errorf("secret name %q contains a path separator", name)
	}
	return filepath.Join(s.Dir, clean+".bin"), nil
}

// Set encrypts data and writes it under name, replacing any previous value.
func (s Store) Set(name string, data []byte) error {
	path, err := s.path(name)
	if err != nil {
		return err
	}
	blob, err := protect(data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("create secrets dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return fmt.Errorf("write secret: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit secret: %w", err)
	}
	return nil
}

// Get decrypts the secret stored under name.
func (s Store) Get(name string) ([]byte, error) {
	path, err := s.path(name)
	if err != nil {
		return nil, err
	}
	blob, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return nil, fmt.Errorf("read secret: %w", err)
	}
	return unprotect(blob)
}

// Has reports whether a secret is stored under name.
func (s Store) Has(name string) bool {
	path, err := s.path(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// List returns the stored secret names, sorted by the filesystem's order.
func (s Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read secrets dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".bin" {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".bin"))
	}
	return names, nil
}

// Delete removes a stored secret.
func (s Store) Delete(name string) error {
	path, err := s.path(name)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return err
}
