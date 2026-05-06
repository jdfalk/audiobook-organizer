// file: internal/database/metadata_fetch_cache.go
// version: 1.3.0
// guid: 9e8d7c6b-5a4f-3e2d-1c0b-9a8b7c6d5e4f
// last-edited: 2026-05-01

package database

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/metrics"
)

// MetadataFetchCache caches external metadata API responses
// keyed by (book_id, source_name). Added after the 2026-04-11
// OpenAI quota incident and subsequent user complaint that
// re-fetching 8000 books re-hit every external API for every
// book even when the result was identical — books don't change
// meaningfully between fetches, so repeat API calls are just
// wasted quota and user review time.
//
// The cache sits INSIDE the metadata fetch path: before any
// Search* call on a MetadataSource, the service asks the cache
// for a hit; on hit, the cached results are returned; on miss,
// the external API is called and the results are written back
// to the cache.
//
// Storage uses the Store's raw key-value methods (SetRaw /
// GetRaw / DeleteRaw) so the cache works against whichever
// primary store the user picked — Pebble today, possibly
// Postgres or CockroachDB in the future — without needing
// backend-specific migrations.
//
// Cache keys are namespaced under "metadata_fetch_cache:" so
// a ScanPrefix can list every cached entry for diagnostics
// or wholesale invalidation.

// CachedMetadataEntry is the serialized shape of a cache row.
// The CachedAt timestamp exists so an optional TTL policy can
// filter stale entries; the current implementation never
// expires, but the timestamp is recorded for future use and
// for the diagnostics surface.
type CachedMetadataEntry struct {
	BookID    string            `json:"book_id"`
	Source    string            `json:"source"`
	Results   json.RawMessage   `json:"results"`         // []metadata.BookMetadata, left as RawMessage so this package doesn't need to import internal/metadata
	BestScore float64           `json:"best_score"`
	CachedAt  time.Time         `json:"cached_at"`
	Extra     map[string]string `json:"extra,omitempty"` // reserved for future use (language tag, TTL override, etc.)
}

// metadataFetchCacheKey formats the storage key for a
// (bookID, source) pair. Kept stable so cache entries survive
// across restarts. Source names are lowercased to avoid
// "Hardcover" vs "hardcover" drift.
func metadataFetchCacheKey(bookID, source string) string {
	return "metadata_fetch_cache:" + bookID + ":" + strings.ToLower(strings.TrimSpace(source))
}

// GetCachedMetadataFetch looks up a cache entry. Returns nil
// on miss (not an error). The returned Results slice is the
// raw JSON payload the caller originally stored — the caller
// is responsible for unmarshalling it into its own type.
func GetCachedMetadataFetch(store Store, bookID, source string) (*CachedMetadataEntry, error) {
	if bookID == "" || source == "" {
		return nil, nil
	}
	start := time.Now()
	defer func() { metrics.ObserveCacheGetDuration("metadata_fetch", time.Since(start)) }()

	blob, err := store.GetRaw(metadataFetchCacheKey(bookID, source))
	if err != nil {
		return nil, fmt.Errorf("cache get: %w", err)
	}
	if blob == nil {
		metrics.RecordCacheMiss("metadata_fetch", "not_found")
		return nil, nil
	}
	var entry CachedMetadataEntry
	if err := json.Unmarshal(blob, &entry); err != nil {
		// Corrupt entry — treat as a miss and delete it so
		// the next call writes a fresh row.
		if err := store.DeleteRaw(metadataFetchCacheKey(bookID, source)); err != nil {
			slog.Warn("failed to delete corrupt cache entry", "key", metadataFetchCacheKey(bookID, source), "error", err)
		}
		metrics.RecordCacheMiss("metadata_fetch", "stale")
		return nil, nil
	}
	metrics.RecordCacheHit("metadata_fetch")
	return &entry, nil
}

// PutCachedMetadataFetch writes a cache entry. The `results`
// parameter is pre-marshalled JSON so this package doesn't
// need to depend on internal/metadata for the type.
//
// Writes are best-effort — a cache put failure is logged by
// the caller but never fails the outer fetch, per the same
// principle as the embedding cache: the cache is an
// optimization, not a correctness layer.
func PutCachedMetadataFetch(store Store, bookID, source string, results json.RawMessage, bestScore float64) error {
	if bookID == "" || source == "" {
		return nil
	}
	entry := CachedMetadataEntry{
		BookID:    bookID,
		Source:    strings.ToLower(strings.TrimSpace(source)),
		Results:   results,
		BestScore: bestScore,
		CachedAt:  time.Now().UTC(),
	}
	blob, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("cache marshal: %w", err)
	}
	if err := store.SetRaw(metadataFetchCacheKey(bookID, source), blob); err != nil {
		return err
	}
	metrics.RecordCacheSet("metadata_fetch")
	return nil
}

// InvalidateCachedMetadataFetch removes the cache entry for
// a single (bookID, source) pair. Called after a metadata
// apply so the next fetch refreshes from the source.
func InvalidateCachedMetadataFetch(store Store, bookID, source string) error {
	if bookID == "" || source == "" {
		return nil
	}
	if err := store.DeleteRaw(metadataFetchCacheKey(bookID, source)); err != nil {
		return err
	}
	metrics.RecordCacheInvalidation("metadata_fetch", "key")
	return nil
}

// CountCachedMetadataFetches returns the number of entries currently
// stored in the DB-backed metadata fetch cache. Used by the cache stats
// handler to populate the Size field for this non-in-memory cache.
func CountCachedMetadataFetches(store Store) (int64, error) {
	return store.CountPrefix("metadata_fetch_cache:")
}

// InvalidateAllCachedMetadataFetchesForBook wipes every source's
// cache entry for a single book. Called when the book's title
// or author changes — any cached candidate is now stale because
// it was queried against different search terms.
func InvalidateAllCachedMetadataFetchesForBook(store Store, bookID string) error {
	if bookID == "" {
		return nil
	}
	prefix := "metadata_fetch_cache:" + bookID + ":"
	pairs, err := store.ScanPrefix(prefix)
	if err != nil {
		return fmt.Errorf("cache scan: %w", err)
	}
	for _, kv := range pairs {
		if err := store.DeleteRaw(kv.Key); err != nil {
			return fmt.Errorf("cache delete %s: %w", kv.Key, err)
		}
	}
	if len(pairs) > 0 {
		metrics.RecordCacheInvalidation("metadata_fetch", "book")
	}
	return nil
}
