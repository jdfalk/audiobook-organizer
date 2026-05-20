// file: internal/database/activity_store.go
// version: 1.10.0
// guid: e2d3f4a5-b6c7-8d9e-0f1a-2b3c4d5e6f7a

package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ActivityEntry represents a single entry in the unified activity log.
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

// ActivityFilter controls which entries Query returns.
type ActivityFilter struct {
	Limit          int
	Offset         int
	Type           string
	Tier           string
	Level          string
	OperationID    string
	BookID         string
	Since          *time.Time
	Until          *time.Time
	Tags           []string
	Search         string   // LIKE %search% on summary
	Source         string   // show only this source
	ExcludeSources []string // hide these sources
	ExcludeTiers   []string // hide these tiers
	ExcludeTags    []string // hide entries that carry any of these tags
}

// CompactResult holds the outcome of a CompactByDay operation.
type CompactResult struct {
	DaysCompacted  int `json:"days_compacted"`
	EntriesDeleted int `json:"entries_deleted"`
}

// DigestItem represents a single compacted entry within a daily digest.
type DigestItem struct {
	Type        string `json:"type"`
	Tier        string `json:"tier,omitempty"`
	Book        string `json:"book,omitempty"`
	BookID      string `json:"book_id,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
	Summary     string `json:"summary"`
	Details     string `json:"details,omitempty"`
}

// DigestDetails is the JSON structure stored in a daily digest row's details column.
type DigestDetails struct {
	Date           string                       `json:"date"`
	OriginalCount  int                          `json:"original_count"`
	Counts         map[string]int               `json:"counts"`
	// TagCounts aggregates entry counts grouped by tag namespace → tag value.
	// Outer key is a namespace like "action" or "source"; inner key is the
	// tag value (e.g. "metadata-apply", "scan"). Used by the frontend as a
	// fallback breakdown when Counts has only one key (e.g. legacy "system_log"
	// entries whose type was not yet classified at compaction time).
	TagCounts      map[string]map[string]int    `json:"tag_counts,omitempty"`
	Items          []DigestItem                 `json:"items"`
	Truncated      bool                         `json:"truncated,omitempty"`
	TruncatedCount int                          `json:"truncated_count,omitempty"`
}

const maxDigestItems = 500

// ActivityStore persists activity log entries in a dedicated SQLite sidecar database.
type ActivityStore struct {
	db *sql.DB
}

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
`

// NewActivityStore opens (or creates) the SQLite activity log at dbPath.
// WAL mode + 30 s busy timeout + BEGIN IMMEDIATE for all transactions.
// _txlock=immediate is load-bearing: CompactByDay starts a tx with a SELECT
// (read), then upgrades to a write on the first DELETE. Under deferred BEGIN
// a concurrent INSERT from Record() can grab the write lock during that
// SELECT window, after which our DELETE upgrade fails with "database is
// locked" instead of waiting on busy_timeout. IMMEDIATE acquires the write
// lock at BEGIN so subsequent writers queue on busy_timeout cleanly.
func NewActivityStore(dbPath string) (*ActivityStore, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=30000&_txlock=immediate&_foreign_keys=off", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("activity_store: open %q: %w", dbPath, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("activity_store: ping %q: %w", dbPath, err)
	}
	if _, err := db.Exec(activitySchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("activity_store: schema: %w", err)
	}
	// Migrate: add compacted column if missing (idempotent)
	// Ignore "duplicate column name" errors since this is idempotent
	if _, err := db.Exec(`ALTER TABLE activity_log ADD COLUMN compacted BOOLEAN NOT NULL DEFAULT 0`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		db.Close()
		return nil, fmt.Errorf("activity_store: add compacted column: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_activity_compacted ON activity_log (compacted)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("activity_store: create compacted index: %w", err)
	}

	// Fix digest timestamps from 23:59:59 to 00:00:00 (one-shot, idempotent)
	if _, err := db.Exec(`UPDATE activity_log SET timestamp = date(timestamp) || ' 00:00:00'
		WHERE tier = 'digest' AND type = 'daily_digest' AND time(timestamp) = '23:59:59'`); err != nil {
		db.Close()
		return nil, fmt.Errorf("activity_store: fix digest timestamps: %w", err)
	}

	return &ActivityStore{db: db}, nil
}

// Close shuts down the underlying database connection.
func (s *ActivityStore) Close() error {
	return s.db.Close()
}

// MigrateSystemActivityLogs backfills old SQLite system_activity_log entries to the unified store.
// Reads from mainSQLiteStore (the old database), transforms each row to ActivityEntry schema,
// and inserts. Idempotent: checks for migration marker entry to avoid re-running.
// Returns count of migrated rows, or 0 if already done.
func (s *ActivityStore) MigrateSystemActivityLogs() (int, error) {
	// Check if already migrated
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM activity_log WHERE tier = 'system' AND type = 'migration_complete' AND tags LIKE '%migration%'`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("activity_store: check migration marker: %w", err)
	}
	if count > 0 {
		// Already migrated
		slog.Info("[activity] system_activity_log migration already complete")
		return 0, nil
	}

	// Read all old system_activity_log rows from the same database
	query := "SELECT id, source, level, message, created_at FROM system_activity_log ORDER BY created_at ASC"
	rows, err := s.db.Query(query)
	if err != nil {
		// Table might not exist — that's OK, migration is a no-op
		if strings.Contains(err.Error(), "no such table") {
			slog.Info("[activity] system_activity_log table not found (no legacy logs)")
			return 0, nil
		}
		return 0, fmt.Errorf("activity_store: query system_activity_log: %w", err)
	}
	defer rows.Close()

	var oldLogs []struct {
		ID        int64
		Source    string
		Level     string
		Message   string
		CreatedAt time.Time
	}
	for rows.Next() {
		var l struct {
			ID        int64
			Source    string
			Level     string
			Message   string
			CreatedAt time.Time
		}
		if err := rows.Scan(&l.ID, &l.Source, &l.Level, &l.Message, &l.CreatedAt); err != nil {
			return 0, fmt.Errorf("activity_store: scan system_activity_log row: %w", err)
		}
		oldLogs = append(oldLogs, l)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("activity_store: scan system_activity_log: %w", err)
	}

	if len(oldLogs) == 0 {
		slog.Info("[activity] no old system_activity_log rows found")
		// Write marker anyway so we don't check again
		_, _ = s.Record(ActivityEntry{
			Tier:    "system",
			Type:    "migration_complete",
			Summary: "Migrated 0 system_activity_log rows (none found)",
			Tags:    []string{"migration"},
		})
		return 0, nil
	}

	// Insert in a transaction to ensure atomicity
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return 0, fmt.Errorf("activity_store: begin transaction: %w", err)
	}
	defer tx.Rollback()

	insertedCount := 0
	for _, old := range oldLogs {
		// Map old schema to new ActivityEntry.
		// Type and Tier are derived from the message content so daily digests
		// can produce meaningful breakdown chips instead of a single "system_log" chip.
		derivedType, derivedTier := deriveTypeFromMessage(old.Message, old.Source)
		entry := ActivityEntry{
			Timestamp: old.CreatedAt,
			Tier:      derivedTier,
			Type:      derivedType,
			Level:     old.Level,
			Source:    old.Source,
			Summary:   old.Message,
			Tags:      enrichLegacyLogTags(old.Message, old.Source, old.Level),
		}

		tagsStr := strings.Join(entry.Tags, ",")
		_, err := tx.Exec(`
			INSERT INTO activity_log
				(timestamp, tier, type, level, source, summary, tags)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			entry.Timestamp.UTC(),
			entry.Tier,
			entry.Type,
			entry.Level,
			entry.Source,
			entry.Summary,
			tagsStr,
		)
		if err != nil {
			return 0, fmt.Errorf("activity_store: insert migrated row: %w", err)
		}
		insertedCount++
	}

	// Write marker entry
	markerStr := fmt.Sprintf("Migrated %d system_activity_log rows from legacy store", insertedCount)
	_, err = tx.Exec(`
		INSERT INTO activity_log
			(timestamp, tier, type, level, source, summary, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		time.Now().UTC(),
		"system",
		"migration_complete",
		"info",
		"activity_store",
		markerStr,
		"migration",
	)
	if err != nil {
		return 0, fmt.Errorf("activity_store: insert migration marker: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("activity_store: commit migration: %w", err)
	}

	slog.Info("[activity] migrated system_activity_log rows", "insertedCount", insertedCount)
	return insertedCount, nil
}

// Record inserts an ActivityEntry and returns its auto-assigned ID.
// Defaults: Timestamp → now, Level → "info".
func (s *ActivityStore) Record(e ActivityEntry) (int64, error) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	if e.Level == "" {
		e.Level = "info"
	}

	detailsJSON, err := nullableJSON(e.Details)
	if err != nil {
		return 0, fmt.Errorf("activity_store: marshal details: %w", err)
	}

	tagsStr := nullIfEmpty(strings.Join(e.Tags, ","))

	res, err := s.db.Exec(`
		INSERT INTO activity_log
			(timestamp, tier, type, level, source, operation_id, book_id,
			 summary, details, tags, pruned_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Timestamp.UTC(),
		e.Tier,
		e.Type,
		e.Level,
		e.Source,
		nullIfEmpty(e.OperationID),
		nullIfEmpty(e.BookID),
		e.Summary,
		detailsJSON,
		tagsStr,
		(*string)(nil), // pruned_at always NULL on insert
	)
	if err != nil {
		return 0, fmt.Errorf("activity_store: insert: %w", err)
	}
	return res.LastInsertId()
}

// Query returns entries matching f, newest-first, plus the total matching count.
// Default limit is 50 when f.Limit == 0.
func (s *ActivityStore) Query(f ActivityFilter) ([]ActivityEntry, int, error) {
	if f.Limit == 0 {
		f.Limit = 50
	}

	where, args := buildActivityWhere(f)

	// Count
	countQuery := "SELECT COUNT(*) FROM activity_log" + where
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("activity_store: count: %w", err)
	}

	// Fetch
	dataQuery := `SELECT id, timestamp, tier, type, level, source,
		operation_id, book_id, summary, details, tags, pruned_at
		FROM activity_log` + where + ` ORDER BY compacted ASC, timestamp DESC LIMIT ? OFFSET ?`
	dataArgs := append(args, f.Limit, f.Offset)

	rows, err := s.db.Query(dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("activity_store: query: %w", err)
	}
	defer rows.Close()

	var entries []ActivityEntry
	for rows.Next() {
		var (
			e          ActivityEntry
			ts         time.Time
			opID       sql.NullString
			bookID     sql.NullString
			detailsRaw sql.NullString
			tagsRaw    sql.NullString
			prunedAt   sql.NullTime
		)
		if err := rows.Scan(
			&e.ID, &ts, &e.Tier, &e.Type, &e.Level, &e.Source,
			&opID, &bookID, &e.Summary, &detailsRaw, &tagsRaw, &prunedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("activity_store: scan: %w", err)
		}
		e.Timestamp = ts.UTC()
		if opID.Valid {
			e.OperationID = opID.String
		}
		if bookID.Valid {
			e.BookID = bookID.String
		}
		if detailsRaw.Valid && detailsRaw.String != "" {
			if err := json.Unmarshal([]byte(detailsRaw.String), &e.Details); err != nil {
				return nil, 0, fmt.Errorf("activity_store: unmarshal details id=%d: %w", e.ID, err)
			}
		}
		if tagsRaw.Valid && tagsRaw.String != "" {
			e.Tags = strings.Split(tagsRaw.String, ",")
		}
		if prunedAt.Valid {
			t := prunedAt.Time.UTC()
			e.PrunedAt = &t
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("activity_store: rows: %w", err)
	}
	return entries, total, nil
}

// Summarize collapses old unpruned entries (older than olderThan, matching tier)
// into one summary row per (operation_id, type) group. Returns the count of
// original rows deleted.
func (s *ActivityStore) Summarize(ctx context.Context, olderThan time.Time, tier string) (int, error) {
	var err error
	defer func() {
		if err != nil {
			// This acts as a placeholder for proper defer logic
		}
	}()

	// Fetch groups that qualify
	groupRows, err := s.db.Query(`
		SELECT operation_id, type, COUNT(*) AS cnt,
		       MIN(timestamp) AS first_ts, MAX(timestamp) AS last_ts
		FROM   activity_log
		WHERE  tier = ?
		  AND  timestamp < ?
		  AND  pruned_at IS NULL
		GROUP BY operation_id, type`,
		tier, olderThan.UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("activity_store: summarize groups: %w", err)
	}

	type group struct {
		opID    sql.NullString
		typ     string
		cnt     int
		firstTS string
		lastTS  string
	}
	var groups []group
	for groupRows.Next() {
		var g group
		if err := groupRows.Scan(&g.opID, &g.typ, &g.cnt, &g.firstTS, &g.lastTS); err != nil {
			groupRows.Close()
			return 0, fmt.Errorf("activity_store: summarize scan group: %w", err)
		}
		groups = append(groups, g)
	}
	groupRows.Close()
	if err := groupRows.Err(); err != nil {
		return 0, fmt.Errorf("activity_store: summarize group rows: %w", err)
	}

	now := time.Now().UTC()
	totalDeleted := 0

	for _, g := range groups {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return totalDeleted, fmt.Errorf("activity_store: summarize begin tx: %w", err)
		}

		summaryText := fmt.Sprintf("Summary: %d %s entries (%s to %s)",
			g.cnt, g.typ, g.firstTS, g.lastTS,
		)

		// Insert summary row
		_, txErr := tx.ExecContext(ctx, `
			INSERT INTO activity_log
				(timestamp, tier, type, level, source, operation_id,
				 summary, pruned_at)
			VALUES (?, ?, ?, 'info', 'summarize', ?, ?, ?)`,
			now, tier, g.typ, g.opID, summaryText, now,
		)
		if txErr != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.Info("Summarize rollback on insert", "rbErr", rbErr)
			}
			return totalDeleted, fmt.Errorf("activity_store: summarize insert: %w", txErr)
		}

		// Delete originals
		var res sql.Result
		if g.opID.Valid {
			res, txErr = tx.ExecContext(ctx, `
				DELETE FROM activity_log
				WHERE tier = ? AND type = ? AND operation_id = ?
				  AND timestamp < ? AND pruned_at IS NULL`,
				tier, g.typ, g.opID.String, olderThan.UTC(),
			)
		} else {
			res, txErr = tx.ExecContext(ctx, `
				DELETE FROM activity_log
				WHERE tier = ? AND type = ? AND operation_id IS NULL
				  AND timestamp < ? AND pruned_at IS NULL`,
				tier, g.typ, olderThan.UTC(),
			)
		}
		if txErr != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.Info("Summarize rollback on delete", "rbErr", rbErr)
			}
			return totalDeleted, fmt.Errorf("activity_store: summarize delete: %w", txErr)
		}

		n, _ := res.RowsAffected()
		if txErr = tx.Commit(); txErr != nil {
			return totalDeleted, fmt.Errorf("activity_store: summarize commit: %w", txErr)
		}
		totalDeleted += int(n)
	}

	return totalDeleted, nil
}

// Prune hard-deletes all entries of the given tier older than olderThan.
// Returns the number of rows deleted.
func (s *ActivityStore) Prune(olderThan time.Time, tier string) (int, error) {
	res, err := s.db.Exec(`
		DELETE FROM activity_log
		WHERE tier = ? AND timestamp < ?`,
		tier, olderThan.UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("activity_store: prune: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// SourceCount holds a source name and how many activity entries it has.
type SourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// GetDistinctSources returns all unique sources with their entry counts,
// ordered by count descending, optionally narrowed by f.
func (s *ActivityStore) GetDistinctSources(f ActivityFilter) ([]SourceCount, error) {
	where, args := buildActivityWhere(f)
	query := "SELECT source, COUNT(*) as cnt FROM activity_log" + where + " GROUP BY source ORDER BY cnt DESC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get distinct sources: %w", err)
	}
	defer rows.Close()
	var sources []SourceCount
	for rows.Next() {
		var sc SourceCount
		if err := rows.Scan(&sc.Source, &sc.Count); err != nil {
			return nil, err
		}
		sources = append(sources, sc)
	}
	return sources, rows.Err()
}

// WipeAllActivity deletes every row from activity_log and returns the row count.
func (s *ActivityStore) WipeAllActivity() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM activity_log`)
	if err != nil {
		return 0, fmt.Errorf("activity_store: wipe: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// buildActivityWhere constructs a WHERE clause and positional args from f.
// Tag filters require ALL requested tags to be present (AND semantics).
func buildActivityWhere(f ActivityFilter) (string, []any) {
	var clauses []string
	var args []any

	if f.Tier != "" {
		clauses = append(clauses, "tier = ?")
		args = append(args, f.Tier)
	}
	if f.Type != "" {
		clauses = append(clauses, "type = ?")
		args = append(args, f.Type)
	}
	if f.Level != "" {
		clauses = append(clauses, "level = ?")
		args = append(args, f.Level)
	}
	if f.OperationID != "" {
		clauses = append(clauses, "operation_id = ?")
		args = append(args, f.OperationID)
	}
	if f.BookID != "" {
		clauses = append(clauses, "book_id = ?")
		args = append(args, f.BookID)
	}
	if f.Since != nil {
		clauses = append(clauses, "timestamp >= ?")
		args = append(args, f.Since.UTC())
	}
	if f.Until != nil {
		clauses = append(clauses, "timestamp <= ?")
		args = append(args, f.Until.UTC())
	}
	// Each requested tag must appear in the comma-separated tags column.
	// Patterns handle: exact match, prefix, suffix, and middle.
	for _, tag := range f.Tags {
		t := tag
		clause := "(tags = ? OR tags LIKE ? OR tags LIKE ? OR tags LIKE ?)"
		clauses = append(clauses, clause)
		args = append(args,
			t,           // exact: "alpha"
			t+",%",      // prefix: "alpha,..."
			"%,"+t+",%", // middle: "...,alpha,..."
			"%,"+t,      // suffix: "...,alpha"
		)
	}
	if f.Search != "" {
		clauses = append(clauses, "summary LIKE ?")
		args = append(args, "%"+f.Search+"%")
	}
	if f.Source != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, f.Source)
	}
	for _, src := range f.ExcludeSources {
		clauses = append(clauses, "(source != ? OR source IS NULL)")
		args = append(args, src)
	}
	for _, tier := range f.ExcludeTiers {
		clauses = append(clauses, "tier != ?")
		args = append(args, tier)
	}
	for _, tag := range f.ExcludeTags {
		t := tag
		clause := "(tags IS NULL OR (tags != ? AND tags NOT LIKE ? AND tags NOT LIKE ? AND tags NOT LIKE ?))"
		clauses = append(clauses, clause)
		args = append(args,
			t,           // exact: "no-op"
			t+",%",      // prefix: "no-op,..."
			"%,"+t+",%", // middle: "...,no-op,..."
			"%,"+t,      // suffix: "...,no-op"
		)
	}

	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// nullIfEmpty returns nil if s is empty, otherwise s.
// Useful for optional text columns that should be NULL rather than "".
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullableJSON marshals v to a JSON string. Returns nil if v is nil/empty map.
func nullableJSON(v map[string]any) (any, error) {
	if len(v) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// CompactByDay collapses old change, debug, and audit entries into one
// daily_digest row per UTC day. Audit entries are folded into the digest
// (preserving their type, summary, and operation_id in DigestItem) so the
// audit record survives compaction in summarized form. Existing 'digest'
// rows are never re-compacted. Each day is processed in its own transaction
// for atomicity.
func (s *ActivityStore) CompactByDay(ctx context.Context, olderThan time.Time) (CompactResult, error) {
	var result CompactResult

	// 1. Fetch all compactable entries.
	// Include 'system' tier so legacy migrated rows (which used to be hardcoded
	// to tier='system') are also compacted into daily digests and produce
	// breakdown chips via TagCounts.
	rows, err := s.db.Query(`
		SELECT id, timestamp, tier, type, level, source, operation_id,
		       book_id, summary, details, tags
		FROM   activity_log
		WHERE  tier IN ('change', 'debug', 'audit', 'system')
		  AND  compacted = 0
		  AND  timestamp < ?
		ORDER BY timestamp ASC`,
		olderThan.UTC(),
	)
	if err != nil {
		return result, fmt.Errorf("activity_store: compact query: %w", err)
	}

	// Scan into memory grouped by date.
	type dayGroup struct {
		entries []ActivityEntry
		ids     []int64
	}
	days := make(map[string]*dayGroup) // key = "2006-01-02"
	var dayOrder []string

	for rows.Next() {
		var (
			e          ActivityEntry
			ts         time.Time
			opID       sql.NullString
			bookID     sql.NullString
			detailsRaw sql.NullString
			tagsRaw    sql.NullString
		)
		if err := rows.Scan(
			&e.ID, &ts, &e.Tier, &e.Type, &e.Level, &e.Source,
			&opID, &bookID, &e.Summary, &detailsRaw, &tagsRaw,
		); err != nil {
			rows.Close()
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
		if tagsRaw.Valid && tagsRaw.String != "" {
			e.Tags = strings.Split(tagsRaw.String, ",")
		}

		dateKey := e.Timestamp.Format("2006-01-02")
		dg, ok := days[dateKey]
		if !ok {
			dg = &dayGroup{}
			days[dateKey] = dg
			dayOrder = append(dayOrder, dateKey)
		}
		dg.entries = append(dg.entries, e)
		dg.ids = append(dg.ids, e.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("activity_store: compact rows: %w", err)
	}

	// 2. Process each day.
	for _, dateKey := range dayOrder {
		dg := days[dateKey]

		// Build counts map (keyed by Type).
		counts := make(map[string]int)
		for _, e := range dg.entries {
			counts[e.Type]++
		}

		// Build tag-counts map: namespace → value → count.
		// Only "action" and "source" namespaces are aggregated for now.
		// These are used by the frontend as a fallback when Counts is sparse
		// (e.g. all entries have type="system_log" from old legacy imports).
		tagCounts := make(map[string]map[string]int)
		for _, e := range dg.entries {
			for _, tag := range e.Tags {
				colonIdx := strings.Index(tag, ":")
				if colonIdx < 1 {
					continue
				}
				ns := tag[:colonIdx]
				val := tag[colonIdx+1:]
				if ns != "action" && ns != "source" {
					continue
				}
				if tagCounts[ns] == nil {
					tagCounts[ns] = make(map[string]int)
				}
				tagCounts[ns][val]++
			}
		}

		// Build items — audit first (forensic record must survive
		// truncation), then error/warn, then the rest.
		var auditItems, errItems, normalItems []DigestItem
		for _, e := range dg.entries {
			item := DigestItem{
				Type:        e.Type,
				Tier:        e.Tier,
				Book:        extractBookName(e),
				BookID:      e.BookID,
				OperationID: e.OperationID,
				Summary:     extractItemSummary(e),
			}
			switch {
			case e.Tier == "audit":
				auditItems = append(auditItems, item)
			case e.Level == "error" || e.Level == "warn":
				item.Details = extractErrorDetails(e)
				errItems = append(errItems, item)
			default:
				normalItems = append(normalItems, item)
			}
		}
		items := append(auditItems, errItems...)
		items = append(items, normalItems...)

		truncated := false
		truncatedCount := 0
		if len(items) > maxDigestItems {
			truncatedCount = len(items) - maxDigestItems
			items = items[:maxDigestItems]
			truncated = true
		}

		var tagCountsOrNil map[string]map[string]int
		if len(tagCounts) > 0 {
			tagCountsOrNil = tagCounts
		}
		dd := DigestDetails{
			Date:           dateKey,
			OriginalCount:  len(dg.entries),
			Counts:         counts,
			TagCounts:      tagCountsOrNil,
			Items:          items,
			Truncated:      truncated,
			TruncatedCount: truncatedCount,
		}

		detailsBytes, err := json.Marshal(dd)
		if err != nil {
			return result, fmt.Errorf("activity_store: compact marshal digest: %w", err)
		}

		// End of day timestamp.
		// Use start-of-day (00:00:00) so digests sort AFTER all live entries
		// in a newest-first list — all digests cluster together at the bottom.
		startOfDay, err := time.Parse("2006-01-02", dateKey)
		if err != nil {
			return result, fmt.Errorf("activity_store: compact parse date %q: %w", dateKey, err)
		}

		// Transaction: merge-or-insert digest + delete originals.
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return result, fmt.Errorf("activity_store: compact begin tx: %w", err)
		}

		// Merge semantics: if a digest already exists for this date
		// (because a previous compact ran and then more entries were
		// written for the same day — background imports, late tasks,
		// etc.), fold its counts/items into the new digest and DELETE
		// the old row inside this transaction. Then INSERT a single
		// combined digest below.
		//
		// Previous behavior was to `continue` on existing digest, which
		// left every late-arriving entry permanently uncompacted. A
		// library that ran compact once a day would accumulate tens of
		// thousands of stragglers forever.
		var (
			existingID          int64
			existingDetailsJSON sql.NullString
		)
		err = tx.QueryRowContext(ctx, `
			SELECT id, details FROM activity_log
			WHERE tier = 'digest' AND type = 'daily_digest'
			  AND date(timestamp) = ?
			ORDER BY id ASC
			LIMIT 1`, dateKey).Scan(&existingID, &existingDetailsJSON)
		if err != nil && err != sql.ErrNoRows {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.Info("CompactByDay rollback on check digest", "rbErr", rbErr)
			}
			return result, fmt.Errorf("activity_store: compact check digest: %w", err)
		}
		if existingID > 0 {
			// Fold existing digest's counts + items into the new digest
			// we just built, then delete the old row.
			if existingDetailsJSON.Valid && existingDetailsJSON.String != "" {
				var existing DigestDetails
				if jsonErr := json.Unmarshal([]byte(existingDetailsJSON.String), &existing); jsonErr == nil {
					// Merge counts.
					for k, v := range existing.Counts {
						dd.Counts[k] += v
					}
					// Merge tag counts.
					for ns, vals := range existing.TagCounts {
						if dd.TagCounts == nil {
							dd.TagCounts = make(map[string]map[string]int)
						}
						if dd.TagCounts[ns] == nil {
							dd.TagCounts[ns] = make(map[string]int)
						}
						for val, cnt := range vals {
							dd.TagCounts[ns][val] += cnt
						}
					}
					dd.OriginalCount += existing.OriginalCount
					// Merge items — old items first so new errors/warnings
					// still sort to the front. Cap at maxDigestItems.
					// Existing digests may already have been truncated; we
					// preserve that signal in the combined row.
					combined := append(existing.Items, dd.Items...)
					if existing.Truncated {
						// Keep the truncation flag since older data was lost.
						dd.Truncated = true
						dd.TruncatedCount += existing.TruncatedCount
					}
					if len(combined) > maxDigestItems {
						dd.TruncatedCount += len(combined) - maxDigestItems
						combined = combined[:maxDigestItems]
						dd.Truncated = true
					}
					dd.Items = combined
				}
			}
			// Re-marshal the merged digest for insertion below.
			merged, mErr := json.Marshal(dd)
			if mErr != nil {
				if rbErr := tx.Rollback(); rbErr != nil {
					slog.Info("CompactByDay rollback on remarshal", "rbErr", rbErr)
				}
				return result, fmt.Errorf("activity_store: compact remarshal merged digest: %w", mErr)
			}
			detailsBytes = merged

			// Delete the old digest row — we'll insert the combined one below.
			if _, delErr := tx.ExecContext(ctx, `DELETE FROM activity_log WHERE id = ?`, existingID); delErr != nil {
				if rbErr := tx.Rollback(); rbErr != nil {
					slog.Info("CompactByDay rollback on delete old digest", "rbErr", rbErr)
				}
				return result, fmt.Errorf("activity_store: compact delete old digest: %w", delErr)
			}
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO activity_log
				(timestamp, tier, type, level, source, summary, details, compacted)
			VALUES (?, 'digest', 'daily_digest', 'info', 'compaction', ?, ?, 1)`,
			startOfDay, fmt.Sprintf("Daily digest for %s (%d entries)", dateKey, dd.OriginalCount),
			string(detailsBytes),
		)
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.Info("CompactByDay rollback on insert digest", "rbErr", rbErr)
			}
			return result, fmt.Errorf("activity_store: compact insert digest: %w", err)
		}

		// Delete originals by ID. Use batched placeholders.
		var deletedCount int64
		for i := 0; i < len(dg.ids); i += 999 {
			end := i + 999
			if end > len(dg.ids) {
				end = len(dg.ids)
			}
			batch := dg.ids[i:end]
			placeholders := strings.Repeat("?,", len(batch))
			placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
			args := make([]any, len(batch))
			for j, id := range batch {
				args[j] = id
			}
			delRes, delErr := tx.ExecContext(ctx, "DELETE FROM activity_log WHERE id IN ("+placeholders+")", args...)
			if delErr != nil {
				if rbErr := tx.Rollback(); rbErr != nil {
					slog.Info("CompactByDay rollback on delete originals", "rbErr", rbErr)
				}
				return result, fmt.Errorf("activity_store: compact delete: %w", delErr)
			}
			n, _ := delRes.RowsAffected()
			deletedCount += n
		}

		if err := tx.Commit(); err != nil {
			return result, fmt.Errorf("activity_store: compact commit: %w", err)
		}

		result.DaysCompacted++
		result.EntriesDeleted += int(deletedCount)
	}

	return result, nil
}

// extractBookName returns the book title from entry details, or "".
func extractBookName(e ActivityEntry) string {
	if v, ok := e.Details["book_title"]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	if v, ok := e.Details["title"]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// extractItemSummary builds a short summary string from the entry based on its type.
func extractItemSummary(e ActivityEntry) string {
	switch e.Type {
	case "metadata_applied":
		fields, _ := e.Details["fields"].(string)
		source, _ := e.Details["source"].(string)
		if fields != "" && source != "" {
			return fields + " from " + source
		}
		if fields != "" {
			return fields
		}
	case "tag_written":
		tagCount := detailNumber(e.Details, "tag_count")
		fileCount := detailNumber(e.Details, "file_count")
		return fmt.Sprintf("wrote %d tags to %d files", tagCount, fileCount)
	case "organize_completed":
		if p, ok := e.Details["new_path"].(string); ok {
			return "moved to " + p
		}
	case "config_changed":
		if k, ok := e.Details["key"].(string); ok {
			return k + " changed"
		}
	}
	// Default: truncate summary to 120 chars.
	if len(e.Summary) > 120 {
		return e.Summary[:120]
	}
	return e.Summary
}

// extractErrorDetails joins error-related fields from entry details.
func extractErrorDetails(e ActivityEntry) string {
	var parts []string
	for _, key := range []string{"error", "path", "file_path"} {
		if v, ok := e.Details[key].(string); ok && v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, ", ")
}

// detailNumber extracts a numeric value from details as int.
func detailNumber(details map[string]any, key string) int {
	v, ok := details[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}
