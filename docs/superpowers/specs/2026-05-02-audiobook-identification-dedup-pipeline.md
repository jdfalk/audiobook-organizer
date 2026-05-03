<!-- file: docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md -->
<!-- version: 1.0.0 -->
<!-- guid: 95a8c54d-c601-4347-82cb-675a6aeeca8d -->
<!-- last-edited: 2026-05-02 -->

# Unified Audiobook Identification + Deduplication Pipeline

> **Status:** Design spec, ready for bot-task burndown.
> **Supersedes:** `docs/archive/superpowers/plans/2026-04-09-embedding-dedup.md`,
> `docs/superpowers/specs/2026-04-09-embedding-dedup-design.md`,
> `internal/maintenance/jobs/dedup_books.go` (legacy, will be migrated).
> **Bot-task index:** `docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-*.md`.

---

## 1. Vision

Every audiobook entering the library is queued through one ordered pipeline. Each
stage produces **signals** (small, typed, scored facts) about the file. A decision
matrix fuses signals into:

- `identity_score ∈ [0,1]` — how confident we are that the file *is* what its
  tags claim.
- `match_groups` — clusters of files that are duplicates of one another, with a
  per-pair `match_score ∈ [0,1]` and an explanation of which signals contributed.

Two non-negotiable principles:

1. **Cheap signals before expensive signals.** SHA-256 short-circuits everything
   else. Whisper transcription only runs if cheaper signals are inconclusive.
2. **Forever-store.** Every fingerprint we ever compute is kept, even after the
   underlying file is deleted, so a re-import can be auto-resolved against
   history. *Storage is cheap, compute is not.*

---

## 2. Pipeline architecture

### 2.1 Stages (DAG)

```
              ┌─────────────────────────────────────────────────────────────┐
              │                       Book ingested                          │
              └─────────────────────────────────┬───────────────────────────┘
                                                │ enqueue per-file pipeline
                                                ▼
                  ┌─────────────────────────────────────────────────────────┐
                  │ STAGE 0 · forever-store lookup (sha256 prefix probe)     │
                  │   if exact hit → emit signal sha_exact, skip 1, 2, 3a    │
                  └────────────────────────────┬────────────────────────────┘
                                               │
       ┌───────────────────────────────────────┼───────────────────────────────────────┐
       ▼                                       ▼                                       ▼
┌──────────────┐                  ┌──────────────────────┐                ┌────────────────────────┐
│ STAGE 1      │                  │ STAGE 2              │                │ STAGE 3                │
│ sha256 full  │                  │ stream-content hash  │                │ chromaprint segments   │
│ (mandatory)  │                  │ (per audio stream)   │                │ (intro+5 body+outro)   │
└──────┬───────┘                  └──────────┬───────────┘                └────────────┬───────────┘
       │                                     │                                         │
       │                                     │                                         ▼
       │                                     │                              ┌────────────────────────┐
       │                                     │                              │ STAGE 3a · acoustid    │
       │                                     │                              │ lookup (uses stage 3)  │
       │                                     │                              └────────────┬───────────┘
       │                                     │                                           │
       └─────────────────┬───────────────────┴───────────────┬───────────────────────────┘
                         ▼                                   ▼
                ┌────────────────────┐            ┌────────────────────────┐
                │ STAGE 4 · tag      │            │ STAGE 5 · filename /   │
                │ normalized match   │            │ path heuristics        │
                └─────────┬──────────┘            └────────────┬───────────┘
                          ▼                                    │
                ┌────────────────────┐                         │
                │ STAGE 6 · embedding│                         │
                │ semantic match     │                         │
                └─────────┬──────────┘                         │
                          ▼                                    │
                ┌────────────────────────────────────┐         │
                │ STAGE 7 · first-2-min transcription│         │
                │  (whisper) only if identity < 0.85 │         │
                └─────────┬──────────────────────────┘         │
                          ▼                                    ▼
                ┌─────────────────────────────────────────────────┐
                │ STAGE 8 · DECISION MATRIX                        │
                │   - identity_score                               │
                │   - match_groups (joined via FingerprintStore)   │
                │   - persisted to identity_results +              │
                │     dedup_match_groups                           │
                └────────────┬────────────────────────────────────┘
                             ▼
                ┌─────────────────────────────────────────────────┐
                │ STAGE 9 · TRUST LADDER                           │
                │   < 0.50 manual · 0.50–0.75 confirm ·            │
                │   0.75–0.90 default-yes · ≥0.90 auto (opt-in)    │
                └─────────────────────────────────────────────────┘
```

Each stage is a self-registering `MaintenanceJob` (using
`internal/maintenance.MaintenanceJob`, see `internal/maintenance/job.go:55`)
plus a per-book sub-routine. The pipeline coordinator (Stage 8) is itself a job
that fans out to stages 1–7 in dependency order, gathers `Signal` rows from
`signal_store`, runs the matrix, writes results, then schedules Stage 9
(trust-ladder action) outside the worker pool so user prompts can be issued
through `realtime.EventHub`.

### 2.2 Signal taxonomy

A signal is the unit of evidence the matrix consumes. Every signal lives in
`signal_store` and has a stable shape:

```go
type Signal struct {
    ID         int64     // autoincrement
    BookID     string    // owning book (UUID/ULID)
    FileID     string    // book_files row, optional (whole-book signals leave empty)
    Kind       string    // see SignalKind constants below
    Value      string    // canonical opaque payload (e.g., the sha256 hex, fp segment)
    Score      float64   // [0,1] strength of THIS signal as evidence
    Confidence float64   // [0,1] how trustworthy this measurement is
    Source     string    // stage id ("sha256-full", "chromaprint-fpcalc", ...)
    EvidenceJSON []byte  // human-readable extras (matched title, hamming distance,
                         //   whisper transcript snippet, etc.)
    ComputedAt time.Time
    ExpiresAt  *time.Time // nullable, used by external lookups (e.g., AcoustID quotas)
}
```

`SignalKind` enum (string constants in `internal/dedup/signals/kind.go`):

| Kind                       | Stage    | Score semantics                          | Confidence semantics                |
| -------------------------- | -------- | ---------------------------------------- | ----------------------------------- |
| `sha_exact`                | 0,1      | always 1.0                               | 1.0                                 |
| `stream_content_hash`      | 2        | 1.0 on exact stream-hash match           | 1.0                                 |
| `chromaprint_segment`      | 3        | 1 − (hamming/maxBits)                    | 0.95                                |
| `chromaprint_full`         | 3        | 1 − (hamming/maxBits)                    | 0.95                                |
| `acoustid_match`           | 3a       | external API confidence                  | 0.85 (drops to 0.4 on stale lookup) |
| `tag_match`                | 4        | weighted of title/author/duration/tracks | 0.75                                |
| `filename_match`           | 5        | normalized Levenshtein / Jaccard         | 0.40                                |
| `embedding_similarity`     | 6        | cosine                                   | 0.65                                |
| `whisper_intro_match`      | 7        | fuzzy match score vs expected title      | 0.80                                |
| `whisper_intro_negative`   | 7        | inverse — title NOT mentioned in intro   | 0.30 (hint, never decisive)         |
| `forever_store_resurrect`  | 0        | 1.0                                      | 1.0                                 |

Score and Confidence are independent: a chromaprint match might have score 0.95
(near-identical bits) and confidence 0.95 (we trust the algorithm). A Whisper
miss is *informational* — confidence is intentionally low so it cannot veto
strong positive signals on its own.

---

## 3. Forever-store: `FingerprintStore`

### 3.1 Purpose

Record every fingerprint we ever computed. Never delete a row. When a book is
removed from the library, its `deleted_at` column is filled and a JSON
`deletion_history` event is appended. New imports always probe this store first.

### 3.2 Schema (SQLite — co-located with `embeddings.db` or its own
`fingerprints.db` — implementer's choice; spec assumes the latter for clean
separation)

```sql
CREATE TABLE fingerprint_files (
    sha256              TEXT PRIMARY KEY,         -- 64 hex chars
    size_bytes          INTEGER NOT NULL,
    container_format    TEXT,                     -- "m4b","mp3","flac","ogg",...
    audio_codec         TEXT,                     -- "aac","mp3","flac",...
    duration_seconds    REAL,
    stream_content_hash TEXT,                     -- nullable until computed
    chromaprint_full    TEXT,                     -- AcoustID fp string, nullable
    chromaprint_intro   TEXT,                     -- 5-min intro segment, nullable
    chromaprint_outro   TEXT,
    chromaprint_body    TEXT,                     -- JSON array of body segments
    acoustid_mbid       TEXT,                     -- musicbrainz recording ID
    acoustid_score      REAL,
    first_filename      TEXT NOT NULL,            -- basename when first seen
    first_path          TEXT NOT NULL,            -- absolute path when first seen
    first_seen_at       DATETIME NOT NULL,
    last_seen_at        DATETIME NOT NULL,
    deleted_at          DATETIME,                 -- NULL = currently in library
    deletion_history    TEXT NOT NULL DEFAULT '[]', -- JSON array of {ts,reason,user}
    schema_version      INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX idx_fp_chromaprint_full ON fingerprint_files(chromaprint_full);
CREATE INDEX idx_fp_chromaprint_intro ON fingerprint_files(chromaprint_intro);
CREATE INDEX idx_fp_acoustid ON fingerprint_files(acoustid_mbid);
CREATE INDEX idx_fp_deleted_at ON fingerprint_files(deleted_at);

CREATE TABLE fingerprint_aliases (
    sha256       TEXT NOT NULL REFERENCES fingerprint_files(sha256),
    seen_at      DATETIME NOT NULL,
    filename     TEXT NOT NULL,
    path         TEXT NOT NULL,
    book_id      TEXT,                            -- nullable; book this file currently belongs to
    PRIMARY KEY (sha256, seen_at, path)
);

CREATE TABLE fingerprint_match_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    incoming_sha256 TEXT NOT NULL,
    matched_sha256  TEXT NOT NULL,
    signal_kind     TEXT NOT NULL,                -- sha_exact, chromaprint_full, ...
    distance        REAL,                         -- if applicable
    matched_at      DATETIME NOT NULL,
    decision        TEXT                          -- "auto-merge","suggested","rejected"
);
```

### 3.3 Go interface

`internal/database/fingerprint_store.go`:

```go
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

The implementation is SQLite-backed. The forever-store is **separate** from the
operational `Store` interface so destructive maintenance jobs cannot touch it
by accident.

### 3.4 Failure modes

| Failure                          | Behavior                                                  |
| -------------------------------- | --------------------------------------------------------- |
| sha256 OOM on 50 GB file         | streaming hash with 4 MiB chunks (already in `internal/scanner/scanner.go:1761`); never load whole file in memory |
| chromaprint backend missing      | emit `chromaprint_*` signals with confidence 0; stage marks itself `unavailable` and the matrix degrades gracefully |
| AcoustID HTTP 429                | exponential backoff via existing `internal/ai/aijobs` job runner; signal stays `pending`; matrix proceeds without it |
| Whisper API down                 | stage skipped; identity_score capped at 0.85 to surface "needs verification" in UI |
| fingerprints.db corruption       | nightly `VACUUM INTO` snapshot kept in `data/fingerprints/backup/`; `Available()` returns false on corruption and the matrix runs in "tags-only" mode rather than crashing imports |

---

## 4. Decision matrix

### 4.1 identity_score

```
identity_score(book) =
    σ( Σ_signals  score_i · confidence_i · weight[kind_i] )
```

where `σ` is a soft clamp (`min(1.0, max(0.0, x))`) — *not* a logistic; we want
SHA + chromaprint to saturate at 1.0 fast, not asymptote.

Weights (justifications below):

| Kind                        | Weight |
| --------------------------- | -----: |
| `sha_exact`                 |   1.00 |
| `forever_store_resurrect`   |   1.00 |
| `stream_content_hash`       |   0.90 |
| `chromaprint_full`          |   0.70 |
| `chromaprint_segment`       |   0.50 |
| `acoustid_match`            |   0.55 |
| `whisper_intro_match`       |   0.45 |
| `tag_match`                 |   0.30 |
| `embedding_similarity`      |   0.25 |
| `filename_match`            |   0.15 |
| `whisper_intro_negative`    |  −0.20 |

**Justifications (cite when reviewers ask):**

- SHA-256 byte-equality → identity is mathematical, not statistical.
- Chromaprint full-file is content-hashed audio: collisions across distinct
  recordings are vanishingly rare (AcoustID matches >100 M tracks against a
  ~30 M-record DB with negligible collision noise; see Lukáš Lalinský's
  Chromaprint paper, 2011).
- AcoustID weight is intentionally lower than chromaprint_full because
  AcoustID's external mapping can be polluted (multiple recordings under one MBID).
- Whisper is *high signal but moderate weight* — the file says "Chapter One of
  *Pride and Prejudice* by Jane Austen" → very strong identity for the book,
  but tells us nothing about narrator/edition/abridgement, which dedup needs.
  Weighted lower than chromaprint to reflect this ceiling.
- Tag/embedding/filename are all metadata-derived and can be wrong (rips
  mislabel constantly); they *contribute* but never *decide*.
- The negative whisper signal is small and capped — a missed title in the
  first two minutes happens for legitimate reasons (long musical preface,
  prologue narration).

### 4.2 match_score(A, B)

For any pair of books, a per-pair score:

```
match_score(A,B) = max(
    sha_eq(A,B),
    stream_eq(A,B),
    chromaprint_sim(A,B) · 0.95,
    acoustid_eq(A,B) · 0.85,
    embedding_sim(A,B) · 0.75,
    tag_match(A,B) · 0.60,
    filename_sim(A,B) · 0.30,
)
```

`max` not sum — one strong signal is enough to declare a match. Weak signals
*group* candidates but cannot promote them past the trust-ladder thresholds.

Match groups are persisted to `dedup_match_groups` (see §6) keyed by the
strongest signal so the UI can explain the decision.

### 4.3 Trust ladder

| Range       | Action                                            | UI                                           |
| ----------- | ------------------------------------------------- | -------------------------------------------- |
| `< 0.50`    | Manual only — no suggestions emitted              | Hidden behind "Show low-confidence" toggle    |
| `0.50–0.75` | Suggest, requires explicit confirm                | "Maybe a duplicate" card, default action = ignore |
| `0.75–0.90` | Suggest, default-yes confirm                      | "Likely a duplicate" card, default action = merge |
| `≥ 0.90`    | Auto-merge if admin opt-in is enabled             | Notification only; reversible via match log |

A global kill switch (`settings.dedup.auto_merge_enabled`) defaults to `false`.
Per-action reversal is always available via the forever-store (`MarkDeleted`
appends to `deletion_history`; `Resurrect` undoes).

---

## 5. Pipeline control plane

The pipeline coordinator is a single self-registering job:
`identification-pipeline` in `internal/maintenance/jobs/identification_pipeline.go`.

Inputs: `book_id` (single-book mode) or `since` (incremental mode).

For each book, the coordinator:

1. Builds a per-stage execution plan, skipping stages already completed for
   the current `signal_revision` (column on `books` row, bumped when tags are
   re-fetched).
2. Spawns sub-jobs (one per stage, prefixed `pipe-stage-{kind}`) using
   `internal/operations.Queue`. Long-running stages (Whisper) are dispatched
   with `PriorityLow`; cheap ones (`sha256-full`) with `PriorityHigh`.
3. Writes signals to `signal_store` as they arrive.
4. When all required stages finish (or a strict timeout fires), runs the
   decision matrix, writes `identity_results` row, computes match groups,
   updates `dedup_match_groups`.
5. Schedules the trust-ladder action.

Every stage uses the existing checkpoint mechanism
(`operations.SaveCheckpoint`) so the pipeline can resume after restart without
recomputing fingerprints.

### 5.1 Backpressure & quotas

- Concurrency caps per stage (configurable in `config.yaml`):
  - `sha256-full`: workers = NumCPU
  - `chromaprint`: workers = max(1, NumCPU/2) (CPU-bound via fpcalc)
  - `whisper`: workers = 2 (rate-limited by API)
  - `acoustid`: workers = 1 (external API, courteous default)
- Pipeline emits Prometheus-friendly metrics via `internal/metrics`:
  `pipeline_stage_duration_seconds{stage}`, `pipeline_signal_total{kind}`,
  `pipeline_identity_score_bucket`, `pipeline_match_group_size_bucket`.

---

## 6. Match groups

`dedup_match_groups` (in main SQLite database):

```sql
CREATE TABLE dedup_match_groups (
    id              TEXT PRIMARY KEY,             -- ULID
    canonical_book  TEXT NOT NULL,                -- chosen "winner"
    strongest_kind  TEXT NOT NULL,                -- e.g. "sha_exact"
    strongest_score REAL NOT NULL,
    signal_summary  TEXT NOT NULL,                -- JSON: per-kind contributions
    state           TEXT NOT NULL,                -- "open","merged","dismissed","split"
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    decided_by      TEXT,                         -- user id or "auto"
    decided_at      DATETIME
);
CREATE TABLE dedup_match_group_members (
    group_id   TEXT NOT NULL REFERENCES dedup_match_groups(id) ON DELETE CASCADE,
    book_id    TEXT NOT NULL,
    pair_score REAL NOT NULL,
    role       TEXT NOT NULL,                     -- "canonical" or "duplicate"
    PRIMARY KEY (group_id, book_id)
);
CREATE INDEX idx_match_groups_state ON dedup_match_groups(state);
CREATE INDEX idx_match_group_members_book ON dedup_match_group_members(book_id);
```

Groups are recomputed incrementally: when a book gains a new signal, the
coordinator re-runs `match_score` against any peer that shares at least one
forever-store fingerprint, and updates the affected groups. Groups never delete
— they transition through states.

---

## 7. HTTP API

All routes mounted under `/api/v1/identification/` (new) and
`/api/v1/dedup/v2/` (new) so the legacy `/api/v1/dedup/*` surface
(`internal/server/dedup_handlers.go`) keeps working through Phase 6.

| Method | Path                                                            | Purpose                                       |
| ------ | --------------------------------------------------------------- | --------------------------------------------- |
| GET    | `/api/v1/identification/books/:id`                              | Pipeline status + signals + identity_score    |
| GET    | `/api/v1/identification/books/:id/signals`                      | Full signal list                              |
| POST   | `/api/v1/identification/books/:id/recompute`                    | Re-run pipeline (body: `{stages:[…]}`)        |
| POST   | `/api/v1/identification/books/:id/recompute/:stage`             | Re-run single stage                           |
| GET    | `/api/v1/identification/fingerprints/sha/:sha`                  | Forever-store record + history                |
| GET    | `/api/v1/identification/fingerprints/chromaprint?fp=…&min=0.85` | Lookup by chromaprint                         |
| GET    | `/api/v1/dedup/v2/match-groups?state=open&limit=…&cursor=…`     | Paged list of match groups                    |
| GET    | `/api/v1/dedup/v2/match-groups/:id`                             | Group detail (members, signals, suggestion)   |
| POST   | `/api/v1/dedup/v2/match-groups/:id/resolve`                     | `{action:"merge"|"dismiss"|"split", canonical_book_id, members:[…]}` |
| GET    | `/api/v1/dedup/v2/stats`                                        | Counts by state, signal coverage              |
| POST   | `/api/v1/dedup/v2/recompute`                                    | Background pipeline run (body: filter)        |

All write endpoints require the `dedup.manage` permission (added to
`internal/auth/permissions.go`).

---

## 8. UI surface

A new top-level "Identification" tab. Three panels:

1. **Library health** — pie of identity-score buckets, table of stages with
   per-stage availability + queue depth, "recompute all" button (requires
   confirmation, dry-run by default).
2. **Match groups** — table of open groups, filters by `strongest_kind` and
   score range, expandable rows showing per-pair signal breakdown and a
   one-click resolve dropdown (merge, dismiss, split).
3. **Per-book drawer** — opens from any book card. Shows the timeline of stage
   executions, each signal as a chip with its score/confidence/weight
   contribution to identity_score, and a "Recompute…" menu.

Data plumbing reuses `useAsyncAction` (see
`docs/superpowers/specs/2026-04-30-frontend-cleanup.md`) and the existing
realtime SSE hub for live stage updates.

---

## 9. Migration plan (Phase 6)

The existing `dedup-books` job (`internal/maintenance/jobs/dedup_books.go`) and
the embedding dedup engine (`internal/dedup/engine.go`) keep running through
Phase 5. Phase 6 performs:

1. **Backfill** — for every existing book, enqueue the new pipeline (idempotent
   via the `signal_revision` column).
2. **Translate** existing `dedup_candidates` rows into `dedup_match_groups` by
   inferring `strongest_kind = "embedding_similarity"` and stamping
   `signal_summary` accordingly. Rows with state `merged`/`dismissed` carry
   forward; `pending` rows are re-evaluated by the new matrix.
3. **Decommission** — once Phase 6 backfill reports ≥ 99 % coverage, the
   legacy `dedup-books` job is rewritten to be a thin shim that calls the new
   coordinator and emits a deprecation warning. Two release cycles later, it
   is deleted (separate bot-task, not in this spec).
4. **Routes** — `/api/v1/dedup/*` returns `Deprecation` and `Sunset` headers
   pointing at `/api/v1/dedup/v2/*` for one minor release, then redirects.

No data is destroyed during migration; the forever-store ensures every
fingerprint computed by the legacy pipeline is recorded.

---

## 10. Testing strategy

Per stage:

- **Unit**: deterministic input → expected `Signal` output. Use small fixtures
  in `testdata/identification/`.
- **Property**: signal scores respect `[0,1]` bounds; SHA equality implies
  full identity; whisper negative never inverts a SHA positive.
- **Integration**: end-to-end pipeline against a tiny library
  (3 fixture audiobooks, 2 of which are byte-identical and 1 a re-encode).
  Asserts (a) auto-merge fires on the byte-identical pair, (b) the re-encode
  is in the same match group with `chromaprint_full` strongest, (c) Whisper
  stage is *not* invoked for any of them (identity_score already saturates).
- **Forever-store**: delete a fixture, re-import it, assert the resurrection
  signal fires and `deletion_history` is appended.
- **Load**: 10k synthetic fingerprint records, assert `LookupBySHA` p95 < 5 ms
  and `LookupByChromaprintFull` p95 < 50 ms.

CI gate (extends `make ci`): pipeline integration test runs on every PR
labelled `pipeline:` (label introduced in Phase 0 bot-task).

---

## 11. Bot-task DAG

```
Phase 0 — schema + store
  P0-01 fingerprint-schema-and-migrations
  P0-02 fingerprint-store-iface
  P0-03 fingerprint-store-sqlite-impl
  P0-04 signal-store-schema-and-impl
  P0-05 identity-results-schema
  P0-06 match-groups-schema

Phase 1 — signals (each independent after P0-04)
  P1-01 stage-sha256-full
  P1-02 stage-stream-content-hash
  P1-03 stage-chromaprint-segments
  P1-04 stage-acoustid-lookup     (depends P1-03)
  P1-05 stage-tag-match
  P1-06 stage-filename-match
  P1-07 stage-embedding-similarity
  P1-08 stage-whisper-intro

Phase 2 — fusion
  P2-01 decision-matrix-engine     (depends P0-05, all P1)
  P2-02 match-group-builder        (depends P0-06, P2-01)

Phase 3 — orchestration
  P3-01 pipeline-coordinator-job   (depends P2-01, P2-02)
  P3-02 backpressure-and-metrics   (depends P3-01)

Phase 4 — HTTP
  P4-01 identification-endpoints   (depends P3-01)
  P4-02 match-groups-v2-endpoints  (depends P2-02)

Phase 5 — UI
  P5-01 identification-tab-shell   (depends P4-01, P4-02)
  P5-02 per-book-drawer            (depends P5-01)
  P5-03 match-groups-table         (depends P5-01)

Phase 6 — migration
  P6-01 backfill-existing-library   (depends P3-01)
  P6-02 translate-dedup-candidates  (depends P2-02)
  P6-03 deprecate-legacy-routes     (depends P4-02, P5-*)

Phase 7 — auto-merge
  P7-01 trust-ladder-runner         (depends P3-01, P4-01)
  P7-02 admin-opt-in-toggle         (depends P7-01)
```

Independent tasks within a phase can be executed in parallel by a
dependency-aware bot runner. Each bot-task file restates its prereqs as a
`gh pr list --label task:PIPE-Pn-NN --state merged | jq length` check.

---

## 12. File-by-file change inventory (cross-reference)

| File                                                            | Change kind | Phase    |
| --------------------------------------------------------------- | ----------- | -------- |
| `internal/database/migrations.go`                               | edit        | P0-01,P0-04,P0-05,P0-06 |
| `internal/database/fingerprint_store.go`                        | create      | P0-02    |
| `internal/database/fingerprint_store_sqlite.go`                 | create      | P0-03    |
| `internal/database/fingerprint_store_test.go`                   | create      | P0-03    |
| `internal/dedup/signals/kind.go`                                | create      | P0-04    |
| `internal/dedup/signals/store.go`                               | create      | P0-04    |
| `internal/dedup/signals/store_test.go`                          | create      | P0-04    |
| `internal/maintenance/jobs/stage_*.go` (one per signal)         | create      | P1-*     |
| `internal/dedup/matrix/matrix.go`                               | create      | P2-01    |
| `internal/dedup/matrix/matrix_test.go`                          | create      | P2-01    |
| `internal/dedup/matrix/groups.go`                               | create      | P2-02    |
| `internal/maintenance/jobs/identification_pipeline.go`          | create      | P3-01    |
| `internal/server/identification_handlers.go`                    | create      | P4-01    |
| `internal/server/dedup_v2_handlers.go`                          | create      | P4-02    |
| `web/src/pages/Identification/*.tsx`                            | create      | P5-*     |
| `internal/maintenance/jobs/dedup_books.go`                      | edit→shim   | P6-03    |
| `internal/server/dedup_handlers.go`                             | edit (deprecation headers) | P6-03 |

Line numbers cited throughout this spec reflect HEAD of branch
`itunes-safety-rails` at the time of writing; bot-tasks include `grep` probes
so they remain valid as the file evolves.
