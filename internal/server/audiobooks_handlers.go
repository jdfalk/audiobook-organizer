// file: internal/server/audiobooks_handlers.go
// version: 1.1.0
// guid: 221bde8e-dd34-458c-8afb-fe71f04597c0
//
// Audiobook HTTP handlers split out of server.go: book CRUD, batch
// operations, file/segment actions, tag read/write, cover art, path
// history, and the various book-level convenience endpoints used by
// the Library and Book-detail pages.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

func (s *Server) listAudiobooks(c *gin.Context) {
	// Build cache key from the full query string
	cacheKey := "list:" + c.Request.URL.RawQuery
	if cached, ok := s.listCache.Get(cacheKey); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	// Parse pagination parameters
	params := ParsePaginationParams(c)
	authorID := ParseQueryIntPtr(c, "author_id")
	seriesID := ParseQueryIntPtr(c, "series_id")

	// Parse optional filters
	sortOrder := ParseQueryString(c, "sort_order")
	if sortOrder != "" && sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "asc"
	}
	filters := ListFilters{
		IsPrimaryVersion: ParseQueryBoolPtr(c, "is_primary_version"),
		LibraryState:     ParseQueryString(c, "library_state"),
		Tag:              ParseQueryString(c, "tag"),
		SortBy:           ParseQueryString(c, "sort_by"),
		SortOrder:        sortOrder,
	}

	// Parse field filters from JSON query param
	if filtersJSON := c.Query("filters"); filtersJSON != "" {
		var fieldFilters []FieldFilter
		if err := json.Unmarshal([]byte(filtersJSON), &fieldFilters); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filters parameter: " + err.Error()})
			return
		}
		filters.FieldFilters = fieldFilters
	}

	// Call service
	books, err := s.audiobookService.GetAudiobooks(c.Request.Context(), params.Limit, params.Offset, params.Search, authorID, seriesID, filters)
	if err != nil {
		internalError(c, "failed to list audiobooks", err)
		return
	}

	// Enrich with author and series names
	enriched := s.audiobookService.EnrichAudiobooksWithNames(books)

	// Get total count for proper pagination
	totalCount := len(enriched)
	hasFilters := filters.IsPrimaryVersion != nil || filters.LibraryState != "" || filters.Tag != ""
	if params.Search == "" && authorID == nil && seriesID == nil {
		if hasFilters {
			if tc, err := s.audiobookService.CountAudiobooksFiltered(c.Request.Context(), filters); err == nil {
				totalCount = tc
			}
		} else {
			if tc, err := s.audiobookService.CountAudiobooks(c.Request.Context()); err == nil {
				totalCount = tc
			}
		}
	}

	resp := gin.H{"items": enriched, "count": totalCount, "limit": params.Limit, "offset": params.Offset}
	s.listCache.Set(cacheKey, resp)
	c.JSON(http.StatusOK, resp)
}

func (s *Server) listSoftDeletedAudiobooks(c *gin.Context) {
	params := ParsePaginationParams(c)
	olderThanDays := ParseQueryIntPtr(c, "older_than_days")

	books, err := s.audiobookService.GetSoftDeletedBooks(c.Request.Context(), params.Limit, params.Offset, olderThanDays)
	if err != nil {
		internalError(c, "failed to list deleted audiobooks", err)
		return
	}

	// Get total count (unpaginated) for proper pagination support
	allBooks, _ := s.audiobookService.GetSoftDeletedBooks(c.Request.Context(), 10000, 0, olderThanDays)
	total := len(allBooks)

	c.JSON(http.StatusOK, gin.H{
		"items":  books,
		"count":  len(books),
		"total":  total,
		"limit":  params.Limit,
		"offset": params.Offset,
	})
}

func (s *Server) purgeSoftDeletedAudiobooks(c *gin.Context) {
	deleteFiles := c.Query("delete_files") == "true"
	olderThanStr := c.Query("older_than_days")

	var olderThanDays *int
	if olderThanStr != "" {
		if days, err := strconv.Atoi(olderThanStr); err == nil && days > 0 {
			olderThanDays = &days
		}
	}

	result, err := s.audiobookService.PurgeSoftDeletedBooks(c.Request.Context(), deleteFiles, olderThanDays)
	if err != nil {
		internalError(c, "failed to purge deleted audiobooks", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) runAutoPurgeSoftDeleted() {
	if config.AppConfig.PurgeSoftDeletedAfterDays <= 0 {
		return
	}
	if s.Store() == nil {
		log.Printf("[DEBUG] Auto-purge skipped: database not initialized")
		return
	}

	days := config.AppConfig.PurgeSoftDeletedAfterDays
	result, err := s.audiobookService.PurgeSoftDeletedBooks(context.Background(), config.AppConfig.PurgeSoftDeletedDeleteFiles, &days)
	if err != nil {
		log.Printf("[WARN] Auto-purge failed: %v", err)
		return
	}

	if result.Attempted > 0 {
		log.Printf("[INFO] Auto-purge soft-deleted books: attempted=%d purged=%d files_deleted=%d errors=%d",
			result.Attempted, result.Purged, result.FilesDeleted, len(result.Errors))
		if len(result.Errors) > 0 {
			for _, e := range result.Errors {
				log.Printf("[WARN] Auto-purge error: %s", e)
			}
		}
	}
}

func (s *Server) restoreAudiobook(c *gin.Context) {
	id := c.Param("id")
	updated, err := s.audiobookService.RestoreAudiobook(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "audiobook restored",
		"book":    updated,
	})
}

func (s *Server) countAudiobooks(c *gin.Context) {
	count, err := s.audiobookService.CountAudiobooks(c.Request.Context())
	if err != nil {
		internalError(c, "failed to count audiobooks", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (s *Server) serveAudiobookCover(c *gin.Context) {
	id := c.Param("id")
	if config.AppConfig.RootDir == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "root_dir not configured"})
		return
	}
	coverPath := metadata.CoverPathForBook(config.AppConfig.RootDir, id)
	if coverPath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no cover art found"})
		return
	}
	c.File(coverPath)
}

func (s *Server) getAudiobook(c *gin.Context) {
	id := c.Param("id")

	book, err := s.audiobookService.GetAudiobook(c.Request.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		internalError(c, "failed to get audiobook", err)
		return
	}

	c.JSON(http.StatusOK, enrichBookForResponse(book))
}

// listAudiobookSegments returns file segments for a multi-file audiobook.
// Backward-compatible: internally queries book_files and returns data in the
// legacy BookSegment JSON shape so the frontend continues to work.
func (s *Server) listAudiobookSegments(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := s.Store().GetBookByID(id)
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	files, err := s.Store().GetBookFiles(book.ID)
	if err != nil {
		internalError(c, "failed to list book files", err)
		return
	}
	if files == nil {
		files = []database.BookFile{}
	}

	// Convert BookFile to legacy segment JSON shape with file_exists
	result := make([]gin.H, 0, len(files))
	for _, f := range files {
		_, statErr := os.Stat(f.FilePath)
		result = append(result, gin.H{
			"id":               f.ID,
			"book_id":          int(crc32.ChecksumIEEE([]byte(f.BookID))),
			"file_path":        f.FilePath,
			"format":           f.Format,
			"size_bytes":       f.FileSize,
			"duration_seconds": f.Duration / 1000, // BookFile stores ms
			"track_number":     f.TrackNumber,
			"total_tracks":     f.TrackCount,
			"segment_title":    f.Title,
			"file_hash":        f.FileHash,
			"active":           !f.Missing,
			"superseded_by":    nil,
			"created_at":       f.CreatedAt,
			"updated_at":       f.UpdatedAt,
			"file_exists":      statErr == nil,
		})
	}

	c.JSON(http.StatusOK, result)
}

// listBookFiles returns all book_files rows for a book with live disk-existence check.
func (s *Server) listBookFiles(c *gin.Context) {
	bookID := c.Param("id")
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	files, err := s.Store().GetBookFiles(bookID)
	if err != nil {
		internalError(c, "failed to get book files", err)
		return
	}
	if files == nil {
		files = []database.BookFile{}
	}
	results := make([]gin.H, 0, len(files))
	for _, f := range files {
		_, statErr := os.Stat(f.FilePath)
		results = append(results, gin.H{
			"id":                   f.ID,
			"book_id":              f.BookID,
			"file_path":            f.FilePath,
			"original_filename":    f.OriginalFilename,
			"itunes_path":          f.ITunesPath,
			"itunes_persistent_id": f.ITunesPersistentID,
			"track_number":         f.TrackNumber,
			"track_count":          f.TrackCount,
			"disc_number":          f.DiscNumber,
			"disc_count":           f.DiscCount,
			"title":                f.Title,
			"format":               f.Format,
			"codec":                f.Codec,
			"duration":             f.Duration,
			"file_size":            f.FileSize,
			"bitrate_kbps":         f.BitrateKbps,
			"sample_rate_hz":       f.SampleRateHz,
			"channels":             f.Channels,
			"bit_depth":            f.BitDepth,
			"file_hash":            f.FileHash,
			"missing":              f.Missing,
			"file_exists":          statErr == nil,
			"created_at":           f.CreatedAt,
			"updated_at":           f.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"files": results, "count": len(results)})
}

// extractTrackInfo parses track/disk numbers from segment filenames and updates segments.
func (s *Server) extractTrackInfo(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := s.Store().GetBookByID(id)
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	files, err := s.Store().GetBookFiles(book.ID)
	if err != nil {
		internalError(c, "failed to list book files", err)
		return
	}

	filePaths := make([]string, len(files))
	for i, f := range files {
		filePaths[i] = f.FilePath
	}

	trackInfos := metadata.ExtractTrackInfoBatch(filePaths)

	// Second pass: normalize track numbers to be 1-indexed and fill gaps
	// Some players/files use 0-based numbering (0-50); we always want 1-based (1-51)
	hasZero := false
	for _, info := range trackInfos {
		if info.TrackNumber != nil && *info.TrackNumber == 0 {
			hasZero = true
			break
		}
	}
	if hasZero {
		for i := range trackInfos {
			if trackInfos[i].TrackNumber != nil {
				n := *trackInfos[i].TrackNumber + 1
				trackInfos[i].TrackNumber = &n
			}
		}
	}

	// Assign sequential numbers to files that had no parseable track number
	usedNumbers := map[int]bool{}
	for _, info := range trackInfos {
		if info.TrackNumber != nil {
			usedNumbers[*info.TrackNumber] = true
		}
	}
	nextNum := 1
	total := len(files)
	for i := range trackInfos {
		if trackInfos[i].TrackNumber == nil {
			for usedNumbers[nextNum] {
				nextNum++
			}
			n := nextNum
			trackInfos[i].TrackNumber = &n
			usedNumbers[nextNum] = true
			nextNum++
		}
		// Ensure TotalTracks is set for all entries
		trackInfos[i].TotalTracks = &total
	}

	updated := 0
	for i, info := range trackInfos {
		oldTrack := files[i].TrackNumber
		if info.TrackNumber != nil {
			files[i].TrackNumber = *info.TrackNumber
		}
		if info.TotalTracks != nil {
			files[i].TrackCount = *info.TotalTracks
		}
		if err := s.Store().UpdateBookFile(files[i].ID, &files[i]); err != nil {
			log.Printf("WARN: failed to update book file %s track info: %v", files[i].ID, err)
			continue
		}
		updated++

		// Record the track number change in history
		var prevVal, newVal string
		if oldTrack != 0 {
			prevVal = strconv.Itoa(oldTrack)
		}
		if info.TrackNumber != nil {
			newVal = strconv.Itoa(*info.TrackNumber)
		}
		prev := prevVal
		nv := newVal
		_ = s.Store().RecordMetadataChange(&database.MetadataChangeRecord{
			BookID:        id,
			Field:         "track_number",
			PreviousValue: &prev,
			NewValue:      &nv,
			ChangeType:    "auto_number",
			Source:        "filename_extraction",
			ChangedAt:     time.Now(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"updated": updated,
		"total":   len(files),
		"files":   files,
	})
}

// relocateBookFiles updates segment file paths when files have been moved.
func (s *Server) relocateBookFiles(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := s.Store().GetBookByID(id)
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	var req RelocateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	files, err := s.Store().GetBookFiles(book.ID)
	if err != nil {
		internalError(c, "failed to list book files", err)
		return
	}

	result := RelocateResult{}

	if req.SegmentID != "" && req.NewPath != "" {
		// Individual mode: update one file (SegmentID maps to file ID)
		for i, f := range files {
			if f.ID == req.SegmentID {
				if _, statErr := os.Stat(req.NewPath); os.IsNotExist(statErr) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "new path does not exist on disk"})
					return
				}
				files[i].FilePath = req.NewPath
				if err := s.Store().UpdateBookFile(files[i].ID, &files[i]); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("update file %s: %v", f.ID, err))
				} else {
					result.Updated++
				}
				break
			}
		}
	} else if req.FolderPath != "" {
		// Folder mode: scan folder and match files by name
		dirEntries, err := os.ReadDir(req.FolderPath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("cannot read folder: %v", err)})
			return
		}

		// Build map of filename -> full path in the new folder
		fileMap := make(map[string]string)
		for _, de := range dirEntries {
			if !de.IsDir() {
				fileMap[de.Name()] = filepath.Join(req.FolderPath, de.Name())
			}
		}

		for i, f := range files {
			oldName := filepath.Base(f.FilePath)
			if newPath, ok := fileMap[oldName]; ok {
				files[i].FilePath = newPath
				if err := s.Store().UpdateBookFile(files[i].ID, &files[i]); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("update file %s: %v", f.ID, err))
				} else {
					result.Updated++
				}
			}
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "must provide segment_id+new_path or folder_path"})
		return
	}

	// Update book's file_path to match first file
	if result.Updated > 0 && len(files) > 0 {
		book.FilePath = files[0].FilePath
		if _, err := s.Store().UpdateBook(book.ID, book); err != nil {
			log.Printf("[WARN] failed to update book file_path: %v", err)
		}
	}

	c.JSON(http.StatusOK, result)
}

// getSegmentTags returns raw metadata tags for a specific segment file.
func (s *Server) getSegmentTags(c *gin.Context) {
	id := c.Param("id")
	segmentId := c.Param("segmentId")
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	book, err := s.Store().GetBookByID(id)
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "audiobook not found"})
		return
	}

	found, err := s.Store().GetBookFileByID(book.ID, segmentId)
	if err != nil {
		internalError(c, "failed to get book file", err)
		return
	}
	if found == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "segment not found"})
		return
	}

	tags := map[string]string{}
	usedFallback := false
	tagsReadError := ""

	meta, err := metadata.ExtractMetadata(found.FilePath, nil)
	if err != nil {
		tagsReadError = err.Error()
	} else {
		usedFallback = meta.UsedFilenameFallback
		if meta.Title != "" {
			tags["title"] = meta.Title
		}
		if meta.Artist != "" {
			tags["artist"] = meta.Artist
		}
		if meta.Album != "" {
			tags["album"] = meta.Album
		}
		if meta.Genre != "" {
			tags["genre"] = meta.Genre
		}
		if meta.Series != "" {
			tags["series"] = meta.Series
		}
		if meta.SeriesIndex != 0 {
			tags["series_index"] = strconv.Itoa(meta.SeriesIndex)
		}
		if meta.Comments != "" {
			tags["comments"] = meta.Comments
		}
		if meta.Year != 0 {
			tags["year"] = strconv.Itoa(meta.Year)
		}
		if meta.Narrator != "" {
			tags["narrator"] = meta.Narrator
		}
		if meta.Language != "" {
			tags["language"] = meta.Language
		}
		if meta.Publisher != "" {
			tags["publisher"] = meta.Publisher
		}
		if meta.ISBN10 != "" {
			tags["isbn10"] = meta.ISBN10
		}
		if meta.ISBN13 != "" {
			tags["isbn13"] = meta.ISBN13
		}
	}

	resp := gin.H{
		"segment_id":             found.ID,
		"file_path":              found.FilePath,
		"format":                 found.Format,
		"size_bytes":             found.FileSize,
		"duration_sec":           found.Duration / 1000,
		"track_number":           found.TrackNumber,
		"total_tracks":           found.TrackCount,
		"tags":                   tags,
		"used_filename_fallback": usedFallback,
	}
	if tagsReadError != "" {
		resp["tags_read_error"] = tagsReadError
	}

	c.JSON(http.StatusOK, resp)
}

func (s *Server) getBookMetadataHistory(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	records, err := s.Store().GetBookChangeHistory(id, limit)
	if err != nil {
		internalError(c, "failed to get metadata history", err)
		return
	}
	if records == nil {
		records = []database.MetadataChangeRecord{}
	}
	c.JSON(http.StatusOK, gin.H{"items": records, "count": len(records)})
}

func (s *Server) getAudiobookFieldStates(c *gin.Context) {
	id := c.Param("id")
	states, err := s.metadataStateService.LoadMetadataState(id)
	if err != nil {
		internalError(c, "failed to get field states", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"field_states": states})
}

func (s *Server) getFieldMetadataHistory(c *gin.Context) {
	id := c.Param("id")
	field := c.Param("field")
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	records, err := s.Store().GetMetadataChangeHistory(id, field, limit)
	if err != nil {
		internalError(c, "failed to get field history", err)
		return
	}
	if records == nil {
		records = []database.MetadataChangeRecord{}
	}
	c.JSON(http.StatusOK, gin.H{"items": records, "count": len(records)})
}

func (s *Server) undoMetadataChange(c *gin.Context) {
	id := c.Param("id")
	field := c.Param("field")
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get the latest change for this field
	records, err := s.Store().GetMetadataChangeHistory(id, field, 1)
	if err != nil {
		internalError(c, "failed to get field history", err)
		return
	}
	if len(records) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no change history found for this field"})
		return
	}

	latest := records[0]

	// Apply the previous value back via metadata state service
	if latest.PreviousValue != nil {
		var prevValue any
		if err := json.Unmarshal([]byte(*latest.PreviousValue), &prevValue); err != nil {
			prevValue = *latest.PreviousValue
		}
		if err := s.metadataStateService.SetOverride(id, field, prevValue, false); err != nil {
			internalError(c, "failed to apply undo", err)
			return
		}
	} else {
		// Previous value was nil, so clear the override
		if err := s.metadataStateService.ClearOverride(id, field); err != nil {
			// Ignore "not found" errors when clearing
			if !strings.Contains(err.Error(), "not found") {
				internalError(c, "failed to clear override", err)
				return
			}
		}
	}

	// Record the undo itself
	undoRecord := &database.MetadataChangeRecord{
		BookID:        id,
		Field:         field,
		PreviousValue: latest.NewValue,
		NewValue:      latest.PreviousValue,
		ChangeType:    "undo",
		Source:        "manual",
		ChangedAt:     time.Now(),
	}
	if err := s.Store().RecordMetadataChange(undoRecord); err != nil {
		log.Printf("[WARN] failed to record undo change for %s/%s: %v", id, field, err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "undo applied", "field": field, "reverted_to": latest.PreviousValue})
}

// undoLastApply reverts all fields changed in the most recent metadata apply for a book.
func (s *Server) undoLastApply(c *gin.Context) {
	id := c.Param("id")
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Get recent history for this book (enough to find the last apply batch)
	history, err := s.Store().GetBookChangeHistory(id, 50)
	if err != nil {
		internalError(c, "failed to get change history", err)
		return
	}
	if len(history) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no change history found"})
		return
	}

	// Find the most recent non-undo change timestamp to identify the batch
	var batchTime time.Time
	for _, rec := range history {
		if rec.ChangeType != "undo" {
			batchTime = rec.ChangedAt
			break
		}
	}
	if batchTime.IsZero() {
		c.JSON(http.StatusNotFound, gin.H{"error": "no changes to undo"})
		return
	}

	// Collect all changes from this batch (within 2 seconds of each other)
	var batchRecords []*database.MetadataChangeRecord
	for i := range history {
		rec := &history[i]
		if rec.ChangeType == "undo" {
			continue
		}
		diff := batchTime.Sub(rec.ChangedAt)
		if diff < 0 {
			diff = -diff
		}
		if diff <= 2*time.Second {
			batchRecords = append(batchRecords, rec)
		}
	}

	if len(batchRecords) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no changes to undo"})
		return
	}

	// Undo each field in the batch
	undoneFields := []string{}
	for _, rec := range batchRecords {
		if rec.PreviousValue != nil {
			var prevValue any
			if jsonErr := json.Unmarshal([]byte(*rec.PreviousValue), &prevValue); jsonErr != nil {
				prevValue = *rec.PreviousValue
			}
			if setErr := s.metadataStateService.SetOverride(id, rec.Field, prevValue, false); setErr != nil {
				log.Printf("[WARN] undo-last-apply: failed to revert %s for %s: %v", rec.Field, id, setErr)
				continue
			}
		} else {
			if clrErr := s.metadataStateService.ClearOverride(id, rec.Field); clrErr != nil {
				if !strings.Contains(clrErr.Error(), "not found") {
					log.Printf("[WARN] undo-last-apply: failed to clear %s for %s: %v", rec.Field, id, clrErr)
					continue
				}
			}
		}
		undoneFields = append(undoneFields, rec.Field)

		// Record the undo
		undoRec := &database.MetadataChangeRecord{
			BookID:        id,
			Field:         rec.Field,
			PreviousValue: rec.NewValue,
			NewValue:      rec.PreviousValue,
			ChangeType:    "undo",
			Source:        "bulk-search-undo",
			ChangedAt:     time.Now(),
		}
		if recErr := s.Store().RecordMetadataChange(undoRec); recErr != nil {
			log.Printf("[WARN] undo-last-apply: failed to record undo for %s/%s: %v", id, rec.Field, recErr)
		}
	}

	// Re-write tags to files if write-back is enabled, restoring original values
	if len(undoneFields) > 0 && s.writeBackBatcher != nil {
		s.writeBackBatcher.Enqueue(id)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       fmt.Sprintf("Undid %d field(s)", len(undoneFields)),
		"undone_fields": undoneFields,
	})
}

func (s *Server) getBookPathHistory(c *gin.Context) {
	id := c.Param("id")
	history, err := s.Store().GetBookPathHistory(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"history": []any{}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"history": history})
}

func (s *Server) getAudiobookExternalIDs(c *gin.Context) {
	id := c.Param("id")
	eidStore := asExternalIDStore(s.Store())
	if eidStore == nil {
		c.JSON(http.StatusOK, gin.H{"external_ids": []any{}, "itunes_linked": false})
		return
	}
	extIDs, err := eidStore.GetExternalIDsForBook(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"external_ids": []any{}, "itunes_linked": false})
		return
	}
	itunesLinked := false
	for _, eid := range extIDs {
		if eid.Source == "itunes" && !eid.Tombstoned {
			itunesLinked = true
			break
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"external_ids":  extIDs,
		"itunes_linked": itunesLinked,
		"total":         len(extIDs),
	})
}

func (s *Server) getAudiobookTags(c *gin.Context) {
	id := c.Param("id")
	compareID := c.Query("compare_id")
	snapshotTS := c.Query("snapshot_ts")
	if snapshotTS != "" {
		if _, err := time.Parse(time.RFC3339Nano, snapshotTS); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid snapshot_ts format, use RFC3339Nano"})
			return
		}
	}
	resp, err := s.audiobookService.GetAudiobookTags(c.Request.Context(), id, compareID, snapshotTS)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		internalError(c, "failed to get tags", err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (s *Server) listAllUserTags(c *gin.Context) {
	tags, err := s.audiobookService.ListAllUserTags()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tags == nil {
		tags = []database.TagWithCount{}
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

func (s *Server) getBookUserTags(c *gin.Context) {
	id := c.Param("id")
	tags, err := s.audiobookService.GetBookUserTags(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tags == nil {
		tags = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

// getBookTagsDetailed returns a book's tags with their source
// attribution ('user' vs 'system'), so the frontend can render
// user-applied and system-applied tags differently. System tags
// follow the namespace from migrations 47/48 — dedup:*,
// metadata:source:*, metadata:language:*, etc. — and should be
// shown as outlined, non-deletable chips by default.
//
// Backlog 7.8. Separate endpoint from /user-tags so existing
// callers that only want the string list don't pay for the
// source lookup.
func (s *Server) getBookTagsDetailed(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book id is required"})
		return
	}
	tags, err := s.Store().GetBookTagsDetailed(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tags == nil {
		tags = []database.BookTag{}
	}
	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

// getBookAlternativeTitles handles GET /audiobooks/:id/alternative-titles.
// Returns the list of alt titles for a book along with source/language
// metadata so the UI can show where each came from.
func (s *Server) getBookAlternativeTitles(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	alts, err := s.Store().GetBookAlternativeTitles(id)
	if err != nil {
		internalError(c, "failed to get alternative titles", err)
		return
	}
	if alts == nil {
		alts = []database.BookAlternativeTitle{}
	}
	c.JSON(http.StatusOK, gin.H{"alternative_titles": alts})
}

// addBookAlternativeTitle handles POST /audiobooks/:id/alternative-titles.
// Body: {"title": "...", "source": "user", "language": "en"}
// Idempotent on (book_id, title) — re-adding the same title is a no-op.
func (s *Server) addBookAlternativeTitle(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	var body struct {
		Title    string `json:"title"`
		Source   string `json:"source,omitempty"`
		Language string `json:"language,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	// Confirm the book exists before inserting — avoids orphan alt
	// title rows for deleted books.
	if book, err := s.Store().GetBookByID(id); err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
		return
	}
	if err := s.Store().AddBookAlternativeTitle(id, body.Title, body.Source, body.Language); err != nil {
		internalError(c, "failed to add alternative title", err)
		return
	}
	alts, _ := s.Store().GetBookAlternativeTitles(id)
	c.JSON(http.StatusOK, gin.H{"alternative_titles": alts})
}

// removeBookAlternativeTitle handles DELETE /audiobooks/:id/alternative-titles.
// Body: {"title": "..."}
// Body is used instead of a path param so titles with slashes/special
// characters don't need URL-encoding hoops.
func (s *Server) removeBookAlternativeTitle(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	if err := s.Store().RemoveBookAlternativeTitle(id, body.Title); err != nil {
		internalError(c, "failed to remove alternative title", err)
		return
	}
	alts, _ := s.Store().GetBookAlternativeTitles(id)
	c.JSON(http.StatusOK, gin.H{"alternative_titles": alts})
}

func (s *Server) batchUpdateTags(c *gin.Context) {
	var body struct {
		BookIDs    []string `json:"book_ids"`
		AddTags    []string `json:"add_tags"`
		RemoveTags []string `json:"remove_tags"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if len(body.BookIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book_ids is required"})
		return
	}
	if len(body.AddTags) == 0 && len(body.RemoveTags) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one of add_tags or remove_tags is required"})
		return
	}
	// Filter out empty strings from tag arrays
	filterEmpty := func(tags []string) []string {
		out := make([]string, 0, len(tags))
		for _, t := range tags {
			if strings.TrimSpace(t) != "" {
				out = append(out, t)
			}
		}
		return out
	}
	body.AddTags = filterEmpty(body.AddTags)
	body.RemoveTags = filterEmpty(body.RemoveTags)
	updated, err := s.audiobookService.BatchUpdateUserTags(body.BookIDs, body.AddTags, body.RemoveTags)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"updated": updated})
}

func (s *Server) getBookChangelog(c *gin.Context) {
	id := c.Param("id")
	entries, err := s.changelogService.GetBookChangelog(id)
	if err != nil {
		internalError(c, "failed to get changelog", err)
		return
	}
	if entries == nil {
		entries = []activity.ChangeLogEntry{}
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries})
}

func (s *Server) updateAudiobook(c *gin.Context) {
	id := c.Param("id")

	var payload map[string]any
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Fetch old book for change history comparison
	var oldBook *database.Book
	if s.Store() != nil {
		oldBook, _ = s.Store().GetBookByID(id)
	}

	updatedBook, err := s.audiobookUpdateService.UpdateAudiobook(id, payload)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		internalError(c, "failed to update audiobook", err)
		return
	}

	// Record metadata change history for manual edits
	if oldBook != nil && s.Store() != nil {
		now := time.Now()
		manualChanges := []struct {
			field  string
			oldVal string
			newVal string
		}{
			{"title", oldBook.Title, updatedBook.Title},
			{"narrator", ptrStr(oldBook.Narrator), ptrStr(updatedBook.Narrator)},
			{"publisher", ptrStr(oldBook.Publisher), ptrStr(updatedBook.Publisher)},
			{"language", ptrStr(oldBook.Language), ptrStr(updatedBook.Language)},
		}
		// Compare author names
		oldAuthor := ""
		if oldBook.AuthorID != nil {
			if a, err := s.Store().GetAuthorByID(*oldBook.AuthorID); err == nil && a != nil {
				oldAuthor = a.Name
			}
		}
		newAuthor := ""
		if updatedBook.AuthorID != nil {
			if a, err := s.Store().GetAuthorByID(*updatedBook.AuthorID); err == nil && a != nil {
				newAuthor = a.Name
			}
		}
		manualChanges = append(manualChanges, struct {
			field  string
			oldVal string
			newVal string
		}{"author_name", oldAuthor, newAuthor})
		// Compare year
		oldYear := ""
		if oldBook.AudiobookReleaseYear != nil {
			oldYear = strconv.Itoa(*oldBook.AudiobookReleaseYear)
		}
		newYear := ""
		if updatedBook.AudiobookReleaseYear != nil {
			newYear = strconv.Itoa(*updatedBook.AudiobookReleaseYear)
		}
		manualChanges = append(manualChanges, struct {
			field  string
			oldVal string
			newVal string
		}{"audiobook_release_year", oldYear, newYear})

		for _, c := range manualChanges {
			if c.newVal == "" || c.newVal == c.oldVal {
				continue
			}
			oldJSON, _ := json.Marshal(c.oldVal)
			newJSON, _ := json.Marshal(c.newVal)
			oldStr := string(oldJSON)
			newStr := string(newJSON)
			record := &database.MetadataChangeRecord{
				BookID:        id,
				Field:         c.field,
				PreviousValue: &oldStr,
				NewValue:      &newStr,
				ChangeType:    "manual",
				Source:        "manual",
				ChangedAt:     now,
			}
			if err := s.Store().RecordMetadataChange(record); err != nil {
				log.Printf("[WARN] failed to record manual metadata change for %s.%s: %v", id, c.field, err)
			}
		}
	}

	// Write updated metadata back to the audio file
	if updatedBook.FilePath != "" {
		tagMap := make(map[string]interface{})
		if v, ok := payload["title"].(string); ok && v != "" {
			tagMap["title"] = v
		}
		if v, ok := payload["author_name"].(string); ok && v != "" {
			tagMap["artist"] = v
		}
		if v, ok := payload["publisher"].(string); ok && v != "" {
			tagMap["publisher"] = v
		}
		if v, ok := payload["narrator"].(string); ok && v != "" {
			tagMap["album_artist"] = v
		}
		if v, ok := payload["audiobook_release_year"].(float64); ok && v != 0 {
			tagMap["year"] = int(v)
		}
		// If we have multiple authors in join table, combine with " & " for file tags
		if _, hasAuthor := tagMap["artist"]; !hasAuthor && s.Store() != nil {
			if authors, err := s.Store().GetBookAuthors(id); err == nil && len(authors) > 1 {
				names := make([]string, 0, len(authors))
				for _, ba := range authors {
					if a, err := s.Store().GetAuthorByID(ba.AuthorID); err == nil && a != nil {
						names = append(names, a.Name)
					}
				}
				if len(names) > 0 {
					tagMap["artist"] = strings.Join(names, ", ")
				}
			}
		}
		// If we have multiple narrators in join table, combine with " & " for file tags
		if _, hasNarr := tagMap["album_artist"]; !hasNarr && s.Store() != nil {
			if narrators, err := s.Store().GetBookNarrators(id); err == nil && len(narrators) > 1 {
				names := make([]string, 0, len(narrators))
				for _, bn := range narrators {
					if n, err := s.Store().GetNarratorByID(bn.NarratorID); err == nil && n != nil {
						names = append(names, n.Name)
					}
				}
				if len(names) > 0 {
					tagMap["album_artist"] = strings.Join(names, " & ")
				}
			}
		}
		if len(tagMap) > 0 {
			if isProtectedPath(updatedBook.FilePath) {
				log.Printf("[INFO] skipping write-back for protected path: %s", updatedBook.FilePath)
			} else {
				opConfig := fileops.OperationConfig{VerifyChecksums: true}
				if writeErr := metadata.WriteMetadataToFile(updatedBook.FilePath, tagMap, opConfig); writeErr != nil {
					log.Printf("[WARN] write-back failed for %s: %v", updatedBook.FilePath, writeErr)
				} else {
					// Stamp last_written_at after successful write-back.
					if stampErr := s.Store().SetLastWrittenAt(updatedBook.ID, time.Now()); stampErr != nil {
						log.Printf("[WARN] failed to stamp last_written_at for book %s: %v", updatedBook.ID, stampErr)
					}
				}
			}
		}
	}

	// Enqueue for iTunes auto write-back if enabled
	if s.writeBackBatcher != nil {
		s.writeBackBatcher.Enqueue(id)
	}

	c.JSON(http.StatusOK, enrichBookForResponse(updatedBook))
}

func (s *Server) deleteAudiobook(c *gin.Context) {
	id := c.Param("id")
	blockHash := c.Query("block_hash") == "true"
	softDelete := c.Query("soft_delete") == "true"

	opts := &DeleteAudiobookOptions{
		SoftDelete: softDelete,
		BlockHash:  blockHash,
	}

	result, err := s.audiobookService.DeleteAudiobook(c.Request.Context(), id, opts)
	if err != nil {
		if strings.Contains(err.Error(), "already soft deleted") {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) batchUpdateAudiobooks(c *gin.Context) {
	var req BatchUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp := s.batchService.UpdateAudiobooks(&req)

	// Enqueue all updated books for iTunes auto write-back
	if s.writeBackBatcher != nil && resp != nil {
		for _, item := range resp.Results {
			if item.Success {
				s.writeBackBatcher.Enqueue(item.ID)
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}

func (s *Server) batchOperations(c *gin.Context) {
	var req BatchOperationsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Operations) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no operations provided"})
		return
	}
	if len(req.Operations) > 10000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max 10000 operations per request"})
		return
	}

	resp := s.batchService.ExecuteOperations(&req)

	if s.writeBackBatcher != nil {
		for _, r := range resp.Results {
			if r.Success {
				s.writeBackBatcher.Enqueue(r.ID)
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}

// getBookChanges returns change tracking records for a book.
func (s *Server) getBookChanges(c *gin.Context) {
	id := c.Param("id")
	changes, err := s.Store().GetBookChanges(id)
	if err != nil {
		internalError(c, "failed to get book changes", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"changes": changes})
}
