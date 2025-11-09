// file: internal/database/database.go
// version: 1.1.0
// guid: 8c9d0e1f-2a3b-4c5d-6e7f-8a9b0c1d2e3f

package database

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// DB is the database connection
var DB *sql.DB

// Initialize sets up the database connection and tables
func Initialize(databasePath string) error {
	var err error
	DB, err = sql.Open("sqlite3", databasePath)
	if err != nil {
		return fmt.Errorf("error opening database: %w", err)
	}

	// Create tables if they don't exist
	if err := createTables(); err != nil {
		return fmt.Errorf("error creating tables: %w", err)
	}

	return nil
}

// createTables creates necessary database tables
func createTables() error {
	// Create authors table
	_, err := DB.Exec(`
        CREATE TABLE IF NOT EXISTS authors (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL UNIQUE
        )
    `)
	if err != nil {
		return err
	}

	// Create series table
	_, err = DB.Exec(`
        CREATE TABLE IF NOT EXISTS series (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL UNIQUE,
            author_id INTEGER,
            FOREIGN KEY (author_id) REFERENCES authors(id)
        )
    `)
	if err != nil {
		return err
	}

	// Create books table
	_, err = DB.Exec(`
        CREATE TABLE IF NOT EXISTS books (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            title TEXT NOT NULL,
            author_id INTEGER,
            series_id INTEGER,
            series_sequence INTEGER,
            file_path TEXT NOT NULL UNIQUE,
            format TEXT,
            duration INTEGER,
            FOREIGN KEY (author_id) REFERENCES authors(id),
            FOREIGN KEY (series_id) REFERENCES series(id)
        )
    `)
	if err != nil {
		return err
	}

	// Create playlists table
	_, err = DB.Exec(`
        CREATE TABLE IF NOT EXISTS playlists (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL UNIQUE,
            series_id INTEGER,
            file_path TEXT,
            FOREIGN KEY (series_id) REFERENCES series(id)
        )
    `)
	if err != nil {
		return err
	}

	// Create playlist_items table
	_, err = DB.Exec(`
        CREATE TABLE IF NOT EXISTS playlist_items (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            playlist_id INTEGER,
            book_id INTEGER,
            position INTEGER,
            FOREIGN KEY (playlist_id) REFERENCES playlists(id),
            FOREIGN KEY (book_id) REFERENCES books(id)
        )
    `)
	if err != nil {
		return err
	}

	// Create library_folders table for web interface
	_, err = DB.Exec(`
        CREATE TABLE IF NOT EXISTS library_folders (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            path TEXT NOT NULL UNIQUE,
            name TEXT NOT NULL,
            enabled BOOLEAN DEFAULT 1,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            last_scan TIMESTAMP,
            book_count INTEGER DEFAULT 0
        )
    `)
	if err != nil {
		return err
	}

	// Create operations table for async operation tracking
	_, err = DB.Exec(`
        CREATE TABLE IF NOT EXISTS operations (
            id TEXT PRIMARY KEY,
            type TEXT NOT NULL,
            status TEXT NOT NULL DEFAULT 'pending',
            progress INTEGER DEFAULT 0,
            total INTEGER DEFAULT 0,
            message TEXT,
            folder_path TEXT,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            started_at TIMESTAMP,
            completed_at TIMESTAMP,
            error_message TEXT
        )
    `)
	if err != nil {
		return err
	}

	// Create operation_logs table for detailed operation history
	_, err = DB.Exec(`
        CREATE TABLE IF NOT EXISTS operation_logs (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            operation_id TEXT NOT NULL,
            level TEXT NOT NULL DEFAULT 'info',
            message TEXT NOT NULL,
            details TEXT,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (operation_id) REFERENCES operations(id)
        )
    `)
	if err != nil {
		return err
	}

	// Create user_preferences table for UI settings
	_, err = DB.Exec(`
        CREATE TABLE IF NOT EXISTS user_preferences (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            key TEXT NOT NULL UNIQUE,
            value TEXT,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    `)
	if err != nil {
		return err
	}

	return nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
