//go:build windows

package secrets

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// description is stored alongside the blob by DPAPI and is only informational.
const description = "autoship credential"

// protect encrypts data with CryptProtectData, scoped to the current user, so
// the blob is useless to any other account on the machine.
func protect(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("refusing to store an empty secret")
	}
	in := blobOf(data)
	var out windows.DataBlob
	desc, err := windows.UTF16PtrFromString(description)
	if err != nil {
		return nil, err
	}
	if err := windows.CryptProtectData(&in, desc, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("encrypt secret: %w", err)
	}
	defer freeBlob(&out)
	return copyBlob(&out), nil
}

// unprotect reverses protect. It fails for any user other than the one that
// wrote the blob, which is the point.
func unprotect(blob []byte) ([]byte, error) {
	if len(blob) == 0 {
		return nil, fmt.Errorf("stored secret is empty")
	}
	in := blobOf(blob)
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("decrypt secret: %w", err)
	}
	defer freeBlob(&out)
	return copyBlob(&out), nil
}

func blobOf(b []byte) windows.DataBlob {
	return windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

// copyBlob copies out of the API-allocated buffer into Go memory.
func copyBlob(b *windows.DataBlob) []byte {
	out := make([]byte, b.Size)
	copy(out, unsafe.Slice(b.Data, b.Size))
	return out
}

func freeBlob(b *windows.DataBlob) {
	if b.Data != nil {
		_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(b.Data)))
	}
}
