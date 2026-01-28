// file: internal/server/itunes.go
// version: 1.0.0
// guid: 719912e9-7b5f-48e1-afa6-1b0b7f57c2fa

package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/oklog/ulid/v2"
)

const (
	itunesImportProgressBatch = 10
	itunesImportErrorLimit    = 50
)

// ITunesValidateRequest represents a validation request for an iTunes library.
type ITunesValidateRequest struct {
	LibraryPath string `json:"library_path" binding:"required"`
}

// ITunesValidateResponse summarizes validation results for an iTunes library.
type ITunesValidateResponse struct {
	TotalTracks     int      `json:"total_tracks"`
	AudiobookTracks int      `json:"audiobook_tracks"`
	FilesFound      int      `json:"files_found"`
	FilesMissing    int      `json:"files_missing"`
	MissingPaths    []string `json:"missing_paths,omitempty"`
	DuplicateCount  int      `json:"duplicate_count"`
	EstimatedTime   string   `json:"estimated_import_time"`
}

// ITunesImportRequest represents a request to import an iTunes library.
type ITunesImportRequest struct {
	LibraryPath      string `json:"library_path" binding:"required"`
	ImportMode       string `json:"import_mode" binding:"required,oneof=organized import organize"`
	PreserveLocation bool   `json:"preserve_location"`
	ImportPlaylists  bool   `json:"import_playlists"`
	SkipDuplicates   bool   `json:"skip_duplicates"`
}

// ITunesImportResponse acknowledges an iTunes import operation.
type ITunesImportResponse struct {
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}

// ITunesWriteBackRequest represents a write-back request for iTunes updates.
type ITunesWriteBackRequest struct {
	LibraryPath  string   `json:"library_path" binding:"required"`
	AudiobookIDs []string `json:"audiobook_ids"`
	CreateBackup bool     `json:"create_backup"`
}

// ITunesWriteBackResponse summarizes write-back results.
type ITunesWriteBackResponse struct {
	Success      bool   `json:"success"`
	UpdatedCount int    `json:"updated_count"`
	BackupPath   string `json:"backup_path,omitempty"`
	Message      string `json:"message"`
}

// ITunesImportStatusResponse reports progress for an iTunes import operation.
type ITunesImportStatusResponse struct {
	OperationID string   `json:"operation_id"`
	Status      string   `json:"status"`
	Progress    int      `json:"progress"`
	Message     string   `json:"message"`
	TotalBooks  int      `json:"total_books,omitempty"`
	Processed   int      `json:"processed,omitempty"`
	Imported    int      `json:"imported,omitempty"`
	Skipped     int      `json:"skipped,omitempty"`
	Failed      int      `json:"failed,omitempty"`
	Errors      []string `json:"errors,omitempty"`
}

type itunesImportStatus struct {
	mu        sync.Mutex
	Total     int
	Processed int
	Imported  int
	Skipped   int
	Failed    int
	Errors    []string
}

var itunesImportStatuses sync.Map

// handleITunesValidate validates an iTunes library without importing.
func (s *Server) handleITunesValidate(c *gin.Context) {
	var req ITunesValidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, err := os.Stat(req.LibraryPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "iTunes library file not found"})
		return
	}

	opts := itunes.ImportOptions{
		LibraryPath:    req.LibraryPath,
		ImportMode:     itunes.ImportModeImport,
		SkipDuplicates: true,
	}

	result, err := itunes.ValidateImport(opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("validation failed: %v", err),
		})
		return
	}

	duplicateCount := 0
	for _, titles := range result.DuplicateHashes {
		if len(titles) > 1 {
			duplicateCount += len(titles) - 1
		}
	}

	response := ITunesValidateResponse{
		TotalTracks:     result.TotalTracks,
		AudiobookTracks: result.AudiobookTracks,
		FilesFound:      result.FilesFound,
		FilesMissing:    result.FilesMissing,
		MissingPaths:    result.MissingPaths,
		DuplicateCount:  duplicateCount,
		EstimatedTime:   result.EstimatedTime,
	}

	c.JSON(http.StatusOK, response)
}

// handleITunesImport starts an asynchronous iTunes library import operation.
func (s *Server) handleITunesImport(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	var req ITunesImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, err := os.Stat(req.LibraryPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "iTunes library file not found"})
		return
	}

	opID := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(opID, "itunes_import", &req.LibraryPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	status := &itunesImportStatus{}
	itunesImportStatuses.Store(op.ID, status)

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return executeITunesImport(ctx, progress, op.ID, req)
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "itunes_import", operations.PriorityNormal, operationFunc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, ITunesImportResponse{
		OperationID: op.ID,
		Status:      "queued",
		Message:     "iTunes import operation queued",
	})
}

// handleITunesWriteBack updates iTunes Library.xml with new file paths.
func (s *Server) handleITunesWriteBack(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req ITunesWriteBackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, err := os.Stat(req.LibraryPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "iTunes library file not found"})
		return
	}

	updates := make([]*itunes.WriteBackUpdate, 0, len(req.AudiobookIDs))
	for _, id := range req.AudiobookIDs {
		book, err := database.GlobalStore.GetBookByID(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("failed to get audiobook %s: %v", id, err),
			})
			return
		}
		if book == nil || book.ITunesPersistentID == nil || *book.ITunesPersistentID == "" {
			continue
		}

		updates = append(updates, &itunes.WriteBackUpdate{
			ITunesPersistentID: *book.ITunesPersistentID,
			OldPath:            "",
			NewPath:            book.FilePath,
		})
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no audiobooks with iTunes persistent IDs found"})
		return
	}

	opts := itunes.WriteBackOptions{
		LibraryPath:  req.LibraryPath,
		Updates:      updates,
		CreateBackup: req.CreateBackup,
	}

	result, err := itunes.WriteBack(opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("write-back failed: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, ITunesWriteBackResponse{
		Success:      result.Success,
		UpdatedCount: result.UpdatedCount,
		BackupPath:   result.BackupPath,
		Message:      result.Message,
	})
}

// handleITunesImportStatus returns the status of an iTunes import operation.
func (s *Server) handleITunesImportStatus(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	opID := c.Param("id")
	op, err := database.GlobalStore.GetOperationByID(opID)
	if err != nil || op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}

	progress := calculatePercent(op.Progress, op.Total)
	snapshot := snapshotITunesImportStatus(op.ID)

	c.JSON(http.StatusOK, ITunesImportStatusResponse{
		OperationID: op.ID,
		Status:      op.Status,
		Progress:    progress,
		Message:     op.Message,
		TotalBooks:  snapshot.Total,
		Processed:   snapshot.Processed,
		Imported:    snapshot.Imported,
		Skipped:     snapshot.Skipped,
		Failed:      snapshot.Failed,
		Errors:      snapshot.Errors,
	})
}

func executeITunesImport(ctx context.Context, progress operations.ProgressReporter, opID string, req ITunesImportRequest) error {
	status := loadITunesImportStatus(opID)
	progressMessage := "Starting iTunes import"
	_ = progress.UpdateProgress(0, 0, progressMessage)
	_ = progress.Log("info", progressMessage, nil)

	library, err := itunes.ParseLibrary(req.LibraryPath)
	if err != nil {
		recordITunesImportError(status, fmt.Sprintf("failed to parse library: %v", err))
		return fmt.Errorf("failed to parse library: %w", err)
	}

	totalBooks := 0
	for _, track := range library.Tracks {
		if itunes.IsAudiobook(track) {
			totalBooks++
		}
	}
	setITunesImportTotal(status, totalBooks)

	_ = progress.Log("info", fmt.Sprintf("Found %d audiobooks to import", totalBooks), nil)
	if totalBooks == 0 {
		_ = progress.UpdateProgress(0, 0, "No audiobooks found")
		return nil
	}

	importMode := resolveITunesImportMode(req.ImportMode)

	processed := 0
	for _, track := range library.Tracks {
		if !itunes.IsAudiobook(track) {
			continue
		}

		if progress.IsCanceled() {
			_ = progress.Log("info", "iTunes import canceled", nil)
			return nil
		}

		processed++
		updateITunesProcessed(status, processed)

		book, err := buildBookFromTrack(track, req.LibraryPath)
		if err != nil {
			recordITunesFailure(status, err.Error())
			_ = progress.Log("error", err.Error(), nil)
			updateITunesProgress(progress, status, processed, totalBooks)
			continue
		}

		assignAuthorAndSeries(book, track)

		hash, err := scanner.ComputeFileHash(book.FilePath)
		if err != nil {
			_ = progress.Log("warn", fmt.Sprintf("Failed to hash %s: %v", book.FilePath, err), nil)
		} else if hash != "" {
			book.FileHash = stringPtr(hash)
			book.OriginalFileHash = stringPtr(hash)
			if importMode == itunes.ImportModeOrganized {
				book.OrganizedFileHash = stringPtr(hash)
			}
			if blocked, err := database.GlobalStore.IsHashBlocked(hash); err == nil && blocked {
				updateITunesSkipped(status)
				_ = progress.Log("warn", fmt.Sprintf("Skipping blocked hash for %s", book.Title), nil)
				updateITunesProgress(progress, status, processed, totalBooks)
				continue
			}
		}

		if req.SkipDuplicates {
			if existing, err := database.GlobalStore.GetBookByFilePath(book.FilePath); err == nil && existing != nil {
				updateITunesSkipped(status)
				_ = progress.Log("info", fmt.Sprintf("Skipping duplicate file path: %s", book.FilePath), nil)
				updateITunesProgress(progress, status, processed, totalBooks)
				continue
			}
			if book.FileHash != nil {
				if existing, err := database.GlobalStore.GetBookByFileHash(*book.FileHash); err == nil && existing != nil {
					updateITunesSkipped(status)
					_ = progress.Log("info", fmt.Sprintf("Skipping duplicate hash: %s", book.Title), nil)
					updateITunesProgress(progress, status, processed, totalBooks)
					continue
				}
			}
		}

		book.LibraryState = stringPtr(importLibraryState(importMode))

		created, err := database.GlobalStore.CreateBook(book)
		if err != nil {
			recordITunesFailure(status, fmt.Sprintf("Failed to save '%s': %v", book.Title, err))
			_ = progress.Log("error", fmt.Sprintf("Failed to save '%s': %v", book.Title, err), nil)
			updateITunesProgress(progress, status, processed, totalBooks)
			continue
		}

		updateITunesImported(status)

		if req.ImportPlaylists {
			tags := itunes.ExtractPlaylistTags(track.TrackID, library.Playlists)
			if len(tags) > 0 {
				_ = progress.Log("info", fmt.Sprintf("Playlist tags for '%s': %s", book.Title, strings.Join(tags, ", ")), nil)
			}
		}

		if importMode == itunes.ImportModeOrganize && !req.PreserveLocation {
			if err := organizeImportedBook(created, progress); err != nil {
				recordITunesFailure(status, fmt.Sprintf("Failed to organize '%s': %v", created.Title, err))
				_ = progress.Log("warn", fmt.Sprintf("Failed to organize '%s': %v", created.Title, err), nil)
			} else {
				created.LibraryState = stringPtr("organized")
				if _, err := database.GlobalStore.UpdateBook(created.ID, created); err != nil {
					_ = progress.Log("warn", fmt.Sprintf("Failed to update organized path for '%s': %v", created.Title, err), nil)
				}
			}
		}

		updateITunesProgress(progress, status, processed, totalBooks)
	}

	summary := buildITunesSummary(status)
	_ = progress.UpdateProgress(totalBooks, totalBooks, summary)
	_ = progress.Log("info", summary, nil)
	_ = ctx
	return nil
}

func buildBookFromTrack(track *itunes.Track, libraryPath string) (*database.Book, error) {
	if track == nil {
		return nil, fmt.Errorf("track is nil")
	}

	filePath, err := itunes.DecodeLocation(track.Location)
	if err != nil {
		return nil, fmt.Errorf("failed to decode location: %w", err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
	var duration *int
	if track.TotalTime > 0 {
		seconds := int(track.TotalTime / 1000)
		duration = &seconds
	}
	var releaseYear *int
	if track.Year > 0 {
		releaseYear = intPtr(track.Year)
	}
	var persistentID *string
	if track.PersistentID != "" {
		persistentID = stringPtr(track.PersistentID)
	}
	title := strings.TrimSpace(track.Name)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}

	book := &database.Book{
		Title:                title,
		FilePath:             filePath,
		Format:               format,
		Duration:             duration,
		OriginalFilename:     stringPtr(filepath.Base(filePath)),
		AudiobookReleaseYear: releaseYear,
		ITunesPersistentID:   persistentID,
		ITunesPlayCount:      intPtr(track.PlayCount),
		ITunesRating:         intPtr(track.Rating),
		ITunesBookmark:       int64Ptr(track.Bookmark),
		ITunesImportSource:   stringPtr(libraryPath),
	}

	if !track.DateAdded.IsZero() {
		book.ITunesDateAdded = &track.DateAdded
	}
	if track.PlayDate > 0 {
		lastPlayed := time.Unix(track.PlayDate, 0)
		book.ITunesLastPlayed = &lastPlayed
	}
	if track.AlbumArtist != "" && track.AlbumArtist != track.Artist {
		book.Narrator = stringPtr(track.AlbumArtist)
	}
	if track.Comments != "" {
		book.Edition = stringPtr(track.Comments)
	}
	if track.Size > 0 {
		size := track.Size
		book.FileSize = &size
	} else if info.Size() > 0 {
		size := info.Size()
		book.FileSize = &size
	}

	return book, nil
}

func assignAuthorAndSeries(book *database.Book, track *itunes.Track) {
	if book == nil || track == nil {
		return
	}

	if track.Artist != "" {
		authorID, err := ensureAuthorID(track.Artist)
		if err == nil {
			book.AuthorID = authorID
		}
	}

	seriesName := extractSeriesName(track.Album)
	if seriesName != "" {
		seriesID, err := ensureSeriesID(seriesName, book.AuthorID)
		if err == nil {
			book.SeriesID = seriesID
		}
	}
}

func ensureAuthorID(name string) (*int, error) {
	author, err := database.GlobalStore.GetAuthorByName(name)
	if err != nil {
		return nil, err
	}
	if author != nil {
		return &author.ID, nil
	}
	author, err = database.GlobalStore.CreateAuthor(name)
	if err != nil {
		return nil, err
	}
	return &author.ID, nil
}

func ensureSeriesID(name string, authorID *int) (*int, error) {
	series, err := database.GlobalStore.GetSeriesByName(name, authorID)
	if err != nil {
		return nil, err
	}
	if series != nil {
		return &series.ID, nil
	}
	series, err = database.GlobalStore.CreateSeries(name, authorID)
	if err != nil {
		return nil, err
	}
	return &series.ID, nil
}

func extractSeriesName(album string) string {
	if album == "" {
		return ""
	}
	parts := strings.Split(album, ",")
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0])
	}
	parts = strings.Split(album, "-")
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0])
	}
	parts = strings.Split(album, ":")
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0])
	}
	return strings.TrimSpace(album)
}

func importLibraryState(mode itunes.ImportMode) string {
	if mode == itunes.ImportModeOrganized {
		return "organized"
	}
	return "imported"
}

func organizeImportedBook(book *database.Book, progress operations.ProgressReporter) error {
	if book == nil {
		return fmt.Errorf("book is nil")
	}
	if config.AppConfig.RootDir == "" {
		return fmt.Errorf("root_dir is not configured")
	}

	org := organizer.NewOrganizer(&config.AppConfig)
	newPath, err := org.OrganizeBook(book)
	if err != nil {
		return err
	}
	if newPath != "" && newPath != book.FilePath {
		book.FilePath = newPath
		applyOrganizedFileMetadata(book, newPath)
		_ = progress.Log("info", fmt.Sprintf("Organized '%s' to %s", book.Title, newPath), nil)
	}
	return nil
}

func resolveITunesImportMode(mode string) itunes.ImportMode {
	switch mode {
	case string(itunes.ImportModeOrganized):
		return itunes.ImportModeOrganized
	case string(itunes.ImportModeOrganize):
		return itunes.ImportModeOrganize
	default:
		return itunes.ImportModeImport
	}
}

func loadITunesImportStatus(opID string) *itunesImportStatus {
	if value, ok := itunesImportStatuses.Load(opID); ok {
		if status, ok := value.(*itunesImportStatus); ok {
			return status
		}
	}
	status := &itunesImportStatus{}
	itunesImportStatuses.Store(opID, status)
	return status
}

func snapshotITunesImportStatus(opID string) itunesImportStatus {
	status := loadITunesImportStatus(opID)
	status.mu.Lock()
	defer status.mu.Unlock()

	snapshot := itunesImportStatus{
		Total:     status.Total,
		Processed: status.Processed,
		Imported:  status.Imported,
		Skipped:   status.Skipped,
		Failed:    status.Failed,
		Errors:    append([]string(nil), status.Errors...),
	}
	return snapshot
}

func setITunesImportTotal(status *itunesImportStatus, total int) {
	status.mu.Lock()
	status.Total = total
	status.mu.Unlock()
}

func updateITunesProcessed(status *itunesImportStatus, processed int) {
	status.mu.Lock()
	status.Processed = processed
	status.mu.Unlock()
}

func updateITunesImported(status *itunesImportStatus) {
	status.mu.Lock()
	status.Imported++
	status.mu.Unlock()
}

func updateITunesSkipped(status *itunesImportStatus) {
	status.mu.Lock()
	status.Skipped++
	status.mu.Unlock()
}

func recordITunesFailure(status *itunesImportStatus, message string) {
	status.mu.Lock()
	status.Failed++
	if len(status.Errors) < itunesImportErrorLimit {
		status.Errors = append(status.Errors, message)
	}
	status.mu.Unlock()
}

func recordITunesImportError(status *itunesImportStatus, message string) {
	status.mu.Lock()
	if len(status.Errors) < itunesImportErrorLimit {
		status.Errors = append(status.Errors, message)
	}
	status.mu.Unlock()
}

func updateITunesProgress(progress operations.ProgressReporter, status *itunesImportStatus, processed, total int) {
	status.mu.Lock()
	current := status.Processed
	imported := status.Imported
	skipped := status.Skipped
	failed := status.Failed
	status.mu.Unlock()

	if processed%itunesImportProgressBatch != 0 && processed != total {
		return
	}

	message := fmt.Sprintf(
		"Processed %d/%d (imported %d, skipped %d, failed %d)",
		current,
		total,
		imported,
		skipped,
		failed,
	)
	_ = progress.UpdateProgress(processed, total, message)
}

func buildITunesSummary(status *itunesImportStatus) string {
	status.mu.Lock()
	defer status.mu.Unlock()
	return fmt.Sprintf(
		"Import completed: %d imported, %d skipped, %d failed",
		status.Imported,
		status.Skipped,
		status.Failed,
	)
}

func calculatePercent(current, total int) int {
	if total <= 0 {
		return 0
	}
	percentage := (current * 100) / total
	if percentage < 0 {
		return 0
	}
	if percentage > 100 {
		return 100
	}
	return percentage
}

func intPtr(value int) *int {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}
