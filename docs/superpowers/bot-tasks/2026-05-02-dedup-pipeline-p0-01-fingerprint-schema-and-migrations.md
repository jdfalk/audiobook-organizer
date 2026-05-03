<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p0-01-fingerprint-schema-and-migrations.md -->
<!-- version: 1.0.0 -->
<!-- guid: d5802265-0e6c-475a-b4ba-6f9a0b417cb3 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Create fingerprints.db schema + migration runner

**Pipeline phase:** P0
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

_None — first task in the chain._


## Branch

```
feat/dedup-pipeline-p0-01-fingerprint-schema-and-migrations
```

## Label

```bash
gh label create "task:PIPE-P0-01" --color "1d76db" --description "Bot task: Create fingerprints.db schema + migration runner" 2>/dev/null || true
```

## What This Does

Creates a new SQLite sidecar `fingerprints.db` that holds the forever-store of
audio file fingerprints. This is intentionally *separate* from the operational
database so destructive maintenance jobs cannot touch it.

The schema includes three tables: `fingerprint_files`, `fingerprint_aliases`,
and `fingerprint_match_log`. See spec §3.2 for column rationale.

## Files to Create / Edit

1. **Create** `internal/database/fingerprint_db.go` — `OpenFingerprintDB` constructor + migration runner
2. **Create** `internal/database/fingerprint_db_test.go` — schema round-trip test
3. **Edit** `internal/config/config.go` — add `FingerprintDBPath string` field with default `data/fingerprints.db`

## Step 1 — Create the DB module

```go
// file: internal/database/fingerprint_db.go
// version: 1.0.0
// guid: <fresh-uuid>
// last-edited: 2026-05-02

package database

import (
    "database/sql"
    "fmt"

    _ "github.com/mattn/go-sqlite3"
)

// OpenFingerprintDB opens (and migrates) the forever-store SQLite database
// that records every (sha256, chromaprint, acoustid) tuple this server has
// ever seen — even for files that have since been deleted from the library.
func OpenFingerprintDB(path string) (*sql.DB, error) {
    db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_synchronous=NORMAL")
    if err != nil {
        return nil, fmt.Errorf("open fingerprints.db: %w", err)
    }
    if err := migrateFingerprintDB(db); err != nil {
        _ = db.Close()
        return nil, err
    }
    return db, nil
}

func migrateFingerprintDB(db *sql.DB) error {
    stmts := []string{
        `CREATE TABLE IF NOT EXISTS fingerprint_files (
            sha256              TEXT PRIMARY KEY,
            size_bytes          INTEGER NOT NULL,
            container_format    TEXT,
            audio_codec         TEXT,
            duration_seconds    REAL,
            stream_content_hash TEXT,
            chromaprint_full    TEXT,
            chromaprint_intro   TEXT,
            chromaprint_outro   TEXT,
            chromaprint_body    TEXT,
            acoustid_mbid       TEXT,
            acoustid_score      REAL,
            first_filename      TEXT NOT NULL,
            first_path          TEXT NOT NULL,
            first_seen_at       DATETIME NOT NULL,
            last_seen_at        DATETIME NOT NULL,
            deleted_at          DATETIME,
            deletion_history    TEXT NOT NULL DEFAULT '[]',
            schema_version      INTEGER NOT NULL DEFAULT 1
        )`,
        `CREATE INDEX IF NOT EXISTS idx_fp_chromaprint_full ON fingerprint_files(chromaprint_full)`,
        `CREATE INDEX IF NOT EXISTS idx_fp_chromaprint_intro ON fingerprint_files(chromaprint_intro)`,
        `CREATE INDEX IF NOT EXISTS idx_fp_acoustid ON fingerprint_files(acoustid_mbid)`,
        `CREATE INDEX IF NOT EXISTS idx_fp_deleted_at ON fingerprint_files(deleted_at)`,
        `CREATE TABLE IF NOT EXISTS fingerprint_aliases (
            sha256   TEXT NOT NULL,
            seen_at  DATETIME NOT NULL,
            filename TEXT NOT NULL,
            path     TEXT NOT NULL,
            book_id  TEXT,
            PRIMARY KEY (sha256, seen_at, path)
        )`,
        `CREATE TABLE IF NOT EXISTS fingerprint_match_log (
            id              INTEGER PRIMARY KEY AUTOINCREMENT,
            incoming_sha256 TEXT NOT NULL,
            matched_sha256  TEXT NOT NULL,
            signal_kind     TEXT NOT NULL,
            distance        REAL,
            matched_at      DATETIME NOT NULL,
            decision        TEXT
        )`,
    }
    for _, s := range stmts {
        if _, err := db.Exec(s); err != nil {
            return fmt.Errorf("fingerprints migration failed: %w\nSQL: %s", err, s)
        }
    }
    return nil
}
```

## Step 2 — Test

```go
// file: internal/database/fingerprint_db_test.go
// version: 1.0.0
// guid: <fresh-uuid>

package database

import (
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestOpenFingerprintDB_Migrations(t *testing.T) {
    db, err := OpenFingerprintDB(filepath.Join(t.TempDir(), "fp.db"))
    require.NoError(t, err)
    defer db.Close()

    for _, table := range []string{"fingerprint_files", "fingerprint_aliases", "fingerprint_match_log"} {
        var name string
        err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
        require.NoError(t, err, "table %s missing", table)
    }
}

func TestOpenFingerprintDB_Idempotent(t *testing.T) {
    p := filepath.Join(t.TempDir(), "fp.db")
    for i := 0; i < 3; i++ {
        db, err := OpenFingerprintDB(p)
        require.NoError(t, err)
        require.NoError(t, db.Close())
    }
}
```

## Step 3 — Config field

In `internal/config/config.go`, add to the main `Config` struct:

```go
FingerprintDBPath string `yaml:"fingerprint_db_path" json:"fingerprint_db_path"`
```

In the defaults helper (search for `DefaultConfig` or where other DB paths are
defaulted), add:

```go
if cfg.FingerprintDBPath == "" {
    cfg.FingerprintDBPath = "data/fingerprints.db"
}
```

## Verify

```bash
go test ./internal/database/ -run TestOpenFingerprintDB
go vet ./internal/database/...
```

## Definition of Done

- [ ] `OpenFingerprintDB` opens an empty path and creates all three tables
- [ ] Re-opening the same path is a no-op (idempotent)
- [ ] `FingerprintDBPath` config field exists with default `data/fingerprints.db`
- [ ] `make build-api` succeeds
- [ ] Tests pass


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): create fingerprints.db schema + migration runner (PIPE-P0-01)" \
  --body "Implements PIPE-P0-01 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P0-01"
```
