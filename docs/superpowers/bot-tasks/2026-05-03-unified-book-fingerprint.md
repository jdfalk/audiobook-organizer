<!-- file: docs/superpowers/bot-tasks/2026-05-03-unified-book-fingerprint.md -->
<!-- version: 1.0.0 -->
<!-- guid: 5ab5d6de-9c60-4974-8fdb-6cd5562d69f5 -->
<!-- last-edited: 2026-05-03 -->

# BOT TASK: Unified per-book audio fingerprint + book-level matching

**Companion master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Branch

```
feat/unified-book-fingerprint
```

## Problem

Today's `Engine.AcoustIDScan` (`internal/dedup/engine.go:1723`) compares
per-file 7-segment fingerprints (`book_files.acoustid_seg{0..6}`) and emits a
candidate when **any** seg from book A matches **any** seg from book B. That
is fine when both copies of a book have the same file split, but it fails
predictably for the multi-file case the user actually has:

- Book A is shipped as a single 18-hour `.m4b`
- Book B is the same content split into 30 numbered `.mp3` chapter files

The two copies are the same audiobook, but their per-file segments line up
on different time boundaries. Matches are coincidental, not structural.

There is no "canonical book-level signature" to compare against, and no
matcher that understands "book A's full audio == concatenation of book B's
files in order".

## Goal

Add a deterministic **book-level** fingerprint synthesized from the per-file
segments, and matcher logic that compares two books on their book-level
signatures regardless of split structure.

## Files to add / change

1. **NEW** `internal/fingerprint/book_signature.go` — synthesis + comparison
2. **NEW** `internal/fingerprint/book_signature_test.go`
3. **EDIT** `internal/database/store.go` (and pebble + sqlite implementations) —
   add `book_signature` columns / fields on the `books` table:
   `book_sig_v1` (TEXT, base64 of synthesized fp), `book_sig_segments`
   (INTEGER count), `book_sig_built_at` (TIMESTAMP)
4. **EDIT** `internal/database/migrations.go` — migration adding the columns
5. **EDIT** `internal/dedup/engine.go` — new `BookSignatureScan` method that
   walks all primary books, ensures the book signature is built, then runs
   pairwise compare with `BookSignatureSimilarity`. Emit `dedup_candidates`
   rows with `layer="book_signature"`.
6. **EDIT** `internal/server/dedup_handlers.go` — new
   `triggerBookSignatureScan` mirroring `triggerDedupAcoustID`
7. **EDIT** `internal/server/server_lifecycle.go` — route
   `POST /api/v1/dedup/scan-book-signature` with `PermScanTrigger`
8. **EDIT** `internal/server/acoustid_backfill.go` — after generating per-file
   segments for a book, synthesize the book signature in the same pass
9. **NEW** `docs/audio-fingerprint/book-signature.md` — design doc

## Algorithm (v1)

### Synthesis

For a book with N files, ordered by `book_files.sort_order` (or
`original_filename` if sort_order is null):

1. Skip files where `acoustid_seg0` is empty (those are still queued for
   fingerprinting — the book signature is incomplete and `book_sig_v1`
   should remain NULL).
2. Concatenate `[seg0, seg1, …, seg6]` from each file, in file order:
   `flat = file0.seg0 || file0.seg1 || … || file0.seg6 || file1.seg0 || …`
3. Each segment is a base64-encoded 32-bit chromaprint hash array; decode
   each to a uint32 slice, concatenate the slices into one big slice
   `bookFP []uint32`.
4. Down-sample `bookFP` to a fixed length (e.g. 4096 uint32s) by
   max-pooling consecutive non-overlapping windows. This makes
   different-length books comparable and bounds the storage cost.
5. Re-encode the down-sampled slice to base64 → `book_sig_v1`.
6. Store `len(bookFP)` (pre-downsample) in `book_sig_segments` for
   diagnostics.

### Comparison

`BookSignatureSimilarity(a, b string) (float64, error)`:

1. Decode both signatures back to `[]uint32` (length 4096 each).
2. Compute Hamming distance per uint32 word (`bits.OnesCount32(a[i] ^ b[i])`),
   sum, divide by `4096 * 32` to get a normalized error rate.
3. `similarity = 1.0 - errorRate`.
4. Same threshold guidance as `internal/fingerprint.HammingSimilarity`:
   exact = 1.0, near-duplicate ≥ `FuzzyMinSimilarity` (currently ~0.85),
   weak match ≥ 0.7.

### Why this works for split mismatches

Down-sampling to a fixed length means a 30-file split and a 1-file copy of
the same book produce **the same length** book signature, and (modulo file
ordering and intro/outro padding) the down-sampled hash sequences will
overlap the same audio peaks. Files don't have to align on segment
boundaries — the pooling collapses the structural difference.

### Edge cases

- **Bonus content / intros / outros that differ.** Down-sampling smooths
  these out into noise distributed across the signature. Threshold tuning
  (start at 0.85, lower if false negatives) handles this.
- **Reordered chapters.** Files are sorted by sort_order or filename. If
  filenames are inconsistent, we fall back to scanning permutations of
  segment groups (out of scope for v1 — log a `book_sig_unstable_order`
  warning instead).
- **Books with one file pending fingerprinting.** Signature is left NULL.
  Backfill will fill it in on its next pass; the matcher skips NULL sigs.
- **Books > 4096 segments after concatenation** (very long books with many
  files). Pooling window grows; the signature length stays 4096. Resolution
  drops slightly but threshold still works because the same pooling is
  applied to the comparison.

## Storage

The down-sampled signature is `4096 * 4 bytes = 16 KiB` raw,
~22 KiB base64-encoded. For 50,000 books this is ~1.1 GiB. Acceptable —
store inline on the `books` row, no separate table.

## Test plan

In `book_signature_test.go`:

1. Synthesize signature from a known-good 7-segment array, verify
   deterministic base64 output.
2. `BookSignatureSimilarity(sig, sig) == 1.0`.
3. Synthesize from a 30-file split using fixture segments, then synthesize
   from a 1-file version of the same audio (also fixtures), assert
   similarity ≥ 0.85.
4. Synthesize from two unrelated books, assert similarity ≤ 0.4.
5. Skip-on-incomplete: book with one missing seg0 returns
   `(string(""), ErrIncompleteFingerprint)`.

In `dedup/engine_test.go`:

6. `BookSignatureScan` over a small fixture library emits the right
   candidates with `layer="book_signature"` and the right similarity
   score.

In `acoustid_backfill_test.go`:

7. After backfill on a fixture book, `book_sig_v1` is populated and not
   empty.

## Acceptance

- `make test ./internal/fingerprint/... ./internal/dedup/... ./internal/server/...` green
- `make build-api` green
- New `POST /api/v1/dedup/scan-book-signature` returns 202 with an
  operation_id; the operation enqueues, runs, and surfaces a candidate
  count via the existing scan-status endpoints
- A targeted manual test: a 1-file vs split-file copy of the same book
  in the production library produces a `book_signature` candidate with
  similarity ≥ 0.85

## Rollback

Schema migration is additive (three new nullable columns). Drop them or
leave them in place; the matcher just skips NULL signatures.

## Out of scope (v1)

- Permutation search for shuffled splits — emit a warning and skip
- Cross-version matching (different narrators) — that's the embedding /
  metadata layer's job
- Updating an in-flight book signature on partial file change — for now
  recompute the full signature when any file changes
