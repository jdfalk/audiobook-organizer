// file: internal/database/activity_compact_test.go
// version: 1.1.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompactByDay_BasicCompaction inserts 5 entries across 2 days (change tier)
// plus 1 audit entry, compacts, and verifies 2 digests created, 5 entries
// deleted, and the audit entry survives.
func TestCompactByDay_BasicCompaction(t *testing.T) {
	s := newTestActivityStore(t)

	day1 := time.Date(2025, 6, 10, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 6, 11, 14, 0, 0, 0, time.UTC)
	olderThan := time.Date(2025, 6, 12, 0, 0, 0, 0, time.UTC)

	// 3 change-tier entries on day 1
	for i := 0; i < 3; i++ {
		_, err := s.Record(ActivityEntry{
			Tier:      "change",
			Type:      "metadata_applied",
			Level:     "info",
			Source:    "pipeline",
			BookID:    "book-1",
			Summary:   "applied metadata",
			Timestamp: day1.Add(time.Duration(i) * time.Minute),
			Details:   map[string]any{"book_title": "Test Book", "fields": "title,author", "source": "openai"},
		})
		require.NoError(t, err)
	}

	// 2 change-tier entries on day 2
	for i := 0; i < 2; i++ {
		_, err := s.Record(ActivityEntry{
			Tier:      "change",
			Type:      "tag_written",
			Level:     "info",
			Source:    "writer",
			BookID:    "book-2",
			Summary:   "wrote tags",
			Timestamp: day2.Add(time.Duration(i) * time.Minute),
			Details:   map[string]any{"title": "Other Book", "tag_count": float64(5), "file_count": float64(2)},
		})
		require.NoError(t, err)
	}

	// 1 audit entry (must survive)
	_, err := s.Record(ActivityEntry{
		Tier:      "audit",
		Type:      "user_login",
		Level:     "info",
		Source:    "auth",
		Summary:   "user logged in",
		Timestamp: day1.Add(30 * time.Minute),
	})
	require.NoError(t, err)

	result, err := s.CompactByDay(olderThan)
	require.NoError(t, err)
	assert.Equal(t, 2, result.DaysCompacted)
	assert.Equal(t, 5, result.EntriesDeleted)

	// Should now have: 2 digest rows + 1 audit row = 3 total
	all, total, err := s.Query(ActivityFilter{Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, 3, total)

	var digestCount, auditCount int
	for _, e := range all {
		switch e.Tier {
		case "digest":
			digestCount++
			assert.Equal(t, "daily_digest", e.Type)
			assert.Equal(t, "compaction", e.Source)

			// Verify digest details structure
			require.NotNil(t, e.Details)
			detailsJSON, err := json.Marshal(e.Details)
			require.NoError(t, err)
			var dd DigestDetails
			err = json.Unmarshal(detailsJSON, &dd)
			require.NoError(t, err)
			assert.NotEmpty(t, dd.Date)
			assert.Greater(t, dd.OriginalCount, 0)
			assert.NotEmpty(t, dd.Counts)
			assert.NotEmpty(t, dd.Items)
		case "audit":
			auditCount++
		}
	}
	assert.Equal(t, 2, digestCount, "expected 2 digest rows")
	assert.Equal(t, 1, auditCount, "audit entry must survive")
}

// TestCompactByDay_Idempotent verifies that compacting twice is a no-op the
// second time.
func TestCompactByDay_Idempotent(t *testing.T) {
	s := newTestActivityStore(t)

	ts := time.Date(2025, 5, 1, 10, 0, 0, 0, time.UTC)
	olderThan := time.Date(2025, 5, 2, 0, 0, 0, 0, time.UTC)

	_, err := s.Record(ActivityEntry{
		Tier:      "change",
		Type:      "config_changed",
		Level:     "info",
		Source:    "settings",
		Summary:   "changed setting",
		Timestamp: ts,
		Details:   map[string]any{"key": "scan_interval"},
	})
	require.NoError(t, err)

	// First compaction
	r1, err := s.CompactByDay(olderThan)
	require.NoError(t, err)
	assert.Equal(t, 1, r1.DaysCompacted)
	assert.Equal(t, 1, r1.EntriesDeleted)

	// Second compaction — should be no-op
	r2, err := s.CompactByDay(olderThan)
	require.NoError(t, err)
	assert.Equal(t, 0, r2.DaysCompacted)
	assert.Equal(t, 0, r2.EntriesDeleted)
}

// TestCompactByDay_SkipsAuditTier verifies that audit-tier entries are never
// compacted.
func TestCompactByDay_SkipsAuditTier(t *testing.T) {
	s := newTestActivityStore(t)

	ts := time.Date(2025, 4, 15, 8, 0, 0, 0, time.UTC)
	olderThan := time.Date(2025, 4, 16, 0, 0, 0, 0, time.UTC)

	_, err := s.Record(ActivityEntry{
		Tier:      "audit",
		Type:      "permission_change",
		Level:     "info",
		Source:    "admin",
		Summary:   "permissions updated",
		Timestamp: ts,
	})
	require.NoError(t, err)

	result, err := s.CompactByDay(olderThan)
	require.NoError(t, err)
	assert.Equal(t, 0, result.DaysCompacted)
	assert.Equal(t, 0, result.EntriesDeleted)

	// Original entry still exists
	all, total, err := s.Query(ActivityFilter{Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, all, 1)
	assert.Equal(t, "audit", all[0].Tier)
}

// TestCompactByDay_TruncatesLargeDays inserts 600 entries on one day and
// verifies items are capped at 500 with truncation metadata.
func TestCompactByDay_TruncatesLargeDays(t *testing.T) {
	s := newTestActivityStore(t)

	day := time.Date(2025, 3, 20, 6, 0, 0, 0, time.UTC)
	olderThan := time.Date(2025, 3, 21, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 600; i++ {
		_, err := s.Record(ActivityEntry{
			Tier:      "debug",
			Type:      "tag_written",
			Level:     "debug",
			Source:    "writer",
			Summary:   "wrote tags",
			Timestamp: day.Add(time.Duration(i) * time.Second),
			Details:   map[string]any{"tag_count": float64(3), "file_count": float64(1)},
		})
		require.NoError(t, err)
	}

	result, err := s.CompactByDay(olderThan)
	require.NoError(t, err)
	assert.Equal(t, 1, result.DaysCompacted)
	assert.Equal(t, 600, result.EntriesDeleted)

	// Verify digest details
	all, total, err := s.Query(ActivityFilter{Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	require.Len(t, all, 1)

	digest := all[0]
	assert.Equal(t, "digest", digest.Tier)
	require.NotNil(t, digest.Details)

	detailsJSON, err := json.Marshal(digest.Details)
	require.NoError(t, err)
	var dd DigestDetails
	err = json.Unmarshal(detailsJSON, &dd)
	require.NoError(t, err)

	assert.Len(t, dd.Items, 500, "items should be capped at 500")
	assert.True(t, dd.Truncated, "Truncated should be true")
	assert.Equal(t, 100, dd.TruncatedCount, "100 items should have been truncated")
	assert.Equal(t, 600, dd.OriginalCount)
}

// TestCompactByDay_MergesIntoExistingDigest is the regression test for the
// "compact Everything (now) returns 0" bug. When a daily digest already
// exists for a date AND more uncompacted change/debug entries have been
// written for that same date (late imports, background tasks, etc.), a
// second compact run used to `continue` past the day and leave the new
// entries permanently uncompacted. This test proves that's fixed: the
// second run merges new entries into the existing digest and deletes the
// originals.
func TestCompactByDay_MergesIntoExistingDigest(t *testing.T) {
	s := newTestActivityStore(t)

	// Day 1: three initial entries at 08:00 on 2025-05-15.
	day := time.Date(2025, 5, 15, 8, 0, 0, 0, time.UTC)
	// olderThan is set to "1 hour after the latest entry we'll add",
	// so every run compacts everything written so far.
	initialCutoff := time.Date(2025, 5, 15, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		_, err := s.Record(ActivityEntry{
			Tier:      "change",
			Type:      "metadata_applied",
			Level:     "info",
			Source:    "test",
			Summary:   "initial entry",
			Timestamp: day,
			Details:   map[string]any{"book_title": "Initial Book"},
		})
		require.NoError(t, err)
	}

	// First compaction — 3 entries compacted into 1 digest.
	r1, err := s.CompactByDay(initialCutoff)
	require.NoError(t, err)
	assert.Equal(t, 1, r1.DaysCompacted, "first run should create 1 digest")
	assert.Equal(t, 3, r1.EntriesDeleted)

	// Late-arriving entries: 5 more entries for the SAME day, written
	// AFTER the first compact ran. This is the real-world scenario —
	// background imports, deferred tasks, crash recovery.
	lateDay := time.Date(2025, 5, 15, 11, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		_, err := s.Record(ActivityEntry{
			Tier:      "change",
			Type:      "tag_written",
			Level:     "info",
			Source:    "test",
			Summary:   "late entry",
			Timestamp: lateDay,
			Details:   map[string]any{"book_title": "Late Book", "tag_count": float64(4), "file_count": float64(1)},
		})
		require.NoError(t, err)
	}

	// Second compaction — must MERGE the 5 late entries into the
	// existing digest, not skip them.
	r2, err := s.CompactByDay(initialCutoff)
	require.NoError(t, err)
	assert.Equal(t, 1, r2.DaysCompacted, "second run should merge into existing digest (counted as 1 day compacted)")
	assert.Equal(t, 5, r2.EntriesDeleted, "all 5 late entries must be deleted")

	// Exactly one digest row for 2025-05-15 (old one deleted, new one
	// inserted with combined data).
	var digestCount int
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM activity_log
		WHERE tier = 'digest' AND type = 'daily_digest'
		  AND date(timestamp) = '2025-05-15'`).Scan(&digestCount)
	require.NoError(t, err)
	assert.Equal(t, 1, digestCount, "must be exactly one digest per day")

	// Zero uncompacted change/debug rows remaining for 2025-05-15.
	var remaining int
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM activity_log
		WHERE tier IN ('change','debug') AND compacted = 0
		  AND date(timestamp) = '2025-05-15'`).Scan(&remaining)
	require.NoError(t, err)
	assert.Equal(t, 0, remaining, "no stragglers should remain")

	// Unmarshal the merged digest and verify it contains both old and new counts.
	var detailsJSON []byte
	err = s.db.QueryRow(`
		SELECT details FROM activity_log
		WHERE tier = 'digest' AND type = 'daily_digest'
		  AND date(timestamp) = '2025-05-15'`).Scan(&detailsJSON)
	require.NoError(t, err)
	var dd DigestDetails
	err = json.Unmarshal(detailsJSON, &dd)
	require.NoError(t, err)

	assert.Equal(t, 8, dd.OriginalCount, "merged digest should cover all 8 entries (3 old + 5 new)")
	assert.Equal(t, 3, dd.Counts["metadata_applied"], "old counts preserved")
	assert.Equal(t, 5, dd.Counts["tag_written"], "new counts added")
	assert.Len(t, dd.Items, 8, "all 8 items present")
}
