//go:build !windows

package secrets

// protect and unprotect exist so the whole tool still builds and tests on
// non-Windows machines; they deliberately refuse rather than falling back to
// something weaker than DPAPI (plan A4).
func protect([]byte) ([]byte, error) { return nil, ErrUnsupported }

func unprotect([]byte) ([]byte, error) { return nil, ErrUnsupported }
