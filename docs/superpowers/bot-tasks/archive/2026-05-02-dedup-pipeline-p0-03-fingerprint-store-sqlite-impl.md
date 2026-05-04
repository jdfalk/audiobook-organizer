<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p0-03-fingerprint-store-sqlite-impl.md -->
<!-- version: 1.0.0 -->
<!-- guid: 24c1c3df-0595-48f7-9f03-6982f534eaed -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: SQLite implementation of FingerprintStore

**Pipeline phase:** P0
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P0-01` — must be merged before this task starts
- `task:P0-02` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P0-01" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P0-01"; exit 0; }
count=$(gh pr list --label "task:P0-02" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P0-02"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p0-03-fingerprint-store-sqlite-impl
```

## Label

```bash
gh label create "task:PIPE-P0-03" --color "1d76db" --description "Bot task: SQLite implementation of FingerprintStore" 2>/dev/null || true
```

## What This Does

Implements `FingerprintStore` against the `fingerprints.db` SQLite database
created in P0-01. Includes the chromaprint similarity matcher (Hamming distance
on base64-decoded segments — already implemented for `internal/fingerprint`,
*reuse* don't reimplement: see `internal/fingerprint/fpcalc.go` and the
`FuzzyMinSimilarity` constant).

## Files to Create / Edit

1. **Create** `internal/database/fingerprint_store_sqlite.go`
2. **Create** `internal/database/fingerprint_store_sqlite_test.go`

## Step 1 — Implementation skeleton

```go
// file: internal/database/fingerprint_store_sqlite.go
// version: 1.0.0
// guid: <fresh-uuid>
// last-edited: 2026-05-02

package database

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "sync"
    "time"

    "github.com/jdfalk/audiobook-organizer/internal/fingerprint"
)

type sqliteFingerprintStore struct {
    db *sql.DB
    mu sync.RWMutex
}

// NewSQLiteFingerprintStore wraps an open *sql.DB (created via
// OpenFingerprintDB) as a FingerprintStore.
func NewSQLiteFingerprintStore(db *sql.DB) FingerprintStore {
    return &sqliteFingerprintStore{db: db}
}

func (s *sqliteFingerprintStore) LookupBySHA(sha string) (*FingerprintRecord, error) {
    row := s.db.QueryRow(`SELECT sha256, size_bytes, container_format, audio_codec,
        duration_seconds, stream_content_hash, chromaprint_full, chromaprint_intro,
        chromaprint_outro, chromaprint_body, acoustid_mbid, acoustid_score,
        first_filename, first_path, first_seen_at, last_seen_at, deleted_at,
        deletion_history FROM fingerprint_files WHERE sha256 = ?`, sha)
    return scanFingerprintRow(row)
}

func (s *sqliteFingerprintStore) Upsert(r FingerprintRecord) error {
    bodyJSON, _ := json.Marshal(r.ChromaprintBody)
    histJSON, _ := json.Marshal(r.DeletionHistory)
    _, err := s.db.Exec(`INSERT INTO fingerprint_files
        (sha256, size_bytes, container_format, audio_codec, duration_seconds,
         stream_content_hash, chromaprint_full, chromaprint_intro,
         chromaprint_outro, chromaprint_body, acoustid_mbid, acoustid_score,
         first_filename, first_path, first_seen_at, last_seen_at,
         deleted_at, deletion_history)
        VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
        ON CONFLICT(sha256) DO UPDATE SET
          size_bytes=excluded.size_bytes,
          container_format=COALESCE(excluded.container_format, container_format),
          audio_codec=COALESCE(excluded.audio_codec, audio_codec),
          duration_seconds=COALESCE(excluded.duration_seconds, duration_seconds),
          stream_content_hash=COALESCE(excluded.stream_content_hash, stream_content_hash),
          chromaprint_full=COALESCE(excluded.chromaprint_full, chromaprint_full),
          chromaprint_intro=COALESCE(excluded.chromaprint_intro, chromaprint_intro),
          chromaprint_outro=COALESCE(excluded.chromaprint_outro, chromaprint_outro),
          chromaprint_body=COALESCE(excluded.chromaprint_body, chromaprint_body),
          acoustid_mbid=COALESCE(excluded.acoustid_mbid, acoustid_mbid),
          acoustid_score=COALESCE(excluded.acoustid_score, acoustid_score),
          last_seen_at=excluded.last_seen_at`,
        r.SHA256, r.SizeBytes, r.ContainerFormat, r.AudioCodec, r.DurationSeconds,
        r.StreamContentHash, r.ChromaprintFull, r.ChromaprintIntro, r.ChromaprintOutro,
        bodyJSON, r.AcoustIDMBID, r.AcoustIDScore, r.FirstFilename, r.FirstPath,
        r.FirstSeenAt, r.LastSeenAt, r.DeletedAt, histJSON)
    return err
}

func (s *sqliteFingerprintStore) AddAlias(sha, filename, path string, bookID *string) error {
    _, err := s.db.Exec(`INSERT OR IGNORE INTO fingerprint_aliases
        (sha256, seen_at, filename, path, book_id) VALUES (?,?,?,?,?)`,
        sha, time.Now().UTC(), filename, path, bookID)
    return err
}

func (s *sqliteFingerprintStore) MarkDeleted(sha, reason, user string) error {
    return s.appendHistory(sha, DeletionEvent{Timestamp: time.Now().UTC(), Reason: reason, User: user, Action: "deleted"}, true)
}

func (s *sqliteFingerprintStore) Resurrect(sha string) error {
    return s.appendHistory(sha, DeletionEvent{Timestamp: time.Now().UTC(), Action: "resurrected"}, false)
}

func (s *sqliteFingerprintStore) appendHistory(sha string, ev DeletionEvent, markDeleted bool) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    var raw string
    if err := s.db.QueryRow(`SELECT deletion_history FROM fingerprint_files WHERE sha256=?`, sha).Scan(&raw); err != nil {
        return err
    }
    var hist []DeletionEvent
    _ = json.Unmarshal([]byte(raw), &hist)
    hist = append(hist, ev)
    out, _ := json.Marshal(hist)
    if markDeleted {
        _, err := s.db.Exec(`UPDATE fingerprint_files SET deletion_history=?, deleted_at=? WHERE sha256=?`, out, ev.Timestamp, sha)
        return err
    }
    _, err := s.db.Exec(`UPDATE fingerprint_files SET deletion_history=?, deleted_at=NULL WHERE sha256=?`, out, sha)
    return err
}

func (s *sqliteFingerprintStore) LogMatch(e FingerprintMatchLogEntry) error {
    _, err := s.db.Exec(`INSERT INTO fingerprint_match_log
        (incoming_sha256, matched_sha256, signal_kind, distance, matched_at, decision)
        VALUES (?,?,?,?,?,?)`,
        e.IncomingSHA256, e.MatchedSHA256, e.SignalKind, e.Distance, e.MatchedAt, e.Decision)
    return err
}

func (s *sqliteFingerprintStore) LookupByAcoustID(mbid string) ([]FingerprintRecord, error) {
    rows, err := s.db.Query(`SELECT sha256, size_bytes, container_format, audio_codec,
        duration_seconds, stream_content_hash, chromaprint_full, chromaprint_intro,
        chromaprint_outro, chromaprint_body, acoustid_mbid, acoustid_score,
        first_filename, first_path, first_seen_at, last_seen_at, deleted_at,
        deletion_history FROM fingerprint_files WHERE acoustid_mbid = ?`, mbid)
    if err != nil { return nil, err }
    defer rows.Close()
    return scanFingerprintRows(rows)
}

func (s *sqliteFingerprintStore) LookupByChromaprintFull(fp string, minSim float64) ([]FingerprintRecord, error) {
    return s.fuzzyChromaprint("chromaprint_full", fp, minSim)
}

func (s *sqliteFingerprintStore) LookupByChromaprintSegment(seg string, minSim float64) ([]FingerprintRecord, error) {
    // NOTE: segment can live in chromaprint_intro, chromaprint_outro, or any
    // body slot. Naive scan; for >100k rows replace with min-hash banding.
    return s.fuzzyChromaprint("chromaprint_intro", seg, minSim)
}

func (s *sqliteFingerprintStore) fuzzyChromaprint(col, fp string, minSim float64) ([]FingerprintRecord, error) {
    rows, err := s.db.Query(fmt.Sprintf(`SELECT sha256, size_bytes, container_format, audio_codec,
        duration_seconds, stream_content_hash, chromaprint_full, chromaprint_intro,
        chromaprint_outro, chromaprint_body, acoustid_mbid, acoustid_score,
        first_filename, first_path, first_seen_at, last_seen_at, deleted_at,
        deletion_history FROM fingerprint_files WHERE %s IS NOT NULL`, col))
    if err != nil { return nil, err }
    defer rows.Close()
    all, err := scanFingerprintRows(rows)
    if err != nil { return nil, err }
    out := make([]FingerprintRecord, 0)
    for _, r := range all {
        candidate := r.ChromaprintFull
        if col == "chromaprint_intro" { candidate = r.ChromaprintIntro }
        sim := fingerprint.Similarity(fp, candidate)
        if sim >= minSim { out = append(out, r) }
    }
    return out, nil
}

func (s *sqliteFingerprintStore) Stats() (FingerprintStoreStats, error) {
    var st FingerprintStoreStats
    _ = s.db.QueryRow(`SELECT COUNT(*),
        SUM(CASE WHEN deleted_at IS NULL THEN 1 ELSE 0 END),
        SUM(CASE WHEN deleted_at IS NOT NULL THEN 1 ELSE 0 END),
        SUM(CASE WHEN chromaprint_full IS NOT NULL THEN 1 ELSE 0 END),
        SUM(CASE WHEN acoustid_mbid IS NOT NULL THEN 1 ELSE 0 END),
        COALESCE(MAX(last_seen_at), '1970-01-01') FROM fingerprint_files`).Scan(
        &st.TotalFiles, &st.LiveFiles, &st.DeletedFiles, &st.WithChromaprint,
        &st.WithAcoustID, &st.LastIngestAt)
    return st, nil
}

// helpers ---------------------------------------------------------

func scanFingerprintRow(row *sql.Row) (*FingerprintRecord, error) {
    var r FingerprintRecord
    var bodyJSON, histJSON string
    var deletedAt sql.NullTime
    err := row.Scan(&r.SHA256, &r.SizeBytes, &r.ContainerFormat, &r.AudioCodec,
        &r.DurationSeconds, &r.StreamContentHash, &r.ChromaprintFull, &r.ChromaprintIntro,
        &r.ChromaprintOutro, &bodyJSON, &r.AcoustIDMBID, &r.AcoustIDScore,
        &r.FirstFilename, &r.FirstPath, &r.FirstSeenAt, &r.LastSeenAt,
        &deletedAt, &histJSON)
    if err == sql.ErrNoRows { return nil, nil }
    if err != nil { return nil, err }
    if deletedAt.Valid { t := deletedAt.Time; r.DeletedAt = &t }
    _ = json.Unmarshal([]byte(bodyJSON), &r.ChromaprintBody)
    _ = json.Unmarshal([]byte(histJSON), &r.DeletionHistory)
    return &r, nil
}

func scanFingerprintRows(rows *sql.Rows) ([]FingerprintRecord, error) {
    out := make([]FingerprintRecord, 0)
    for rows.Next() {
        var r FingerprintRecord
        var bodyJSON, histJSON string
        var deletedAt sql.NullTime
        if err := rows.Scan(&r.SHA256, &r.SizeBytes, &r.ContainerFormat, &r.AudioCodec,
            &r.DurationSeconds, &r.StreamContentHash, &r.ChromaprintFull, &r.ChromaprintIntro,
            &r.ChromaprintOutro, &bodyJSON, &r.AcoustIDMBID, &r.AcoustIDScore,
            &r.FirstFilename, &r.FirstPath, &r.FirstSeenAt, &r.LastSeenAt,
            &deletedAt, &histJSON); err != nil { return nil, err }
        if deletedAt.Valid { t := deletedAt.Time; r.DeletedAt = &t }
        _ = json.Unmarshal([]byte(bodyJSON), &r.ChromaprintBody)
        _ = json.Unmarshal([]byte(histJSON), &r.DeletionHistory)
        out = append(out, r)
    }
    return out, rows.Err()
}
```

> **Note:** `fingerprint.Similarity(a,b)` is assumed to exist. If it does not,
> add it as a thin wrapper over the existing fuzzy-Hamming logic in
> `internal/fingerprint/fpcalc.go` (search for `FuzzyMinSimilarity` /
> `Hamming`). Keep that wrapper in scope — DO NOT reimplement Hamming here.

## Step 2 — Tests

```go
// file: internal/database/fingerprint_store_sqlite_test.go
// version: 1.0.0
// guid: <fresh-uuid>

package database

import (
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/require"
)

func newFP(t *testing.T) FingerprintStore {
    db, err := OpenFingerprintDB(filepath.Join(t.TempDir(), "fp.db"))
    require.NoError(t, err)
    t.Cleanup(func() { _ = db.Close() })
    return NewSQLiteFingerprintStore(db)
}

func TestFingerprintStore_UpsertLookup(t *testing.T) {
    store := newFP(t)
    rec := FingerprintRecord{SHA256: "a"+stringRepeat("0",63), SizeBytes: 100,
        FirstFilename: "x.m4b", FirstPath: "/x/x.m4b"}
    require.NoError(t, store.Upsert(rec))
    got, err := store.LookupBySHA(rec.SHA256)
    require.NoError(t, err)
    require.NotNil(t, got)
    require.Equal(t, int64(100), got.SizeBytes)
}

func TestFingerprintStore_MarkDeletedAppendsHistoryAndResurrect(t *testing.T) {
    store := newFP(t)
    sha := "b"+stringRepeat("0",63)
    require.NoError(t, store.Upsert(FingerprintRecord{SHA256: sha, FirstFilename:"x", FirstPath:"x"}))
    require.NoError(t, store.MarkDeleted(sha, "user delete", "alice"))
    got, _ := store.LookupBySHA(sha)
    require.NotNil(t, got.DeletedAt)
    require.Len(t, got.DeletionHistory, 1)
    require.NoError(t, store.Resurrect(sha))
    got, _ = store.LookupBySHA(sha)
    require.Nil(t, got.DeletedAt)
    require.Len(t, got.DeletionHistory, 2)
}

func stringRepeat(s string, n int) string { out := ""; for i := 0; i < n; i++ { out += s }; return out }
```

## Verify

```bash
go test ./internal/database/ -run TestFingerprintStore -count=1
```

## Definition of Done

- [ ] All `FingerprintStore` methods implemented
- [ ] MarkDeleted preserves the row, appends to history JSON, sets `deleted_at`
- [ ] Resurrect clears `deleted_at` and appends a `resurrected` event
- [ ] Tests pass
- [ ] No new top-level `internal/fingerprint` similarity helper if one exists; reuse


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): sqlite implementation of fingerprintstore (PIPE-P0-03)" \
  --body "Implements PIPE-P0-03 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P0-03"
```
