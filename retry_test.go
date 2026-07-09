package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
)

// apiErr returns a *googleapi.Error with the given HTTP status code.
func apiErr(code int) error {
	return &googleapi.Error{Code: code}
}

func TestIsRateLimitErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"rate_limit_429", apiErr(429), true},
		{"other_api_error", apiErr(500), false},
		{"non_api_error", errors.New("oops"), false},
		{"nil", nil, false},
		{"wrapped_429", fmt.Errorf("op failed: %w", apiErr(429)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isRateLimitErr(tt.err))
		})
	}
}

// zeroDelay overrides backoffDelayFn for the duration of a test so that
// withRetry returns immediately rather than sleeping. Tests that need a
// non-zero delay to distinguish context cancellation should set their own.
func zeroDelay(t *testing.T) {
	t.Helper()
	orig := backoffDelayFn
	backoffDelayFn = func(int) time.Duration { return 0 }
	t.Cleanup(func() { backoffDelayFn = orig })
}

func TestWithRetry_success(t *testing.T) {
	calls := 0
	err := withRetry(context.Background(), false, func() error {
		calls++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestWithRetry_success_after_retries(t *testing.T) {
	zeroDelay(t)

	// fn returns 429 twice, then succeeds.
	calls := 0
	err := withRetry(context.Background(), false, func() error {
		calls++
		if calls < 3 {
			return apiErr(429)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestWithRetry_non_rate_limit_error(t *testing.T) {
	// Non-429 errors must be returned immediately without retrying.
	calls := 0
	sentinel := apiErr(500)
	err := withRetry(context.Background(), false, func() error {
		calls++
		return sentinel
	})
	assert.Equal(t, sentinel, err)
	assert.Equal(t, 1, calls, "should not retry on non-429 errors")
}

func TestWithRetry_exhausts_retries(t *testing.T) {
	zeroDelay(t)

	// fn always returns 429 — withRetry should give up after maxRetryAttempts+1 calls.
	calls := 0
	rateErr := apiErr(429)
	err := withRetry(context.Background(), false, func() error {
		calls++
		return rateErr
	})
	assert.Equal(t, rateErr, err)
	assert.Equal(t, maxRetryAttempts+1, calls)
}

func TestWithRetry_context_cancelled(t *testing.T) {
	// Use a long delay so that only ctx.Done fires in the select, not the timer.
	orig := backoffDelayFn
	backoffDelayFn = func(int) time.Duration { return time.Minute }
	t.Cleanup(func() { backoffDelayFn = orig })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := withRetry(ctx, false, func() error {
		return apiErr(429)
	})
	assert.Equal(t, context.Canceled, err)
}

func TestWithRetry_verbose_logs_to_stderr(t *testing.T) {
	zeroDelay(t)

	// Smoke-test that verbose=true doesn't panic or error; actual stderr output
	// is not captured here since the function's correctness does not depend on it.
	calls := 0
	err := withRetry(context.Background(), true, func() error {
		calls++
		if calls < 2 {
			return apiErr(429)
		}
		return nil
	})
	require.NoError(t, err)
}

func TestBackoffDelay_bounds(t *testing.T) {
	// For each attempt the delay must be in [0, maxWait) where
	// maxWait = baseRetryDelay<<attempt (capped at 32 s).
	// Run many iterations to gain confidence in the random range.
	cases := []struct {
		attempt int
		maxWait time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{6, 32 * time.Second}, // above the shift cap; must not exceed 32 s
	}
	for _, tc := range cases {
		for i := 0; i < 200; i++ {
			d := backoffDelay(tc.attempt)
			assert.GreaterOrEqual(t, d, time.Duration(0), "attempt %d", tc.attempt)
			assert.Less(t, d, tc.maxWait, "attempt %d", tc.attempt)
		}
	}
}
