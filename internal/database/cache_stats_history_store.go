// file: internal/database/cache_stats_history_store.go
// version: 2.0.0
// guid: f1e2d3c4-b5a6-9788-7766-554433221100

package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// MetricsStore owns a dedicated SQLite sidecar database used for operational
// telemetry that should be queryable regardless of the primary store backend
// (PebbleDB or SQLite). Today it persists cache observability snapshots; future
// metrics belong here too rather than polluting the main store.
type MetricsStore struct {
	db *sql.DB
}

const metricsSchema = `
CREATE TABLE IF NOT EXISTS cache_stats_history (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	cache_name TEXT NOT NULL,
	ts TIMESTAMP NOT NULL,
	hits INTEGER NOT NULL DEFAULT 0,
	misses INTEGER NOT NULL DEFAULT 0,
	sets INTEGER NOT NULL DEFAULT 0,
	invalidations INTEGER NOT NULL DEFAULT 0,
	evictions INTEGER NOT NULL DEFAULT 0,
	size INTEGER NOT NULL DEFAULT 0,
	get_duration_count INTEGER NOT NULL DEFAULT 0,
	get_duration_sum REAL NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_cache_stats_history_name_ts ON cache_stats_history(cache_name, ts);
CREATE INDEX IF NOT EXISTS idx_cache_stats_history_ts ON cache_stats_history(ts);
`

// CacheStatsSnapshot is one row in cache_stats_history. Misses, Invalidations,
// and Evictions are flattened sums across reasons/scopes — the per-reason
// breakdown still lives in Prometheus; this table answers "how did this cache
// trend over time" not "why did it miss."
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

// NewMetricsStore opens (or creates) the metrics sidecar SQLite at dbPath
// and ensures the schema is current. WAL + 5s busy timeout for concurrent
// reads alongside the snapshotter writer.
func NewMetricsStore(dbPath string) (*MetricsStore, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=off", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("metrics_store: open %q: %w", dbPath, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("metrics_store: ping %q: %w", dbPath, err)
	}
	if _, err := db.Exec(metricsSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("metrics_store: schema: %w", err)
	}
	return &MetricsStore{db: db}, nil
}

// Close shuts down the underlying database connection.
func (s *MetricsStore) Close() error { return s.db.Close() }

// RecordCacheStatsSnapshots inserts a batch of snapshots in a single transaction.
func (s *MetricsStore) RecordCacheStatsSnapshots(snapshots []CacheStatsSnapshot) error {
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
func (s *MetricsStore) GetCacheStatsHistory(cacheName string, since time.Time, limit int) ([]CacheStatsSnapshot, error) {
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

// PruneCacheStatsHistory deletes snapshots older than the cutoff. Returns
// the number of rows deleted.
func (s *MetricsStore) PruneCacheStatsHistory(olderThan time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM cache_stats_history WHERE ts < ?`, olderThan)
	if err != nil {
		return 0, fmt.Errorf("prune: %w", err)
	}
	return res.RowsAffected()
}
