# Migrate Legacy System Activity Logs

## Goal
Backfill old SQLite `system_activity_log` table (pre-May 12) into the current PebbleDB-backed `ActivityStore`. This recovers ~4 months of missing activity history and provides a single source of truth for all activity logging. The migration runs once at server startup, reads old rows, transforms them to new schema, and marks completion to avoid re-running.

## Affected files
- `internal/database/activity_store.go` — Add `MigrateSystemActivityLogs()` function that opens the old SQLite main DB, reads system_activity_log, and inserts ActivityEntry records. Include idempotency flag to prevent re-running.
- `internal/server/server_lifecycle.go` — Call migration during server init (before ActivityWriter starts), after ActivityStore is opened.
- `internal/database/store.go` — Define `SystemActivityLogMigrationDone` flag (e.g., in config or as a marker row in activity_log).
- `internal/database/sqlite_store_activity.go` — Ensure old table is readable; confirm schema (id, user_id, source, level, message, created_at).
- Tests: `internal/database/activity_store_test.go` — Add test that verifies old rows are migrated with correct field mapping (message→summary, created_at→timestamp, tier="system", type="system_log", tags=["legacy"]).

## Steps

1. **Confirm old schema and write reader function**
   - Verify `system_activity_log` table exists and schema matches (id, user_id, source, level, message, created_at).
   - Write `GetSystemActivityLogRows()` helper in `sqlite_store_activity.go` to SELECT all rows, ordered by created_at DESC (newest first, so IDs auto-increment sensibly in new store).

2. **Implement MigrateSystemActivityLogs in ActivityStore**
   - Open the main SQLite DB (same path as ActivityStore parent config uses).
   - Read all system_activity_log rows via GetSystemActivityLogRows().
   - Map fields:
     - `timestamp` ← `created_at`
     - `tier` ← `"system"` (constant)
     - `type` ← `"system_log"` (constant)
     - `level` ← `level` (pass through)
     - `source` ← `source` (pass through)
     - `summary` ← `message` (pass through)
     - `details` ← `nil` (old logs don't have structured details)
     - `tags` ← `[]string{"legacy", "system_activity_log"}` (mark origin)
   - Write each as ActivityEntry via `Record()` in a transaction.
   - On success, write a marker entry: `ActivityEntry{Tier: "system", Type: "migration_complete", Summary: "Migrated N system_activity_log rows", Tags: []string{"migration"}}`.
   - Check for marker at start of next migration call and skip if present (idempotent).

3. **Integrate into server lifecycle**
   - In `server_lifecycle.go` `initActivityStore()` (or similar init phase):
     - After `NewActivityStore()`, call `activityStore.MigrateSystemActivityLogs()`.
     - Log start/completion with operation counts.
     - If migration fails, log error but don't block server startup (graceful degradation).

4. **Write test**
   - Create test that:
     - Sets up old SQLite DB with sample system_activity_log rows (3-5 rows with various levels/sources).
     - Opens ActivityStore and calls MigrateSystemActivityLogs().
     - Queries activity_log for tags containing "legacy".
     - Verifies count matches, fields mapped correctly, marker entry exists.
     - Calls migration again and verifies idempotency (no duplicates).

5. **Update TODO.md**
   - Mark `BUG-ACTIVITY-MISSING-OLD-LOGS` as complete.
   - Commit message: `fix(activity): migrate legacy system_activity_log to unified ActivityStore`

## Test strategy
- **Unit test:** `go test ./internal/database -run TestMigrateSystemActivityLogs`
  - Success: old rows present in activity_log with correct tier, type, tags; marker row exists; second migration call is no-op.
- **Integration test (manual):** Deploy to staging or run locally with production DB backup.
  - Query `SELECT COUNT(*) FROM activity_log WHERE tags LIKE '%legacy%'` → should see pre-May-12 entries.
  - Verify `created_at` gaps are now filled.
- **Full suite:** `make ci` (includes all backend tests).

## Rollback
- If migration corrupts data: delete the marker entry and activity_log rows with `tags LIKE '%legacy%'`, then re-run.
- If server fails during migration: old table remains untouched; restart server and retry (migration is idempotent).
- Git: `git reset --hard HEAD~1` (or revert the commit if already pushed).
