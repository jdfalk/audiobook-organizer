# Task 1.1: Chai SQL Schema Design — COMPLETE

## Overview

Designed and implemented a complete normalized Chai SQL schema to replace 9,300 lines of manual Pebble indexing in `pebble_store.go`. This is Phase 1, Foundation Setup — blocking work for all downstream migration tasks.

## Deliverables

### 1. `internal/database/schema.sql` (589 lines)

Full CREATE TABLE statements with:
- **25 normalized tables** covering all core entities
- **31 strategic indexes** on frequently queried columns
- **FOREIGN KEY constraints** for referential integrity
- **UNIQUE constraints** on natural keys

### 2. `internal/database/migration.go` (223 lines)

Schema initialization and utility functions:
- `InitializeChaiSchema(ctx, db)` — idempotent schema setup (called by NewChaiDB)
- `validateSchemaIntegrity(ctx, db)` — defensive post-init check
- `DropChaiSchema(ctx, db)` — test teardown (DANGEROUS)
- `ExportSchemaAsSQL()` — returns schema for reversibility
- `splitStatements()` — parses multi-statement SQL for Chai compatibility
- `SchemaVersion()` — tracks breaking changes

## Schema Design Details

### Core Tables

| Table | Purpose | Rows | PK | FKs |
|-------|---------|------|----|----|
| `authors` | Audiobook authors | 8,837 | int | — |
| `series` | Book series | 21,668 | int | author_id |
| `books` | Audiobooks (primary content) | 10,891 | text (ULID) | author_id, series_id |
| `book_files` | File-level metadata (chapters) | ~35K | text | book_id |
| `book_authors` | Many-to-many author relationship | ~10K | (book_id, author_id) | books, authors |

### Relationship Tables

- `book_narrators` — narrator assignments (many-to-many)
- `book_segments` — chapter/track boundaries
- `user_positions` — playback resume points (user, book, segment)
- `book_versions` — library centralization (spec 3.1)

### Support Tables

| Category | Tables |
|----------|--------|
| **User Management** | users, roles, api_keys, invites |
| **Playlists** | playlists, user_playlists, playlist_items |
| **Operations** | operations, operation_logs |
| **Content** | works, book_alternative_titles, author_aliases, narrators |
| **Settings** | user_preferences, blocked_hashes, import_paths |

### Critical Indexes

**For aggregation queries** (replacing denormalized index prefixes):
- `idx_books_series_id` — GetAllSeriesBookCounts
- `idx_books_author_id` — GetAllAuthorBookCounts
- `idx_book_files_book_id` — GetAllSeriesFileCounts
- `idx_book_authors_author_id` — GetBooksByAuthorID

**For filtering** (composite index for common WHERE patterns):
- `idx_books_primary_not_deleted` — (is_primary_version, marked_for_deletion)

**For deduplication**:
- `idx_book_files_file_hash` — file dedup detection
- `idx_book_alternative_titles_title` — Layer 1 dedup

### Key Design Decisions

#### 1. Replaced Denormalized Indexes

**Before** (Pebble manual):
```
book:series:<series_id>:<id> → Book JSON (full serialize)
book:author:<author_id>:<id> → Book JSON (full serialize)
```

**After** (Chai SQL):
```
book_authors(book_id, author_id, role, position)
  → JOIN books b ON b.id = ba.book_id WHERE ba.author_id = ?
```

Benefits:
- Single write → automatic index updates (no dual-serialize)
- Atomic transactions (no race conditions)
- SQL optimizer handles the JOIN

#### 2. All Book Fields Mapped to Columns

- Core metadata: title, duration, format, narrator
- ISBNs: isbn10, isbn13, asin
- External IDs: open_library_id, hardcover_id, google_books_id
- iTunes fields: persistent_id, play_count, rating, bookmark
- Version tracking: is_primary_version, version_group_id
- Lifecycle: marked_for_deletion, quarantine_reason
- Timestamps: created_at, updated_at, metadata_updated_at, last_written_at
- Advanced: book_sig_v1, audible_rating_*, user_rating_*

#### 3. Multi-User Support

- `users` — application users (ULID IDs)
- `roles` — permission bundles
- `api_keys` — bearer tokens with scoping
- `invites` — user invitations with redemption tracking

#### 4. Metadata Provenance Tracking

Denormalized as `metadata_provenance` JSON in Book struct; can be expanded to a separate table if needed:
```
book_metadata_provenance(book_id, field, file_value, fetched_value, stored_value, override_value, override_locked, effective_source)
```

#### 5. Reversibility

- Schema exported via `ExportSchemaAsSQL()` for version control
- No proprietary SQL syntax (compatible with SQLite migration if needed)
- Data structure matches Pebble key schema for audit trails
- Version numbers track breaking changes

## Acceptance Criteria — ALL MET

- [x] **Books table** has all fields from Book struct (167+ fields mapped)
- [x] **Authors/series/files tables** with proper relationships (FK constraints)
- [x] **Indexes on frequently queried columns**: series_id, author_id, is_primary_version, marked_for_deletion
- [x] **Schema reversible**: ExportSchemaAsSQL() + ULID-based PKs maintain auditability
- [x] **FOREIGN KEY constraints** prevent orphan rows
- [x] **Composite index** on (is_primary_version, marked_for_deletion) for common WHERE patterns
- [x] **31 strategic indexes** covering all common query patterns
- [x] **25 normalized tables** covering all data model entities

## Expected Impact

### Code Reduction

- **Current Pebble**: 9,300 lines of manual indexing in pebble_store.go
- **Estimated with Chai**: ~2,000 lines (78% reduction)

### Performance Improvement

| Query | Pebble | Chai | Speedup |
|-------|--------|------|---------|
| GetAllSeriesBookCounts | O(n) scan | Indexed range + GROUP BY | 10-100x |
| GetAllSeriesFileCounts | Two-phase scan | Single JOIN | 10-100x |
| ListBooks + pagination | Full scan + slice | LIMIT/OFFSET | 50-500x |
| CountFiles | Two-phase | Single subquery | 10-100x |

### Testing Strategy for Phase 2+

Each migration task will:
1. Implement SQL version of aggregation/list function
2. Run both Pebble and Chai versions on 50K library
3. Assert identical results
4. Benchmark both (time, memory)
5. Add feature flag to switch implementations
6. After all tests pass, remove Pebble fallback

## Next Steps

- **Task 1.2** (Chai Integration Layer) — depends on this schema
- **Phase 2** (Aggregation migrations) — can proceed in parallel (5 tasks)
- **Phase 3** (List/filter migrations) — depends on aggregations working
- **Phase 4** (Cleanup) — remove manual index logic, deprecate Pebble code

## Files

- Push branch: `worktree-genji-poc`
- Commit: `feat(database): design normalized Chai SQL schema replacing Pebble manual indexing`
- Files created:
  - `internal/database/schema.sql` (589 lines)
  - `internal/database/migration.go` (223 lines)
