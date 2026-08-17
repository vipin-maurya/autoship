package runner

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRun_CapturesOutputAndExitCode(t *testing.T) {
	var out bytes.Buffer
	r := ExecRunner{Out: &out}

	code, err := r.Run(context.Background(), "", "go", "env", "GOOS")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(out.String()) != runtime.GOOS {
		t.Errorf("output = %q, want %q", out.String(), runtime.GOOS)
	}
}

func TestRun_ReportsNonZeroExit(t *testing.T) {
	r := ExecRunner{}
	code, err := r.Run(context.Background(), "", "go", "run", "no-such-package-xyz")
	if err == nil {
		t.Fatal("Run = nil error, want a failure")
	}
	if code == 0 {
		t.Errorf("exit code = 0, want non-zero")
	}
}

func TestRun_UnknownCommandIsAnError(t *testing.T) {
	r := ExecRunner{}
	if _, err := r.Run(context.Background(), "", "definitely-not-a-real-binary-xyz"); err == nil {
		t.Fatal("Run = nil error, want a start failure")
	}
}

func TestRun_TimeoutCancels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	// `go run` on a sleeping program is portable and reliably outlives 50ms.
	_, err := ExecRunner{}.Run(ctx, "", "go", "run", "testdata/sleep.go")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Errorf("cancellation took %v, want it to be prompt", elapsed)
	}
}

func TestSpy_RecordsCalls(t *testing.T) {
	s := &Spy{ExitFor: FailOnArg("boom", 1)}
	if _, err := s.Run(context.Background(), "dir", "gradlew", "clean"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	code, err := s.Run(context.Background(), "dir", "gradlew", "boom")
	if err == nil || code != 1 {
		t.Fatalf("Run = (%d, %v), want (1, error)", code, err)
	}
	if s.Count() != 2 {
		t.Fatalf("Count = %d, want 2", s.Count())
	}
	if got := s.Calls()[0].String(); got != "gradlew clean" {
		t.Errorf("first call = %q", got)
	}
}
