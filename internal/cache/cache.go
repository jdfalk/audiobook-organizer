// file: internal/cache/cache.go
// version: 1.4.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package cache

import (
	"container/list"
	"sort"
	"sync"
	"time"

	"github.com/falkcorp/audiobook-organizer/internal/metrics"
)

// entryNode is the value stored in each LRU list element. The key is denormalized
// here so capacity eviction can find the map slot to delete in O(1).
type entryNode[T any] struct {
	key       string
	value     T
	expiresAt time.Time
}

// Cache is a generic TTL cache safe for concurrent use.
//
// Each instance carries a name used as the {cache} label on Prometheus
// metrics; keep names short, stable, and drawn from a small enum so the
// metric cardinality stays bounded.
//
// When maxEntries > 0 the cache enforces an LRU capacity bound by evicting
// the least recently accessed entry on Set; expired entries are also
// reaped lazily on Get regardless of the bound. Both eviction paths emit
// cache_evictions_total{reason}.
type Cache[T any] struct {
	mu         sync.Mutex
	name       string
	items      map[string]*list.Element
	lru        *list.List // front = most recent, back = least recent
	maxEntries int        // 0 means unbounded
	defaultTTL time.Duration
}

// New creates an unbounded cache with the given name and default TTL.
func New[T any](name string, defaultTTL time.Duration) *Cache[T] {
	return NewWithLimit[T](name, defaultTTL, 0)
}

// NewWithLimit creates a cache with the given name, default TTL, and an
// optional LRU capacity bound. maxEntries <= 0 means unbounded.
func NewWithLimit[T any](name string, defaultTTL time.Duration, maxEntries int) *Cache[T] {
	if maxEntries < 0 {
		maxEntries = 0
	}
	c := &Cache[T]{
		name:       name,
		items:      make(map[string]*list.Element),
		lru:        list.New(),
		maxEntries: maxEntries,
		defaultTTL: defaultTTL,
	}
	register(c)
	return c
}

// Name returns the cache instance name.
func (c *Cache[T]) Name() string { return c.name }

// Get retrieves a value if it exists and hasn't expired. Expired entries
// are evicted in-place and counted as cache_evictions_total{reason="expired"}.
func (c *Cache[T]) Get(key string) (T, bool) {
	start := time.Now()
	defer func() { metrics.ObserveCacheGetDuration(c.name, time.Since(start)) }()

	c.mu.Lock()
	elem, ok := c.items[key]
	if !ok {
		c.mu.Unlock()
		metrics.RecordCacheMiss(c.name, "not_found")
		var zero T
		return zero, false
	}
	node := elem.Value.(*entryNode[T])
	if time.Now().After(node.expiresAt) {
		// Lazy reap so cache_size reflects reality and capacity is freed.
		c.lru.Remove(elem)
		delete(c.items, key)
		size := len(c.items)
		c.mu.Unlock()
		metrics.RecordCacheMiss(c.name, "expired")
		metrics.RecordCacheEviction(c.name, "expired")
		metrics.SetCacheSize(c.name, size)
		var zero T
		return zero, false
	}
	c.lru.MoveToFront(elem)
	value := node.value
	c.mu.Unlock()
	metrics.RecordCacheHit(c.name)
	return value, true
}

// Set stores a value with the default TTL.
func (c *Cache[T]) Set(key string, value T) {
	c.SetWithTTL(key, value, c.defaultTTL)
}

// SetWithTTL stores a value with a specific TTL. If a capacity bound is
// configured and the cache is at capacity, the least recently used entry
// is evicted (counted as cache_evictions_total{reason="capacity"}).
func (c *Cache[T]) SetWithTTL(key string, value T, ttl time.Duration) {
	c.mu.Lock()
	expires := time.Now().Add(ttl)
	if elem, ok := c.items[key]; ok {
		// Update in place, keep recency.
		node := elem.Value.(*entryNode[T])
		node.value = value
		node.expiresAt = expires
		c.lru.MoveToFront(elem)
		size := len(c.items)
		c.mu.Unlock()
		metrics.RecordCacheSet(c.name)
		metrics.SetCacheSize(c.name, size)
		return
	}
	node := &entryNode[T]{key: key, value: value, expiresAt: expires}
	elem := c.lru.PushFront(node)
	c.items[key] = elem

	evicted := false
	if c.maxEntries > 0 && len(c.items) > c.maxEntries {
		oldest := c.lru.Back()
		if oldest != nil {
			old := oldest.Value.(*entryNode[T])
			c.lru.Remove(oldest)
			delete(c.items, old.key)
			evicted = true
		}
	}
	size := len(c.items)
	c.mu.Unlock()

	metrics.RecordCacheSet(c.name)
	if evicted {
		metrics.RecordCacheEviction(c.name, "capacity")
	}
	metrics.SetCacheSize(c.name, size)
}

// Invalidate removes a single key.
func (c *Cache[T]) Invalidate(key string) {
	c.mu.Lock()
	if elem, ok := c.items[key]; ok {
		c.lru.Remove(elem)
		delete(c.items, key)
	}
	size := len(c.items)
	c.mu.Unlock()
	metrics.RecordCacheInvalidation(c.name, "key")
	metrics.SetCacheSize(c.name, size)
}

// InvalidateAll removes all entries.
func (c *Cache[T]) InvalidateAll() {
	c.mu.Lock()
	c.items = make(map[string]*list.Element)
	c.lru.Init()
	c.mu.Unlock()
	metrics.RecordCacheInvalidation(c.name, "all")
	metrics.SetCacheSize(c.name, 0)
}

// Len returns the number of entries (including expired ones not yet evicted).
func (c *Cache[T]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// Keys returns a sorted snapshot of all current keys in the cache.
func (c *Cache[T]) Keys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, 0, len(c.items))
	for k := range c.items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
