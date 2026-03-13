// file: internal/metadata/circuitbreaker_test.go
// version: 1.0.0
// guid: f3a4b5c6-d7e8-9012-fa34-567890abcdef

package metadata

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCircuitBreaker_ClosedAllowsRequests(t *testing.T) {
	cb := NewCircuitBreaker("test", 3, time.Second)
	assert.NoError(t, cb.AllowRequest())
	assert.Equal(t, "closed", cb.StateName())
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker("test", 3, time.Second)

	cb.RecordFailure()
	cb.RecordFailure()
	assert.NoError(t, cb.AllowRequest(), "should still allow before threshold")

	cb.RecordFailure()
	assert.ErrorIs(t, cb.AllowRequest(), ErrCircuitOpen)
	assert.Equal(t, "open", cb.StateName())
}

func TestCircuitBreaker_ResetsOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker("test", 2, time.Second)

	cb.RecordFailure()
	cb.RecordSuccess()
	assert.Equal(t, 0, cb.Failures())
	assert.Equal(t, "closed", cb.StateName())
}

func TestCircuitBreaker_HalfOpenAfterCooldown(t *testing.T) {
	cb := NewCircuitBreaker("test", 1, 10*time.Millisecond)

	cb.RecordFailure()
	assert.ErrorIs(t, cb.AllowRequest(), ErrCircuitOpen)

	time.Sleep(20 * time.Millisecond)
	assert.NoError(t, cb.AllowRequest(), "should allow probe after cooldown")
	assert.Equal(t, "half-open", cb.StateName())

	// Second request while half-open should be rejected
	assert.ErrorIs(t, cb.AllowRequest(), ErrCircuitOpen)
}

func TestCircuitBreaker_HalfOpenSuccessCloses(t *testing.T) {
	cb := NewCircuitBreaker("test", 1, 10*time.Millisecond)

	cb.RecordFailure()
	time.Sleep(20 * time.Millisecond)
	_ = cb.AllowRequest() // transitions to half-open

	cb.RecordSuccess()
	assert.Equal(t, "closed", cb.StateName())
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker("test", 1, 10*time.Millisecond)

	cb.RecordFailure()
	time.Sleep(20 * time.Millisecond)
	_ = cb.AllowRequest() // transitions to half-open

	cb.RecordFailure()
	assert.Equal(t, "open", cb.StateName())
}
