// file: internal/cache/cache.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package cache

import (
	"sync"
	"time"
)

type entry[T any] struct {
	value     T
	expiresAt time.Time
}

// Cache is a simple generic TTL cache safe for concurrent use.
type Cache[T any] struct {
	mu      sync.RWMutex
	items   map[string]entry[T]
	defaultTTL time.Duration
}

// New creates a cache with the given default TTL.
func New[T any](defaultTTL time.Duration) *Cache[T] {
	return &Cache[T]{
		items:      make(map[string]entry[T]),
		defaultTTL: defaultTTL,
	}
}

// Get retrieves a value if it exists and hasn't expired.
func (c *Cache[T]) Get(key string) (T, bool) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expiresAt) {
		var zero T
		return zero, false
	}
	return e.value, true
}

// Set stores a value with the default TTL.
func (c *Cache[T]) Set(key string, value T) {
	c.SetWithTTL(key, value, c.defaultTTL)
}

// SetWithTTL stores a value with a specific TTL.
func (c *Cache[T]) SetWithTTL(key string, value T, ttl time.Duration) {
	c.mu.Lock()
	c.items[key] = entry[T]{value: value, expiresAt: time.Now().Add(ttl)}
	c.mu.Unlock()
}

// Invalidate removes a single key.
func (c *Cache[T]) Invalidate(key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// InvalidateAll removes all entries.
func (c *Cache[T]) InvalidateAll() {
	c.mu.Lock()
	c.items = make(map[string]entry[T])
	c.mu.Unlock()
}
