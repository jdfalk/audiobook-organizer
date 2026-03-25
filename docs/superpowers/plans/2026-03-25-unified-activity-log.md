# Unified Activity Log Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace 5 disconnected logging/tracking systems with a single `activity_log` table in a dedicated SQLite sidecar (`activity.db`), exposed via one API and consumed by a unified frontend.

**Architecture:** A new `ActivityLogStore` wraps a standalone SQLite database (`activity.db`) opened alongside the main store. An `ActivityLogService` provides `Record()`, `Query()`, `Summarize()`, and `Prune()`. Existing write paths dual-write to both old tables and the new activity log. The frontend reads from a new `/api/v1/activity` endpoint. A maintenance task handles tier-based retention.

**Tech Stack:** Go + `database/sql` + `github.com/mattn/go-sqlite3`, React + MUI, Gin HTTP framework

**Spec:** `docs/superpowers/specs/2026-03-25-unified-activity-log-design.md`

---

## File Structure

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `internal/database/activity_store.go` | SQLite sidecar: open DB, create table/indexes, CRUD operations |
| Create | `internal/database/activity_store_test.go` | Unit tests for store layer |
| Create | `internal/server/activity_service.go` | Business logic: Record, Query, Summarize, Prune |
| Create | `internal/server/activity_service_test.go` | Unit tests for service layer |
| Create | `internal/server/activity_handlers.go` | HTTP handlers for `/api/v1/activity` |
| Create | `internal/server/activity_handlers_test.go` | HTTP handler tests |
| Modify | `internal/config/config.go` | Add retention config fields |
| Modify | `internal/server/server.go` | Init ActivityLogStore + ActivityService, add to Server struct, register routes, wire dual-write hooks |
| Modify | `internal/server/scheduler.go` | Register `cleanup_activity_log` maintenance task |
| Modify | `internal/operations/queue.go` | Dual-write operation changes to activity log |
| Modify | `internal/logger/operation.go` | Dual-write operation logs to activity log with operation context |
| Modify | `internal/server/metadata_fetch_service.go` | Dual-write metadata changes + path changes + tag writes to activity log |
| Modify | `internal/logger/standard.go` | Dual-write system activity to activity log (full ActivityEntry) |
| Modify | `internal/server/itunes.go` | Dual-write iTunes sync per-book updates to activity log |
| Modify | `internal/server/scan_service.go` | Dual-write scan new-book events to activity log |
| Create | `web/src/pages/ActivityLog.tsx` | Activity feed page with filters |
| Create | `web/src/services/activityApi.ts` | API client functions for activity endpoints |
| Modify | `web/src/App.tsx` | Add `/activity` route |
| Modify | `web/src/components/layout/Sidebar.tsx` | Add "Activity" nav item |
| Modify | `web/src/pages/Operations.tsx` | Changes button queries activity API |
| Modify | `web/src/components/ChangeLog.tsx` | Query activity API instead of changelog endpoint |

**Deferred:** `internal/server/changelog_service.go` and `internal/logger/retention.go` are NOT modified in this plan. The frontend bypasses `changelog_service.go` by calling the activity API directly. The old `PruneOldLogs` continues to handle old-table cleanup independently of the new activity log cleanup task.

---

## Task 1: Activity Store — Schema and Open/Close

**Files:**
- Create: `internal/database/activity_store.go`
- Create: `internal/database/activity_store_test.go`

- [ ] **Step 1: Write the failing test — open and close store**

```go
// internal/database/activity_store_test.go
package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestActivityStore_OpenClose(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "activity.db")
	store, err := NewActivityStore(dbPath)
	if err != nil {
		t.Fatalf("NewActivityStore: %v", err)
	}
	defer store.Close()

	// DB file should exist
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("db file not created: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/database/ -run TestActivityStore_OpenClose -v`
Expected: FAIL — `NewActivityStore` undefined

- [ ] **Step 3: Write minimal implementation — struct, open, schema, close**

```go
// internal/database/activity_store.go
package database

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ActivityStore manages the activity.db sidecar SQLite database.
type ActivityStore struct {
	db *sql.DB
}

// ActivityEntry represents a single activity log row.
type ActivityEntry struct {
	ID          int64          `json:"id"`
	Timestamp   time.Time      `json:"timestamp"`
	Tier        string         `json:"tier"`
	Type        string         `json:"type"`
	Level       string         `json:"level"`
	Source      string         `json:"source"`
	OperationID string         `json:"operation_id,omitempty"`
	BookID      string         `json:"book_id,omitempty"`
	Summary     string         `json:"summary"`
	Details     map[string]any `json:"details,omitempty"`
	Tags        []string       `json:"tags,omitempty"`
	PrunedAt    *time.Time     `json:"pruned_at,omitempty"`
}

// ActivityFilter controls what Query returns.
type ActivityFilter struct {
	Limit       int
	Offset      int
	Type        string
	Tier        string
	Level       string
	OperationID string
	BookID      string
	Since       *time.Time
	Until       *time.Time
	Tags        []string
}

// NewActivityStore opens (or creates) the activity.db SQLite sidecar.
func NewActivityStore(dbPath string) (*ActivityStore, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open activity db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping activity db: %w", err)
	}
	s := &ActivityStore{db: db}
	if err := s.createSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return s, nil
}

func (s *ActivityStore) createSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS activity_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		tier TEXT NOT NULL,
		type TEXT NOT NULL,
		level TEXT NOT NULL DEFAULT 'info',
		source TEXT NOT NULL,
		operation_id TEXT,
		book_id TEXT,
		summary TEXT NOT NULL,
		details JSON,
		tags TEXT,
		pruned_at DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_activity_timestamp ON activity_log(timestamp);
	CREATE INDEX IF NOT EXISTS idx_activity_type_ts ON activity_log(type, timestamp);
	CREATE INDEX IF NOT EXISTS idx_activity_operation ON activity_log(operation_id) WHERE operation_id IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_activity_book_ts ON activity_log(book_id, timestamp) WHERE book_id IS NOT NULL;
	CREATE INDEX IF NOT EXISTS idx_activity_tier ON activity_log(tier);
	CREATE INDEX IF NOT EXISTS idx_activity_tags ON activity_log(tags);
	`
	_, err := s.db.Exec(schema)
	return err
}

// Close closes the underlying database connection.
func (s *ActivityStore) Close() error {
	return s.db.Close()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/database/ -run TestActivityStore_OpenClose -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/database/activity_store.go internal/database/activity_store_test.go
git commit -m "feat: add ActivityStore with SQLite schema for unified activity log"
```

---

## Task 2: Activity Store — Record and Query

**Files:**
- Modify: `internal/database/activity_store.go`
- Modify: `internal/database/activity_store_test.go`

- [ ] **Step 1: Write the failing test — Record + Query round-trip**

```go
func TestActivityStore_RecordAndQuery(t *testing.T) {
	store := newTestActivityStore(t)
	defer store.Close()

	entry := ActivityEntry{
		Tier:        "change",
		Type:        "itunes_sync",
		Level:       "info",
		Source:      "scheduler",
		OperationID: "op-123",
		BookID:      "book-456",
		Summary:     "Sync: 312 updated, 39 new",
		Details:     map[string]any{"updated": 312, "new": 39},
		Tags:        []string{"scheduled", "itunes"},
	}

	id, err := store.Record(entry)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive ID, got %d", id)
	}

	entries, total, err := store.Query(ActivityFilter{Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total=1, got %d", total)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	got := entries[0]
	if got.Tier != "change" || got.Type != "itunes_sync" {
		t.Errorf("tier/type mismatch: %s/%s", got.Tier, got.Type)
	}
	if got.OperationID != "op-123" || got.BookID != "book-456" {
		t.Errorf("operation/book mismatch: %s/%s", got.OperationID, got.BookID)
	}
	if got.Summary != "Sync: 312 updated, 39 new" {
		t.Errorf("summary mismatch: %s", got.Summary)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "scheduled" {
		t.Errorf("tags mismatch: %v", got.Tags)
	}
	v, ok := got.Details["updated"]
	if !ok || v != float64(312) {
		t.Errorf("details.updated mismatch: %v", got.Details)
	}
}

func newTestActivityStore(t *testing.T) *ActivityStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "activity.db")
	store, err := NewActivityStore(dbPath)
	if err != nil {
		t.Fatalf("NewActivityStore: %v", err)
	}
	return store
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/database/ -run TestActivityStore_RecordAndQuery -v`
Expected: FAIL — `Record` and `Query` undefined

- [ ] **Step 3: Implement Record and Query**

Add to `activity_store.go`:

```go
// Record inserts an activity entry. Returns the new row ID.
func (s *ActivityStore) Record(e ActivityEntry) (int64, error) {
	var detailsJSON []byte
	if e.Details != nil {
		var err error
		detailsJSON, err = json.Marshal(e.Details)
		if err != nil {
			return 0, fmt.Errorf("marshal details: %w", err)
		}
	}
	tagsStr := strings.Join(e.Tags, ",")

	ts := e.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	if e.Level == "" {
		e.Level = "info"
	}

	res, err := s.db.Exec(`
		INSERT INTO activity_log (timestamp, tier, type, level, source, operation_id, book_id, summary, details, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ts, e.Tier, e.Type, e.Level, e.Source,
		nullIfEmpty(e.OperationID), nullIfEmpty(e.BookID),
		e.Summary, nullableJSON(detailsJSON), nullIfEmpty(tagsStr),
	)
	if err != nil {
		return 0, fmt.Errorf("insert activity: %w", err)
	}
	return res.LastInsertId()
}

// Query retrieves activity entries matching the filter.
// Returns entries (newest first) and total count.
func (s *ActivityStore) Query(f ActivityFilter) ([]ActivityEntry, int, error) {
	where, args := buildActivityWhere(f)

	// Count
	var total int
	countSQL := "SELECT COUNT(*) FROM activity_log" + where
	if err := s.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count activity: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	querySQL := `SELECT id, timestamp, tier, type, level, source,
		COALESCE(operation_id,''), COALESCE(book_id,''), summary,
		details, COALESCE(tags,''), pruned_at
		FROM activity_log` + where + ` ORDER BY timestamp DESC LIMIT ? OFFSET ?`
	rows, err := s.db.Query(querySQL, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query activity: %w", err)
	}
	defer rows.Close()

	var entries []ActivityEntry
	for rows.Next() {
		var e ActivityEntry
		var detailsRaw sql.NullString
		var tagsRaw string
		var prunedAt sql.NullTime
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Tier, &e.Type, &e.Level, &e.Source,
			&e.OperationID, &e.BookID, &e.Summary,
			&detailsRaw, &tagsRaw, &prunedAt); err != nil {
			return nil, 0, fmt.Errorf("scan activity row: %w", err)
		}
		if detailsRaw.Valid && detailsRaw.String != "" {
			_ = json.Unmarshal([]byte(detailsRaw.String), &e.Details)
		}
		if tagsRaw != "" {
			e.Tags = strings.Split(tagsRaw, ",")
		}
		if prunedAt.Valid {
			e.PrunedAt = &prunedAt.Time
		}
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

func buildActivityWhere(f ActivityFilter) (string, []any) {
	var conds []string
	var args []any
	if f.Type != "" {
		conds = append(conds, "type = ?")
		args = append(args, f.Type)
	}
	if f.Tier != "" {
		conds = append(conds, "tier = ?")
		args = append(args, f.Tier)
	}
	if f.Level != "" {
		conds = append(conds, "level = ?")
		args = append(args, f.Level)
	}
	if f.OperationID != "" {
		conds = append(conds, "operation_id = ?")
		args = append(args, f.OperationID)
	}
	if f.BookID != "" {
		conds = append(conds, "book_id = ?")
		args = append(args, f.BookID)
	}
	if f.Since != nil {
		conds = append(conds, "timestamp >= ?")
		args = append(args, *f.Since)
	}
	if f.Until != nil {
		conds = append(conds, "timestamp <= ?")
		args = append(args, *f.Until)
	}
	for _, tag := range f.Tags {
		conds = append(conds, "(tags LIKE ? OR tags LIKE ? OR tags LIKE ? OR tags = ?)")
		args = append(args, tag+",%", "%,"+tag+",%", "%,"+tag, tag)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullableJSON(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}
```

(Requires adding `"database/sql"`, `"encoding/json"`, `"strings"` to imports.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/database/ -run TestActivityStore_RecordAndQuery -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/database/activity_store.go internal/database/activity_store_test.go
git commit -m "feat: add Record and Query to ActivityStore"
```

---

## Task 3: Activity Store — Query Filters

**Files:**
- Modify: `internal/database/activity_store_test.go`

- [ ] **Step 1: Write the failing tests — filter by tier, type, operation, book, tags, date range**

```go
func TestActivityStore_QueryFilters(t *testing.T) {
	store := newTestActivityStore(t)
	defer store.Close()

	// Insert diverse entries
	entries := []ActivityEntry{
		{Tier: "change", Type: "itunes_sync", Level: "info", Source: "scheduler", OperationID: "op-1", Summary: "sync 1", Tags: []string{"itunes"}},
		{Tier: "change", Type: "metadata_apply", Level: "info", Source: "api", BookID: "book-1", Summary: "apply 1", Tags: []string{"manual"}},
		{Tier: "debug", Type: "progress", Level: "debug", Source: "background", OperationID: "op-1", Summary: "progress 1"},
		{Tier: "audit", Type: "metadata_apply", Level: "info", Source: "manual", BookID: "book-1", Summary: "audit 1", Tags: []string{"itunes", "manual"}},
	}
	for _, e := range entries {
		if _, err := store.Record(e); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	tests := []struct {
		name   string
		filter ActivityFilter
		want   int
	}{
		{"by tier=change", ActivityFilter{Tier: "change"}, 2},
		{"by tier=debug", ActivityFilter{Tier: "debug"}, 1},
		{"by type=metadata_apply", ActivityFilter{Type: "metadata_apply"}, 2},
		{"by operation_id", ActivityFilter{OperationID: "op-1"}, 2},
		{"by book_id", ActivityFilter{BookID: "book-1"}, 2},
		{"by tag=itunes", ActivityFilter{Tags: []string{"itunes"}}, 2},
		{"by tag=manual", ActivityFilter{Tags: []string{"manual"}}, 2},
		{"by two tags", ActivityFilter{Tags: []string{"itunes", "manual"}}, 1},
		{"limit=2", ActivityFilter{Limit: 2}, 2},
		{"offset=2 limit=10", ActivityFilter{Limit: 10, Offset: 2}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := store.Query(tt.filter)
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			if len(got) != tt.want {
				t.Errorf("expected %d entries, got %d", tt.want, len(got))
			}
		})
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/database/ -run TestActivityStore_QueryFilters -v`
Expected: PASS (filters already implemented in Task 2)

- [ ] **Step 3: Commit**

```bash
git add internal/database/activity_store_test.go
git commit -m "test: add query filter tests for ActivityStore"
```

---

## Task 4: Activity Store — Summarize and Prune

**Files:**
- Modify: `internal/database/activity_store.go`
- Modify: `internal/database/activity_store_test.go`

- [ ] **Step 1: Write the failing test — Summarize compresses change entries**

```go
func TestActivityStore_Summarize(t *testing.T) {
	store := newTestActivityStore(t)
	defer store.Close()

	old := time.Now().Add(-100 * 24 * time.Hour)

	// Insert 5 old change entries for same operation
	for i := 0; i < 5; i++ {
		_, err := store.Record(ActivityEntry{
			Timestamp:   old,
			Tier:        "change",
			Type:        "rename",
			Level:       "info",
			Source:      "background",
			OperationID: "op-rename",
			BookID:      fmt.Sprintf("book-%d", i),
			Summary:     fmt.Sprintf("Moved file %d of 5", i+1),
		})
		if err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	// Insert 1 recent change entry (should NOT be summarized)
	_, _ = store.Record(ActivityEntry{
		Tier: "change", Type: "scan", Level: "info", Source: "api",
		Summary: "recent scan",
	})

	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	n, err := store.Summarize(cutoff, "change")
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 summarized, got %d", n)
	}

	// Should have 2 entries now: 1 summary + 1 recent
	entries, total, err := store.Query(ActivityFilter{Limit: 100})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 entries after summarize, got %d", total)
	}

	// Find the summary entry
	var found bool
	for _, e := range entries {
		if e.PrunedAt != nil {
			found = true
			if e.Type != "rename" || e.OperationID != "op-rename" {
				t.Errorf("summary type/op mismatch: %s/%s", e.Type, e.OperationID)
			}
		}
	}
	if !found {
		t.Error("no summary entry with pruned_at set")
	}
}
```

- [ ] **Step 2: Write the failing test — Prune deletes debug entries**

```go
func TestActivityStore_Prune(t *testing.T) {
	store := newTestActivityStore(t)
	defer store.Close()

	old := time.Now().Add(-60 * 24 * time.Hour)

	// Old debug entries
	for i := 0; i < 3; i++ {
		_, _ = store.Record(ActivityEntry{
			Timestamp: old, Tier: "debug", Type: "progress",
			Level: "debug", Source: "background", Summary: fmt.Sprintf("progress %d", i),
		})
	}
	// Old audit entry (should NOT be pruned)
	_, _ = store.Record(ActivityEntry{
		Timestamp: old, Tier: "audit", Type: "metadata_apply",
		Level: "info", Source: "api", Summary: "audit entry",
	})
	// Recent debug entry (should NOT be pruned)
	_, _ = store.Record(ActivityEntry{
		Tier: "debug", Type: "system", Level: "info",
		Source: "background", Summary: "recent",
	})

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	n, err := store.Prune(cutoff, "debug")
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 pruned, got %d", n)
	}

	entries, total, _ := store.Query(ActivityFilter{Limit: 100})
	if total != 2 {
		t.Errorf("expected 2 entries after prune, got %d (entries: %v)", total, entries)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/database/ -run "TestActivityStore_Summarize|TestActivityStore_Prune" -v`
Expected: FAIL — `Summarize` and `Prune` undefined

- [ ] **Step 4: Implement Summarize and Prune**

Add to `activity_store.go`:

```go
// Summarize compresses old change entries into summary rows.
// Groups by (operation_id, type), inserts one summary row per group,
// deletes the originals, and sets pruned_at on the summary.
// Returns the number of original rows replaced.
func (s *ActivityStore) Summarize(olderThan time.Time, tier string) (int, error) {
	// Find groups of old, un-pruned entries for the given tier
	rows, err := s.db.Query(`
		SELECT operation_id, type, COUNT(*) as cnt,
			GROUP_CONCAT(DISTINCT book_id) as book_ids,
			MIN(timestamp) as first_ts
		FROM activity_log
		WHERE tier = ? AND pruned_at IS NULL AND timestamp < ?
		GROUP BY COALESCE(operation_id,''), type
		HAVING cnt > 0`, tier, olderThan)
	if err != nil {
		return 0, fmt.Errorf("find summarizable groups: %w", err)
	}

	type group struct {
		operationID string
		typ         string
		count       int
		bookIDs     string
		firstTS     time.Time
	}
	var groups []group
	for rows.Next() {
		var g group
		var opID sql.NullString
		if err := rows.Scan(&opID, &g.typ, &g.count, &g.bookIDs, &g.firstTS); err != nil {
			rows.Close()
			return 0, err
		}
		if opID.Valid {
			g.operationID = opID.String
		}
		groups = append(groups, g)
	}
	rows.Close()

	totalReplaced := 0
	now := time.Now().UTC()

	for _, g := range groups {
		tx, err := s.db.Begin()
		if err != nil {
			return totalReplaced, err
		}

		// Build summary
		summary := fmt.Sprintf("%s: %d entries", g.typ, g.count)
		details := map[string]any{
			"original_entry_count": g.count,
		}
		if g.bookIDs != "" {
			details["books"] = strings.Split(g.bookIDs, ",")
		}
		detailsJSON, _ := json.Marshal(details)

		// Insert summary row
		_, err = tx.Exec(`
			INSERT INTO activity_log (timestamp, tier, type, level, source, operation_id, summary, details, pruned_at)
			VALUES (?, ?, ?, 'info', 'system', ?, ?, ?, ?)`,
			g.firstTS, tier, g.typ, nullIfEmpty(g.operationID), summary, string(detailsJSON), now)
		if err != nil {
			tx.Rollback()
			return totalReplaced, err
		}

		// Delete originals
		var res sql.Result
		if g.operationID != "" {
			res, err = tx.Exec(`
				DELETE FROM activity_log
				WHERE tier = ? AND type = ? AND operation_id = ? AND pruned_at IS NULL AND timestamp < ?`,
				tier, g.typ, g.operationID, olderThan)
		} else {
			res, err = tx.Exec(`
				DELETE FROM activity_log
				WHERE tier = ? AND type = ? AND operation_id IS NULL AND pruned_at IS NULL AND timestamp < ?`,
				tier, g.typ, olderThan)
		}
		if err != nil {
			tx.Rollback()
			return totalReplaced, err
		}

		deleted, _ := res.RowsAffected()
		totalReplaced += int(deleted)

		if err := tx.Commit(); err != nil {
			return totalReplaced, err
		}
	}

	return totalReplaced, nil
}

// Prune deletes entries of the given tier older than the cutoff.
// Returns the number of rows deleted.
func (s *ActivityStore) Prune(olderThan time.Time, tier string) (int, error) {
	res, err := s.db.Exec(
		"DELETE FROM activity_log WHERE tier = ? AND timestamp < ?",
		tier, olderThan)
	if err != nil {
		return 0, fmt.Errorf("prune activity %s: %w", tier, err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/database/ -run "TestActivityStore_Summarize|TestActivityStore_Prune" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/database/activity_store.go internal/database/activity_store_test.go
git commit -m "feat: add Summarize and Prune to ActivityStore"
```

---

## Task 5: Config — Add Retention Settings

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Add config fields**

Add after `LogRetentionDays` (around line 132):

```go
// Activity log retention (separate from operation log retention)
ActivityLogRetentionChangeDays int `json:"activity_log_retention_change_days"` // default 90
ActivityLogRetentionDebugDays  int `json:"activity_log_retention_debug_days"`  // default 30
```

- [ ] **Step 2: Set defaults**

Find the defaults section (search for `PurgeSoftDeletedAfterDays: 30`) and add:

```go
ActivityLogRetentionChangeDays: 90,
ActivityLogRetentionDebugDays:  30,
```

- [ ] **Step 3: Add viper bindings**

Find where `LogRetentionDays` is loaded from viper and add:

```go
ActivityLogRetentionChangeDays: viper.GetInt("activity_log_retention_change_days"),
ActivityLogRetentionDebugDays:  viper.GetInt("activity_log_retention_debug_days"),
```

- [ ] **Step 4: Run existing tests**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: add activity log retention config fields"
```

---

## Task 6: Activity Service — Business Logic Layer

**Files:**
- Create: `internal/server/activity_service.go`
- Create: `internal/server/activity_service_test.go`

- [ ] **Step 1: Write the failing test — Record and Query through service**

```go
// internal/server/activity_service_test.go
package server

import (
	"path/filepath"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func newTestActivityService(t *testing.T) *ActivityService {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "activity.db")
	store, err := database.NewActivityStore(dbPath)
	if err != nil {
		t.Fatalf("NewActivityStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return NewActivityService(store)
}

func TestActivityService_RecordAndQuery(t *testing.T) {
	svc := newTestActivityService(t)

	err := svc.Record(database.ActivityEntry{
		Tier:    "change",
		Type:    "scan",
		Source:  "api",
		Summary: "Scan found 39 new books",
		Tags:    []string{"import"},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	entries, total, err := svc.Query(database.ActivityFilter{Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d/%d", total, len(entries))
	}
	if entries[0].Type != "scan" {
		t.Errorf("type mismatch: %s", entries[0].Type)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestActivityService_RecordAndQuery -v`
Expected: FAIL — `NewActivityService` undefined

- [ ] **Step 3: Implement ActivityService**

```go
// internal/server/activity_service.go
package server

import (
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// ActivityService provides business logic over the ActivityStore.
type ActivityService struct {
	store *database.ActivityStore
}

// NewActivityService creates a new activity service.
func NewActivityService(store *database.ActivityStore) *ActivityService {
	return &ActivityService{store: store}
}

// Record writes an activity entry. Safe to call from any goroutine (SQLite WAL handles concurrency).
func (s *ActivityService) Record(entry database.ActivityEntry) error {
	_, err := s.store.Record(entry)
	return err
}

// Query retrieves filtered activity entries.
func (s *ActivityService) Query(filter database.ActivityFilter) ([]database.ActivityEntry, int, error) {
	return s.store.Query(filter)
}

// Summarize compresses old entries of the given tier into summary rows.
func (s *ActivityService) Summarize(olderThan time.Time, tier string) (int, error) {
	return s.store.Summarize(olderThan, tier)
}

// Prune deletes old entries of a given tier.
func (s *ActivityService) Prune(olderThan time.Time, tier string) (int, error) {
	return s.store.Prune(olderThan, tier)
}

// Store returns the underlying store (for direct access if needed).
func (s *ActivityService) Store() *database.ActivityStore {
	return s.store
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestActivityService_RecordAndQuery -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/activity_service.go internal/server/activity_service_test.go
git commit -m "feat: add ActivityService business logic layer"
```

---

## Task 7: HTTP Handlers — GET /api/v1/activity

**Files:**
- Create: `internal/server/activity_handlers.go`
- Create: `internal/server/activity_handlers_test.go`

- [ ] **Step 1: Write the failing test — handler returns entries**

```go
// internal/server/activity_handlers_test.go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func setupActivityTestRouter(t *testing.T) (*gin.Engine, *ActivityService) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "activity.db")
	store, err := database.NewActivityStore(dbPath)
	if err != nil {
		t.Fatalf("NewActivityStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	svc := NewActivityService(store)

	r := gin.New()
	s := &Server{activityService: svc}
	r.GET("/api/v1/activity", s.listActivity)
	return r, svc
}

func TestListActivity_Empty(t *testing.T) {
	r, _ := setupActivityTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/activity", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Entries []database.ActivityEntry `json:"entries"`
		Total   int                      `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 0 || len(resp.Entries) != 0 {
		t.Errorf("expected empty, got total=%d entries=%d", resp.Total, len(resp.Entries))
	}
}

func TestListActivity_WithFilters(t *testing.T) {
	r, svc := setupActivityTestRouter(t)

	_ = svc.Record(database.ActivityEntry{Tier: "change", Type: "scan", Source: "api", Summary: "scan 1"})
	_ = svc.Record(database.ActivityEntry{Tier: "debug", Type: "progress", Source: "bg", Summary: "progress 1"})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/activity?tier=change", nil)
	r.ServeHTTP(w, req)

	var resp struct {
		Entries []database.ActivityEntry `json:"entries"`
		Total   int                      `json:"total"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Total != 1 {
		t.Errorf("expected 1 change entry, got %d", resp.Total)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run "TestListActivity" -v`
Expected: FAIL — `activityService` field and `listActivity` undefined

- [ ] **Step 3: Implement the handler**

```go
// internal/server/activity_handlers.go
package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// listActivity handles GET /api/v1/activity
func (s *Server) listActivity(c *gin.Context) {
	filter := database.ActivityFilter{
		Type:        c.Query("type"),
		Tier:        c.Query("tier"),
		Level:       c.Query("level"),
		OperationID: c.Query("operation_id"),
		BookID:      c.Query("book_id"),
	}

	if v := c.Query("limit"); v != "" {
		filter.Limit, _ = strconv.Atoi(v)
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if v := c.Query("offset"); v != "" {
		filter.Offset, _ = strconv.Atoi(v)
	}
	if v := c.Query("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Since = &t
		}
	}
	if v := c.Query("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.Until = &t
		}
	}
	if v := c.Query("tags"); v != "" {
		filter.Tags = strings.Split(v, ",")
	}

	entries, total, err := s.activityService.Query(filter)
	if err != nil {
		internalError(c, err)
		return
	}

	if entries == nil {
		entries = []database.ActivityEntry{}
	}

	c.JSON(http.StatusOK, gin.H{
		"entries": entries,
		"total":   total,
	})
}
```

- [ ] **Step 4: Add `activityService` field to Server struct**

In `internal/server/server.go`, add to the `Server` struct (after `changelogService`):

```go
activityService *ActivityService
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/server/ -run "TestListActivity" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/activity_handlers.go internal/server/activity_handlers_test.go internal/server/server.go
git commit -m "feat: add GET /api/v1/activity HTTP handler"
```

---

## Task 8: Server Wiring — Init Store, Service, Routes

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Initialize ActivityStore in NewServer()**

After the AI scan store initialization block (around line 744-758), add:

```go
// Open activity log sidecar alongside main DB
if dbPath := config.AppConfig.DatabasePath; dbPath != "" {
	activityDBPath := filepath.Join(filepath.Dir(dbPath), "activity.db")
	activityStore, err := database.NewActivityStore(activityDBPath)
	if err != nil {
		log.Printf("[WARN] Failed to open activity log store: %v", err)
	} else {
		server.activityService = NewActivityService(activityStore)
	}
}
```

- [ ] **Step 2: Register the route in setupRoutes()**

Find the protected routes group (around line 1375 with operations routes) and add:

```go
// Activity log
protected.GET("/activity", s.listActivity)
```

- [ ] **Step 3: Add cleanup on shutdown**

Find the `Shutdown` or `Close` method on Server (or the place where `aiScanStore.Close()` is called) and add:

```go
if s.activityService != nil {
	s.activityService.Store().Close()
}
```

- [ ] **Step 4: Run the full test suite**

Run: `make test`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go
git commit -m "feat: wire ActivityStore and activity API route in server"
```

---

## Task 9: Maintenance Task — cleanup_activity_log

**Files:**
- Modify: `internal/server/scheduler.go`

- [ ] **Step 1: Register the maintenance task**

Find the `purge_old_logs` task registration (around line 944) and add a new task after it:

```go
ts.registerTask(TaskDefinition{
	Name:        "cleanup_activity_log",
	Description: "Summarize old change entries and prune old debug entries from activity log",
	Category:    "maintenance",
	TriggerFn: func() (*database.Operation, error) {
		op, err := database.GlobalStore.CreateOperation(
			operations.NewOperationID(), "cleanup_activity_log", nil)
		if err != nil {
			return nil, err
		}
		_ = operations.GlobalQueue.Enqueue(op.ID, "cleanup_activity_log", operations.PriorityLow,
			func(ctx context.Context, progress operations.ProgressReporter) error {
				if ts.server.activityService == nil {
					return nil
				}
				changeDays := config.AppConfig.ActivityLogRetentionChangeDays
				if changeDays <= 0 {
					changeDays = 90
				}
				debugDays := config.AppConfig.ActivityLogRetentionDebugDays
				if debugDays <= 0 {
					debugDays = 30
				}

				changeCutoff := time.Now().AddDate(0, 0, -changeDays)
				debugCutoff := time.Now().AddDate(0, 0, -debugDays)

				summarized, err := ts.server.activityService.Summarize(changeCutoff, "change")
				if err != nil {
					return fmt.Errorf("summarize activity: %w", err)
				}

				pruned, err := ts.server.activityService.Prune(debugCutoff, "debug")
				if err != nil {
					return fmt.Errorf("prune activity: %w", err)
				}

				log.Printf("Activity log cleanup: summarized %d change entries, pruned %d debug entries", summarized, pruned)
				return nil
			},
		)
		return op, nil
	},
	IsEnabled:              func() bool { return ts.server.activityService != nil },
	GetInterval:            func() time.Duration { return 24 * time.Hour },
	RunOnStart:             func() bool { return false },
	RunInMaintenanceWindow: func() bool { return true },
})
```

- [ ] **Step 2: Add to maintenance order**

Find `ts.maintenanceOrder` and add `"cleanup_activity_log"` before `"db_optimize"`:

```go
"cleanup_activity_log",
"db_optimize",
```

- [ ] **Step 3: Run existing scheduler tests**

Run: `go test ./internal/server/ -run "Scheduler" -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/server/scheduler.go
git commit -m "feat: register cleanup_activity_log maintenance task"
```

---

## Task 10: Dual-Write — Operation Changes

**Files:**
- Modify: `internal/operations/queue.go`

- [ ] **Step 1: Add activity log recording to the queue store adapter**

The `queueStoreAdapter` in `queue.go` wraps store calls. Find `CreateOperationChange` (which was fixed from a no-op) and add dual-write logic. The adapter needs access to the activity service.

In `queue.go`, find or add a package-level variable for the activity service:

```go
// ActivityRecorder is set by the server to enable dual-write to the activity log.
var ActivityRecorder func(entry database.ActivityEntry)
```

Then in the `CreateOperationChange` method, after the existing store write succeeds, add:

```go
if ActivityRecorder != nil {
	ActivityRecorder(database.ActivityEntry{
		Tier:        "change",
		Type:        change.ChangeType,
		Level:       "info",
		Source:      "background",
		OperationID: change.OperationID,
		BookID:      change.BookID,
		Summary:     formatChangeSummary(change),
		Details: map[string]any{
			"field":     change.FieldName,
			"old_value": change.OldValue,
			"new_value": change.NewValue,
		},
	})
}
```

Add helper:

```go
func formatChangeSummary(c *database.OperationChange) string {
	if c.FieldName != "" {
		return fmt.Sprintf("%s: %s changed", c.ChangeType, c.FieldName)
	}
	return c.ChangeType
}
```

- [ ] **Step 2: Wire ActivityRecorder in NewServer()**

In `server.go`, after the activity service is created, add:

```go
if server.activityService != nil {
	operations.ActivityRecorder = func(entry database.ActivityEntry) {
		_ = server.activityService.Record(entry)
	}
}
```

- [ ] **Step 3: Run existing queue tests**

Run: `go test ./internal/operations/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/operations/queue.go internal/server/server.go
git commit -m "feat: dual-write operation changes to activity log"
```

---

## Task 11: Dual-Write — Metadata Changes

**Files:**
- Modify: `internal/server/metadata_fetch_service.go`

- [ ] **Step 1: Add dual-write after RecordMetadataChange calls**

Find all calls to `store.RecordMetadataChange(...)` in `metadata_fetch_service.go`. After each successful call, add:

```go
if s.server.activityService != nil {
	_ = s.server.activityService.Record(database.ActivityEntry{
		Tier:   "change",
		Type:   "metadata_apply",
		Level:  "info",
		Source: "background",
		BookID: record.BookID,
		Summary: fmt.Sprintf("Applied %s: %s → %s", record.Field, record.PreviousValue, record.NewValue),
		Details: map[string]any{
			"field":     record.Field,
			"old_value": record.PreviousValue,
			"new_value": record.NewValue,
			"source":    record.Source,
		},
	})
}
```

Note: The `MetadataFetchService` needs access to `server` to reach `activityService`. Check if it already has a `server *Server` field — if not, add one and wire it in `NewServer()`.

- [ ] **Step 2: Run metadata tests**

Run: `go test ./internal/server/ -run "Metadata" -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/server/metadata_fetch_service.go
git commit -m "feat: dual-write metadata changes to activity log"
```

---

## Task 12: Dual-Write — System Activity Log

**Files:**
- Modify: `internal/logger/standard.go`

- [ ] **Step 1: Add activity entry recorder callback type**

The `StandardLogger` already has `activityWriter ActivityLogWriter`. Add a second optional callback for the unified activity log. The callback accepts a full `ActivityEntry` struct so all context (operation ID, book ID, details) is preserved:

```go
// ActivityEntryRecorder writes a structured activity entry to the unified activity log.
// Accepts a full entry so callers can provide operation/book context.
type ActivityEntryRecorder func(entry ActivityLogEntry)

// ActivityLogEntry is a lightweight struct matching database.ActivityEntry fields.
// Defined here to avoid an import cycle between logger and database packages.
type ActivityLogEntry struct {
	Tier        string
	Type        string
	Level       string
	Source      string
	OperationID string
	BookID      string
	Summary     string
	Details     map[string]any
}
```

Add field to `StandardLogger`:

```go
activityRecorder ActivityEntryRecorder
```

- [ ] **Step 2: In the log() method, dual-write system activity**

After the existing `l.activityWriter.AddSystemActivityLog(...)` call, add:

```go
if l.activityRecorder != nil {
	l.activityRecorder(ActivityLogEntry{
		Tier: "debug", Type: "system", Level: level.String(),
		Source: l.subsystem, Summary: formatted,
	})
}
```

- [ ] **Step 3: Add setter and convenience methods**

```go
func (l *StandardLogger) SetActivityRecorder(r ActivityEntryRecorder) {
	l.activityRecorder = r
}

// RecordActivity sends a structured entry to the activity log with full context.
// Use this for operation-scoped events where operation_id/book_id matter.
func (l *StandardLogger) RecordActivity(entry ActivityLogEntry) {
	if l.activityRecorder != nil {
		if entry.Source == "" {
			entry.Source = l.subsystem
		}
		l.activityRecorder(entry)
	}
}
```

- [ ] **Step 4: Write test for dual-write**

```go
func TestStandardLogger_ActivityRecorder(t *testing.T) {
	var recorded []ActivityLogEntry
	logger := New("test")
	logger.SetActivityRecorder(func(entry ActivityLogEntry) {
		recorded = append(recorded, entry)
	})
	logger.Info("hello %s", "world")
	if len(recorded) != 1 {
		t.Fatalf("expected 1 recorded entry, got %d", len(recorded))
	}
	if recorded[0].Summary != "hello world" {
		t.Errorf("expected 'hello world', got %s", recorded[0].Summary)
	}
	if recorded[0].Tier != "debug" || recorded[0].Type != "system" {
		t.Errorf("expected debug/system, got %s/%s", recorded[0].Tier, recorded[0].Type)
	}
}

func TestStandardLogger_RecordActivity(t *testing.T) {
	var recorded []ActivityLogEntry
	logger := New("myservice")
	logger.SetActivityRecorder(func(entry ActivityLogEntry) {
		recorded = append(recorded, entry)
	})
	logger.RecordActivity(ActivityLogEntry{
		Tier: "debug", Type: "progress", Level: "info",
		OperationID: "op-123", Summary: "Processing 45 of 312",
	})
	if len(recorded) != 1 {
		t.Fatalf("expected 1, got %d", len(recorded))
	}
	if recorded[0].OperationID != "op-123" {
		t.Errorf("operation_id lost: %s", recorded[0].OperationID)
	}
	if recorded[0].Source != "myservice" {
		t.Errorf("source not auto-set: %s", recorded[0].Source)
	}
}
```

- [ ] **Step 5: Wire in server.go**

After activity service creation, set the recorder on loggers via a package-level hook:

```go
if server.activityService != nil {
	logger.SetGlobalActivityRecorder(func(entry logger.ActivityLogEntry) {
		_ = server.activityService.Record(database.ActivityEntry{
			Tier: entry.Tier, Type: entry.Type, Level: entry.Level,
			Source: entry.Source, OperationID: entry.OperationID,
			BookID: entry.BookID, Summary: entry.Summary,
			Details: entry.Details,
		})
	})
}
```

- [ ] **Step 6: Run logger tests**

Run: `go test ./internal/logger/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/logger/standard.go internal/logger/standard_test.go internal/server/server.go
git commit -m "feat: dual-write system activity logs to activity log"
```

---

## Task 13: Dual-Write — Operation Logs (AddOperationLog)

**Files:**
- Modify: `internal/logger/operation.go`

The spec lists `store.AddOperationLog(opID, lvl, msg, details)` as a dual-write target mapping to `{Tier:"debug", Type:opType, OperationID:opID, ...}`. Operation logs are written by `OperationLogger` (which wraps the store's `AddOperationLog`). Since Task 12 added `RecordActivity()` with full context support, operation loggers can use it to dual-write with `operationID` preserved.

- [ ] **Step 1: Find the OperationLogger implementation**

Run: `grep -n "AddOperationLog\|OperationLogger" internal/logger/operation.go | head -20`

- [ ] **Step 2: Add dual-write in OperationLogger's log method**

The `OperationLogger` already calls `store.AddOperationLog(opID, level, msg, details)`. After this call, add:

```go
if l.activityRecorder != nil {
	l.activityRecorder(ActivityLogEntry{
		Tier:        "debug",
		Type:        l.operationType, // e.g., "itunes_sync", "scan", "metadata_fetch"
		Level:       level,
		Source:      "background",
		OperationID: l.operationID,
		Summary:     msg,
	})
}
```

If `OperationLogger` doesn't have an `activityRecorder` field, add one and inherit it from the global recorder set in Task 12.

- [ ] **Step 3: Write test**

```go
func TestOperationLogger_ActivityRecorder(t *testing.T) {
	var recorded []ActivityLogEntry
	opLogger := NewOperationLogger("op-test", "scan", nil)
	opLogger.SetActivityRecorder(func(entry ActivityLogEntry) {
		recorded = append(recorded, entry)
	})
	opLogger.Info("Processing book 45 of 312")
	if len(recorded) != 1 {
		t.Fatalf("expected 1, got %d", len(recorded))
	}
	if recorded[0].OperationID != "op-test" {
		t.Errorf("operation_id lost: %s", recorded[0].OperationID)
	}
	if recorded[0].Type != "scan" {
		t.Errorf("type mismatch: %s", recorded[0].Type)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/logger/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/logger/operation.go internal/logger/operation_test.go
git commit -m "feat: dual-write operation logs to activity log with operation context"
```

---

## Task 14: Dual-Write — Path Changes (Renames)

**Files:**
- Modify: `internal/server/metadata_fetch_service.go`

The spec lists `store.RecordPathChange(change)` as a dual-write target. Path changes are recorded during the apply pipeline when files are renamed.

- [ ] **Step 1: Find all RecordPathChange calls**

Run: `grep -n "RecordPathChange\|book_path_history" internal/server/metadata_fetch_service.go`

- [ ] **Step 2: Add dual-write after each RecordPathChange call**

After each successful `store.RecordPathChange(...)` or path history write, add:

```go
if s.server.activityService != nil {
	_ = s.server.activityService.Record(database.ActivityEntry{
		Tier:   "change",
		Type:   "rename",
		Level:  "info",
		Source: "background",
		BookID: bookID,
		Summary: fmt.Sprintf("Moved: %s → %s", oldPath, newPath),
		Details: map[string]any{
			"old_path": oldPath,
			"new_path": newPath,
		},
	})
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/server/ -run "Metadata|Rename" -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/server/metadata_fetch_service.go
git commit -m "feat: dual-write path changes (renames) to activity log"
```

---

## Task 15: Dual-Write — iTunes Sync Per-Book Updates

**Files:**
- Modify: `internal/server/itunes.go` (or wherever the iTunes sync loop writes per-book updates)

- [ ] **Step 1: Find the iTunes sync per-book update loop**

Run: `grep -n "PlayCount\|Rating\|play_count\|rating" internal/server/itunes.go | head -30`

- [ ] **Step 2: Add activity recording for play count and rating changes**

In the sync loop where individual book updates happen, add:

```go
if s.activityService != nil {
	_ = s.activityService.Record(database.ActivityEntry{
		Tier:        "change",
		Type:        "itunes_sync",
		Level:       "info",
		Source:      "scheduler",
		OperationID: operationID,
		BookID:      bookID,
		Summary:     fmt.Sprintf("Updated %s: %v → %v", field, oldVal, newVal),
		Details: map[string]any{
			"field":     field,
			"old_value": oldVal,
			"new_value": newVal,
		},
		Tags: []string{"itunes"},
	})
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/server/ -run "iTunes|Sync" -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/server/itunes.go
git commit -m "feat: dual-write iTunes sync per-book updates to activity log"
```

---

## Task 16: Dual-Write — Scan New Books and Tag Writes

**Files:**
- Modify: `internal/server/scan_service.go` (scan new-book events)
- Modify: `internal/server/metadata_fetch_service.go` (tag write events)

- [ ] **Step 1: Find scan new-book creation points**

Run: `grep -n "CreateBook\|new.*book\|scan.*found" internal/server/scan_service.go | head -20`

- [ ] **Step 2: Add activity recording for new books found during scan**

After each new book is created during a scan:

```go
if s.server.activityService != nil {
	_ = s.server.activityService.Record(database.ActivityEntry{
		Tier:        "change",
		Type:        "scan",
		Level:       "info",
		Source:      "background",
		OperationID: operationID,
		BookID:      newBook.ID,
		Summary:     fmt.Sprintf("Scan found: %s", newBook.Title),
		Details: map[string]any{
			"file_path": newBook.FilePath,
			"format":    newBook.Format,
		},
	})
}
```

- [ ] **Step 3: Find tag write points**

Run: `grep -n "WriteTags\|writeMetadata\|tag_write\|tagWrite" internal/server/metadata_fetch_service.go | head -20`

- [ ] **Step 4: Add activity recording for tag writes**

After each successful tag write:

```go
if s.server.activityService != nil {
	_ = s.server.activityService.Record(database.ActivityEntry{
		Tier:   "change",
		Type:   "tag_write",
		Level:  "info",
		Source: "background",
		BookID: bookID,
		Summary: fmt.Sprintf("Wrote %d tags to %s", tagCount, filepath.Base(filePath)),
		Details: map[string]any{
			"file_path": filePath,
			"tag_count": tagCount,
		},
	})
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/server/ -v -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/scan_service.go internal/server/metadata_fetch_service.go
git commit -m "feat: dual-write scan and tag-write events to activity log"
```

---

## Task 17: Frontend — Activity API Client

**Files:**
- Create: `web/src/services/activityApi.ts`

- [ ] **Step 1: Create the API client**

```typescript
// web/src/services/activityApi.ts

export interface ActivityEntry {
  id: number;
  timestamp: string;
  tier: 'audit' | 'change' | 'debug';
  type: string;
  level: 'debug' | 'info' | 'warn' | 'error';
  source: string;
  operation_id?: string;
  book_id?: string;
  summary: string;
  details?: Record<string, unknown>;
  tags?: string[];
  pruned_at?: string;
}

export interface ActivityResponse {
  entries: ActivityEntry[];
  total: number;
}

export interface ActivityFilter {
  limit?: number;
  offset?: number;
  type?: string;
  tier?: string;
  level?: string;
  operation_id?: string;
  book_id?: string;
  since?: string;
  until?: string;
  tags?: string;
}

const API_BASE = import.meta.env.VITE_API_URL || '/api/v1';

export async function fetchActivity(filter: ActivityFilter = {}): Promise<ActivityResponse> {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filter)) {
    if (value !== undefined && value !== '') {
      params.set(key, String(value));
    }
  }
  const url = `${API_BASE}/activity?${params.toString()}`;
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`Activity API error: ${res.status}`);
  }
  return res.json();
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/services/activityApi.ts
git commit -m "feat: add activity log API client"
```

---

## Task 18: Frontend — Activity Log Page

**Files:**
- Create: `web/src/pages/ActivityLog.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/components/layout/Sidebar.tsx`

- [ ] **Step 1: Create the ActivityLog page component**

```tsx
// web/src/pages/ActivityLog.tsx
import { useCallback, useEffect, useState } from 'react';
import {
  Box, Typography, Table, TableHead, TableBody, TableRow, TableCell,
  Chip, Pagination, Select, MenuItem, FormControl, InputLabel, TextField,
  Stack, Paper, CircularProgress, SelectChangeEvent,
} from '@mui/material';
import { fetchActivity, ActivityEntry, ActivityFilter } from '../services/activityApi';

const TIERS = ['', 'audit', 'change', 'debug'];
const TYPES = ['', 'itunes_sync', 'metadata_apply', 'metadata_fetch', 'tag_write',
  'rename', 'scan', 'organize', 'transcode', 'merge', 'delete', 'import',
  'isbn_enrichment', 'system', 'progress', 'error'];

const tierColor: Record<string, 'default' | 'primary' | 'secondary' | 'warning'> = {
  audit: 'primary',
  change: 'secondary',
  debug: 'default',
};

const levelColor: Record<string, 'default' | 'info' | 'warning' | 'error'> = {
  debug: 'default',
  info: 'info',
  warn: 'warning',
  error: 'error',
};

export default function ActivityLog() {
  const [entries, setEntries] = useState<ActivityEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [tier, setTier] = useState('');
  const [type, setType] = useState('');
  const [operationId, setOperationId] = useState('');
  const limit = 50;

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const filter: ActivityFilter = {
        limit,
        offset: (page - 1) * limit,
      };
      if (tier) filter.tier = tier;
      if (type) filter.type = type;
      if (operationId) filter.operation_id = operationId;
      const data = await fetchActivity(filter);
      setEntries(data.entries);
      setTotal(data.total);
    } catch (err) {
      console.error('Failed to load activity:', err);
    } finally {
      setLoading(false);
    }
  }, [page, tier, type, operationId]);

  useEffect(() => { load(); }, [load]);

  const totalPages = Math.ceil(total / limit);

  return (
    <Box sx={{ p: 2 }}>
      <Typography variant="h5" gutterBottom>Activity Log</Typography>

      <Stack direction="row" spacing={2} sx={{ mb: 2 }} alignItems="center">
        <FormControl size="small" sx={{ minWidth: 120 }}>
          <InputLabel>Tier</InputLabel>
          <Select value={tier} label="Tier" onChange={(e: SelectChangeEvent) => { setTier(e.target.value); setPage(1); }}>
            {TIERS.map(t => <MenuItem key={t} value={t}>{t || 'All'}</MenuItem>)}
          </Select>
        </FormControl>

        <FormControl size="small" sx={{ minWidth: 150 }}>
          <InputLabel>Type</InputLabel>
          <Select value={type} label="Type" onChange={(e: SelectChangeEvent) => { setType(e.target.value); setPage(1); }}>
            {TYPES.map(t => <MenuItem key={t} value={t}>{t || 'All'}</MenuItem>)}
          </Select>
        </FormControl>

        <TextField
          size="small"
          label="Operation ID"
          value={operationId}
          onChange={(e) => { setOperationId(e.target.value); setPage(1); }}
          sx={{ width: 200 }}
        />

        <Typography variant="body2" color="text.secondary">
          {total} entries
        </Typography>
      </Stack>

      <Paper>
        {loading ? (
          <Box sx={{ p: 4, textAlign: 'center' }}><CircularProgress /></Box>
        ) : (
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Time</TableCell>
                <TableCell>Tier</TableCell>
                <TableCell>Type</TableCell>
                <TableCell>Level</TableCell>
                <TableCell>Summary</TableCell>
                <TableCell>Source</TableCell>
                <TableCell>Tags</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {entries.map((e) => (
                <TableRow key={e.id} hover>
                  <TableCell sx={{ whiteSpace: 'nowrap' }}>
                    {new Date(e.timestamp).toLocaleString()}
                  </TableCell>
                  <TableCell>
                    <Chip label={e.tier} size="small" color={tierColor[e.tier] || 'default'} />
                  </TableCell>
                  <TableCell>{e.type}</TableCell>
                  <TableCell>
                    <Chip label={e.level} size="small" color={levelColor[e.level] || 'default'} variant="outlined" />
                  </TableCell>
                  <TableCell>{e.summary}</TableCell>
                  <TableCell>{e.source}</TableCell>
                  <TableCell>
                    {e.tags?.map(tag => (
                      <Chip key={tag} label={tag} size="small" sx={{ mr: 0.5 }} />
                    ))}
                  </TableCell>
                </TableRow>
              ))}
              {entries.length === 0 && (
                <TableRow>
                  <TableCell colSpan={7} align="center">No activity entries found</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        )}
      </Paper>

      {totalPages > 1 && (
        <Box sx={{ mt: 2, display: 'flex', justifyContent: 'center' }}>
          <Pagination count={totalPages} page={page} onChange={(_, p) => setPage(p)} />
        </Box>
      )}
    </Box>
  );
}
```

- [ ] **Step 2: Add route to App.tsx**

In `web/src/App.tsx`, add the import and route:

```tsx
import ActivityLog from './pages/ActivityLog';
// In Routes:
<Route path="/activity" element={<ActivityLog />} />
```

- [ ] **Step 3: Add nav item to Sidebar.tsx**

In `web/src/components/layout/Sidebar.tsx`, find the `menuItems` array and add after "Operations":

```tsx
{ text: 'Activity', path: '/activity', icon: <TimelineIcon /> },
```

Add the import:

```tsx
import TimelineIcon from '@mui/icons-material/Timeline';
```

- [ ] **Step 4: Build frontend to verify**

Run: `cd web && npm run build`
Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/ActivityLog.tsx web/src/App.tsx web/src/components/layout/Sidebar.tsx
git commit -m "feat: add Activity Log page with filtering"
```

---

## Task 19: Frontend — Operations Changes Button Uses Activity API

**Files:**
- Modify: `web/src/pages/Operations.tsx`

- [ ] **Step 1: Update handleToggleChanges to use activity API**

Replace the `getOperationChanges(opId)` call with `fetchActivity({ operation_id: opId, tier: 'change' })`.

Update the changes rendering to use `ActivityEntry[]` instead of `OperationChange[]`.

```tsx
import { fetchActivity, ActivityEntry } from '../services/activityApi';

// Replace changes state type:
const [changes, setChanges] = useState<ActivityEntry[]>([]);

// Update handleToggleChanges:
const handleToggleChanges = async (opId: string) => {
  if (expandedChanges === opId) {
    setExpandedChanges(null);
    return;
  }
  setExpandedChanges(opId);
  setChangesLoading(true);
  try {
    const data = await fetchActivity({ operation_id: opId, tier: 'change', limit: 100 });
    setChanges(data.entries);
  } catch (error) {
    console.error('Failed to load changes', error);
    setChanges([]);
  } finally {
    setChangesLoading(false);
  }
};
```

Update the table rendering to show `entry.type`, `entry.summary`, `entry.timestamp` instead of `change.change_type`, `change.field_name`, `change.old_value`, `change.new_value`.

- [ ] **Step 2: Build and verify**

Run: `cd web && npm run build`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/Operations.tsx
git commit -m "feat: operations Changes button reads from activity log API"
```

---

## Task 20: Frontend — ChangeLog Component Uses Activity API

**Files:**
- Modify: `web/src/components/ChangeLog.tsx`

- [ ] **Step 1: Update ChangeLog to fetch from activity API**

Replace the `getBookChangelog(bookId)` call with `fetchActivity({ book_id: bookId, tier: 'change', limit: 50 })`.

Map `ActivityEntry` to the existing `ChangeLogEntry` interface to minimize UI changes:

```tsx
import { fetchActivity } from '../services/activityApi';

// In the fetch logic:
const data = await fetchActivity({ book_id: bookId, tier: 'change', limit: 50 });
const mapped: ChangeLogEntry[] = data.entries.map(e => ({
  timestamp: e.timestamp,
  type: e.type as ChangeLogEntry['type'],
  summary: e.summary,
  details: e.details,
}));
setEntries(mapped);
```

- [ ] **Step 2: Build and verify**

Run: `cd web && npm run build`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add web/src/components/ChangeLog.tsx
git commit -m "feat: ChangeLog component reads from activity log API"
```

---

## Task 21: Integration Test — Full Round-Trip

**Files:**
- Create: `internal/server/activity_integration_test.go`

- [ ] **Step 1: Write integration test — record via service, query via HTTP**

```go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestActivity_Integration_RecordAndHTTPQuery(t *testing.T) {
	r, svc := setupActivityTestRouter(t)

	// Simulate an iTunes sync writing activity
	_ = svc.Record(database.ActivityEntry{
		Tier:        "change",
		Type:        "itunes_sync",
		Source:      "scheduler",
		OperationID: "op-sync-1",
		Summary:     "Sync: 312 updated, 39 new",
		Details:     map[string]any{"updated": 312, "new": 39},
		Tags:        []string{"scheduled", "itunes"},
	})

	// Simulate a metadata apply
	_ = svc.Record(database.ActivityEntry{
		Tier:   "change",
		Type:   "metadata_apply",
		Source: "api",
		BookID: "book-1",
		Summary: "Applied title: old → new",
	})

	// Simulate debug progress
	_ = svc.Record(database.ActivityEntry{
		Tier:        "debug",
		Type:        "progress",
		Source:      "background",
		OperationID: "op-sync-1",
		Summary:     "Processing book 45 of 312...",
	})

	// Query all
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/activity?limit=100", nil)
	r.ServeHTTP(w, req)

	var resp struct {
		Entries []database.ActivityEntry `json:"entries"`
		Total   int                      `json:"total"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Total != 3 {
		t.Fatalf("expected 3, got %d", resp.Total)
	}

	// Query by operation
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/activity?operation_id=op-sync-1", nil)
	r.ServeHTTP(w, req)
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 2 {
		t.Errorf("expected 2 entries for op-sync-1, got %d", resp.Total)
	}

	// Query by book
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/activity?book_id=book-1&tier=change", nil)
	r.ServeHTTP(w, req)
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 {
		t.Errorf("expected 1 entry for book-1, got %d", resp.Total)
	}

	// Query by tags
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/activity?tags=itunes", nil)
	r.ServeHTTP(w, req)
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 {
		t.Errorf("expected 1 entry with tag=itunes, got %d", resp.Total)
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test ./internal/server/ -run TestActivity_Integration -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/server/activity_integration_test.go
git commit -m "test: add activity log integration test (record + HTTP query)"
```

---

## Task 22: Full Build Verification

- [ ] **Step 1: Run backend tests**

Run: `make test`
Expected: PASS

- [ ] **Step 2: Run frontend build**

Run: `make build`
Expected: PASS (full build with embedded frontend)

- [ ] **Step 3: Verify no regressions**

Run: `make ci`
Expected: PASS

- [ ] **Step 4: Final commit if any fixups needed**

```bash
git add -A && git commit -m "fix: address build/test issues from activity log implementation"
```

---

## Summary

| Task | Description | Files | Estimated Steps |
|------|-------------|-------|-----------------|
| 1 | Store schema + open/close | `activity_store.go`, test | 5 |
| 2 | Store Record + Query | `activity_store.go`, test | 5 |
| 3 | Store query filter tests | test | 3 |
| 4 | Store Summarize + Prune | `activity_store.go`, test | 6 |
| 5 | Config retention fields | `config.go` | 5 |
| 6 | Activity service layer | `activity_service.go`, test | 5 |
| 7 | HTTP handler | `activity_handlers.go`, test | 6 |
| 8 | Server wiring | `server.go` | 5 |
| 9 | Maintenance task | `scheduler.go` | 4 |
| 10 | Dual-write: operation changes | `queue.go`, `server.go` | 4 |
| 11 | Dual-write: metadata changes | `metadata_fetch_service.go` | 3 |
| 12 | Dual-write: system activity | `standard.go`, `server.go` | 7 |
| 13 | Dual-write: operation logs | `operation.go` | 5 |
| 14 | Dual-write: path changes (renames) | `metadata_fetch_service.go` | 4 |
| 15 | Dual-write: iTunes sync per-book | `itunes.go` | 4 |
| 16 | Dual-write: scan + tag writes | `scan_service.go`, `metadata_fetch_service.go` | 6 |
| 17 | Frontend API client | `activityApi.ts` | 2 |
| 18 | Frontend Activity page | `ActivityLog.tsx`, routing | 5 |
| 19 | Frontend Operations changes | `Operations.tsx` | 3 |
| 20 | Frontend ChangeLog | `ChangeLog.tsx` | 3 |
| 21 | Integration test | `activity_integration_test.go` | 3 |
| 22 | Full build verification | — | 4 |
