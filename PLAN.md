# Enhance Legacy Activity Log Migration with Smart Tag Derivation

## Goal
Improve the existing legacy SQLite `system_activity_log` migration to apply intelligent, multi-tag enrichment based on log content rather than generic ["legacy"] tags. Distinguish between compacted and non-compacted logs, infer action/outcome/source tags from message patterns, and apply the same tag-derivation logic used in the unified activity system.

## Current State
- Migration already exists in `internal/database/activity_store.go:MigrateSystemActivityLogs()`
- Currently applies generic tags: `["legacy", "system_activity_log"]` to all entries
- No distinction between log types or intelligent tag inference

## Affected files

- `internal/database/activity_store.go` — refactor `MigrateSystemActivityLogs()` to call new `enrichLegacyLogTags()` helper
- `internal/activity/legacy_tag_enrichment.go` — new file with `enrichLegacyLogTags(message, source, level) []string` function
- `internal/activity/legacy_tag_enrichment_test.go` — test cases covering message patterns → tag mappings
- `internal/database/activity_store_test.go` — update existing migration test to verify enriched tags

## Steps

### Step 1: Implement tag derivation logic
In new file `internal/activity/legacy_tag_enrichment.go`, create `enrichLegacyLogTags(message, source, level string) []string` that:

**Detect log type (compacted vs regular):**
- If message contains "Compacted" or "Daily compaction" → add `tier:maintenance`, `action:compact`
- If message contains "Purged" or "deleted" → add `action:purge`
- If message contains "scanned" or "Scan" → add `action:scan`
- If message contains "metadata" or "Metadata" → add `action:metadata-apply`
- If message contains "ISBN" → add `action:metadata-apply`

**Derive outcome tags:**
- `level: "warning"` → `outcome:warn`
- `level: "error"` → `outcome:error`
- `level: "info"` → `outcome:ok`

**Derive source tags:**
- If source non-empty → `source:<source>`

**Always include:**
- `legacy` — identifies as migrated legacy entry

**Return:** De-duplicated list of derived tags

### Step 2: Update MigrateSystemActivityLogs()
Modify the migration loop to call `enrichLegacyLogTags(old.Message, old.Source, old.Level)` instead of hardcoding `["legacy", "system_activity_log"]`.

Keep the migration-complete marker as-is (uses `type: "migration_complete"`, `tags: ["migration"]`).

### Step 3: Add comprehensive tests
In `legacy_tag_enrichment_test.go`:
- Test message patterns → expected tag mappings
- Test level → outcome:* mapping
- Test source presence → source:* tag
- Test de-duplication (no duplicate tags returned)
- Test edge cases (empty message, unknown level, etc.)

In `activity_store_test.go`:
- Update existing `TestMigrateSystemActivityLogs` to verify a few sample entries have sensible tags (not just `["legacy", "system_activity_log"]`)

### Step 4: Verify and measure
- Run `go test ./internal/activity/... ./internal/database/... -v`
- Spot-check results: does a "Compacted" message get `action:compact, tier:maintenance, legacy`? Does a "warning" level get `outcome:warn`?
- No behavioral change for production: migration is still idempotent, doesn't re-run

## Test strategy

- **Unit tests:** `go test ./internal/activity/... -run TestEnrichLegacyLogTags -v`
  - Verify each message pattern → tag derivation works
  - Verify level → outcome mapping
  - Verify de-duplication
  - Verify edge cases (empty/nil inputs)

- **Integration smoke test:** `go test ./internal/database/... -run TestMigrateSystemActivityLogs -v`
  - Create mock legacy logs with diverse messages
  - Run migration
  - Verify entries have enriched tags (not just generic ["legacy"])
  - Verify idempotency (second run doesn't create duplicates)

- **Success criteria:**
  - All legacy entries have meaningful derived tags reflecting log content
  - No generic catch-all tags — each tag should explain *what happened*
  - Compacted/maintenance logs have `tier:maintenance` or `action:compact`
  - Warning/error logs have outcome tags
  - All tests pass
  - No performance regression (tag enrichment is O(string search) — negligible)

## Rollback

- `git revert` the commits
- Re-run migration: entries already migrated keep their old tags (can clean up via future admin API)
- No schema changes — rollback is safe

## Message Pattern Reference (Examples)

| Message Pattern | Suggested Tags |
|---|---|
| "Compacted X entries" | `action:compact`, `tier:maintenance`, `legacy` |
| "Daily compaction" | `action:compact`, `tier:maintenance`, `legacy` |
| "Purged X deleted books" | `action:purge`, `legacy` |
| "Scanned Y files" | `action:scan`, `legacy` |
| "Applied metadata" | `action:metadata-apply`, `legacy` |
| "ISBN enriched" | `action:metadata-apply`, `legacy` |

(Start with these; expand based on actual legacy log content discovery)
