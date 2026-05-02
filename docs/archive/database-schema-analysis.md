<!-- file: docs/database-schema-analysis.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890 -->

# Database Schema Analysis

**Project:** audiobook-organizer  
**Date:** 2026-04-04  
**Scope:** All persistent stores — main library, sidecars, KV stores  
**Status:** Reference document — update when migrations run

---

## Table of Contents

1. [Overview: All Databases](#1-overview-all-databases)
2. [Database 1 — Main Library Store (PebbleDB or SQLite)](#2-database-1--main-library-store)
3. [Database 2 — Activity Log Sidecar (SQLite)](#3-database-2--activity-log-sidecar)
4. [Database 3 — AI Scan Store (PebbleDB)](#4-database-3--ai-scan-store)
5. [Database 4 — Open Library Dump Store (PebbleDB)](#5-database-4--open-library-dump-store)
6. [Cross-Database Redundancy Analysis](#6-cross-database-redundancy-analysis)
7. [Dead Tables and Dead Columns](#7-dead-tables-and-dead-columns)
8. [Missing Indexes](#8-missing-indexes)
9. [Prioritized Action Items](#9-prioritized-action-items)
10. [Migration Plan](#10-migration-plan)

---

## 1. Overview: All Databases

| # | Name | Engine | Default Path | Opened By |
|---|------|--------|-------------|-----------|
| 1 | Main Library Store | PebbleDB (default) or SQLite (opt-in) | `config.DatabasePath` (e.g. `/var/lib/audiobook-organizer/audiobooks.pebble`) | `database.InitializeStore()` at server startup |
| 2 | Activity Log Sidecar | SQLite | `{dir(DatabasePath)}/activity.db` | `database.NewActivityStore()` at server startup |
| 3 | AI Scan Store | PebbleDB | `{dir(DatabasePath)}/ai_scans.db` | `database.NewAIScanStore()` at server startup |
| 4 | Open Library Dump Store | PebbleDB | `{OpenLibraryDumpDir}/oldb` (default: `{RootDir}/openlibrary-dumps/oldb`) | `openlibrary.NewOLStore()` on demand / auto-detect |

All four databases live in or near the same directory. There is no cross-database foreign key enforcement — referential integrity between databases is maintained in application code only.

**Production facts (from MEMORY.md):**
- Main library: PebbleDB at `/var/lib/audiobook-organizer/audiobooks.pebble`
- Activity log: `/var/lib/audiobook-organizer/activity.db`
- AI scans: `/var/lib/audiobook-organizer/ai_scans.db`
- OL dump store: `{RootDir}/openlibrary-dumps/oldb`
- Library stats: 10,891 books / 2,970 authors / 8,507 series
- External ID map: 97,000+ mappings

---

## 2. Database 1 — Main Library Store

**Engine:** PebbleDB (production) or SQLite (developer/test opt-in, enabled with `--enable-sqlite3-i-know-the-risks`)  
**Opened at:** `internal/database/store.go:InitializeStore()`  
**Implementation files:**
- `internal/database/pebble_store.go` — PebbleDB implementation
- `internal/database/sqlite_store.go` — SQLite implementation (1,100+ lines)
- `internal/database/migrations.go` — 42 migrations for SQLite; PebbleDB is schema-free
- `internal/database/database.go` — Legacy global `DB` variable, `Initialize()` (pre-Store era)
- `internal/database/web.go` — Legacy web helpers reading from global `DB`
- `internal/database/settings.go` — Settings CRUD for both backends
- `internal/database/store.go` — `Store` interface definition + `GlobalStore`

**Current migration level:** 42

### 2.1 SQLite Schema (all tables)

The SQLite schema is the canonical reference because it is fully declarative. PebbleDB uses equivalent key-value namespaces documented in the `PebbleStore` comment header.

---

#### Table: `authors`

```sql
CREATE TABLE authors (
    id    INTEGER PRIMARY KEY AUTOINCREMENT,
    name  TEXT NOT NULL UNIQUE
);
CREATE INDEX idx_authors_name ON authors(name);
```

**Added by:** migration 1 (initial schema)  
**Reads:** `GetAllAuthors`, `GetAuthorByID`, `GetAuthorByName`, all book queries that JOIN authors  
**Writes:** `CreateAuthor`, `UpdateAuthorName`, migration 22 (backfill split "&"-joined names)  
**Notes:** `name` has a UNIQUE constraint. For multi-author books the join table `book_authors` is used; `authors.name` should contain only canonical single-author names. There is no `wanted` column in the canonical `createTables` schema — it was added by migration 13 via `ALTER TABLE`. Check your production schema.

---

#### Table: `author_aliases`

```sql
CREATE TABLE author_aliases (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    author_id   INTEGER NOT NULL REFERENCES authors(id) ON DELETE CASCADE,
    alias_name  TEXT NOT NULL,
    alias_type  TEXT NOT NULL DEFAULT 'alias',
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_author_aliases_author ON author_aliases(author_id);
CREATE INDEX idx_author_aliases_name   ON author_aliases(alias_name);
CREATE UNIQUE INDEX idx_author_aliases_unique ON author_aliases(author_id, alias_name);
```

**Added by:** migration 1 (initial createTables schema)  
**Reads:** `GetAuthorAliases`, `GetAllAuthorAliases`, `FindAuthorByAlias`  
**Writes:** `CreateAuthorAlias`, `DeleteAuthorAlias`  
**Notes:** Used to resolve alternate author name spellings during scanning. `alias_type` values in practice: `"alias"`, `"pen_name"`.

---

#### Table: `series`

```sql
CREATE TABLE series (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    author_id  INTEGER,
    FOREIGN KEY (author_id) REFERENCES authors(id),
    UNIQUE(name, author_id)
);
CREATE INDEX idx_series_name   ON series(name);
CREATE INDEX idx_series_author ON series(author_id);
```

**Added by:** migration 1  
**Reads:** `GetAllSeries`, `GetSeriesByID`, `GetSeriesByName`  
**Writes:** `CreateSeries`, `UpdateSeriesName`, `DeleteSeries`  
**Notes:** SQLite UNIQUE does not catch `NULL = NULL`, so multiple series with the same name and `author_id IS NULL` can accumulate. `NewSQLiteStore` calls `deduplicateSeries()` on every open to handle this. Migration 13 added `wanted BOOLEAN DEFAULT 0` via ALTER TABLE.

---

#### Table: `works`

```sql
CREATE TABLE works (
    id         TEXT PRIMARY KEY,   -- ULID
    title      TEXT NOT NULL,
    author_id  INTEGER,
    series_id  INTEGER,
    alt_titles TEXT,               -- JSON array of alternate titles
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME
);
CREATE INDEX idx_works_title ON works(title);
```

**Added by:** createTables (initial schema, not a numbered migration)  
**Reads:** `GetAllWorks`, `GetWorkByID`, `GetBooksByWorkID`  
**Writes:** `CreateWork`, `UpdateWork`, `DeleteWork`  
**Notes:** "Works" are logical groupings across editions/narrations (the same story, different recordings). The `id` is a ULID string. `alt_titles` is stored as a raw JSON string, not normalized. The `series_id` FK references `series(id)` but there is no `ON DELETE` action defined here.

---

#### Table: `books` (core entity — 58 columns)

```sql
CREATE TABLE books (
    -- Identity
    id                         TEXT PRIMARY KEY,        -- ULID
    title                      TEXT NOT NULL,
    file_path                  TEXT NOT NULL UNIQUE,
    original_filename          TEXT,
    format                     TEXT,                    -- "mp3", "m4b", "flac", etc.

    -- Relationships
    author_id                  INTEGER,                 -- FK authors(id), denormalized (primary)
    series_id                  INTEGER,                 -- FK series(id)
    series_sequence            INTEGER,
    work_id                    TEXT,                    -- FK works(id) (logical title)

    -- Bibliographic metadata
    narrator                   TEXT,                    -- legacy single-value, superseded by book_narrators
    edition                    TEXT,
    description                TEXT,
    language                   TEXT,
    publisher                  TEXT,
    genre                      TEXT,                    -- added migration 36
    print_year                 INTEGER,
    audiobook_release_year     INTEGER,
    isbn10                     TEXT,
    isbn13                     TEXT,
    asin                       TEXT,                    -- added migration 25
    open_library_id            TEXT,                    -- added migration 28
    hardcover_id               TEXT,                    -- added migration 28
    google_books_id            TEXT,                    -- added migration 28
    cover_url                  TEXT,                    -- added migration 15

    -- iTunes integration
    itunes_persistent_id       TEXT,                    -- added migration 11
    itunes_date_added          TIMESTAMP,
    itunes_play_count          INTEGER DEFAULT 0,
    itunes_last_played         TIMESTAMP,
    itunes_rating              INTEGER,
    itunes_bookmark            INTEGER,
    itunes_import_source       TEXT,
    itunes_path                TEXT,                    -- added migration 38
    itunes_sync_status         TEXT,                    -- added migration 41 ("synced","dirty",NULL)

    -- File/media info
    file_hash                  TEXT,
    file_size                  INTEGER,
    bitrate_kbps               INTEGER,                 -- added migration 5
    codec                      TEXT,
    sample_rate_hz             INTEGER,
    channels                   INTEGER,
    bit_depth                  INTEGER,
    quality                    TEXT,
    original_file_hash         TEXT,                    -- added migration 6
    organized_file_hash        TEXT,

    -- Version management
    is_primary_version         BOOLEAN DEFAULT 1,
    version_group_id           TEXT,
    version_notes              TEXT,
    narrators_json             TEXT,                    -- added migration 15, JSON array

    -- State machine
    library_state              TEXT DEFAULT 'imported', -- 'imported','organized','wanted','needs_review','deleted'
    quantity                   INTEGER DEFAULT 1,
    marked_for_deletion        BOOLEAN DEFAULT 0,
    marked_for_deletion_at     DATETIME,

    -- Scan cache
    last_scan_mtime            INTEGER DEFAULT NULL,    -- added migration 32, unix mtime
    last_scan_size             INTEGER DEFAULT NULL,
    needs_rescan               BOOLEAN DEFAULT 0,

    -- Timestamps
    created_at                 DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                 DATETIME,
    metadata_updated_at        DATETIME,                -- added migration 23
    last_written_at            DATETIME,

    -- Review / organize tracking
    metadata_review_status     TEXT,                    -- added migration 24
    last_organize_operation_id TEXT,                    -- added migration 40
    last_organized_at          DATETIME,

    FOREIGN KEY (author_id) REFERENCES authors(id),
    FOREIGN KEY (series_id) REFERENCES series(id)
);
```

**Indexes on `books`:**
```
idx_books_title                ON books(title)
idx_books_author               ON books(author_id)
idx_books_series               ON books(series_id)
idx_books_file_path            ON books(file_path)
idx_books_file_hash            ON books(file_hash)
idx_books_itunes_persistent_id ON books(itunes_persistent_id)
idx_books_original_hash        ON books(original_file_hash)
idx_books_organized_hash       ON books(organized_file_hash)
idx_books_library_state        ON books(library_state)
idx_books_marked_for_deletion  ON books(marked_for_deletion)
idx_books_version_group        ON books(version_group_id)              [migration 5]
idx_books_is_primary           ON books(is_primary_version)            [migration 5]
idx_books_notdeleted_title     ON books(COALESCE(marked_for_deletion,0), title) [migration 17]
idx_books_created_at           ON books(created_at)                    [migration 17]
idx_books_author_title         ON books(author_id, title)              [migration 17]
idx_books_scan_cache           ON books(file_path, last_scan_mtime, last_scan_size) [migration 32]
idx_books_needs_rescan         ON books(needs_rescan) WHERE needs_rescan = 1 [migration 32]
idx_books_itunes_sync_status   ON books(itunes_sync_status) WHERE itunes_sync_status IS NOT NULL [migration 42]
idx_books_dirty_primary        ON books(itunes_sync_status, is_primary_version) WHERE itunes_sync_status = 'dirty' [migration 42]
```

**FTS5 virtual table (migration 17):**
```sql
CREATE VIRTUAL TABLE books_fts USING fts5(title, content=books, content_rowid=rowid);
-- With insert/update/delete triggers to keep index in sync
```

**Reads:** Nearly every handler. Key patterns:
- `GetAllBooks(limit, offset)` — paginated scan
- `GetBookByID(ulid)` — single book fetch
- `GetBookByFilePath(path)` — scanner deduplication
- `GetBookByITunesPersistentID(pid)` — iTunes linking
- `GetBookByFileHash / GetBookByOriginalHash / GetBookByOrganizedHash` — dedup
- `GetDuplicateBooks`, `GetFolderDuplicates`, `GetDuplicateBooksByMetadata`
- Filtered list queries: by author, series, state, tag, FTS

**Writes:**
- `CreateBook` — scanner, import
- `UpdateBook` — full column replacement (all 58 columns written every update)
- `DeleteBook` (moves to `book_tombstones`)
- Migration backfills

---

#### Table: `book_authors` (junction)

```sql
CREATE TABLE book_authors (
    book_id   TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    author_id INTEGER NOT NULL REFERENCES authors(id),
    role      TEXT NOT NULL DEFAULT 'author',    -- 'author', 'co-author'
    position  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (book_id, author_id)
);
CREATE INDEX idx_book_authors_book   ON book_authors(book_id);
CREATE INDEX idx_book_authors_author ON book_authors(author_id);
```

**Added by:** migration 15  
**Reads:** `GetBookAuthors`, `GetBooksByAuthorIDWithRole`  
**Writes:** `SetBookAuthors`, migration 22 (backfill from `books.author_id`)  
**Notes:** Supersedes the denormalized `books.author_id` for multi-author books. `books.author_id` is kept for backward-compat and is always the primary author.

---

#### Table: `narrators`

```sql
CREATE TABLE narrators (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_narrators_name ON narrators(name);
```

**Added by:** migration 20  
**Reads:** `GetNarratorByID`, `GetNarratorByName`, `ListNarrators`  
**Writes:** `CreateNarrator`

---

#### Table: `book_narrators` (junction)

```sql
CREATE TABLE book_narrators (
    book_id     TEXT NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    narrator_id INTEGER NOT NULL REFERENCES narrators(id),
    role        TEXT NOT NULL DEFAULT 'narrator',  -- 'narrator', 'co-narrator'
    position    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (book_id, narrator_id)
);
CREATE INDEX idx_book_narrators_book     ON book_narrators(book_id);
CREATE INDEX idx_book_narrators_narrator ON book_narrators(narrator_id);
```

**Added by:** migration 20  
**Reads:** `GetBookNarrators`  
**Writes:** `SetBookNarrators`, migration 22 (backfill from `books.narrator`)  
**Notes:** Supersedes `books.narrator` (legacy text column kept for compat).

---

#### Table: `book_files`

```sql
CREATE TABLE book_files (
    id                   TEXT PRIMARY KEY,       -- ULID
    book_id              TEXT NOT NULL,          -- FK books(id) ON DELETE CASCADE
    file_path            TEXT NOT NULL,
    original_filename    TEXT,
    itunes_path          TEXT,
    itunes_persistent_id TEXT,
    track_number         INTEGER,
    track_count          INTEGER,
    disc_number          INTEGER,
    disc_count           INTEGER,
    title                TEXT,
    format               TEXT,
    codec                TEXT,
    duration             INTEGER,                -- milliseconds
    file_size            INTEGER,
    bitrate_kbps         INTEGER,
    sample_rate_hz       INTEGER,
    channels             INTEGER,
    bit_depth            INTEGER,
    file_hash            TEXT,
    original_file_hash   TEXT,
    missing              INTEGER NOT NULL DEFAULT 0,   -- 1 = file not found on disk
    created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE
);
CREATE INDEX idx_book_files_book_id   ON book_files(book_id);
CREATE INDEX idx_book_files_itunes_pid ON book_files(itunes_persistent_id) WHERE itunes_persistent_id IS NOT NULL;
CREATE INDEX idx_book_files_file_hash  ON book_files(file_hash) WHERE file_hash IS NOT NULL;
CREATE INDEX idx_book_files_file_path  ON book_files(file_path);
CREATE INDEX idx_book_files_book_active ON book_files(book_id, missing);  -- migration 42
```

**Added by:** migration 39 (created table + migrated from `book_segments`)  
**Reads:** book detail panel "Files & History" tab, organize operations, iTunes linking  
**Writes:** scanner (creates/updates file records), organize (updates `file_path`), tag-write (updates `file_hash`)  
**Notes:** `duration` is milliseconds here (unlike `book_segments` which used seconds). `book_id` is a proper ULID string, unlike `book_segments.book_id` which was a CRC32 numeric hash of the ULID — this was the primary reason for migration 39.

---

#### Table: `book_segments` (DEPRECATED)

```sql
CREATE TABLE book_segments (
    id               TEXT PRIMARY KEY,
    book_id          INTEGER NOT NULL,    -- WRONG: CRC32 hash of ULID, not a real FK
    file_path        TEXT NOT NULL,
    format           TEXT NOT NULL DEFAULT '',
    size_bytes       INTEGER NOT NULL DEFAULT 0,
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    track_number     INTEGER,
    total_tracks     INTEGER,
    active           INTEGER NOT NULL DEFAULT 1,
    superseded_by    TEXT,
    file_hash        TEXT,               -- added migration 30
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    version          INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX idx_book_segments_book ON book_segments(book_id);
CREATE INDEX idx_book_segments_hash ON book_segments(file_hash);
```

**Added by:** migration 16  
**Status:** DEPRECATED — superseded by `book_files`. Migration 39 migrated data out. Migration 42 comment says "Don't drop book_segments yet — deprecate interface first, drop in a future migration." Plan to drop in migration 43.  
**Notes:** The `book_id` column contains a CRC32 numeric hash of the book ULID (not an integer primary key and not a real FK), which was a design error. All new code should use `book_files`.

---

#### Table: `book_tags`

```sql
CREATE TABLE book_tags (
    book_id    TEXT NOT NULL,
    tag        TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (book_id, tag)
);
CREATE INDEX idx_book_tags_tag ON book_tags(tag);
```

**Added by:** migration 37  
**Reads:** `GetBookTags`, `GetBookUserTags`, filtered list queries  
**Writes:** `AddBookTag`, `AddBookUserTag`, `RemoveBookTag`, `DeleteAllBookTags`  
**Notes:** User-defined tags for custom categorization. Part of the library-enhancement PR (#189/#190). The index `idx_book_tags_tag` supports tag-based filtering across the whole library.

---

#### Table: `book_tombstones`

```sql
CREATE TABLE book_tombstones (
    id         TEXT PRIMARY KEY,           -- same ULID as books.id
    data       TEXT NOT NULL,              -- JSON snapshot of deleted book
    created_at DATETIME DEFAULT (datetime('now'))
);
```

**Added by:** migration 26  
**Reads:** `GetBookTombstone`, restore-deleted-book operations  
**Writes:** `DeleteBook` (moves book here before deleting from `books`)  
**Notes:** Soft-delete pattern. `data` holds the full serialized `Book` struct so the record can be fully restored. No TTL cleanup currently implemented.

---

#### Table: `book_path_history`

```sql
CREATE TABLE book_path_history (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id     TEXT NOT NULL,
    old_path    TEXT NOT NULL,
    new_path    TEXT NOT NULL,
    change_type TEXT NOT NULL DEFAULT 'rename',   -- 'rename', 'organize', 'move'
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_path_history_book      ON book_path_history(book_id);
CREATE INDEX idx_path_history_book_time ON book_path_history(book_id, created_at DESC);  -- migration 42
```

**Added by:** migration 35  
**Reads:** "Files & History" tab, diagnostic reports  
**Writes:** organizer (every rename/move records an entry)  
**Notes:** No TTL. Could grow unbounded for active libraries.

---

#### Table: `external_id_map`

```sql
CREATE TABLE external_id_map (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    source      TEXT NOT NULL,          -- 'itunes', 'audible', etc.
    external_id TEXT NOT NULL,          -- persistent ID from external system
    book_id     TEXT NOT NULL,          -- ULID of local book
    track_number INTEGER,
    file_path   TEXT,
    tombstoned  INTEGER DEFAULT 0,      -- 1 = mapping is no longer valid
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX idx_ext_id_source_eid ON external_id_map(source, external_id);
CREATE INDEX idx_ext_id_book             ON external_id_map(book_id);
CREATE INDEX idx_ext_id_tombstone        ON external_id_map(source, tombstoned) WHERE tombstoned = 0;
CREATE INDEX idx_ext_id_book_source      ON external_id_map(book_id, source);  -- migration 42
```

**Added by:** migration 34  
**Production size:** 97,000+ rows  
**Reads:** iTunes sync, Audible linking, any external ID lookup  
**Writes:** iTunes importer, backfill scripts  
**Notes:** The primary mechanism for linking iTunes PIDs to book ULIDs. Also used for Audible ASINs. `tombstoned = 1` preserves history when a mapping is invalidated without deleting it.

---

#### Table: `deferred_itunes_updates`

```sql
CREATE TABLE deferred_itunes_updates (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id       TEXT NOT NULL,
    persistent_id TEXT NOT NULL,
    old_path      TEXT NOT NULL,
    new_path      TEXT NOT NULL,
    update_type   TEXT NOT NULL DEFAULT 'transcode',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    applied_at    DATETIME
);
CREATE INDEX idx_deferred_itunes_pending ON deferred_itunes_updates(applied_at) WHERE applied_at IS NULL;
```

**Added by:** migration 33  
**Reads:** `GetPendingDeferredITunesUpdates` — consumed by iTunes sync pass  
**Writes:** transcode pipeline (creates records when output path differs from iTunes path)  
**Notes:** Queue pattern. Rows stay after `applied_at` is set — they are records, not consumed. No TTL cleanup.

---

#### Table: `metadata_states`

```sql
CREATE TABLE metadata_states (
    book_id         TEXT NOT NULL,
    field           TEXT NOT NULL,
    fetched_value   TEXT,           -- value from external source (Open Library, etc.)
    override_value  TEXT,           -- user-set override
    override_locked BOOLEAN NOT NULL DEFAULT 0,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (book_id, field)
);
CREATE INDEX idx_metadata_states_book ON metadata_states(book_id);
```

**Added by:** migration 10  
**Reads:** `GetMetadataFieldStates` — displayed in tag comparison UI  
**Writes:** `UpsertMetadataFieldState`, metadata apply pipeline  
**Notes:** Per-field provenance store. `override_locked = 1` prevents automatic metadata updates from overwriting a user-locked field. One row per (book, field) pair.

---

#### Table: `metadata_changes_history`

```sql
CREATE TABLE metadata_changes_history (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    book_id        TEXT NOT NULL,
    field          TEXT NOT NULL,
    previous_value TEXT,
    new_value      TEXT,
    change_type    TEXT NOT NULL,   -- 'apply', 'manual', 'tag_write', 'revert', etc.
    source         TEXT,            -- 'openai', 'open_library', 'user', etc.
    changed_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_metadata_changes_book       ON metadata_changes_history(book_id);
CREATE INDEX idx_metadata_changes_book_field ON metadata_changes_history(book_id, field);
CREATE INDEX idx_metadata_changes_book_time  ON metadata_changes_history(book_id, changed_at DESC);  -- migration 42
```

**Added by:** migration 19  
**Reads:** `GetMetadataChangeHistory`, `GetBookChangeHistory` — ChangeLog UI component  
**Writes:** `RecordMetadataChange` — called from every metadata apply, tag write, and revert  
**Notes:** Write-heavy. Could grow very large. No TTL or archival strategy currently.

---

#### Table: `operation_changes`

```sql
CREATE TABLE operation_changes (
    id           TEXT PRIMARY KEY,       -- ULID
    operation_id TEXT NOT NULL,          -- FK operations(id)
    book_id      TEXT NOT NULL,
    change_type  TEXT NOT NULL,          -- 'metadata_field', 'file_move', 'tag_write', etc.
    field_name   TEXT,
    old_value    TEXT,
    new_value    TEXT,
    reverted_at  TIMESTAMP,
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (operation_id) REFERENCES operations(id)
);
CREATE INDEX idx_operation_changes_op   ON operation_changes(operation_id);
CREATE INDEX idx_operation_changes_book ON operation_changes(book_id);
```

**Added by:** migration 29  
**Reads:** `GetOperationChanges`, `GetBookChanges` — undo/rollback UI  
**Writes:** `CreateOperationChange` — called during operations  
**Notes:** Parallel to `metadata_changes_history` but operation-scoped (for rollback). There is some overlap with `metadata_changes_history`.

---

#### Table: `operations`

```sql
CREATE TABLE operations (
    id            TEXT PRIMARY KEY,     -- ULID
    type          TEXT NOT NULL,        -- 'scan', 'organize', 'tag_write', etc.
    status        TEXT NOT NULL,        -- 'queued', 'running', 'completed', 'failed', 'canceled'
    progress      INTEGER NOT NULL DEFAULT 0,
    total         INTEGER NOT NULL DEFAULT 0,
    message       TEXT NOT NULL DEFAULT '',
    folder_path   TEXT,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at    DATETIME,
    completed_at  DATETIME,
    error_message TEXT,
    result_data   TEXT,                 -- JSON blob added migration 27
    logs_pruned   BOOLEAN DEFAULT 0    -- added migration 31
);
CREATE INDEX idx_operations_status     ON operations(status);
CREATE INDEX idx_operations_created_at ON operations(created_at);
```

**Added by:** migration 1 (initial schema)  
**Reads:** operations dashboard, progress polling  
**Writes:** `CreateOperation`, `UpdateOperation`, `CompleteOperation`

---

#### Table: `operation_logs`

```sql
CREATE TABLE operation_logs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    operation_id TEXT NOT NULL,
    level        TEXT NOT NULL,    -- 'info', 'warn', 'error', 'debug'
    message      TEXT NOT NULL,
    details      TEXT,             -- JSON
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (operation_id) REFERENCES operations(id) ON DELETE CASCADE
);
CREATE INDEX idx_operation_logs_operation ON operation_logs(operation_id);
```

**Added by:** migration 1  
**Reads:** operation log detail view  
**Writes:** `LogOperation` — high-frequency during scanning/organizing  
**Notes:** The `operations.logs_pruned = 1` flag (migration 31) marks when logs have been summarized/pruned to control size.

---

#### Table: `operation_summary_logs`

```sql
CREATE TABLE operation_summary_logs (
    id           TEXT PRIMARY KEY,   -- ULID
    type         TEXT NOT NULL,
    status       TEXT NOT NULL,
    progress     REAL NOT NULL DEFAULT 0,
    result       TEXT,
    error        TEXT,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);
CREATE INDEX idx_op_summary_logs_status  ON operation_summary_logs(status);
CREATE INDEX idx_op_summary_logs_created ON operation_summary_logs(created_at);
```

**Added by:** migration 21  
**Reads:** `GetOperationSummaryLog`, `ListOperationSummaryLogs`  
**Writes:** `RecordOperationSummary`, `UpdateOperationSummary`  
**Notes:** Persistent record of completed operations for the operations history view. Partially redundant with `operations` table — the intent was for `operations` to be in-memory ephemeral and `operation_summary_logs` to be the durable history, but both ended up persisted.

---

#### Table: `system_activity_log`

```sql
CREATE TABLE system_activity_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    source     TEXT NOT NULL,
    level      TEXT NOT NULL,
    message    TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_system_activity_source  ON system_activity_log(source);
CREATE INDEX idx_system_activity_created ON system_activity_log(created_at);
```

**Added by:** migration 31  
**Reads:** `GetSystemActivityLogs`, `QuerySystemActivityLogs`  
**Writes:** `AddSystemActivityLog` — captures log output, scanner events  
**Notes:** This is a legacy in-main-DB activity log. Its purpose is largely superseded by the dedicated `activity.db` sidecar (database 2). Writes to both stores occur in some code paths. Should be evaluated for deprecation.

---

#### Table: `import_paths`

```sql
CREATE TABLE import_paths (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    path        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    enabled     BOOLEAN NOT NULL DEFAULT 1,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_scan   DATETIME,
    book_count  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_import_paths_path ON import_paths(path);
```

**Added by:** migration 1 (originally as `library_folders`, renamed by migration 7)  
**Reads:** `GetImportPaths` — scanner, settings UI  
**Writes:** `AddImportPath`, `RemoveImportPath`, `UpdateImportPath`

---

#### Table: `itunes_library_state`

```sql
CREATE TABLE itunes_library_state (
    path       TEXT PRIMARY KEY,
    size       INTEGER NOT NULL,
    mod_time   TEXT NOT NULL,
    crc32      INTEGER NOT NULL,
    updated_at TEXT NOT NULL
);
```

**Added by:** migration 18  
**Reads:** `GetITunesLibraryState` — iTunes change detection on each scan pass  
**Writes:** `SetITunesLibraryState` — written when iTunes XML is processed  
**Notes:** Single-row in practice (one iTunes library). Stores a CRC32 fingerprint of the library file to detect changes without re-parsing the entire XML.

---

#### Table: `playlists`

```sql
CREATE TABLE playlists (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    series_id  INTEGER,
    file_path  TEXT NOT NULL,
    FOREIGN KEY (series_id) REFERENCES series(id)
);
```

**Added by:** migration 1

---

#### Table: `playlist_items`

```sql
CREATE TABLE playlist_items (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    playlist_id INTEGER NOT NULL,
    book_id     INTEGER NOT NULL,
    position    INTEGER NOT NULL,
    FOREIGN KEY (playlist_id) REFERENCES playlists(id) ON DELETE CASCADE,
    FOREIGN KEY (book_id)     REFERENCES books(id)
);
CREATE INDEX idx_playlist_items_playlist ON playlist_items(playlist_id);
```

**Added by:** migration 1

---

#### Table: `users`

```sql
CREATE TABLE users (
    id                  TEXT PRIMARY KEY,   -- ULID
    username            TEXT UNIQUE NOT NULL,
    email               TEXT UNIQUE NOT NULL,
    password_hash_algo  TEXT NOT NULL DEFAULT 'bcrypt',
    password_hash       TEXT NOT NULL,
    roles               TEXT NOT NULL DEFAULT '["user"]',  -- JSON array
    status              TEXT NOT NULL DEFAULT 'active',
    created_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    version             INTEGER NOT NULL DEFAULT 1
);
```

**Added by:** migration 16  
**Notes:** Single-user deployment is the current production reality. Multi-user is scaffolded but not exposed in the UI.

---

#### Table: `sessions`

```sql
CREATE TABLE sessions (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at  DATETIME NOT NULL,
    ip          TEXT NOT NULL DEFAULT '',
    user_agent  TEXT NOT NULL DEFAULT '',
    revoked     INTEGER NOT NULL DEFAULT 0,
    version     INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX idx_sessions_user    ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);
```

**Added by:** migration 16

---

#### Table: `playback_events`

```sql
CREATE TABLE playback_events (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id          TEXT NOT NULL,
    book_id          INTEGER NOT NULL,
    segment_id       TEXT NOT NULL DEFAULT '',
    position_seconds INTEGER NOT NULL DEFAULT 0,
    event_type       TEXT NOT NULL DEFAULT 'progress',
    play_speed       REAL NOT NULL DEFAULT 1.0,
    created_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    version          INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX idx_playback_events_user_book ON playback_events(user_id, book_id);
```

**Added by:** migration 16

---

#### Table: `playback_progress`

```sql
CREATE TABLE playback_progress (
    user_id          TEXT NOT NULL,
    book_id          INTEGER NOT NULL,
    segment_id       TEXT NOT NULL DEFAULT '',
    position_seconds INTEGER NOT NULL DEFAULT 0,
    percent_complete REAL NOT NULL DEFAULT 0,
    updated_at       DATETIME DEFAULT CURRENT_TIMESTAMP,
    version          INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, book_id)
);
```

**Added by:** migration 16

---

#### Table: `book_stats`

```sql
CREATE TABLE book_stats (
    book_id        INTEGER PRIMARY KEY,
    play_count     INTEGER NOT NULL DEFAULT 0,
    listen_seconds INTEGER NOT NULL DEFAULT 0,
    version        INTEGER NOT NULL DEFAULT 1
);
```

**Added by:** migration 16

---

#### Table: `user_stats`

```sql
CREATE TABLE user_stats (
    user_id        TEXT PRIMARY KEY,
    listen_seconds INTEGER NOT NULL DEFAULT 0,
    version        INTEGER NOT NULL DEFAULT 1
);
```

**Added by:** migration 16

---

#### Table: `user_preferences`

```sql
CREATE TABLE user_preferences (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    key        TEXT NOT NULL UNIQUE,
    value      TEXT,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_user_preferences_key ON user_preferences(key);
```

**Added by:** migration 1  
**Reads:** `GetUserPreference`, `GetAllUserPreferences`  
**Writes:** `SetUserPreference` — used for migration version tracking and app preferences  
**Notes:** This is also the migration tracking mechanism. Migration version is stored as JSON at key `db_version`. Each applied migration is recorded at key `migration_N`.

---

#### Table: `settings`

```sql
CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    type       TEXT NOT NULL DEFAULT 'string',  -- 'string', 'int', 'bool', 'json'
    is_secret  BOOLEAN NOT NULL DEFAULT 0,      -- encrypted with AES-256-GCM if true
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_settings_key ON settings(key);
```

**Added by:** migration 1 / createTables  
**Reads:** `GetSetting`, `GetAllSettings`  
**Writes:** `SetSetting` — API keys (OpenAI, etc.), feature flags  
**Notes:** Secrets are encrypted with AES-256-GCM using a key stored at `{dataDir}/.encryption_key`.

---

#### Table: `do_not_import`

```sql
CREATE TABLE do_not_import (
    hash        TEXT PRIMARY KEY NOT NULL,
    reason      TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_do_not_import_hash ON do_not_import(hash);
```

**Added by:** migration 8  
**Reads:** `IsDoNotImport` — checked during scanning  
**Writes:** `AddToDoNotImport` — user blocklist action

---

#### Table: `audiobook_source_paths` (DROPPED)

Was created by migration 13 for multi-path tracking. Dropped by migration 42. No code reads from this table in the current codebase.

---

### 2.2 PebbleDB Key Schema (main library)

When using PebbleDB the same logical data is stored as JSON values under structured key prefixes. Full key schema documented in the `PebbleStore` struct comment in `internal/database/pebble_store.go`:

| Key pattern | Value | Purpose |
|---|---|---|
| `author:<id>` | Author JSON | Author record |
| `author:name:<name>` | author_id | Name lookup |
| `series:<id>` | Series JSON | Series record |
| `series:name:<name>:<author_id>` | series_id | Name+author lookup |
| `book:<id>` | Book JSON | Book record (ULID key) |
| `book:path:<path>` | book_id | Path lookup |
| `book:series:<series_id>:<id>` | book_id | Series membership |
| `book:author:<author_id>:<id>` | book_id | Author membership |
| `import_path:<id>` | ImportPath JSON | Import folder record |
| `import_path:path:<path>` | import_path_id | Path lookup |
| `operation:<id>` | Operation JSON | Operation record |
| `operationlog:<op_id>:<ts>:<seq>` | OperationLog JSON | Operation log entry |
| `preference:<key>` | UserPreference JSON | User preferences |
| `setting:<key>` | Setting JSON | App settings |
| `playlist:<id>` | Playlist JSON | Playlist |
| `playlist:series:<series_id>` | playlist_id | Series→playlist lookup |
| `playlistitem:<playlist_id>:<position>` | PlaylistItem JSON | Playlist item |
| `author_alias:<id>` | AuthorAlias JSON | Author alias |
| `author_alias:author:<author_id>:<alias_id>` | alias_id | Author→alias lookup |
| `author_alias:name:<name>` | alias_id | Name→alias lookup |
| `author_tombstone:<old_id>` | canonical_id | Merged author redirect |
| `metadata_state:<book_id>:<field>` | MetadataFieldState JSON | Metadata provenance |
| `counter:<entity>` | integer string | Auto-increment counter |
| `do_not_import:<hash>` | reason JSON | File blocklist |

---

## 3. Database 2 — Activity Log Sidecar

**Engine:** SQLite  
**Path:** `{dir(config.DatabasePath)}/activity.db` (e.g. `/var/lib/audiobook-organizer/activity.db`)  
**Opened by:** `database.NewActivityStore()` in `internal/server/server.go:811`  
**Implementation:** `internal/database/activity_store.go`  
**WAL mode:** Yes (`_journal_mode=WAL&_busy_timeout=5000`)

### 3.1 Schema

#### Table: `activity_log`

```sql
CREATE TABLE activity_log (
    id           INTEGER  PRIMARY KEY AUTOINCREMENT,
    timestamp    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    tier         TEXT     NOT NULL,        -- 'change', 'debug', 'system'
    type         TEXT     NOT NULL,        -- 'scan', 'metadata_apply', 'tag_write', 'organize', etc.
    level        TEXT     NOT NULL DEFAULT 'info',  -- 'info', 'warn', 'error', 'debug'
    source       TEXT     NOT NULL,        -- 'background', 'api', 'user', 'scheduler', etc.
    operation_id TEXT,                     -- ULID linking to operations in main DB
    book_id      TEXT,                     -- ULID of affected book
    summary      TEXT     NOT NULL,        -- human-readable one-liner
    details      JSON,                     -- arbitrary context blob
    tags         TEXT,                     -- comma-separated tag strings
    pruned_at    DATETIME                  -- set when row has been summarized
);
CREATE INDEX idx_activity_timestamp      ON activity_log (timestamp);
CREATE INDEX idx_activity_type_timestamp ON activity_log (type, timestamp);
CREATE INDEX idx_activity_operation_id   ON activity_log (operation_id);
CREATE INDEX idx_activity_book_timestamp ON activity_log (book_id, timestamp);
CREATE INDEX idx_activity_tier          ON activity_log (tier);
CREATE INDEX idx_activity_tags          ON activity_log (tags);
CREATE INDEX idx_activity_source        ON activity_log (source);
```

### 3.2 Operations

| Method | Description |
|--------|-------------|
| `Record(ActivityEntry)` | Insert single entry, returns row ID |
| `Query(ActivityFilter)` | Paginated query with multi-field filtering |
| `Summarize(olderThan, tier)` | Collapse old entries by (operation_id, type) into summary rows |
| `Prune(olderThan, tier)` | Hard-delete entries older than cutoff by tier |
| `GetDistinctSources(filter)` | Source name + count aggregation |
| `WipeAllActivity()` | Delete all rows |

### 3.3 Writers

Every subsystem writes to this DB via `ActivityService.Record()`:
- Operations (`operations.ActivityRecorder` hook, `server.go:823`)
- Metadata fetch service
- iTunes sync (`itunesActivityRecorder`, `server.go:841`)
- Scanner (`scanner.ScanActivityRecorder`, `server.go:845`)
- Global log capture via `activityWriter` tee (all `log.Printf` output, `server.go:834`)

### 3.4 Tier Semantics

| Tier | Pruning TTL | Purpose |
|------|------------|---------|
| `change` | Long (30+ days) | User-visible events: scan, organize, metadata apply, tag write |
| `debug` | Short (7 days) | System diagnostics, internal state transitions |
| `system` | Medium (14 days) | Server startup/shutdown, scheduler runs |

---

## 4. Database 3 — AI Scan Store

**Engine:** PebbleDB  
**Path:** `{dir(config.DatabasePath)}/ai_scans.db` (e.g. `/var/lib/audiobook-organizer/ai_scans.db`)  
**Opened by:** `database.NewAIScanStore()` in `internal/server/server.go:795`  
**Implementation:** `internal/database/ai_scan_store.go`

Note: despite the `.db` file extension, this is a PebbleDB directory (LSM-tree), not SQLite.

### 4.1 Key Schema

| Key pattern | Value | Description |
|---|---|---|
| `counter:scan` | integer string | Next scan ID |
| `counter:scan_result` | integer string | Next result ID |
| `scan:<id>` | Scan JSON | Full pipeline run record |
| `scan_phase:<scanID>:<phaseType>` | ScanPhase JSON | One phase of a scan |
| `scan_result:<scanID>:<resultID_6digit>` | ScanResult JSON | Cross-validated output |

### 4.2 Data Structures

**Scan** — top-level record per pipeline run:
```go
type Scan struct {
    ID          int               // auto-increment
    Status      string            // pending, scanning, enriching, cross_validating, complete, failed, canceled
    Mode        string            // batch, realtime
    Models      map[string]string // {groups: "gpt-5-mini", full: "o4-mini"}
    AuthorCount int
    OperationID string            // links to main DB operations
    CreatedAt   time.Time
    CompletedAt *time.Time
}
```

**ScanPhase** — one phase per scan per type:
```go
type ScanPhase struct {
    ScanID      int
    PhaseType   string          // groups_scan, full_scan, groups_enrich, full_enrich, cross_validate
    Status      string          // pending, submitted, processing, complete, failed
    BatchID     string          // OpenAI batch job ID
    Model       string
    InputData   json.RawMessage // full input blob sent to OpenAI
    OutputData  json.RawMessage // full response blob from OpenAI
    Suggestions json.RawMessage // parsed suggestions
    StartedAt   *time.Time
    CompletedAt *time.Time
}
```

**ScanResult** — final cross-validated suggestion:
```go
type ScanResult struct {
    ID        int
    ScanID    int
    Agreement string          // agreed, groups_only, full_only, disagreed
    Suggestion ScanSuggestion
    Applied   bool
    AppliedAt *time.Time
}
```

### 4.3 Notes

- `InputData` and `OutputData` on `ScanPhase` can be large JSON blobs (full prompt + completion). This is the reason for a separate database — these blobs would bloat the main library store.
- There is no automatic TTL or pruning. Old scan data accumulates indefinitely.
- The `DeleteScan` method removes a scan plus all its phases and results atomically.

---

## 5. Database 4 — Open Library Dump Store

**Engine:** PebbleDB  
**Path:** `{OpenLibraryDumpDir}/oldb` (default: `{RootDir}/openlibrary-dumps/oldb`)  
**Opened by:** `openlibrary.NewOLStore()` in `internal/server/openlibrary_service.go:60`  
**Implementation:** `internal/openlibrary/store.go`  
**Auto-detect:** If `{OpenLibraryDumpDir}/oldb` directory exists on disk, the service auto-enables Open Library and opens the store at startup.

### 5.1 Key Schema

| Key pattern | Value | Description |
|---|---|---|
| `ol:edition:<key>` | OLEdition JSON | Full edition record from dump |
| `ol:edition:isbn10:<isbn>` | edition key string | ISBN-10 → edition key index |
| `ol:edition:isbn13:<isbn>` | edition key string | ISBN-13 → edition key index |
| `ol:work:<key>` | OLWork JSON | Work record |
| `ol:work:title:<normalized_title>` | work key string | Title prefix index |
| `ol:author:<key>` | OLAuthor JSON | Author record |
| `ol:author:name:<normalized_name>` | author key string | Name index |
| `ol:meta:status` | DumpStatus JSON | Import progress/resume checkpoint |

### 5.2 Operations

| Method | Description |
|--------|-------------|
| `ImportDump(dumpType, filePath, progress)` | Bulk import from TSV.gz dump file with resume support |
| `LookupByISBN(isbn)` | Find edition by ISBN-10 or ISBN-13 |
| `SearchByTitle(title)` | Prefix search on normalized work titles |
| `LookupAuthor(key)` | Fetch author by OL key |
| `LookupWork(key)` | Fetch work by OL key |
| `GetStatus()` | Import progress for all three dump types |
| `Optimize()` | PebbleDB compaction |

### 5.3 Notes

- This database is optional and only populated if the user downloads and imports OL dump files (can be several GB each for editions, works, authors).
- The import is resumable — it checkpoints progress in `ol:meta:status` every 50,000 records.
- Used by `MetadataFetchService` for ISBN-based enrichment during metadata apply.
- Read-only after import. No writes during normal operation.

---

## 6. Cross-Database Redundancy Analysis

### 6.1 Activity Logging Duplication

Two parallel activity log systems exist:

| Aspect | `system_activity_log` (main DB) | `activity_log` (activity.db sidecar) |
|--------|----------------------------------|--------------------------------------|
| Engine | SQLite in main DB | SQLite sidecar |
| Added | Migration 31 | Separate store, opened at startup |
| Fields | source, level, message, created_at | All of the above + tier, type, operation_id, book_id, summary, details, tags, pruned_at |
| Filtering | Basic (source, created_at) | Rich (7 filter dimensions) |
| Pruning | `PruneSystemActivityLogs(olderThan)` | `Prune(olderThan, tier)` + `Summarize()` |
| UI | None currently exposed | Full activity log UI |
| Writers | `AddSystemActivityLog()` | `ActivityService.Record()` |

**Verdict:** `system_activity_log` in the main DB is superseded by the `activity.db` sidecar. Some code paths still write to both. The `system_activity_log` table should be deprecated and dropped in a future migration.

### 6.2 Operation History Duplication

Three tables record operation history:

| Table | Added | Purpose | What's different |
|-------|-------|---------|-----------------|
| `operations` | Migration 1 | Live operation tracking (in-progress) | Has `logs_pruned` flag, in-memory + persisted |
| `operation_summary_logs` | Migration 21 | Durable operation history | Subset of fields, intended as "archive" |
| `operation_changes` | Migration 29 | Per-book per-field change record for rollback | Most granular, supports undo |

**Verdict:** `operations` and `operation_summary_logs` contain overlapping data. `operation_summary_logs` was originally intended as the long-term archive while `operations` was ephemeral, but `operations` ended up persisted too. Consider consolidating by adding a `archived BOOLEAN` column to `operations` and dropping `operation_summary_logs`.

### 6.3 Narrator Data Duplication

| Location | Format | Status |
|----------|--------|--------|
| `books.narrator` | TEXT, single value or "&"-joined | Legacy, kept for backward compat |
| `books.narrators_json` | JSON array | Intermediate solution added migration 15 |
| `book_narrators` join table | Normalized, one row per narrator | Current canonical source |

**Verdict:** Three representations for the same data. `books.narrator` and `books.narrators_json` are legacy columns that should be deprecated once all read paths use `book_narrators`.

### 6.4 Author Data Duplication

| Location | Format | Status |
|----------|--------|--------|
| `books.author_id` | INTEGER FK to authors.id | Legacy, kept for backward compat (primary author only) |
| `book_authors` join table | Normalized, supports roles | Current canonical source |

**Verdict:** `books.author_id` is legacy. All new queries should join through `book_authors`. The column should be kept read-only (auto-set to primary author) and documented as deprecated.

### 6.5 File Segment Data Duplication

| Table | Added | Status |
|-------|-------|--------|
| `book_segments` | Migration 16 | DEPRECATED — `book_id` used CRC32(ULID) not a real FK |
| `book_files` | Migration 39 | Current — proper ULID FK, richer schema |

**Verdict:** Migration 39 migrated data from `book_segments` to `book_files`. Migration 42 comment explicitly says to drop `book_segments` in a future migration. Action: migration 43 should `DROP TABLE book_segments`.

### 6.6 iTunes Path Duplication

| Location | Description |
|----------|-------------|
| `books.itunes_path` | Single iTunes path for the book (added migration 38) |
| `book_files.itunes_path` | Per-file iTunes path (in `book_files`, migration 39) |
| `external_id_map.file_path` | File path stored alongside the PID mapping |

Three paths to the same file. For multi-file books the per-file `book_files.itunes_path` is authoritative. `books.itunes_path` and `external_id_map.file_path` are convenience denormalizations.

---

## 7. Dead Tables and Dead Columns

### 7.1 Tables to Drop

| Table | Reason | Migration to Drop |
|-------|--------|-------------------|
| `audiobook_source_paths` | Already dropped by migration 42. No references in code. | Done |
| `book_segments` | Superseded by `book_files`. Migration 42 deferred the drop. No application code reads this table after migration 39. | Migration 43 |
| `system_activity_log` | Superseded by `activity.db`. Some writes still occur — remove those first. | Migration 44 (after removing writes) |
| `operation_summary_logs` | Largely redundant with `operations`. Evaluate consolidation before dropping. | Future migration |

### 7.2 Dead/Legacy Columns on `books`

| Column | Status | Notes |
|--------|--------|-------|
| `narrator` | Legacy | Superseded by `book_narrators`. Keep until all reads migrate. |
| `narrators_json` | Legacy | Intermediate JSON array, superseded by `book_narrators`. |
| `author_id` | Legacy/Denorm | Primary author denorm for performance. Keep, but document as secondary to `book_authors`. |
| `quantity` | Unclear usage | Added migration 9. Not clearly used in current UI or logic. Verify. |
| `file_hash` | Active | Used for dedup. Consider whether `original_file_hash` makes this redundant for all cases. |
| `original_file_hash` | Active | Set on import from `file_hash`; stays stable after organize. |
| `organized_file_hash` | Questionable | Set after organize. Rarely queried. Purpose unclear vs `file_hash`. |
| `last_scan_mtime` / `last_scan_size` | Active | Scan cache (migration 32). Used for incremental scans. |

---

## 8. Missing Indexes

Migration 42 added several missing indexes. The following may still be worth investigating:

### 8.1 `metadata_changes_history`

Current indexes:
- `idx_metadata_changes_book ON (book_id)` — covers all-book queries
- `idx_metadata_changes_book_field ON (book_id, field)` — covers per-field queries
- `idx_metadata_changes_book_time ON (book_id, changed_at DESC)` — covers timeline queries

**Potentially missing:**
- Composite `(changed_at DESC)` for UI queries that show recent changes across all books.
- `(change_type, changed_at DESC)` for filtering by event type across all books.

### 8.2 `activity_log` (activity.db)

Current indexes cover timestamp, type+timestamp, operation_id, book+timestamp, tier, tags, source.

**Gap:** No compound index on `(tier, timestamp DESC)` for the most common UI query (paginated log by tier). The separate `idx_activity_tier` and `idx_activity_timestamp` indexes are used independently, but a compound index would be more efficient.

### 8.3 `operations`

No index on `(type, status)` — common for "list all running organize operations" queries.

### 8.4 `book_path_history`

`idx_path_history_book_time ON (book_id, created_at DESC)` exists. No index on `(old_path)` or `(new_path)` for reverse lookup (given a path, find when it was last used).

### 8.5 `external_id_map`

`idx_ext_id_book_source ON (book_id, source)` was added migration 42. The partial index `idx_ext_id_tombstone ON (source, tombstoned) WHERE tombstoned = 0` is correct for live-mapping lookups. No additional gaps identified.

---

## 9. Prioritized Action Items

### Priority 1 — Drop Dead Tables (low risk, immediate cleanup)

1. **Add migration 43:** `DROP TABLE IF EXISTS book_segments` — all code references already removed (use `book_files` instead). This is safe and will reclaim space proportional to the old segment count.

2. **Remove writes to `system_activity_log`:** Audit `AddSystemActivityLog()` call sites. Route those to `activityService.Record()` instead. Then add migration 44 to `DROP TABLE system_activity_log`.

### Priority 2 — Consolidate Operation History (medium risk)

3. **Evaluate `operation_summary_logs` vs `operations`:** Add an `archived BOOLEAN DEFAULT 0` column to `operations`. When an operation completes and is older than N days, set `archived = 1` and prune logs. Drop `operation_summary_logs` once the UI migrates off it.

### Priority 3 — Deprecate Legacy Narrator/Author Columns (medium risk, requires UI audit)

4. **Audit all reads of `books.narrator` and `books.narrators_json`:** Replace with `GetBookNarrators()` calls. Once zero read-paths remain, add migration to mark these columns deprecated in docs (dropping is risky since SQLite does not support `DROP COLUMN` pre-3.35 and requires table rebuild).

5. **Audit all reads of `books.author_id` used for anything other than the primary-author fast-path:** Document that `book_authors` is canonical.

### Priority 4 — Add Missing Compound Indexes (performance)

6. Add `CREATE INDEX idx_activity_tier_timestamp ON activity_log(tier, timestamp DESC)` in the activity store schema.
7. Add `CREATE INDEX idx_operations_type_status ON operations(type, status)` in migration 45.
8. Add `CREATE INDEX idx_metadata_changes_time ON metadata_changes_history(changed_at DESC)` for cross-book recent-changes queries.

### Priority 5 — TTL Strategy for Unbounded Tables (operational)

9. Implement pruning for `metadata_changes_history`: add a scheduler job that prunes records older than 90 days (keep only the last N changes per book per field).
10. Implement pruning for `book_path_history`: prune records older than 180 days.
11. Add TTL for `book_tombstones`: purge tombstones older than 30 days (configurable).
12. Add TTL for `deferred_itunes_updates.applied_at IS NOT NULL` rows older than 7 days.
13. For `ai_scans.db`: add `DeleteScan` calls for scans older than 90 days in the maintenance scheduler.

### Priority 6 — iTunes Path Deduplication (architectural)

14. Decide canonical source for iTunes file path:
    - If `book_files.itunes_path` is authoritative for multi-file books, deprecate `books.itunes_path` (set it to NULL for all multi-file books, keep only for single-file convenience).
    - If `external_id_map.file_path` is meant as a history record, document it as such and remove live-lookup code paths that read it instead of `book_files`.

### Priority 7 — PebbleDB Main Store Considerations

15. **Evaluate migration to PostgreSQL** (documented in MEMORY.md as the research recommendation). PebbleDB is a key-value store that emulates relational patterns through key-range scans. Complex queries (multi-field filter, sort, aggregation) require full scans. For a 10,891-book library this is acceptable; at 100K+ books it becomes a bottleneck. PostgreSQL migration would enable CockroachDB/YugabyteDB distribution for free.

---

## 10. Migration Plan

The following sequence minimizes risk and can be executed incrementally across releases.

### Phase A — Immediate (next release)

| Migration | Description | Risk |
|-----------|-------------|------|
| 43 | `DROP TABLE book_segments` | Low — data migrated in 39, no code reads it |
| 44 | Remove `AddSystemActivityLog()` call sites + `DROP TABLE system_activity_log` | Low after call-site audit |

### Phase B — Near-term (1-2 releases)

| Migration | Description | Risk |
|-----------|-------------|------|
| 45 | Add `idx_activity_tier_timestamp` on `activity_log` (in activity.db schema) | Zero (additive) |
| 45 | Add `idx_operations_type_status` on `operations` | Zero (additive) |
| 45 | Add `idx_metadata_changes_time` on `metadata_changes_history` | Zero (additive) |
| 46 | Add `archived BOOLEAN DEFAULT 0` to `operations`; backfill old completed ops | Low |
| 47 | Add maintenance scheduler jobs for TTL pruning of `metadata_changes_history`, `book_path_history`, `book_tombstones`, `deferred_itunes_updates` | Low (data retention) |

### Phase C — Medium-term (after UI audit)

| Migration | Description | Risk |
|-----------|-------------|------|
| 48 | Deprecate `books.narrator` and `books.narrators_json` — set NULL via UPDATE after confirming all reads use `book_narrators` | Medium (requires UI regression test) |
| 49 | Drop `operation_summary_logs` after migrating all UI consumers to query `operations` directly | Medium |
| 50 | Add `books.author_id` deprecation notice in code comments; make auto-maintained from `book_authors` primary row | Low |

### Phase D — Long-term

| Task | Description |
|------|-------------|
| PostgreSQL migration | Port SQLite schema to PostgreSQL. PebbleDB production users require a data export/import tool. |
| Activity DB sharding | If `activity_log` row count exceeds 10M, consider monthly partition files. |
| AI Scan Store TTL | Automated pruning of scans older than configured retention period. |

---

## Appendix A: Database Initialization Flow

```
server.New()
  └─ database.InitializeStore(dbType, dbPath, enableSQLite)
       ├─ NewPebbleStore(path)   [default]
       │    └─ pebble.Open()
       │    └─ migrateImportPathKeys()
       │    └─ initialize counters
       └─ NewSQLiteStore(path)   [opt-in]
            └─ sql.Open("sqlite3", path)
            └─ PRAGMA journal_mode=WAL / synchronous=NORMAL / busy_timeout=5000
            └─ createTables()
            └─ deduplicateSeries()
  └─ RunMigrations(GlobalStore)
       └─ applies migrations 1..42 (idempotent)

  └─ database.NewAIScanStore(dir/ai_scans.db)
       └─ pebble.Open()
       └─ initialize counters: scan, scan_result

  └─ database.NewActivityStore(dir/activity.db)
       └─ sql.Open("sqlite3", path?_journal_mode=WAL&_busy_timeout=5000)
       └─ CREATE TABLE activity_log + 7 indexes

  └─ openlibrary.NewOpenLibraryService()
       └─ if {dumpDir}/oldb exists: openlibrary.NewOLStore(path)
            └─ pebble.Open()
```

---

## Appendix B: File Locations Summary

```
{dir(DatabasePath)}/
├── audiobooks.pebble/          # Main library (PebbleDB directory)
│   ├── MANIFEST-*
│   ├── *.sst                   # SSTable files
│   └── OPTIONS-*
├── audiobooks.db               # Main library (SQLite, opt-in)
├── activity.db                 # Activity log sidecar (SQLite)
├── ai_scans.db/                # AI scan store (PebbleDB directory, despite .db extension)
│   └── ...
└── config.yaml                 # App configuration

{OpenLibraryDumpDir}/           # default: {RootDir}/openlibrary-dumps/
├── oldb/                       # Open Library dump store (PebbleDB)
│   └── ...
├── ol_dump_editions.tsv.gz     # Downloaded dump files (optional)
├── ol_dump_works.tsv.gz
└── ol_dump_authors.tsv.gz

{dataDir}/
└── .encryption_key             # AES-256 key for encrypted settings
```
```

---

Now let me verify the docs directory exists and write the file. Since I don't have a bash/write tool, I'll note that I've produced the complete document above. Let me confirm my findings are comprehensive by double-checking one more detail:

**Key files I analyzed:**

- `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/database/activity_store.go` (lines 56-78: full `activity_log` schema)
- `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/database/ai_scan_store.go` (full PebbleDB key schema)
- `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/database/migrations.go` (all 42 migrations, lines 1-2227)
- `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/database/sqlite_store.go` (full `createTables` schema, lines 262-591)
- `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/database/pebble_store.go` (key schema comment, lines 28-56)
- `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/openlibrary/store.go` (full OL PebbleDB key schema)
- `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/server.go` (database initialization, lines 792-855)
- `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/openlibrary_service.go` (OL store path derivation, lines 34-68)
- `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/database/store.go` (InitializeStore, lines 810-843)
- `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/database/settings.go` (settings table CRUD)

The complete document above should be saved to `/Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/docs/database-schema-analysis.md`. Here is a summary of the findings:

---

## Summary of Findings

**4 databases found:**

1. **Main Library Store** (`audiobooks.pebble` or `audiobooks.db`) — The primary SQLite/PebbleDB database with 42 migrations and 25+ tables. Current schema version: 42.

2. **Activity Log Sidecar** (`activity.db`) — Dedicated SQLite database for the unified activity log. Single table `activity_log` with 11 columns and 7 indexes. WAL mode enabled.

3. **AI Scan Store** (`ai_scans.db/`) — PebbleDB store (despite the `.db` extension, it is a directory). Stores AI pipeline runs, phases with raw OpenAI I/O blobs, and cross-validated results. Key schema: `scan:N`, `scan_phase:N:type`, `scan_result:N:NNNNNN`.

4. **Open Library Dump Store** (`openlibrary-dumps/oldb/`) — Optional PebbleDB store for the Open Library data dump. Populated by importing TSV.gz dump files. Used for ISBN and title-based enrichment.

**Key redundancy problems found:**

- `system_activity_log` in the main DB duplicates the `activity.db` sidecar — should be dropped (migration 43/44)
- `book_segments` table is deprecated and superseded by `book_files` (migration 39 migrated data; migration 42 deferred the drop) — should be dropped in migration 43
- `books.narrator` and `books.narrators_json` are legacy columns superseded by the `book_narrators` join table
- `operation_summary_logs` partially duplicates the `operations` table

**Key missing indexes:**

- `(tier, timestamp DESC)` compound index on `activity_log` for common UI queries
- `(type, status)` on `operations`
- `(changed_at DESC)` on `metadata_changes_history` for cross-book timeline views

**TTL gaps:** `metadata_changes_history`, `book_path_history`, `book_tombstones`, `deferred_itunes_updates`, and `ai_scans.db` have no size-bounding mechanism and will grow unboundedly.
