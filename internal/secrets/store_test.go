package secrets

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// skipIfUnsupported probes the platform's secret store the same way Set
// would. Every supported OS (windows, darwin, linux) is expected to have one
// wired up in CI; a bare ErrUnsupported (e.g. secret-tool missing locally)
// skips rather than fails, but any other error is a real bug.
func skipIfUnsupported(t *testing.T) {
	t.Helper()
	if _, err := protect([]byte("probe")); err != nil {
		if errors.Is(err, ErrUnsupported) {
			t.Skipf("no secret store available on %s: %v", runtime.GOOS, err)
		}
		t.Fatalf("protect probe failed unexpectedly on %s: %v", runtime.GOOS, err)
	}
}

func TestStore_RoundTrip(t *testing.T) {
	skipIfUnsupported(t)
	dir := t.TempDir()
	s := Store{Dir: dir}

	secret := []byte(`{"type":"service_account","private_key":"-----BEGIN PRIVATE KEY-----"}`)
	if err := s.Set(PlayServiceAccount, secret); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(PlayServiceAccount)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Errorf("Get = %q, want the original bytes", got)
	}

	// The point of the exercise: the file on disk is not the secret.
	raw, err := os.ReadFile(filepath.Join(dir, PlayServiceAccount+".bin"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("private_key")) || bytes.Contains(raw, secret) {
		t.Error("the stored blob contains the plaintext secret")
	}
}

func TestStore_GetMissing(t *testing.T) {
	skipIfUnsupported(t)
	s := Store{Dir: t.TempDir()}
	if _, err := s.Get("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get = %v, want ErrNotFound", err)
	}
}

func TestStore_ListAndDelete(t *testing.T) {
	skipIfUnsupported(t)
	s := Store{Dir: t.TempDir()}
	if err := s.Set(KeystorePassword, []byte("hunter2")); err != nil {
		t.Fatal(err)
	}
	if !s.Has(KeystorePassword) {
		t.Error("Has = false after Set")
	}
	names, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != KeystorePassword {
		t.Errorf("List = %v, want [%s]", names, KeystorePassword)
	}
	if err := s.Delete(KeystorePassword); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if s.Has(KeystorePassword) {
		t.Error("Has = true after Delete")
	}
	if err := s.Delete(KeystorePassword); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete = %v, want ErrNotFound", err)
	}
}

func TestStore_RejectsBadNames(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	for _, name := range []string{"", "  ", `..\escape`, "a/b"} {
		if err := s.Set(name, []byte("x")); err == nil {
			t.Errorf("Set(%q) = nil error, want one", name)
		}
	}
}

func TestStore_ListOnMissingDir(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "absent")}
	names, err := s.List()
	if err != nil {
		t.Fatalf("List on a missing dir = %v, want nil", err)
	}
	if len(names) != 0 {
		t.Errorf("List = %v, want empty", names)
	}
}
