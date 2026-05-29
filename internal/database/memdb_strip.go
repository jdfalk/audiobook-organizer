// file: internal/database/memdb_strip.go
// version: 1.0.0
// guid: a1b2c3d4-mema-aaaa-aaaa-stripbook0001

package database

// stripBookForMemdb returns a shallow copy of `src` with heavy, rarely-
// queried fields cleared. Memdb-resident Books are used for indexed
// iteration and predicate filtering — they don't need the full payload.
// Callers that need the full Book (UI enrichment, write paths) fetch it
// from Pebble via GetBookByID, which is the canonical source.
//
// Memory math (392K-book production library):
//
//	Description avg ~500-2000 chars  → ~400MB-1.5GB across all books
//	BookSigV1 base64 4096 uint32s    → ~22KB per fingerprinted book
//	BookSigV1Mask base64 4096-bit    → ~700B per fingerprinted book
//	VersionNotes (rare, multi-line)  → typically empty, occasionally KB
//
// Stripping these from memdb cuts the radix tree's resident size from
// ~10GB to ~2GB. Fields are cleared to nil pointers (saves the string
// data, not just the pointer), so the underlying string bytes become
// GC-eligible after the original *Book goes out of scope.
//
// Predicates that filter by these fields (e.g. `field:description`)
// silently miss against stripped books. The predicate paths that need
// them (rare) should be routed through Pebble's GetBookByID instead.
func stripBookForMemdb(src *Book) *Book {
	if src == nil {
		return nil
	}
	cp := *src
	cp.Description = nil
	cp.VersionNotes = nil
	cp.BookSigV1 = nil
	cp.BookSigV1Mask = nil
	cp.BookSigSegments = nil
	cp.BookSigBuiltAt = nil
	cp.BookSigCoveragePct = nil
	// Pre-resolved Author/Series pointers are nil at warm time anyway —
	// they're hydrated separately via authorsMap/seriesMap in the
	// service layer. Clear defensively in case a caller pre-fills them.
	cp.Author = nil
	cp.Series = nil
	return &cp
}

// stripBookFileForMemdb returns a shallow copy of `src` with heavy
// fingerprint-diagnostic fields cleared before memdb insertion. Mirrors the
// stripBookForMemdb pattern: memdb holds a projection sufficient for indexed
// iteration and predicate filtering; callers needing the full payload fetch
// it from Pebble via GetBookFiles(bookID).
//
// Memory math (~308K book_files in production):
//
//	AcoustIDSeg1..6 (6 strings × ~300-500B each)  → ~60-90 MB total
//	FingerprintDiagnosticJSON (*string, KB-class) → can dominate when populated
//	FingerprintFailureReason / Detail (*string)   → small but per-row
//	FingerprintFailedAt (*time.Time)              → 24B + heap overhead
//
// Combined expected drop: ~70 MB.
//
// Critical: AcoustIDSeg0 is intentionally NOT stripped. It is read on every
// /api/v1/audiobooks list response by fingerprint.ComputeFingerprintFields
// (via the memdb-routed GetBookFilesForIDs path) to compute the per-book
// fingerprint_status badge. Clearing seg0 would silently make every book
// report "none".
//
// Safe to strip because:
//   - Seg1..6 + diagnostic fields are only read by acoustid backfill,
//     fingerprint_diagnosis_handler, and dedup. Backfill and dedup fetch
//     files via GetBookFiles(bookID), which is Pebble-direct (no memdb).
//     fingerprint_diagnosis_handler is rerouted in this PR to use
//     getAllBookFilesPebbleScan() so it also bypasses memdb.
//   - listBookFiles handler (which exposes seg1..6 in API responses) uses
//     GetBookFiles(bookID) — Pebble-direct, sees the full payload.
func stripBookFileForMemdb(src *BookFile) *BookFile {
	if src == nil {
		return nil
	}
	cp := *src
	cp.AcoustIDSeg1 = ""
	cp.AcoustIDSeg2 = ""
	cp.AcoustIDSeg3 = ""
	cp.AcoustIDSeg4 = ""
	cp.AcoustIDSeg5 = ""
	cp.AcoustIDSeg6 = ""
	cp.FingerprintFailedAt = nil
	cp.FingerprintFailureReason = nil
	cp.FingerprintFailureDetail = nil
	cp.FingerprintDiagnosticJSON = nil
	return &cp
}
