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

	return nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}
