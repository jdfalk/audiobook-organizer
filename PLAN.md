# Partial Book Signature + Structured File Diagnosis

## Goal

Three coupled improvements for files that fail fingerprinting:

**Part A — Partial signatures:** Multi-part books with any failed file currently get no
`book_sig_v1`. Fix by zero-padding the missing slot (estimated from `Duration` /
`FileSize` / `BitrateKbps`), synthesizing a partial sig, and storing a 4096-bit mask so
comparisons exclude the zero-padded region.

**Part B — Structured file diagnosis:** When fingerprinting fails, run a tool cascade
(`file` → `ffprobe` → `mediainfo`) and store the results as a JSON blob on the
`BookFile`. A new `internal/diagnosis/` package handles tool availability caching,
invocation, and structured output.

**Part C — Diagnosis endpoint:** `GET /api/v1/diagnostics/fingerprint-failures` returns
failure counts grouped by reason with full diagnostic detail, so bad files can be acted
on (re-download, delete, ignore).

---

## What the tools give us (verified on production)

**Production has:** `ffprobe` v7.1.1, `mediainfo` v25.04, `file` (system). No `fpcalc`,
no `exiftool`.

| Tool | Key additions over bare fpcalc failure |
|---|---|
| `file` | Magic-byte MIME type; catches empty files ("empty"), wrong-extension (HTML/ZIP named .mp3) |
| `ffprobe -v error -show_error -print_format json` | Structured error code + string; stream codec/bitrate/samplerate/channels; DRM channel decode errors on `.aax` |
| `ffprobe -show_streams -show_format` | `IsStreamable` equivalent via moov position; full stream details |
| `mediainfo --Output=JSON` | `Encoded_Application` ("inAudible 1.94" = originally-DRM Audible rip); `Encryption` field for active DRM; `IsStreamable` (Yes/No); track position + total; `HeaderSize`/`DataSize`/`FooterSize` to detect truncated downloads; encoding library |

**Derived categories from combined output:**

| Reason | Detection logic |
|---|---|
| `empty_file` | `file` says "empty" OR file size == 0 |
| `incomplete_download` | ffprobe `moov atom not found` + file exists + size > 0; or mediainfo `IsStreamable: No` |
| `wrong_format` | `file` MIME is not audio (e.g. text/html, application/zip) |
| `corrupt_audio` | ffprobe error code -1094995529 (`Invalid data found`) without moov hint |
| `active_drm` | mediainfo `Encryption` non-empty; or ffprobe `channel element N.N is not allocated` on .aax/.aa |
| `originally_drm` | mediainfo `Encoded_Application` contains "inAudible" / "DeDRM" / "Requiem" |
| `unsupported_codec` | ffprobe stderr "Decoder … not found" / "no such codec" |
| `too_short` | ffprobe `duration` < 1.0s AND file > 0 bytes |
| `fpcalc_error` | fpcalc/ffmpeg exits non-zero for any other reason |

---

## Affected files

**New package**
- `internal/diagnosis/probe.go` — `FileProbe` struct (tool availability cache),
  `FileDiagnostic` struct, `ProbeFile(path string) FileDiagnostic`
- `internal/diagnosis/probe_test.go` — tests using httptest-style fake executables

**Part A — partial signatures**
- `internal/fingerprint/book_signature.go` — `EstimateSegmentCount`, `FileSegmentInput`,
  `SynthesizePartialBookSignature`, `EncodeMask`, `BookSignatureSimilarityMasked`
- `internal/fingerprint/book_signature_test.go` — tests for all new functions
- `internal/database/store.go` — add `BookSigV1Mask *string` and `BookSigCoveragePct *int`
  to `Book`
- `internal/database/migrations.go` — new migration: 2 columns on `books` table
- `internal/server/acoustid_backfill.go` — `synthesizeBookSignatureForBook` uses partial
  synthesis + saves mask/coverage
- `internal/dedup/engine.go` — `BookSignatureScan` uses masked similarity when mask present

**Part B/C — diagnosis + endpoint**
- `internal/database/store.go` — add `FingerprintFailureReason *string`,
  `FingerprintFailureDetail *string`, `FingerprintDiagnosticJSON *string` to `BookFile`
- `internal/database/migrations.go` — same migration adds 3 columns to `book_files`
- `internal/database/iface_metadata.go` (or whichever owns file-query interfaces) —
  add `GetFilesWithFingerprintFailures(reason string, limit, offset int) ([]BookFile, int64, error)`
- `internal/database/pebble_store.go` — implement `GetFilesWithFingerprintFailures`
  (scan book_file keys, filter on `FingerprintFailedAt != nil`)
- `internal/database/sqlite_store_books.go` — SQL implementation
- `internal/server/acoustid_backfill.go` — `fingerprintBookFile` runs `FileProbe` on
  failure, stores reason + detail + diagnostic JSON
- `internal/server/fingerprint_diagnosis_handler.go` — new `GET /api/v1/diagnostics/fingerprint-failures`
- routing file (check which file registers `/api/v1/diagnostics/*`) — add route

---

## `FileDiagnostic` struct (internal/diagnosis/probe.go)

```go
type FileDiagnostic struct {
    // file(1) output
    FileMagic   string `json:"file_magic,omitempty"`   // "ISO Media, Apple iTunes ALAC/AAC-LC (.M4A) Audio"
    IsEmpty     bool   `json:"is_empty,omitempty"`

    // ffprobe
    ContainerFormat string  `json:"container_format,omitempty"` // "mov,mp4,m4a,3gp,3g2,mj2"
    Codec           string  `json:"codec,omitempty"`
    DurationSec     float64 `json:"duration_sec,omitempty"`
    BitrateKbps     int     `json:"bitrate_kbps,omitempty"`
    SampleRateHz    int     `json:"sample_rate_hz,omitempty"`
    Channels        int     `json:"channels,omitempty"`
    FFProbeErrorStr string  `json:"ffprobe_error,omitempty"`
    FFProbeErrorCode int    `json:"ffprobe_error_code,omitempty"`

    // mediainfo
    MediaInfoFormat        string `json:"mi_format,omitempty"`         // "MPEG-4"
    MediaInfoFormatProfile string `json:"mi_format_profile,omitempty"` // "Apple audio with iTunes info"
    EncodedApplication     string `json:"encoded_application,omitempty"` // "inAudible 1.94"
    EncodedLibrary         string `json:"encoded_library,omitempty"`
    IsStreamable           bool   `json:"is_streamable,omitempty"`     // moov before data
    Encryption             string `json:"encryption,omitempty"`        // active DRM
    TrackPosition          int    `json:"track_position,omitempty"`
    TrackTotal             int    `json:"track_total,omitempty"`
    HasCoverArt            bool   `json:"has_cover_art,omitempty"`
    HeaderSizeBytes        int64  `json:"header_size_bytes,omitempty"`
    DataSizeBytes          int64  `json:"data_size_bytes,omitempty"`

    // Derived
    HasActiveDRM      bool `json:"has_active_drm,omitempty"`
    WasOriginallyDRM  bool `json:"was_originally_drm,omitempty"` // inAudible / DeDRM
    IsTruncated       bool `json:"is_truncated,omitempty"`       // moov not found

    // Meta
    ToolsUsed  []string `json:"tools_used"`
    ProbeError string   `json:"probe_error,omitempty"` // internal probe failure, not file failure
}
```

`FileProbe` caches tool availability in a `sync.Once` block at first use. Tool outputs
are capped at 4 KB each before JSON-marshalling. The full JSON blob is stored in
`BookFile.FingerprintDiagnosticJSON`; `FingerprintFailureReason` and
`FingerprintFailureDetail` are the short summary fields for quick filtering.

---

## Ordered steps

### Step 1 — `internal/diagnosis/probe.go` + `probe_test.go`
New package. `FileProbe.ProbeFile(ctx, path)` runs the tool cascade and returns
`FileDiagnostic`. Tests use `exec.Command` overrides or temp fake binaries to avoid
needing real audio files.

### Step 2 — `internal/fingerprint/book_signature.go` (Part A core)
New functions (existing ones untouched):
- `EstimateSegmentCount(durationSec, fileSizeBytes, bitrateKbps int, peerRatio float64) int`
- `FileSegmentInput` struct
- `SynthesizePartialBookSignature([]FileSegmentInput) (sig, mask string, coveragePct, preLen int, err error)`
- `EncodeMask(realPositions []bool, totalLen, targetLen int) string`
- `BookSignatureSimilarityMasked(a, b, maskA, maskB string) (float64, int, error)`
  — nil/empty mask = "all real"

### Step 3 — `internal/fingerprint/book_signature_test.go` (Part A tests)
Table-driven: `EstimateSegmentCount`, `SynthesizePartialBookSignature`, `EncodeMask`,
`BookSignatureSimilarityMasked`.

### Step 4 — `internal/database/store.go`
Add to `Book`:
```go
BookSigV1Mask      *string `json:"book_sig_v1_mask,omitempty"`
BookSigCoveragePct *int    `json:"book_sig_coverage_pct,omitempty"`
```
Add to `BookFile`:
```go
FingerprintFailureReason  *string `json:"fingerprint_failure_reason,omitempty"`
FingerprintFailureDetail  *string `json:"fingerprint_failure_detail,omitempty"`
FingerprintDiagnosticJSON *string `json:"fingerprint_diagnostic_json,omitempty"`
```

### Step 5 — `internal/database/migrations.go`
Single new migration, 5 nullable columns total:
```sql
ALTER TABLE books      ADD COLUMN book_sig_v1_mask           TEXT;
ALTER TABLE books      ADD COLUMN book_sig_coverage_pct      INTEGER;
ALTER TABLE book_files ADD COLUMN fingerprint_failure_reason TEXT;
ALTER TABLE book_files ADD COLUMN fingerprint_failure_detail TEXT;
ALTER TABLE book_files ADD COLUMN fingerprint_diagnostic_json TEXT;
```

### Step 6 — `internal/database/` interface + store implementations
Add `GetFilesWithFingerprintFailures(reason string, limit, offset int) ([]BookFile, int64, error)`
to the appropriate interface. Implement in pebble (KV scan) and sqlite (SQL query).

### Step 7 — `internal/server/acoustid_backfill.go`
Two changes:

**`fingerprintBookFile`**: on failure, call `FileProbe.ProbeFile`, classify reason, truncate
ffprobe/mediainfo detail to 512 bytes, store all three new fields on the file via
`UpdateBookFile`.

**`synthesizeBookSignatureForBook`**: build `[]FileSegmentInput` with estimated lengths for
missing files, call `SynthesizePartialBookSignature`, save mask + coverage if ≥ 50%.

### Step 8 — `internal/server/fingerprint_diagnosis_handler.go` + route
`GET /api/v1/diagnostics/fingerprint-failures?reason=&limit=&offset=` returns:
```json
{
  "total": 1842,
  "by_reason": { "incomplete_download": 541, "corrupt_audio": 923, ... },
  "files": [{
    "book_id": "…", "book_title": "…", "file_path": "…",
    "reason": "incomplete_download",
    "detail": "moov atom not found",
    "diagnostic": { ...FileDiagnostic fields... },
    "failed_at": "…"
  }]
}
```

### Step 9 — `internal/dedup/engine.go`
In `BookSignatureScan`: use `BookSignatureSimilarityMasked` when either book has a
non-nil `BookSigV1Mask`; skip at DEBUG if overlap < 512 words.

---

## Test strategy

```bash
go test ./internal/diagnosis/...    -v -count=1
go test ./internal/fingerprint/...  -v -count=1
go test ./internal/dedup/...        -v -count=1 -run BookSig
go test ./internal/server/...       -v -count=1 -run "Acoust|FingerprintDiag"
go test ./internal/database/...     -v -count=1 -run "FingerprintFail"
go build ./...
```

Success criteria:
- All new tests pass; `TestBookSignatureSimilarity` unchanged
- `synthesizeBookSignatureForBook` on a book with one missing-fingerprint file → non-nil
  `BookSigV1` with `BookSigCoveragePct` < 100 and non-nil `BookSigV1Mask`
- `fingerprintBookFile` on a missing/corrupt file → `FingerprintFailureReason` set,
  `FingerprintDiagnosticJSON` is valid JSON
- `GET /api/v1/diagnostics/fingerprint-failures` returns valid JSON (integration mock)
- `go build ./...` clean

## Rollback

- All new fields are nullable + `json:",omitempty"` — nil mask = "all real"; old data
  unaffected
- SQLite migrations are additive ALTER TABLE ADD COLUMN
- New `diagnosis` package is purely additive; nothing imports it except `acoustid_backfill.go`
- New route is additive
- To revert: `git revert <merge-commit>`
