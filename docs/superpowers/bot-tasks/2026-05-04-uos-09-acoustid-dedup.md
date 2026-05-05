<!-- file: docs/superpowers/bot-tasks/2026-05-04-uos-09-acoustid-dedup.md -->
<!-- version: 1.0.0 -->
<!-- guid: 79d0e1f2-a3b4-5c6d-7e8f-9a0b1c2d3e4f -->
<!-- last-edited: 2026-05-04 -->

# UOS-09 — Migrate AcoustID + Dedup ops

**Companion human spec:** §11 phase B, §15.

## Branch

```
feat/uos-09-acoustid-dedup
```

## Goal

Migrate the remaining dedup ops (acoustid scan, llm review,
fingerprint rescan, book-signature scan, dedup full scan) and the
acoustid backfill. The `embed-scan` was migrated in UOS-07; do not
re-touch it.

## Files to add

1. `internal/plugins/dedup/full_scan.go` — `fullScanDef()`:
   - `ID`: `"dedup.full-scan"`
   - `Run`: extracted from `triggerDedupScan` opFunc
   - `Cancellable`: `true`
   - `Isolate`: `false`
   - `ResumePolicy`: `ResumeRequeue` (FullScan is large; checkpointed
     resume is future work)
   - `ConcurrencyKey`: `"dedup.full-scan"`
   - `Capabilities`: `[CapLibraryRead, CapLibraryWrite,
     CapNetworkOpenAI]` (calls EmbedBatch internally)

2. `internal/plugins/dedup/llm_review.go` — `llmReviewDef()`:
   - `ID`: `"dedup.llm-review"`
   - `Run`: extracted from `triggerDedupLLM`
   - `ResumePolicy`: `ResumeDrop` (LLM review is expensive; do not
     auto-resume)
   - `Capabilities`: `[CapLibraryRead, CapLibraryWrite,
     CapNetworkOpenAI]`

3. `internal/plugins/acoustid/plugin.go` — new plugin shell.

4. `internal/plugins/acoustid/scan.go` — `scanDef()`:
   - `ID`: `"acoustid.scan"`
   - `Run`: extracted from `triggerDedupAcoustID` opFunc + Engine
     `AcoustIDScan` method
   - `Isolate`: **true** (subprocess; ffmpeg/chromaprint stderr is
     auto-tagged with op_id per spec §9)
   - `ResumePolicy`: `ResumeDrop` (271K-file hash; restart from zero
     is correct; this is the bug-prevention motivation)
   - `Capabilities`: `[CapLibraryRead, CapFilesRead,
     CapFilesExecute, CapSubprocessSpawn]`
   - `ConcurrencyKey`: `"acoustid.scan"`
   - `Timeout`: `6 * time.Hour`

5. `internal/plugins/acoustid/backfill.go` — `backfillDef()`:
   - `ID`: `"acoustid.backfill"`
   - `Run`: extracted from existing `acoustid_backfill.go`
   - `Schedule`: nightly cron (e.g. `"0 3 * * *"` — 3am)
   - `ResumePolicy`: `ResumeRestart` with checkpoint per file
     processed
   - `Triggers`: `[{EventName: "book.imported"}]` (auto-extract for
     newly imported books) — note: this fires per-book, not per-bulk
   - `Isolate`: `true`
   - `Capabilities`: same as `acoustid.scan`

6. `internal/plugins/acoustid/fingerprint_rescan.go` —
   `fingerprintRescanDef()`:
   - `ID`: `"acoustid.fingerprint-rescan"`
   - `Run`: extracted from existing `triggerFingerprintRescan`
   - `ResumePolicy`: `ResumeDrop`
   - `Isolate`: `true`

7. `internal/plugins/dedup/book_signature_scan.go` —
   `bookSignatureScanDef()` from `triggerBookSignatureScan`.
   - `ResumePolicy`: `ResumeRequeue`

8. Tests for each new def: register + enqueue + run-to-completion
   happy path.

## Files to edit

1. `internal/server/dedup_handlers.go`:
   - `triggerDedupScan`, `triggerDedupLLM`, `triggerDedupAcoustID`,
     `triggerBookSignatureScan` ALL become thin redirects calling
     `EnqueueOp` with the corresponding `def_id`.
2. `internal/server/fingerprint_rescan.go`:
   - `triggerFingerprintRescan` becomes a thin redirect.
3. `internal/plugins/dedup/plugin.go`:
   - `Register` now registers all dedup ops, not just embed-scan.
4. `internal/plugins/plugins.go`:
   - Add `_ "github.com/jdfalk/audiobook-organizer/internal/plugins/acoustid"`.
5. `internal/server/server.go`:
   - Construct + register the acoustid plugin alongside dedup.

## Hard rules

- `acoustid.scan` MUST declare `ResumePolicy: ResumeDrop`. This is
  the structural fix for the queue-jam incident.
- `acoustid.backfill` runs `Isolate: true` so ffmpeg stderr is
  auto-tagged — this is what fixes the "ffmpeg warnings have no
  op_id" complaint.
- Run functions MUST be functionally identical to existing handlers.
  If the existing code has a bug, fix it in a SEPARATE PR — not here.
- Existing direct-goroutine acoustid backfill (if any) MUST be
  replaced by event-trigger via `book.imported`. Do not keep both.

## Acceptance criteria

- [ ] `go test ./internal/plugins/...` passes.
- [ ] `make ci` passes.
- [ ] Manual on staging: trigger each dedup op via the existing UI,
      observe correct behavior + op_id-tagged ffmpeg warnings in the
      Activity log view.
- [ ] Manual: import a book (or trigger a fake `book.imported` event),
      observe `acoustid.backfill` op spawned with the import op as
      `parent_id`.
- [ ] Manual restart-test: trigger acoustid.scan, restart server
      mid-run, confirm op ends as `interrupted_dropped` and is NOT
      auto-resumed.

## PR title

```
feat(uos): migrate AcoustID + remaining dedup ops to UOS
```
