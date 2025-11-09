// file: internal/database/store.go
// version: 2.0.0
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

	// Authors
	GetAllAuthors() ([]Author, error)
	GetAuthorByID(id int) (*Author, error)
	GetAuthorByName(name string) (*Author, error)
	CreateAuthor(name string) (*Author, error)

	// Series
	GetAllSeries() ([]Series, error)
	GetSeriesByID(id int) (*Series, error)
	GetSeriesByName(name string, authorID *int) (*Series, error)
	CreateSeries(name string, authorID *int) (*Series, error)

	// Books
	GetAllBooks(limit, offset int) ([]Book, error)
	GetBookByID(id int) (*Book, error)
	GetBookByFilePath(path string) (*Book, error)
	GetBooksBySeriesID(seriesID int) ([]Book, error)
	GetBooksByAuthorID(authorID int) ([]Book, error)
	CreateBook(book *Book) (*Book, error)
	UpdateBook(id int, book *Book) (*Book, error)
	DeleteBook(id int) error
	SearchBooks(query string, limit, offset int) ([]Book, error)
	CountBooks() (int, error)

	// Library Folders
	GetAllLibraryFolders() ([]LibraryFolder, error)
	GetLibraryFolderByID(id int) (*LibraryFolder, error)
	GetLibraryFolderByPath(path string) (*LibraryFolder, error)
	CreateLibraryFolder(path, name string) (*LibraryFolder, error)
	UpdateLibraryFolder(id int, folder *LibraryFolder) error
	DeleteLibraryFolder(id int) error

	// Operations
	CreateOperation(id, opType string, folderPath *string) (*Operation, error)
	GetOperationByID(id string) (*Operation, error)
	GetRecentOperations(limit int) ([]Operation, error)
	UpdateOperationStatus(id, status string, progress, total int, message string) error
	UpdateOperationError(id, errorMessage string) error

	// Operation Logs
	AddOperationLog(operationID, level, message string, details *string) error
	GetOperationLogs(operationID string) ([]OperationLog, error)

	// User Preferences
	GetUserPreference(key string) (*UserPreference, error)
	SetUserPreference(key, value string) error
	GetAllUserPreferences() ([]UserPreference, error)

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
}

// Common data structures used by all store implementations
// Note: These are defined here instead of in web.go to avoid circular dependencies

// Author represents an audiobook author
type Author struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Series represents an audiobook series
type Series struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	AuthorID *int   `json:"author_id,omitempty"`
}

// Book represents an audiobook
type Book struct {
	ID             int    `json:"id"`
	Title          string `json:"title"`
	AuthorID       *int   `json:"author_id,omitempty"`
	SeriesID       *int   `json:"series_id,omitempty"`
	SeriesSequence *int   `json:"series_sequence,omitempty"`
	FilePath       string `json:"file_path"`
	Format         string `json:"format,omitempty"`
	Duration       *int   `json:"duration,omitempty"`
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

// LibraryFolder represents a managed library folder
type LibraryFolder struct {
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
