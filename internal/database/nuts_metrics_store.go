// file: internal/database/nuts_metrics_store.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-0002-bcde-000000000002

package database

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/nutsdb/nutsdb"
)

const (
	metricsTTL        uint32 = 30 * 24 * 3600 // 30 days in seconds
	metricsIdxBucket         = "met:_idx"      // tracks known cache names
	metricsKeyMaxTime        = "99999999999999999999"
)

// NutsMetricsStore persists cache-stats snapshots in a NutsDB directory.
// It is a drop-in replacement for MetricsStore (SQLite).
type NutsMetricsStore struct {
	db *nutsdb.DB
}

func metsBucket(cacheName string) string { return "met:" + cacheName }

func metsTimeKey(t time.Time) []byte {
	return []byte(fmt.Sprintf("%020d", t.UnixNano()))
}

// NewNutsMetricsStore opens (or creates) a NutsDB metrics store at dirPath.
func NewNutsMetricsStore(dirPath string) (*NutsMetricsStore, error) {
	opts := nutsdb.DefaultOptions
	opts.Dir = dirPath
	opts.EntryIdxMode = nutsdb.HintKeyAndRAMIdxMode
	opts.SyncEnable = false
	opts.GCWhenClose = true
	opts.MergeInterval = 6 * time.Hour
	opts.SegmentSize = 64 << 20 // 64 MB — metrics is low-volume

	db, err := nutsdb.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("nuts_metrics_store: open %q: %w", dirPath, err)
	}
	return &NutsMetricsStore{db: db}, nil
}

// Close shuts down the underlying NutsDB.
func (s *NutsMetricsStore) Close() error { return s.db.Close() }

// RecordCacheStatsSnapshots writes all snapshots in a single transaction.
// Each entry gets a 30-day TTL so old data expires automatically.
func (s *NutsMetricsStore) RecordCacheStatsSnapshots(snapshots []CacheStatsSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	return s.db.Update(func(tx *nutsdb.Tx) error {
		for _, snap := range snapshots {
			b, err := json.Marshal(snap)
			if err != nil {
				return fmt.Errorf("marshal %s: %w", snap.CacheName, err)
			}
			bucket := metsBucket(snap.CacheName)
			if err := tx.Put(bucket, metsTimeKey(snap.Timestamp), b, metricsTTL); err != nil {
				return fmt.Errorf("put snapshot %s: %w", snap.CacheName, err)
			}
			// Track cache name in the index bucket (idempotent).
			_ = tx.Put(metricsIdxBucket, []byte(snap.CacheName), []byte("1"), 0)
		}
		return nil
	})
}

// GetCacheStatsHistory returns snapshots since the given time, oldest-first.
// If cacheName is empty, all known caches are returned merged and sorted.
func (s *NutsMetricsStore) GetCacheStatsHistory(cacheName string, since time.Time, limit int) ([]CacheStatsSnapshot, error) {
	names, err := s.cacheNames(cacheName)
	if err != nil {
		return nil, err
	}

	start := metsTimeKey(since)
	end := []byte(metricsKeyMaxTime)

	var out []CacheStatsSnapshot
	for _, name := range names {
		err := s.db.View(func(tx *nutsdb.Tx) error {
			_, vals, err := tx.RangeScanEntries(metsBucket(name), start, end, false, true)
			if err != nil {
				if nutsdb.IsBucketNotFound(err) || nutsdb.IsBucketEmpty(err) {
					return nil
				}
				return err
			}
			for _, v := range vals {
				var snap CacheStatsSnapshot
				if err := json.Unmarshal(v, &snap); err != nil {
					continue
				}
				out = append(out, snap)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan cache %s: %w", name, err)
		}
	}

	// Sort oldest-first; if multi-cache, entries from different buckets need sorting.
	if len(names) > 1 {
		sort.Slice(out, func(i, j int) bool {
			return out[i].Timestamp.Before(out[j].Timestamp)
		})
	}

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// PruneCacheStatsHistory is a no-op — 30-day TTL on each entry handles expiry.
func (s *NutsMetricsStore) PruneCacheStatsHistory(_ time.Time) (int64, error) {
	return 0, nil
}

// cacheNames returns [cacheName] when non-empty, or all known names from the index.
func (s *NutsMetricsStore) cacheNames(cacheName string) ([]string, error) {
	if cacheName != "" {
		return []string{cacheName}, nil
	}
	var names []string
	err := s.db.View(func(tx *nutsdb.Tx) error {
		keys, _, err := tx.RangeScanEntries(metricsIdxBucket, []byte(""), []byte("\xff\xff\xff\xff"), true, false)
		if err != nil {
			if nutsdb.IsBucketNotFound(err) || nutsdb.IsBucketEmpty(err) {
				return nil
			}
			return err
		}
		for _, k := range keys {
			names = append(names, string(k))
		}
		return nil
	})
	return names, err
}
