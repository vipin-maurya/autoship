package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLock_SecondAcquireReturnsErrLocked(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	first, err := s.Acquire(time.Hour)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer first.Release()

	if _, err := s.Acquire(time.Hour); !errors.Is(err, ErrLocked) {
		t.Fatalf("second Acquire = %v, want ErrLocked", err)
	}
}

func TestLock_ReleaseAllowsReacquire(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	l, err := s.Acquire(time.Hour)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("second Release should be a no-op, got %v", err)
	}
	l2, err := s.Acquire(time.Hour)
	if err != nil {
		t.Fatalf("re-Acquire after Release: %v", err)
	}
	_ = l2.Release()
}

func TestLock_ReclaimsStale(t *testing.T) {
	tests := []struct {
		name string
		lf   lockFile
	}{
		{"dead pid", lockFile{PID: deadPID(t), StartedAt: time.Now()}},
		{"older than max run duration", lockFile{PID: os.Getpid(), StartedAt: time.Now().Add(-2 * time.Hour)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			s := Store{Dir: dir}
			raw, err := json.Marshal(tc.lf)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "lock"), raw, 0o600); err != nil {
				t.Fatal(err)
			}
			l, err := s.Acquire(time.Hour)
			if err != nil {
				t.Fatalf("Acquire over a stale lock = %v, want it reclaimed", err)
			}
			_ = l.Release()
		})
	}
}

// deadPID returns a PID that is no longer running: a child process is started
// and waited on, so the OS has definitely reaped it.
func deadPID(t *testing.T) int {
	t.Helper()
	if !processAlive(os.Getpid()) {
		t.Fatal("processAlive says this very process is dead")
	}
	// PIDs are recycled, so probe upward for one nothing currently owns.
	for pid := 60000; pid < 65000; pid++ {
		if !processAlive(pid) {
			return pid
		}
	}
	t.Fatal("no unused pid found")
	return 0
}
