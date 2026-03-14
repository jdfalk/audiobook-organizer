// file: internal/server/diagnostics_service.go
// version: 1.0.0
// guid: d1a9n0st-1cs0-s3rv-1c3z-1pexp0rt001

package server

import (
	"archive/zip"
	"encoding/json"
	"os"
	"runtime"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

// DiagnosticsService generates diagnostic ZIP exports for troubleshooting and AI analysis.
type DiagnosticsService struct {
	db            database.Store
	aiPipeline    interface{} // *ai.Pipeline or nil
	itunesXMLPath string
}

// NewDiagnosticsService creates a new DiagnosticsService.
func NewDiagnosticsService(db database.Store, aiPipeline interface{}, itunesXMLPath string) *DiagnosticsService {
	return &DiagnosticsService{
		db:            db,
		aiPipeline:    aiPipeline,
		itunesXMLPath: itunesXMLPath,
	}
}

// slimBook is a reduced representation of a book for diagnostics export.
type slimBook struct {
	ID                   string  `json:"id"`
	Title                string  `json:"title"`
	AuthorID             *int    `json:"author_id,omitempty"`
	Narrator             *string `json:"narrator,omitempty"`
	SeriesID             *int    `json:"series_id,omitempty"`
	Format               string  `json:"format,omitempty"`
	Duration             *int    `json:"duration,omitempty"`
	FilePath             string  `json:"file_path"`
	FileSize             *int64  `json:"file_size,omitempty"`
	VersionGroupID       *string `json:"version_group_id,omitempty"`
	IsPrimaryVersion     *bool   `json:"is_primary_version,omitempty"`
	WorkID               *string `json:"work_id,omitempty"`
	ITunesPersistentID   *string `json:"itunes_persistent_id,omitempty"`
	AudiobookReleaseYear *int    `json:"audiobook_release_year,omitempty"`
	Publisher            *string `json:"publisher,omitempty"`
	LibraryState         *string `json:"library_state,omitempty"`
	MarkedForDeletion    *bool   `json:"marked_for_deletion,omitempty"`
}

func toSlimBook(b database.Book) slimBook {
	return slimBook{
		ID:                   b.ID,
		Title:                b.Title,
		AuthorID:             b.AuthorID,
		Narrator:             b.Narrator,
		SeriesID:             b.SeriesID,
		Format:               b.Format,
		Duration:             b.Duration,
		FilePath:             b.FilePath,
		FileSize:             b.FileSize,
		VersionGroupID:       b.VersionGroupID,
		IsPrimaryVersion:     b.IsPrimaryVersion,
		WorkID:               b.WorkID,
		ITunesPersistentID:   b.ITunesPersistentID,
		AudiobookReleaseYear: b.AudiobookReleaseYear,
		Publisher:            b.Publisher,
		LibraryState:         b.LibraryState,
		MarkedForDeletion:    b.MarkedForDeletion,
	}
}

// systemInfo is the structure written to system_info.json.
type systemInfo struct {
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	GoVersion   string `json:"go_version"`
	BookCount   int    `json:"book_count"`
	AuthorCount int    `json:"author_count"`
	SeriesCount int    `json:"series_count"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

// authorWithCounts extends Author with book/file counts.
type authorWithCounts struct {
	database.Author
	BookCount int `json:"book_count"`
	FileCount int `json:"file_count"`
}

// seriesWithCounts extends Series with book/file counts.
type seriesWithCounts struct {
	database.Series
	BookCount int `json:"book_count"`
	FileCount int `json:"file_count"`
}

// versionGroup represents a group of books sharing a version group ID.
type versionGroup struct {
	GroupID string     `json:"group_id"`
	Books   []slimBook `json:"books"`
}

// itunesAlbumSummary summarizes tracks grouped by album from iTunes XML.
type itunesAlbumSummary struct {
	Album      string `json:"album"`
	TrackCount int    `json:"track_count"`
	Artist     string `json:"artist,omitempty"`
}

// missingFieldEntry records a book with missing metadata fields.
type missingFieldEntry struct {
	BookID        string   `json:"book_id"`
	Title         string   `json:"title"`
	MissingFields []string `json:"missing_fields"`
}

// GenerateExport creates a temporary ZIP file containing diagnostic data.
// Category must be one of: "deduplication", "error_analysis", "metadata_quality", "general".
func (ds *DiagnosticsService) GenerateExport(category, description string) (string, error) {
	// Create temp file for the ZIP
	tmpFile, err := os.CreateTemp("", "diagnostics-*.zip")
	if err != nil {
		return "", err
	}
	zipPath := tmpFile.Name()

	zw := zip.NewWriter(tmpFile)

	// Collect all books (paginated)
	allBooks, err := ds.collectAllBooks()
	if err != nil {
		zw.Close()
		tmpFile.Close()
		os.Remove(zipPath)
		return "", err
	}

	// Always: system_info.json
	if err := ds.writeSystemInfo(zw, category, description); err != nil {
		zw.Close()
		tmpFile.Close()
		os.Remove(zipPath)
		return "", err
	}

	// Always: books.json
	if err := ds.writeBooks(zw, allBooks); err != nil {
		zw.Close()
		tmpFile.Close()
		os.Remove(zipPath)
		return "", err
	}

	// Always: authors.json
	if err := ds.writeAuthors(zw); err != nil {
		zw.Close()
		tmpFile.Close()
		os.Remove(zipPath)
		return "", err
	}

	// Always: series.json
	if err := ds.writeSeries(zw); err != nil {
		zw.Close()
		tmpFile.Close()
		os.Remove(zipPath)
		return "", err
	}

	// Always: batch.jsonl (stub for now)
	if err := ds.writeBatchJSONL(zw); err != nil {
		zw.Close()
		tmpFile.Close()
		os.Remove(zipPath)
		return "", err
	}

	// Category-specific files
	if category == "error_analysis" || category == "general" {
		if err := ds.writeLogs(zw); err != nil {
			zw.Close()
			tmpFile.Close()
			os.Remove(zipPath)
			return "", err
		}
		if err := ds.writeOperations(zw); err != nil {
			zw.Close()
			tmpFile.Close()
			os.Remove(zipPath)
			return "", err
		}
	}

	if category == "deduplication" || category == "general" {
		if err := ds.writeVersionGroups(zw, allBooks); err != nil {
			zw.Close()
			tmpFile.Close()
			os.Remove(zipPath)
			return "", err
		}
		if err := ds.writeITunesAlbums(zw); err != nil {
			zw.Close()
			tmpFile.Close()
			os.Remove(zipPath)
			return "", err
		}
	}

	if category == "metadata_quality" || category == "general" {
		if err := ds.writeMissingFields(zw, allBooks); err != nil {
			zw.Close()
			tmpFile.Close()
			os.Remove(zipPath)
			return "", err
		}
	}

	if err := zw.Close(); err != nil {
		tmpFile.Close()
		os.Remove(zipPath)
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(zipPath)
		return "", err
	}

	return zipPath, nil
}

// collectAllBooks paginates through GetAllBooks until no more results.
func (ds *DiagnosticsService) collectAllBooks() ([]database.Book, error) {
	const pageSize = 10000
	var allBooks []database.Book
	offset := 0
	for {
		books, err := ds.db.GetAllBooks(pageSize, offset)
		if err != nil {
			return nil, err
		}
		if len(books) == 0 {
			break
		}
		allBooks = append(allBooks, books...)
		offset += pageSize
	}
	return allBooks, nil
}

// writeJSON is a helper that marshals data and writes it into the ZIP.
func writeJSON(zw *zip.Writer, filename string, data interface{}) error {
	w, err := zw.Create(filename)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// writeRaw writes raw bytes into the ZIP.
func writeRaw(zw *zip.Writer, filename string, data []byte) error {
	w, err := zw.Create(filename)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func (ds *DiagnosticsService) writeSystemInfo(zw *zip.Writer, category, description string) error {
	bookCount, _ := ds.db.CountBooks()
	authorCount, _ := ds.db.CountAuthors()
	seriesCount, _ := ds.db.CountSeries()

	info := systemInfo{
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		GoVersion:   runtime.Version(),
		BookCount:   bookCount,
		AuthorCount: authorCount,
		SeriesCount: seriesCount,
		Category:    category,
		Description: description,
	}
	return writeJSON(zw, "system_info.json", info)
}

func (ds *DiagnosticsService) writeBooks(zw *zip.Writer, allBooks []database.Book) error {
	slim := make([]slimBook, len(allBooks))
	for i, b := range allBooks {
		slim[i] = toSlimBook(b)
	}
	return writeJSON(zw, "books.json", slim)
}

func (ds *DiagnosticsService) writeAuthors(zw *zip.Writer) error {
	authors, err := ds.db.GetAllAuthors()
	if err != nil {
		return err
	}

	bookCounts, err := ds.db.GetAllAuthorBookCounts()
	if err != nil {
		bookCounts = map[int]int{}
	}

	fileCounts, err := ds.db.GetAllAuthorFileCounts()
	if err != nil {
		fileCounts = map[int]int{}
	}

	result := make([]authorWithCounts, len(authors))
	for i, a := range authors {
		result[i] = authorWithCounts{
			Author:    a,
			BookCount: bookCounts[a.ID],
			FileCount: fileCounts[a.ID],
		}
	}
	return writeJSON(zw, "authors.json", result)
}

func (ds *DiagnosticsService) writeSeries(zw *zip.Writer) error {
	series, err := ds.db.GetAllSeries()
	if err != nil {
		return err
	}

	bookCounts, err := ds.db.GetAllSeriesBookCounts()
	if err != nil {
		bookCounts = map[int]int{}
	}

	fileCounts, err := ds.db.GetAllSeriesFileCounts()
	if err != nil {
		fileCounts = map[int]int{}
	}

	result := make([]seriesWithCounts, len(series))
	for i, s := range series {
		result[i] = seriesWithCounts{
			Series:    s,
			BookCount: bookCounts[s.ID],
			FileCount: fileCounts[s.ID],
		}
	}
	return writeJSON(zw, "series.json", result)
}

// writeBatchJSONL writes a stub batch.jsonl file. Task 5 will implement the real builder.
func (ds *DiagnosticsService) writeBatchJSONL(zw *zip.Writer) error {
	return writeRaw(zw, "batch.jsonl", []byte{})
}

// buildBatchJSONL is a stub for Task 5. Returns empty bytes.
func (ds *DiagnosticsService) buildBatchJSONL() []byte {
	return []byte{}
}

func (ds *DiagnosticsService) writeLogs(zw *zip.Writer) error {
	logs, err := ds.db.GetSystemActivityLogs("", 10000)
	if err != nil {
		logs = []database.SystemActivityLog{}
	}

	// Filter to last 24 hours
	cutoff := time.Now().Add(-24 * time.Hour)
	filtered := make([]database.SystemActivityLog, 0, len(logs))
	for _, log := range logs {
		if log.CreatedAt.After(cutoff) {
			filtered = append(filtered, log)
		}
	}

	return writeJSON(zw, "logs.json", filtered)
}

func (ds *DiagnosticsService) writeOperations(zw *zip.Writer) error {
	ops, err := ds.db.GetRecentOperations(100)
	if err != nil {
		ops = []database.Operation{}
	}
	return writeJSON(zw, "operations.json", ops)
}

func (ds *DiagnosticsService) writeVersionGroups(zw *zip.Writer, allBooks []database.Book) error {
	groups := map[string][]slimBook{}
	for _, b := range allBooks {
		if b.VersionGroupID != nil && *b.VersionGroupID != "" {
			groups[*b.VersionGroupID] = append(groups[*b.VersionGroupID], toSlimBook(b))
		}
	}

	result := make([]versionGroup, 0, len(groups))
	for gid, books := range groups {
		result = append(result, versionGroup{GroupID: gid, Books: books})
	}
	return writeJSON(zw, "version_groups.json", result)
}

func (ds *DiagnosticsService) writeITunesAlbums(zw *zip.Writer) error {
	if ds.itunesXMLPath == "" {
		return writeJSON(zw, "itunes_albums.json", []itunesAlbumSummary{})
	}

	lib, err := itunes.ParseLibrary(ds.itunesXMLPath)
	if err != nil {
		// If parsing fails, write empty array
		return writeJSON(zw, "itunes_albums.json", []itunesAlbumSummary{})
	}

	albumMap := map[string]*itunesAlbumSummary{}
	for _, track := range lib.Tracks {
		album := track.Album
		if album == "" {
			album = "(no album)"
		}
		if existing, ok := albumMap[album]; ok {
			existing.TrackCount++
		} else {
			albumMap[album] = &itunesAlbumSummary{
				Album:      album,
				TrackCount: 1,
				Artist:     track.Artist,
			}
		}
	}

	result := make([]itunesAlbumSummary, 0, len(albumMap))
	for _, summary := range albumMap {
		result = append(result, *summary)
	}
	return writeJSON(zw, "itunes_albums.json", result)
}

func (ds *DiagnosticsService) writeMissingFields(zw *zip.Writer, allBooks []database.Book) error {
	var missing []missingFieldEntry
	for _, b := range allBooks {
		var fields []string
		if b.Title == "" {
			fields = append(fields, "title")
		}
		if b.AuthorID == nil {
			fields = append(fields, "author")
		}
		if b.SeriesID == nil {
			fields = append(fields, "series")
		}
		if len(fields) > 0 {
			missing = append(missing, missingFieldEntry{
				BookID:        b.ID,
				Title:         b.Title,
				MissingFields: fields,
			})
		}
	}
	if missing == nil {
		missing = []missingFieldEntry{}
	}
	return writeJSON(zw, "missing_fields.json", missing)
}
