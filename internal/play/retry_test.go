package play

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/api/googleapi"
)

// noSleep replaces the backoff for the duration of a test.
func noSleep(t *testing.T) {
	t.Helper()
	orig := sleeper
	sleeper = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { sleeper = orig })
}

func TestRetry_RetriesServerErrors(t *testing.T) {
	noSleep(t)
	attempts := 0
	err := withRetry(context.Background(), "upload", func() error {
		attempts++
		if attempts < 3 {
			return &googleapi.Error{Code: 503, Message: "backend error"}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withRetry = %v, want nil", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestRetry_DoesNotRetryClientErrors(t *testing.T) {
	noSleep(t)
	attempts := 0
	err := withRetry(context.Background(), "upload", func() error {
		attempts++
		return &googleapi.Error{Code: 400, Message: "bad bundle"}
	})
	if err == nil {
		t.Fatal("withRetry = nil error, want the 400 returned")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

func TestRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	noSleep(t)
	attempts := 0
	err := withRetry(context.Background(), "upload", func() error {
		attempts++
		return &googleapi.Error{Code: 500}
	})
	if err == nil {
		t.Fatal("withRetry = nil error, want a failure")
	}
	if attempts != maxAttempts {
		t.Errorf("attempts = %d, want %d", attempts, maxAttempts)
	}
}

func TestRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"503", &googleapi.Error{Code: 503}, true},
		{"429", &googleapi.Error{Code: 429}, true},
		{"400", &googleapi.Error{Code: 400}, false},
		{"403", &googleapi.Error{Code: 403}, false},
		{"409 edit conflict", &googleapi.Error{Code: 409}, false},
		{"network", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, true},
		{"context cancelled", context.Canceled, false},
		{"plain", errors.New("nope"), false},
		{"nil", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryable(tc.err); got != tc.want {
				t.Errorf("retryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestRetry_StopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	attempts := 0
	err := withRetry(ctx, "upload", func() error {
		attempts++
		return &googleapi.Error{Code: 503}
	})
	if err == nil {
		t.Fatal("withRetry = nil error, want a failure")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 before the cancelled context stopped it", attempts)
	}
}
