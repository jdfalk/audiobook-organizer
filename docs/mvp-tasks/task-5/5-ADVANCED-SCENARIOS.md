<!-- file: docs/TASK-5-ADVANCED-SCENARIOS.md -->
<!-- version: 1.0.0 -->
<!-- guid: 5e9c1f2d-8a4b-4c5d-9f7e-1a8b2c3d4e5f -->

# Task 5: Advanced Scenarios & Code Deep Dive (Hash Tracking)

Use these scenarios when core testing passes but edge conditions need
validation.

## 🔄 Hash Drift Detection

**Scenario:** Library copy modified (e.g., metadata edited), hash changes.

```bash
# After organize: library_hash = ABC123
# User edits tags: library_hash should become DEF456
# Original import still has original_hash = ABC123
```

**Handling:**

- Re-scan detects hash change; update `library_hash`.
- Keep `original_hash` unchanged to track source.
- If library copy deleted, can re-copy from import (matching `original_hash`).

## 📦 Multi-Version Edge Case

**Scenario:** Same book, different editions (unabridged, abridged), same import
path.

```bash
# book-unabridged.m4b -> hash X
# book-abridged.m4b -> hash Y
# Both should be tracked separately
```

- Ensure version grouping (from Task 4) doesn't interfere with hash tracking.
- Each version has its own `original_hash` and `library_hash`.

## 🗑️ Soft Delete & Purge Logic

**Scenario:** Soft-deleted book lingers, needs purging after retention period.

```bash
# Background job query
SELECT id, title, soft_deleted_at FROM books WHERE state = 'soft_deleted' AND soft_deleted_at < NOW() - INTERVAL '30 days';
```

- Job runs daily/weekly.
- Permanently deletes books and their hashes from `do_not_import` (optional:
  keep hashes if user wants permanent block).
- Configurable retention in Settings: `soft_delete_retention_days`.

## 🚫 Orphaned Import Copies

**Scenario:** Import copy deleted manually (outside app), library copy remains.

```bash
# original_hash points to missing file
# library_hash points to existing file
```

**Handling:**

- Re-scan detects missing import; mark `original_hash` as null or keep for
  reference.
- Library copy still functional; no reimport needed.

## 🔁 Quantity / Reference Counting

**Scenario:** Multiple users/playlists reference same book.

```bash
# quantity = 0: unwanted
# quantity = 1: one reference (library)
# quantity = 2+: multiple references (don't delete until all removed)
```

- Increment `quantity` when added to playlist or marked as wanted.
- Decrement on removal; soft-delete only when quantity reaches 0.
- Out of scope for MVP but field reserved.

## 🧰 Backend Code Checklist

- Migration adds fields: `original_hash`, `library_hash`, `state`,
  `soft_deleted_at`, `quantity`.
- Migration creates `do_not_import` table with hash index.
- Scanner checks `do_not_import` before importing; logs skip reason.
- Delete handler updates `state`, `soft_deleted_at`, and inserts into
  `do_not_import` if flag set.
- Settings API provides CRUD for `do_not_import` entries.

## 🪛 Frontend Checklist

- Delete dialog has checkbox: "Prevent this file from being imported again".
- Settings page has "Blocked Hashes" tab listing hashes with unblock button.
- Book detail shows state badge: imported/organized/soft_deleted.
- Library view can filter by state (exclude soft_deleted by default).

## 🔬 Performance Considerations

- Hash computation on large files: show progress during import/organize.
- `do_not_import` lookups: index on hash for O(1) checks during scan.
- Purge job: batch deletes, avoid locking entire table.

When an edge condition is identified, document in `TASK-5-TROUBLESHOOTING.md`.
