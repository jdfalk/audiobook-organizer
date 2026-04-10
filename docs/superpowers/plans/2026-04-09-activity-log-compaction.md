# Activity Log Compaction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace old activity log entries with daily digest rows that preserve what happened in a compact, expandable format.

**Architecture:** Add a `compacted` column to the SQLite `activity_log` table. New `CompactByDay()` method groups entries by UTC day, builds a structured JSON digest (counts + per-item summaries), inserts one digest row per day, deletes originals. Frontend renders digest rows as expandable cards. Manual trigger via API + automatic during maintenance window.

**Tech Stack:** Go/SQLite (backend), React/MUI (frontend), existing ActivityStore/ActivityService pattern.

---

### Task 1: Add `compacted` column to activity_log schema

**Files:**
- Modify: `internal/database/activity_store.go:56-80`

- [ ] **Step 1: Add the column migration to activitySchema**

In `internal/database/activity_store.go`, after the existing `CREATE INDEX` statements in `activitySchema`, add:

```go
const activitySchema = `
CREATE TABLE IF NOT EXISTS activity_log (
    id           INTEGER  PRIMARY KEY AUTOINCREMENT,
    timestamp    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    tier         TEXT     NOT NULL,
    type         TEXT     NOT NULL,
    level        TEXT     NOT NULL DEFAULT 'info',
    source       TEXT     NOT NULL,
    operation_id TEXT,
    book_id      TEXT,
    summary      TEXT     NOT NULL,
    details      JSON,
    tags         TEXT,
    pruned_at    DATETIME,
    compacted    BOOLEAN  NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_activity_timestamp        ON activity_log (timestamp);
CREATE INDEX IF NOT EXISTS idx_activity_type_timestamp   ON activity_log (type, timestamp);
CREATE INDEX IF NOT EXISTS idx_activity_operation_id     ON activity_log (operation_id);
CREATE INDEX IF NOT EXISTS idx_activity_book_timestamp   ON activity_log (book_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_activity_tier             ON activity_log (tier);
CREATE INDEX IF NOT EXISTS idx_activity_tags             ON activity_log (tags);
CREATE INDEX IF NOT EXISTS idx_activity_source           ON activity_log (source);
CREATE INDEX IF NOT EXISTS idx_activity_tier_timestamp   ON activity_log (tier, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_activity_level_timestamp  ON activity_log (level, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_activity_compacted        ON activity_log (compacted);
`
```

- [ ] **Step 2: Add an auto-migration in NewActivityStore**

After the schema exec in `NewActivityStore`, add a migration that adds the column to existing databases:

```go
func NewActivityStore(dbPath string) (*ActivityStore, error) {
	// ... existing open/ping/schema code ...

	// Migrate: add compacted column if missing (idempotent)
	_, _ = db.Exec(`ALTER TABLE activity_log ADD COLUMN compacted BOOLEAN NOT NULL DEFAULT 0`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_activity_compacted ON activity_log (compacted)`)

	return &ActivityStore{db: db}, nil
}
```

- [ ] **Step 3: Build and verify**

Run: `make build-api`
Expected: Builds successfully.

- [ ] **Step 4: Commit**

```bash
git add internal/database/activity_store.go
git commit -m "feat: add compacted column to activity_log schema"
```

---

### Task 2: Implement CompactByDay on ActivityStore

**Files:**
- Modify: `internal/database/activity_store.go`
- Create: `internal/database/activity_compact_test.go`

- [ ] **Step 1: Write the test**

Create `internal/database/activity_compact_test.go`:

```go
package database

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestActivityStore(t *testing.T) *ActivityStore {
	t.Helper()
	dir := t.TempDir()
	store, err := NewActivityStore(filepath.Join(dir, "test_activity.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestCompactByDay_BasicCompaction(t *testing.T) {
	store := newTestActivityStore(t)

	// Insert 5 entries across 2 days in the "change" tier
	day1 := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 3, 2, 14, 0, 0, 0, time.UTC)

	entries := []ActivityEntry{
		{Timestamp: day1, Tier: "change", Type: "metadata_applied", Level: "info", Source: "metadata", BookID: "book1", Summary: "Applied metadata from Audible", Details: map[string]any{"fields": []any{"title", "narrator"}, "source": "Audible"}},
		{Timestamp: day1.Add(time.Hour), Tier: "change", Type: "tag_written", Level: "info", Source: "tagger", BookID: "book2", Summary: "Wrote tags to 3 files", Details: map[string]any{"tag_count": 14, "file_count": 3}},
		{Timestamp: day1.Add(2 * time.Hour), Tier: "change", Type: "organize_completed", Level: "info", Source: "organizer", BookID: "book3", Summary: "Organized book", Details: map[string]any{"new_path": "/Author/Title/"}},
		{Timestamp: day2, Tier: "change", Type: "metadata_applied", Level: "info", Source: "metadata", BookID: "book4", Summary: "Applied metadata from Google Books"},
		{Timestamp: day2.Add(time.Hour), Tier: "change", Type: "error", Level: "error", Source: "tagger", BookID: "book5", Summary: "Tag write failed: file not valid", Details: map[string]any{"error": "taglib: file not valid", "path": "/mnt/data/book.m4b"}},
	}
	for _, e := range entries {
		_, err := store.Record(e)
		require.NoError(t, err)
	}

	// Also insert an audit entry that should NOT be compacted
	_, err := store.Record(ActivityEntry{Timestamp: day1, Tier: "audit", Type: "user_action", Level: "info", Source: "auth", Summary: "User logged in"})
	require.NoError(t, err)

	// Compact everything older than March 3
	cutoff := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
	result, err := store.CompactByDay(cutoff)
	require.NoError(t, err)
	assert.Equal(t, 2, result.DaysCompacted)
	assert.Equal(t, 5, result.EntriesDeleted)

	// Query all entries — should have 2 digests + 1 audit entry
	all, total, err := store.Query(ActivityFilter{Limit: 100})
	require.NoError(t, err)
	assert.Equal(t, 3, total)

	// Find the digest entries
	var digests []ActivityEntry
	for _, e := range all {
		if e.Tier == "digest" {
			digests = append(digests, e)
		}
	}
	assert.Len(t, digests, 2)

	// Check day 2 digest (newest first)
	d := digests[0]
	assert.Equal(t, "daily_digest", d.Type)
	assert.Equal(t, "compaction", d.Source)
	assert.Contains(t, d.Summary, "2 activities")

	// Verify details structure
	raw, _ := json.Marshal(d.Details)
	var details DigestDetails
	require.NoError(t, json.Unmarshal(raw, &details))
	assert.Equal(t, 2, details.OriginalCount)
	assert.Len(t, details.Items, 2)

	// Audit entry still exists
	auditEntries, _, _ := store.Query(ActivityFilter{Tier: "audit"})
	assert.Len(t, auditEntries, 1)
}

func TestCompactByDay_Idempotent(t *testing.T) {
	store := newTestActivityStore(t)

	day1 := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	_, err := store.Record(ActivityEntry{Timestamp: day1, Tier: "change", Type: "metadata_applied", Level: "info", Source: "test", Summary: "test"})
	require.NoError(t, err)

	cutoff := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)

	// First compaction
	r1, err := store.CompactByDay(cutoff)
	require.NoError(t, err)
	assert.Equal(t, 1, r1.DaysCompacted)

	// Second compaction — should be a no-op
	r2, err := store.CompactByDay(cutoff)
	require.NoError(t, err)
	assert.Equal(t, 0, r2.DaysCompacted)
	assert.Equal(t, 0, r2.EntriesDeleted)
}

func TestCompactByDay_SkipsAuditTier(t *testing.T) {
	store := newTestActivityStore(t)

	day1 := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	_, _ = store.Record(ActivityEntry{Timestamp: day1, Tier: "audit", Type: "user_action", Level: "info", Source: "auth", Summary: "login"})

	cutoff := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	result, err := store.CompactByDay(cutoff)
	require.NoError(t, err)
	assert.Equal(t, 0, result.DaysCompacted)

	all, total, _ := store.Query(ActivityFilter{Limit: 100})
	assert.Equal(t, 1, total)
	assert.Equal(t, "audit", all[0].Tier)
}

func TestCompactByDay_TruncatesLargeDays(t *testing.T) {
	store := newTestActivityStore(t)

	day1 := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	// Insert 600 entries
	for i := 0; i < 600; i++ {
		_, _ = store.Record(ActivityEntry{
			Timestamp: day1.Add(time.Duration(i) * time.Second),
			Tier:      "debug",
			Type:      "scan_progress",
			Level:     "info",
			Source:    "scanner",
			Summary:   "Scanning...",
		})
	}

	cutoff := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	result, err := store.CompactByDay(cutoff)
	require.NoError(t, err)
	assert.Equal(t, 1, result.DaysCompacted)
	assert.Equal(t, 600, result.EntriesDeleted)

	digests, _, _ := store.Query(ActivityFilter{Tier: "digest"})
	require.Len(t, digests, 1)

	raw, _ := json.Marshal(digests[0].Details)
	var details DigestDetails
	require.NoError(t, json.Unmarshal(raw, &details))
	assert.Equal(t, 600, details.OriginalCount)
	assert.LessOrEqual(t, len(details.Items), 500)
	assert.True(t, details.Truncated)
	assert.Equal(t, 100, details.TruncatedCount)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/database/ -run TestCompactByDay -v -count=1`
Expected: FAIL — `CompactByDay` and `DigestDetails` not defined.

- [ ] **Step 3: Implement the types and CompactByDay method**

Add to `internal/database/activity_store.go`:

```go
// CompactResult holds the result of a CompactByDay operation.
type CompactResult struct {
	DaysCompacted  int `json:"days_compacted"`
	EntriesDeleted int `json:"entries_deleted"`
}

// DigestItem is one entry in a compacted daily digest.
type DigestItem struct {
	Type    string `json:"type"`
	Book    string `json:"book,omitempty"`
	BookID  string `json:"book_id,omitempty"`
	Summary string `json:"summary"`
	Details string `json:"details,omitempty"` // extra detail for errors
}

// DigestDetails is the JSON structure stored in a digest entry's details field.
type DigestDetails struct {
	Date           string         `json:"date"`
	OriginalCount  int            `json:"original_count"`
	Counts         map[string]int `json:"counts"`
	Items          []DigestItem   `json:"items"`
	Truncated      bool           `json:"truncated,omitempty"`
	TruncatedCount int            `json:"truncated_count,omitempty"`
}

const maxDigestItems = 500

// CompactByDay groups old change/debug entries by UTC day, creates a digest
// row per day, and deletes the originals. Audit tier is never compacted.
// Idempotent — skips days that already have a digest row.
func (s *ActivityStore) CompactByDay(olderThan time.Time) (CompactResult, error) {
	var result CompactResult

	// 1. Fetch all compactable entries ordered by timestamp
	rows, err := s.db.Query(`
		SELECT id, timestamp, tier, type, level, source, operation_id, book_id,
		       summary, details, tags
		FROM   activity_log
		WHERE  tier IN ('change', 'debug')
		  AND  compacted = 0
		  AND  timestamp < ?
		ORDER BY timestamp ASC`,
		olderThan.UTC(),
	)
	if err != nil {
		return result, fmt.Errorf("activity_store: compact query: %w", err)
	}
	defer rows.Close()

	// Group entries by UTC date
	type dayGroup struct {
		date    string // "2006-01-02"
		entries []ActivityEntry
	}
	groupMap := map[string]*dayGroup{}
	var groupOrder []string

	for rows.Next() {
		var (
			e          ActivityEntry
			ts         time.Time
			opID       sql.NullString
			bookID     sql.NullString
			detailsRaw sql.NullString
			tagsRaw    sql.NullString
		)
		if err := rows.Scan(&e.ID, &ts, &e.Tier, &e.Type, &e.Level, &e.Source,
			&opID, &bookID, &e.Summary, &detailsRaw, &tagsRaw); err != nil {
			return result, fmt.Errorf("activity_store: compact scan: %w", err)
		}
		e.Timestamp = ts.UTC()
		if opID.Valid {
			e.OperationID = opID.String
		}
		if bookID.Valid {
			e.BookID = bookID.String
		}
		if detailsRaw.Valid && detailsRaw.String != "" {
			_ = json.Unmarshal([]byte(detailsRaw.String), &e.Details)
		}

		dateKey := e.Timestamp.Format("2006-01-02")
		if _, ok := groupMap[dateKey]; !ok {
			groupMap[dateKey] = &dayGroup{date: dateKey}
			groupOrder = append(groupOrder, dateKey)
		}
		groupMap[dateKey].entries = append(groupMap[dateKey].entries, e)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("activity_store: compact rows: %w", err)
	}

	// 2. Process each day
	for _, dateKey := range groupOrder {
		grp := groupMap[dateKey]

		// Check idempotency — skip if digest already exists for this date
		var exists int
		err := s.db.QueryRow(`
			SELECT COUNT(*) FROM activity_log
			WHERE tier = 'digest' AND type = 'daily_digest'
			  AND date(timestamp) = ?`, dateKey).Scan(&exists)
		if err != nil {
			return result, fmt.Errorf("activity_store: compact check digest: %w", err)
		}
		if exists > 0 {
			continue
		}

		// Build counts and items
		counts := map[string]int{}
		var items []DigestItem
		var errorItems []DigestItem

		for _, e := range grp.entries {
			counts[e.Type]++
			item := DigestItem{
				Type:    e.Type,
				BookID:  e.BookID,
				Book:    extractBookName(e),
				Summary: extractItemSummary(e),
			}
			if e.Level == "error" || e.Level == "warn" {
				item.Details = extractErrorDetails(e)
				errorItems = append(errorItems, item)
			} else {
				items = append(items, item)
			}
		}

		// Build final items list: all errors first, then others up to cap
		digest := DigestDetails{
			Date:          dateKey,
			OriginalCount: len(grp.entries),
			Counts:        counts,
		}

		allItems := append(errorItems, items...)
		if len(allItems) > maxDigestItems {
			digest.Items = allItems[:maxDigestItems]
			digest.Truncated = true
			digest.TruncatedCount = len(allItems) - maxDigestItems
		} else {
			digest.Items = allItems
		}

		detailsJSON, err := json.Marshal(digest)
		if err != nil {
			return result, fmt.Errorf("activity_store: compact marshal: %w", err)
		}

		// Parse date for timestamp (end of day)
		dayTime, _ := time.Parse("2006-01-02", dateKey)
		endOfDay := dayTime.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

		summaryText := fmt.Sprintf("%s — %d activities",
			dayTime.Format("January 2, 2006"), len(grp.entries))

		// Insert digest + delete originals in a transaction
		tx, err := s.db.Begin()
		if err != nil {
			return result, fmt.Errorf("activity_store: compact begin tx: %w", err)
		}

		_, err = tx.Exec(`
			INSERT INTO activity_log
				(timestamp, tier, type, level, source, summary, details, compacted)
			VALUES (?, 'digest', 'daily_digest', 'info', 'compaction', ?, ?, 1)`,
			endOfDay, summaryText, string(detailsJSON),
		)
		if err != nil {
			tx.Rollback()
			return result, fmt.Errorf("activity_store: compact insert digest: %w", err)
		}

		// Collect IDs to delete
		ids := make([]any, len(grp.entries))
		placeholders := make([]string, len(grp.entries))
		for i, e := range grp.entries {
			ids[i] = e.ID
			placeholders[i] = "?"
		}

		delQuery := fmt.Sprintf("DELETE FROM activity_log WHERE id IN (%s)",
			strings.Join(placeholders, ","))
		res, err := tx.Exec(delQuery, ids...)
		if err != nil {
			tx.Rollback()
			return result, fmt.Errorf("activity_store: compact delete: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return result, fmt.Errorf("activity_store: compact commit: %w", err)
		}

		n, _ := res.RowsAffected()
		result.DaysCompacted++
		result.EntriesDeleted += int(n)
	}

	return result, nil
}

// extractBookName pulls a book name from an activity entry.
func extractBookName(e ActivityEntry) string {
	if e.Details != nil {
		if title, ok := e.Details["book_title"].(string); ok && title != "" {
			return title
		}
		if title, ok := e.Details["title"].(string); ok && title != "" {
			return title
		}
	}
	// Try to extract from summary — many summaries start with the book title
	return ""
}

// extractItemSummary creates a one-line summary from an activity entry.
func extractItemSummary(e ActivityEntry) string {
	switch e.Type {
	case "metadata_applied":
		if e.Details != nil {
			if fields, ok := e.Details["fields"].([]any); ok {
				names := make([]string, 0, len(fields))
				for _, f := range fields {
					if s, ok := f.(string); ok {
						names = append(names, s)
					}
				}
				source := ""
				if s, ok := e.Details["source"].(string); ok {
					source = " from " + s
				}
				if len(names) > 0 {
					return strings.Join(names, ", ") + source
				}
			}
		}
	case "tag_written":
		if e.Details != nil {
			tagCount, _ := e.Details["tag_count"].(float64)
			fileCount, _ := e.Details["file_count"].(float64)
			if tagCount > 0 || fileCount > 0 {
				return fmt.Sprintf("wrote %d tags to %d files", int(tagCount), int(fileCount))
			}
		}
	case "organize_completed":
		if e.Details != nil {
			if newPath, ok := e.Details["new_path"].(string); ok {
				return "moved to " + newPath
			}
		}
	case "config_changed":
		if e.Details != nil {
			if key, ok := e.Details["key"].(string); ok {
				return key + " changed"
			}
		}
	}
	// Fallback: use original summary, truncated
	s := e.Summary
	if len(s) > 120 {
		s = s[:117] + "..."
	}
	return s
}

// extractErrorDetails pulls error-specific detail from an activity entry.
func extractErrorDetails(e ActivityEntry) string {
	if e.Details != nil {
		parts := []string{}
		if errMsg, ok := e.Details["error"].(string); ok {
			parts = append(parts, errMsg)
		}
		if path, ok := e.Details["path"].(string); ok {
			parts = append(parts, "path: "+path)
		}
		if file, ok := e.Details["file_path"].(string); ok {
			parts = append(parts, "file: "+file)
		}
		if len(parts) > 0 {
			return strings.Join(parts, ", ")
		}
	}
	return ""
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/database/ -run TestCompactByDay -v -count=1`
Expected: All 4 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/database/activity_store.go internal/database/activity_compact_test.go
git commit -m "feat: implement CompactByDay for activity log daily digests"
```

---

### Task 3: Add CompactByDay to ActivityService and config

**Files:**
- Modify: `internal/server/activity_service.go`
- Modify: `internal/config/config.go`

- [ ] **Step 1: Add CompactByDay to ActivityService**

In `internal/server/activity_service.go`, add after the `Prune` method:

```go
// CompactByDay groups old change/debug entries by UTC day into digest rows.
func (s *ActivityService) CompactByDay(olderThan time.Time) (database.CompactResult, error) {
	return s.store.CompactByDay(olderThan)
}
```

- [ ] **Step 2: Add config key**

In `internal/config/config.go`, find the `ActivityLogRetentionDebugDays` field and add after it:

```go
ActivityLogCompactionDays int `json:"activity_log_compaction_days"` // default 14
```

And in the defaults struct (near line 842), add:

```go
ActivityLogCompactionDays: 14,
```

- [ ] **Step 3: Build and verify**

Run: `make build-api`
Expected: Builds successfully.

- [ ] **Step 4: Commit**

```bash
git add internal/server/activity_service.go internal/config/config.go
git commit -m "feat: expose CompactByDay on ActivityService, add config key"
```

---

### Task 4: Add POST /api/v1/activity/compact endpoint

**Files:**
- Modify: `internal/server/activity_handlers.go`
- Modify: `internal/server/server.go` (route registration)

- [ ] **Step 1: Add the handler**

In `internal/server/activity_handlers.go`, add after the existing `listActivitySources` function:

```go
// compactActivity handles POST /api/v1/activity/compact.
// Body: { "older_than_days": 14 }
func (s *Server) compactActivity(c *gin.Context) {
	if s.activityService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "activity log not available"})
		return
	}

	var req struct {
		OlderThanDays int `json:"older_than_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.OlderThanDays <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "older_than_days must be a positive integer"})
		return
	}

	cutoff := time.Now().AddDate(0, 0, -req.OlderThanDays)
	result, err := s.activityService.CompactByDay(cutoff)
	if err != nil {
		internalError(c, "activity compaction failed", err)
		return
	}

	c.JSON(http.StatusOK, result)
}
```

- [ ] **Step 2: Register the route**

In `internal/server/server.go`, find the line that registers `GET /api/v1/activity/sources` and add after it:

```go
protected.POST("/activity/compact", s.compactActivity)
```

- [ ] **Step 3: Build and verify**

Run: `make build-api`
Expected: Builds successfully.

- [ ] **Step 4: Commit**

```bash
git add internal/server/activity_handlers.go internal/server/server.go
git commit -m "feat: add POST /api/v1/activity/compact endpoint"
```

---

### Task 5: Integrate compaction into maintenance window scheduler

**Files:**
- Modify: `internal/server/scheduler.go:980-1010`

- [ ] **Step 1: Add compaction step before summarize/prune**

In `internal/server/scheduler.go`, replace the `cleanup_activity_log` task's enqueued function (the func passed to `Enqueue`) with:

```go
func(ctx context.Context, progress operations.ProgressReporter) error {
	if ts.server.activityService == nil {
		return nil
	}

	// Step 1: Compact old entries into daily digests
	compactionDays := config.AppConfig.ActivityLogCompactionDays
	if compactionDays <= 0 {
		compactionDays = 14
	}
	compactionCutoff := time.Now().AddDate(0, 0, -compactionDays)
	compacted, err := ts.server.activityService.CompactByDay(compactionCutoff)
	if err != nil {
		return fmt.Errorf("compact activity: %w", err)
	}

	// Step 2: Summarize remaining old change entries
	changeDays := config.AppConfig.ActivityLogRetentionChangeDays
	if changeDays <= 0 {
		changeDays = 90
	}
	changeCutoff := time.Now().AddDate(0, 0, -changeDays)
	summarized, err := ts.server.activityService.Summarize(changeCutoff, "change")
	if err != nil {
		return fmt.Errorf("summarize activity: %w", err)
	}

	// Step 3: Prune old debug entries
	debugDays := config.AppConfig.ActivityLogRetentionDebugDays
	if debugDays <= 0 {
		debugDays = 30
	}
	debugCutoff := time.Now().AddDate(0, 0, -debugDays)
	pruned, err := ts.server.activityService.Prune(debugCutoff, "debug")
	if err != nil {
		return fmt.Errorf("prune activity: %w", err)
	}

	log.Printf("Activity log cleanup: compacted %d days (%d entries), summarized %d, pruned %d",
		compacted.DaysCompacted, compacted.EntriesDeleted, summarized, pruned)
	return nil
},
```

- [ ] **Step 2: Build and verify**

Run: `make build-api`
Expected: Builds successfully.

- [ ] **Step 3: Commit**

```bash
git add internal/server/scheduler.go
git commit -m "feat: run activity compaction during maintenance window"
```

---

### Task 6: Add compactActivityLog to frontend API

**Files:**
- Modify: `web/src/services/activityApi.ts`

- [ ] **Step 1: Add the API function and update types**

In `web/src/services/activityApi.ts`, update the `ActivityEntry` tier type and add the compact function:

```typescript
export interface ActivityEntry {
  id: string;
  timestamp: string;
  tier: 'audit' | 'change' | 'debug' | 'digest';
  type: string;
  level: string;
  source: string;
  operation_id?: string;
  book_id?: string;
  summary: string;
  details?: Record<string, unknown>;
  tags?: string[];
  pruned_at?: string;
}

// ... existing code stays the same ...

export interface CompactResult {
  days_compacted: number;
  entries_deleted: number;
}

export async function compactActivityLog(olderThanDays: number): Promise<CompactResult> {
  const response = await fetch(`${API_BASE}/activity/compact`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ older_than_days: olderThanDays }),
  });
  if (!response.ok) {
    throw new Error(`Failed to compact activity log: ${response.status}`);
  }
  return response.json();
}
```

- [ ] **Step 2: Type check**

Run: `cd web && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/services/activityApi.ts
git commit -m "feat: add compactActivityLog API function and digest tier type"
```

---

### Task 7: Add Compact button and digest row rendering to ActivityLog

**Files:**
- Modify: `web/src/pages/ActivityLog.tsx`

- [ ] **Step 1: Add digest tier color and compact button state**

In `ActivityLog.tsx`, update `TIER_COLORS`:

```typescript
const TIER_COLORS: Record<string, string> = {
  audit: '#1976d2',
  change: '#9c27b0',
  debug: '#757575',
  digest: '#00897b', // teal
};
```

Update the tier chips array (around line 396) from `['audit', 'change', 'debug']` to:

```typescript
{['audit', 'change', 'debug', 'digest'].map((tier) => (
```

Also update `allTiers` (around line 223):

```typescript
const allTiers = ['audit', 'change', 'debug', 'digest'];
```

And the default tiers state (around line 116):

```typescript
const [tiers, setTiers] = useState<Set<string>>(new Set(['audit', 'change', 'digest']));
```

Also update the resetFilters function (around line 381):

```typescript
setTiers(new Set(['audit', 'change', 'digest']));
```

- [ ] **Step 2: Add compact button imports and state**

At the top imports, add `compactActivityLog` import:

```typescript
import { fetchActivity, fetchActivitySources, compactActivityLog } from '../services/activityApi';
```

Add imports for Menu/MenuItem if not already present:

```typescript
import { Menu, MenuItem } from '@mui/material';
```

Add state for the compact menu:

```typescript
const [compactAnchor, setCompactAnchor] = useState<null | HTMLElement>(null);
const [compacting, setCompacting] = useState(false);
```

- [ ] **Step 3: Add compact handler**

Add the compact handler function:

```typescript
const handleCompact = async (days: number) => {
  setCompactAnchor(null);
  setCompacting(true);
  try {
    const result = await compactActivityLog(days);
    // Use window.alert or a simple notification — adapt to your toast system
    alert(`Compacted ${result.days_compacted} days, removed ${result.entries_deleted.toLocaleString()} entries`);
    loadEntries();
  } catch (err) {
    alert(`Compaction failed: ${err}`);
  } finally {
    setCompacting(false);
  }
};
```

Note: If the page uses a toast system, replace `alert()` with that. Check for a `toast` prop or `useSnackbar` hook and use that instead.

- [ ] **Step 4: Add compact button to the toolbar**

Find the toolbar area near the tier chips and source button. Add the Compact button after them:

```tsx
<Button
  size="small"
  variant="outlined"
  disabled={compacting}
  onClick={(e) => setCompactAnchor(e.currentTarget)}
>
  {compacting ? 'Compacting…' : 'Compact'}
</Button>
<Menu
  anchorEl={compactAnchor}
  open={Boolean(compactAnchor)}
  onClose={() => setCompactAnchor(null)}
>
  {[7, 14, 30, 60].map((days) => (
    <MenuItem key={days} onClick={() => handleCompact(days)}>
      Older than {days} days
    </MenuItem>
  ))}
</Menu>
```

- [ ] **Step 5: Add digest row rendering**

In the `entries.map((entry) => ...)` table body rendering, wrap the existing `<TableRow>` in a conditional. Before the existing map, add state for expanded digests:

```typescript
const [expandedDigests, setExpandedDigests] = useState<Set<number>>(new Set());
```

Replace the table body `entries.map` with logic that detects digest rows and renders them differently:

```tsx
{entries.map((entry) => {
  // Digest rows get special expandable rendering
  if (entry.tier === 'digest') {
    const isExpanded = expandedDigests.has(Number(entry.id));
    const details = entry.details as {
      date?: string;
      original_count?: number;
      counts?: Record<string, number>;
      items?: Array<{ type: string; book?: string; book_id?: string; summary: string; details?: string }>;
      truncated?: boolean;
      truncated_count?: number;
    } | undefined;
    const counts = details?.counts || {};
    const items = details?.items || [];

    return (
      <React.Fragment key={entry.id}>
        <TableRow
          hover
          sx={{
            bgcolor: 'rgba(0, 137, 123, 0.06)',
            cursor: 'pointer',
          }}
          onClick={() => {
            setExpandedDigests((prev) => {
              const next = new Set(prev);
              if (next.has(Number(entry.id))) next.delete(Number(entry.id));
              else next.add(Number(entry.id));
              return next;
            });
          }}
        >
          <TableCell sx={{ whiteSpace: 'nowrap', color: 'text.secondary', fontSize: '0.75rem' }}>
            {details?.date || formatTimestamp(entry.timestamp)}
          </TableCell>
          <TableCell>
            <Chip size="small" label="digest" sx={{ bgcolor: TIER_COLORS.digest, color: 'white' }} />
          </TableCell>
          <TableCell>
            <Stack direction="row" spacing={0.5} flexWrap="wrap">
              {Object.entries(counts).slice(0, 6).map(([type, count]) => (
                <Chip key={type} size="small" variant="outlined" label={`${count} ${type.replace(/_/g, ' ')}`} />
              ))}
            </Stack>
          </TableCell>
          <TableCell>
            <Typography variant="body2">
              {entry.summary} {isExpanded ? '▾' : '▸'}
            </Typography>
          </TableCell>
          {!isMobile && <TableCell />}
          {!isMobile && <TableCell />}
          <TableCell />
        </TableRow>
        {isExpanded && (
          <TableRow>
            <TableCell colSpan={isMobile ? 5 : 7} sx={{ py: 0, px: 2 }}>
              <Box sx={{ maxHeight: 400, overflow: 'auto', py: 1 }}>
                {items.map((item, idx) => (
                  <Stack
                    key={idx}
                    direction="row"
                    spacing={1}
                    alignItems="center"
                    sx={{
                      py: 0.5,
                      borderBottom: '1px solid',
                      borderColor: 'divider',
                      color: item.type === 'error' ? 'error.main' : 'text.primary',
                    }}
                  >
                    <Chip size="small" label={item.type.replace(/_/g, ' ')} sx={{ minWidth: 100 }} />
                    {item.book_id ? (
                      <Typography
                        variant="body2"
                        sx={{ cursor: 'pointer', color: 'primary.main', fontWeight: 500 }}
                        onClick={(e) => { e.stopPropagation(); navigate(`/library/${item.book_id}`); }}
                      >
                        {item.book || item.book_id}
                      </Typography>
                    ) : (
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>
                        {item.book || '—'}
                      </Typography>
                    )}
                    <Typography variant="body2" color="text.secondary" sx={{ flex: 1 }}>
                      {item.summary}
                    </Typography>
                    {item.details && (
                      <Typography variant="caption" color="error.main">
                        {item.details}
                      </Typography>
                    )}
                  </Stack>
                ))}
                {details?.truncated && (
                  <Typography variant="caption" color="text.secondary" sx={{ pt: 1, display: 'block' }}>
                    … and {details.truncated_count?.toLocaleString()} more entries not shown
                  </Typography>
                )}
              </Box>
            </TableCell>
          </TableRow>
        )}
      </React.Fragment>
    );
  }

  // Regular entry rendering (existing code)
  return (
    <TableRow
      key={entry.id}
      // ... existing TableRow code unchanged ...
```

Make sure to add `React` to the imports at the top if `React.Fragment` is used (or use `<>...</>` shorthand instead).

- [ ] **Step 6: Type check and build**

Run: `cd web && npx tsc --noEmit`
Expected: No errors.

Run: `cd .. && make build-api`
Expected: Builds with embedded frontend.

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/ActivityLog.tsx
git commit -m "feat: compact button and expandable digest rows in activity log"
```

---

### Task 8: End-to-end verification

- [ ] **Step 1: Run all backend tests**

Run: `go test ./internal/database/ -run TestCompactByDay -v -count=1`
Expected: All tests pass.

Run: `go test ./internal/server/ -count=1 -timeout 120s`
Expected: All tests pass.

- [ ] **Step 2: Type check frontend**

Run: `cd web && npx tsc --noEmit`
Expected: No errors.

- [ ] **Step 3: Full build**

Run: `make build`
Expected: Builds successfully with embedded frontend.

- [ ] **Step 4: Deploy**

Run: `make deploy-debug`
Expected: Deploys and server starts without errors.

- [ ] **Step 5: Verify on production**

1. Open Activity Log page — verify digest tier chip appears in teal
2. Click "Compact" button → select "Older than 7 days"
3. Verify toast shows compaction result
4. Verify digest rows appear with date headers and count chips
5. Click a digest row to expand — verify items list with clickable book links
6. Verify errors are highlighted in red with extra details

- [ ] **Step 6: Final commit if any fixups needed**

```bash
git add -A
git commit -m "fix: activity log compaction polish"
```
