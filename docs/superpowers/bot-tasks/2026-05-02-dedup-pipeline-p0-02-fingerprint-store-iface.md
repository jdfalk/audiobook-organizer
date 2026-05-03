<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p0-02-fingerprint-store-iface.md -->
<!-- version: 1.0.0 -->
<!-- guid: 3c25f7ea-493d-4f53-a05c-c9b07e4bda5f -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Define FingerprintStore interface + record types

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
feat/dedup-pipeline-p0-02-fingerprint-store-iface
```

## Label

```bash
gh label create "task:PIPE-P0-02" --color "1d76db" --description "Bot task: Define FingerprintStore interface + record types" 2>/dev/null || true
```

## What This Does

Defines the public `FingerprintStore` interface (spec §3.3) and the supporting
record types. No implementation yet — that lands in P0-03. Splitting the
interface from the impl lets P1-* signal stages depend on the interface alone
and be unblocked as soon as P0-02 merges.

## Files to Create / Edit

1. **Create** `internal/database/fingerprint_store.go`

## Step 1 — Interface + types

```go
// file: internal/database/fingerprint_store.go
// version: 1.0.0
// guid: <fresh-uuid>
// last-edited: 2026-05-02

package database

import "time"

// FingerprintRecord is one row of the forever-store. Mirrors the
// fingerprint_files SQLite table; see spec §3.2.
type FingerprintRecord struct {
    SHA256             string
    SizeBytes          int64
    ContainerFormat    string
    AudioCodec         string
    DurationSeconds    float64
    StreamContentHash  string
    ChromaprintFull    string
    ChromaprintIntro   string
    ChromaprintOutro   string
    ChromaprintBody    []string // JSON-encoded in storage
    AcoustIDMBID       string
    AcoustIDScore      float64
    FirstFilename      string
    FirstPath          string
    FirstSeenAt        time.Time
    LastSeenAt         time.Time
    DeletedAt          *time.Time
    DeletionHistory    []DeletionEvent
}

// DeletionEvent records a single deletion (or restoration) of the file
// associated with a SHA. Append-only.
type DeletionEvent struct {
    Timestamp time.Time `json:"ts"`
    Reason    string    `json:"reason"`
    User      string    `json:"user"`
    Action    string    `json:"action"` // "deleted" | "resurrected"
}

// FingerprintMatchLogEntry records that an incoming file matched an existing
// forever-store record. Used for forensic auditing of auto-merge decisions.
type FingerprintMatchLogEntry struct {
    IncomingSHA256 string
    MatchedSHA256  string
    SignalKind     string
    Distance       float64
    MatchedAt      time.Time
    Decision       string
}

// FingerprintStoreStats are the aggregate counters surfaced on the
// identification UI.
type FingerprintStoreStats struct {
    TotalFiles        int64
    LiveFiles         int64
    DeletedFiles      int64
    WithChromaprint   int64
    WithAcoustID      int64
    LastIngestAt      time.Time
}

// FingerprintStore is the forever-store interface. Implementations MUST be
// safe for concurrent use. Implementations MUST NOT physically delete rows;
// MarkDeleted only annotates.
type FingerprintStore interface {
    LookupBySHA(sha string) (*FingerprintRecord, error)
    LookupByChromaprintFull(fp string, minSimilarity float64) ([]FingerprintRecord, error)
    LookupByChromaprintSegment(seg string, minSimilarity float64) ([]FingerprintRecord, error)
    LookupByAcoustID(mbid string) ([]FingerprintRecord, error)
    Upsert(r FingerprintRecord) error
    AddAlias(sha, filename, path string, bookID *string) error
    MarkDeleted(sha, reason, user string) error
    Resurrect(sha string) error
    LogMatch(entry FingerprintMatchLogEntry) error
    Stats() (FingerprintStoreStats, error)
}
```

## Verify

```bash
go vet ./internal/database/...
go build ./...
```

## Definition of Done

- [ ] Interface compiles
- [ ] No new code paths actually use the interface yet (impl arrives in P0-03)
- [ ] `go build ./...` succeeds


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): define fingerprintstore interface + record types (PIPE-P0-02)" \
  --body "Implements PIPE-P0-02 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P0-02"
```
