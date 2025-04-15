package scanner

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/matcher"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/schollz/progressbar/v3"
)

// Book represents an audiobook file
type Book struct {
	FilePath string
	Title    string
	Author   string
	Series   string
	Position int
	Format   string
	Duration int
}

// ScanDirectory scans the given directory for audiobook files
func ScanDirectory(rootDir string) ([]Book, error) {
	var books []Book

	fmt.Println("Scanning for audiobook files...")

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		for _, supportedExt := range config.AppConfig.SupportedExtensions {
			if ext == supportedExt {
				relPath, err := filepath.Rel(rootDir, path)
				if err != nil {
					relPath = path
				}

				books = append(books, Book{
					FilePath: path,
					Format:   ext,
				})
				break
			}
		}

		return nil
	})

	return books, err
}

// ProcessBooks processes the discovered books to extract metadata and identify series
func ProcessBooks(books []Book) error {
	fmt.Println("Processing audiobook metadata...")

	bar := progressbar.Default(int64(len(books)))

	for i := range books {
		// Extract metadata from the file
		meta, err := metadata.ExtractMetadata(books[i].FilePath)
		if err != nil {
			fmt.Printf("Warning: Could not extract metadata from %s: %v\n", books[i].FilePath, err)
		} else {
			books[i].Title = meta.Title
			books[i].Author = meta.Artist
		}

		// If metadata is incomplete, try to extract info from filepath
		if books[i].Title == "" || books[i].Author == "" {
			extractInfoFromPath(&books[i])
		}

		// Identify series based on title and filepath
		series, position := matcher.IdentifySeries(books[i].Title, books[i].FilePath)
		books[i].Series = series
		books[i].Position = position

		// Save to database
		saveBookToDatabase(&books[i])

		bar.Add(1)
	}

	// After processing all books, try to match series using external APIs for uncertain cases
	identifySeriesUsingExternalAPIs(books)

	return nil
}

// extractInfoFromPath tries to extract author and title information from the file path
func extractInfoFromPath(book *Book) {
	path := book.FilePath

	// Remove the extension
	baseName := filepath.Base(path)
	baseName = strings.TrimSuffix(baseName, filepath.Ext(baseName))

	// Check if we can extract series and title information
	// Many naming conventions use " - " to separate series from title
	parts := strings.Split(baseName, " - ")
	if len(parts) > 1 {
		book.Title = strings.TrimSpace(parts[len(parts)-1])
		if book.Series == "" {
			book.Series = strings.TrimSpace(parts[0])
		}
	} else {
		book.Title = baseName
	}

	// Extract author from directory name
	dirs := strings.Split(filepath.Dir(path), string(os.PathSeparator))
	if len(dirs) > 0 {
		authorDir := dirs[len(dirs)-1]
		// If we don't have an author yet, use the directory name
		if book.Author == "" {
			book.Author = authorDir
		}
	}
}

// saveBookToDatabase saves the book information to the database
func saveBookToDatabase(book *Book) error {
	// First ensure the author exists
	var authorID int64
	err := database.DB.QueryRow("SELECT id FROM authors WHERE name = ?", book.Author).Scan(&authorID)
	if err != nil {
		// Insert new author
		result, err := database.DB.Exec("INSERT INTO authors (name) VALUES (?)", book.Author)
		if err != nil {
			return fmt.Errorf("failed to insert author: %w", err)
		}
		authorID, _ = result.LastInsertId()
	}

	// Then handle the series if it exists
	var seriesID sql.NullInt64
	if book.Series != "" {
		var id int64
		err := database.DB.QueryRow("SELECT id FROM series WHERE name = ?", book.Series).Scan(&id)
		if err != nil {
			// Insert new series
			result, err := database.DB.Exec("INSERT INTO series (name, author_id) VALUES (?, ?)",
				book.Series, authorID)
			if err != nil {
				return fmt.Errorf("failed to insert series: %w", err)
			}
			id, _ = result.LastInsertId()
		}
		seriesID.Int64 = id
		seriesID.Valid = true
	}

	// Finally insert the book
	_, err = database.DB.Exec(`
        INSERT INTO books (title, author_id, series_id, series_sequence, file_path, format, duration)
        VALUES (?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(file_path)
        DO UPDATE SET title=?, author_id=?, series_id=?, series_sequence=?, format=?, duration=?
    `,
		book.Title, authorID, seriesID, book.Position, book.FilePath, book.Format, book.Duration,
		book.Title, authorID, seriesID, book.Position, book.Format, book.Duration,
	)

	return err
}

// identifySeriesUsingExternalAPIs tries to match books to series using external APIs
func identifySeriesUsingExternalAPIs(books []Book) error {
	// Implement API calls to GoodReads or similar services
	// This is a placeholder - actual implementation would depend on available APIs
	return nil
}
