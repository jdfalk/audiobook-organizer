// file: internal/database/pebble_store_metadata_cache.go
// version: 1.0.0

package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/cockroachdb/pebble/v2"
)

// metadataCacheKeyPrefix is the prefix every per-book cache key shares.
const metadataCacheKeyPrefix = "metadata_cache:"

func metadataCacheKey(bookID string) []byte {
	return []byte(metadataCacheKeyPrefix + bookID)
}

// GetMetadataCache reads the cache entry for bookID, or returns
// (nil, nil) when the key is absent.
func (p *PebbleStore) GetMetadataCache(bookID string) (*MetadataCandidateCache, error) {
	val, closer, err := p.db.Get(metadataCacheKey(bookID))
	if err != nil {
		if errors.Is(err, pebble.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("pebble get metadata_cache:%s: %w", bookID, err)
	}
	defer closer.Close()

	var entry MetadataCandidateCache
	if err := json.Unmarshal(val, &entry); err != nil {
		return nil, fmt.Errorf("decode metadata_cache:%s: %w", bookID, err)
	}
	return &entry, nil
}

// PutMetadataCache writes (or replaces) the cache entry for entry.BookID.
func (p *PebbleStore) PutMetadataCache(entry *MetadataCandidateCache) error {
	if entry == nil || entry.BookID == "" {
		return fmt.Errorf("PutMetadataCache: nil entry or empty BookID")
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode metadata_cache:%s: %w", entry.BookID, err)
	}
	if err := p.db.Set(metadataCacheKey(entry.BookID), data, pebble.Sync); err != nil {
		return fmt.Errorf("pebble set metadata_cache:%s: %w", entry.BookID, err)
	}
	return nil
}

// DeleteMetadataCache removes the cache entry for bookID. Missing
// keys are not an error.
func (p *PebbleStore) DeleteMetadataCache(bookID string) error {
	if err := p.db.Delete(metadataCacheKey(bookID), pebble.Sync); err != nil {
		return fmt.Errorf("pebble delete metadata_cache:%s: %w", bookID, err)
	}
	return nil
}

// ListMetadataCacheKeys returns one summary per cached entry, ordered
// by FetchedAt descending. Caller paginates.
func (p *PebbleStore) ListMetadataCacheKeys() ([]MetadataCacheSummary, error) {
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(metadataCacheKeyPrefix),
		UpperBound: []byte("metadata_cache;"), // ';' is one byte after ':'
	})
	if err != nil {
		return nil, fmt.Errorf("new iter metadata_cache: %w", err)
	}
	defer iter.Close()

	var out []MetadataCacheSummary
	for iter.First(); iter.Valid(); iter.Next() {
		var entry MetadataCandidateCache
		if err := json.Unmarshal(iter.Value(), &entry); err != nil {
			// Skip corrupt rows rather than fail the whole list.
			continue
		}
		out = append(out, MetadataCacheSummary{
			BookID:         entry.BookID,
			FetchedAt:      entry.FetchedAt,
			CandidateCount: len(entry.Candidates),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].FetchedAt.After(out[j].FetchedAt)
	})
	return out, nil
}
