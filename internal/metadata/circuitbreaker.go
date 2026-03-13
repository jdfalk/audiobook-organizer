// file: internal/metadata/circuitbreaker.go
// version: 1.0.0
// guid: e2f3a4b5-c6d7-8901-ef23-456789abcdef

package metadata

import (
	"errors"
	"log"
	"sync"
	"time"
)

// ErrCircuitOpen is returned when the circuit breaker is open and calls are rejected.
var ErrCircuitOpen = errors.New("circuit breaker open: external metadata source is unavailable")

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	StateClosed   CircuitState = iota // Normal operation
	StateOpen                        // Failing fast
	StateHalfOpen                    // Allowing one probe
)

// CircuitBreaker prevents cascading failures by stopping calls to a failing external service.
// It is safe for concurrent use.
type CircuitBreaker struct {
	mu           sync.Mutex
	state        CircuitState
	failures     int
	threshold    int           // Number of failures before opening (default: 5)
	cooldown     time.Duration // How long to stay open before allowing a probe
	lastFailedAt time.Time
	sourceName   string
}

// NewCircuitBreaker creates a breaker with the given threshold and cooldown.
func NewCircuitBreaker(sourceName string, threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:      StateClosed,
		threshold:  threshold,
		cooldown:   cooldown,
		sourceName: sourceName,
	}
}

// AllowRequest returns nil if the caller should proceed with the network call.
// Returns ErrCircuitOpen if the circuit is open and the cooldown has not elapsed.
func (cb *CircuitBreaker) AllowRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return nil
	case StateOpen:
		if time.Since(cb.lastFailedAt) > cb.cooldown {
			cb.state = StateHalfOpen
			return nil
		}
		return ErrCircuitOpen
	case StateHalfOpen:
		return ErrCircuitOpen
	}
	return nil
}

// RecordSuccess resets the breaker to closed state.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.failures = 0
}

// RecordFailure increments the failure counter and opens the circuit if threshold reached.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailedAt = time.Now()

	if cb.state == StateHalfOpen {
		cb.state = StateOpen
		log.Printf("[WARN] circuit breaker: %s probe failed, reopened", cb.sourceName)
		return
	}

	if cb.failures >= cb.threshold {
		cb.state = StateOpen
		log.Printf("[WARN] circuit breaker: %s opened after %d consecutive failures", cb.sourceName, cb.failures)
	}
}

// State returns the current state (for monitoring/status endpoints).
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// StateName returns the current state as a string.
func (cb *CircuitBreaker) StateName() string {
	switch cb.State() {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	}
	return "unknown"
}

// Failures returns the current failure count.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failures
}

// SourceName returns the name of the source this breaker protects.
func (cb *CircuitBreaker) SourceName() string {
	return cb.sourceName
}

// ProtectedSource wraps a MetadataSource with a circuit breaker.
type ProtectedSource struct {
	source  MetadataSource
	breaker *CircuitBreaker
}

// NewProtectedSource wraps a MetadataSource with circuit breaker protection.
func NewProtectedSource(source MetadataSource, threshold int, cooldown time.Duration) *ProtectedSource {
	return &ProtectedSource{
		source:  source,
		breaker: NewCircuitBreaker(source.Name(), threshold, cooldown),
	}
}

func (ps *ProtectedSource) Name() string {
	return ps.source.Name()
}

func (ps *ProtectedSource) SearchByTitle(title string) ([]BookMetadata, error) {
	if err := ps.breaker.AllowRequest(); err != nil {
		return nil, err
	}
	results, err := ps.source.SearchByTitle(title)
	if err != nil {
		ps.breaker.RecordFailure()
		return nil, err
	}
	ps.breaker.RecordSuccess()
	return results, nil
}

func (ps *ProtectedSource) SearchByTitleAndAuthor(title, author string) ([]BookMetadata, error) {
	if err := ps.breaker.AllowRequest(); err != nil {
		return nil, err
	}
	results, err := ps.source.SearchByTitleAndAuthor(title, author)
	if err != nil {
		ps.breaker.RecordFailure()
		return nil, err
	}
	ps.breaker.RecordSuccess()
	return results, nil
}

// Breaker returns the underlying CircuitBreaker for status reporting.
func (ps *ProtectedSource) Breaker() *CircuitBreaker {
	return ps.breaker
}
