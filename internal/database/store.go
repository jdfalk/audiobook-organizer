// file: internal/database/store.go
// version: 2.56.0
// guid: 8a9b0c1d-2e3f-4a5b-6c7d-8e9f0a1b2c3d

package database

import (
	"fmt"
	"sync"
	"time"
)

// Store defines the full database surface. Most services should depend
// on a narrower sub-interface defined in iface_*.go; Store itself is
// used by the server bootstrap and test fixtures that genuinely need
// wide access. See docs/superpowers/specs/2026-04-17-store-interface-segregation-design.md.
type Store interface {
	LifecycleStore
	BookStore
	AuthorStore
	SeriesStore
	UserStore
	NarratorStore
	WorkStore
	SessionStore
	RoleStore
	APIKeyStore
	InviteStore
	UserPreferenceStore
	UserPositionStore
	BookVersionStore
	BookFileStore
	BookSegmentStore
	PlaylistStore
	UserPlaylistStore
	ImportPathStore
	OperationStore
	TagStore
	UserTagStore
	MetadataStore
	HashBlocklistStore
	ITunesStateStore
	PathHistoryStore
	ExternalIDStore
	RawKVStore
	PlaybackStore
	SettingsStore
	StatsStore
	MaintenanceStore
	SystemActivityStore
}

// BookAlternativeTitle represents a variant name for a book — romaji
// vs English, ampersand vs "and", subtitle reordering, rebrands, etc.
// Used by the dedup engine's Layer 1 exact title matching and by
// library search so either form finds the book.
type BookAlternativeTitle struct {
	ID        int64     `json:"id"`
	BookID    string    `json:"book_id"`
	Title     string    `json:"title"`
	Source    string    `json:"source"`   // "user", "metadata_fetch", "auto_ampersand", etc.
	Language  string    `json:"language,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Common data structures used by all store implementations
// Note: These are defined here instead of in web.go to avoid circular dependencies

// Author represents an audiobook author
type Author struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// AuthorAlias represents a pen name, handle, or alternative name for an author
type AuthorAlias struct {
	ID        int       `json:"id"`
	AuthorID  int       `json:"author_id"`
	AliasName string    `json:"alias_name"`
	AliasType string    `json:"alias_type"` // pen_name, real_name, handle, abbreviation, alias
	CreatedAt time.Time `json:"created_at"`
}

// BookAuthor represents the many-to-many relationship between books and authors
type BookAuthor struct {
	BookID   string `json:"book_id"`
	AuthorID int    `json:"author_id"`
	Role     string `json:"role"`     // author, co-author, editor
	Position int    `json:"position"` // 0 = primary
}

// Narrator represents an audiobook narrator
type Narrator struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// BookNarrator represents the many-to-many relationship between books and narrators
type BookNarrator struct {
	BookID     string `json:"book_id"`
	NarratorID int    `json:"narrator_id"`
	Role       string `json:"role"`     // narrator, co-narrator
	Position   int    `json:"position"` // 0 = primary
}

// Series represents an audiobook series
type Series struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	AuthorID *int   `json:"author_id,omitempty"`
}

// Book represents an audiobook
type Book struct {
	ID             string `json:"id"` // ULID format
	Title          string `json:"title"`
	AuthorID       *int   `json:"author_id,omitempty"`
	SeriesID       *int   `json:"series_id,omitempty"`
	SeriesSequence *int   `json:"series_sequence,omitempty"`
	FilePath       string `json:"file_path"`
	Format         string `json:"format,omitempty"`
	Duration       *int   `json:"duration,omitempty"`
	// Extended metadata (optional)
	WorkID               *string `json:"work_id,omitempty"`
	Narrator             *string `json:"narrator,omitempty"`
	Edition              *string `json:"edition,omitempty"`
	Description          *string `json:"description,omitempty"`
	Language             *string `json:"language,omitempty"`
	Publisher            *string `json:"publisher,omitempty"`
	Genre                *string `json:"genre,omitempty"`
	PrintYear            *int    `json:"print_year,omitempty"`
	AudiobookReleaseYear *int    `json:"audiobook_release_year,omitempty"`
	ISBN10               *string `json:"isbn10,omitempty"`
	ISBN13               *string `json:"isbn13,omitempty"`
	ASIN                 *string `json:"asin,omitempty"`
	// External provider IDs
	OpenLibraryID *string `json:"open_library_id,omitempty"`
	HardcoverID   *string `json:"hardcover_id,omitempty"`
	GoogleBooksID *string `json:"google_books_id,omitempty"`
	// iTunes import fields
	ITunesPersistentID *string    `json:"itunes_persistent_id,omitempty"`
	ITunesDateAdded    *time.Time `json:"itunes_date_added,omitempty"`
	ITunesPlayCount    *int       `json:"itunes_play_count,omitempty"`
	ITunesLastPlayed   *time.Time `json:"itunes_last_played,omitempty"`
	ITunesRating       *int       `json:"itunes_rating,omitempty"`
	ITunesBookmark     *int64     `json:"itunes_bookmark,omitempty"`
	ITunesImportSource *string    `json:"itunes_import_source,omitempty"`
	// Deprecated: use book_files.itunes_path instead. Will be removed in a future migration.
	ITunesPath         *string    `json:"itunes_path,omitempty"`
	OriginalFilename   *string    `json:"original_filename,omitempty"`
	// Media info fields
	Bitrate    *int    `json:"bitrate_kbps,omitempty"`
	Codec      *string `json:"codec,omitempty"`
	SampleRate *int    `json:"sample_rate_hz,omitempty"`
	Channels   *int    `json:"channels,omitempty"`
	BitDepth   *int    `json:"bit_depth,omitempty"`
	Quality    *string `json:"quality,omitempty"`
	// Version management
	IsPrimaryVersion *bool   `json:"is_primary_version,omitempty"`
	VersionGroupID   *string `json:"version_group_id,omitempty"`
	VersionNotes     *string `json:"version_notes,omitempty"`
	// File hash tracking for deduplication
	FileHash *string `json:"file_hash,omitempty"`
	FileSize *int64  `json:"file_size,omitempty"`
	// Lifecycle tracking
	OriginalFileHash    *string    `json:"original_file_hash,omitempty"`
	OrganizedFileHash   *string    `json:"organized_file_hash,omitempty"`
	LibraryState        *string    `json:"library_state,omitempty"`
	Quantity            *int       `json:"quantity,omitempty"`
	MarkedForDeletion   *bool      `json:"marked_for_deletion,omitempty"`
	MarkedForDeletionAt *time.Time `json:"marked_for_deletion_at,omitempty"`
	CreatedAt           *time.Time `json:"created_at,omitempty"`
	// UpdatedAt is set on every DB write (system-level).
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	// MetadataUpdatedAt is set only when user-visible metadata fields change.
	MetadataUpdatedAt *time.Time `json:"metadata_updated_at,omitempty"`
	// LastWrittenAt is set when metadata is written back to the audio files on disk.
	LastWrittenAt *time.Time `json:"last_written_at,omitempty"`
	// LastOrganizeOperationID is the operation ID of the last organize run that processed this book.
	LastOrganizeOperationID *string `json:"last_organize_operation_id,omitempty"`
	// LastOrganizedAt is when this book was last stamped by an organize run (organized, re-organized, or confirmed correct).
	LastOrganizedAt *time.Time `json:"last_organized_at,omitempty"`
	// MetadataReviewStatus tracks manual metadata matching: null, "no_match", "matched".
	MetadataReviewStatus *string `json:"metadata_review_status,omitempty"`
	// ITunesSyncStatus tracks whether this book's metadata is in sync with the iTunes library.
	// Values: "synced" (up-to-date in ITL), "dirty" (changed since last write-back),
	// "unlinked" (no iTunes presence), "pending" (new, needs adding to iTunes).
	ITunesSyncStatus *string `json:"itunes_sync_status,omitempty"`
	// Cover art
	CoverURL *string `json:"cover_url,omitempty"`
	// Narrators as JSON array
	NarratorsJSON *string `json:"narrators_json,omitempty"`
	// Scan cache for incremental scanning (set by scanner, not user-facing)
	LastScanMtime *int64 `json:"last_scan_mtime,omitempty"`
	LastScanSize  *int64 `json:"last_scan_size,omitempty"`
	NeedsRescan   *bool  `json:"needs_rescan,omitempty"`
	// Related objects (populated via joins, not stored in DB)
	Author               *Author                            `json:"author,omitempty" db:"-"`
	Authors              []BookAuthor                       `json:"authors,omitempty" db:"-"`
	Series               *Series                            `json:"series,omitempty" db:"-"`
	MetadataProvenance   map[string]MetadataProvenanceEntry `json:"metadata_provenance,omitempty" db:"-"`
	MetadataProvenanceAt *time.Time                         `json:"metadata_provenance_at,omitempty" db:"-"`
}

// MetadataProvenanceEntry represents the source breakdown for a metadata field.
type MetadataProvenanceEntry struct {
	FileValue       interface{} `json:"file_value,omitempty"`
	FetchedValue    interface{} `json:"fetched_value,omitempty"`
	StoredValue     interface{} `json:"stored_value,omitempty"`
	OverrideValue   interface{} `json:"override_value,omitempty"`
	OverrideLocked  bool        `json:"override_locked"`
	EffectiveValue  interface{} `json:"effective_value,omitempty"`
	EffectiveSource string      `json:"effective_source,omitempty"`
	ComparisonValue interface{} `json:"comparison_value,omitempty"`
	UpdatedAt       *time.Time  `json:"updated_at,omitempty"`
}

// Work represents a logical title-level grouping that may span multiple editions,
// narrations, languages, or publishers. Books can optionally reference a Work via WorkID.
type Work struct {
	ID        string   `json:"id"`    // ULID format
	Title     string   `json:"title"` // Canonical title
	AuthorID  *int     `json:"author_id,omitempty"`
	SeriesID  *int     `json:"series_id,omitempty"`
	AltTitles []string `json:"alt_titles,omitempty"` // Optional alternate titles
}

// Playlist represents an auto-generated series playlist (the old
// M3U-style playlist generator). For the 3.4 user-facing playlist
// feature, see UserPlaylist below.
type Playlist struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	SeriesID *int   `json:"series_id,omitempty"`
	FilePath string `json:"file_path"`
}

// UserPlaylist represents a user-created playlist (spec 3.4) —
// either a static ordered book list or a smart (live-evaluated)
// filter expression. Distinct from the auto-generated Playlist
// type above which is part of the older series-playlist generator.
type UserPlaylist struct {
	ID          string `json:"id"` // ULID
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Type discriminator: "static" or "smart"
	Type string `json:"type"`
	// Static playlists: ordered book ID list.
	BookIDs []string `json:"book_ids,omitempty"`
	// Smart playlists: DSL query string, sort + limit directives.
	Query string `json:"query,omitempty"`
	// SortJSON is a JSON-encoded []{field, direction} for stable
	// ordering in smart playlists.
	SortJSON string `json:"sort_json,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	// MaterializedBookIDs caches the last evaluation of a smart
	// playlist for fast iTunes-sync pushes without re-running the
	// query. Refreshed on every sync pass.
	MaterializedBookIDs []string `json:"materialized_book_ids,omitempty"`
	// ITunesPersistentID links the playlist to its iTunes row
	// once it's been pushed. Null until first sync.
	ITunesPersistentID string `json:"itunes_persistent_id,omitempty"`
	// ITunesRawCriteriaB64 stores the original iTunes Smart
	// Criteria blob for playlists imported from iTunes (migration
	// audit trail). Empty for app-native playlists.
	ITunesRawCriteriaB64 string `json:"itunes_raw_criteria_b64,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	CreatedByUserID      string    `json:"created_by_user_id,omitempty"`
	// Dirty marks the playlist as pending iTunes push.
	Dirty   bool `json:"dirty"`
	Version int  `json:"version"`
}

// UserPlaylist type constants.
const (
	UserPlaylistTypeStatic = "static"
	UserPlaylistTypeSmart  = "smart"
)

// PlaylistItem represents an item in a playlist
type PlaylistItem struct {
	ID         int `json:"id"`
	PlaylistID int `json:"playlist_id"`
	BookID     int `json:"book_id"`
	Position   int `json:"position"`
}

// ImportPath represents a managed import path monitored for new audiobooks
type ImportPath struct {
	ID        int        `json:"id"`
	Path      string     `json:"path"`
	Name      string     `json:"name"`
	Enabled   bool       `json:"enabled"`
	CreatedAt time.Time  `json:"created_at"`
	LastScan  *time.Time `json:"last_scan,omitempty"`
	BookCount int        `json:"book_count"`
}

// Operation represents an async operation
type Operation struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id,omitempty"`
	Type         string     `json:"type"`
	Status       string     `json:"status"`
	Progress     int        `json:"progress"`
	Total        int        `json:"total"`
	Message      string     `json:"message"`
	FolderPath   *string    `json:"folder_path,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	ResultData   *string    `json:"result_data,omitempty"`
}

// OperationLog represents a log entry for an operation
type OperationLog struct {
	ID          int       `json:"id"`
	OperationID string    `json:"operation_id"`
	Level       string    `json:"level"`
	Message     string    `json:"message"`
	Details     *string   `json:"details,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// OperationChange tracks a single destructive change made during an operation for undo support.
type OperationChange struct {
	ID          string     `json:"id"`
	OperationID string     `json:"operation_id"`
	UserID      string     `json:"user_id,omitempty"`
	BookID      string     `json:"book_id"`
	ChangeType  string     `json:"change_type"`  // "file_move", "metadata_update", "tag_write"
	FieldName   string     `json:"field_name"`
	OldValue    string     `json:"old_value"`
	NewValue    string     `json:"new_value"`
	RevertedAt  *time.Time `json:"reverted_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// SystemActivityLog represents a log entry from a housekeeping goroutine.
type SystemActivityLog struct {
	ID        int       `json:"id"`
	UserID    string    `json:"user_id,omitempty"`
	Source    string    `json:"source"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// OperationSummaryLog represents a completed operation persisted for history across restarts.
type OperationSummaryLog struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Status      string     `json:"status"`
	Progress    float64    `json:"progress"`
	Result      *string    `json:"result,omitempty"`       // JSON-encoded result
	Error       *string    `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// OperationResult holds structured per-book output for a bulk operation.
type OperationResult struct {
	ID          int       `json:"id"`
	OperationID string    `json:"operation_id"`
	BookID      string    `json:"book_id"`
	ResultJSON  string    `json:"result_json"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// UserPreference represents a user preference setting
type UserPreference struct {
	ID        int       `json:"id"`
	Key       string    `json:"key"`
	Value     *string   `json:"value,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DoNotImport represents a blocked file hash to prevent reimport
type DoNotImport struct {
	Hash      string    `json:"hash"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// LibraryFingerprintRecord stores the last-known state of an iTunes Library.xml file.
type LibraryFingerprintRecord struct {
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"mod_time"`
	CRC32     uint32    `json:"crc32"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Extended models (Pebble only; SQLite may ignore)

// User represents an application user (ULID IDs)
type User struct {
	ID               string    `json:"id"`
	Username         string    `json:"username"`
	Email            string    `json:"email"`
	PasswordHashAlgo string    `json:"password_hash_algo"`
	PasswordHash     string    `json:"password_hash"`
	Roles            []string  `json:"roles"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Version          int       `json:"version"`
}

// BookVersion represents one version of a book's content (spec 3.1
// library centralization). Every book has at least one version with
// status = active; alt versions (different format, quality, source)
// live under `.versions/{version_id}/` on the filesystem. File-level
// data (paths, hashes, sizes) lives on BookFile rows scoped by
// version_id; this struct carries only version-level metadata.
type BookVersion struct {
	ID                 string    `json:"id"`
	BookID             string    `json:"book_id"`
	Status             string    `json:"status"` // see BookVersionStatus* constants below
	Format             string    `json:"format"` // m4b | mp3 | flac | ...
	Source             string    `json:"source"` // deluge | manual | transcoded | imported
	SourceOriginalPath string    `json:"source_original_path,omitempty"`
	TorrentHash        string    `json:"torrent_hash,omitempty"` // infohash, fast-path fingerprint match
	IngestDate         time.Time `json:"ingest_date"`
	PurgedDate         *time.Time `json:"purged_date,omitempty"`
	MetadataJSON       string    `json:"metadata_json,omitempty"` // source-specific catch-all
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Version            int       `json:"version"`
}

// BookVersionStatus values. See spec 3.1 for the full state
// machine — the transitions users trigger (trash/restore, swap
// primary, hard-delete from Purged view) are enforced in the
// server layer, not in the store.
const (
	BookVersionStatusPending              = "pending"
	BookVersionStatusActive               = "active"
	BookVersionStatusAlt                  = "alt"
	BookVersionStatusSwappingIn           = "swapping_in"
	BookVersionStatusSwappingOut          = "swapping_out"
	BookVersionStatusTrash                = "trash"
	BookVersionStatusInactivePurged       = "inactive_purged"
	BookVersionStatusBlockedForRedownload = "blocked_for_redownload"
)

// Role is a named bundle of permissions for the multi-user model
// (spec 3.7). Permissions are Go string constants validated at
// route-registration time, carried inline on the Role to avoid a
// separate role_permissions join table. ID is typically the
// lowercase role name (e.g. "admin", "editor", "viewer") so seeded
// roles have stable, well-known IDs; custom roles get a ULID.
type Role struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Permissions []string  `json:"permissions"`
	IsSeed      bool      `json:"is_seed,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Version     int       `json:"version"`
}

// APIKey is a personal bearer token (JWT jti) for a user (spec 3.7).
// The JWT itself carries the ID as `jti`; verification loads this
// row to check RevokedAt. Stored separately from Session so rotating
// API keys doesn't require a re-login.
type APIKey struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// Invite is a single-use admin-generated token for creating a new
// user account (spec 3.7). Token is the PK since lookup is always
// by the token-in-URL the invitee opens. ConsumeInvite is atomic:
// invite row deleted + user created + role membership written in
// one Pebble batch.
type Invite struct {
	Token           string     `json:"token"`
	Username        string     `json:"username"`
	Email           string     `json:"email,omitempty"`
	RoleID          string     `json:"role_id"`
	CreatedByUserID string     `json:"created_by_user_id"`
	CreatedAt       time.Time  `json:"created_at"`
	ExpiresAt       time.Time  `json:"expires_at"`
	UsedAt          *time.Time `json:"used_at,omitempty"`
}

// Session represents an authenticated session token
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	Revoked   bool      `json:"revoked"`
	Version   int       `json:"version"`
}

// UserPreferenceKV represents per-user preference (key/value)
type UserPreferenceKV struct {
	UserID    string    `json:"user_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int       `json:"version"`
}

// BookSegment represents a physical file segment of a book
type BookSegment struct {
	ID           string    `json:"id"`
	BookID       int       `json:"book_id"` // link to numeric book ID (legacy)
	FilePath     string    `json:"file_path"`
	Format       string    `json:"format"`
	SizeBytes    int64     `json:"size_bytes"`
	DurationSec  int       `json:"duration_seconds"`
	TrackNumber  *int      `json:"track_number,omitempty"`
	TotalTracks  *int      `json:"total_tracks,omitempty"`
	SegmentTitle *string   `json:"segment_title,omitempty"`
	FileHash     *string   `json:"file_hash,omitempty"`
	Active       bool      `json:"active"`
	SupersededBy *string   `json:"superseded_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Version      int       `json:"version"`
}

// BookFile represents an individual audio file within a book.
// Replaces BookSegment with ULID string book_id, iTunes integration fields,
// and comprehensive audio metadata.
type BookFile struct {
	ID                 string    `json:"id"`
	BookID             string    `json:"book_id"`
	// VersionID ties this file to a specific book_versions row when
	// library centralization (spec 3.1) is rolled out. NULL until
	// migration runs; after migration, every file belongs to exactly
	// one version. Cascaded delete with book_versions.
	VersionID          string    `json:"version_id,omitempty"`
	FilePath           string    `json:"file_path"`
	OriginalFilename   string    `json:"original_filename,omitempty"`
	ITunesPath         string    `json:"itunes_path,omitempty"`
	ITunesPersistentID string    `json:"itunes_persistent_id,omitempty"`
	TrackNumber        int       `json:"track_number,omitempty"`
	TrackCount         int       `json:"track_count,omitempty"`
	DiscNumber         int       `json:"disc_number,omitempty"`
	DiscCount          int       `json:"disc_count,omitempty"`
	Title              string    `json:"title,omitempty"`
	Format             string    `json:"format,omitempty"`
	Codec              string    `json:"codec,omitempty"`
	Duration           int       `json:"duration,omitempty"`
	FileSize           int64     `json:"file_size,omitempty"`
	BitrateKbps        int       `json:"bitrate_kbps,omitempty"`
	SampleRateHz       int       `json:"sample_rate_hz,omitempty"`
	Channels           int       `json:"channels,omitempty"`
	BitDepth           int       `json:"bit_depth,omitempty"`
	FileHash           string    `json:"file_hash,omitempty"`
	OriginalFileHash   string    `json:"original_file_hash,omitempty"`
	OrganizeMethod     string    `json:"organize_method,omitempty"` // "reflink", "hardlink", "copy", "symlink"
	Missing            bool      `json:"missing"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// UserPosition is one user's resume point for one segment of one
// book (spec 3.6). Keyed per (user, book, segment) — a multi-file
// audiobook has one row per chapter/segment actively listened to.
// The canonical "where am I in the book" is derived from the most
// recently-updated UserPosition for the book.
type UserPosition struct {
	UserID          string    `json:"user_id"`
	BookID          string    `json:"book_id"`
	SegmentID       string    `json:"segment_id"`
	PositionSeconds float64   `json:"position_seconds"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// UserBookState is the per-(user, book) derived state used by UI and
// filters (spec 3.6): read status, last activity timestamp, resume
// pointer, cached listened-seconds and progress percent. Recomputed
// when positions change; can be manually overridden (status_manual).
type UserBookState struct {
	UserID                string    `json:"user_id"`
	BookID                string    `json:"book_id"`
	Status                string    `json:"status"` // see UserBookStatus* constants
	StatusManual          bool      `json:"status_manual"`
	LastActivityAt        time.Time `json:"last_activity_at"`
	LastSegmentID         string    `json:"last_segment_id,omitempty"`
	TotalListenedSeconds  float64   `json:"total_listened_seconds,omitempty"`
	ProgressPct           int       `json:"progress_pct"` // 0-100
	UpdatedAt             time.Time `json:"updated_at"`
}

// UserBookStatus values. Auto-computed from UserPosition rows
// unless StatusManual=true, in which case the server leaves the
// stored value alone. See spec 3.6 §6-7 for the state machine.
const (
	UserBookStatusUnstarted  = "unstarted"
	UserBookStatusInProgress = "in_progress"
	UserBookStatusFinished   = "finished"
	UserBookStatusAbandoned  = "abandoned"
)

// FinishedThreshold is the fraction of total duration that triggers
// an auto-flip to UserBookStatusFinished. Spec 3.6 §6.
const FinishedThreshold = 0.95

// PlaybackEvent immutable event
type PlaybackEvent struct {
	UserID      string    `json:"user_id"`
	BookID      int       `json:"book_id"`
	SegmentID   string    `json:"segment_id"`
	PositionSec int       `json:"position_seconds"`
	EventType   string    `json:"event_type"` // progress|start|pause|complete
	PlaySpeed   float64   `json:"play_speed"`
	CreatedAt   time.Time `json:"created_at"`
	Version     int       `json:"version"`
}

// PlaybackProgress latest snapshot
type PlaybackProgress struct {
	UserID      string    `json:"user_id"`
	BookID      int       `json:"book_id"`
	SegmentID   string    `json:"segment_id"`
	PositionSec int       `json:"position_seconds"`
	Percent     float64   `json:"percent_complete"`
	UpdatedAt   time.Time `json:"updated_at"`
	Version     int       `json:"version"`
}

// BookStats aggregated counters
type BookStats struct {
	BookID        int `json:"book_id"`
	PlayCount     int `json:"play_count"`
	ListenSeconds int `json:"listen_seconds"`
	Version       int `json:"version"`
}

// UserStats aggregated counters
type UserStats struct {
	UserID        string `json:"user_id"`
	ListenSeconds int    `json:"listen_seconds"`
	Version       int    `json:"version"`
}

// MetadataFieldState persists per-field metadata provenance (fetched vs override)
// using JSON-encoded values to preserve original types.
type MetadataFieldState struct {
	BookID         string    `json:"book_id"`
	Field          string    `json:"field"`
	FetchedValue   *string   `json:"fetched_value,omitempty"`  // JSON-encoded value
	OverrideValue  *string   `json:"override_value,omitempty"` // JSON-encoded value
	OverrideLocked bool      `json:"override_locked"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// MetadataChangeRecord tracks a single change to a metadata field for undo/audit.
type MetadataChangeRecord struct {
	ID            int       `json:"id"`
	BookID        string    `json:"book_id"`
	Field         string    `json:"field"`
	PreviousValue *string   `json:"previous_value,omitempty"` // JSON-encoded
	NewValue      *string   `json:"new_value,omitempty"`      // JSON-encoded
	ChangeType    string    `json:"change_type"`              // "fetched", "override", "clear", "undo", "bulk_update"
	Source        string    `json:"source,omitempty"`          // e.g. "Open Library", "manual", "AI parsing"
	ChangedAt     time.Time `json:"changed_at"`
}

// BookSnapshot represents an immutable snapshot of a book at a point in time.
type BookSnapshot struct {
	BookID    string    `json:"book_id"`
	Timestamp time.Time `json:"timestamp"`
	Data      []byte    `json:"data"` // Full JSON-serialized Book
}

// DeferredITunesUpdate records a path change that should be applied to the
// iTunes library (.itl) the next time write-back is enabled and a sync runs.
type DeferredITunesUpdate struct {
	ID           int        `json:"id"`
	BookID       string     `json:"book_id"`
	PersistentID string     `json:"persistent_id"`
	OldPath      string     `json:"old_path"`
	NewPath      string     `json:"new_path"`
	UpdateType   string     `json:"update_type"`
	CreatedAt    time.Time  `json:"created_at"`
	AppliedAt    *time.Time `json:"applied_at,omitempty"`
}

// KVPair is a raw key-value pair from the store.
type KVPair struct {
	Key   string
	Value []byte
}

// ExternalIDMapping maps an external identifier (iTunes PID, Audible ASIN, etc.) to a book.
type ExternalIDMapping struct {
	ID          int        `json:"id"`
	Source      string     `json:"source"`
	ExternalID  string     `json:"external_id"`
	BookID      string     `json:"book_id"`
	TrackNumber *int       `json:"track_number,omitempty"`
	FilePath    string     `json:"file_path,omitempty"`
	Tombstoned  bool       `json:"tombstoned"`
	Provenance  string     `json:"provenance,omitempty"`  // "itunes", "generated", "recycled"
	RemovedAt   *time.Time `json:"removed_at,omitempty"`  // when we sent ITL remove; null = live
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// BookPathChange records a file path change (rename/move) for a book.
type BookPathChange struct {
	ID         int       `json:"id"`
	BookID     string    `json:"book_id"`
	OldPath    string    `json:"old_path"`
	NewPath    string    `json:"new_path"`
	ChangeType string    `json:"change_type"`
	CreatedAt  time.Time `json:"created_at"`
}

// BookTag represents one tag row on a book. Source distinguishes
// user-applied tags ("user") from system-applied provenance tags
// ("system", e.g. dedup:merge-survivor:llm-auto, metadata:source:*).
// BookID is populated on reads that need to identify the owning book
// (historically PebbleStore serialized it into the JSON value); for
// the per-book read path it's left empty since the caller already
// has the ID.
type BookTag struct {
	BookID    string    `json:"book_id,omitempty"`
	Tag       string    `json:"tag"`
	Source    string    `json:"source,omitempty"` // "user" | "system"
	CreatedAt time.Time `json:"created_at"`
}

// TagWithCount represents a tag with its usage count.
type TagWithCount struct {
	Tag    string `json:"tag"`
	Count  int    `json:"count"`
	Source string `json:"source,omitempty"` // "user" or "system" — empty when mixed
}

// ScanCacheEntry holds mtime/size for incremental scan skip checks.
type ScanCacheEntry struct {
	Mtime       int64
	Size        int64
	NeedsRescan bool
}

// DashboardStats holds aggregated statistics computed via SQL rather than loading all books.
type DashboardStats struct {
	TotalBooks         int            `json:"total_books"`
	TotalFiles         int            `json:"total_files"`
	TotalAuthors       int            `json:"total_authors"`
	TotalSeries        int            `json:"total_series"`
	TotalDuration      int64          `json:"total_duration"`
	TotalSize          int64          `json:"total_size"`
	StateDistribution  map[string]int `json:"state_distribution"`
	FormatDistribution map[string]int `json:"format_distribution"`
}

// Global store instance — use GetGlobalStore/SetGlobalStore for concurrent access.
// Direct assignment is allowed in single-goroutine contexts (init, main).
var globalStore Store
var globalStoreMu sync.RWMutex

// GetGlobalStore returns the global store with read-lock protection.
func GetGlobalStore() Store {
	globalStoreMu.RLock()
	s := globalStore
	globalStoreMu.RUnlock()
	return s
}

// SetGlobalStore sets the global store with write-lock protection.
func SetGlobalStore(s Store) {
	globalStoreMu.Lock()
	globalStore = s
	globalStoreMu.Unlock()
}

// InitializeStore initializes the database store based on configuration
func InitializeStore(dbType, path string, enableSQLite bool) error {
	var err error

	switch dbType {
	case "sqlite", "sqlite3":
		if !enableSQLite {
			return fmt.Errorf("SQLite3 is not enabled. To use SQLite3, you must explicitly enable it with --enable-sqlite3-i-know-the-risks or set 'enable_sqlite3_i_know_the_risks: true' in your config file. PebbleDB is the recommended database for production use")
		}
		globalStore, err = NewSQLiteStore(path)
		if err != nil {
			return fmt.Errorf("failed to initialize SQLite store: %w", err)
		}
	case "pebble", "":
		// PebbleDB is the default
		globalStore, err = NewPebbleStore(path)
		if err != nil {
			return fmt.Errorf("failed to initialize PebbleDB store: %w", err)
		}
	default:
		return fmt.Errorf("unsupported database type: %s (supported: pebble, sqlite)", dbType)
	}

	// Maintain backwards compatibility with the global DB variable for SQLite
	if sqliteStore, ok := globalStore.(*SQLiteStore); ok {
		DB = sqliteStore.db
	}

	// Run migrations to ensure schema is up to date
	if err := RunMigrations(globalStore); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// CloseStore closes the global store
func CloseStore() error {
	// Grab and nil the global ref first so lingering goroutines
	// see nil and fail gracefully instead of hitting a closed DB.
	store := globalStore
	globalStore = nil

	if store != nil {
		// Brief pause to let in-flight goroutines notice the nil
		time.Sleep(100 * time.Millisecond)
		return store.Close()
	}
	// Backwards compatibility
	if DB != nil {
		return DB.Close()
	}
	return nil
}
