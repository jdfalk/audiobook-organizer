// file: internal/database/cache_stats_history_store_test.go
// version: 2.0.0
// guid: c1d2e3f4-a5b6-9788-7766-554433221101

package database

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestMetricsStore(t *testing.T) *MetricsStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	store, err := NewMetricsStore(dbPath)
	if err != nil {
		t.Fatalf("NewMetricsStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestCacheStatsHistoryRoundTrip(t *testing.T) {
	store := newTestMetricsStore(t)

	now := time.Now().UTC().Truncate(time.Second)
	snaps := []CacheStatsSnapshot{
		{CacheName: "dashboard", Timestamp: now.Add(-2 * time.Minute), Hits: 100, Misses: 5, Sets: 8, Invalidations: 1, Size: 12, GetDurationCount: 105, GetDurationSum: 0.0421},
		{CacheName: "dashboard", Timestamp: now.Add(-1 * time.Minute), Hits: 150, Misses: 7, Sets: 10, Invalidations: 1, Size: 14, GetDurationCount: 157, GetDurationSum: 0.0612},
		{CacheName: "ai_response", Timestamp: now, Hits: 30, Misses: 2, Sets: 5, Size: 5, GetDurationCount: 32, GetDurationSum: 0.0123},
	}
	if err := store.RecordCacheStatsSnapshots(snaps); err != nil {
		t.Fatalf("record: %v", err)
	}

	gotAll, err := store.GetCacheStatsHistory("", now.Add(-1*time.Hour), 0)
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if len(gotAll) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(gotAll))
	}

	gotDashboard, err := store.GetCacheStatsHistory("dashboard", now.Add(-1*time.Hour), 0)
	if err != nil {
		t.Fatalf("get dashboard: %v", err)
	}
	if len(gotDashboard) != 2 {
		t.Fatalf("expected 2 dashboard rows, got %d", len(gotDashboard))
	}
	if gotDashboard[0].Hits != 100 || gotDashboard[1].Hits != 150 {
		t.Fatalf("unexpected order: %+v", gotDashboard)
	}

	limited, err := store.GetCacheStatsHistory("dashboard", now.Add(-1*time.Hour), 1)
	if err != nil {
		t.Fatalf("get limited: %v", err)
	}
	if len(limited) != 1 || limited[0].Hits != 100 {
		t.Fatalf("limit broke: %+v", limited)
	}

	deleted, err := store.PruneCacheStatsHistory(now.Add(-30 * time.Second))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 pruned, got %d", deleted)
	}
}

func TestCacheStatsHistoryEmptyInsert(t *testing.T) {
	store := newTestMetricsStore(t)
	if err := store.RecordCacheStatsSnapshots(nil); err != nil {
		t.Fatalf("nil insert should be no-op: %v", err)
	}
}
