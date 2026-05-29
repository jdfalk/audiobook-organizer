# PLAN: G2+G4 Split-Book Backfill Detector + CLI

## Goal

Detect existing split-book clusters (one Book per chapter) in the production
DB and provide a one-shot CLI that can dry-run and execute a portable merge.

## Files to add

- `internal/dedup/split_book_detector.go` (+ test) — pure detector.
- `internal/dedup/split_book_storage.go` — Pebble JSON store of candidates.
- `internal/dedup/split_book_merge.go` — portable cluster merge.
- `internal/plugins/dedup/split_book_scan.go` — OperationDef wrapper.
- `internal/plugins/dedup/plugin.go` — register the new op.
- `internal/server/split_book_handlers.go` — list/run/merge handlers.
- `internal/server/server_lifecycle.go` — route bindings.
- `tools/cmd/merge-split-books/main.go` — CLI.

## Heuristic

Group by `filepath.Dir(FilePath)` (parent) and `filepath.Dir(filepath.Dir(FilePath))` (grandparent).

Qualifies as candidate when:
- ≥3 books in the group
- all share same AuthorID (or all nil)
- all share same SeriesID or all nil
- extracted integers from filename or parent-dir name form near-sequential run (≥70% coverage of [min..max], no gap >2)

Grandparent emit only when every child parent-group has size 1.
A book is assigned to at most one candidate (parent wins).

## Storage

Pebble keyspace `split_book_candidate:<ulid>` → JSON. List by prefix scan.
Each entry: ID, ParentFolder, BookIDs, SuggestedTitle, SuggestedAuthor,
SequentialPattern, CreatedAt.

## Merge (portable; works on Pebble)

For `keepID + srcIDs`:
1. For each src: GetBookFiles, MoveBookFilesToBook → keepID.
2. Recompute keep duration as sum of all bookfile durations.
3. Update keep title to SuggestedTitle.
4. SoftDelete each src via merge.SoftDeleteBook.

Avoids SQLite-only MergeChapterBooks and avoids merge.MergeBooks (which deletes losers + their files).

## CLI

`tools/cmd/merge-split-books` — mirror reconcile-paths structure.
Flags: `--api`, `--key`, `--dry-run` (default true), `--execute`,
`--min-group-size 3`, `--limit 0`.

## Test strategy

Unit tests for detector (flat case, grandparent case, qualify/disqualify).
Manual smoke for CLI (documented in PR body).

## Rollback

Pure additive. Revert PR.
