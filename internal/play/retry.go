package play

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"google.golang.org/api/googleapi"
)

// Retry policy for the Play API (spec §10): 5xx and network failures are worth
// retrying, 4xx never is — a rejected bundle stays rejected.
const (
	maxAttempts  = 3
	baseBackoff  = 2 * time.Second
	backoffScale = 2
)

// sleeper is swapped out in tests so retries do not really wait.
var sleeper = func(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// withRetry runs fn up to maxAttempts times, backing off exponentially between
// retryable failures.
func withRetry(ctx context.Context, what string, fn func() error) error {
	var err error
	backoff := baseBackoff
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !retryable(err) || attempt == maxAttempts {
			break
		}
		if waitErr := sleeper(ctx, backoff); waitErr != nil {
			return fmt.Errorf("%s: %w", what, waitErr)
		}
		backoff *= backoffScale
	}
	return fmt.Errorf("%s: %w", what, err)
}

// retryable reports whether an error is worth another attempt.
func retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.Code >= 500:
			return true
		case apiErr.Code == http.StatusTooManyRequests:
			return true
		default:
			return false
		}
	}

	// Anything that never reached Play — DNS, dial, reset, timeout — is the
	// network being the network.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return false
}
