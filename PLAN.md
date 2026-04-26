# iTunes Path Repair

Branch: `feat/itunes-path-repair` · Worktree: `.worktrees/itunes-path-repair`

## Goal

After organize/rename, iTunes still references stale paths for hundreds of
files that no longer exist on disk. Add an operation that **dumps the iTunes
XML, finds every track whose `Location` doesn't exist, re-discovers the
correct file via a three-tier strategy (PID → embedded tag → fuzzy match),
and enqueues path corrections through the existing `WriteBackBatcher`** so
they ship in normal batched .itl writes.

## Affected files

### New
- `internal/itunes/service/path_repair.go` — `PathRepairer` operation;
  mirrors `path_reconcile.go` (small store interface, `Start` HTTP handler
  that registers a queued operation, `Repair(ctx, opID, progress)` worker).
- `internal/itunes/service/path_repair_resolver.go` — pure-function
  resolver implementing tiers A/B/C (no store deps, easy to unit-test).
- `internal/itunes/service/path_repair_test.go` — operation-level integration
  test using fixture XML + tmpdir tree.
- `internal/itunes/service/path_repair_resolver_test.go` — per-tier unit
  tests.

### Modified
- `internal/itunes/service/service.go` — wire `PathRepairer` into the
  service struct + constructor (same pattern as `PathReconciler`).
- `internal/server/server.go` — register
  `POST /operations/itunes-path-repair` next to the existing reconcile
  route (around line 2365).
- `TODO.md` / `CHANGELOG.md` — entry for the new operation.

### Reused (read-only dependencies)
- `itunes.ParseLibrary` (parser.go) — XML dump
- `metafetch.ComputeITunesPath` — recompute iTunes-format paths
- `store.GetBookByExternalID("itunes", pid)` — tier A lookup
- `store.GetBookPathHistory` / `store.RecordPathChange` — audit trail
- `store.UpdateBook` — write corrected `FilePath` back to DB
- `Enqueuer.Enqueue(bookID)` — hand off to `WriteBackBatcher`; existing
  `SafeWriteITL` provides backups + atomic rename
- audio-tag reader (whichever package `metafetch.ExtractMetadata` uses)
  for tier B PID extraction

## Design

### Resolution tiers (applied in order)

1. **(A) PID → DB lookup.** Each XML track has a `Persistent ID`. Call
   `GetBookByExternalID("itunes", pid)`. If a book is found, take
   `Book.FilePath` (or the matching `BookFile.FilePath` for multi-segment
   books). If that path exists on disk, that's the correct location.
   Cheap, exact, handles the common case.
2. **(B) Embedded tag scan.** When (A) leaves residue, walk the audiobook
   root once, extract `AUDIOBOOK_ORGANIZER_PERSISTENT_ID` from each audio
   file's tags, build an in-memory `pid → on-disk-path` map. Re-resolve
   unmatched tracks against this map. Recovers when `external_id_map` is
   stale or never populated.
3. **(C) Fuzzy match.** For still-unresolved tracks, score candidates by
   filename + title similarity. **Never auto-apply.** Emit ranked
   candidates to a `needs_review` list in the operation result for human
   confirmation.

### Source-of-truth ordering

Filesystem > DB > iTunes XML. When (A) or (B) finds a real path that
differs from `Book.FilePath`, the repairer:

1. `RecordPathChange(book_id, old_path, new_path, "repair")`.
2. `UpdateBook` with the new `FilePath`.
3. Recompute `Book.ITunesPath` via `ComputeITunesPath(new_path)`.
4. `Enqueuer.Enqueue(bookID)` — the batcher writes through
   `UpdateITLLocations` on its normal schedule.

This deliberately reuses the same path the reconciler already drives, so we
inherit existing backup/atomic-rename safety.

### Dry-run by default

Operation defaults to dry-run: result lists `auto_resolved`, `needs_review`,
`unresolved` with counts and book IDs but writes nothing. `?apply=true`
flips it to write mode (matches the convention in `path_reconcile.go:69`).

### Concurrency / scale

Hundreds of books × thousands of files. Tiers A is cheap (DB lookup +
`os.Stat`); parallelize with a worker pool sized to `runtime.NumCPU()`.
Tier B runs only on the residue and is the expensive step (tag read per
file) — same worker-pool pattern. Tier C is small and sequential.

### Logging (per `feedback_logging.md`)

- Start: `iTunes path repair started: tracks=N`
- Per-tier rollup: `tier=A resolved=X unresolved=Y`
- Per resolution: `repair pid=… old=… new=… tier=A|B|C action=enqueue|skip`
- Skip: `tier=A skipped reason=path_exists`
- Complete: `iTunes path repair complete: missing=N auto=X review=Y unresolved=Z duration=…`

## Ordered steps (one commit each, conventional commits)

1. **Scaffold** — `path_repair.go` + service wiring + route + dry-run
   response shape with empty results. `make build-api` green.
2. **Tier A** — XML parse, stat each `Location`, resolve missing via
   `GetBookByExternalID`. Unit + small integration test with fixture XML.
3. **Tier B** — pure resolver function backed by a tag reader; lazy
   invocation; tests for PID match, multi-segment, no PID tag.
4. **Tier C** — fuzzy filename + title scoring; emit to `needs_review`,
   never auto-apply. Tests for ranking + threshold.
5. **Apply mode** — `?apply=true` triggers `RecordPathChange` +
   `UpdateBook` + `Enqueuer.Enqueue`. Test that the batcher receives the
   right book IDs (mock enqueuer).
6. **End-to-end** — fixture XML with one OK / one A-resolvable /
   one B-resolvable track + tmpdir tree; assert dry-run report and
   apply-mode side effects.
7. **Docs** — TODO.md, CHANGELOG.md.

## Test strategy

- `go test ./internal/itunes/service/... -run TestPathRepair -v`
- `go test ./internal/itunes/... -run "TestPath|TestParseLibrary"` —
  no regression in path mapping / parser
- `make test` — full Go suite stays green
- `make ci` before opening the PR
- **Manual dry-run smoke against prod XML** (read-only) before any
  `?apply=true` discussion: hit the endpoint with a dev DB pointed at the
  prod XML and confirm `missing` count is plausible and most resolutions
  land in tier A.

Success: `make ci` green; dry-run smoke returns sane counts with the
majority resolved by tier A.

## Rollback

- All iTunes writes go through existing `Enqueuer` → `WriteBackBatcher` →
  `SafeWriteITL`, which keeps timestamped .itl backups. Restore from
  backup if a bad apply lands.
- DB-side: every `Book.FilePath` change is recorded in `book_path_history`
  with `change_type=repair`; revert by replaying the most recent `repair`
  row per book in reverse.
- Code-side: revert the feature branch — operation is purely additive, no
  schema changes, no behavior change to existing endpoints.

## Open questions / clarifications wanted

- Is dry-run-default acceptable, or should the first run be apply-on?
  (Default: dry-run.)
- For tier C, what's the desired similarity threshold? Suggest start at
  `0.85` Jaro-Winkler on basename and require title containment.
- Should the operation persist its report (e.g. `docs/reports/itunes-repair-<ts>.json`)
  or just return it in the HTTP response? Suggest both — return inline
  for small results, write to disk if `>1000` items.
