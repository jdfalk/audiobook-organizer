// file: internal/playlist/playlist.go
// version: 1.1.0
// guid: 2a3b4c5d-6e7f-8a9b-0c1d-2e3f4a5b6c7d

package playlist

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// PlaylistItem represents a book in a playlist
type PlaylistItem struct {
	BookID   int
	Title    string
	Author   string
	FilePath string
	Position int
}

// GeneratePlaylistsForSeries generates playlists for all identified series
func GeneratePlaylistsForSeries() error {
	// First, get all series from the database
	rows, err := database.DB.Query(`
        SELECT series.id, series.name, authors.name
        FROM series
        JOIN authors ON series.author_id = authors.id
    `)
	if err != nil {
		return fmt.Errorf("failed to query series: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var seriesID int
		var seriesName, authorName string

		if err := rows.Scan(&seriesID, &seriesName, &authorName); err != nil {
			return fmt.Errorf("failed to scan series row: %w", err)
		}

		// Get all books in this series
		items, err := getBooksInSeries(seriesID)
		if err != nil {
			return fmt.Errorf("failed to get books for series %s: %w", seriesName, err)
		}

		// Skip if no books found
		if len(items) == 0 {
			continue
		}

		// Sort books by position
		sort.Slice(items, func(i, j int) bool {
			if items[i].Position == items[j].Position {
				return items[i].Title < items[j].Title
			}
			return items[i].Position < items[j].Position
		})

		// Generate playlist name
		playlistName := fmt.Sprintf("%s - %s", seriesName, authorName)

		// Create iTunes XML playlist
		playlistPath, err := createiTunesPlaylist(playlistName, items)
		if err != nil {
			return fmt.Errorf("failed to create playlist %s: %w", playlistName, err)
		}

		// Save playlist info to database
		if err := savePlaylistToDatabase(seriesID, playlistName, playlistPath); err != nil {
			return fmt.Errorf("failed to save playlist to database: %w", err)
		}

		fmt.Printf("Generated playlist: %s\n", playlistName)
	}

	return nil
}

// getBooksInSeries retrieves all books belonging to a specific series
func getBooksInSeries(seriesID int) ([]PlaylistItem, error) {
	var items []PlaylistItem

	rows, err := database.DB.Query(`
        SELECT books.id, books.title, authors.name, books.file_path, books.series_sequence
        FROM books
        JOIN authors ON books.author_id = authors.id
        WHERE books.series_id = ?
        ORDER BY books.series_sequence, books.title
    `, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item PlaylistItem
		var position sql.NullInt64

		if err := rows.Scan(&item.BookID, &item.Title, &item.Author, &item.FilePath, &position); err != nil {
			return nil, err
		}

		if position.Valid {
			item.Position = int(position.Int64)
		}

		items = append(items, item)
	}

	return items, nil
}

// createiTunesPlaylist generates an iTunes XML playlist file
func createiTunesPlaylist(playlistName string, items []PlaylistItem) (string, error) {
	// Sanitize playlist name for filename
	safePlaylistName := strings.ReplaceAll(playlistName, "/", "-")
	safePlaylistName = strings.ReplaceAll(safePlaylistName, "\\", "-")
	safePlaylistName = strings.ReplaceAll(safePlaylistName, ":", "-")

	playlistPath := filepath.Join(config.AppConfig.PlaylistDir, safePlaylistName+".m3u")

	f, err := os.Create(playlistPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Write M3U header
	_, err = f.WriteString("#EXTM3U\n")
	if err != nil {
		return "", err
	}

	// Add each book to the playlist
	for _, item := range items {
		// Write extended info
		_, err = f.WriteString(fmt.Sprintf("#EXTINF:-1,%s - %s\n", item.Author, item.Title))
		if err != nil {
			return "", err
		}

		// Write file path
		_, err = f.WriteString(item.FilePath + "\n")
		if err != nil {
			return "", err
		}
	}

	return playlistPath, nil
}

// savePlaylistToDatabase saves the playlist information to the database
func savePlaylistToDatabase(seriesID int, playlistName, playlistPath string) error {
	// First check if playlist already exists
	var playlistID int
	err := database.DB.QueryRow("SELECT id FROM playlists WHERE series_id = ?", seriesID).Scan(&playlistID)

	if err == sql.ErrNoRows {
		// Insert new playlist
		result, err := database.DB.Exec(`
            INSERT INTO playlists (name, series_id, file_path)
            VALUES (?, ?, ?)
        `, playlistName, seriesID, playlistPath)
		if err != nil {
			return err
		}
		playlistID64, _ := result.LastInsertId()
		playlistID = int(playlistID64)
	} else if err != nil {
		return err
	} else {
		// Update existing playlist
		_, err := database.DB.Exec(`
            UPDATE playlists
            SET name = ?, file_path = ?
            WHERE id = ?
        `, playlistName, playlistPath, playlistID)
		if err != nil {
			return err
		}

		// Remove old playlist items
		_, err = database.DB.Exec("DELETE FROM playlist_items WHERE playlist_id = ?", playlistID)
		if err != nil {
			return err
		}
	}

	// Get books in this series and add them to playlist_items
	rows, err := database.DB.Query(`
        SELECT id, series_sequence FROM books WHERE series_id = ? ORDER BY series_sequence, title
    `, seriesID)
	if err != nil {
		return err
	}
	defer rows.Close()

	position := 1
	for rows.Next() {
		var bookID int
		var sequence sql.NullInt64

		if err := rows.Scan(&bookID, &sequence); err != nil {
			return err
		}

		_, err = database.DB.Exec(`
            INSERT INTO playlist_items (playlist_id, book_id, position)
            VALUES (?, ?, ?)
        `, playlistID, bookID, position)
		if err != nil {
			return err
		}

		position++
	}

	return nil
}
