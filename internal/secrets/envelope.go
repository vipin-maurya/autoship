package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
)

// masterKeySize is 32 bytes for AES-256.
const masterKeySize = 32

// macOS and Linux have no "encrypt this arbitrary blob for the current user"
// primitive the way Windows DPAPI does. Both platforms' backends instead ask
// their native secret store (Keychain, Secret Service) to hold one small,
// fixed-size AES key and use it to seal the actual secret with AES-256-GCM.
// The stored .bin file is the ciphertext; the OS never sees it.

// newMasterKey generates a random AES-256 key.
func newMasterKey() ([]byte, error) {
	key := make([]byte, masterKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate master key: %w", err)
	}
	return key, nil
}

// sealWithKey encrypts plaintext with key, prefixing the result with the
// nonce it generated.
func sealWithKey(key, plaintext []byte) ([]byte, error) {
	gcm, err := gcmFor(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// openWithKey reverses sealWithKey.
func openWithKey(key, blob []byte) ([]byte, error) {
	gcm, err := gcmFor(key)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, fmt.Errorf("stored secret is truncated")
	}
	nonce, ciphertext := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	return plaintext, nil
}

func gcmFor(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("init cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("init gcm: %w", err)
	}
	return gcm, nil
}
