// file: internal/database/activity_store_test.go
// version: 1.2.1
// guid: f3a1b2c4-d5e6-7f8a-9b0c-1d2e3f4a5b6c

package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestActivityStore creates a temp ActivityStore and registers cleanup.
func newTestActivityStore(t *testing.T) *ActivityStore {
	t.Helper()
	dir := t.TempDir()
	store, err := NewActivityStore(filepath.Join(dir, "activity.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// TestActivityStore_OpenClose verifies the DB file is created on disk.
func TestActivityStore_OpenClose(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "activity.db")

	store, err := NewActivityStore(dbPath)
	require.NoError(t, err)
	require.NotNil(t, store)

	_, err = os.Stat(dbPath)
	assert.NoError(t, err, "DB file should exist after open")

	require.NoError(t, store.Close())
}

// TestActivityStore_RecordAndQuery inserts a fully-populated entry and reads it back.
func TestActivityStore_RecordAndQuery(t *testing.T) {
	s := newTestActivityStore(t)

	opID := "op-abc-123"
	bookID := "book-xyz-456"
	ts := time.Now().UTC().Truncate(time.Second)

	entry := ActivityEntry{
		Timestamp:   ts,
		Tier:        "realtime",
		Type:        "metadata_apply",
		Level:       "info",
		Source:      "apply_pipeline",
		OperationID: opID,
		BookID:      bookID,
		Summary:     "Applied metadata",
		Details:     map[string]any{"field": "title", "old": "foo", "new": "bar"},
		Tags:        []string{"enrichment", "isbn"},
	}

	id, err := s.Record(entry)
	require.NoError(t, err)
	assert.Greater(t, id, int64(0))

	entries, total, err := s.Query(ActivityFilter{Limit: 10})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, entries, 1)

	got := entries[0]
	assert.Equal(t, id, got.ID)
	assert.Equal(t, "realtime", got.Tier)
	assert.Equal(t, "metadata_apply", got.Type)
	assert.Equal(t, "info", got.Level)
	assert.Equal(t, "apply_pipeline", got.Source)
	assert.Equal(t, opID, got.OperationID)
	assert.Equal(t, bookID, got.BookID)
	assert.Equal(t, "Applied metadata", got.Summary)
	assert.Nil(t, got.PrunedAt)

	// Details round-trip
	require.NotNil(t, got.Details)
	assert.Equal(t, "title", got.Details["field"])

	// Tags round-trip
	assert.ElementsMatch(t, []string{"enrichment", "isbn"}, got.Tags)
}

// TestActivityStore_QueryFilters tests each filter field in isolation.
func TestActivityStore_QueryFilters(t *testing.T) {
	s := newTestActivityStore(t)

	now := time.Now().UTC()

	entries := []ActivityEntry{
		{Tier: "realtime", Type: "metadata_apply", Level: "info", Source: "s1",
			OperationID: "op-1", BookID: "book-1", Summary: "A",
			Tags: []string{"alpha", "beta"}, Timestamp: now.Add(-4 * time.Minute)},
		{Tier: "realtime", Type: "tag_write", Level: "warn", Source: "s2",
			OperationID: "op-1", BookID: "book-2", Summary: "B",
			Tags: []string{"beta", "gamma"}, Timestamp: now.Add(-3 * time.Minute)},
		{Tier: "background", Type: "isbn_lookup", Level: "info", Source: "s3",
			OperationID: "op-2", BookID: "book-1", Summary: "C",
			Tags: []string{"gamma"}, Timestamp: now.Add(-2 * time.Minute)},
		{Tier: "background", Type: "isbn_lookup", Level: "error", Source: "s4",
			OperationID: "op-3", BookID: "book-3", Summary: "D",
			Tags: []string{"alpha"}, Timestamp: now.Add(-1 * time.Minute)},
	}
	for _, e := range entries {
		_, err := s.Record(e)
		require.NoError(t, err)
	}

	t.Run("filter_by_tier", func(t *testing.T) {
		res, total, err := s.Query(ActivityFilter{Tier: "background", Limit: 50})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, res, 2)
		for _, r := range res {
			assert.Equal(t, "background", r.Tier)
		}
	})

	t.Run("filter_by_type", func(t *testing.T) {
		res, total, err := s.Query(ActivityFilter{Type: "isbn_lookup", Limit: 50})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, res, 2)
	})

	t.Run("filter_by_operation_id", func(t *testing.T) {
		res, total, err := s.Query(ActivityFilter{OperationID: "op-1", Limit: 50})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, res, 2)
	})

	t.Run("filter_by_book_id", func(t *testing.T) {
		res, total, err := s.Query(ActivityFilter{BookID: "book-1", Limit: 50})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, res, 2)
	})

	t.Run("filter_single_tag_alpha", func(t *testing.T) {
		res, total, err := s.Query(ActivityFilter{Tags: []string{"alpha"}, Limit: 50})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, res, 2)
	})

	t.Run("filter_two_tags_requires_both", func(t *testing.T) {
		// Only entry[0] has both alpha AND beta
		res, total, err := s.Query(ActivityFilter{Tags: []string{"alpha", "beta"}, Limit: 50})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, res, 1)
		assert.Equal(t, "A", res[0].Summary)
	})

	t.Run("limit", func(t *testing.T) {
		res, total, err := s.Query(ActivityFilter{Limit: 2})
		require.NoError(t, err)
		assert.Equal(t, 4, total)
		assert.Len(t, res, 2)
	})

	t.Run("offset", func(t *testing.T) {
		res, total, err := s.Query(ActivityFilter{Limit: 50, Offset: 3})
		require.NoError(t, err)
		assert.Equal(t, 4, total)
		assert.Len(t, res, 1)
	})
}

// TestActivityStore_Summarize inserts old entries + one recent, summarizes, checks result.
func TestActivityStore_Summarize(t *testing.T) {
	s := newTestActivityStore(t)

	cutoff := time.Now().UTC().Add(-1 * time.Hour)

	// 5 old entries for the same operation
	for i := 0; i < 5; i++ {
		_, err := s.Record(ActivityEntry{
			Tier:        "realtime",
			Type:        "metadata_apply",
			Level:       "info",
			Source:      "pipeline",
			OperationID: "op-summarize",
			BookID:      "book-99",
			Summary:     "step",
			Timestamp:   cutoff.Add(-time.Duration(i+1) * time.Minute),
		})
		require.NoError(t, err)
	}

	// 1 recent entry that should NOT be summarized
	_, err := s.Record(ActivityEntry{
		Tier:        "realtime",
		Type:        "metadata_apply",
		Level:       "info",
		Source:      "pipeline",
		OperationID: "op-summarize",
		BookID:      "book-99",
		Summary:     "recent step",
		Timestamp:   time.Now().UTC(),
	})
	require.NoError(t, err)

	deleted, err := s.Summarize(context.Background(), cutoff, "realtime")
	require.NoError(t, err)
	assert.Equal(t, 5, deleted, "all 5 old originals should be deleted")

	// After summarize: 1 summary row + 1 recent row = 2 total
	all, total, err := s.Query(ActivityFilter{Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, all, 2)

	// Find the summary row (should have pruned_at set)
	var summaryFound bool
	for _, e := range all {
		if e.PrunedAt != nil {
			summaryFound = true
			assert.Equal(t, "realtime", e.Tier)
		}
	}
	assert.True(t, summaryFound, "one entry should have pruned_at set (the summary)")
}

// TestActivityStore_Prune deletes old entries of the given tier.
func TestActivityStore_Prune(t *testing.T) {
	s := newTestActivityStore(t)

	cutoff := time.Now().UTC().Add(-1 * time.Hour)

	// 3 old debug entries
	for i := 0; i < 3; i++ {
		_, err := s.Record(ActivityEntry{
			Tier: "debug", Type: "tag_write", Level: "debug",
			Source: "writer", Summary: "old debug",
			Timestamp: cutoff.Add(-time.Duration(i+1) * time.Minute),
		})
		require.NoError(t, err)
	}

	// 1 old audit entry (different tier — should NOT be pruned)
	_, err := s.Record(ActivityEntry{
		Tier: "audit", Type: "tag_write", Level: "info",
		Source: "writer", Summary: "old audit",
		Timestamp: cutoff.Add(-5 * time.Minute),
	})
	require.NoError(t, err)

	// 1 recent debug (should NOT be pruned — too new)
	_, err = s.Record(ActivityEntry{
		Tier: "debug", Type: "tag_write", Level: "debug",
		Source: "writer", Summary: "recent debug",
		Timestamp: time.Now().UTC(),
	})
	require.NoError(t, err)

	deleted, err := s.Prune(cutoff, "debug")
	require.NoError(t, err)
	assert.Equal(t, 3, deleted)

	all, total, err := s.Query(ActivityFilter{Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, all, 2)

	for _, e := range all {
		assert.NotEqual(t, "old debug", e.Summary)
	}
}

// TestActivityStore_SearchFilter verifies that Search filters on summary substring.
func TestActivityStore_SearchFilter(t *testing.T) {
	s := newTestActivityStore(t)

	entries := []ActivityEntry{
		{Tier: "realtime", Type: "info_event", Level: "info", Source: "gin",
			Summary: "Project Hail Mary is a great book"},
		{Tier: "realtime", Type: "info_event", Level: "info", Source: "gin",
			Summary: "The Martian is also excellent"},
		{Tier: "realtime", Type: "info_event", Level: "info", Source: "gin",
			Summary: "Andromeda Strain is a classic"},
	}
	for _, e := range entries {
		_, err := s.Record(e)
		require.NoError(t, err)
	}

	res, total, err := s.Query(ActivityFilter{Search: "Hail Mary", Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, res, 1)
	assert.Contains(t, res[0].Summary, "Hail Mary")
}

// TestActivityStore_SourceFilters verifies Source and ExcludeSources filters.
func TestActivityStore_SourceFilters(t *testing.T) {
	s := newTestActivityStore(t)

	entries := []ActivityEntry{
		{Tier: "realtime", Type: "request", Level: "info", Source: "gin", Summary: "GET /api/1"},
		{Tier: "realtime", Type: "request", Level: "info", Source: "gin", Summary: "GET /api/2"},
		{Tier: "background", Type: "cron_tick", Level: "info", Source: "scheduler", Summary: "daily tick"},
		{Tier: "background", Type: "metadata_apply", Level: "info", Source: "metadata", Summary: "applied"},
	}
	for _, e := range entries {
		_, err := s.Record(e)
		require.NoError(t, err)
	}

	t.Run("source_exact", func(t *testing.T) {
		res, total, err := s.Query(ActivityFilter{Source: "scheduler", Limit: 50})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		require.Len(t, res, 1)
		assert.Equal(t, "scheduler", res[0].Source)
	})

	t.Run("exclude_sources", func(t *testing.T) {
		res, total, err := s.Query(ActivityFilter{ExcludeSources: []string{"gin"}, Limit: 50})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		require.Len(t, res, 2)
		for _, r := range res {
			assert.NotEqual(t, "gin", r.Source)
		}
	})
}

// TestActivityStore_GetDistinctSources verifies source aggregation with and without filters.
func TestActivityStore_GetDistinctSources(t *testing.T) {
	s := newTestActivityStore(t)

	entries := []ActivityEntry{
		{Tier: "realtime", Type: "request", Level: "info", Source: "gin", Summary: "req 1"},
		{Tier: "debug", Type: "request", Level: "debug", Source: "gin", Summary: "req 2"},
		{Tier: "background", Type: "scan", Level: "info", Source: "scanner", Summary: "scan 1"},
		{Tier: "realtime", Type: "metadata_apply", Level: "info", Source: "metadata", Summary: "apply 1"},
	}
	for _, e := range entries {
		_, err := s.Record(e)
		require.NoError(t, err)
	}

	t.Run("unfiltered_returns_all_sources", func(t *testing.T) {
		sources, err := s.GetDistinctSources(ActivityFilter{})
		require.NoError(t, err)
		assert.Len(t, sources, 3, "expected gin, scanner, metadata")

		// gin has 2 entries so should be first
		assert.Equal(t, "gin", sources[0].Source)
		assert.Equal(t, 2, sources[0].Count)
	})

	t.Run("filtered_by_tier_debug", func(t *testing.T) {
		sources, err := s.GetDistinctSources(ActivityFilter{Tier: "debug"})
		require.NoError(t, err)
		assert.Len(t, sources, 1)
		assert.Equal(t, "gin", sources[0].Source)
		assert.Equal(t, 1, sources[0].Count)
	})
}

// TestMigrateSystemActivityLogs verifies legacy system_activity_log rows are migrated.
func TestMigrateSystemActivityLogs(t *testing.T) {
	dir := t.TempDir()

	// Create ActivityStore database.
	actDBPath := filepath.Join(dir, "activity.db")
	actStore, err := NewActivityStore(actDBPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = actStore.Close() })

	// Manually create old system_activity_log table in the same database.
	_, err = actStore.db.Exec(`CREATE TABLE IF NOT EXISTS system_activity_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id TEXT,
		source TEXT NOT NULL,
		level TEXT NOT NULL DEFAULT 'info',
		message TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)

	// Insert a few old system_activity_log rows.
	now := time.Now().UTC()
	oldRows := []struct {
		source, level, message string
		createdAt              time.Time
	}{
		{"itunes", "info", "iTunes scan started", now.Add(-100 * time.Hour)},
		{"reconcile", "warning", "Found 5 missing files", now.Add(-50 * time.Hour)},
		{"maintenance", "error", "Backup failed: disk full", now.Add(-24 * time.Hour)},
	}

	for _, row := range oldRows {
		_, err := actStore.db.Exec(`
			INSERT INTO system_activity_log (source, level, message, created_at)
			VALUES (?, ?, ?, ?)`,
			row.source, row.level, row.message, row.createdAt)
		require.NoError(t, err)
	}

	// Run migration.
	count, err := actStore.MigrateSystemActivityLogs()
	require.NoError(t, err)
	assert.Equal(t, 3, count, "should migrate 3 rows")

	// Verify migrated entries are in activity_log with correct fields.
	entries, total, err := actStore.Query(ActivityFilter{
		Tags: []string{"legacy"},
		Limit: 100,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, total, "should find 3 entries with 'legacy' tag")
	assert.Len(t, entries, 3)

	// Verify all entries have the correct structure (order is newest-first).
	sourcesSeen := make(map[string]bool)
	for _, e := range entries {
		assert.Equal(t, "system", e.Tier, "all should have tier='system'")
		assert.Equal(t, "system_log", e.Type, "all should have type='system_log'")
		assert.NotEmpty(t, e.Summary, "summary should be populated from old message field")
		assert.Equal(t, []string{"legacy", "system_activity_log"}, e.Tags)
		sourcesSeen[e.Source] = true
	}
	// Verify all three sources are present.
	assert.Contains(t, sourcesSeen, "itunes")
	assert.Contains(t, sourcesSeen, "reconcile")
	assert.Contains(t, sourcesSeen, "maintenance")

	// Verify migration is idempotent: running again returns 0.
	count2, err := actStore.MigrateSystemActivityLogs()
	require.NoError(t, err)
	assert.Equal(t, 0, count2, "second migration should be no-op")

	// Verify no duplicate entries were created.
	entries2, total2, err := actStore.Query(ActivityFilter{
		Tags: []string{"legacy"},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, total2, "still exactly 3 legacy entries")
	assert.Len(t, entries2, 3)
}
