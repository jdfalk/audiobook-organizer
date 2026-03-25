// file: internal/database/activity_store_test.go
// version: 1.0.0
// guid: f3a1b2c4-d5e6-7f8a-9b0c-1d2e3f4a5b6c

package database

import (
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

	deleted, err := s.Summarize(cutoff, "realtime")
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
