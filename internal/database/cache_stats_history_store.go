// file: internal/database/cache_stats_history_store.go
// version: 1.0.0
// guid: f1e2d3c4-b5a6-9788-7766-554433221100

package database

import (
	"database/sql"
	"fmt"
	"time"
)

// CacheStatsSnapshot is one row in cache_stats_history.
//
// Misses, Invalidations, and Evictions are stored as flattened sums (across all
// reasons/scopes) because the live per-reason breakdown still lives in
// Prometheus; the history table answers "how did this cache trend over time"
// not "why did it miss." If reason-level history becomes useful later, add
// extra columns rather than denormalizing into JSON.
type CacheStatsSnapshot struct {
	CacheName        string    `json:"cache_name"`
	Timestamp        time.Time `json:"ts"`
	Hits             int64     `json:"hits"`
	Misses           int64     `json:"misses"`
	Sets             int64     `json:"sets"`
	Invalidations    int64     `json:"invalidations"`
	Evictions        int64     `json:"evictions"`
	Size             int64     `json:"size"`
	GetDurationCount int64     `json:"get_duration_count"`
	GetDurationSum   float64   `json:"get_duration_sum"`
}

// RecordCacheStatsSnapshots inserts a batch of snapshots in a single transaction.
// PebbleDB stores ignore the call (no-op) since this table is SQLite-only.
func (s *SQLiteStore) RecordCacheStatsSnapshots(snapshots []CacheStatsSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO cache_stats_history
		(cache_name, ts, hits, misses, sets, invalidations, evictions, size, get_duration_count, get_duration_sum)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, snap := range snapshots {
		if _, err := stmt.Exec(
			snap.CacheName, snap.Timestamp,
			snap.Hits, snap.Misses, snap.Sets, snap.Invalidations, snap.Evictions,
			snap.Size, snap.GetDurationCount, snap.GetDurationSum,
		); err != nil {
			return fmt.Errorf("insert snapshot %s: %w", snap.CacheName, err)
		}
	}
	return tx.Commit()
}

// GetCacheStatsHistory returns snapshots for a cache name (or all if empty)
// since the given timestamp, ordered oldest-first. limit caps row count
// (0 means no limit).
func (s *SQLiteStore) GetCacheStatsHistory(cacheName string, since time.Time, limit int) ([]CacheStatsSnapshot, error) {
	var (
		rows *sql.Rows
		err  error
	)
	switch {
	case cacheName != "" && limit > 0:
		rows, err = s.db.Query(`SELECT cache_name, ts, hits, misses, sets, invalidations, evictions, size, get_duration_count, get_duration_sum
			FROM cache_stats_history WHERE cache_name = ? AND ts >= ? ORDER BY ts ASC LIMIT ?`, cacheName, since, limit)
	case cacheName != "":
		rows, err = s.db.Query(`SELECT cache_name, ts, hits, misses, sets, invalidations, evictions, size, get_duration_count, get_duration_sum
			FROM cache_stats_history WHERE cache_name = ? AND ts >= ? ORDER BY ts ASC`, cacheName, since)
	case limit > 0:
		rows, err = s.db.Query(`SELECT cache_name, ts, hits, misses, sets, invalidations, evictions, size, get_duration_count, get_duration_sum
			FROM cache_stats_history WHERE ts >= ? ORDER BY ts ASC LIMIT ?`, since, limit)
	default:
		rows, err = s.db.Query(`SELECT cache_name, ts, hits, misses, sets, invalidations, evictions, size, get_duration_count, get_duration_sum
			FROM cache_stats_history WHERE ts >= ? ORDER BY ts ASC`, since)
	}
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var out []CacheStatsSnapshot
	for rows.Next() {
		var snap CacheStatsSnapshot
		if err := rows.Scan(
			&snap.CacheName, &snap.Timestamp,
			&snap.Hits, &snap.Misses, &snap.Sets, &snap.Invalidations, &snap.Evictions,
			&snap.Size, &snap.GetDurationCount, &snap.GetDurationSum,
		); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

// PruneCacheStatsHistory deletes snapshots older than the cutoff. Used by
// background maintenance to keep the table from growing unboundedly. Returns
// the number of rows deleted.
func (s *SQLiteStore) PruneCacheStatsHistory(olderThan time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM cache_stats_history WHERE ts < ?`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("prune: %w", err)
	}
	return res.RowsAffected()
}
