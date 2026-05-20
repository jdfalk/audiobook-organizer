# Fix iTunes Memory Leak – Stream Parse XML

## Goal
Refactor `BackfillITunesTrackPIDs` to use a streaming XML parser instead of loading the entire 88K-track iTunes Library.xml into memory at once. Current behavior: 53GB peak memory during backfill (unsustainable on prod). Target: <500MB memory usage by processing tracks incrementally.

## Affected files
- `internal/itunes/parser.go` — Add `StreamingParseLibrary()` function that yields tracks via a callback instead of returning a full Library struct
- `internal/itunes/backfill.go` — Refactor `BackfillITunesTrackPIDs()` to use streaming parser; process albums and flush batch writes incrementally instead of building full in-memory maps
- `internal/itunes/backfill_test.go` — Update tests to verify streaming behavior; add memory profile test for backfill

## Steps
1. **Add streaming parser to parser.go**
   - Implement `StreamingParseLibrary(ctx, path, onTrack callback)` that:
     - Opens file and streams XML token-by-token (xml.Decoder)
     - Unmarshals each track dict on-the-fly without building full Library
     - Calls `onTrack(track)` for each parsed track
     - Respects context cancellation
   - Keep existing `ParseLibrary()` for backward compatibility with other code

2. **Refactor BackfillITunesTrackPIDs to use streaming parser**
   - Remove `lib, err := ParseLibrary(...)` and full albums map
   - Replace with `StreamingParseLibrary()` callback:
     - Build up only the current album being processed
     - When album changes or EOF, flush pending batch and reset
     - Keep only `pidToBook` and `titleToBook` indexes in memory (lightweight, ~12K + 40K entries)
   - Batch writes every 5000 tracks (same as before, but streaming)
   - Add progress logging every 10K tracks

3. **Update tests**
   - Verify streaming parser yields same track count as full parser
   - Add test for context cancellation during parse
   - Add memory profile assertion for backfill (peak <500MB)
   - Ensure BackfillITunesTrackPIDs still produces same external_id_mapping count

## Test strategy
- **Unit tests:** `make test -- -run TestBackfill` (backfill_test.go)
- **Memory profile:** Run backfill on staging/prod with `pprof` to verify <500MB peak
- **Integration:** Restart prod service, trigger backfill manually via admin API, monitor memory via systemctl
- **Success criteria:**
  - All existing backfill tests pass
  - Memory peak <500MB during backfill (vs current 53GB)
  - Same number of external ID mappings created as before
  - Backfill completes in <5 minutes

## Rollback
- Revert to `main` and rebuild: `git checkout main && make clean && make build`
- Restart service: `sudo systemctl restart audiobook-organizer.service`
- No data migration needed — backfill is idempotent and already completed on prod (flag set)
