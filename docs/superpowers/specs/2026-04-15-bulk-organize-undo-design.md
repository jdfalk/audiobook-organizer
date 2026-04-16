<!-- file: docs/superpowers/specs/2026-04-15-bulk-organize-undo-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 4b9e2c8f-1a7d-4f50-a3c8-2e8d0f1b6a47 -->

# Bulk Organize Undo — Design

**Status:** Design complete (Apr 15, 2026). Ready for implementation plan.
**Scope item:** TODO.md §3.2.
**Depends on:** existing `operation_changes` table + per-book revert buttons (already in place).

## Goal

Let a user undo a completed organize operation — either the whole op in one click or a per-book subset — by reversing every file move and DB mutation it recorded. The reversal preserves any later edits (metadata apply, tag writes, ISBN enrichment) by scoping the restore to only the fields organize itself touched.

## Locked decisions

### 1. Both op-level and per-book undo

- **Op-level** (primary new UX): "Undo this organize" reverses every `operation_changes` row for the operation.
- **Per-book** (already in place per memory): "Revert this change" button on BookDetail ChangeLog filters to one book's changes.
- Same reversal engine; op-level is `WHERE operation_id = ?`, per-book is `WHERE book_id = ? AND operation_id = ?`.

### 2. `operation_changes` schema (verify against existing shape in implementation)

```
operation_id   FK → operations.id   (NOT NULL)
book_id        FK → books.id        (NULLABLE for directory-only changes)
change_type    TEXT                  -- file_move | db_update | dir_create | dir_delete
before_json    TEXT                  -- JSON snapshot of reversible fields
after_json     TEXT                  -- JSON snapshot of new values
created_at     TIMESTAMP
reverted_at    TIMESTAMP              -- NULL until undone
```

Payload shapes per `change_type`:

| Type | `before_json` | `after_json` |
|---|---|---|
| `file_move` | `{file_path: "/old/..."}` | `{file_path: "/new/..."}` |
| `db_update` | `{file_path, library_state, last_organized_at, ...}` (only fields organize touched) | matching `after` values |
| `dir_create` | `{path: "/dir/..."}` | `{}` |
| `dir_delete` | `{}` | `{path: "/dir/..."}` |

### 3. Undo as a tracked operation

`type: "undo_operation"`, params `{target_operation_id, scope: "all" | "book:{bookID}"}`. Behaves like any other tracked op — progress, cancel, resume on restart via PR #270's framework.

Execution:

1. Load non-reverted `operation_changes` rows for the target op (filtered by scope), order by `created_at DESC` so reversal happens in reverse
2. For each row: reverse the change per the table in §5
3. On success, set `reverted_at = now()`
4. Aggregate result: `{reverted, skipped_conflict, failed}`
5. Activity log entry summarizing

### 4. Field-scoped restore — the key property

For `db_update` rows: `before_json` lists only the fields the operation touched (`file_path`, `library_state`, `last_organized_at`, and matching `book_files.*.file_path`). Undo sets only those fields back to their `before` values. Any field not in `before_json` is left alone.

This is what preserves later edits: if the user applied metadata after organize, the title/author/cover on the book stay as-is; only the organize-specific fields roll back.

### 5. Per-change reversal semantics

| Change type | Reversal step | Conflict case |
|---|---|---|
| `file_move` | `os.Rename(after_path, before_path)`; `MkdirAll` parent if absent | `after_path` missing → skip "file no longer at expected location"; `before_path` occupied → skip "target now occupied" |
| `db_update` (books) | Field-scoped restore from `before_json` | None; last-write-wins is acceptable — user launched the undo |
| `db_update` (book_files) | Same field-scoped restore per row | None |
| `dir_create` | `os.Remove` only if empty | Dir not empty → skip "directory now populated" |
| `dir_delete` | `MkdirAll` | None |

### 6. Conflict pre-flight

Before executing, scan `operation_changes` and gather:

- Files modified since the op (`mtime > created_at`) → "content may have changed, undo will move a possibly-different file"
- Books soft-deleted since → will be skipped
- Books re-organized in a later operation → conflict, will be skipped
- Torrent removal in deluge → still notify deluge on path restore

Surface a pre-flight dialog with counts per category. User can **Proceed** (run over conflicts per §5 rules), **Cancel**, or **Select subset** (proceed only on non-conflict books).

### 7. Torrent `move_storage` on undo

For any affected book with `book_versions.torrent_hash != NULL`, call deluge `move_storage` after the filesystem rename reversal. Same retry-3-with-backoff as the forward path in the 3.1 centralization design.

### 8. Redo — not in MVP

If user wants the organize back, re-run organize. Schema doesn't preclude future redo (undo itself would produce `operation_changes` rows if we ever want it), but it's a rabbit hole of nested state and out of scope.

### 9. Which op types are undoable

**MVP:** `organize` (including batch and per-book organize).

**Future:** `bulk_write_back` (tag writes; `.bak-*` backups exist but semantics need separate thought), other targeted mutation ops.

**Not planned:** `scan` (no reversible forward state), `metadata_apply` (already has per-book revert via `recordChangeHistory`), maintenance/cleanup ops.

Mechanism is permissive: any op that records `operation_changes` rows is undoable. No allowlist needed in the engine; the producing op opts in by writing the rows.

## UI

- **Operation detail page** (reachable from Operations drawer / Activity log): existing completed-op card gains an **Undo** button when:
  - Op type is in the allowlist above
  - At least one `operation_changes` row for the op has `reverted_at IS NULL`
- **Pre-flight dialog**: shows conflict counts, offers Proceed / Cancel / Select subset
- **Progress**: shared progress-bar component with other tracked ops
- **Post-undo**: result summary + link to the undo op's own detail page (which is itself viewable)
- **BookDetail ChangeLog**: existing per-book revert buttons stay; no change

## Storage cost

`operation_changes` rows per book per organize: ~1 file_move + ~1 db_update + occasional dir changes. For 10K books × 10 organize ops historical = ~200K rows. Small JSON payloads. PebbleDB handles trivially. No retention trimming in MVP; revisit when rows exceed a few million.

## Concurrency

Undo is a tracked op queued through `operations.GlobalQueue`, so two undos of the same op can't run concurrently — queue serializes. If the user re-runs organize on the same books *while* an undo is in flight, the organize op waits in the queue (or the user sees a "busy" state). No new locking primitive required.

## Risks

- **Stale `operation_changes` data.** If the existing machinery records changes in a shape that doesn't match §2, the implementation PR will need a light schema migration. Verify in the code before coding the engine — treat §2 as target-shape, not ground-truth of current schema.
- **Partial-success reporting.** Users need clear messaging when some books were reverted and some skipped. The summary format has to distinguish "reverted" from "skipped due to conflict" from "failed with error."
- **Torrent tracking drift.** If deluge was unaware of the original forward move (somehow), the reverse call fails. Treat deluge errors as non-fatal warnings — filesystem reversal is the main event.

## Non-goals

- Redo (future if/when we need it)
- Undo of arbitrary op types beyond the allowlist in §9
- Rolling back metadata edits made after the organize (explicitly the property of field-scoped restore: those edits are preserved)
- Distributed transactions / rollback across multiple machines

## Open implementation questions

- Exact shape of existing `operation_changes` rows — one of the first read tasks during implementation
- Whether the existing per-book revert UI already uses the same engine, or has a parallel code path to unify
- UX for "skipped: %d conflicts" — inline in the undo op detail page, or a separate downloadable report?
