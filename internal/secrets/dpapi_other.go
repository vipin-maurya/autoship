//go:build !windows && !darwin && !linux

package secrets

// protect and unprotect exist so the whole tool still builds and tests on
// platforms with no supported secret store (Windows has DPAPI, macOS has
// Keychain, Linux has the Secret Service — see dpapi_windows.go,
// dpapi_darwin.go, dpapi_linux.go); they deliberately refuse rather than
// falling back to something weaker (plan A4).
func protect([]byte) ([]byte, error) { return nil, ErrUnsupported }

func unprotect([]byte) ([]byte, error) { return nil, ErrUnsupported }
