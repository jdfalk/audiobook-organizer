// file: internal/cache/cache.go
// version: 1.3.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package cache

import (
	"sort"
	"sync"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/metrics"
)

type entry[T any] struct {
	value     T
	expiresAt time.Time
}

// Cache is a simple generic TTL cache safe for concurrent use.
//
// Each instance carries a name used as the {cache} label on Prometheus
// metrics; keep names short, stable, and drawn from a small enum so the
// metric cardinality stays bounded.
type Cache[T any] struct {
	mu         sync.RWMutex
	name       string
	items      map[string]entry[T]
	defaultTTL time.Duration
}

// New creates a cache with the given name and default TTL. The name is used
// as the {cache} label on emitted Prometheus metrics.
func New[T any](name string, defaultTTL time.Duration) *Cache[T] {
	c := &Cache[T]{
		name:       name,
		items:      make(map[string]entry[T]),
		defaultTTL: defaultTTL,
	}
	register(c)
	return c
}

// Name returns the cache instance name.
func (c *Cache[T]) Name() string { return c.name }

// Get retrieves a value if it exists and hasn't expired.
func (c *Cache[T]) Get(key string) (T, bool) {
	start := time.Now()
	defer func() { metrics.ObserveCacheGetDuration(c.name, time.Since(start)) }()

	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		metrics.RecordCacheMiss(c.name, "not_found")
		var zero T
		return zero, false
	}
	if time.Now().After(e.expiresAt) {
		metrics.RecordCacheMiss(c.name, "expired")
		var zero T
		return zero, false
	}
	metrics.RecordCacheHit(c.name)
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
	n := len(c.items)
	c.mu.Unlock()
	metrics.RecordCacheSet(c.name)
	metrics.SetCacheSize(c.name, n)
}

// Invalidate removes a single key.
func (c *Cache[T]) Invalidate(key string) {
	c.mu.Lock()
	delete(c.items, key)
	n := len(c.items)
	c.mu.Unlock()
	metrics.RecordCacheInvalidation(c.name, "key")
	metrics.SetCacheSize(c.name, n)
}

// InvalidateAll removes all entries.
func (c *Cache[T]) InvalidateAll() {
	c.mu.Lock()
	c.items = make(map[string]entry[T])
	c.mu.Unlock()
	metrics.RecordCacheInvalidation(c.name, "all")
	metrics.SetCacheSize(c.name, 0)
}

// Len returns the number of entries (including expired ones not yet evicted).
func (c *Cache[T]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Keys returns a sorted snapshot of all current keys in the cache.
func (c *Cache[T]) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]string, 0, len(c.items))
	for k := range c.items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
