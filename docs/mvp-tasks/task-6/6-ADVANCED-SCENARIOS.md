<!-- file: docs/TASK-6-ADVANCED-SCENARIOS.md -->
<!-- version: 1.0.0 -->
<!-- guid: 9f4e2d3c-8b5a-4c6d-9f7e-1a8b2c3d4e5f -->

# Task 6: Advanced Scenarios & Code Deep Dive (Book Detail & Delete)

Use these scenarios when core testing passes but edge conditions need
validation.

## 📚 Multi-File Books (Series/Multi-Part)

**Scenario:** Book consists of multiple audio files (CD1, CD2, etc.).

```bash
# Detail view should list all files
# Delete should handle all files atomically
# Hashes: compute per-file or combined?
```

**Handling:**

- List all related files in Files tab.
- Decision: block import of any file from the set (hash all files) or just the
  primary?
- Prefer blocking all file hashes to prevent partial reimports.

## 🔗 Version Group Delete

**Scenario:** User deletes one version from a group; other versions remain.

```bash
# Book A: unabridged (primary)
# Book B: abridged (linked)
# Delete Book A -> should not delete Book B
```

**Handling:**

- Delete only affects selected book.
- If primary version deleted, optionally promote another version to primary.
- Versions tab on remaining book should update.

## 🎨 Cover Art and Metadata Files

**Scenario:** Book has separate cover.jpg, metadata.nfo files.

```bash
# Files tab should show:
# - book.m4b (audio)
# - cover.jpg (image)
# - metadata.nfo (data)
```

**Handling:**

- List non-audio files in Files tab (grouped or separate section).
- Delete should remove or orphan these files (configurable: keep metadata).

## 🚫 Partial Delete (Keep Files, Remove from Library)

**Scenario:** User wants to remove book from library but keep files.

```bash
# Delete options:
# - Soft delete (keep in DB, mark deleted)
# - Hard delete + keep files (remove DB entry only)
# - Hard delete + remove files (remove everything)
```

**Handling:**

- Current implementation: soft delete by default.
- Future: add "Also delete files" checkbox (separate from reimport prevention).

## 🔁 Undo Delete

**Scenario:** User accidentally deleted book; wants to restore.

```bash
# Soft-deleted book still in DB
# Restore: set state back to organized, clear soft_deleted_at
```

**Handling:**

- Add "Restore" button in deleted books view (filter by state=soft_deleted).
- Restoring should remove hashes from blocklist (if added during delete).

## 🧰 Backend Code Checklist

- Detail API endpoint: `GET /api/v1/audiobooks/:id` with full fields.
- Versions API: `GET /api/v1/audiobooks/:id/versions`.
- Files API: `GET /api/v1/audiobooks/:id/files` (if separate endpoint needed).
- Delete handler accepts `prevent_reimport` flag and `reason` string.
- Delete logic: update state, timestamp, insert hashes into `do_not_import`.

## 🪛 Frontend Checklist

- BookDetail component with tabs: Info, Files, Versions.
- Info tab shows all metadata fields in read-only or editable form.
- Files tab lists files with icons, sizes, paths.
- Versions tab shows linked books with quality badges, set primary action.
- Delete dialog enhanced with checkbox and confirmation.
- Settings > Blocked Hashes tab shows list with unblock action.

## 🔬 Performance Considerations

- Detail page loads single book; no pagination issues.
- Versions query should be fast (indexed by version_group_id).
- Delete operation should be near-instant (DB updates only, no file I/O unless
  hard delete).

When an edge condition is identified, document in `TASK-6-TROUBLESHOOTING.md`.
