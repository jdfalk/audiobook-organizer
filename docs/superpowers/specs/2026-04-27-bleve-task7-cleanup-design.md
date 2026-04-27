<!-- file: docs/superpowers/specs/2026-04-27-bleve-task7-cleanup-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: f193b46a-e045-4f53-d061-345613e0adcd -->

# Bleve Task 7 — Remove FTS5 + LIKE Cleanup

**TODO ID:** DES-1-T7 (final task of [Bleve library search plan](../plans/2026-04-15-bleve-library-search.md))
**Audience:** human reviewer
**Companion bot recipe:** [`docs/superpowers/bot-tasks/2026-04-27-bleve-task7-cleanup.md`](../bot-tasks/2026-04-27-bleve-task7-cleanup.md)
**Size:** S — one PR, ~200 LOC removed.

## Status

Bleve search shipped tasks 1–6 (April 16, 2026). The new path is in production. The old FTS5 + LIKE + UNION path in `internal/database/sqlite_store.go:2810` and the prefix-scan path in `internal/database/pebble_store.go` are dead code today.

Cleanup PR was deferred from the original Bleve sprint. This task closes it.

## Goal

Three deletions:

1. The FTS5 + LIKE `SearchBooks` body in `sqlite_store.go` (the function stays — it now delegates to the bleve path or returns "use the new endpoint").
2. The prefix-scan `SearchBooks` body in `pebble_store.go`.
3. The `fuzzyRankBooks` Go-side re-ranker IF no other callers exist outside the old SearchBooks path.

Plus one schema operation:

4. New migration that drops the `books_fts` SQLite FTS5 virtual table on existing installs.

## Why this matters

- ~250 LOC of dead code.
- The FTS5 virtual table consumes disk space proportional to library size on existing SQLite installs (~10–20% overhead). Dropping it after the new path is verified is a free win.
- Future readers shouldn't trip over two different search implementations and have to figure out which is canonical.

## Design decisions

**Migration drops the virtual table; it does not back it up.** The data is fully recreated by Bleve. Loss of the virtual table is loss of an index, not loss of data.

**`SearchBooks` signature stays.** Callers that still hit the old path (if any — verify with grep) get a deprecation comment and either delegate to the bleve path or return an error pointing at the new endpoint.

**Single PR, not split.** Each delete is independent enough but small enough that one PR is easier to review than three.

## Risk

Low if Bleve is healthy in production. Verify by running search queries via the API and confirming Bleve serves them. The migration is reversible only by recreating the FTS5 table from `books`; not impossible, just an unnecessary operator burden if we have to roll back. Recommend keeping migration in a separate `versions: down` block for explicit revert.

## Out of scope

- Performance comparison. That was task 5's job; this PR doesn't benchmark.
- Frontend changes. Task 6 already updated the search bar; nothing else to touch.

## Bot recipe

[`docs/superpowers/bot-tasks/2026-04-27-bleve-task7-cleanup.md`](../bot-tasks/2026-04-27-bleve-task7-cleanup.md).
