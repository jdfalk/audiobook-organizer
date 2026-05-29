// file: internal/database/sqlite_store_books.go
// version: 1.0.5
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-05-16

package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/fingerprint"
	ulid "github.com/oklog/ulid/v2"
)

// ---- Book Segments ----

func (s *SQLiteStore) CreateBookSegment(bookNumericID int, segment *BookSegment) (*BookSegment, error) {
	if segment.ID == "" {
		segment.ID = ulid.Make().String()
	}
	now := time.Now()
	segment.BookID = bookNumericID
	segment.CreatedAt = now
	segment.UpdatedAt = now
	segment.Version = 1
	_, err := s.db.Exec(
		`INSERT INTO book_segments (id, book_id, file_path, format, size_bytes, duration_seconds, track_number, total_tracks, file_hash, active, superseded_by, created_at, updated_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		segment.ID, bookNumericID, segment.FilePath, segment.Format, segment.SizeBytes, segment.DurationSec,
		segment.TrackNumber, segment.TotalTracks, segment.FileHash, func() int {
			if segment.Active {
				return 1
			}
			return 0
		}(), segment.SupersededBy,
		segment.CreatedAt, segment.UpdatedAt, segment.Version,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create book segment: %w", err)
	}
	return segment, nil
}

func (s *SQLiteStore) UpdateBookSegment(segment *BookSegment) error {
	segment.UpdatedAt = time.Now()
	segment.Version++
	_, err := s.db.Exec(
		`UPDATE book_segments SET track_number=?, total_tracks=?, updated_at=?, version=? WHERE id=?`,
		segment.TrackNumber, segment.TotalTracks, segment.UpdatedAt, segment.Version, segment.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update book segment: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListBookSegments(bookNumericID int) ([]BookSegment, error) {
	rows, err := s.db.Query(
		`SELECT id, book_id, file_path, format, size_bytes, duration_seconds, track_number, total_tracks, file_hash, active, superseded_by, created_at, updated_at, version
		 FROM book_segments WHERE book_id = ? ORDER BY track_number ASC, created_at ASC`, bookNumericID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var segments []BookSegment
	for rows.Next() {
		var seg BookSegment
		var active int
		if err := rows.Scan(&seg.ID, &seg.BookID, &seg.FilePath, &seg.Format, &seg.SizeBytes, &seg.DurationSec,
			&seg.TrackNumber, &seg.TotalTracks, &seg.FileHash, &active, &seg.SupersededBy, &seg.CreatedAt, &seg.UpdatedAt, &seg.Version); err != nil {
			return nil, err
		}
		seg.Active = active != 0
		segments = append(segments, seg)
	}
	return segments, rows.Err()
}

// GetBookSegmentByID retrieves a single segment by its ULID.
func (s *SQLiteStore) GetBookSegmentByID(segmentID string) (*BookSegment, error) {
	row := s.db.QueryRow(
		`SELECT id, book_id, file_path, format, size_bytes, duration_seconds, track_number, total_tracks, file_hash, active, superseded_by, created_at, updated_at, version
		 FROM book_segments WHERE id = ?`, segmentID,
	)
	var seg BookSegment
	var active int
	if err := row.Scan(&seg.ID, &seg.BookID, &seg.FilePath, &seg.Format, &seg.SizeBytes, &seg.DurationSec,
		&seg.TrackNumber, &seg.TotalTracks, &seg.FileHash, &active, &seg.SupersededBy, &seg.CreatedAt, &seg.UpdatedAt, &seg.Version); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("segment not found: %s", segmentID)
		}
		return nil, err
	}
	seg.Active = active != 0
	return &seg, nil
}

// MoveSegmentsToBook reassigns segments to a different book (by numeric ID).
func (s *SQLiteStore) MoveSegmentsToBook(segmentIDs []string, targetBookNumericID int) error {
	if len(segmentIDs) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()
	for _, segID := range segmentIDs {
		result, err := tx.Exec(
			`UPDATE book_segments SET book_id = ?, updated_at = ?, version = version + 1 WHERE id = ?`,
			targetBookNumericID, now, segID,
		)
		if err != nil {
			return fmt.Errorf("failed to move segment %s: %w", segID, err)
		}
		rows, _ := result.RowsAffected()
		if rows == 0 {
			return fmt.Errorf("segment not found: %s", segID)
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) MergeBookSegments(bookNumericID int, newSegment *BookSegment, supersedeIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Mark old segments as superseded
	for _, oldID := range supersedeIDs {
		if _, err := tx.Exec(
			`UPDATE book_segments SET active = 0, superseded_by = ?, updated_at = ? WHERE id = ? AND book_id = ?`,
			newSegment.ID, time.Now(), oldID, bookNumericID,
		); err != nil {
			return fmt.Errorf("failed to supersede segment %s: %w", oldID, err)
		}
	}

	// Insert the new merged segment
	if newSegment.ID == "" {
		newSegment.ID = ulid.Make().String()
	}
	now := time.Now()
	if _, err := tx.Exec(
		`INSERT INTO book_segments (id, book_id, file_path, format, size_bytes, duration_seconds, track_number, total_tracks, file_hash, active, created_at, updated_at, version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, 1)`,
		newSegment.ID, bookNumericID, newSegment.FilePath, newSegment.Format, newSegment.SizeBytes, newSegment.DurationSec,
		newSegment.TrackNumber, newSegment.TotalTracks, newSegment.FileHash, now, now,
	); err != nil {
		return fmt.Errorf("failed to insert merged segment: %w", err)
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetAllAuthors() ([]Author, error) {
	rows, err := s.db.Query("SELECT id, name FROM authors ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var authors []Author
	for rows.Next() {
		var author Author
		if err := rows.Scan(&author.ID, &author.Name); err != nil {
			return nil, err
		}
		authors = append(authors, author)
	}
	return authors, rows.Err()
}

func (s *SQLiteStore) GetAuthorByID(id int) (*Author, error) {
	var author Author
	err := s.db.QueryRow("SELECT id, name FROM authors WHERE id = ?", id).Scan(&author.ID, &author.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &author, nil
}

func (s *SQLiteStore) GetAuthorsByIDs(ids []int) (map[int]*Author, error) {
	result := make(map[int]*Author, len(ids))
	for _, id := range ids {
		if _, already := result[id]; already {
			continue
		}
		a, err := s.GetAuthorByID(id)
		if err != nil {
			return nil, err
		}
		if a != nil {
			result[id] = a
		}
	}
	return result, nil
}

func (s *SQLiteStore) GetAuthorByName(name string) (*Author, error) {
	var author Author
	// Use LOWER() for case-insensitive lookup
	err := s.db.QueryRow("SELECT id, name FROM authors WHERE LOWER(name) = LOWER(?)", name).Scan(&author.ID, &author.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &author, nil
}

func (s *SQLiteStore) CreateAuthor(name string) (*Author, error) {
	result, err := s.db.Exec("INSERT INTO authors (name) VALUES (?)", name)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Author{ID: int(id), Name: name}, nil
}

func (s *SQLiteStore) DeleteAuthor(id int) error {
	_, err := s.db.Exec("DELETE FROM book_authors WHERE author_id = ?", id)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM authors WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) UpdateAuthorName(id int, name string) error {
	_, err := s.db.Exec("UPDATE authors SET name = ? WHERE id = ?", name, id)
	return err
}

// Author Alias operations

func (s *SQLiteStore) GetAuthorAliases(authorID int) ([]AuthorAlias, error) {
	rows, err := s.db.Query("SELECT id, author_id, alias_name, alias_type, created_at FROM author_aliases WHERE author_id = ? ORDER BY alias_name", authorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aliases []AuthorAlias
	for rows.Next() {
		var a AuthorAlias
		if err := rows.Scan(&a.ID, &a.AuthorID, &a.AliasName, &a.AliasType, &a.CreatedAt); err != nil {
			return nil, err
		}
		aliases = append(aliases, a)
	}
	return aliases, rows.Err()
}

func (s *SQLiteStore) GetAllAuthorAliases() ([]AuthorAlias, error) {
	rows, err := s.db.Query("SELECT id, author_id, alias_name, alias_type, created_at FROM author_aliases ORDER BY alias_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aliases []AuthorAlias
	for rows.Next() {
		var a AuthorAlias
		if err := rows.Scan(&a.ID, &a.AuthorID, &a.AliasName, &a.AliasType, &a.CreatedAt); err != nil {
			return nil, err
		}
		aliases = append(aliases, a)
	}
	return aliases, rows.Err()
}

func (s *SQLiteStore) CreateAuthorAlias(authorID int, aliasName string, aliasType string) (*AuthorAlias, error) {
	result, err := s.db.Exec("INSERT INTO author_aliases (author_id, alias_name, alias_type) VALUES (?, ?, ?)", authorID, aliasName, aliasType)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &AuthorAlias{
		ID:        int(id),
		AuthorID:  authorID,
		AliasName: aliasName,
		AliasType: aliasType,
	}, nil
}

func (s *SQLiteStore) DeleteAuthorAlias(id int) error {
	_, err := s.db.Exec("DELETE FROM author_aliases WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) FindAuthorByAlias(aliasName string) (*Author, error) {
	var a Author
	err := s.db.QueryRow("SELECT a.id, a.name FROM authors a JOIN author_aliases aa ON a.id = aa.author_id WHERE LOWER(aa.alias_name) = LOWER(?)", aliasName).Scan(&a.ID, &a.Name)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Series operations

func (s *SQLiteStore) GetAllSeries() ([]Series, error) {
	rows, err := s.db.Query("SELECT id, name, author_id FROM series ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var series []Series
	for rows.Next() {
		var s Series
		if err := rows.Scan(&s.ID, &s.Name, &s.AuthorID); err != nil {
			return nil, err
		}
		series = append(series, s)
	}
	return series, rows.Err()
}

func (s *SQLiteStore) DeleteSeries(id int) error {
	_, err := s.db.Exec("DELETE FROM series WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) UpdateSeriesName(id int, name string) error {
	_, err := s.db.Exec("UPDATE series SET name = ? WHERE id = ?", name, id)
	return err
}

func (s *SQLiteStore) GetAllSeriesBookCounts() (map[int]int, error) {
	rows, err := s.db.Query(`SELECT series_id, COUNT(*)
		FROM books
		WHERE series_id IS NOT NULL AND COALESCE(marked_for_deletion, 0) = 0 AND COALESCE(is_primary_version, 1) = 1
		GROUP BY series_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[int]int)
	for rows.Next() {
		var seriesID, count int
		if err := rows.Scan(&seriesID, &count); err != nil {
			return nil, err
		}
		counts[seriesID] = count
	}
	return counts, rows.Err()
}

// GetAllSeriesFileCounts returns the number of audio files per series.
// Books with active segments count their segments; books without count as 1.
func (s *SQLiteStore) GetAllSeriesFileCounts() (map[int]int, error) {
	rows, err := s.db.Query(`
		SELECT series_id, id
		FROM books
		WHERE series_id IS NOT NULL AND COALESCE(marked_for_deletion, 0) = 0 AND COALESCE(is_primary_version, 1) = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect book IDs per series
	seriesBooks := make(map[int][]string)
	for rows.Next() {
		var seriesID int
		var bookID string
		if err := rows.Scan(&seriesID, &bookID); err != nil {
			return nil, err
		}
		seriesBooks[seriesID] = append(seriesBooks[seriesID], bookID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Get file counts per book_id from book_files table
	bookFileCounts := make(map[string]int)
	fileRows, err := s.db.Query("SELECT book_id, COUNT(*) FROM book_files WHERE missing = 0 GROUP BY book_id")
	if err == nil {
		defer fileRows.Close()
		for fileRows.Next() {
			var bookID string
			var count int
			if err := fileRows.Scan(&bookID, &count); err != nil {
				break
			}
			bookFileCounts[bookID] = count
		}
	}

	counts := make(map[int]int)
	for seriesID, ids := range seriesBooks {
		total := 0
		for _, id := range ids {
			if fileCount, ok := bookFileCounts[id]; ok && fileCount > 0 {
				total += fileCount
			} else {
				total++ // No files, counts as 1 file
			}
		}
		counts[seriesID] = total
	}

	return counts, nil
}

func (s *SQLiteStore) GetSeriesByID(id int) (*Series, error) {
	var series Series
	err := s.db.QueryRow("SELECT id, name, author_id FROM series WHERE id = ?", id).
		Scan(&series.ID, &series.Name, &series.AuthorID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &series, nil
}

func (s *SQLiteStore) GetSeriesByIDs(ids []int) (map[int]*Series, error) {
	result := make(map[int]*Series, len(ids))
	for _, id := range ids {
		if _, already := result[id]; already {
			continue
		}
		se, err := s.GetSeriesByID(id)
		if err != nil {
			return nil, err
		}
		if se != nil {
			result[id] = se
		}
	}
	return result, nil
}

func (s *SQLiteStore) GetSeriesByName(name string, authorID *int) (*Series, error) {
	var series Series
	var err error
	// Use LOWER() for case-insensitive lookup
	if authorID != nil {
		err = s.db.QueryRow("SELECT id, name, author_id FROM series WHERE LOWER(name) = LOWER(?) AND author_id = ?", name, *authorID).
			Scan(&series.ID, &series.Name, &series.AuthorID)
	} else {
		err = s.db.QueryRow("SELECT id, name, author_id FROM series WHERE LOWER(name) = LOWER(?) AND author_id IS NULL", name).
			Scan(&series.ID, &series.Name, &series.AuthorID)
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &series, nil
}

func (s *SQLiteStore) CreateSeries(name string, authorID *int) (*Series, error) {
	// Check for existing series first (handles NULL author_id which bypasses UNIQUE constraint)
	existing, err := s.GetSeriesByName(name, authorID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	result, err := s.db.Exec("INSERT INTO series (name, author_id) VALUES (?, ?)", name, authorID)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Series{ID: int(id), Name: name, AuthorID: authorID}, nil
}

// Work operations

func (s *SQLiteStore) GetAllWorks() ([]Work, error) {
	rows, err := s.db.Query("SELECT id, title, author_id, series_id, alt_titles FROM works ORDER BY title")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var works []Work
	for rows.Next() {
		var w Work
		var altTitlesStr *string
		if err := rows.Scan(&w.ID, &w.Title, &w.AuthorID, &w.SeriesID, &altTitlesStr); err != nil {
			return nil, err
		}
		if altTitlesStr != nil && *altTitlesStr != "" {
			w.AltTitles = strings.Split(*altTitlesStr, "|")
		}
		works = append(works, w)
	}
	return works, rows.Err()
}

func (s *SQLiteStore) GetWorkByID(id string) (*Work, error) {
	var w Work
	var altTitlesStr *string
	err := s.db.QueryRow("SELECT id, title, author_id, series_id, alt_titles FROM works WHERE id = ?", id).
		Scan(&w.ID, &w.Title, &w.AuthorID, &w.SeriesID, &altTitlesStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if altTitlesStr != nil && *altTitlesStr != "" {
		w.AltTitles = strings.Split(*altTitlesStr, "|")
	}
	return &w, nil
}

func (s *SQLiteStore) CreateWork(work *Work) (*Work, error) {
	if work.ID == "" {
		id, err := newULID()
		if err != nil {
			return nil, err
		}
		work.ID = id
	}
	var altTitlesStr *string
	if len(work.AltTitles) > 0 {
		joined := strings.Join(work.AltTitles, "|")
		altTitlesStr = &joined
	}
	_, err := s.db.Exec("INSERT INTO works (id, title, author_id, series_id, alt_titles, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		work.ID, work.Title, work.AuthorID, work.SeriesID, altTitlesStr, time.Now())
	if err != nil {
		return nil, err
	}
	return work, nil
}

func (s *SQLiteStore) UpdateWork(id string, work *Work) (*Work, error) {
	var altTitlesStr *string
	if len(work.AltTitles) > 0 {
		joined := strings.Join(work.AltTitles, "|")
		altTitlesStr = &joined
	}
	result, err := s.db.Exec("UPDATE works SET title = ?, author_id = ?, series_id = ?, alt_titles = ?, updated_at = ? WHERE id = ?",
		work.Title, work.AuthorID, work.SeriesID, altTitlesStr, time.Now(), id)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, fmt.Errorf("work not found")
	}
	work.ID = id
	return work, nil
}

func (s *SQLiteStore) DeleteWork(id string) error {
	result, err := s.db.Exec("DELETE FROM works WHERE id = ?", id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("work not found")
	}
	return nil
}

func (s *SQLiteStore) GetBooksByWorkID(workID string) ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE work_id = ? ORDER BY title`, bookSelectColumns)
	rows, err := s.db.Query(query, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

// Book operations

func (s *SQLiteStore) GetAllBooks(limit, offset int) ([]Book, error) {
	if limit <= 0 {
		limit = 1_000_000
	}
	if offset < 0 {
		offset = 0
	}
	query := fmt.Sprintf(`SELECT %s FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 ORDER BY title LIMIT ? OFFSET ?`, bookSelectColumns)
	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

// ListBookIDs returns the IDs of all non-deleted books, without
// materializing Book structs. ID-only projection saves the per-row scan
// cost when callers only need the ID set.
func (s *SQLiteStore) ListBookIDs() ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM books WHERE COALESCE(marked_for_deletion, 0) = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0, 1024)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *SQLiteStore) GetAllBookSummaries(limit, offset int) ([]BookSummary, error) {
	if limit <= 0 {
		limit = 1_000_000
	}
	if offset < 0 {
		offset = 0
	}
	query := fmt.Sprintf(`SELECT %s FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 ORDER BY title LIMIT ? OFFSET ?`, bookSummarySelectColumns)
	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []BookSummary
	for rows.Next() {
		var summary BookSummary
		if err := scanBookSummary(rows, &summary); err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

func (s *SQLiteStore) GetDistinctGenres() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT genre FROM books WHERE genre IS NOT NULL AND genre != '' AND COALESCE(marked_for_deletion,0)=0 ORDER BY genre`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetDistinctLanguages() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT language FROM books WHERE language IS NOT NULL AND language != '' AND COALESCE(marked_for_deletion,0)=0 ORDER BY language`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetBookByID(id string) (*Book, error) {
	var book Book
	query := fmt.Sprintf(`SELECT %s FROM books WHERE id = ?`, bookSelectColumns)
	err := scanBook(s.db.QueryRow(query, id), &book)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &book, nil
}

func (s *SQLiteStore) GetBookByFilePath(path string) (*Book, error) {
	var book Book
	query := fmt.Sprintf(`SELECT %s FROM books WHERE file_path = ?`, bookSelectColumns)
	err := scanBook(s.db.QueryRow(query, path), &book)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &book, nil
}

func (s *SQLiteStore) GetBookByITunesPersistentID(persistentID string) (*Book, error) {
	var book Book
	query := fmt.Sprintf(`SELECT %s FROM books WHERE itunes_persistent_id = ? LIMIT 1`, bookSelectColumns)
	err := scanBook(s.db.QueryRow(query, persistentID), &book)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &book, nil
}

func (s *SQLiteStore) GetBookByFileHash(hash string) (*Book, error) {
	var book Book
	query := fmt.Sprintf(`SELECT %s FROM books WHERE file_hash = ? LIMIT 1`, bookSelectColumns)
	err := scanBook(s.db.QueryRow(query, hash), &book)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &book, nil
}

func (s *SQLiteStore) GetBookByOriginalHash(hash string) (*Book, error) {
	var book Book
	query := fmt.Sprintf(`SELECT %s FROM books WHERE original_file_hash = ? LIMIT 1`, bookSelectColumns)
	err := scanBook(s.db.QueryRow(query, hash), &book)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &book, nil
}

func (s *SQLiteStore) GetBookByOrganizedHash(hash string) (*Book, error) {
	var book Book
	query := fmt.Sprintf(`SELECT %s FROM books WHERE organized_file_hash = ? LIMIT 1`, bookSelectColumns)
	err := scanBook(s.db.QueryRow(query, hash), &book)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &book, nil
}

// GetBookBySegmentFileHash returns the parent Book of the first book_file whose
// file_hash or original_file_hash matches hash. Used by the scanner multi-file
// dedup tally so individual segment files can be matched against existing books
// without assuming the whole containing directory is the same book.
func (s *SQLiteStore) GetBookBySegmentFileHash(hash string) (*Book, error) {
	if hash == "" {
		return nil, nil
	}
	query := fmt.Sprintf(`
		SELECT %s FROM books
		JOIN book_files ON book_files.book_id = books.id
		WHERE (book_files.file_hash = ? OR book_files.original_file_hash = ?)
		LIMIT 1`, bookSelectColumns)
	var book Book
	err := scanBook(s.db.QueryRow(query, hash, hash), &book)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &book, nil
}

// GetBooksByMetadataSourceHash returns all non-merged books whose metadata_source_hash
// matches the given value. Typically returns 0 or 1 books; 2+ means duplicates
// were applied from the exact same external record.
func (s *SQLiteStore) GetBooksByMetadataSourceHash(hash string) ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE metadata_source_hash = ? AND merged_into_book_id IS NULL ORDER BY created_at`, bookSelectColumns)
	rows, err := s.db.Query(query, hash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var books []Book
	for rows.Next() {
		var b Book
		if err := scanBook(rows, &b); err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	return books, rows.Err()
}

// GetDuplicateBooks returns groups of books with identical file hashes
// Only returns groups with 2+ books (actual duplicates)
func (s *SQLiteStore) GetDuplicateBooks() ([][]Book, error) {
	// Find all hashes that have duplicates (appear more than once)
	// Use COALESCE to handle null hashes and prefer organized_file_hash
	hashQuery := `
		SELECT COALESCE(organized_file_hash, file_hash) as hash, COUNT(*) as count
		FROM books
		WHERE COALESCE(organized_file_hash, file_hash) IS NOT NULL
		  AND COALESCE(marked_for_deletion, 0) = 0
		GROUP BY COALESCE(organized_file_hash, file_hash)
		HAVING count > 1
		ORDER BY count DESC
	`

	hashRows, err := s.db.Query(hashQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query duplicate hashes: %w", err)
	}
	defer hashRows.Close()

	var duplicateGroups [][]Book
	for hashRows.Next() {
		var hash string
		var count int
		if err := hashRows.Scan(&hash, &count); err != nil {
			return nil, fmt.Errorf("failed to scan hash row: %w", err)
		}

		// Get all books with this hash
		booksQuery := fmt.Sprintf(`
				SELECT %s FROM books
				WHERE COALESCE(organized_file_hash, file_hash) = ?
				  AND COALESCE(marked_for_deletion, 0) = 0
				ORDER BY file_path
			`, bookSelectColumns)

		bookRows, err := s.db.Query(booksQuery, hash)
		if err != nil {
			return nil, fmt.Errorf("failed to query books for hash %s: %w", hash, err)
		}

		var group []Book
		for bookRows.Next() {
			var book Book
			if err := scanBook(bookRows, &book); err != nil {
				bookRows.Close()
				return nil, fmt.Errorf("failed to scan book: %w", err)
			}
			group = append(group, book)
		}
		bookRows.Close()

		if err := bookRows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating book rows: %w", err)
		}
		// Only add groups with 2+ books
		if len(group) >= 2 {
			duplicateGroups = append(duplicateGroups, group)
		}
	}

	if err := hashRows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating hash rows: %w", err)
	}
	return duplicateGroups, nil
}

// GetFolderDuplicates detects potential duplicates by grouping books
// that share the same parent directory and title (case-insensitive).
// This catches M4B + MP3 versions of the same book in the same folder.
// It prefers single-file M4B over multi-file formats.
// GetBooksByTitleInDir finds books with the given normalized (lowercased) title
// in the given directory path. Results are ordered so M4B files come first.
func (s *SQLiteStore) GetBooksByTitleInDir(normalizedTitle, dirPath string) ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE LOWER(title) = ? AND file_path LIKE ? AND COALESCE(marked_for_deletion, 0) = 0
		ORDER BY CASE WHEN format = 'm4b' THEN 0 ELSE 1 END`, bookSelectColumns)
	rows, err := s.db.Query(query, normalizedTitle, dirPath+"/%")
	if err != nil {
		return nil, fmt.Errorf("failed to query books by title in dir: %w", err)
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

func (s *SQLiteStore) GetFolderDuplicates() ([][]Book, error) {
	// Group by parent directory + lower(title) where there are 2+ books
	query := `
		WITH book_dirs AS (
			SELECT id, LOWER(title) as ltitle,
			       SUBSTR(file_path, 1, LENGTH(file_path) - LENGTH(REPLACE(file_path, '/', '')) - LENGTH(SUBSTR(file_path, LENGTH(file_path) - LENGTH(REPLACE(file_path, '/', '')) + 1))) as dir
			FROM books
			WHERE COALESCE(marked_for_deletion, 0) = 0
			  AND (version_group_id IS NULL OR version_group_id = '')
		)
		SELECT dir, ltitle, COUNT(*) as cnt
		FROM book_dirs
		WHERE dir != '' AND ltitle != ''
		GROUP BY dir, ltitle
		HAVING cnt > 1
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query folder duplicates: %w", err)
	}
	defer rows.Close()

	var groups [][]Book
	for rows.Next() {
		var dir, ltitle string
		var cnt int
		if err := rows.Scan(&dir, &ltitle, &cnt); err != nil {
			return nil, err
		}
		// Fetch the actual books
		booksQuery := fmt.Sprintf(`
			SELECT %s FROM books
			WHERE LOWER(title) = ? AND file_path LIKE ?
			  AND COALESCE(marked_for_deletion, 0) = 0
			  AND (version_group_id IS NULL OR version_group_id = '')
			ORDER BY CASE WHEN format = 'm4b' THEN 0 ELSE 1 END, file_path
		`, bookSelectColumns)
		bookRows, err := s.db.Query(booksQuery, ltitle, dir+"/%")
		if err != nil {
			return nil, err
		}
		var group []Book
		for bookRows.Next() {
			var book Book
			if err := scanBook(bookRows, &book); err != nil {
				bookRows.Close()
				return nil, err
			}
			group = append(group, book)
		}
		bookRows.Close()
		if len(group) >= 2 {
			groups = append(groups, group)
		}
	}
	return groups, nil
}

// normalizeTitle normalizes a book title for comparison: lowercase, strip articles,
// remove parenthesized suffixes like "(Unabridged)", collapse whitespace.

// GetDuplicateBooksByMetadata finds books that appear to be duplicates based on
// title + author matching. Books with the same author_id and similar titles
// (after normalization) are grouped together. The threshold parameter controls
// how similar titles must be (0.0–1.0, where 1.0 = exact match). Duration is
// used as an additional signal: if both books have duration, they must be within
// 5% to be grouped.
func (s *SQLiteStore) GetDuplicateBooksByMetadata(threshold float64) ([][]Book, error) {
	// Fetch all non-deleted books that aren't already in a version group
	query := fmt.Sprintf(`
		SELECT %s FROM books
		WHERE COALESCE(marked_for_deletion, 0) = 0
		  AND title != ''
		  AND author_id IS NOT NULL
		ORDER BY author_id, LOWER(title)
	`, bookSelectColumns)

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query books for metadata dedup: %w", err)
	}
	defer rows.Close()

	var allBooks []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		allBooks = append(allBooks, book)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Group books by author_id first, then find title matches within each author group
	authorGroups := map[int][]Book{}
	for _, b := range allBooks {
		if b.AuthorID == nil {
			continue
		}
		authorGroups[*b.AuthorID] = append(authorGroups[*b.AuthorID], b)
	}

	var duplicateGroups [][]Book

	for _, books := range authorGroups {
		if len(books) < 2 {
			continue
		}
		// Track which books have been assigned to a group
		assigned := make([]bool, len(books))

		for i := 0; i < len(books); i++ {
			if assigned[i] {
				continue
			}
			group := []Book{books[i]}
			assigned[i] = true

			normI := normalizeTitle(books[i].Title)

			for j := i + 1; j < len(books); j++ {
				if assigned[j] {
					continue
				}
				normJ := normalizeTitle(books[j].Title)
				sim := jaroWinkler(normI, normJ)
				if sim < threshold {
					continue
				}
				// If both have duration, check within 5%
				if books[i].Duration != nil && books[j].Duration != nil {
					di := float64(*books[i].Duration)
					dj := float64(*books[j].Duration)
					if di > 0 && dj > 0 {
						ratio := di / dj
						if ratio < 0.95 || ratio > 1.05 {
							continue
						}
					}
				}
				group = append(group, books[j])
				assigned[j] = true
			}

			if len(group) >= 2 {
				duplicateGroups = append(duplicateGroups, group)
			}
		}
	}

	return duplicateGroups, nil
}

func (s *SQLiteStore) GetBooksBySeriesID(seriesID int) ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE series_id = ? AND COALESCE(marked_for_deletion, 0) = 0 ORDER BY series_sequence, title`, bookSelectColumns)
	rows, err := s.db.Query(query, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

func (s *SQLiteStore) GetBooksByAuthorID(authorID int) ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE author_id = ? AND COALESCE(marked_for_deletion, 0) = 0 ORDER BY title`, bookSelectColumns)
	rows, err := s.db.Query(query, authorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

func (s *SQLiteStore) GetBookAuthors(bookID string) ([]BookAuthor, error) {
	rows, err := s.db.Query(`SELECT book_id, author_id, role, position FROM book_authors WHERE book_id = ? ORDER BY position`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var authors []BookAuthor
	for rows.Next() {
		var ba BookAuthor
		if err := rows.Scan(&ba.BookID, &ba.AuthorID, &ba.Role, &ba.Position); err != nil {
			return nil, err
		}
		authors = append(authors, ba)
	}
	return authors, rows.Err()
}

func (s *SQLiteStore) SetBookAuthors(bookID string, authors []BookAuthor) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM book_authors WHERE book_id = ?`, bookID); err != nil {
		return err
	}

	for _, ba := range authors {
		if _, err := tx.Exec(
			`INSERT INTO book_authors (book_id, author_id, role, position) VALUES (?, ?, ?, ?)`,
			bookID, ba.AuthorID, ba.Role, ba.Position,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetBooksByAuthorIDWithRole(authorID int) ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE id IN (
		SELECT book_id FROM book_authors WHERE author_id = ?
	) AND COALESCE(marked_for_deletion, 0) = 0 ORDER BY title`, bookSelectColumns)
	rows, err := s.db.Query(query, authorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

func (s *SQLiteStore) GetAllAuthorBookCounts() (map[int]int, error) {
	rows, err := s.db.Query(`SELECT ba.author_id, COUNT(DISTINCT ba.book_id)
		FROM book_authors ba
		JOIN books b ON b.id = ba.book_id
		WHERE COALESCE(b.marked_for_deletion, 0) = 0 AND COALESCE(b.is_primary_version, 1) = 1
		GROUP BY ba.author_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[int]int)
	for rows.Next() {
		var authorID, count int
		if err := rows.Scan(&authorID, &count); err != nil {
			return nil, err
		}
		counts[authorID] = count
	}
	return counts, rows.Err()
}

// GetAllAuthorFileCounts returns the number of audio files per author.
// Books with active segments count their segments; books without count as 1.
func (s *SQLiteStore) GetAllAuthorFileCounts() (map[int]int, error) {
	// For each author's books, calculate file count:
	// - Books with active segments: count segments
	// - Books without segments: count as 1 file
	rows, err := s.db.Query(`
		SELECT ba.author_id, b.id
		FROM book_authors ba
		JOIN books b ON b.id = ba.book_id
		WHERE COALESCE(b.marked_for_deletion, 0) = 0 AND COALESCE(b.is_primary_version, 1) = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect book IDs per author
	authorBooks := make(map[int][]string)
	for rows.Next() {
		var authorID int
		var bookID string
		if err := rows.Scan(&authorID, &bookID); err != nil {
			return nil, err
		}
		authorBooks[authorID] = append(authorBooks[authorID], bookID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Get file counts per book from book_files table
	bookFileCounts := make(map[string]int)
	fileRows, err := s.db.Query("SELECT book_id, COUNT(*) FROM book_files WHERE missing = 0 GROUP BY book_id")
	if err == nil {
		defer fileRows.Close()
		for fileRows.Next() {
			var bookID string
			var count int
			if err := fileRows.Scan(&bookID, &count); err != nil {
				break
			}
			bookFileCounts[bookID] = count
		}
	}

	// Calculate file counts per author
	counts := make(map[int]int)
	for authorID, ids := range authorBooks {
		total := 0
		for _, id := range ids {
			if fileCount, ok := bookFileCounts[id]; ok && fileCount > 0 {
				total += fileCount
			} else {
				total++ // No files, counts as 1 file
			}
		}
		counts[authorID] = total
	}

	return counts, nil
}

// --- Narrator methods ---

func (s *SQLiteStore) CreateNarrator(name string) (*Narrator, error) {
	result, err := s.db.Exec("INSERT INTO narrators (name) VALUES (?)", name)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetNarratorByID(int(id))
}

func (s *SQLiteStore) GetNarratorByID(id int) (*Narrator, error) {
	var n Narrator
	err := s.db.QueryRow("SELECT id, name, created_at FROM narrators WHERE id = ?", id).Scan(&n.ID, &n.Name, &n.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *SQLiteStore) GetNarratorByName(name string) (*Narrator, error) {
	var n Narrator
	err := s.db.QueryRow("SELECT id, name, created_at FROM narrators WHERE LOWER(name) = LOWER(?)", name).Scan(&n.ID, &n.Name, &n.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *SQLiteStore) ListNarrators() ([]Narrator, error) {
	rows, err := s.db.Query("SELECT id, name, created_at FROM narrators ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var narrators []Narrator
	for rows.Next() {
		var n Narrator
		if err := rows.Scan(&n.ID, &n.Name, &n.CreatedAt); err != nil {
			return nil, err
		}
		narrators = append(narrators, n)
	}
	return narrators, rows.Err()
}

func (s *SQLiteStore) GetBookNarrators(bookID string) ([]BookNarrator, error) {
	rows, err := s.db.Query(`SELECT book_id, narrator_id, role, position FROM book_narrators WHERE book_id = ? ORDER BY position`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var narrators []BookNarrator
	for rows.Next() {
		var bn BookNarrator
		if err := rows.Scan(&bn.BookID, &bn.NarratorID, &bn.Role, &bn.Position); err != nil {
			return nil, err
		}
		narrators = append(narrators, bn)
	}
	return narrators, rows.Err()
}

func (s *SQLiteStore) SetBookNarrators(bookID string, narrators []BookNarrator) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM book_narrators WHERE book_id = ?`, bookID); err != nil {
		return err
	}

	for _, bn := range narrators {
		if _, err := tx.Exec(
			`INSERT INTO book_narrators (book_id, narrator_id, role, position) VALUES (?, ?, ?, ?)`,
			bookID, bn.NarratorID, bn.Role, bn.Position,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) CreateBook(book *Book) (*Book, error) {
	// Generate ULID if not provided
	if book.ID == "" {
		id, err := newULID()
		if err != nil {
			return nil, err
		}
		book.ID = id
	}

	// Set timestamps
	now := time.Now()
	book.CreatedAt = &now
	book.UpdatedAt = &now

	// Set initial iTunes sync status if not already set
	if book.ITunesSyncStatus == nil {
		if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" {
			synced := "synced" // came from iTunes — assume in sync
			book.ITunesSyncStatus = &synced
		}
		// Books without a PID get nil status (unlinked) — set when they're added to iTunes
	}

	// Wrap in a transaction: book insert + path history write must be atomic
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("CreateBook begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.Info("CreateBook rollback", "rbErr", rbErr)
			}
		}
	}()

	query := `INSERT INTO books (
		id, title, author_id, series_id, series_sequence, file_path, original_filename,
		format, duration, work_id, narrator, edition, description, language, publisher, genre,
		print_year, audiobook_release_year, isbn10, isbn13, asin,
		open_library_id, hardcover_id, google_books_id,
		itunes_persistent_id, itunes_date_added, itunes_play_count, itunes_last_played,
		itunes_rating, itunes_bookmark, itunes_import_source, itunes_path,
		file_hash, file_size, bitrate_kbps, codec, sample_rate_hz, channels,
		bit_depth, quality, is_primary_version, version_group_id, version_notes,
		original_file_hash, organized_file_hash, library_state, quantity, marked_for_deletion, marked_for_deletion_at,
		created_at, updated_at, cover_url, narrators_json,
		last_organize_operation_id, last_organized_at, itunes_sync_status,
		source_import_path
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = tx.Exec(query,
		book.ID, book.Title, book.AuthorID, book.SeriesID, book.SeriesSequence, book.FilePath, book.OriginalFilename,
		book.Format, book.Duration, book.WorkID, book.Narrator, book.Edition, book.Description, book.Language, book.Publisher, book.Genre,
		book.PrintYear, book.AudiobookReleaseYear, book.ISBN10, book.ISBN13, book.ASIN,
		book.OpenLibraryID, book.HardcoverID, book.GoogleBooksID,
		book.ITunesPersistentID, book.ITunesDateAdded, book.ITunesPlayCount, book.ITunesLastPlayed,
		book.ITunesRating, book.ITunesBookmark, book.ITunesImportSource, book.ITunesPath,
		book.FileHash, book.FileSize, book.Bitrate, book.Codec, book.SampleRate, book.Channels,
		book.BitDepth, book.Quality, book.IsPrimaryVersion, book.VersionGroupID, book.VersionNotes,
		book.OriginalFileHash, book.OrganizedFileHash, book.LibraryState, book.Quantity, book.MarkedForDeletion, book.MarkedForDeletionAt,
		book.CreatedAt, book.UpdatedAt, book.CoverURL, book.NarratorsJSON,
		book.LastOrganizeOperationID, book.LastOrganizedAt, book.ITunesSyncStatus,
		book.SourceImportPath,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateBook insert: %w", err)
	}

	// Record the original import path so full provenance is preserved forever.
	// book_path_history is created in migration 35; skip gracefully on older schemas.
	_, err = tx.Exec(
		`INSERT INTO book_path_history (book_id, old_path, new_path, change_type) VALUES (?, ?, ?, ?)`,
		book.ID, "", book.FilePath, "import",
	)
	if err != nil && !strings.Contains(err.Error(), "no such table") {
		return nil, fmt.Errorf("CreateBook record path: %w", err)
	}
	err = nil // reset so defer rollback doesn't fire on a skipped path-history write

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("CreateBook commit: %w", err)
	}

	return book, nil
}

// metadataChanged returns true if any user-visible metadata field differs between
// old and new. Internal-only fields (FileHash, LibraryState, ITunes*, etc.) are
// intentionally excluded so that system updates do not bump metadata_updated_at.
func metadataChanged(old, new *Book) bool {
	if old.Title != new.Title {
		return true
	}
	if !equalIntPtr(old.AuthorID, new.AuthorID) {
		return true
	}
	if !equalIntPtr(old.SeriesID, new.SeriesID) {
		return true
	}
	if !equalIntPtr(old.SeriesSequence, new.SeriesSequence) {
		return true
	}
	if !equalStringPtr(old.Narrator, new.Narrator) {
		return true
	}
	if !equalStringPtr(old.Publisher, new.Publisher) {
		return true
	}
	if !equalStringPtr(old.Language, new.Language) {
		return true
	}
	if !equalIntPtr(old.AudiobookReleaseYear, new.AudiobookReleaseYear) {
		return true
	}
	if !equalIntPtr(old.PrintYear, new.PrintYear) {
		return true
	}
	if !equalStringPtr(old.ISBN10, new.ISBN10) {
		return true
	}
	if !equalStringPtr(old.ISBN13, new.ISBN13) {
		return true
	}
	if !equalStringPtr(old.CoverURL, new.CoverURL) {
		return true
	}
	if !equalStringPtr(old.NarratorsJSON, new.NarratorsJSON) {
		return true
	}
	return false
}

// equalStringPtr returns true if both pointers are nil, or both point to equal strings.
func equalStringPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// equalIntPtr returns true if both pointers are nil, or both point to equal ints.
func equalIntPtr(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func (s *SQLiteStore) UpdateBook(id string, book *Book) (*Book, error) {
	// Always stamp updated_at — this tracks every DB write for debugging.
	now := time.Now()
	book.UpdatedAt = &now

	// Fetch the current book to detect whether metadata actually changed.
	current, fetchErr := s.GetBookByID(id)

	if fetchErr == nil && current != nil && metadataChanged(current, book) {
		book.MetadataUpdatedAt = &now
		// Auto-dirty: if metadata changed and this book has an iTunes PID,
		// mark it as needing a write-back to the iTunes library.
		if current.ITunesPersistentID != nil && *current.ITunesPersistentID != "" {
			dirty := "dirty"
			book.ITunesSyncStatus = &dirty
		}
	} else if fetchErr == nil && current != nil {
		// Preserve the existing metadata_updated_at value — nothing changed.
		book.MetadataUpdatedAt = current.MetadataUpdatedAt
		// Also preserve iTunes sync status if no metadata change.
		if book.ITunesSyncStatus == nil {
			book.ITunesSyncStatus = current.ITunesSyncStatus
		}
	}

	// Never touch last_written_at in UpdateBook. It is set by SetLastWrittenAt only.
	if current != nil {
		book.LastWrittenAt = current.LastWrittenAt
	}

	query := `UPDATE books SET
		title = ?, author_id = ?, series_id = ?, series_sequence = ?,
		file_path = ?, original_filename = ?, format = ?, duration = ?,
		work_id = ?, narrator = ?, edition = ?, description = ?, language = ?, publisher = ?, genre = ?,
		print_year = ?, audiobook_release_year = ?, isbn10 = ?, isbn13 = ?, asin = ?,
		open_library_id = ?, hardcover_id = ?, google_books_id = ?,
		itunes_persistent_id = ?, itunes_date_added = ?, itunes_play_count = ?, itunes_last_played = ?,
		itunes_rating = ?, itunes_bookmark = ?, itunes_import_source = ?, itunes_path = ?,
		file_hash = ?, file_size = ?, bitrate_kbps = ?, codec = ?, sample_rate_hz = ?, channels = ?,
		bit_depth = ?, quality = ?, is_primary_version = ?, version_group_id = ?, version_notes = ?,
		original_file_hash = ?, organized_file_hash = ?, library_state = ?, quantity = ?,
		marked_for_deletion = ?, marked_for_deletion_at = ?,
		quarantine_reason = ?, quarantined_at = ?,
		updated_at = ?, metadata_updated_at = ?, last_written_at = ?,
		metadata_review_status = ?, cover_url = ?, narrators_json = ?,
		last_organize_operation_id = ?, last_organized_at = ?, itunes_sync_status = ?,
		audible_rating_overall = ?, audible_rating_performance = ?, audible_rating_story = ?,
		audible_rating_count = ?, audible_num_reviews = ?,
		google_rating_average = ?, google_rating_count = ?,
		user_rating_overall = ?, user_rating_story = ?, user_rating_performance = ?, user_rating_notes = ?,
		metadata_source_hash = ?,
		source_import_path = COALESCE(source_import_path, ?)
	WHERE id = ?`
	result, err := s.db.Exec(query,
		book.Title, book.AuthorID, book.SeriesID, book.SeriesSequence,
		book.FilePath, book.OriginalFilename, book.Format, book.Duration,
		book.WorkID, book.Narrator, book.Edition, book.Description, book.Language, book.Publisher, book.Genre,
		book.PrintYear, book.AudiobookReleaseYear, book.ISBN10, book.ISBN13, book.ASIN,
		book.OpenLibraryID, book.HardcoverID, book.GoogleBooksID,
		book.ITunesPersistentID, book.ITunesDateAdded, book.ITunesPlayCount, book.ITunesLastPlayed,
		book.ITunesRating, book.ITunesBookmark, book.ITunesImportSource, book.ITunesPath,
		book.FileHash, book.FileSize, book.Bitrate, book.Codec, book.SampleRate, book.Channels,
		book.BitDepth, book.Quality, book.IsPrimaryVersion, book.VersionGroupID, book.VersionNotes,
		book.OriginalFileHash, book.OrganizedFileHash, book.LibraryState, book.Quantity,
		book.MarkedForDeletion, book.MarkedForDeletionAt,
		book.QuarantineReason, book.QuarantinedAt,
		book.UpdatedAt, book.MetadataUpdatedAt, book.LastWrittenAt,
		book.MetadataReviewStatus, book.CoverURL, book.NarratorsJSON,
		book.LastOrganizeOperationID, book.LastOrganizedAt, book.ITunesSyncStatus,
		book.AudibleRatingOverall, book.AudibleRatingPerformance, book.AudibleRatingStory,
		book.AudibleRatingCount, book.AudibleNumReviews,
		book.GoogleRatingAverage, book.GoogleRatingCount,
		book.UserRatingOverall, book.UserRatingStory, book.UserRatingPerformance, book.UserRatingNotes,
		book.MetadataSourceHash,
		book.SourceImportPath, id,
	)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, fmt.Errorf("book not found")
	}
	book.ID = id
	return book, nil
}

// UpdateBookRating updates only the user rating fields for the given book ID.
// Fields are applied selectively: nil pointer = no change, Clear* = set to NULL,
// non-nil pointer = set to the value.
func (s *SQLiteStore) UpdateBookRating(id string, req UpdateBookRatingRequest) error {
	setClauses := []string{}
	args := []interface{}{}

	if req.ClearOverall {
		setClauses = append(setClauses, "user_rating_overall = NULL")
	} else if req.Overall != nil {
		setClauses = append(setClauses, "user_rating_overall = ?")
		args = append(args, *req.Overall)
	}

	if req.ClearStory {
		setClauses = append(setClauses, "user_rating_story = NULL")
	} else if req.Story != nil {
		setClauses = append(setClauses, "user_rating_story = ?")
		args = append(args, *req.Story)
	}

	if req.ClearPerf {
		setClauses = append(setClauses, "user_rating_performance = NULL")
	} else if req.Performance != nil {
		setClauses = append(setClauses, "user_rating_performance = ?")
		args = append(args, *req.Performance)
	}

	if req.ClearNotes {
		setClauses = append(setClauses, "user_rating_notes = NULL")
	} else if req.Notes != nil {
		setClauses = append(setClauses, "user_rating_notes = ?")
		args = append(args, *req.Notes)
	}

	if len(setClauses) == 0 {
		return nil // nothing to do
	}

	query := "UPDATE books SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	args = append(args, id)

	result, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("UpdateBookRating: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("UpdateBookRating rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("book not found")
	}
	return nil
}

// SetLastWrittenAt stamps the last_written_at timestamp for book id.
func (s *SQLiteStore) SetLastWrittenAt(id string, t time.Time) error {
	_, err := s.db.Exec(
		`UPDATE books SET last_written_at = ? WHERE id = ?`,
		t, id,
	)
	return err
}

// MarkITunesSynced sets itunes_sync_status to "synced" for the given book IDs.
func (s *SQLiteStore) MarkITunesSynced(bookIDs []string) (int64, error) {
	if len(bookIDs) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(bookIDs))
	args := make([]interface{}, len(bookIDs))
	for i, id := range bookIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(
		`UPDATE books SET itunes_sync_status = 'synced' WHERE id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	result, err := s.db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetITunesPurgePendingBooks returns all books with itunes_sync_status = "purge_pending"
// and a non-null iTunes persistent ID. These are quarantined books that should be
// removed from iTunes before their PID linkage is cleared.
func (s *SQLiteStore) GetITunesPurgePendingBooks() ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE itunes_sync_status = 'purge_pending' AND itunes_persistent_id IS NOT NULL`, bookSelectColumns)
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

// GetITunesDirtyBooks returns all primary books with itunes_sync_status = "dirty".
func (s *SQLiteStore) GetITunesDirtyBooks() ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE itunes_sync_status = 'dirty' AND (is_primary_version = 1 OR is_primary_version IS NULL)`, bookSelectColumns)
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

func (s *SQLiteStore) DeleteBook(id string) error {
	// Wrap in a transaction: book delete + metadata_states delete must be atomic
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("DeleteBook begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.Info("DeleteBook rollback", "rbErr", rbErr)
			}
		}
	}()

	result, err := tx.Exec("DELETE FROM books WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("DeleteBook exec: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("DeleteBook rows affected: %w", err)
	}
	if rowsAffected == 0 {
		// Set outer err so the deferred rollback fires and releases the transaction.
		err = fmt.Errorf("book not found")
		return err
	}
	if _, err = tx.Exec("DELETE FROM metadata_states WHERE book_id = ?", id); err != nil {
		return fmt.Errorf("DeleteBook metadata delete: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("DeleteBook commit: %w", err)
	}

	return nil
}

func (s *SQLiteStore) SearchBooks(query string, limit, offset int) ([]Book, error) {
	// Fetch a larger pool for fuzzy re-ranking (we re-rank in Go, then paginate)
	fetchLimit := limit * 3
	if fetchLimit < 100 {
		fetchLimit = 100
	}

	// Search by title (FTS5) and author name (LIKE on authors table)
	ftsQuery := sanitizeFTS5Query(query)
	likeParam := "%" + query + "%"

	// Try FTS5 for title + LIKE for author via UNION
	bq := bookSelectColumnsQualified
	searchSQL := fmt.Sprintf(
		`SELECT %s FROM books
		 JOIN books_fts ON books.rowid = books_fts.rowid
		 WHERE books_fts MATCH ? AND COALESCE(books.marked_for_deletion, 0) = 0
		 UNION
		 SELECT %s FROM books
		 JOIN authors ON books.author_id = authors.id
		 WHERE authors.name LIKE ? AND COALESCE(books.marked_for_deletion, 0) = 0
		 UNION
		 SELECT %s FROM books
		 JOIN book_authors ba ON ba.book_id = books.id
		 JOIN authors a2 ON ba.author_id = a2.id
		 WHERE a2.name LIKE ? AND COALESCE(books.marked_for_deletion, 0) = 0
		 LIMIT ?`, bq, bq, bq)

	rows, err := s.db.Query(searchSQL, ftsQuery, likeParam, likeParam, fetchLimit)
	if err != nil {
		// Fall back to pure LIKE if FTS5 not available
		likeSQL := fmt.Sprintf(
			`SELECT %s FROM books
			 WHERE books.title LIKE ? AND COALESCE(books.marked_for_deletion, 0) = 0
			 UNION
			 SELECT %s FROM books
			 JOIN authors ON books.author_id = authors.id
			 WHERE authors.name LIKE ? AND COALESCE(books.marked_for_deletion, 0) = 0
			 UNION
			 SELECT %s FROM books
			 JOIN book_authors ba ON ba.book_id = books.id
			 JOIN authors a2 ON ba.author_id = a2.id
			 WHERE a2.name LIKE ? AND COALESCE(books.marked_for_deletion, 0) = 0
			 LIMIT ?`, bq, bq, bq)
		rows, err = s.db.Query(likeSQL, likeParam, likeParam, likeParam, fetchLimit)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Re-rank using fuzzy scoring
	books = fuzzyRankBooks(query, books)

	// Apply pagination after ranking
	if offset >= len(books) {
		return []Book{}, nil
	}
	end := offset + limit
	if end > len(books) {
		end = len(books)
	}
	return books[offset:end], nil
}

// fuzzyRankBooks re-ranks books by fuzzy match score against the query.

func (s *SQLiteStore) CountBooks() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 AND COALESCE(is_primary_version, 1) = 1").Scan(&count)
	return count, err
}

// CountFiles returns the total number of audio files across all books.
// Books with book_files count their non-missing files; books without files count as 1 file each.
func (s *SQLiteStore) CountFiles() (int, error) {
	// Count non-missing book files
	var fileCount int
	err := s.db.QueryRow("SELECT COUNT(*) FROM book_files WHERE missing = 0").Scan(&fileCount)
	if err != nil {
		// Table may not exist yet; treat as 0
		fileCount = 0
	}

	// Count all primary, non-deleted books
	var bookCount int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 AND COALESCE(is_primary_version, 1) = 1`).Scan(&bookCount)
	if err != nil {
		return 0, err
	}

	// Count distinct books that have non-missing files
	var booksWithFiles int
	err = s.db.QueryRow("SELECT COUNT(DISTINCT book_id) FROM book_files WHERE missing = 0").Scan(&booksWithFiles)
	if err != nil {
		// Table may not exist yet; treat as 0
		booksWithFiles = 0
	}

	// Total files = non-missing book files + books without any files (each counts as 1 file)
	return fileCount + (bookCount - booksWithFiles), nil
}

func (s *SQLiteStore) CountAuthors() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM authors").Scan(&count)
	return count, err
}

func (s *SQLiteStore) CountSeries() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM series").Scan(&count)
	return count, err
}

func (s *SQLiteStore) GetBookCountsByLocation(rootDir string) (library, import_ int, err error) {
	const primaryFilter = " AND COALESCE(is_primary_version, 1) = 1"
	if rootDir == "" {
		// No root dir configured, all books are imports
		err = s.db.QueryRow("SELECT COUNT(*) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0" + primaryFilter).Scan(&import_)
		return 0, import_, err
	}
	// Normalise: strip trailing slash so LIKE '/path/%' works regardless of
	// how the caller stored the root dir.
	dir := strings.TrimRight(rootDir, "/")
	err = s.db.QueryRow("SELECT COUNT(*) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 AND (file_path LIKE ? OR file_path = ?)"+primaryFilter, dir+"/%", dir).Scan(&library)
	if err != nil {
		return
	}
	err = s.db.QueryRow("SELECT COUNT(*) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 AND file_path NOT LIKE ? AND file_path != ?"+primaryFilter, dir+"/%", dir).Scan(&import_)
	return
}

func (s *SQLiteStore) GetBookSizesByLocation(rootDir string) (librarySize, importSize int64, err error) {
	if rootDir == "" {
		err = s.db.QueryRow("SELECT COALESCE(SUM(file_size), 0) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0").Scan(&importSize)
		return 0, importSize, err
	}
	dir := strings.TrimRight(rootDir, "/")
	err = s.db.QueryRow("SELECT COALESCE(SUM(file_size), 0) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 AND (file_path LIKE ? OR file_path = ?)", dir+"/%", dir).Scan(&librarySize)
	if err != nil {
		return
	}
	err = s.db.QueryRow("SELECT COALESCE(SUM(file_size), 0) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0 AND file_path NOT LIKE ? AND file_path != ?", dir+"/%", dir).Scan(&importSize)
	return
}

func (s *SQLiteStore) ListSoftDeletedBooks(limit, offset int, olderThan *time.Time) ([]Book, error) {
	if limit <= 0 {
		limit = 1_000_000
	}
	if offset < 0 {
		offset = 0
	}
	query := fmt.Sprintf(`SELECT %s FROM books WHERE COALESCE(marked_for_deletion, 0) = 1`, bookSelectColumns)
	args := []interface{}{}
	if olderThan != nil {
		query += " AND marked_for_deletion_at IS NOT NULL AND marked_for_deletion_at <= ?"
		args = append(args, olderThan.UTC())
	}
	query += " ORDER BY (marked_for_deletion_at IS NULL), marked_for_deletion_at DESC, title LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

// GetDashboardStats returns aggregated dashboard statistics using SQL aggregation
// instead of loading all books into memory.
func (s *SQLiteStore) GetDashboardStats() (*DashboardStats, error) {
	const primaryFilter = " AND COALESCE(is_primary_version, 1) = 1"
	stats := &DashboardStats{
		StateDistribution:  make(map[string]int),
		FormatDistribution: make(map[string]int),
		BooksByImportPath:  make(map[int]int),
		SizeByImportPath:   make(map[int]int64),
		ComputedAt:         time.Now(),
	}

	// Aggregate counts and totals
	if err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(duration), 0), COALESCE(SUM(file_size), 0)
		FROM books WHERE COALESCE(marked_for_deletion, 0) = 0`).Scan(
		&stats.TotalBooks, &stats.TotalDuration, &stats.TotalSize); err != nil {
		return nil, fmt.Errorf("failed to get book aggregates: %w", err)
	}

	if fc, err := s.CountFiles(); err == nil {
		stats.TotalFiles = fc
	}
	if ac, err := s.CountAuthors(); err == nil {
		stats.TotalAuthors = ac
	}
	if sc, err := s.CountSeries(); err == nil {
		stats.TotalSeries = sc
	}

	// Organized vs unorganized (primary, non-deleted)
	if s.rootDir != "" {
		dir := strings.TrimRight(s.rootDir, "/")
		s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(file_size),0) FROM books
			WHERE COALESCE(marked_for_deletion,0)=0`+primaryFilter+` AND (file_path LIKE ? OR file_path = ?)`,
			dir+"/%", dir).Scan(&stats.OrganizedBooks, &stats.OrganizedSize)
		s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(file_size),0) FROM books
			WHERE COALESCE(marked_for_deletion,0)=0`+primaryFilter+` AND file_path NOT LIKE ? AND file_path != ?`,
			dir+"/%", dir).Scan(&stats.UnorganizedBooks, &stats.UnorganizedSize)
	} else {
		s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(file_size),0) FROM books
			WHERE COALESCE(marked_for_deletion,0)=0`+primaryFilter).Scan(
			&stats.UnorganizedBooks, &stats.UnorganizedSize)
	}

	// State distribution
	rows, err := s.db.Query(`SELECT COALESCE(library_state, 'imported'), COUNT(*)
		FROM books WHERE COALESCE(marked_for_deletion, 0) = 0
		GROUP BY COALESCE(library_state, 'imported')`)
	if err != nil {
		return nil, fmt.Errorf("failed to get state distribution: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, err
		}
		stats.StateDistribution[state] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Format distribution
	rows2, err := s.db.Query(`SELECT COALESCE(codec, 'unknown'), COUNT(*)
		FROM books WHERE COALESCE(marked_for_deletion, 0) = 0
		GROUP BY COALESCE(codec, 'unknown')`)
	if err != nil {
		return nil, fmt.Errorf("failed to get format distribution: %w", err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var codec string
		var count int
		if err := rows2.Scan(&codec, &count); err != nil {
			return nil, err
		}
		stats.FormatDistribution[codec] = count
	}
	return stats, rows2.Err()
}

// Book Tombstones (safe deletion pattern)
// SQLite uses a dedicated tombstones table. For simplicity, we serialize the book as JSON.

func (s *SQLiteStore) CreateBookTombstone(book *Book) error {
	data, err := json.Marshal(book)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT OR REPLACE INTO book_tombstones (id, data, created_at) VALUES (?, ?, datetime('now'))`, book.ID, string(data))
	return err
}

func (s *SQLiteStore) GetBookTombstone(id string) (*Book, error) {
	row := s.db.QueryRow(`SELECT data FROM book_tombstones WHERE id = ?`, id)
	var data string
	if err := row.Scan(&data); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	var book Book
	if err := json.Unmarshal([]byte(data), &book); err != nil {
		return nil, err
	}
	return &book, nil
}

func (s *SQLiteStore) DeleteBookTombstone(id string) error {
	_, err := s.db.Exec(`DELETE FROM book_tombstones WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) ListBookTombstones(limit int) ([]Book, error) {
	rows, err := s.db.Query(`SELECT data FROM book_tombstones ORDER BY created_at ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var books []Book
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			continue
		}
		var book Book
		if err := json.Unmarshal([]byte(data), &book); err != nil {
			continue
		}
		books = append(books, book)
	}
	return books, rows.Err()
}

// GetBooksByVersionGroup returns all books in a version group
func (s *SQLiteStore) GetBooksByVersionGroup(groupID string) ([]Book, error) {
	query := fmt.Sprintf(`SELECT %s FROM books WHERE version_group_id = ? AND COALESCE(marked_for_deletion, 0) = 0 ORDER BY is_primary_version DESC, title`, bookSelectColumns)
	rows, err := s.db.Query(query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		if err := scanBook(rows, &book); err != nil {
			return nil, err
		}
		books = append(books, book)
	}

	return books, rows.Err()
}

// Import path operations

func (s *SQLiteStore) GetAllImportPaths() ([]ImportPath, error) {
	// Use a live subquery for book_count so the value is always accurate,
	// even when a scan has never been run for a newly-added import path.
	// The prefix pattern uses RTRIM to normalise any trailing slash on the
	// stored path before appending '/%', preventing false matches against
	// sibling folders that share the same prefix (e.g. /books vs /books2).
	// Note: we intentionally do NOT filter by is_primary_version here — the
	// storage page should reflect every book physically located in the folder,
	// including non-primary (duplicate) copies.
	query := `SELECT ip.id, ip.path, ip.name, ip.enabled, ip.created_at, ip.last_scan,
			  COALESCE((
			    SELECT COUNT(*)
			    FROM books b
			    WHERE (b.file_path LIKE RTRIM(ip.path, '/') || '/%'
			           OR b.file_path = RTRIM(ip.path, '/'))
			      AND COALESCE(b.marked_for_deletion, 0) = 0
			  ), 0) AS book_count
			  FROM import_paths ip ORDER BY ip.name`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []ImportPath
	for rows.Next() {
		var folder ImportPath
		if err := rows.Scan(&folder.ID, &folder.Path, &folder.Name, &folder.Enabled,
			&folder.CreatedAt, &folder.LastScan, &folder.BookCount); err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}
	return folders, rows.Err()
}

func (s *SQLiteStore) GetImportPathByID(id int) (*ImportPath, error) {
	var folder ImportPath
	query := `SELECT id, path, name, enabled, created_at, last_scan, book_count
			  FROM import_paths WHERE id = ?`
	err := s.db.QueryRow(query, id).Scan(&folder.ID, &folder.Path, &folder.Name,
		&folder.Enabled, &folder.CreatedAt, &folder.LastScan, &folder.BookCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &folder, nil
}

func (s *SQLiteStore) GetImportPathByPath(path string) (*ImportPath, error) {
	var folder ImportPath
	query := `SELECT id, path, name, enabled, created_at, last_scan, book_count
			  FROM import_paths WHERE path = ?`
	err := s.db.QueryRow(query, path).Scan(&folder.ID, &folder.Path, &folder.Name,
		&folder.Enabled, &folder.CreatedAt, &folder.LastScan, &folder.BookCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &folder, nil
}

func (s *SQLiteStore) CreateImportPath(path, name string) (*ImportPath, error) {
	result, err := s.db.Exec("INSERT INTO import_paths (path, name) VALUES (?, ?)", path, name)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &ImportPath{
		ID:        int(id),
		Path:      path,
		Name:      name,
		Enabled:   true,
		CreatedAt: time.Now(),
		BookCount: 0,
	}, nil
}

func (s *SQLiteStore) UpdateImportPath(id int, folder *ImportPath) error {
	_, err := s.db.Exec(`UPDATE import_paths SET path = ?, name = ?, enabled = ?,
		last_scan = ?, book_count = ? WHERE id = ?`,
		folder.Path, folder.Name, folder.Enabled, folder.LastScan, folder.BookCount, id)
	return err
}

func (s *SQLiteStore) DeleteImportPath(id int) error {
	_, err := s.db.Exec("DELETE FROM import_paths WHERE id = ?", id)
	return err
}

// CountBooksByPathPrefix returns the total number of books that originated from
// the given import path. It checks source_import_path first (set on books
// discovered after this change), and falls back to file_path for older records
// that pre-date the field. This ensures the count remains correct even after
// auto-organize relocates books to RootDir.
func (s *SQLiteStore) CountBooksByPathPrefix(prefix string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM books
		  WHERE source_import_path LIKE ? || '%'
		     OR (source_import_path IS NULL AND file_path LIKE ? || '%')`,
		prefix, prefix,
	).Scan(&count)
	return count, err
}

func (s *SQLiteStore) AddBlockedHash(hash, reason string) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO do_not_import (hash, reason, created_at) VALUES (?, ?, ?)",
		hash, reason, time.Now(),
	)
	return err
}

// RemoveBlockedHash removes a hash from the blocklist
func (s *SQLiteStore) RemoveBlockedHash(hash string) error {
	_, err := s.db.Exec("DELETE FROM do_not_import WHERE hash = ?", hash)
	return err
}

// GetAllBlockedHashes returns all blocked hashes
func (s *SQLiteStore) GetAllBlockedHashes() ([]DoNotImport, error) {
	rows, err := s.db.Query("SELECT hash, reason, created_at FROM do_not_import ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocked []DoNotImport
	for rows.Next() {
		var item DoNotImport
		if err := rows.Scan(&item.Hash, &item.Reason, &item.CreatedAt); err != nil {
			return nil, err
		}
		blocked = append(blocked, item)
	}
	return blocked, rows.Err()
}

// GetBlockedHashByHash retrieves a specific blocked hash entry
func (s *SQLiteStore) GetBlockedHashByHash(hash string) (*DoNotImport, error) {
	var item DoNotImport
	err := s.db.QueryRow(
		"SELECT hash, reason, created_at FROM do_not_import WHERE hash = ?",
		hash,
	).Scan(&item.Hash, &item.Reason, &item.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// SaveLibraryFingerprint stores or updates the fingerprint for an iTunes library file.
func (s *SQLiteStore) SaveLibraryFingerprint(path string, size int64, modTime time.Time, crc32val uint32) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO itunes_library_state (path, size, mod_time, crc32, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		path, size, modTime.Format(time.RFC3339), crc32val, time.Now().Format(time.RFC3339),
	)
	return err
}

// GetLibraryFingerprint retrieves the stored fingerprint for an iTunes library file.
func (s *SQLiteStore) GetLibraryFingerprint(path string) (*LibraryFingerprintRecord, error) {
	row := s.db.QueryRow(
		"SELECT path, size, mod_time, crc32, updated_at FROM itunes_library_state WHERE path = ?",
		path,
	)
	var rec LibraryFingerprintRecord
	var modTimeStr, updatedAtStr string
	err := row.Scan(&rec.Path, &rec.Size, &modTimeStr, &rec.CRC32, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rec.ModTime, err = time.Parse(time.RFC3339, modTimeStr)
	if err != nil {
		return nil, fmt.Errorf("parse ModTime %q: %w", modTimeStr, err)
	}
	rec.UpdatedAt, err = time.Parse(time.RFC3339, updatedAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse UpdatedAt %q: %w", updatedAtStr, err)
	}
	return &rec, nil
}
func (s *SQLiteStore) CreateDeferredITunesUpdate(bookID, persistentID, oldPath, newPath, updateType string) error {
	_, err := s.db.Exec(
		`INSERT INTO deferred_itunes_updates (book_id, persistent_id, old_path, new_path, update_type)
		 VALUES (?, ?, ?, ?, ?)`,
		bookID, persistentID, oldPath, newPath, updateType,
	)
	return err
}

// GetPendingDeferredITunesUpdates returns all deferred updates that haven't been applied yet.
func (s *SQLiteStore) GetPendingDeferredITunesUpdates() ([]DeferredITunesUpdate, error) {
	rows, err := s.db.Query(
		`SELECT id, book_id, persistent_id, old_path, new_path, update_type, created_at
		 FROM deferred_itunes_updates WHERE applied_at IS NULL ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DeferredITunesUpdate
	for rows.Next() {
		var d DeferredITunesUpdate
		var createdAtStr string
		if err := rows.Scan(&d.ID, &d.BookID, &d.PersistentID, &d.OldPath, &d.NewPath, &d.UpdateType, &createdAtStr); err != nil {
			return nil, err
		}
		d.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("parse CreatedAt %q: %w", createdAtStr, err)
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

// MarkDeferredITunesUpdateApplied sets the applied_at timestamp on a deferred update.
func (s *SQLiteStore) MarkDeferredITunesUpdateApplied(id int) error {
	_, err := s.db.Exec(
		`UPDATE deferred_itunes_updates SET applied_at = ? WHERE id = ?`,
		time.Now().Format(time.RFC3339), id,
	)
	return err
}

// GetDeferredITunesUpdatesByBookID returns all deferred updates for a specific book.
func (s *SQLiteStore) GetDeferredITunesUpdatesByBookID(bookID string) ([]DeferredITunesUpdate, error) {
	rows, err := s.db.Query(
		`SELECT id, book_id, persistent_id, old_path, new_path, update_type, created_at, applied_at
		 FROM deferred_itunes_updates WHERE book_id = ? ORDER BY created_at ASC`,
		bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DeferredITunesUpdate
	for rows.Next() {
		var d DeferredITunesUpdate
		var createdAtStr string
		var appliedAtStr *string
		if err := rows.Scan(&d.ID, &d.BookID, &d.PersistentID, &d.OldPath, &d.NewPath, &d.UpdateType, &createdAtStr, &appliedAtStr); err != nil {
			return nil, err
		}
		d.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("parse CreatedAt %q: %w", createdAtStr, err)
		}
		if appliedAtStr != nil {
			t, err := time.Parse(time.RFC3339, *appliedAtStr)
			if err != nil {
				return nil, fmt.Errorf("parse AppliedAt %q: %w", *appliedAtStr, err)
			}
			d.AppliedAt = &t
		}
		results = append(results, d)
	}
	return results, rows.Err()
}
func (s *SQLiteStore) CreateExternalIDMapping(mapping *ExternalIDMapping) error {
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO external_id_map (source, external_id, book_id, track_number, file_path, tombstoned, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, COALESCE((SELECT created_at FROM external_id_map WHERE source = ? AND external_id = ?), ?), ?)`,
		mapping.Source, mapping.ExternalID, mapping.BookID, mapping.TrackNumber, mapping.FilePath,
		boolToInt(mapping.Tombstoned),
		mapping.Source, mapping.ExternalID, now, now,
	)
	return err
}

// GetBookByExternalID returns the book_id for a non-tombstoned external ID.
func (s *SQLiteStore) GetBookByExternalID(source, externalID string) (string, error) {
	var bookID string
	err := s.db.QueryRow(
		`SELECT book_id FROM external_id_map WHERE source = ? AND external_id = ? AND tombstoned = 0`,
		source, externalID,
	).Scan(&bookID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return bookID, err
}

// GetExternalIDsForBook returns all external ID mappings for a book.
func (s *SQLiteStore) GetExternalIDsForBook(bookID string) ([]ExternalIDMapping, error) {
	rows, err := s.db.Query(
		`SELECT id, source, external_id, book_id, track_number, file_path, tombstoned, created_at, updated_at
		 FROM external_id_map WHERE book_id = ? ORDER BY source, external_id`,
		bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ExternalIDMapping
	for rows.Next() {
		var m ExternalIDMapping
		var trackNumber sql.NullInt64
		var filePath sql.NullString
		var tombstoned int
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&m.ID, &m.Source, &m.ExternalID, &m.BookID, &trackNumber, &filePath, &tombstoned, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		if trackNumber.Valid {
			tn := int(trackNumber.Int64)
			m.TrackNumber = &tn
		}
		if filePath.Valid {
			m.FilePath = filePath.String
		}
		m.Tombstoned = tombstoned != 0
		m.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("parse CreatedAt %q: %w", createdAtStr, err)
		}
		m.UpdatedAt, err = time.Parse(time.RFC3339, updatedAtStr)
		if err != nil {
			return nil, fmt.Errorf("parse UpdatedAt %q: %w", updatedAtStr, err)
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// IsExternalIDTombstoned checks whether an external ID is tombstoned.
func (s *SQLiteStore) IsExternalIDTombstoned(source, externalID string) (bool, error) {
	var tombstoned int
	err := s.db.QueryRow(
		`SELECT tombstoned FROM external_id_map WHERE source = ? AND external_id = ?`,
		source, externalID,
	).Scan(&tombstoned)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return tombstoned != 0, nil
}

// TombstoneExternalID marks an external ID as tombstoned to prevent reimport.
func (s *SQLiteStore) TombstoneExternalID(source, externalID string) error {
	_, err := s.db.Exec(
		`UPDATE external_id_map SET tombstoned = 1, updated_at = ? WHERE source = ? AND external_id = ?`,
		time.Now().Format(time.RFC3339), source, externalID,
	)
	return err
}

// ReassignExternalIDs moves all external ID mappings from one book to another (for merges).
func (s *SQLiteStore) ReassignExternalIDs(oldBookID, newBookID string) error {
	_, err := s.db.Exec(
		`UPDATE external_id_map SET book_id = ?, updated_at = ? WHERE book_id = ?`,
		newBookID, time.Now().Format(time.RFC3339), oldBookID,
	)
	return err
}

// BulkCreateExternalIDMappings inserts multiple external ID mappings in a transaction.
// Uses INSERT OR IGNORE so existing mappings are not overwritten.
func (s *SQLiteStore) BulkCreateExternalIDMappings(mappings []ExternalIDMapping) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT OR IGNORE INTO external_id_map (source, external_id, book_id, track_number, file_path, tombstoned, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
	)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().Format(time.RFC3339)
	for _, m := range mappings {
		if _, err := stmt.Exec(m.Source, m.ExternalID, m.BookID, m.TrackNumber, m.FilePath, now, now); err != nil {
			return fmt.Errorf("failed to insert external ID mapping (%s/%s): %w", m.Source, m.ExternalID, err)
		}
	}

	return tx.Commit()
}

// MarkExternalIDRemoved stamps removed_at and tombstones the mapping.
// Called when we remove a track from the ITL.
func (s *SQLiteStore) MarkExternalIDRemoved(source, externalID string) error {
	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(
		`UPDATE external_id_map SET removed_at = ?, tombstoned = 1, updated_at = ? WHERE source = ? AND external_id = ?`,
		now, now, source, externalID,
	)
	return err
}

// SetExternalIDProvenance sets the provenance field for a PID mapping.
func (s *SQLiteStore) SetExternalIDProvenance(source, externalID, provenance string) error {
	_, err := s.db.Exec(
		`UPDATE external_id_map SET provenance = ?, updated_at = ? WHERE source = ? AND external_id = ?`,
		provenance, time.Now().Format(time.RFC3339), source, externalID,
	)
	return err
}

// GetRemovedExternalIDs returns all removed PIDs for a source (for recycling detection).
func (s *SQLiteStore) GetRemovedExternalIDs(source string) ([]ExternalIDMapping, error) {
	rows, err := s.db.Query(
		`SELECT id, source, external_id, book_id, track_number, file_path, tombstoned, provenance, removed_at, created_at, updated_at
		 FROM external_id_map WHERE source = ? AND removed_at IS NOT NULL ORDER BY removed_at DESC`,
		source,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ExternalIDMapping
	for rows.Next() {
		var m ExternalIDMapping
		var provenance, removedAt, filePath sql.NullString
		var trackNum sql.NullInt64
		if err := rows.Scan(&m.ID, &m.Source, &m.ExternalID, &m.BookID, &trackNum, &filePath, &m.Tombstoned, &provenance, &removedAt, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		if trackNum.Valid {
			tn := int(trackNum.Int64)
			m.TrackNumber = &tn
		}
		if filePath.Valid {
			m.FilePath = filePath.String
		}
		if provenance.Valid {
			m.Provenance = provenance.String
		}
		if removedAt.Valid {
			t, parseErr := time.Parse(time.RFC3339, removedAt.String)
			if parseErr != nil {
				return nil, fmt.Errorf("parse RemovedAt %q: %w", removedAt.String, parseErr)
			}
			m.RemovedAt = &t
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// --- User Tags (free-form labels on books) ---

// GetBookUserTags returns all user-defined tags for a book.
func (s *SQLiteStore) GetBookUserTags(bookID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT tag FROM book_tags WHERE book_id = ? ORDER BY tag`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, rows.Err()
}

// SetBookUserTags replaces all user-defined tags for a book.
func (s *SQLiteStore) SetBookUserTags(bookID string, tags []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM book_tags WHERE book_id = ?`, bookID); err != nil {
		return err
	}
	for _, tag := range tags {
		if _, err := tx.Exec(`INSERT INTO book_tags (book_id, tag) VALUES (?, ?)`, bookID, tag); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddBookUserTag adds a single user-defined tag to a book (idempotent).
func (s *SQLiteStore) AddBookUserTag(bookID string, tag string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO book_tags (book_id, tag) VALUES (?, ?)`, bookID, tag)
	return err
}

// RemoveBookUserTag removes a single user-defined tag from a book.
func (s *SQLiteStore) RemoveBookUserTag(bookID string, tag string) error {
	_, err := s.db.Exec(`DELETE FROM book_tags WHERE book_id = ? AND tag = ?`, bookID, tag)
	return err
}

// GetBookAlternativeTitles returns every alt title for a book, ordered
// by source (user-entered first, auto-generated last) then title.
func (s *SQLiteStore) GetBookAlternativeTitles(bookID string) ([]BookAlternativeTitle, error) {
	rows, err := s.db.Query(`
        SELECT id, book_id, title, source, COALESCE(language, ''), created_at
        FROM book_alternative_titles
        WHERE book_id = ?
        ORDER BY CASE source WHEN 'user' THEN 0 ELSE 1 END, title`, bookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BookAlternativeTitle
	for rows.Next() {
		var alt BookAlternativeTitle
		if err := rows.Scan(&alt.ID, &alt.BookID, &alt.Title, &alt.Source, &alt.Language, &alt.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, alt)
	}
	return out, rows.Err()
}

// AddBookAlternativeTitle adds a variant title to a book. Idempotent
// on (book_id, title) via the UNIQUE constraint — re-adding the same
// title with a different source is a no-op, which is intentional:
// user-added titles should not be overwritten by auto-generated ones.
func (s *SQLiteStore) AddBookAlternativeTitle(bookID, title, source, language string) error {
	if title == "" {
		return fmt.Errorf("alternative title cannot be empty")
	}
	if source == "" {
		source = "user"
	}
	var langParam interface{}
	if language != "" {
		langParam = language
	}
	_, err := s.db.Exec(`
        INSERT OR IGNORE INTO book_alternative_titles (book_id, title, source, language)
        VALUES (?, ?, ?, ?)`, bookID, title, source, langParam)
	return err
}

// RemoveBookAlternativeTitle deletes one variant. No-op if absent.
func (s *SQLiteStore) RemoveBookAlternativeTitle(bookID, title string) error {
	_, err := s.db.Exec(`DELETE FROM book_alternative_titles WHERE book_id = ? AND title = ?`, bookID, title)
	return err
}

// SetBookAlternativeTitles replaces every alt title for a book. Used
// by the PUT endpoint that takes the full list as a single request.
func (s *SQLiteStore) SetBookAlternativeTitles(bookID string, titles []BookAlternativeTitle) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM book_alternative_titles WHERE book_id = ?`, bookID); err != nil {
		return err
	}
	for _, alt := range titles {
		if alt.Title == "" {
			continue
		}
		src := alt.Source
		if src == "" {
			src = "user"
		}
		var langParam interface{}
		if alt.Language != "" {
			langParam = alt.Language
		}
		if _, err := tx.Exec(`
            INSERT INTO book_alternative_titles (book_id, title, source, language)
            VALUES (?, ?, ?, ?)`, bookID, alt.Title, src, langParam); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Reset clears all data from all tables
func (s *SQLiteStore) Reset() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Dynamically discover all user tables from sqlite_master
	// This ensures we don't miss any tables if the schema evolves
	rows, err := tx.Query(`
		SELECT name FROM sqlite_master
		WHERE type='table'
		AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		return fmt.Errorf("failed to query table list: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return fmt.Errorf("failed to scan table name: %w", err)
		}
		tables = append(tables, tableName)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate table list: %w", err)
	}

	// Delete all rows from each discovered table
	for _, table := range tables {
		// Use parameterized table name verification by checking it's in our discovered list
		// Table names from sqlite_master are safe, but we double-check format anyway
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			// Log but continue - some tables might have constraints or other issues
			// This is safe because table names come directly from sqlite_master metadata
			continue
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// WipeTable deletes all rows from the named table and returns the row count.
// The table name must be a known safe identifier — callers are responsible for
// passing only vetted, hardcoded table names.
func (s *SQLiteStore) WipeTable(table string) (int64, error) {
	// Allowlist of tables that can be wiped via this method.
	allowed := map[string]bool{
		"books":           true,
		"book_files":      true,
		"book_segments":   true,
		"authors":         true,
		"series":          true,
		"external_id_map": true,
	}
	if !allowed[table] {
		return 0, fmt.Errorf("WipeTable: table %q not in allowlist", table)
	}
	res, err := s.db.Exec(fmt.Sprintf("DELETE FROM %s", table))
	if err != nil {
		return 0, fmt.Errorf("WipeTable %q: %w", table, err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// CountTableRows returns the row count for the named table.
// Only tables in the allowlist are accepted.
func (s *SQLiteStore) CountTableRows(table string) (int64, error) {
	allowed := map[string]bool{
		"books":           true,
		"book_files":      true,
		"book_segments":   true,
		"authors":         true,
		"series":          true,
		"external_id_map": true,
	}
	if !allowed[table] {
		return 0, fmt.Errorf("CountTableRows: table %q not in allowlist", table)
	}
	var n int64
	row := s.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table))
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("CountTableRows %q: %w", table, err)
	}
	return n, nil
}

// GetBookSnapshots is a stub — book versioning is not yet supported in SQLite store.
func (s *SQLiteStore) GetBookSnapshots(id string, limit int) ([]BookSnapshot, error) {
	return nil, nil
}

// GetBookAtVersion is a stub — book versioning is not yet supported in SQLite store.
func (s *SQLiteStore) GetBookAtVersion(id string, ts time.Time) (*Book, error) {
	return nil, fmt.Errorf("book versioning not supported in SQLite store")
}

// RevertBookToVersion is a stub — book versioning is not yet supported in SQLite store.
func (s *SQLiteStore) RevertBookToVersion(id string, ts time.Time) (*Book, error) {
	return nil, fmt.Errorf("book versioning not supported in SQLite store")
}

// PruneBookSnapshots is a stub — book versioning is not yet supported in SQLite store.
func (s *SQLiteStore) PruneBookSnapshots(id string, keepCount int) (int, error) {
	return 0, nil
}

// Optimize runs PRAGMA optimize and VACUUM to compact and optimize the SQLite database.
func (s *SQLiteStore) Optimize() error {
	if _, err := s.db.Exec("PRAGMA analysis_limit=1000; PRAGMA optimize"); err != nil {
		return fmt.Errorf("PRAGMA optimize failed: %w", err)
	}
	if _, err := s.db.Exec("VACUUM"); err != nil {
		return fmt.Errorf("VACUUM failed: %w", err)
	}
	return nil
}

// CreateOperationChange inserts a new operation change record.

// PruneSystemActivityLogs deletes system activity log entries older than the given time.
func (s *SQLiteStore) PruneSystemActivityLogs(olderThan time.Time) (int, error) {
	result, err := s.db.Exec("DELETE FROM system_activity_log WHERE created_at < ?", olderThan)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// GetScanCacheMap returns a map of file_path -> ScanCacheEntry for all books
// that have a non-empty file_path and a non-NULL last_scan_mtime.
func (s *SQLiteStore) GetScanCacheMap() (map[string]ScanCacheEntry, error) {
	rows, err := s.db.Query(
		`SELECT file_path, last_scan_mtime, last_scan_size, needs_rescan
		 FROM books WHERE file_path != '' AND last_scan_mtime IS NOT NULL`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]ScanCacheEntry)
	for rows.Next() {
		var path string
		var mtime, size sql.NullInt64
		var needsRescan sql.NullBool
		if err := rows.Scan(&path, &mtime, &size, &needsRescan); err != nil {
			return nil, err
		}
		entry := ScanCacheEntry{}
		if mtime.Valid {
			entry.Mtime = mtime.Int64
		}
		if size.Valid {
			entry.Size = size.Int64
		}
		if needsRescan.Valid {
			entry.NeedsRescan = needsRescan.Bool
		}
		result[path] = entry
	}
	return result, rows.Err()
}

// UpdateScanCache sets last_scan_mtime, last_scan_size, and clears needs_rescan for a book.
func (s *SQLiteStore) UpdateScanCache(bookID string, mtime int64, size int64) error {
	_, err := s.db.Exec(
		`UPDATE books SET last_scan_mtime = ?, last_scan_size = ?, needs_rescan = 0 WHERE id = ?`,
		mtime, size, bookID,
	)
	return err
}

// MarkNeedsRescan sets needs_rescan = 1 for the given book.
func (s *SQLiteStore) MarkNeedsRescan(bookID string) error {
	_, err := s.db.Exec(
		`UPDATE books SET needs_rescan = 1 WHERE id = ?`,
		bookID,
	)
	return err
}

// GetDirtyBookFolders returns a deduplicated list of parent directories for all
// books that have needs_rescan = 1.
func (s *SQLiteStore) GetDirtyBookFolders() ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT file_path FROM books WHERE needs_rescan = 1 AND file_path != ''`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]struct{})
	var dirs []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		dir := filepath.Dir(path)
		if _, ok := seen[dir]; !ok {
			seen[dir] = struct{}{}
			dirs = append(dirs, dir)
		}
	}
	return dirs, rows.Err()
}

// RecordPathChange inserts a path change record into book_path_history.
func (s *SQLiteStore) RecordPathChange(change *BookPathChange) error {
	_, err := s.db.Exec(
		`INSERT INTO book_path_history (book_id, old_path, new_path, change_type) VALUES (?, ?, ?, ?)`,
		change.BookID, change.OldPath, change.NewPath, change.ChangeType,
	)
	return err
}

// GetBookPathHistory returns all path changes for a book, newest first.
func (s *SQLiteStore) GetBookPathHistory(bookID string) ([]BookPathChange, error) {
	rows, err := s.db.Query(
		`SELECT id, book_id, old_path, new_path, change_type, created_at
		 FROM book_path_history WHERE book_id = ? ORDER BY created_at DESC`,
		bookID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []BookPathChange
	for rows.Next() {
		var c BookPathChange
		var createdAtStr string
		if err := rows.Scan(&c.ID, &c.BookID, &c.OldPath, &c.NewPath, &c.ChangeType, &createdAtStr); err != nil {
			return nil, err
		}
		c.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("parse CreatedAt %q: %w", createdAtStr, err)
		}
		results = append(results, c)
	}
	return results, rows.Err()
}

// AddBookTag adds a user-sourced tag to a book. Server code that
// auto-applies tags should call AddBookTagWithSource so provenance
// is preserved (see migration 47 namespace notes).
// Generates a ULID if file.ID is empty, and sets CreatedAt/UpdatedAt to now.
func (s *SQLiteStore) CreateBookFile(file *BookFile) error {
	if file.ID == "" {
		file.ID = ulid.Make().String()
	}
	now := time.Now()
	file.CreatedAt = now
	file.UpdatedAt = now
	missingInt := 0
	if file.Missing {
		missingInt = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO book_files (
			id, book_id, file_path, original_filename, itunes_path, itunes_persistent_id,
			track_number, track_count, disc_number, disc_count, title, format, codec, duration,
			file_size, bitrate_kbps, sample_rate_hz, channels, bit_depth, file_hash, original_file_hash,
			post_metadata_hash,
			acoustid_seg0, acoustid_seg1, acoustid_seg2, acoustid_seg3, acoustid_seg4, acoustid_seg5, acoustid_seg6,
			missing, created_at, updated_at,
			deluge_hash, deluge_original_path, imported_from_deluge_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		file.ID, file.BookID, file.FilePath,
		nullableStringVal(file.OriginalFilename), nullableStringVal(file.ITunesPath), nullableStringVal(file.ITunesPersistentID),
		nullableIntVal(file.TrackNumber), nullableIntVal(file.TrackCount),
		nullableIntVal(file.DiscNumber), nullableIntVal(file.DiscCount),
		nullableStringVal(file.Title), nullableStringVal(file.Format), nullableStringVal(file.Codec),
		nullableIntVal(file.Duration), nullableInt64Val(file.FileSize),
		nullableIntVal(file.BitrateKbps), nullableIntVal(file.SampleRateHz),
		nullableIntVal(file.Channels), nullableIntVal(file.BitDepth),
		nullableStringVal(file.FileHash), nullableStringVal(file.OriginalFileHash),
		nullableStringVal(file.PostMetadataHash),
		nullableStringVal(file.AcoustIDSeg0), nullableStringVal(file.AcoustIDSeg1),
		nullableStringVal(file.AcoustIDSeg2), nullableStringVal(file.AcoustIDSeg3),
		nullableStringVal(file.AcoustIDSeg4), nullableStringVal(file.AcoustIDSeg5),
		nullableStringVal(file.AcoustIDSeg6),
		missingInt, file.CreatedAt, file.UpdatedAt,
		nullableStringVal(file.DelugeHash), nullableStringVal(file.DelugeOriginalPath),
		nullableTimeVal(file.ImportedFromDelugeAt),
	)
	if err != nil {
		return fmt.Errorf("CreateBookFile: %w", err)
	}
	return nil
}

// UpdateBookFile updates all mutable fields of a book_files row identified by id.
func (s *SQLiteStore) UpdateBookFile(id string, file *BookFile) error {
	file.UpdatedAt = time.Now()
	missingInt := 0
	if file.Missing {
		missingInt = 1
	}
	_, err := s.db.Exec(
		`UPDATE book_files SET
			book_id=?, file_path=?, original_filename=?, itunes_path=?, itunes_persistent_id=?,
			track_number=?, track_count=?, disc_number=?, disc_count=?,
			title=?, format=?, codec=?, duration=?,
			file_size=?, bitrate_kbps=?, sample_rate_hz=?, channels=?, bit_depth=?,
			file_hash=?, original_file_hash=?, post_metadata_hash=?,
			acoustid_seg0=?, acoustid_seg1=?, acoustid_seg2=?, acoustid_seg3=?,
			acoustid_seg4=?, acoustid_seg5=?, acoustid_seg6=?,
			missing=?, updated_at=?,
			deluge_hash=?, deluge_original_path=?, imported_from_deluge_at=?,
			fingerprint_failed_at=?, organize_method=?,
			fingerprint_failure_reason=?, fingerprint_failure_detail=?, fingerprint_diagnostic_json=?
		WHERE id=?`,
		file.BookID, file.FilePath,
		nullableStringVal(file.OriginalFilename), nullableStringVal(file.ITunesPath), nullableStringVal(file.ITunesPersistentID),
		nullableIntVal(file.TrackNumber), nullableIntVal(file.TrackCount),
		nullableIntVal(file.DiscNumber), nullableIntVal(file.DiscCount),
		nullableStringVal(file.Title), nullableStringVal(file.Format), nullableStringVal(file.Codec),
		nullableIntVal(file.Duration), nullableInt64Val(file.FileSize),
		nullableIntVal(file.BitrateKbps), nullableIntVal(file.SampleRateHz),
		nullableIntVal(file.Channels), nullableIntVal(file.BitDepth),
		nullableStringVal(file.FileHash), nullableStringVal(file.OriginalFileHash),
		nullableStringVal(file.PostMetadataHash),
		nullableStringVal(file.AcoustIDSeg0), nullableStringVal(file.AcoustIDSeg1),
		nullableStringVal(file.AcoustIDSeg2), nullableStringVal(file.AcoustIDSeg3),
		nullableStringVal(file.AcoustIDSeg4), nullableStringVal(file.AcoustIDSeg5),
		nullableStringVal(file.AcoustIDSeg6),
		missingInt, file.UpdatedAt,
		nullableStringVal(file.DelugeHash), nullableStringVal(file.DelugeOriginalPath),
		nullableTimeVal(file.ImportedFromDelugeAt),
		nullableTimeVal(file.FingerprintFailedAt), nullableStringVal(file.OrganizeMethod),
		nullablePtrStringVal(file.FingerprintFailureReason),
		nullablePtrStringVal(file.FingerprintFailureDetail),
		nullablePtrStringVal(file.FingerprintDiagnosticJSON),
		id,
	)
	if err != nil {
		return fmt.Errorf("UpdateBookFile %s: %w", id, err)
	}
	return nil
}

// GetBookFiles returns all book_files rows for the given book, ordered by
// disc_number ASC, track_number ASC, file_path ASC.
func (s *SQLiteStore) GetBookFiles(bookID string) ([]BookFile, error) {
	rows, err := s.db.Query(
		`SELECT `+bookFileCols+`
		 FROM book_files WHERE book_id = ?
		 ORDER BY disc_number ASC, track_number ASC, file_path ASC`,
		bookID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetBookFiles: %w", err)
	}
	defer rows.Close()
	var files []BookFile
	for rows.Next() {
		f, err := bookFileScan(rows)
		if err != nil {
			return nil, fmt.Errorf("GetBookFiles scan: %w", err)
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// GetAllBookFiles returns every book_file row in the database. Used by bulk
// maintenance scans that would otherwise make one GetBookFiles call per book.
func (s *SQLiteStore) GetAllBookFiles() ([]BookFile, error) {
	rows, err := s.db.Query(
		`SELECT ` + bookFileCols + `
		 FROM book_files
		 ORDER BY book_id ASC, disc_number ASC, track_number ASC, file_path ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("GetAllBookFiles: %w", err)
	}
	defer rows.Close()
	var files []BookFile
	for rows.Next() {
		f, err := bookFileScan(rows)
		if err != nil {
			return nil, fmt.Errorf("GetAllBookFiles scan: %w", err)
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// GetBookFilesNeedingDelugeImport returns book_files that have a non-empty
// deluge_hash but have not yet been imported (imported_from_deluge_at IS NULL).
func (s *SQLiteStore) GetBookFilesNeedingDelugeImport() ([]BookFile, error) {
	rows, err := s.db.Query(
		`SELECT ` + bookFileCols + `
		 FROM book_files
		 WHERE deluge_hash IS NOT NULL AND deluge_hash != ''
		   AND imported_from_deluge_at IS NULL
		 ORDER BY book_id ASC, file_path ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("GetBookFilesNeedingDelugeImport: %w", err)
	}
	defer rows.Close()
	var files []BookFile
	for rows.Next() {
		f, err := bookFileScan(rows)
		if err != nil {
			return nil, fmt.Errorf("GetBookFilesNeedingDelugeImport scan: %w", err)
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// ClearITunesPID clears itunes_persistent_id and itunes_path on the
// book_files row with the given PID. Returns (true, nil) if a row was
// updated, (false, nil) if no such PID exists. Used by the iTunes
// orphan-cleanup path so DB state stays consistent with the ITL after
// a successful remove.
func (s *SQLiteStore) ClearITunesPID(itunesPID string) (bool, error) {
	if itunesPID == "" {
		return false, nil
	}
	res, err := s.db.Exec(
		`UPDATE book_files SET itunes_persistent_id = '', itunes_path = '', updated_at = ? WHERE itunes_persistent_id = ?`,
		time.Now().Format(time.RFC3339), itunesPID,
	)
	if err != nil {
		return false, fmt.Errorf("ClearITunesPID: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetBookFileByPID returns the book_file with the given iTunes persistent ID, or
// nil if not found.
func (s *SQLiteStore) GetBookFileByPID(itunesPID string) (*BookFile, error) {
	row := s.db.QueryRow(
		`SELECT `+bookFileCols+` FROM book_files WHERE itunes_persistent_id = ? LIMIT 1`,
		itunesPID,
	)
	f, err := bookFileScan(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetBookFileByPID: %w", err)
	}
	return &f, nil
}

// GetBookFileByPath returns the book_file with the given file path, or nil if
// not found.
func (s *SQLiteStore) GetBookFileByPath(filePath string) (*BookFile, error) {
	row := s.db.QueryRow(
		`SELECT `+bookFileCols+` FROM book_files WHERE file_path = ? LIMIT 1`,
		filePath,
	)
	f, err := bookFileScan(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetBookFileByPath: %w", err)
	}
	return &f, nil
}

// GetBookFileByAcoustID returns the first book_file whose any of the 7
// acoustid_seg columns exactly matches fp, or nil if not found.
func (s *SQLiteStore) GetBookFileByAcoustID(fp string) (*BookFile, error) {
	row := s.db.QueryRow(
		`SELECT `+bookFileCols+` FROM book_files
		 WHERE acoustid_seg0 = ? OR acoustid_seg1 = ? OR acoustid_seg2 = ?
		    OR acoustid_seg3 = ? OR acoustid_seg4 = ? OR acoustid_seg5 = ?
		    OR acoustid_seg6 = ?
		 LIMIT 1`,
		fp, fp, fp, fp, fp, fp, fp,
	)
	f, err := bookFileScan(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetBookFileByAcoustID: %w", err)
	}
	return &f, nil
}

// GetBookFileByAcoustIDFuzzy scans all fingerprinted book_files and returns the
// first whose Hamming similarity to fp across any of the 7 segments is >= minSimilarity.
// More expensive than GetBookFileByAcoustID — only called when exact match misses.
func (s *SQLiteStore) GetBookFileByAcoustIDFuzzy(fp string, minSimilarity float64) (*BookFile, error) {
	rows, err := s.db.Query(
		`SELECT ` + bookFileCols + ` FROM book_files WHERE acoustid_seg0 IS NOT NULL AND acoustid_seg0 != ''`,
	)
	if err != nil {
		return nil, fmt.Errorf("GetBookFileByAcoustIDFuzzy: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		f, err := bookFileScan(rows)
		if err != nil {
			return nil, fmt.Errorf("GetBookFileByAcoustIDFuzzy scan: %w", err)
		}
		// Check all 7 segments for a fuzzy match.
		segs := [7]string{f.AcoustIDSeg0, f.AcoustIDSeg1, f.AcoustIDSeg2, f.AcoustIDSeg3,
			f.AcoustIDSeg4, f.AcoustIDSeg5, f.AcoustIDSeg6}
		for _, seg := range segs {
			if seg == "" {
				continue
			}
			sim, err := fingerprint.HammingSimilarity(fp, seg)
			if err != nil {
				continue
			}
			if sim >= minSimilarity {
				return &f, nil
			}
		}
	}
	return nil, rows.Err()
}

// DeleteBookFile deletes a book_file by its ID.
func (s *SQLiteStore) DeleteBookFile(id string) error {
	_, err := s.db.Exec(`DELETE FROM book_files WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("DeleteBookFile %s: %w", id, err)
	}
	return nil
}

// DeleteBookFilesForBook deletes all book_files rows that belong to a given book.
func (s *SQLiteStore) DeleteBookFilesForBook(bookID string) error {
	_, err := s.db.Exec(`DELETE FROM book_files WHERE book_id = ?`, bookID)
	if err != nil {
		return fmt.Errorf("DeleteBookFilesForBook %s: %w", bookID, err)
	}
	return nil
}

// UpsertBookFile inserts or updates a book_files row.
// Match priority:
//  1. If ITunesPersistentID is set: look up by (book_id, itunes_persistent_id).
//  2. Otherwise: look up by (book_id, file_path).
//
// If found, UpdateBookFile is called. If not found, CreateBookFile is called.
func (s *SQLiteStore) UpsertBookFile(file *BookFile) error {
	var existing *BookFile
	var err error

	if file.ITunesPersistentID != "" {
		row := s.db.QueryRow(
			`SELECT `+bookFileCols+` FROM book_files WHERE book_id = ? AND itunes_persistent_id = ? LIMIT 1`,
			file.BookID, file.ITunesPersistentID,
		)
		f, scanErr := bookFileScan(row)
		if scanErr != nil && scanErr != sql.ErrNoRows {
			return fmt.Errorf("UpsertBookFile lookup by PID: %w", scanErr)
		}
		if scanErr == nil {
			existing = &f
		}
	} else {
		row := s.db.QueryRow(
			`SELECT `+bookFileCols+` FROM book_files WHERE book_id = ? AND file_path = ? LIMIT 1`,
			file.BookID, file.FilePath,
		)
		f, scanErr := bookFileScan(row)
		if scanErr != nil && scanErr != sql.ErrNoRows {
			return fmt.Errorf("UpsertBookFile lookup by path: %w", scanErr)
		}
		if scanErr == nil {
			existing = &f
		}
	}

	if existing != nil {
		file.ID = existing.ID
		file.CreatedAt = existing.CreatedAt
		err = s.UpdateBookFile(existing.ID, file)
	} else {
		err = s.CreateBookFile(file)
	}
	return err
}

// BatchUpsertBookFiles upserts a slice of BookFile records in a single
// transaction. Each file is matched by iTunes persistent ID (if set) or by
// (book_id, file_path). Errors from individual rows are collected but do not
// abort the whole transaction — the transaction is committed if at least one
// row succeeded, or rolled back only if Begin itself fails.
func (s *SQLiteStore) BatchUpsertBookFiles(files []*BookFile) error {
	if len(files) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("BatchUpsertBookFiles begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now()
	var errs []error
	for _, file := range files {
		if file == nil {
			continue
		}

		// Lookup existing row inside the transaction.
		var existingID string
		var existingCreatedAt time.Time
		if file.ITunesPersistentID != "" {
			row := tx.QueryRow(
				`SELECT id, created_at FROM book_files WHERE book_id = ? AND itunes_persistent_id = ? LIMIT 1`,
				file.BookID, file.ITunesPersistentID,
			)
			_ = row.Scan(&existingID, &existingCreatedAt)
		} else if file.FilePath != "" {
			row := tx.QueryRow(
				`SELECT id, created_at FROM book_files WHERE book_id = ? AND file_path = ? LIMIT 1`,
				file.BookID, file.FilePath,
			)
			_ = row.Scan(&existingID, &existingCreatedAt)
		}

		missingInt := 0
		if file.Missing {
			missingInt = 1
		}

		if existingID != "" {
			// UPDATE path
			file.ID = existingID
			file.CreatedAt = existingCreatedAt
			file.UpdatedAt = now
			_, execErr := tx.Exec(
				`UPDATE book_files SET
					book_id=?, file_path=?, original_filename=?, itunes_path=?, itunes_persistent_id=?,
					track_number=?, track_count=?, disc_number=?, disc_count=?,
					title=?, format=?, codec=?, duration=?,
					file_size=?, bitrate_kbps=?, sample_rate_hz=?, channels=?, bit_depth=?,
					file_hash=?, original_file_hash=?, missing=?, updated_at=?
				WHERE id=?`,
				file.BookID, file.FilePath,
				nullableStringVal(file.OriginalFilename), nullableStringVal(file.ITunesPath), nullableStringVal(file.ITunesPersistentID),
				nullableIntVal(file.TrackNumber), nullableIntVal(file.TrackCount),
				nullableIntVal(file.DiscNumber), nullableIntVal(file.DiscCount),
				nullableStringVal(file.Title), nullableStringVal(file.Format), nullableStringVal(file.Codec),
				nullableIntVal(file.Duration), nullableInt64Val(file.FileSize),
				nullableIntVal(file.BitrateKbps), nullableIntVal(file.SampleRateHz),
				nullableIntVal(file.Channels), nullableIntVal(file.BitDepth),
				nullableStringVal(file.FileHash), nullableStringVal(file.OriginalFileHash),
				missingInt, now,
				existingID,
			)
			if execErr != nil {
				errs = append(errs, fmt.Errorf("BatchUpsertBookFiles update %s: %w", existingID, execErr))
			}
		} else {
			// INSERT path
			if file.ID == "" {
				file.ID = ulid.Make().String()
			}
			file.CreatedAt = now
			file.UpdatedAt = now
			_, execErr := tx.Exec(
				`INSERT INTO book_files (
					id, book_id, file_path, original_filename, itunes_path, itunes_persistent_id,
					track_number, track_count, disc_number, disc_count, title, format, codec, duration,
					file_size, bitrate_kbps, sample_rate_hz, channels, bit_depth, file_hash, original_file_hash,
					missing, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				file.ID, file.BookID, file.FilePath,
				nullableStringVal(file.OriginalFilename), nullableStringVal(file.ITunesPath), nullableStringVal(file.ITunesPersistentID),
				nullableIntVal(file.TrackNumber), nullableIntVal(file.TrackCount),
				nullableIntVal(file.DiscNumber), nullableIntVal(file.DiscCount),
				nullableStringVal(file.Title), nullableStringVal(file.Format), nullableStringVal(file.Codec),
				nullableIntVal(file.Duration), nullableInt64Val(file.FileSize),
				nullableIntVal(file.BitrateKbps), nullableIntVal(file.SampleRateHz),
				nullableIntVal(file.Channels), nullableIntVal(file.BitDepth),
				nullableStringVal(file.FileHash), nullableStringVal(file.OriginalFileHash),
				missingInt, now, now,
			)
			if execErr != nil {
				errs = append(errs, fmt.Errorf("BatchUpsertBookFiles insert %s: %w", file.ID, execErr))
			}
		}
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return fmt.Errorf("BatchUpsertBookFiles commit: %w", commitErr)
	}
	if len(errs) > 0 {
		return fmt.Errorf("BatchUpsertBookFiles partial errors (%d): %v", len(errs), errs[0])
	}
	return nil
}

// GetBookFileByID returns a single book_file by its ID within a book.
func (s *SQLiteStore) GetBookFileByID(bookID, fileID string) (*BookFile, error) {
	row := s.db.QueryRow(
		`SELECT `+bookFileCols+` FROM book_files WHERE book_id = ? AND id = ? LIMIT 1`,
		bookID, fileID,
	)
	f, err := bookFileScan(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetBookFileByID: %w", err)
	}
	return &f, nil
}

// MoveBookFilesToBook reassigns book_files from sourceBookID to targetBookID.
func (s *SQLiteStore) MoveBookFilesToBook(fileIDs []string, sourceBookID, targetBookID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("MoveBookFilesToBook begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()
	for _, fid := range fileIDs {
		_, err := tx.Exec(
			`UPDATE book_files SET book_id = ?, updated_at = ? WHERE id = ? AND book_id = ?`,
			targetBookID, now, fid, sourceBookID,
		)
		if err != nil {
			return fmt.Errorf("MoveBookFilesToBook update %s: %w", fid, err)
		}
	}
	return tx.Commit()
}

// GetQuarantinedBooks returns books with a non-nil quarantined_at, newest first.
func (s *SQLiteStore) GetQuarantinedBooks(limit, offset int) ([]Book, error) {
	q := `SELECT id FROM books WHERE quarantined_at IS NOT NULL ORDER BY quarantined_at DESC LIMIT ? OFFSET ?`
	if limit <= 0 {
		limit = 10000
	}
	rows, err := s.db.Query(q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	var result []Book
	for _, id := range ids {
		b, err := s.GetBookByID(id)
		if err != nil || b == nil {
			continue
		}
		result = append(result, *b)
	}
	return result, nil
}

// CountQuarantinedBooks returns the total number of quarantined books.
func (s *SQLiteStore) CountQuarantinedBooks() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM books WHERE quarantined_at IS NOT NULL`).Scan(&n)
	return n, err
}

// GetScanFailCount returns the scan-fail counter stored in PebbleDB for the given path hash.
// SQLiteStore delegates to the settings table using the same key scheme.
//  1. Moves all book_files rows to primaryID.
//  2. Sets source books as non-primary (is_primary_version=0) and records
//     the consolidated target via merged_into_book_id=primaryID.
//  3. Updates the primary book's duration (rounded to nearest second) and title.
func (s *SQLiteStore) MergeChapterBooks(primaryID string, srcIDs []string, commonTitle string, totalDuration float64) error {
	if len(srcIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(srcIDs))
	idArgs := make([]interface{}, len(srcIDs))
	for i, id := range srcIDs {
		placeholders[i] = "?"
		idArgs[i] = id
	}
	ph := strings.Join(placeholders, ", ")

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("MergeChapterBooks: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	moveArgs := append([]interface{}{primaryID}, idArgs...)
	if _, err := tx.Exec(
		fmt.Sprintf("UPDATE book_files SET book_id = ? WHERE book_id IN (%s)", ph),
		moveArgs...,
	); err != nil {
		return fmt.Errorf("MergeChapterBooks: move book_files: %w", err)
	}

	markArgs := append([]interface{}{primaryID}, idArgs...)
	if _, err := tx.Exec(
		fmt.Sprintf("UPDATE books SET is_primary_version = 0, merged_into_book_id = ? WHERE id IN (%s)", ph),
		markArgs...,
	); err != nil {
		return fmt.Errorf("MergeChapterBooks: mark source books: %w", err)
	}

	durInt := int(totalDuration + 0.5) // round to nearest second
	if _, err := tx.Exec(
		"UPDATE books SET duration = ?, title = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		durInt, commonTitle, primaryID,
	); err != nil {
		return fmt.Errorf("MergeChapterBooks: update primary book: %w", err)
	}

	return tx.Commit()
}

// FlagMetadataHashDuplicate marks duplicateID as absorbed into primaryID.
// Sets merged_into_book_id=primaryID and is_primary_version=0 on the duplicate.
func (s *SQLiteStore) FlagMetadataHashDuplicate(primaryID, duplicateID string) error {
	_, err := s.db.Exec(
		`UPDATE books SET merged_into_book_id = ?, is_primary_version = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		primaryID, duplicateID,
	)
	return err
}

// SQLiteTableStat holds a row count for a single table.

// UpdateBookFileHashes updates only the original_file_hash and post_metadata_hash
// columns for the given book file ID. This is a surgical update used to record
// hashes before/after a metadata tag write without touching other fields.
func (s *SQLiteStore) UpdateBookFileHashes(id, originalHash, postMetadataHash string) error {
	_, err := s.db.Exec(
		`UPDATE book_files SET
			original_file_hash = CASE WHEN original_file_hash IS NULL OR original_file_hash = '' THEN ? ELSE original_file_hash END,
			post_metadata_hash = ?,
			updated_at = ?
		WHERE id = ?`,
		nullableStringVal(originalHash),
		nullableStringVal(postMetadataHash),
		time.Now(),
		id,
	)
	if err != nil {
		return fmt.Errorf("UpdateBookFileHashes %s: %w", id, err)
	}
	return nil
}

// SetBookFileHash sets file_hash on a book_file row and also sets
// original_file_hash if it is currently empty, matching scanner behaviour.
func (s *SQLiteStore) SetBookFileHash(id, hash string) error {
	_, err := s.db.Exec(
		`UPDATE book_files SET
			file_hash = ?,
			original_file_hash = CASE WHEN original_file_hash IS NULL OR original_file_hash = '' THEN ? ELSE original_file_hash END,
			updated_at = ?
		WHERE id = ?`,
		nullableStringVal(hash),
		nullableStringVal(hash),
		time.Now(),
		id,
	)
	if err != nil {
		return fmt.Errorf("SetBookFileHash %s: %w", id, err)
	}
	return nil
}

// GetDuplicateFilesByHash returns groups of book_files that share the same
// original_file_hash, indicating identical audio content at multiple paths.
func (s *SQLiteStore) GetDuplicateFilesByHash(limit int) ([]DuplicateFileGroup, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `
WITH dup_hashes AS (
  SELECT original_file_hash,
         COUNT(*)                       AS cnt,
         COUNT(DISTINCT book_id)        AS bcnt,
         SUM(COALESCE(file_size, 0))    AS tsz
  FROM book_files
  WHERE original_file_hash IS NOT NULL AND original_file_hash != ''
  GROUP BY original_file_hash
  HAVING COUNT(*) >= 2
  ORDER BY cnt DESC, tsz DESC
  LIMIT ?
)
SELECT dh.original_file_hash, dh.cnt, dh.bcnt, dh.tsz,
       bf.id, bf.book_id,
       COALESCE(b.title, ''),
       COALESCE(bf.file_path, ''),
       COALESCE(b.file_path, ''),
       COALESCE(bf.file_size, 0)
FROM dup_hashes dh
JOIN book_files bf ON bf.original_file_hash = dh.original_file_hash
JOIN books b ON b.id = bf.book_id AND COALESCE(b.marked_for_deletion, 0) = 0
ORDER BY dh.cnt DESC, dh.tsz DESC, bf.book_id`

	rows, err := s.db.Query(q, limit)
	if err != nil {
		return nil, fmt.Errorf("GetDuplicateFilesByHash query: %w", err)
	}
	defer rows.Close()

	groupMap := make(map[string]*DuplicateFileGroup)
	var order []string

	for rows.Next() {
		var (
			hash, fileID, bookID, bookTitle, filePath, bookPath string
			cnt, bcnt                                           int
			tsz, fileSize                                       int64
		)
		if err := rows.Scan(&hash, &cnt, &bcnt, &tsz,
			&fileID, &bookID, &bookTitle, &filePath, &bookPath, &fileSize); err != nil {
			return nil, fmt.Errorf("GetDuplicateFilesByHash scan: %w", err)
		}
		if _, seen := groupMap[hash]; !seen {
			groupMap[hash] = &DuplicateFileGroup{
				Hash:      hash,
				FileCount: cnt,
				BookCount: bcnt,
				TotalSize: tsz,
			}
			order = append(order, hash)
		}
		groupMap[hash].Files = append(groupMap[hash].Files, DuplicateFileInfo{
			BookFileID: fileID,
			BookID:     bookID,
			BookTitle:  bookTitle,
			FilePath:   filePath,
			BookPath:   bookPath,
			FileSize:   fileSize,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetDuplicateFilesByHash rows: %w", err)
	}

	groups := make([]DuplicateFileGroup, 0, len(order))
	for _, h := range order {
		groups = append(groups, *groupMap[h])
	}
	return groups, nil
}

// GetBookFileHashStats returns aggregate hash-coverage statistics for all book_files,
// broken down by which configured library path each file belongs to.
func (s *SQLiteStore) GetBookFileHashStats() (*BookFileHashStats, error) {
	stats := &BookFileHashStats{}

	// Overall totals
	err := s.db.QueryRow(`SELECT COUNT(*) FROM book_files`).Scan(&stats.TotalBookFiles)
	if err != nil {
		return nil, fmt.Errorf("GetBookFileHashStats total: %w", err)
	}
	err = s.db.QueryRow(`SELECT COUNT(*) FROM book_files WHERE file_hash IS NOT NULL AND file_hash != ''`).Scan(&stats.WithFileHash)
	if err != nil {
		return nil, fmt.Errorf("GetBookFileHashStats with_hash: %w", err)
	}
	err = s.db.QueryRow(`SELECT COUNT(*) FROM book_files WHERE original_file_hash IS NOT NULL AND original_file_hash != ''`).Scan(&stats.WithOriginalHash)
	if err != nil {
		return nil, fmt.Errorf("GetBookFileHashStats with_original_hash: %w", err)
	}
	stats.MissingFileHash = stats.TotalBookFiles - stats.WithFileHash

	err = s.db.QueryRow(`SELECT COUNT(*) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0`).Scan(&stats.TotalBooks)
	if err != nil {
		return nil, fmt.Errorf("GetBookFileHashStats total_books: %w", err)
	}
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM books
		WHERE COALESCE(marked_for_deletion, 0) = 0
		  AND NOT EXISTS (SELECT 1 FROM book_files WHERE book_id = books.id)`).Scan(&stats.BooksWithNoFiles)
	if err != nil {
		return nil, fmt.Errorf("GetBookFileHashStats books_no_files: %w", err)
	}

	// Per-library breakdown: derive top-level paths from books.source_import_path and books.file_path.
	// Group by the first two path segments (e.g. /mnt/data/audiobooks) so we get one row per library root.
	libRows, lerr := s.db.Query(`
		SELECT DISTINCT COALESCE(source_import_path, '') AS lib
		FROM books
		WHERE COALESCE(marked_for_deletion, 0) = 0
		  AND COALESCE(source_import_path, '') != ''
		ORDER BY lib`)
	if lerr == nil {
		defer libRows.Close()
		for libRows.Next() {
			var lib string
			if scanErr := libRows.Scan(&lib); scanErr != nil || lib == "" {
				continue
			}
			prefix := strings.TrimSuffix(lib, "/") + "/"
			var row BookFileHashStatsByLib
			row.Path = lib
			_ = s.db.QueryRow(
				`SELECT COUNT(*) FROM book_files WHERE file_path LIKE ?`, prefix+"%",
			).Scan(&row.TotalFiles)
			_ = s.db.QueryRow(
				`SELECT COUNT(*) FROM book_files WHERE file_path LIKE ? AND file_hash IS NOT NULL AND file_hash != ''`, prefix+"%",
			).Scan(&row.WithHash)
			row.MissingHash = row.TotalFiles - row.WithHash
			stats.ByLibrary = append(stats.ByLibrary, row)
		}
	}
	return stats, nil
}

// GetBookMetadataHashStats returns metadata_source_hash coverage across all books.
func (s *SQLiteStore) GetBookMetadataHashStats() (*BookMetadataHashStats, error) {
	stats := &BookMetadataHashStats{}

	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM books WHERE COALESCE(marked_for_deletion, 0) = 0`,
	).Scan(&stats.TotalBooks); err != nil {
		return nil, fmt.Errorf("GetBookMetadataHashStats total: %w", err)
	}
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM books
		 WHERE COALESCE(marked_for_deletion, 0) = 0
		   AND metadata_source_hash IS NOT NULL AND metadata_source_hash != ''`,
	).Scan(&stats.WithMetadataHash); err != nil {
		return nil, fmt.Errorf("GetBookMetadataHashStats with_hash: %w", err)
	}
	stats.MissingMetadataHash = stats.TotalBooks - stats.WithMetadataHash

	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM books
		 WHERE COALESCE(marked_for_deletion, 0) = 0
		   AND (
		     (asin IS NOT NULL AND asin != '') OR
		     (isbn13 IS NOT NULL AND isbn13 != '') OR
		     (isbn10 IS NOT NULL AND isbn10 != '')
		   )`,
	).Scan(&stats.WithASINOrISBN); err != nil {
		return nil, fmt.Errorf("GetBookMetadataHashStats with_id: %w", err)
	}
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM books
		 WHERE COALESCE(marked_for_deletion, 0) = 0
		   AND (metadata_source_hash IS NULL OR metadata_source_hash = '')
		   AND (
		     (asin IS NOT NULL AND asin != '') OR
		     (isbn13 IS NOT NULL AND isbn13 != '') OR
		     (isbn10 IS NOT NULL AND isbn10 != '')
		   )`,
	).Scan(&stats.MissingHashHasID); err != nil {
		return nil, fmt.Errorf("GetBookMetadataHashStats missing_hash_has_id: %w", err)
	}

	libRows, lerr := s.db.Query(`
		SELECT DISTINCT COALESCE(source_import_path, '') AS lib
		FROM books
		WHERE COALESCE(marked_for_deletion, 0) = 0
		  AND COALESCE(source_import_path, '') != ''
		ORDER BY lib`)
	if lerr == nil {
		defer libRows.Close()
		for libRows.Next() {
			var lib string
			if scanErr := libRows.Scan(&lib); scanErr != nil || lib == "" {
				continue
			}
			prefix := strings.TrimSuffix(lib, "/") + "/"
			var row BookMetadataHashStatsByLib
			row.Path = lib
			_ = s.db.QueryRow(
				`SELECT COUNT(*) FROM books
				 WHERE COALESCE(marked_for_deletion,0)=0 AND source_import_path = ?`, lib,
			).Scan(&row.TotalBooks)
			_ = s.db.QueryRow(
				`SELECT COUNT(*) FROM books
				 WHERE COALESCE(marked_for_deletion,0)=0 AND source_import_path = ?
				   AND metadata_source_hash IS NOT NULL AND metadata_source_hash != ''`, lib,
			).Scan(&row.WithMetadataHash)
			row.MissingMetadataHash = row.TotalBooks - row.WithMetadataHash
			_ = s.db.QueryRow(
				`SELECT COUNT(*) FROM books
				 WHERE COALESCE(marked_for_deletion,0)=0 AND source_import_path = ?
				   AND (metadata_source_hash IS NULL OR metadata_source_hash = '')
				   AND ((asin IS NOT NULL AND asin != '') OR
				        (isbn13 IS NOT NULL AND isbn13 != '') OR
				        (isbn10 IS NOT NULL AND isbn10 != ''))`, lib,
			).Scan(&row.MissingHashHasID)
			_ = prefix // used for file-hash stats; not needed here
			stats.ByLibrary = append(stats.ByLibrary, row)
		}
	}
	return stats, nil
}

// GetFilesWithFingerprintFailures returns book_files where fingerprint_failed_at is set,
// optionally filtered by reason. Returns the paginated slice and total matching count.
func (s *SQLiteStore) GetFilesWithFingerprintFailures(reason string, limit, offset int) ([]BookFile, int64, error) {
	whereExtra := ""
	args := []interface{}{}
	if reason != "" {
		whereExtra = " AND fingerprint_failure_reason = ?"
		args = append(args, reason)
	}

	var total int64
	countQ := `SELECT COUNT(*) FROM book_files WHERE fingerprint_failed_at IS NOT NULL` + whereExtra
	if err := s.db.QueryRow(countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("GetFilesWithFingerprintFailures count: %w", err)
	}

	pageArgs := append(args, limit, offset)
	pageQ := `SELECT ` + bookFileCols + ` FROM book_files WHERE fingerprint_failed_at IS NOT NULL` + whereExtra +
		` ORDER BY fingerprint_failed_at DESC LIMIT ? OFFSET ?`
	rows, err := s.db.Query(pageQ, pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("GetFilesWithFingerprintFailures query: %w", err)
	}
	defer rows.Close()
	var files []BookFile
	for rows.Next() {
		f, err := bookFileScan(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("GetFilesWithFingerprintFailures scan: %w", err)
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("GetFilesWithFingerprintFailures rows: %w", err)
	}
	return files, total, nil
}

// GetAcoustIDStats is not implemented for SQLite (PebbleDB is primary).
func (s *SQLiteStore) GetAcoustIDStats() (*AcoustIDStats, error) {
	return &AcoustIDStats{}, nil
}
