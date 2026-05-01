// file: internal/database/sqlite_store_util.go
// version: 1.0.1
// guid: b8c9d0e1-f2a3-4567-b012-890123456789
// last-edited: 2026-05-02

package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	matcher "github.com/jdfalk/audiobook-organizer/internal/matcher"
)

func scanBookSummary(scanner rowScanner, summary *BookSummary) error {
	var (
		authorID, seriesID, seriesSequence, duration           sql.NullInt64
		fileSize                                               sql.NullInt64
		title, filePath, format                                string
		originalFilename                                       sql.NullString
		fileHash, originalFileHash, organizedFileHash          sql.NullString
		libraryState, quarantineReason, coverURL, narrator     sql.NullString
		metadataReviewStatus, versionGroupID                   sql.NullString
		isPrimaryVersion                                       sql.NullBool
		quarantinedAt, createdAt, updatedAt, metadataUpdatedAt sql.NullTime
	)

	if err := scanner.Scan(
		&summary.ID, &title, &authorID, &seriesID, &seriesSequence,
		&filePath, &format, &duration, &originalFilename,
		&fileHash, &fileSize, &originalFileHash, &organizedFileHash,
		&libraryState, &quarantinedAt, &quarantineReason, &coverURL, &narrator,
		&createdAt, &updatedAt, &metadataUpdatedAt,
		&isPrimaryVersion, &versionGroupID, &metadataReviewStatus,
	); err != nil {
		return err
	}

	summary.Title = title
	summary.FilePath = filePath
	summary.Format = format
	summary.AuthorID = nullableInt(authorID)
	summary.SeriesID = nullableInt(seriesID)
	summary.SeriesSequence = nullableInt(seriesSequence)
	summary.Duration = nullableInt(duration)
	summary.OriginalFilename = nullableString(originalFilename)
	if fileSize.Valid {
		size := fileSize.Int64
		summary.FileSize = &size
	}
	summary.FileHash = nullableString(fileHash)
	summary.OriginalFileHash = nullableString(originalFileHash)
	summary.OrganizedFileHash = nullableString(organizedFileHash)
	summary.LibraryState = nullableString(libraryState)
	if quarantinedAt.Valid {
		summary.QuarantinedAt = &quarantinedAt.Time
	}
	summary.QuarantineReason = nullableString(quarantineReason)
	summary.CoverURL = nullableString(coverURL)
	summary.Narrator = nullableString(narrator)
	if createdAt.Valid {
		summary.CreatedAt = &createdAt.Time
	}
	if updatedAt.Valid {
		summary.UpdatedAt = &updatedAt.Time
	}
	if metadataUpdatedAt.Valid {
		summary.MetadataUpdatedAt = &metadataUpdatedAt.Time
	}
	if isPrimaryVersion.Valid {
		val := isPrimaryVersion.Bool
		summary.IsPrimaryVersion = &val
	}
	summary.VersionGroupID = nullableString(versionGroupID)
	summary.MetadataReviewStatus = nullableString(metadataReviewStatus)

	return nil
}

func scanBook(scanner rowScanner, book *Book) error {
	var (
		authorID, seriesID, seriesSequence, duration, printYear, releaseYear sql.NullInt64
		itunesPlayCount, itunesRating, itunesBookmark                        sql.NullInt64
		fileSize, bitrate, sampleRate, channels, bitDepth, quantity          sql.NullInt64
		title, filePath, format                                              string
		originalFilename                                                     sql.NullString
		workID, narrator, edition, description, language, publisher, genre   sql.NullString
		itunesPersistentID, itunesImportSource, itunesPath                   sql.NullString
		itunesDateAdded, itunesLastPlayed                                    sql.NullTime
		isbn10, isbn13, asin                                                 sql.NullString
		openLibraryID, hardcoverID, googleBooksID                            sql.NullString
		fileHash, quality, codec                                             sql.NullString
		originalFileHash, organizedFileHash                                  sql.NullString
		versionGroupID, versionNotes                                         sql.NullString
		coverURL, narratorsJSON, metadataReviewStatus                        sql.NullString
		isPrimaryVersion                                                     sql.NullBool
		libraryState                                                         sql.NullString
		markedForDeletion                                                    sql.NullBool
		markedForDeletionAt, createdAt, updatedAt                            sql.NullTime
		metadataUpdatedAt, lastWrittenAt                                     sql.NullTime
		lastOrganizeOperationID                                              sql.NullString
		lastOrganizedAt                                                      sql.NullTime
		itunesSyncStatus                                                     sql.NullString
		quarantineReason                                                     sql.NullString
		quarantinedAt                                                        sql.NullTime
		audibleRatingOverall, audibleRatingPerformance, audibleRatingStory   sql.NullFloat64
		audibleRatingCount, audibleNumReviews                                sql.NullInt64
		googleRatingAverage                                                  sql.NullFloat64
		googleRatingCount                                                    sql.NullInt64
		userRatingOverall, userRatingStory, userRatingPerformance            sql.NullFloat64
		userRatingNotes                                                      sql.NullString
		metadataSourceHash                                                   sql.NullString
		mergedIntoBookID                                                     sql.NullString
	)

	if err := scanner.Scan(
		&book.ID, &title, &authorID, &seriesID, &seriesSequence,
		&filePath, &originalFilename, &format, &duration,
		&workID, &narrator, &edition, &description, &language, &publisher, &genre,
		&printYear, &releaseYear, &isbn10, &isbn13, &asin,
		&openLibraryID, &hardcoverID, &googleBooksID,
		&itunesPersistentID, &itunesDateAdded, &itunesPlayCount,
		&itunesLastPlayed, &itunesRating, &itunesBookmark, &itunesImportSource, &itunesPath,
		&fileHash, &fileSize, &bitrate, &codec, &sampleRate, &channels,
		&bitDepth, &quality, &isPrimaryVersion, &versionGroupID, &versionNotes,
		&originalFileHash, &organizedFileHash, &libraryState, &quantity,
		&markedForDeletion, &markedForDeletionAt, &createdAt, &updatedAt,
		&metadataUpdatedAt, &lastWrittenAt, &metadataReviewStatus, &coverURL, &narratorsJSON,
		&lastOrganizeOperationID, &lastOrganizedAt, &itunesSyncStatus,
		&quarantineReason, &quarantinedAt,
		&audibleRatingOverall, &audibleRatingPerformance, &audibleRatingStory,
		&audibleRatingCount, &audibleNumReviews,
		&googleRatingAverage, &googleRatingCount,
		&userRatingOverall, &userRatingStory, &userRatingPerformance, &userRatingNotes,
		&metadataSourceHash,
		&mergedIntoBookID,
	); err != nil {
		return err
	}

	book.Title = title
	book.FilePath = filePath
	book.OriginalFilename = nullableString(originalFilename)
	book.Format = format
	book.AuthorID = nullableInt(authorID)
	book.SeriesID = nullableInt(seriesID)
	book.SeriesSequence = nullableInt(seriesSequence)
	book.Duration = nullableInt(duration)
	book.WorkID = nullableString(workID)
	book.Narrator = nullableString(narrator)
	book.Edition = nullableString(edition)
	book.Description = nullableString(description)
	book.Language = nullableString(language)
	book.Publisher = nullableString(publisher)
	book.Genre = nullableString(genre)
	book.PrintYear = nullableInt(printYear)
	book.AudiobookReleaseYear = nullableInt(releaseYear)
	book.ISBN10 = nullableString(isbn10)
	book.ISBN13 = nullableString(isbn13)
	book.ASIN = nullableString(asin)
	book.OpenLibraryID = nullableString(openLibraryID)
	book.HardcoverID = nullableString(hardcoverID)
	book.GoogleBooksID = nullableString(googleBooksID)
	book.ITunesPersistentID = nullableString(itunesPersistentID)
	if itunesDateAdded.Valid {
		book.ITunesDateAdded = &itunesDateAdded.Time
	}
	book.ITunesPlayCount = nullableInt(itunesPlayCount)
	if itunesLastPlayed.Valid {
		book.ITunesLastPlayed = &itunesLastPlayed.Time
	}
	book.ITunesRating = nullableInt(itunesRating)
	if itunesBookmark.Valid {
		bookmark := itunesBookmark.Int64
		book.ITunesBookmark = &bookmark
	}
	book.ITunesImportSource = nullableString(itunesImportSource)
	book.ITunesPath = nullableString(itunesPath)
	book.FileHash = nullableString(fileHash)
	if fileSize.Valid {
		size := fileSize.Int64
		book.FileSize = &size
	}
	book.Bitrate = nullableInt(bitrate)
	book.Codec = nullableString(codec)
	book.SampleRate = nullableInt(sampleRate)
	book.Channels = nullableInt(channels)
	book.BitDepth = nullableInt(bitDepth)
	book.Quality = nullableString(quality)
	if isPrimaryVersion.Valid {
		val := isPrimaryVersion.Bool
		book.IsPrimaryVersion = &val
	}
	book.VersionGroupID = nullableString(versionGroupID)
	book.VersionNotes = nullableString(versionNotes)
	book.OriginalFileHash = nullableString(originalFileHash)
	book.OrganizedFileHash = nullableString(organizedFileHash)
	book.LibraryState = nullableString(libraryState)
	book.Quantity = nullableInt(quantity)
	if markedForDeletion.Valid {
		val := markedForDeletion.Bool
		book.MarkedForDeletion = &val
	}
	if markedForDeletionAt.Valid {
		book.MarkedForDeletionAt = &markedForDeletionAt.Time
	}
	if createdAt.Valid {
		book.CreatedAt = &createdAt.Time
	}
	if updatedAt.Valid {
		book.UpdatedAt = &updatedAt.Time
	}
	if metadataUpdatedAt.Valid {
		book.MetadataUpdatedAt = &metadataUpdatedAt.Time
	}
	if lastWrittenAt.Valid {
		book.LastWrittenAt = &lastWrittenAt.Time
	}
	book.MetadataReviewStatus = nullableString(metadataReviewStatus)
	book.CoverURL = nullableString(coverURL)
	book.NarratorsJSON = nullableString(narratorsJSON)
	book.LastOrganizeOperationID = nullableString(lastOrganizeOperationID)
	if lastOrganizedAt.Valid {
		book.LastOrganizedAt = &lastOrganizedAt.Time
	}
	book.ITunesSyncStatus = nullableString(itunesSyncStatus)
	book.QuarantineReason = nullableString(quarantineReason)
	if quarantinedAt.Valid {
		book.QuarantinedAt = &quarantinedAt.Time
	}
	book.AudibleRatingOverall = nullableFloat(audibleRatingOverall)
	book.AudibleRatingPerformance = nullableFloat(audibleRatingPerformance)
	book.AudibleRatingStory = nullableFloat(audibleRatingStory)
	book.AudibleRatingCount = nullableInt(audibleRatingCount)
	book.AudibleNumReviews = nullableInt(audibleNumReviews)
	book.GoogleRatingAverage = nullableFloat(googleRatingAverage)
	book.GoogleRatingCount = nullableInt(googleRatingCount)
	book.UserRatingOverall = nullableFloat(userRatingOverall)
	book.UserRatingStory = nullableFloat(userRatingStory)
	book.UserRatingPerformance = nullableFloat(userRatingPerformance)
	book.UserRatingNotes = nullableString(userRatingNotes)
	book.MetadataSourceHash = nullableString(metadataSourceHash)
	book.MergedIntoBookID = nullableString(mergedIntoBookID)
	return nil
}

func nullableString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	val := ns.String
	return &val
}

func nullableInt(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	val := int(ni.Int64)
	return &val
}

func nullableFloat(nf sql.NullFloat64) *float64 {
	if !nf.Valid {
		return nil
	}
	return &nf.Float64
}
func normalizeTitle(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	// Remove common parenthesized suffixes
	for _, suffix := range []string{"(unabridged)", "(abridged)", "(audiobook)", "(audio)"} {
		s = strings.ReplaceAll(s, suffix, "")
	}
	// Remove leading articles
	for _, article := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(s, article) {
			s = s[len(article):]
			break
		}
	}
	// Collapse multiple spaces
	parts := strings.Fields(s)
	return strings.Join(parts, " ")
}

// jaroWinkler computes the Jaro-Winkler similarity between two strings (0.0–1.0).
func jaroWinkler(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	// Jaro distance
	matchDist := max(len(s1), len(s2))/2 - 1
	if matchDist < 0 {
		matchDist = 0
	}

	s1Matches := make([]bool, len(s1))
	s2Matches := make([]bool, len(s2))
	matches := 0
	transpositions := 0

	for i := 0; i < len(s1); i++ {
		start := i - matchDist
		if start < 0 {
			start = 0
		}
		end := i + matchDist + 1
		if end > len(s2) {
			end = len(s2)
		}
		for j := start; j < end; j++ {
			if s2Matches[j] || s1[i] != s2[j] {
				continue
			}
			s1Matches[i] = true
			s2Matches[j] = true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0.0
	}

	k := 0
	for i := 0; i < len(s1); i++ {
		if !s1Matches[i] {
			continue
		}
		for !s2Matches[k] {
			k++
		}
		if s1[i] != s2[k] {
			transpositions++
		}
		k++
	}

	jaro := (float64(matches)/float64(len(s1)) +
		float64(matches)/float64(len(s2)) +
		float64(matches-transpositions/2)/float64(matches)) / 3.0

	// Winkler modification: boost for common prefix (up to 4 chars)
	prefix := 0
	for i := 0; i < min(4, min(len(s1), len(s2))); i++ {
		if s1[i] == s2[i] {
			prefix++
		} else {
			break
		}
	}

	return jaro + float64(prefix)*0.1*(1.0-jaro)
}
func fuzzyRankBooks(query string, books []Book) []Book {
	if len(books) <= 1 {
		return books
	}
	type scored struct {
		book  Book
		score int
	}
	items := make([]scored, len(books))
	for i, b := range books {
		items[i] = scored{book: b, score: matcher.ScoreMatch(query, b.Title)}
	}
	// Insertion sort (stable, fine for small N)
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].score > items[j-1].score; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
	result := make([]Book, len(items))
	for i, it := range items {
		result[i] = it.book
	}
	return result
}

// sanitizeFTS5Query escapes FTS5 special characters and wraps terms for prefix matching.
func sanitizeFTS5Query(q string) string {
	// Remove FTS5 operators that could cause syntax errors
	replacer := strings.NewReplacer(
		`"`, ``,
		`*`, ``,
		`(`, ``,
		`)`, ``,
	)
	cleaned := replacer.Replace(q)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return `""`
	}
	// Quote the whole thing and add prefix matching
	return `"` + cleaned + `"` + "*"
}
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
func scanOperationChanges(rows *sql.Rows) ([]*OperationChange, error) {
	var changes []*OperationChange
	for rows.Next() {
		c := &OperationChange{}
		if err := rows.Scan(&c.ID, &c.OperationID, &c.BookID, &c.ChangeType, &c.FieldName, &c.OldValue, &c.NewValue, &c.RevertedAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		changes = append(changes, c)
	}
	return changes, rows.Err()
}

// ---- BookFile CRUD ----

// bookFileScan scans a single row into a BookFile.
// Use with queries that SELECT bookFileCols in the same order.
func bookFileScan(row interface {
	Scan(dest ...any) error
}) (BookFile, error) {
	var f BookFile
	var originalFilename, itunesPath, itunesPID sql.NullString
	var trackNumber, trackCount, discNumber, discCount sql.NullInt64
	var delugeHash, delugeOriginalPath sql.NullString
	var importedFromDelugeAt sql.NullTime
	var title, format, codec sql.NullString
	var duration, fileSize, bitrateKbps, sampleRateHz, channels, bitDepth sql.NullInt64
	var fileHash, originalFileHash, postMetadataHash sql.NullString
	var acoustidSeg0, acoustidSeg1, acoustidSeg2, acoustidSeg3, acoustidSeg4, acoustidSeg5, acoustidSeg6 sql.NullString
	var missing int
	err := row.Scan(
		&f.ID, &f.BookID, &f.FilePath,
		&originalFilename, &itunesPath, &itunesPID,
		&trackNumber, &trackCount, &discNumber, &discCount,
		&title, &format, &codec,
		&duration, &fileSize, &bitrateKbps, &sampleRateHz, &channels, &bitDepth,
		&fileHash, &originalFileHash,
		&postMetadataHash,
		&acoustidSeg0, &acoustidSeg1, &acoustidSeg2, &acoustidSeg3, &acoustidSeg4, &acoustidSeg5, &acoustidSeg6,
		&missing, &f.CreatedAt, &f.UpdatedAt,
		&delugeHash, &delugeOriginalPath, &importedFromDelugeAt,
	)
	if err != nil {
		return f, err
	}
	if originalFilename.Valid {
		f.OriginalFilename = originalFilename.String
	}
	if itunesPath.Valid {
		f.ITunesPath = itunesPath.String
	}
	if itunesPID.Valid {
		f.ITunesPersistentID = itunesPID.String
	}
	if trackNumber.Valid {
		f.TrackNumber = int(trackNumber.Int64)
	}
	if trackCount.Valid {
		f.TrackCount = int(trackCount.Int64)
	}
	if discNumber.Valid {
		f.DiscNumber = int(discNumber.Int64)
	}
	if discCount.Valid {
		f.DiscCount = int(discCount.Int64)
	}
	if title.Valid {
		f.Title = title.String
	}
	if format.Valid {
		f.Format = format.String
	}
	if codec.Valid {
		f.Codec = codec.String
	}
	if duration.Valid {
		f.Duration = int(duration.Int64)
	}
	if fileSize.Valid {
		f.FileSize = fileSize.Int64
	}
	if bitrateKbps.Valid {
		f.BitrateKbps = int(bitrateKbps.Int64)
	}
	if sampleRateHz.Valid {
		f.SampleRateHz = int(sampleRateHz.Int64)
	}
	if channels.Valid {
		f.Channels = int(channels.Int64)
	}
	if bitDepth.Valid {
		f.BitDepth = int(bitDepth.Int64)
	}
	if fileHash.Valid {
		f.FileHash = fileHash.String
	}
	if originalFileHash.Valid {
		f.OriginalFileHash = originalFileHash.String
	}
	if postMetadataHash.Valid {
		f.PostMetadataHash = postMetadataHash.String
	}
	if acoustidSeg0.Valid {
		f.AcoustIDSeg0 = acoustidSeg0.String
	}
	if acoustidSeg1.Valid {
		f.AcoustIDSeg1 = acoustidSeg1.String
	}
	if acoustidSeg2.Valid {
		f.AcoustIDSeg2 = acoustidSeg2.String
	}
	if acoustidSeg3.Valid {
		f.AcoustIDSeg3 = acoustidSeg3.String
	}
	if acoustidSeg4.Valid {
		f.AcoustIDSeg4 = acoustidSeg4.String
	}
	if acoustidSeg5.Valid {
		f.AcoustIDSeg5 = acoustidSeg5.String
	}
	if acoustidSeg6.Valid {
		f.AcoustIDSeg6 = acoustidSeg6.String
	}
	f.Missing = missing != 0
	if delugeHash.Valid {
		f.DelugeHash = delugeHash.String
	}
	if delugeOriginalPath.Valid {
		f.DelugeOriginalPath = delugeOriginalPath.String
	}
	if importedFromDelugeAt.Valid {
		t := importedFromDelugeAt.Time
		f.ImportedFromDelugeAt = &t
	}
	return f, nil
}

// nullableStringVal converts a string to sql.NullString (empty string = NULL).
func nullableStringVal(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullableIntVal converts an int to sql.NullInt64 (zero = NULL).
func nullableIntVal(n int) sql.NullInt64 {
	if n == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(n), Valid: true}
}

// nullableInt64Val converts an int64 to sql.NullInt64 (zero = NULL).
func nullableInt64Val(n int64) sql.NullInt64 {
	if n == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: n, Valid: true}
}

// nullableTimeVal converts a *time.Time to sql.NullTime (nil pointer = NULL).
func nullableTimeVal(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// CreateBookFile inserts a new book_files row.
type SQLiteTableStat struct {
	Name     string `json:"name"`
	RowCount int64  `json:"row_count"`
}

// TableRowCounts returns row counts for the primary tables in the SQLite store.
// Used by the DB health diagnostics endpoint.
func (s *SQLiteStore) TableRowCounts() ([]SQLiteTableStat, error) {
	tables := []string{
		"books", "book_files", "authors", "series",
		"operations", "operation_logs",
	}
	stats := make([]SQLiteTableStat, 0, len(tables))
	for _, tbl := range tables {
		var count int64
		row := s.db.QueryRow(`SELECT COUNT(*) FROM ` + tbl) //nolint:gosec
		if err := row.Scan(&count); err != nil {
			count = -1
		}
		stats = append(stats, SQLiteTableStat{Name: tbl, RowCount: count})
	}
	return stats, nil
}

// SQLitePageSizeBytes returns the on-disk size of the SQLite database
// estimated via page_count * page_size.
func (s *SQLiteStore) SQLitePageSizeBytes() int64 {
	var pageCount, pageSize int64
	_ = s.db.QueryRow(`PRAGMA page_count`).Scan(&pageCount)
	_ = s.db.QueryRow(`PRAGMA page_size`).Scan(&pageSize)
	return pageCount * pageSize
}

type BookPathPrefix struct {
	Prefix    string `json:"prefix"`
	BookCount int64  `json:"book_count"`
}

// GetBookPathPrefixes returns the top-N root path prefixes (depth-2 from /)
// found in books.file_path, ordered by descending count. This helps diagnose
// mismatches between configured import paths and the actual stored paths.
func (s *SQLiteStore) GetBookPathPrefixes(limit int) ([]BookPathPrefix, error) {
	if limit <= 0 {
		limit = 20
	}
	// Extract up to 3 path segments so e.g. "/mnt/bigdata/books" is the prefix
	// for "/mnt/bigdata/books/newbooks/Author/Title".
	rows, err := s.db.Query(`
		SELECT
		  RTRIM(
		    SUBSTR(file_path, 1,
		      CASE
		        WHEN INSTR(SUBSTR(file_path,
		               INSTR(SUBSTR(file_path, 2), '/') + 2), '/') > 0
		        THEN INSTR(SUBSTR(file_path,
		               INSTR(SUBSTR(file_path, 2), '/') + 2), '/') +
		             INSTR(SUBSTR(file_path, 2), '/') + 1
		        ELSE LENGTH(file_path)
		      END
		    ), '/') AS prefix,
		  COUNT(*) AS cnt
		FROM books
		WHERE file_path != '' AND COALESCE(marked_for_deletion, 0) = 0
		GROUP BY prefix
		ORDER BY cnt DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BookPathPrefix
	for rows.Next() {
		var p BookPathPrefix
		if err := rows.Scan(&p.Prefix, &p.BookCount); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetAuthorsByBookIDs returns a map from bookID → []Author for all given book IDs.
func (s *SQLiteStore) GetAuthorsByBookIDs(ctx context.Context, bookIDs []string) (map[string][]Author, error) {
	if len(bookIDs) == 0 {
		return map[string][]Author{}, nil
	}
	placeholders := strings.Repeat("?,", len(bookIDs))
	placeholders = placeholders[:len(placeholders)-1]
	query := fmt.Sprintf(
		"SELECT ba.book_id, a.id, a.name FROM book_authors ba JOIN authors a ON ba.author_id = a.id WHERE ba.book_id IN (%s) ORDER BY ba.position",
		placeholders,
	)
	args := make([]interface{}, len(bookIDs))
	for i, id := range bookIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("GetAuthorsByBookIDs query: %w", err)
	}
	defer rows.Close()
	result := make(map[string][]Author, len(bookIDs))
	for rows.Next() {
		var bookID string
		var a Author
		if err := rows.Scan(&bookID, &a.ID, &a.Name); err != nil {
			return nil, fmt.Errorf("GetAuthorsByBookIDs scan: %w", err)
		}
		result[bookID] = append(result[bookID], a)
	}
	return result, rows.Err()
}

// GetNarratorsByBookIDs returns a map from bookID → []Narrator for all given book IDs.
func (s *SQLiteStore) GetNarratorsByBookIDs(ctx context.Context, bookIDs []string) (map[string][]Narrator, error) {
	if len(bookIDs) == 0 {
		return map[string][]Narrator{}, nil
	}
	placeholders := strings.Repeat("?,", len(bookIDs))
	placeholders = placeholders[:len(placeholders)-1]
	query := fmt.Sprintf(
		"SELECT bn.book_id, n.id, n.name, n.created_at FROM book_narrators bn JOIN narrators n ON bn.narrator_id = n.id WHERE bn.book_id IN (%s) ORDER BY bn.position",
		placeholders,
	)
	args := make([]interface{}, len(bookIDs))
	for i, id := range bookIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("GetNarratorsByBookIDs query: %w", err)
	}
	defer rows.Close()
	result := make(map[string][]Narrator, len(bookIDs))
	for rows.Next() {
		var bookID string
		var n Narrator
		if err := rows.Scan(&bookID, &n.ID, &n.Name, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("GetNarratorsByBookIDs scan: %w", err)
		}
		result[bookID] = append(result[bookID], n)
	}
	return result, rows.Err()
}
