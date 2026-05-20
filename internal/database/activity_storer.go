// file: internal/database/activity_storer.go
// version: 1.0.2
// guid: a1b2c3d4-e5f6-0001-abcd-000000000001

package database

import (
	"context"
	"time"
)

// ActivityStorer is the minimal interface required by activity.Service and
// activity.Writer. Both ActivityStore (SQLite) and NutsActivityStore satisfy it.
type ActivityStorer interface {
	Record(ActivityEntry) (int64, error)
	Query(ActivityFilter) ([]ActivityEntry, int, error)
	Summarize(ctx context.Context, olderThan time.Time, tier string) (int, error)
	Prune(olderThan time.Time, tier string) (int, error)
	GetDistinctSources(ActivityFilter) ([]SourceCount, error)
	WipeAllActivity() (int64, error)
	CompactByDay(ctx context.Context, olderThan time.Time) (CompactResult, error)
	RecompactDigests(ctx context.Context) (RecompactResult, error)
	MigrateSystemActivityLogs() (int, error)
	Close() error
}

// MetricsStorer is the minimal interface required by server cache handlers.
// Both MetricsStore (SQLite) and NutsMetricsStore satisfy it.
type MetricsStorer interface {
	RecordCacheStatsSnapshots([]CacheStatsSnapshot) error
	GetCacheStatsHistory(cacheName string, since time.Time, limit int) ([]CacheStatsSnapshot, error)
	PruneCacheStatsHistory(olderThan time.Time) (int64, error)
	Close() error
}
