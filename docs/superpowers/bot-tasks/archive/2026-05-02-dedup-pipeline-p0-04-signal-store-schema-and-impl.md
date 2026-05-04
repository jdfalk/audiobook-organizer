<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p0-04-signal-store-schema-and-impl.md -->
<!-- version: 1.0.0 -->
<!-- guid: af56b8c6-ebac-448d-a600-623c737f749d -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Create signal_store table + Go store

**Pipeline phase:** P0
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P0-01` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P0-01" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P0-01"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p0-04-signal-store-schema-and-impl
```

## Label

```bash
gh label create "task:PIPE-P0-04" --color "1d76db" --description "Bot task: Create signal_store table + Go store" 2>/dev/null || true
```

## What This Does

Adds the `signal_store` table to the **main** SQLite database (not the
forever-store) and a thin Go API over it. Signals are per-`(book_id, kind)`
records emitted by every Phase 1 stage.

## Files to Create / Edit

1. **Edit** `internal/database/migrations.go` — add new `CREATE TABLE` near
   the other dedup-related tables (search for `dedup_candidates`).
2. **Create** `internal/dedup/signals/kind.go`
3. **Create** `internal/dedup/signals/store.go`
4. **Create** `internal/dedup/signals/store_test.go`

## Step 1 — Migration

Add this block inside `migrateSchema` (or the closest equivalent) in
`internal/database/migrations.go`. Use `CREATE TABLE IF NOT EXISTS` so it is
safe to run twice.

```go
`CREATE TABLE IF NOT EXISTS dedup_signals (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id       TEXT NOT NULL,
    file_id       TEXT,
    kind          TEXT NOT NULL,
    value         TEXT NOT NULL,
    score         REAL NOT NULL,
    confidence    REAL NOT NULL,
    source        TEXT NOT NULL,
    evidence_json TEXT,
    computed_at   DATETIME NOT NULL,
    expires_at    DATETIME
)`,
`CREATE INDEX IF NOT EXISTS idx_dedup_signals_book ON dedup_signals(book_id)`,
`CREATE INDEX IF NOT EXISTS idx_dedup_signals_kind_value ON dedup_signals(kind, value)`,
```

## Step 2 — Constants

```go
// file: internal/dedup/signals/kind.go
// version: 1.0.0
// guid: <fresh-uuid>

package signals

// Kind identifies a class of evidence the dedup matrix consumes. Add a new
// constant here whenever a new pipeline stage is introduced. Persisted as a
// column value, so DO NOT renumber or rename existing constants.
type Kind string

const (
    KindSHAExact              Kind = "sha_exact"
    KindStreamContentHash     Kind = "stream_content_hash"
    KindChromaprintFull       Kind = "chromaprint_full"
    KindChromaprintSegment    Kind = "chromaprint_segment"
    KindAcoustIDMatch         Kind = "acoustid_match"
    KindTagMatch              Kind = "tag_match"
    KindFilenameMatch         Kind = "filename_match"
    KindEmbeddingSimilarity   Kind = "embedding_similarity"
    KindWhisperIntroMatch     Kind = "whisper_intro_match"
    KindWhisperIntroNegative  Kind = "whisper_intro_negative"
    KindForeverStoreResurrect Kind = "forever_store_resurrect"
)

// Weight returns the matrix weight applied to this signal. Source: master spec
// §4.1. If you change a value, you MUST also update the spec and bump the
// signal_revision column on books so the matrix re-runs.
func Weight(k Kind) float64 {
    switch k {
    case KindSHAExact, KindForeverStoreResurrect: return 1.00
    case KindStreamContentHash: return 0.90
    case KindChromaprintFull: return 0.70
    case KindChromaprintSegment: return 0.50
    case KindAcoustIDMatch: return 0.55
    case KindWhisperIntroMatch: return 0.45
    case KindTagMatch: return 0.30
    case KindEmbeddingSimilarity: return 0.25
    case KindFilenameMatch: return 0.15
    case KindWhisperIntroNegative: return -0.20
    }
    return 0
}
```

## Step 3 — Store

```go
// file: internal/dedup/signals/store.go
// version: 1.0.0
// guid: <fresh-uuid>

package signals

import (
    "database/sql"
    "time"
)

type Signal struct {
    ID           int64
    BookID       string
    FileID       string
    Kind         Kind
    Value        string
    Score        float64
    Confidence   float64
    Source       string
    EvidenceJSON []byte
    ComputedAt   time.Time
    ExpiresAt    *time.Time
}

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Insert(sig Signal) error {
    _, err := s.db.Exec(`INSERT INTO dedup_signals
        (book_id, file_id, kind, value, score, confidence, source, evidence_json, computed_at, expires_at)
        VALUES (?,?,?,?,?,?,?,?,?,?)`,
        sig.BookID, sig.FileID, string(sig.Kind), sig.Value, sig.Score, sig.Confidence,
        sig.Source, sig.EvidenceJSON, sig.ComputedAt, sig.ExpiresAt)
    return err
}

func (s *Store) ListByBook(bookID string) ([]Signal, error) {
    rows, err := s.db.Query(`SELECT id, book_id, COALESCE(file_id,''), kind, value, score, confidence,
        source, COALESCE(evidence_json,''), computed_at, expires_at FROM dedup_signals
        WHERE book_id = ? ORDER BY computed_at DESC`, bookID)
    if err != nil { return nil, err }
    defer rows.Close()
    out := make([]Signal, 0)
    for rows.Next() {
        var sig Signal
        var ev string
        var exp sql.NullTime
        if err := rows.Scan(&sig.ID, &sig.BookID, &sig.FileID, &sig.Kind, &sig.Value,
            &sig.Score, &sig.Confidence, &sig.Source, &ev, &sig.ComputedAt, &exp); err != nil {
            return nil, err
        }
        if ev != "" { sig.EvidenceJSON = []byte(ev) }
        if exp.Valid { t := exp.Time; sig.ExpiresAt = &t }
        out = append(out, sig)
    }
    return out, rows.Err()
}

func (s *Store) DeleteForBook(bookID string) error {
    _, err := s.db.Exec(`DELETE FROM dedup_signals WHERE book_id = ?`, bookID)
    return err
}
```

## Step 4 — Test

```go
// file: internal/dedup/signals/store_test.go
// version: 1.0.0
// guid: <fresh-uuid>

package signals

import (
    "database/sql"
    "testing"
    "time"

    _ "github.com/mattn/go-sqlite3"
    "github.com/stretchr/testify/require"
)

func newDB(t *testing.T) *sql.DB {
    db, err := sql.Open("sqlite3", ":memory:")
    require.NoError(t, err)
    _, err = db.Exec(`CREATE TABLE dedup_signals (
        id INTEGER PRIMARY KEY AUTOINCREMENT, book_id TEXT NOT NULL, file_id TEXT,
        kind TEXT NOT NULL, value TEXT NOT NULL, score REAL NOT NULL,
        confidence REAL NOT NULL, source TEXT NOT NULL, evidence_json TEXT,
        computed_at DATETIME NOT NULL, expires_at DATETIME)`)
    require.NoError(t, err)
    return db
}

func TestStore_InsertAndList(t *testing.T) {
    db := newDB(t)
    s := NewStore(db)
    require.NoError(t, s.Insert(Signal{BookID: "b1", Kind: KindSHAExact,
        Value: "abc", Score: 1.0, Confidence: 1.0, Source: "sha256-full",
        ComputedAt: time.Now()}))
    out, err := s.ListByBook("b1")
    require.NoError(t, err)
    require.Len(t, out, 1)
    require.Equal(t, KindSHAExact, out[0].Kind)
}

func TestWeight_KnownKinds(t *testing.T) {
    require.Equal(t, 1.00, Weight(KindSHAExact))
    require.Equal(t, -0.20, Weight(KindWhisperIntroNegative))
    require.Equal(t, 0.0, Weight(Kind("nonexistent")))
}
```

## Verify

```bash
go test ./internal/dedup/signals/... -count=1
go vet ./internal/dedup/signals/...
```

## Definition of Done

- [ ] Migration runs cleanly on a fresh DB
- [ ] All `Kind` constants documented and weighted to match spec §4.1
- [ ] Tests pass


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): create signal_store table + go store (PIPE-P0-04)" \
  --body "Implements PIPE-P0-04 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P0-04"
```
