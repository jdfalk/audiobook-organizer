// file: internal/ai/retry.go
// version: 1.0.0
// guid: f6a7b8c9-d0e1-2345-fabc-678901234567
// last-edited: 2026-05-01

package ai

import (
	"context"
	"time"
)

// withRetry calls fn up to maxRetries+1 times with quadratic backoff.
// Backoff formula: attempt^2 * 2 seconds (attempt 1: 2s, attempt 2: 8s, attempt 3: 18s, etc).
// Returns result on first success. Respects ctx cancellation.
// Generic return type T supports callers with different result types.
func withRetry[T any](ctx context.Context, maxRetries int, fn func() (T, error)) (T, error) {
	var lastErr error
	var zero T
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * 2 * time.Second
			select {
			case <-ctx.Done():
				return zero, ctx.Err()
			case <-time.After(backoff):
			}
		}
		result, err := fn()
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return zero, lastErr
}
