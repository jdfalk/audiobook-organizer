// file: internal/database/audiobooks.go
// version: 1.0.0
// guid: 7f8a9b0c-1d2e-3f4a-5b6c-7d8e9f0a1b2c

package database

import (
	"fmt"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/models"
)

// GetAudiobooks returns paginated list of audiobooks with filtering and sorting
func GetAudiobooks(req models.AudiobookListRequest) (models.AudiobookListResponse, error) {
	// Set defaults
	if req.Page < 1 {
		req.Page = 1
	}
	if req.Limit < 1 {
		req.Limit = 50
	}
	if req.Limit > 200 {
		req.Limit = 200
	}
	if req.SortBy == "" {
		req.SortBy = "title"
	}
	if req.SortDir != "desc" {
		req.SortDir = "asc"
	}

	// Build query
	baseQuery := `
		SELECT b.id, b.title, b.author_id, b.series_id, b.series_sequence, 
		       b.file_path, b.format, b.duration,
		       a.id as author_id, a.name as author_name,
		       s.id as series_id, s.name as series_name
		FROM books b
		LEFT JOIN authors a ON b.author_id = a.id
		LEFT JOIN series s ON b.series_id = s.id
	`

	// Build WHERE clauses
	var whereClauses []string
	var args []interface{}

	if req.Search != "" {
		whereClauses = append(whereClauses, "(b.title LIKE ? OR a.name LIKE ? OR s.name LIKE ?)")
		searchTerm := "%" + req.Search + "%"
		args = append(args, searchTerm, searchTerm, searchTerm)
	}

	if req.Author != "" {
		whereClauses = append(whereClauses, "a.name LIKE ?")
		args = append(args, "%"+req.Author+"%")
	}

	if req.Series != "" {
		whereClauses = append(whereClauses, "s.name LIKE ?")
		args = append(args, "%"+req.Series+"%")
	}

	if req.Format != "" {
		whereClauses = append(whereClauses, "b.format = ?")
		args = append(args, req.Format)
	}

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Count total records
	countQuery := "SELECT COUNT(*) FROM books b LEFT JOIN authors a ON b.author_id = a.id LEFT JOIN series s ON b.series_id = s.id" + whereClause
	var total int
	err := DB.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return models.AudiobookListResponse{}, fmt.Errorf("failed to count audiobooks: %w", err)
	}

	// Build ORDER BY clause
	validSortFields := map[string]string{
		"title":    "b.title",
		"author":   "a.name",
		"series":   "s.name",
		"format":   "b.format",
		"duration": "b.duration",
	}

	sortField, ok := validSortFields[req.SortBy]
	if !ok {
		sortField = "b.title"
	}

	orderClause := fmt.Sprintf(" ORDER BY %s %s", sortField, strings.ToUpper(req.SortDir))

	// Build final query with pagination
	query := baseQuery + whereClause + orderClause + " LIMIT ? OFFSET ?"
	offset := (req.Page - 1) * req.Limit
	args = append(args, req.Limit, offset)

	// Execute query
	rows, err := DB.Query(query, args...)
	if err != nil {
		return models.AudiobookListResponse{}, fmt.Errorf("failed to query audiobooks: %w", err)
	}
	defer rows.Close()

	var audiobooks []models.Audiobook
	for rows.Next() {
		var book models.Audiobook
		var authorID, authorName, seriesID, seriesName *string

		err := rows.Scan(
			&book.ID, &book.Title, &book.AuthorID, &book.SeriesID, &book.SeriesSequence,
			&book.FilePath, &book.Format, &book.Duration,
			&authorID, &authorName, &seriesID, &seriesName,
		)
		if err != nil {
			return models.AudiobookListResponse{}, fmt.Errorf("failed to scan audiobook: %w", err)
		}

		// Populate related objects
		if authorID != nil && authorName != nil {
			book.Author = &models.Author{
				ID:   book.ID, // This should be parsed from authorID
				Name: *authorName,
			}
		}

		if seriesID != nil && seriesName != nil {
			book.Series = &models.Series{
				ID:   book.ID, // This should be parsed from seriesID
				Name: *seriesName,
			}
		}

		audiobooks = append(audiobooks, book)
	}

	if err = rows.Err(); err != nil {
		return models.AudiobookListResponse{}, fmt.Errorf("error iterating audiobooks: %w", err)
	}

	// Calculate pages
	pages := (total + req.Limit - 1) / req.Limit

	return models.AudiobookListResponse{
		Audiobooks: audiobooks,
		Total:      total,
		Page:       req.Page,
		Limit:      req.Limit,
		Pages:      pages,
	}, nil
}

// GetAudiobookByID returns a specific audiobook by ID
func GetAudiobookByID(id int) (*models.Audiobook, error) {
	query := `
		SELECT b.id, b.title, b.author_id, b.series_id, b.series_sequence, 
		       b.file_path, b.format, b.duration,
		       a.id as author_id, a.name as author_name,
		       s.id as series_id, s.name as series_name
		FROM books b
		LEFT JOIN authors a ON b.author_id = a.id
		LEFT JOIN series s ON b.series_id = s.id
		WHERE b.id = ?
	`

	row := DB.QueryRow(query, id)

	var book models.Audiobook
	var authorID, authorName, seriesID, seriesName *string

	err := row.Scan(
		&book.ID, &book.Title, &book.AuthorID, &book.SeriesID, &book.SeriesSequence,
		&book.FilePath, &book.Format, &book.Duration,
		&authorID, &authorName, &seriesID, &seriesName,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get audiobook: %w", err)
	}

	// Populate related objects
	if authorID != nil && authorName != nil {
		book.Author = &models.Author{
			ID:   *book.AuthorID,
			Name: *authorName,
		}
	}

	if seriesID != nil && seriesName != nil {
		book.Series = &models.Series{
			ID:   *book.SeriesID,
			Name: *seriesName,
		}
	}

	return &book, nil
}

// UpdateAudiobook updates an audiobook's metadata
func UpdateAudiobook(id int, req models.AudiobookUpdateRequest) (*models.Audiobook, error) {
	var setParts []string
	var args []interface{}

	if req.Title != nil {
		setParts = append(setParts, "title = ?")
		args = append(args, *req.Title)
	}

	// Handle author - create if doesn't exist
	if req.Author != nil {
		authorID, err := GetOrCreateAuthor(*req.Author)
		if err != nil {
			return nil, fmt.Errorf("failed to get/create author: %w", err)
		}
		setParts = append(setParts, "author_id = ?")
		args = append(args, authorID)
	}

	// Handle series - create if doesn't exist
	if req.Series != nil {
		if *req.Series == "" {
			// Remove from series
			setParts = append(setParts, "series_id = NULL, series_sequence = NULL")
		} else {
			seriesID, err := GetOrCreateSeries(*req.Series, nil) // TODO: Handle author relationship
			if err != nil {
				return nil, fmt.Errorf("failed to get/create series: %w", err)
			}
			setParts = append(setParts, "series_id = ?")
			args = append(args, seriesID)
		}
	}

	if req.SeriesSequence != nil {
		setParts = append(setParts, "series_sequence = ?")
		args = append(args, *req.SeriesSequence)
	}

	if req.Format != nil {
		setParts = append(setParts, "format = ?")
		args = append(args, *req.Format)
	}

	if req.Duration != nil {
		setParts = append(setParts, "duration = ?")
		args = append(args, *req.Duration)
	}

	if len(setParts) == 0 {
		// No changes, return current audiobook
		return GetAudiobookByID(id)
	}

	query := fmt.Sprintf("UPDATE books SET %s WHERE id = ?", strings.Join(setParts, ", "))
	args = append(args, id)

	_, err := DB.Exec(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to update audiobook: %w", err)
	}

	return GetAudiobookByID(id)
}

// DeleteAudiobook removes an audiobook from the database
func DeleteAudiobook(id int) error {
	query := "DELETE FROM books WHERE id = ?"
	_, err := DB.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete audiobook: %w", err)
	}
	return nil
}

// GetOrCreateAuthor gets an existing author or creates a new one
func GetOrCreateAuthor(name string) (int, error) {
	// First try to find existing author
	var id int
	query := "SELECT id FROM authors WHERE name = ?"
	err := DB.QueryRow(query, name).Scan(&id)
	if err == nil {
		return id, nil
	}

	// Create new author
	query = "INSERT INTO authors (name) VALUES (?)"
	result, err := DB.Exec(query, name)
	if err != nil {
		return 0, fmt.Errorf("failed to create author: %w", err)
	}

	newID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get new author ID: %w", err)
	}

	return int(newID), nil
}

// GetOrCreateSeries gets an existing series or creates a new one
func GetOrCreateSeries(name string, authorID *int) (int, error) {
	// First try to find existing series
	var id int
	var query string
	var args []interface{}

	if authorID != nil {
		query = "SELECT id FROM series WHERE name = ? AND author_id = ?"
		args = []interface{}{name, *authorID}
	} else {
		query = "SELECT id FROM series WHERE name = ? AND author_id IS NULL"
		args = []interface{}{name}
	}

	err := DB.QueryRow(query, args...).Scan(&id)
	if err == nil {
		return id, nil
	}

	// Create new series
	query = "INSERT INTO series (name, author_id) VALUES (?, ?)"
	result, err := DB.Exec(query, name, authorID)
	if err != nil {
		return 0, fmt.Errorf("failed to create series: %w", err)
	}

	newID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get new series ID: %w", err)
	}

	return int(newID), nil
}

// GetAllAuthors returns all authors
func GetAllAuthors() ([]models.Author, error) {
	query := "SELECT id, name FROM authors ORDER BY name"
	rows, err := DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query authors: %w", err)
	}
	defer rows.Close()

	var authors []models.Author
	for rows.Next() {
		var author models.Author
		err := rows.Scan(&author.ID, &author.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to scan author: %w", err)
		}
		authors = append(authors, author)
	}

	return authors, rows.Err()
}

// GetAllSeries returns all series
func GetAllSeries() ([]models.Series, error) {
	query := `
		SELECT s.id, s.name, s.author_id, a.name as author_name
		FROM series s
		LEFT JOIN authors a ON s.author_id = a.id
		ORDER BY s.name
	`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query series: %w", err)
	}
	defer rows.Close()

	var seriesList []models.Series
	for rows.Next() {
		var series models.Series
		var authorName *string

		err := rows.Scan(&series.ID, &series.Name, &series.AuthorID, &authorName)
		if err != nil {
			return nil, fmt.Errorf("failed to scan series: %w", err)
		}

		if series.AuthorID != nil && authorName != nil {
			series.Author = &models.Author{
				ID:   *series.AuthorID,
				Name: *authorName,
			}
		}

		seriesList = append(seriesList, series)
	}

	return seriesList, rows.Err()
}
