// file: internal/scanner/scanner.go
// version: 1.2.0
// guid: 0e1f2a3b-4c5d-6e7f-8a9b-0c1d2e3f4a5b

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
	FilePath  string
	Title     string
	Author    string
	Series    string
	Position  int
	Format    string
	Duration  int
	Narrator  string
	Language  string
	Publisher string
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
				// Calculate relative path for informational purposes if needed later
				// We're not using the relative path right now, but keeping the calculation
				// in case it's needed in the future
				_, _ = filepath.Rel(rootDir, path)

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
			books[i].Narrator = meta.Narrator
			books[i].Language = meta.Language
			books[i].Publisher = meta.Publisher
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
		if err := saveBookToDatabase(&books[i]); err != nil {
			fmt.Printf("Warning: Failed to save book to database: %v\n", err)
		}

		bar.Add(1)
	}

	// After processing all books, try to match series using external APIs for uncertain cases
	if err := identifySeriesUsingExternalAPIs(books); err != nil {
		fmt.Printf("Warning: Error identifying series using external APIs: %v\n", err)
	}

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
	// Prefer using the unified Store API when available
	if database.GlobalStore != nil {
		// Resolve author (create if missing)
		var authorID *int
		if book.Author != "" {
			author, err := database.GlobalStore.GetAuthorByName(book.Author)
			if err != nil {
				return fmt.Errorf("author lookup failed: %w", err)
			}
			if author == nil {
				author, err = database.GlobalStore.CreateAuthor(book.Author)
				if err != nil {
					return fmt.Errorf("author create failed: %w", err)
				}
			}
			authorID = &author.ID
		}

		// Resolve series (create if missing)
		var seriesID *int
		if book.Series != "" {
			series, err := database.GlobalStore.GetSeriesByName(book.Series, authorID)
			if err != nil {
				return fmt.Errorf("series lookup failed: %w", err)
			}
			if series == nil {
				series, err = database.GlobalStore.CreateSeries(book.Series, authorID)
				if err != nil {
					return fmt.Errorf("series create failed: %w", err)
				}
			}
			seriesID = &series.ID
		}

		// Attempt Work association (normalize title + author)
		var workID *string
		if book.Title != "" {
			canonical := strings.ToLower(strings.TrimSpace(book.Title))
			// Simple heuristic: try existing works then create new
			works, err := database.GlobalStore.GetAllWorks()
			if err == nil { // non-critical
				for _, w := range works {
					if strings.ToLower(strings.TrimSpace(w.Title)) == canonical && ((authorID == nil && w.AuthorID == nil) || (authorID != nil && w.AuthorID != nil && *authorID == *w.AuthorID)) {
						wid := w.ID
						workID = &wid
						break
					}
				}
			}
			if workID == nil {
				newWork := &database.Work{Title: book.Title, AuthorID: authorID}
				created, err := database.GlobalStore.CreateWork(newWork)
				if err == nil {
					wid := created.ID
					workID = &wid
				}
			}
		}

		dbBook := &database.Book{
			Title:          book.Title,
			AuthorID:       authorID,
			SeriesID:       seriesID,
			SeriesSequence: &book.Position,
			FilePath:       book.FilePath,
			Format:         strings.TrimPrefix(book.Format, "."),
			Duration:       &book.Duration,
			WorkID:         workID,
			Narrator:       nullablePtr(book.Narrator),
			Language:       nullablePtr(book.Language),
			Publisher:      nullablePtr(book.Publisher),
		}

		// Upsert semantics: try lookup by file path first
		existing, err := database.GlobalStore.GetBookByFilePath(book.FilePath)
		if err != nil {
			return fmt.Errorf("book lookup failed: %w", err)
		}
		if existing == nil {
			_, err = database.GlobalStore.CreateBook(dbBook)
			return err
		}
		_, err = database.GlobalStore.UpdateBook(existing.ID, dbBook)
		return err
	}

	// Fallback legacy path using raw DB for backward compatibility
	var authorIDInt int64
	err := database.DB.QueryRow("SELECT id FROM authors WHERE name = ?", book.Author).Scan(&authorIDInt)
	if err != nil {
		result, err2 := database.DB.Exec("INSERT INTO authors (name) VALUES (?)", book.Author)
		if err2 != nil {
			return fmt.Errorf("failed to insert author: %w", err2)
		}
		authorIDInt, _ = result.LastInsertId()
	}
	var seriesID sql.NullInt64
	if book.Series != "" {
		var id int64
		serr := database.DB.QueryRow("SELECT id FROM series WHERE name = ?", book.Series).Scan(&id)
		if serr != nil {
			result, ierr := database.DB.Exec("INSERT INTO series (name, author_id) VALUES (?, ?)", book.Series, authorIDInt)
			if ierr != nil {
				return fmt.Errorf("failed to insert series: %w", ierr)
			}
			id, _ = result.LastInsertId()
		}
		seriesID.Int64 = id
		seriesID.Valid = true
	}
	_, err = database.DB.Exec(`
	        INSERT INTO books (title, author_id, series_id, series_sequence, file_path, format, duration)
	        VALUES (?, ?, ?, ?, ?, ?, ?)
	        ON CONFLICT(file_path)
	        DO UPDATE SET title=?, author_id=?, series_id=?, series_sequence=?, format=?, duration=?
	    `,
		book.Title, authorIDInt, seriesID, book.Position, book.FilePath, book.Format, book.Duration,
		book.Title, authorIDInt, seriesID, book.Position, book.Format, book.Duration,
	)
	return err
}

func nullablePtr(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

// identifySeriesUsingExternalAPIs tries to match books to series using external APIs
func identifySeriesUsingExternalAPIs(books []Book) error {
	// Implement API calls to GoodReads or similar services
	// This is a placeholder - actual implementation would depend on available APIs
	return nil
}
