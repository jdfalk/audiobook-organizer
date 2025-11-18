// file: internal/scanner/scanner.go
// version: 1.5.0
// guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f

package scanner

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

	// Remove leading track/chapter numbers
	parts := strings.Split(baseName, " ")
	if len(parts) > 0 {
		if _, err := strconv.Atoi(parts[0]); err == nil {
			baseName = strings.Join(parts[1:], " ")
		}
	}
	baseName = strings.TrimSpace(baseName)

	// Remove chapter info from end
	re := regexp.MustCompile(`(?i)[-_]\d+\s+Chapter\s+\d+$`)
	baseName = re.ReplaceAllString(baseName, "")

	// Try underscore separator first
	if strings.Contains(baseName, "_") && !strings.Contains(baseName, " - ") {
		parts := strings.SplitN(baseName, "_", 2)
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			if looksLikePersonName(right) && !looksLikePersonName(left) && book.Author == "" {
				book.Author = right
				book.Title = left
				return
			} else if looksLikePersonName(left) && !looksLikePersonName(right) && book.Author == "" {
				book.Author = left
				book.Title = right
				return
			}
		}
	}

	// Try to parse "Title - Author" or "Author - Title" patterns from filename
	if strings.Contains(baseName, " - ") {
		title, author := parseFilenameForAuthor(baseName)
		if author != "" && book.Author == "" {
			book.Author = author
			book.Title = title
		} else {
			// Fallback to old behavior: treat as "Series - Title"
			parts := strings.Split(baseName, " - ")
			if len(parts) > 1 {
				book.Title = strings.TrimSpace(parts[len(parts)-1])
				if book.Series == "" {
					book.Series = strings.TrimSpace(parts[0])
				}
			} else {
				book.Title = baseName
			}
		}
	} else {
		book.Title = baseName
	}

	// Extract author from directory name
	if book.Author == "" {
		book.Author = extractAuthorFromDirectory(path)
	}
}

// extractAuthorFromDirectory extracts author from directory with validation
func extractAuthorFromDirectory(filePath string) string {
	dirs := strings.Split(filepath.Dir(filePath), string(os.PathSeparator))
	if len(dirs) == 0 {
		return ""
	}

	dirName := dirs[len(dirs)-1]

	// Skip common non-author directory names
	skipDirs := map[string]bool{
		"books": true, "audiobooks": true, "newbooks": true, "downloads": true,
		"media": true, "audio": true, "library": true, "collection": true,
		"bt": true, "incomplete": true, "data": true,
	}

	if skipDirs[strings.ToLower(dirName)] {
		return ""
	}

	// Handle "Author, Co-Author - translator - Title" patterns
	if strings.Contains(dirName, " - translator - ") || strings.Contains(dirName, " - narrated by - ") {
		re := regexp.MustCompile(`^([^-]+)\s*-\s*(?:translator|narrated by)\s*-`)
		matches := re.FindStringSubmatch(dirName)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	// Extract from "Author - Title" directory pattern
	if strings.Contains(dirName, " - ") {
		parts := strings.SplitN(dirName, " - ", 2)
		if len(parts) > 0 {
			author := strings.TrimSpace(parts[0])
			if isValidAuthor(author) {
				return author
			}
		}
	}

	// Use directory name if valid
	if isValidAuthor(dirName) {
		return dirName
	}

	return ""
}

// isValidAuthor checks if extracted author string is valid
func isValidAuthor(author string) bool {
	if author == "" {
		return false
	}

	lower := strings.ToLower(author)

	// Skip invalid patterns
	if strings.HasPrefix(lower, "book") || strings.HasPrefix(lower, "chapter") ||
		strings.HasPrefix(lower, "part") || strings.HasPrefix(lower, "vol") ||
		strings.HasPrefix(lower, "volume") || strings.HasPrefix(lower, "disc") {
		return false
	}

	// Skip purely numeric
	if _, err := strconv.Atoi(author); err == nil {
		return false
	}

	// Skip chapter patterns
	if strings.HasPrefix(lower, "chapter ") {
		return false
	}

	return true
} // parseFilenameForAuthor attempts to intelligently parse title and author from filename
// Handles patterns like "Title - Author" or "Author - Title"
// Returns (title, author) where author is empty string if pattern not detected
func parseFilenameForAuthor(filename string) (string, string) {
	parts := strings.Split(filename, " - ")
	if len(parts) != 2 {
		return "", "" // Not a simple two-part pattern
	}

	left := strings.TrimSpace(parts[0])
	right := strings.TrimSpace(parts[1])

	// Heuristic: check if right side looks like an author name
	rightIsName := looksLikePersonName(right)
	leftIsName := looksLikePersonName(left)

	if rightIsName && !leftIsName {
		// Pattern: "Title - Author"
		return left, right
	} else if leftIsName && !rightIsName {
		// Pattern: "Author - Title"
		return right, left
	} else if rightIsName {
		// Both could be names, prefer "Title - Author" pattern
		return left, right
	}

	// Couldn't determine, return empty author
	return "", ""
}

// looksLikePersonName checks if a string looks like a person's name
// Looks for patterns like "John Smith", "J. Smith", "J. K. Rowling"
func looksLikePersonName(s string) bool {
	if !isValidAuthor(s) {
		return false
	}

	// Check for initials like "J. K. Rowling" or "J.K. Rowling"
	if strings.Contains(s, ".") {
		// Count uppercase letters and periods
		uppers := 0
		for _, r := range s {
			if r >= 'A' && r <= 'Z' {
				uppers++
			}
		}
		if uppers >= 2 {
			return true
		}
	}

	// Check for multi-word names with proper capitalization
	words := strings.Fields(s)
	if len(words) >= 2 && len(words) <= 4 {
		// Check if all words start with uppercase
		allProperCase := true
		for _, word := range words {
			if len(word) == 0 || (word[0] < 'A' || word[0] > 'Z') {
				allProperCase = false
				break
			}
		}
		if allProperCase {
			return true
		}
	}

	// Check for "FirstName LastName" pattern (at least one space, proper case)
	if len(words) >= 2 {
		// First word starts with capital
		if len(words[0]) > 0 && words[0][0] >= 'A' && words[0][0] <= 'Z' {
			// Second word starts with capital
			if len(words[1]) > 0 && words[1][0] >= 'A' && words[1][0] <= 'Z' {
				return true
			}
		}
	}

	return false
} // saveBookToDatabase saves the book information to the database
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
