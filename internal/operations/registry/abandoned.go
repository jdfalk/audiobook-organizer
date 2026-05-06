// file: internal/operations/registry/abandoned.go
// version: 1.0.0
// guid: 1a2b3c4d-5e6f-7890-abcd-ef1234567890
// last-edited: 2026-05-06

package registry

import (
	"sync"
)

// abandonedTracker tracks goroutines that have been ctx-canceled but have not
// yet returned. This can happen when an op ignores ctx.Done(). Each such
// goroutine holds a worker slot that has been released and replaced, so the
// pool size stays constant — but unbounded abandoned goroutines for a single
// plugin indicate a bug in that plugin's Run function.
//
// When count[plugin] reaches cap, the dispatcher refuses new dispatches for
// that plugin until at least one abandoned goroutine finishes.
type abandonedTracker struct {
	mu    sync.Mutex
	count map[string]int // plugin → count of abandoned goroutines
	cap   int            // default 4
}

// newAbandonedTracker returns a tracker with the default capacity.
func newAbandonedTracker(cap int) *abandonedTracker {
	if cap <= 0 {
		cap = 4
	}
	return &abandonedTracker{
		count: make(map[string]int),
		cap:   cap,
	}
}

// increment records one more abandoned goroutine for plugin.
func (a *abandonedTracker) increment(plugin string) {
	a.mu.Lock()
	a.count[plugin]++
	a.mu.Unlock()
}

// decrement records that one abandoned goroutine for plugin has returned.
func (a *abandonedTracker) decrement(plugin string) {
	a.mu.Lock()
	a.count[plugin]--
	if a.count[plugin] < 0 {
		a.count[plugin] = 0
	}
	a.mu.Unlock()
}

// isBlocked returns true if the plugin has >= cap abandoned goroutines.
func (a *abandonedTracker) isBlocked(plugin string) bool {
	a.mu.Lock()
	blocked := a.count[plugin] >= a.cap
	a.mu.Unlock()
	return blocked
}

// countFor returns the current abandoned count for a plugin (for testing/metrics).
func (a *abandonedTracker) countFor(plugin string) int {
	a.mu.Lock()
	n := a.count[plugin]
	a.mu.Unlock()
	return n
}
