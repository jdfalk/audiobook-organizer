// file: internal/database/store.go
// version: 1.0.0
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
	ID             int     `json:"id"`
	Title          string  `json:"title"`
	AuthorID       *int    `json:"author_id,omitempty"`
	SeriesID       *int    `json:"series_id,omitempty"`
	SeriesSequence *int    `json:"series_sequence,omitempty"`
	FilePath       string  `json:"file_path"`
	Format         string  `json:"format,omitempty"`
	Duration       *int    `json:"duration,omitempty"`
}

// Playlist represents a playlist
type Playlist struct {
	ID       int     `json:"id"`
	Name     string  `json:"name"`
	SeriesID *int    `json:"series_id,omitempty"`
	FilePath string  `json:"file_path"`
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