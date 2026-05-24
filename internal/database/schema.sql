-- Chai SQL Schema for Audiobook Organizer
-- Version: 1.0.0
-- Migrates from Pebble manual indexing (9,300 lines) to normalized SQL (70%+ reduction)
--
-- Primary design goals:
-- 1. Normalize Book/Author/Series relationships (no denormalized indexes)
-- 2. Index frequently queried columns (series_id, author_id, is_primary_version, marked_for_deletion)
-- 3. Support all current Book/Author/Series/BookFile fields
-- 4. Enable atomic transactions (no more manual dual-write index sync)
-- 5. Maintain reversible schema (data can be exported back to Pebble if needed)

-- Authors table
-- Maps to Author struct; used by series and books
CREATE TABLE authors (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create index on author name for lookups (replaces author:name:* prefix)
CREATE INDEX idx_authors_name ON authors(name);

-- Series table
-- Maps to Series struct; may have author_id
CREATE TABLE series (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    author_id INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (author_id) REFERENCES authors(id)
);

-- Create index on series name (replaces series:name:* prefix)
-- Composite index on (author_id, name) for author-specific series lookups
CREATE INDEX idx_series_name ON series(name);
CREATE INDEX idx_series_author_name ON series(author_id, name);

-- Books table (primary content entity)
-- Maps to Book struct; all fields except computed/joined fields (Author, Authors, Series, MetadataProvenance)
CREATE TABLE books (
    -- Core identity
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    author_id INTEGER,
    series_id INTEGER,
    series_sequence INTEGER,

    -- File reference
    file_path TEXT NOT NULL UNIQUE,
    format TEXT,

    -- Basic metadata
    duration INTEGER,
    work_id TEXT,
    narrator TEXT,
    edition TEXT,
    description TEXT,
    language TEXT,
    publisher TEXT,
    genre TEXT,
    print_year INTEGER,
    audiobook_release_year INTEGER,

    -- ISBN and identifiers
    isbn10 TEXT,
    isbn13 TEXT,
    asin TEXT,

    -- External provider IDs
    open_library_id TEXT,
    hardcover_id TEXT,
    google_books_id TEXT,

    -- iTunes import fields
    itunes_persistent_id TEXT,
    itunes_date_added TIMESTAMP,
    itunes_play_count INTEGER,
    itunes_last_played TIMESTAMP,
    itunes_rating INTEGER,
    itunes_bookmark INTEGER,
    itunes_import_source TEXT,
    itunes_path TEXT,

    -- Original filename and media info
    original_filename TEXT,
    bitrate_kbps INTEGER,
    codec TEXT,
    sample_rate_hz INTEGER,
    channels INTEGER,
    bit_depth INTEGER,
    quality TEXT,

    -- Version management (is_primary_version is critical for filtering)
    is_primary_version BOOLEAN DEFAULT true,
    version_group_id TEXT,
    version_notes TEXT,

    -- File hash tracking for deduplication
    file_hash TEXT,
    file_size INTEGER,
    original_file_hash TEXT,
    organized_file_hash TEXT,

    -- Lifecycle tracking
    library_state TEXT,
    quantity INTEGER,
    marked_for_deletion BOOLEAN DEFAULT false,
    marked_for_deletion_at TIMESTAMP,
    quarantine_reason TEXT,
    quarantined_at TIMESTAMP,

    -- Timestamps (system level)
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    metadata_updated_at TIMESTAMP,
    last_written_at TIMESTAMP,

    -- Organization tracking
    last_organize_operation_id TEXT,
    last_organized_at TIMESTAMP,

    -- Metadata review and source tracking
    metadata_review_status TEXT,
    metadata_source TEXT,

    -- Book signature (unified fingerprint spec)
    book_sig_v1 TEXT,
    book_sig_segments INTEGER,
    book_sig_built_at TIMESTAMP,
    book_sig_v1_mask TEXT,
    book_sig_coverage_pct INTEGER,

    -- iTunes sync status
    itunes_sync_status TEXT,

    -- Audible runtime (minutes)
    audible_runtime_min INTEGER,

    -- Metadata source hash for dedup detection
    metadata_source_hash TEXT,

    -- Merged version tracking
    merged_into_book_id TEXT,

    -- Audible ratings (1–5 scale)
    audible_rating_overall REAL,
    audible_rating_performance REAL,
    audible_rating_story REAL,
    audible_rating_count INTEGER,
    audible_num_reviews INTEGER,

    -- Google Books rating
    google_rating_average REAL,
    google_rating_count INTEGER,

    -- User personal ratings
    user_rating_overall REAL,
    user_rating_story REAL,
    user_rating_performance REAL,
    user_rating_notes TEXT,

    -- Cover art
    cover_url TEXT,

    -- Narrators JSON (denormalized, see book_narrators table for normalization)
    narrators_json TEXT,

    -- Source import path (set on CreateBook only, never mutated)
    source_import_path TEXT,

    -- Scan cache (set by scanner, not user-facing)
    last_scan_mtime INTEGER,
    last_scan_size INTEGER,
    needs_rescan BOOLEAN DEFAULT false,

    -- Foreign keys
    FOREIGN KEY (author_id) REFERENCES authors(id),
    FOREIGN KEY (series_id) REFERENCES series(id)
);

-- Indexes on frequently filtered columns
-- These replace manual "book:series" and "book:author" index prefixes
CREATE INDEX idx_books_series_id ON books(series_id);
CREATE INDEX idx_books_author_id ON books(author_id);
CREATE INDEX idx_books_is_primary_version ON books(is_primary_version);
CREATE INDEX idx_books_marked_for_deletion ON books(marked_for_deletion);
CREATE INDEX idx_books_version_group_id ON books(version_group_id);
CREATE INDEX idx_books_file_path ON books(file_path);

-- Composite index for common filtering patterns
-- Used by GetAllBooks, GetBooksBySeriesID, GetBooksByAuthorID with WHERE clauses
CREATE INDEX idx_books_primary_not_deleted ON books(is_primary_version, marked_for_deletion);

-- Book-Author many-to-many relationship
-- Replaces manual "book:author" denormalized index storage
CREATE TABLE book_authors (
    book_id TEXT NOT NULL,
    author_id INTEGER NOT NULL,
    role TEXT DEFAULT 'author',
    position INTEGER DEFAULT 0,

    PRIMARY KEY (book_id, author_id),
    FOREIGN KEY (book_id) REFERENCES books(id),
    FOREIGN KEY (author_id) REFERENCES authors(id)
);

-- Index for reverse lookups (find all books by author)
CREATE INDEX idx_book_authors_author_id ON book_authors(author_id);

-- BookFile table (one-to-many with books)
-- Maps to BookFile struct; stores file-level metadata and fingerprints
CREATE TABLE book_files (
    id TEXT PRIMARY KEY,
    book_id TEXT NOT NULL,
    version_id TEXT,

    -- File identity
    file_path TEXT NOT NULL,
    original_filename TEXT,
    itunes_path TEXT,
    itunes_persistent_id TEXT,

    -- Track/disc numbering
    track_number INTEGER,
    track_count INTEGER,
    disc_number INTEGER,
    disc_count INTEGER,
    title TEXT,

    -- Audio format/codec info
    format TEXT,
    codec TEXT,
    duration INTEGER,
    file_size INTEGER,
    bitrate_kbps INTEGER,
    sample_rate_hz INTEGER,
    channels INTEGER,
    bit_depth INTEGER,

    -- File hash tracking
    file_hash TEXT,
    original_file_hash TEXT,
    post_metadata_hash TEXT,

    -- Acoustic fingerprint segments (7 segments: intro, body x5, outro)
    acoustid_seg0 TEXT,
    acoustid_seg1 TEXT,
    acoustid_seg2 TEXT,
    acoustid_seg3 TEXT,
    acoustid_seg4 TEXT,
    acoustid_seg5 TEXT,
    acoustid_seg6 TEXT,

    -- Fingerprinting failure tracking
    fingerprint_failed_at TIMESTAMP,
    fingerprint_failure_reason TEXT,
    fingerprint_failure_detail TEXT,
    fingerprint_diagnostic_json TEXT,

    -- Organization method
    organize_method TEXT,

    -- File state
    missing BOOLEAN DEFAULT false,
    skip_scan BOOLEAN DEFAULT false,

    -- Timestamps
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- Deluge integration
    deluge_hash TEXT,
    deluge_original_path TEXT,
    imported_from_deluge_at TIMESTAMP,

    FOREIGN KEY (book_id) REFERENCES books(id)
);

-- Indexes for common file queries
CREATE INDEX idx_book_files_book_id ON book_files(book_id);
CREATE INDEX idx_book_files_file_hash ON book_files(file_hash);
CREATE INDEX idx_book_files_missing ON book_files(missing);

-- BookNarrator many-to-many (normalized narrator tracking)
-- Used by GetNarrators*, UpdateNarrators* functions
CREATE TABLE narrators (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE book_narrators (
    book_id TEXT NOT NULL,
    narrator_id INTEGER NOT NULL,
    role TEXT DEFAULT 'narrator',
    position INTEGER DEFAULT 0,

    PRIMARY KEY (book_id, narrator_id),
    FOREIGN KEY (book_id) REFERENCES books(id),
    FOREIGN KEY (narrator_id) REFERENCES narrators(id)
);

CREATE INDEX idx_book_narrators_narrator_id ON book_narrators(narrator_id);

-- UserPreference table (key-value preferences)
-- Maps to UserPreference struct
CREATE TABLE user_preferences (
    id INTEGER PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    value TEXT,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_user_preferences_key ON user_preferences(key);

-- DoNotImport/BlockedHash table (prevent re-import of files)
-- Maps to DoNotImport struct
CREATE TABLE blocked_hashes (
    hash TEXT PRIMARY KEY,
    reason TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- ImportPath table (monitored import directories)
-- Maps to ImportPath struct
CREATE TABLE import_paths (
    id INTEGER PRIMARY KEY,
    path TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_scan TIMESTAMP,
    book_count INTEGER DEFAULT 0
);

CREATE INDEX idx_import_paths_path ON import_paths(path);
CREATE INDEX idx_import_paths_enabled ON import_paths(enabled);

-- BookSegment table (for chapter/track information)
-- Maps to BookSegment struct; relates to BookFile and User positions
CREATE TABLE book_segments (
    id TEXT PRIMARY KEY,
    book_id TEXT NOT NULL,
    file_id TEXT,
    sequence_number INTEGER,
    title TEXT,
    duration_seconds INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (book_id) REFERENCES books(id),
    FOREIGN KEY (file_id) REFERENCES book_files(id)
);

CREATE INDEX idx_book_segments_book_id ON book_segments(book_id);

-- User table (application users, ULID IDs)
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash_algo TEXT,
    password_hash TEXT,
    roles TEXT,
    status TEXT DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    version INTEGER DEFAULT 0
);

CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_email ON users(email);

-- UserPosition table (playback resume points)
-- Maps to UserPosition struct
CREATE TABLE user_positions (
    user_id TEXT NOT NULL,
    book_id TEXT NOT NULL,
    segment_id TEXT NOT NULL,
    position_seconds REAL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (user_id, book_id, segment_id),
    FOREIGN KEY (user_id) REFERENCES users(id),
    FOREIGN KEY (book_id) REFERENCES books(id),
    FOREIGN KEY (segment_id) REFERENCES book_segments(id)
);

CREATE INDEX idx_user_positions_user_id ON user_positions(user_id);

-- BookVersion table (library centralization, spec 3.1)
-- Maps to BookVersion struct
CREATE TABLE book_versions (
    id TEXT PRIMARY KEY,
    book_id TEXT NOT NULL,
    status TEXT DEFAULT 'active',
    format TEXT,
    source TEXT,
    source_original_path TEXT,
    torrent_hash TEXT,
    ingest_date TIMESTAMP,
    purged_date TIMESTAMP,
    metadata_json TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    version INTEGER DEFAULT 0,

    FOREIGN KEY (book_id) REFERENCES books(id)
);

CREATE INDEX idx_book_versions_book_id ON book_versions(book_id);
CREATE INDEX idx_book_versions_status ON book_versions(status);

-- Role table (multi-user access control, spec 3.7)
CREATE TABLE roles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    permissions TEXT,
    is_seed BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    version INTEGER DEFAULT 0
);

-- APIKey table (bearer tokens for users)
CREATE TABLE api_keys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    status TEXT DEFAULT 'active',
    expires_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,

    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);

-- Invite table (for inviting new users)
CREATE TABLE invites (
    id TEXT PRIMARY KEY,
    created_by_user_id TEXT NOT NULL,
    email TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    role_id TEXT,
    expires_at TIMESTAMP,
    redeemed_at TIMESTAMP,
    redeemed_by_user_id TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (created_by_user_id) REFERENCES users(id),
    FOREIGN KEY (role_id) REFERENCES roles(id),
    FOREIGN KEY (redeemed_by_user_id) REFERENCES users(id)
);

-- Playlist table (auto-generated series playlists, legacy)
CREATE TABLE playlists (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    series_id INTEGER,
    file_path TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (series_id) REFERENCES series(id)
);

-- UserPlaylist table (user-created playlists, spec 3.4)
CREATE TABLE user_playlists (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    type TEXT DEFAULT 'static',
    book_ids TEXT,
    query TEXT,
    sort_json TEXT,
    limit_value INTEGER,
    materialized_book_ids TEXT,
    itunes_persistent_id TEXT,
    itunes_raw_criteria_b64 TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by_user_id TEXT,
    dirty BOOLEAN DEFAULT false,
    version INTEGER DEFAULT 0,

    FOREIGN KEY (created_by_user_id) REFERENCES users(id)
);

-- PlaylistItem table (legacy playlist items)
CREATE TABLE playlist_items (
    id INTEGER PRIMARY KEY,
    playlist_id INTEGER NOT NULL,
    book_id TEXT NOT NULL,
    position INTEGER,

    FOREIGN KEY (playlist_id) REFERENCES playlists(id),
    FOREIGN KEY (book_id) REFERENCES books(id)
);

-- Operation table (async operations)
CREATE TABLE operations (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    status TEXT DEFAULT 'pending',
    priority INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    error_message TEXT
);

-- OperationLog table (audit trail)
CREATE TABLE operation_logs (
    id TEXT PRIMARY KEY,
    operation_id TEXT NOT NULL,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    message TEXT,
    level TEXT DEFAULT 'info',

    FOREIGN KEY (operation_id) REFERENCES operations(id)
);

CREATE INDEX idx_operation_logs_operation_id ON operation_logs(operation_id);

-- Work table (title-level grouping across editions/narrations)
CREATE TABLE works (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    author_id INTEGER,
    series_id INTEGER,
    alt_titles TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (author_id) REFERENCES authors(id),
    FOREIGN KEY (series_id) REFERENCES series(id)
);

-- BookAlternativeTitle table (dedup layer 1)
CREATE TABLE book_alternative_titles (
    id INTEGER PRIMARY KEY,
    book_id TEXT NOT NULL,
    title TEXT NOT NULL,
    source TEXT,
    language TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (book_id) REFERENCES books(id)
);

CREATE INDEX idx_book_alternative_titles_book_id ON book_alternative_titles(book_id);
CREATE INDEX idx_book_alternative_titles_title ON book_alternative_titles(title);

-- AuthorAlias table (pen names, handles, alternative names)
CREATE TABLE author_aliases (
    id INTEGER PRIMARY KEY,
    author_id INTEGER NOT NULL,
    alias_name TEXT NOT NULL,
    alias_type TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (author_id) REFERENCES authors(id)
);

CREATE INDEX idx_author_aliases_author_id ON author_aliases(author_id);
CREATE INDEX idx_author_aliases_alias_name ON author_aliases(alias_name);
