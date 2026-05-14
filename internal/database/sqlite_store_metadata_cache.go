// file: internal/database/sqlite_store_metadata_cache.go
// version: 1.0.0

// SQLiteStore does NOT implement the metadata cache. Consistent with
// the Pebble-primary policy (feedback_pebble_primary.md): hot paths
// that need new storage land on PebbleStore only, and SQLite paths
// return ErrUnsupported. The metafetch.Service nil-checks before use
// so SQLite-only deployments (tests that opt in) degrade gracefully —
// no cached candidates, every fetch hits the metadata sources fresh.

package database

import "errors"

// ErrMetadataCacheUnsupported is returned by SQLiteStore for all
// MetadataCacheStore methods. Distinct error so callers can detect
// the "this backend doesn't support caching" path vs. a real failure.
var ErrMetadataCacheUnsupported = errors.New("metadata cache: not supported by this store backend")

func (s *SQLiteStore) GetMetadataCache(bookID string) (*MetadataCandidateCache, error) {
	// nil entry, no error — treated as cache-miss by callers.
	return nil, nil
}
func (s *SQLiteStore) PutMetadataCache(entry *MetadataCandidateCache) error {
	return ErrMetadataCacheUnsupported
}
func (s *SQLiteStore) DeleteMetadataCache(bookID string) error { return nil }
func (s *SQLiteStore) ListMetadataCacheKeys() ([]MetadataCacheSummary, error) {
	return nil, nil
}
