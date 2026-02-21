// file: internal/database/store.go
// version: 2.19.0
// guid: 8a9b0c1d-2e3f-4a5b-6c7d-8e9f0a1b2c3d

package database

import (
	"fmt"
	"time"
)

// Store defines the interface for our database operations
// This abstraction allows us to support both PebbleDB (default) and SQLite3 (opt-in)
type Store interface {
	// Lifecycle
	Close() error
	Reset() error

	// Metadata provenance and overrides
	GetMetadataFieldStates(bookID string) ([]MetadataFieldState, error)
	UpsertMetadataFieldState(state *MetadataFieldState) error
	DeleteMetadataFieldState(bookID, field string) error

	// Authors
	GetAllAuthors() ([]Author, error)
	GetAuthorByID(id int) (*Author, error)
	GetAuthorByName(name string) (*Author, error)
	CreateAuthor(name string) (*Author, error)

	// Book-Author relationships
	GetBookAuthors(bookID string) ([]BookAuthor, error)
	SetBookAuthors(bookID string, authors []BookAuthor) error
	GetBooksByAuthorIDWithRole(authorID int) ([]Book, error)

	// Series
	GetAllSeries() ([]Series, error)
	GetSeriesByID(id int) (*Series, error)
	GetSeriesByName(name string, authorID *int) (*Series, error)
	CreateSeries(name string, authorID *int) (*Series, error)

	// Works (logical title-level grouping across editions/narrations)
	GetAllWorks() ([]Work, error)
	GetWorkByID(id string) (*Work, error) // ULID ID
	CreateWork(work *Work) (*Work, error) // Generates ULID if empty
	UpdateWork(id string, work *Work) (*Work, error)
	DeleteWork(id string) error
	GetBooksByWorkID(workID string) ([]Book, error)

	// Books
	GetAllBooks(limit, offset int) ([]Book, error)
	GetBookByID(id string) (*Book, error) // ID is ULID string
	GetBookByFilePath(path string) (*Book, error)
	GetBookByFileHash(hash string) (*Book, error)
	GetBookByOriginalHash(hash string) (*Book, error)
	GetBookByOrganizedHash(hash string) (*Book, error)
	GetDuplicateBooks() ([][]Book, error) // Returns groups of duplicate books
	GetBooksBySeriesID(seriesID int) ([]Book, error)
	GetBooksByAuthorID(authorID int) ([]Book, error)
	CreateBook(book *Book) (*Book, error)            // Generates ULID if ID is empty
	UpdateBook(id string, book *Book) (*Book, error) // ID is ULID string
	DeleteBook(id string) error                      // ID is ULID string
	SearchBooks(query string, limit, offset int) ([]Book, error)
	CountBooks() (int, error)
	CountAuthors() (int, error)
	CountSeries() (int, error)
	GetBookCountsByLocation(rootDir string) (library, import_ int, err error)
	GetBookSizesByLocation(rootDir string) (librarySize, importSize int64, err error)
	GetDashboardStats() (*DashboardStats, error)
	ListSoftDeletedBooks(limit, offset int, olderThan *time.Time) ([]Book, error)

	// Version Management
	GetBooksByVersionGroup(groupID string) ([]Book, error)

	// Import Paths
	GetAllImportPaths() ([]ImportPath, error)
	GetImportPathByID(id int) (*ImportPath, error)
	GetImportPathByPath(path string) (*ImportPath, error)
	CreateImportPath(path, name string) (*ImportPath, error)
	UpdateImportPath(id int, importPath *ImportPath) error
	DeleteImportPath(id int) error

	// Operations
	CreateOperation(id, opType string, folderPath *string) (*Operation, error)
	GetOperationByID(id string) (*Operation, error)
	GetRecentOperations(limit int) ([]Operation, error)
	UpdateOperationStatus(id, status string, progress, total int, message string) error
	UpdateOperationError(id, errorMessage string) error

	// Operation State Persistence (resumable operations)
	SaveOperationState(opID string, state []byte) error
	GetOperationState(opID string) ([]byte, error)
	SaveOperationParams(opID string, params []byte) error
	GetOperationParams(opID string) ([]byte, error)
	DeleteOperationState(opID string) error
	GetInterruptedOperations() ([]Operation, error)

	// Operation Logs
	AddOperationLog(operationID, level, message string, details *string) error
	GetOperationLogs(operationID string) ([]OperationLog, error)

	// User Preferences
	GetUserPreference(key string) (*UserPreference, error)
	SetUserPreference(key, value string) error
	GetAllUserPreferences() ([]UserPreference, error)

	// Settings (persistent configuration with encryption support)
	GetSetting(key string) (*Setting, error)
	SetSetting(key, value, typ string, isSecret bool) error
	GetAllSettings() ([]Setting, error)
	DeleteSetting(key string) error

	// Playlists
	CreatePlaylist(name string, seriesID *int, filePath string) (*Playlist, error)
	GetPlaylistByID(id int) (*Playlist, error)
	GetPlaylistBySeriesID(seriesID int) (*Playlist, error)
	AddPlaylistItem(playlistID, bookID, position int) error
	GetPlaylistItems(playlistID int) ([]PlaylistItem, error)

	// Advanced (Pebble extended keyspace) - optional no-op for SQLite implementation
	// Users & Auth
	CreateUser(username, email, passwordHashAlgo, passwordHash string, roles []string, status string) (*User, error)
	GetUserByID(id string) (*User, error)
	GetUserByUsername(username string) (*User, error)
	GetUserByEmail(email string) (*User, error)
	UpdateUser(user *User) error

	// Sessions
	CreateSession(userID, ip, userAgent string, ttl time.Duration) (*Session, error)
	GetSession(id string) (*Session, error)
	RevokeSession(id string) error
	ListUserSessions(userID string) ([]Session, error)
	DeleteExpiredSessions(now time.Time) (int, error)

	// Auth bootstrap helpers
	CountUsers() (int, error)

	// Per-user preferences
	SetUserPreferenceForUser(userID, key, value string) error
	GetUserPreferenceForUser(userID, key string) (*UserPreferenceKV, error)
	GetAllPreferencesForUser(userID string) ([]UserPreferenceKV, error)

	// Book segments & merge operations
	CreateBookSegment(bookNumericID int, segment *BookSegment) (*BookSegment, error)
	ListBookSegments(bookNumericID int) ([]BookSegment, error)
	MergeBookSegments(bookNumericID int, newSegment *BookSegment, supersedeIDs []string) error

	// Playback events & progress
	AddPlaybackEvent(event *PlaybackEvent) error
	ListPlaybackEvents(userID string, bookNumericID int, limit int) ([]PlaybackEvent, error)
	UpdatePlaybackProgress(progress *PlaybackProgress) error
	GetPlaybackProgress(userID string, bookNumericID int) (*PlaybackProgress, error)

	// Stats aggregation
	IncrementBookPlayStats(bookNumericID int, seconds int) error
	GetBookStats(bookNumericID int) (*BookStats, error)
	IncrementUserListenStats(userID string, seconds int) error
	GetUserStats(userID string) (*UserStats, error)

	// Hash blocklist (do_not_import)
	IsHashBlocked(hash string) (bool, error)
	AddBlockedHash(hash, reason string) error
	RemoveBlockedHash(hash string) error
	GetAllBlockedHashes() ([]DoNotImport, error)
	GetBlockedHashByHash(hash string) (*DoNotImport, error)
}

// Common data structures used by all store implementations
// Note: These are defined here instead of in web.go to avoid circular dependencies

// Author represents an audiobook author
type Author struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// BookAuthor represents the many-to-many relationship between books and authors
type BookAuthor struct {
	BookID   string `json:"book_id"`
	AuthorID int    `json:"author_id"`
	Role     string `json:"role"`     // author, co-author, editor
	Position int    `json:"position"` // 0 = primary
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
	Language             *string `json:"language,omitempty"`
	Publisher            *string `json:"publisher,omitempty"`
	PrintYear            *int    `json:"print_year,omitempty"`
	AudiobookReleaseYear *int    `json:"audiobook_release_year,omitempty"`
	ISBN10               *string `json:"isbn10,omitempty"`
	ISBN13               *string `json:"isbn13,omitempty"`
	// iTunes import fields
	ITunesPersistentID *string    `json:"itunes_persistent_id,omitempty"`
	ITunesDateAdded    *time.Time `json:"itunes_date_added,omitempty"`
	ITunesPlayCount    *int       `json:"itunes_play_count,omitempty"`
	ITunesLastPlayed   *time.Time `json:"itunes_last_played,omitempty"`
	ITunesRating       *int       `json:"itunes_rating,omitempty"`
	ITunesBookmark     *int64     `json:"itunes_bookmark,omitempty"`
	ITunesImportSource *string    `json:"itunes_import_source,omitempty"`
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
	UpdatedAt           *time.Time `json:"updated_at,omitempty"`
	// Cover art
	CoverURL *string `json:"cover_url,omitempty"`
	// Narrators as JSON array
	NarratorsJSON *string `json:"narrators_json,omitempty"`
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

// Playlist represents a playlist
type Playlist struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	SeriesID *int   `json:"series_id,omitempty"`
	FilePath string `json:"file_path"`
}

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
	Active       bool      `json:"active"`
	SupersededBy *string   `json:"superseded_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Version      int       `json:"version"`
}

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

// DashboardStats holds aggregated statistics computed via SQL rather than loading all books.
type DashboardStats struct {
	TotalBooks         int            `json:"total_books"`
	TotalAuthors       int            `json:"total_authors"`
	TotalSeries        int            `json:"total_series"`
	TotalDuration      int64          `json:"total_duration"`
	TotalSize          int64          `json:"total_size"`
	StateDistribution  map[string]int `json:"state_distribution"`
	FormatDistribution map[string]int `json:"format_distribution"`
}

// Global store instance
var GlobalStore Store

// InitializeStore initializes the database store based on configuration
func InitializeStore(dbType, path string, enableSQLite bool) error {
	var err error

	switch dbType {
	case "sqlite", "sqlite3":
		if !enableSQLite {
			return fmt.Errorf("SQLite3 is not enabled. To use SQLite3, you must explicitly enable it with --enable-sqlite3-i-know-the-risks or set 'enable_sqlite3_i_know_the_risks: true' in your config file. PebbleDB is the recommended database for production use")
		}
		GlobalStore, err = NewSQLiteStore(path)
		if err != nil {
			return fmt.Errorf("failed to initialize SQLite store: %w", err)
		}
	case "pebble", "":
		// PebbleDB is the default
		GlobalStore, err = NewPebbleStore(path)
		if err != nil {
			return fmt.Errorf("failed to initialize PebbleDB store: %w", err)
		}
	default:
		return fmt.Errorf("unsupported database type: %s (supported: pebble, sqlite)", dbType)
	}

	// Maintain backwards compatibility with the global DB variable for SQLite
	if sqliteStore, ok := GlobalStore.(*SQLiteStore); ok {
		DB = sqliteStore.db
	}

	// Run migrations to ensure schema is up to date
	if err := RunMigrations(GlobalStore); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// CloseStore closes the global store
func CloseStore() error {
	if GlobalStore != nil {
		return GlobalStore.Close()
	}
	// Backwards compatibility
	if DB != nil {
		return DB.Close()
	}
	return nil
}
