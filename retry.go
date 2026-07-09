package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"time"

	"google.golang.org/api/googleapi"
)

const (
	// maxRetryAttempts is the number of retries after the initial call.
	// Back-off schedule: ~1 s, ~2 s, ~4 s, ~8 s, ~16 s.
	maxRetryAttempts = 5

	// baseRetryDelay is the base wait for the first retry.
	baseRetryDelay = time.Second

	// sheetsCallDelay is a best-effort pause inserted between consecutive
	// Sheets API BatchUpdate calls to reduce burstiness and lower the chance of
	// hitting the 60 writes/minute/user quota during normal operation.
	sheetsCallDelay = 250 * time.Millisecond
)

// withRetry calls fn and retries on HTTP 429 (Too Many Requests) using
// truncated exponential back-off with full jitter, as recommended by the
// Google Sheets API documentation. Any other error is returned immediately
// without retrying.
//
// fn must be safe to call more than once. Google 429s are issued before the
// request is processed, so retrying BatchUpdate and AddSheet calls is safe.
//
// When verbose is true, each retry attempt is logged to stderr.
// The context is checked before every sleep so that cancellation is honoured.
func withRetry(ctx context.Context, verbose bool, fn func() error) error {
	for attempt := range maxRetryAttempts + 1 {
		err := fn()
		if err == nil {
			return nil
		}
		if !isRateLimitErr(err) || attempt == maxRetryAttempts {
			return err
		}
		delay := backoffDelayFn(attempt)
		if verbose {
			fmt.Fprintf(os.Stderr,
				"Sheets API rate limit (429); retrying in %v (attempt %d/%d)…\n",
				delay.Round(time.Millisecond), attempt+1, maxRetryAttempts)
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		}
	}
	panic("unreachable") // loop always returns before this point
}

// isRateLimitErr reports whether err is an HTTP 429 response from a Google API.
func isRateLimitErr(err error) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == http.StatusTooManyRequests
}

// backoffDelayFn computes the wait duration for a retry attempt.
// Tests replace this with a zero-duration stub to avoid real sleeps.
var backoffDelayFn = backoffDelay

// backoffDelay returns the wait duration for a given retry attempt (0-indexed).
// It uses full-jitter exponential back-off: a random value in
// [0, min(baseRetryDelay<<attempt, 32s)). The mean grows with each attempt
// while avoiding thundering-herd synchronisation across concurrent callers.
func backoffDelay(attempt int) time.Duration {
	maxWait := baseRetryDelay << attempt // 1 s, 2 s, 4 s, 8 s, 16 s, …
	if maxWait > 32*time.Second {
		maxWait = 32 * time.Second
	}
	return time.Duration(rand.Int64N(int64(maxWait)))
}
