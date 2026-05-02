// file: internal/server/itunes_handlers.go
// version: 2.3.0
// guid: 7f2e1a4c-8b3d-4e5f-9a1b-2c3d4e5f6a7b
// last-edited: 2026-05-01

// iTunes HTTP handlers. All business logic lives in internal/itunes/service.
// Handlers that call s.itunesSvc.Importer.* guard with itunesEnabledOrError
// so they return 503 when iTunes is disabled rather than panicking.

package server

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
	itunesservice "github.com/jdfalk/audiobook-organizer/internal/itunes/service"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/oklog/ulid/v2"
)

// itunesEnabledOrError returns false and sends a 503 error when the iTunes service
// is nil or disabled. Callers should return immediately on false.
func (s *Server) itunesEnabledOrError(c *gin.Context) bool {
	if s.itunesSvc == nil || !s.itunesSvc.Enabled() {
		httputil.RespondWithServiceUnavailable(c, itunesservice.ErrITunesDisabled.Error())
		return false
	}
	return true
}

// --- request / response types ---

// ITunesValidateRequest represents a validation request for an iTunes library.
type ITunesValidateRequest struct {
	LibraryPath  string               `json:"library_path" binding:"required"`
	PathMappings []itunes.PathMapping `json:"path_mappings,omitempty"`
}

// ITunesValidateResponse summarizes validation results for an iTunes library.
type ITunesValidateResponse struct {
	TotalTracks     int      `json:"total_tracks"`
	AudiobookTracks int      `json:"audiobook_tracks"`
	AudiobookCount  int      `json:"audiobook_count"`
	FilesFound      int      `json:"files_found"`
	FilesMissing    int      `json:"files_missing"`
	MissingPaths    []string `json:"missing_paths,omitempty"`
	PathPrefixes    []string `json:"path_prefixes,omitempty"`
	DuplicateCount  int      `json:"duplicate_count"`
	EstimatedTime   string   `json:"estimated_import_time"`
}

// ITunesImportRequest represents a request to import an iTunes library.
type ITunesImportRequest struct {
	LibraryPath      string               `json:"library_path" binding:"required"`
	ImportMode       string               `json:"import_mode" binding:"required,oneof=organized import organize"`
	PreserveLocation bool                 `json:"preserve_location"`
	ImportPlaylists  bool                 `json:"import_playlists"`
	SkipDuplicates   bool                 `json:"skip_duplicates"`
	FetchMetadata    bool                 `json:"fetch_metadata"`
	PathMappings     []itunes.PathMapping `json:"path_mappings,omitempty"`
}

// ITunesImportResponse acknowledges an iTunes import operation.
type ITunesImportResponse struct {
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}

// ITunesWriteBackRequest represents a write-back request for iTunes ITL updates.
type ITunesWriteBackRequest struct {
	LibraryPath  string               `json:"library_path"`
	AudiobookIDs []string             `json:"audiobook_ids"`
	PathMappings []itunes.PathMapping `json:"path_mappings,omitempty"`
}

// ITunesWriteBackResponse reports the result of an ITL write-back.
type ITunesWriteBackResponse struct {
	Success      bool   `json:"success"`
	UpdatedCount int    `json:"updated_count"`
	Message      string `json:"message"`
}

// ITunesBookMapping is a single book-to-iTunes-path mapping used in preview.
type ITunesBookMapping struct {
	BookID             string `json:"book_id"`
	Title              string `json:"title"`
	Author             string `json:"author"`
	ITunesPersistentID string `json:"itunes_persistent_id"`
	LocalPath          string `json:"local_path"`
	ITunesPath         string `json:"itunes_path,omitempty"`
	PathDiffers        bool   `json:"path_differs,omitempty"`
}

// ITunesWriteBackPreviewRequest is the wire type for POST /itunes/write-back-preview.
type ITunesWriteBackPreviewRequest struct {
	LibraryPath string   `json:"library_path" binding:"required"`
	BookIDs     []string `json:"book_ids,omitempty"`
}

// ITunesWriteBackPreviewResponse is returned by POST /itunes/write-back-preview.
type ITunesWriteBackPreviewResponse struct {
	Items []ITunesBookMapping `json:"items"`
	Total int                 `json:"total"`
}

// ITunesImportStatusResponse is returned by GET /itunes/import/:id.
type ITunesImportStatusResponse struct {
	OperationID string   `json:"operation_id"`
	Status      string   `json:"status"`
	Progress    int      `json:"progress"`
	Message     string   `json:"message"`
	TotalBooks  int      `json:"total_books"`
	Processed   int      `json:"processed"`
	Imported    int      `json:"imported"`
	Skipped     int      `json:"skipped"`
	Failed      int      `json:"failed"`
	Errors      []string `json:"errors,omitempty"`
}

// ITunesTestMappingRequest tests a single path mapping against the library.
type ITunesTestMappingRequest struct {
	LibraryPath string `json:"library_path" binding:"required"`
	From        string `json:"from" binding:"required"`
	To          string `json:"to" binding:"required"`
}

// ITunesTestMappingResponse returns sample results from testing a mapping.
type ITunesTestMappingResponse struct {
	Tested   int                 `json:"tested"`
	Found    int                 `json:"found"`
	Examples []ITunesTestExample `json:"examples"`
}

// ITunesTestExample is a single found file example.
type ITunesTestExample struct {
	Title string `json:"title"`
	Path  string `json:"path"`
}

// ITunesSyncRequest is the wire type for POST /itunes/sync.
type ITunesSyncRequest struct {
	LibraryPath  string               `json:"library_path,omitempty"`
	PathMappings []itunes.PathMapping `json:"path_mappings,omitempty"`
	Force        bool                 `json:"force,omitempty"`
}

// ITunesSyncResponse acknowledges a sync operation.
type ITunesSyncResponse struct {
	OperationID string `json:"operation_id"`
	Message     string `json:"message"`
}

// --- handlers ---

// handleITunesValidate validates an iTunes library without importing.
func (s *Server) handleITunesValidate(c *gin.Context) {
	var req ITunesValidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	svcMappings := make([]itunesservice.PathMapping, len(req.PathMappings))
	for i, m := range req.PathMappings {
		svcMappings[i] = itunesservice.PathMapping{From: m.From, To: m.To}
	}

	resp, err := itunesservice.Validate(itunesservice.ValidateRequest{
		LibraryPath:  req.LibraryPath,
		PathMappings: svcMappings,
	})
	if err != nil {
		if errors.Is(err, itunesservice.ErrLibraryNotFound) {
			httputil.RespondWithBadRequest(c, err.Error())
		} else {
			httputil.InternalError(c, "validation failed", err)
		}
		return
	}

	httputil.RespondWithOK(c, ITunesValidateResponse{
		TotalTracks:     resp.TotalTracks,
		AudiobookTracks: resp.AudiobookTracks,
		AudiobookCount:  resp.AudiobookCount,
		FilesFound:      resp.FilesFound,
		FilesMissing:    resp.FilesMissing,
		MissingPaths:    resp.MissingPaths,
		PathPrefixes:    resp.PathPrefixes,
		DuplicateCount:  resp.DuplicateCount,
		EstimatedTime:   resp.EstimatedTime,
	})
}

// handleITunesTestMapping tests a single path mapping against a few tracks.
func (s *Server) handleITunesTestMapping(c *gin.Context) {
	var req ITunesTestMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	resp, err := itunesservice.TestMapping(itunesservice.TestMappingRequest{
		LibraryPath: req.LibraryPath,
		From:        req.From,
		To:          req.To,
	})
	if err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	examples := make([]ITunesTestExample, len(resp.Examples))
	for i, e := range resp.Examples {
		examples[i] = ITunesTestExample{Title: e.Title, Path: e.Path}
	}
	httputil.RespondWithOK(c, ITunesTestMappingResponse{
		Tested:   resp.Tested,
		Found:    resp.Found,
		Examples: examples,
	})
}

// handleITunesImport starts an asynchronous iTunes library import operation.
func (s *Server) handleITunesImport(c *gin.Context) {
	if !s.itunesEnabledOrError(c) {
		return
	}
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	if s.queue == nil {
		httputil.RespondWithInternalError(c, "operation queue not initialized")
		return
	}

	var req ITunesImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	if _, err := os.Stat(req.LibraryPath); os.IsNotExist(err) {
		httputil.RespondWithBadRequest(c, "iTunes library file not found")
		return
	}

	opID := ulid.Make().String()
	op, err := s.Store().CreateOperation(opID, "itunes_import", &req.LibraryPath)
	if err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	svcMappings := make([]itunesservice.PathMapping, len(req.PathMappings))
	for i, m := range req.PathMappings {
		svcMappings[i] = itunesservice.PathMapping{From: m.From, To: m.To}
	}
	svcReq := itunesservice.ImportRequest{
		LibraryPath:      req.LibraryPath,
		ImportMode:       req.ImportMode,
		PreserveLocation: req.PreserveLocation,
		ImportPlaylists:  req.ImportPlaylists,
		SkipDuplicates:   req.SkipDuplicates,
		FetchMetadata:    req.FetchMetadata,
		PathMappings:     svcMappings,
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.itunesSvc.Importer.Execute(ctx, op.ID, svcReq, operations.LoggerFromReporter(progress))
	}

	if err := s.queue.Enqueue(op.ID, "itunes_import", operations.PriorityNormal, operationFunc); err != nil {
		httputil.InternalError(c, "failed to enqueue operation", err)
		return
	}

	httputil.RespondWithSuccess(c, http.StatusAccepted, ITunesImportResponse{
		OperationID: op.ID,
		Status:      "queued",
		Message:     "iTunes import operation queued",
	})
}

// handleITunesWriteBack updates the iTunes ITL binary with new file paths.
func (s *Server) handleITunesWriteBack(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	var req ITunesWriteBackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	if !config.AppConfig.ITLWriteBackEnabled || config.AppConfig.ITunesLibraryWritePath == "" {
		httputil.RespondWithBadRequest(c, "ITL write-back is not enabled in config")
		return
	}

	pathMappings := req.PathMappings
	if len(pathMappings) == 0 {
		for _, m := range config.AppConfig.ITunesPathMappings {
			pathMappings = append(pathMappings, itunes.PathMapping{From: m.From, To: m.To})
		}
	}

	var itlUpdates []itunes.ITLLocationUpdate
	for _, id := range req.AudiobookIDs {
		book, err := s.Store().GetBookByID(id)
		if err != nil {
			httputil.RespondWithInternalError(c, fmt.Sprintf("failed to get audiobook %s: %v", id, err))
			return
		}
		if book == nil || book.ITunesPersistentID == nil || *book.ITunesPersistentID == "" {
			continue
		}

		itunesPath := itunes.ReverseRemapPath(book.FilePath, pathMappings)
		itlUpdates = append(itlUpdates, itunes.ITLLocationUpdate{
			PersistentID: *book.ITunesPersistentID,
			NewLocation:  itunesPath,
		})
	}

	if len(itlUpdates) == 0 {
		httputil.RespondWithBadRequest(c, "no audiobooks with iTunes persistent IDs found")
		return
	}

	itlPath := config.AppConfig.ITunesLibraryWritePath
	itlResult, itlErr := itunes.UpdateITLLocations(itlPath, itlPath+".tmp", itlUpdates)
	if itlErr != nil {
		stdlog.Printf("[WARN] ITL write-back failed: %v", itlErr)
		httputil.RespondWithInternalError(c, fmt.Sprintf("ITL write-back failed: %v", itlErr))
		return
	}

	if renameErr := itunes.RenameITLFile(itlPath+".tmp", itlPath); renameErr != nil {
		stdlog.Printf("[WARN] ITL rename failed: %v", renameErr)
		httputil.RespondWithInternalError(c, fmt.Sprintf("ITL rename failed: %v", renameErr))
		return
	}

	stdlog.Printf("[INFO] ITL write-back: updated %d tracks", itlResult.UpdatedCount)
	httputil.RespondWithOK(c, ITunesWriteBackResponse{
		Success:      true,
		UpdatedCount: itlResult.UpdatedCount,
		Message:      fmt.Sprintf("Successfully updated %d audiobook locations in ITL", itlResult.UpdatedCount),
	})
}

// handleITunesWriteBackAll writes ALL books with iTunes persistent IDs back to the ITL.
func (s *Server) handleITunesWriteBackAll(c *gin.Context) {
	if !s.itunesEnabledOrError(c) {
		return
	}
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	if !config.AppConfig.ITLWriteBackEnabled {
		httputil.RespondWithBadRequest(c, "ITL write-back is not enabled in config")
		return
	}

	itlPath := config.AppConfig.ITunesLibraryWritePath
	if itlPath == "" {
		httputil.RespondWithBadRequest(c, "no ITL library path configured")
		return
	}

	if _, err := os.Stat(itlPath); os.IsNotExist(err) {
		httputil.RespondWithBadRequest(c, "ITL file not found at configured path")
		return
	}

	if err := itunesservice.CheckITLConflict(itlPath); err != nil {
		httputil.RespondWithConflict(c, err.Error())
		return
	}

	itlUpdates, writtenBookIDs := s.itunesSvc.Importer.CollectITLUpdatesWithBookIDs()

	if len(itlUpdates) == 0 {
		httputil.RespondWithOK(c, gin.H{
			"success":       true,
			"updated_count": 0,
			"message":       "no books with iTunes persistent IDs found",
		})
		return
	}

	itlResult, itlErr := itunes.UpdateITLLocations(itlPath, itlPath+".tmp", itlUpdates)
	if itlErr != nil {
		httputil.RespondWithInternalError(c, fmt.Sprintf("ITL write-back failed: %v", itlErr))
		return
	}

	if renameErr := itunes.RenameITLFile(itlPath+".tmp", itlPath); renameErr != nil {
		httputil.RespondWithInternalError(c, fmt.Sprintf("ITL rename failed: %v", renameErr))
		return
	}

	itunesservice.RecordITLReadTime()
	stdlog.Printf("[INFO] Bulk ITL write-back: updated %d tracks out of %d candidates", itlResult.UpdatedCount, len(itlUpdates))

	if n, markErr := s.Store().MarkITunesSynced(writtenBookIDs); markErr == nil && n > 0 {
		stdlog.Printf("[INFO] Marked %d books as iTunes-synced after write-back", n)
	}

	httputil.RespondWithOK(c, gin.H{
		"success":            true,
		"updated_count":      itlResult.UpdatedCount,
		"file_pid_pairs":     len(itlUpdates),
		"primary_book_count": len(writtenBookIDs),
		"message":            fmt.Sprintf("ITL write-back complete: %d ITL chunks updated across %d (file,PID) pairs from %d primary books", itlResult.UpdatedCount, len(itlUpdates), len(writtenBookIDs)),
	})
}

// handleITunesCleanupOrphans removes iTunes tracks that should not be in the
// library: tracks owned by non-primary book versions, tracks owned by
// soft-deleted books, and tracks present in the ITL but with no matching
// row in the DB ("true orphans" left behind by past hard-deletes that did
// not fire an iTunes remove hook). Each candidate PID is enqueued via the
// WriteBackBatcher; the actual ITL mutation happens on the next flush.
func (s *Server) handleITunesCleanupOrphans(c *gin.Context) {
	if !s.itunesEnabledOrError(c) {
		return
	}
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	if s.writeBackBatcher == nil {
		httputil.RespondWithBadRequest(c, "iTunes write-back batcher not configured")
		return
	}
	itlPath := config.AppConfig.ITunesLibraryWritePath
	if itlPath == "" {
		httputil.RespondWithBadRequest(c, "no ITL library path configured")
		return
	}

	// Build the set of "valid" PIDs from the DB: every primary,
	// non-soft-deleted book contributes its book_files PIDs.
	allBooks, err := s.Store().GetAllBooks(1_000_000, 0)
	if err != nil {
		httputil.InternalError(c, "failed to enumerate books", err)
		return
	}
	validPIDs := make(map[string]bool)
	type fileRef struct {
		bookID string
		fileID string
		pid    string
	}
	var nonPrimaryFiles []fileRef
	for i := range allBooks {
		b := &allBooks[i]
		files, _ := s.Store().GetBookFiles(b.ID)
		isPrimary := b.IsPrimaryVersion == nil || *b.IsPrimaryVersion
		isSoftDeleted := b.MarkedForDeletion != nil && *b.MarkedForDeletion
		if isPrimary && !isSoftDeleted {
			for _, f := range files {
				if f.ITunesPersistentID != "" {
					validPIDs[strings.ToLower(f.ITunesPersistentID)] = true
				}
			}
		} else {
			// Non-primary OR soft-deleted: collect for removal.
			for _, f := range files {
				if f.ITunesPersistentID != "" {
					nonPrimaryFiles = append(nonPrimaryFiles, fileRef{bookID: b.ID, fileID: f.ID, pid: f.ITunesPersistentID})
				}
			}
		}
	}

	// Soft-deleted books are excluded from GetAllBooks above (filter is
	// COALESCE(marked_for_deletion,0)=0). Pull them separately.
	softDeleted, _ := s.Store().ListSoftDeletedBooks(1_000_000, 0, nil)
	var softDeletedFiles []fileRef
	for i := range softDeleted {
		b := &softDeleted[i]
		files, _ := s.Store().GetBookFiles(b.ID)
		for _, f := range files {
			if f.ITunesPersistentID != "" {
				softDeletedFiles = append(softDeletedFiles, fileRef{bookID: b.ID, fileID: f.ID, pid: f.ITunesPersistentID})
			}
		}
	}

	// Walk the ITL master list and find any PID NOT in validPIDs —
	// those are true orphans (DB row was hard-deleted in the past
	// without firing the EnqueueRemove hook).
	var truePIDOrphans []string
	if library, parseErr := itunes.ParseLibrary(itlPath); parseErr == nil {
		for _, track := range library.Tracks {
			if track.PersistentID == "" {
				continue
			}
			if !validPIDs[strings.ToLower(track.PersistentID)] {
				truePIDOrphans = append(truePIDOrphans, track.PersistentID)
			}
		}
	}

	enqueued := 0
	cleared := 0
	clearFile := func(fr fileRef) {
		full, err := s.Store().GetBookFileByID(fr.bookID, fr.fileID)
		if err != nil || full == nil {
			return
		}
		if full.ITunesPersistentID == "" && full.ITunesPath == "" {
			return
		}
		full.ITunesPersistentID = ""
		full.ITunesPath = ""
		if err := s.Store().UpdateBookFile(fr.fileID, full); err == nil {
			cleared++
		}
	}
	for _, fr := range nonPrimaryFiles {
		s.writeBackBatcher.EnqueueRemove(fr.pid)
		enqueued++
		clearFile(fr)
	}
	for _, fr := range softDeletedFiles {
		s.writeBackBatcher.EnqueueRemove(fr.pid)
		enqueued++
		clearFile(fr)
	}
	for _, pid := range truePIDOrphans {
		s.writeBackBatcher.EnqueueRemove(pid)
		enqueued++
		// True orphans by definition have no DB row to clear.
	}

	stdlog.Printf("[INFO] iTunes cleanup-orphans: enqueued %d removes (non_primary=%d soft_deleted=%d true_orphans=%d), cleared %d DB PIDs",
		enqueued, len(nonPrimaryFiles), len(softDeletedFiles), len(truePIDOrphans), cleared)

	httputil.RespondWithOK(c, gin.H{
		"success":         true,
		"enqueued":        enqueued,
		"non_primary":     len(nonPrimaryFiles),
		"soft_deleted":    len(softDeletedFiles),
		"true_orphans":    len(truePIDOrphans),
		"db_pids_cleared": cleared,
		"valid_pid_count": len(validPIDs),
		"message":         fmt.Sprintf("enqueued %d iTunes removes (non_primary=%d soft_deleted=%d true_orphans=%d), cleared %d stale DB PIDs. The next batcher flush will splice removed tracks out of the ITL.", enqueued, len(nonPrimaryFiles), len(softDeletedFiles), len(truePIDOrphans), cleared),
	})
}

// handleITunesWriteBackPreview returns a comparison of local paths vs iTunes paths.
func (s *Server) handleITunesWriteBackPreview(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	var req ITunesWriteBackPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	if _, err := os.Stat(req.LibraryPath); os.IsNotExist(err) {
		httputil.RespondWithBadRequest(c, "iTunes library file not found")
		return
	}

	library, err := itunes.ParseLibrary(req.LibraryPath)
	if err != nil {
		httputil.InternalError(c, "failed to parse iTunes library", err)
		return
	}

	itunesLocations := make(map[string]string)
	for _, track := range library.Tracks {
		if track.PersistentID != "" {
			decoded, decErr := itunes.DecodeLocation(track.Location)
			if decErr == nil {
				itunesLocations[track.PersistentID] = decoded
			} else {
				itunesLocations[track.PersistentID] = track.Location
			}
		}
	}

	var books []database.Book
	if len(req.BookIDs) > 0 {
		for _, id := range req.BookIDs {
			book, bErr := s.Store().GetBookByID(id)
			if bErr != nil || book == nil {
				continue
			}
			if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" {
				books = append(books, *book)
			}
		}
	} else {
		allBooks, bErr := s.Store().GetAllBooks(0, 0)
		if bErr != nil {
			httputil.InternalError(c, "failed to list books", bErr)
			return
		}
		for _, book := range allBooks {
			if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" {
				books = append(books, book)
			}
		}
	}

	var previewMappings []itunes.PathMapping
	for _, m := range config.AppConfig.ITunesPathMappings {
		previewMappings = append(previewMappings, itunes.PathMapping{From: m.From, To: m.To})
	}

	items := make([]ITunesBookMapping, 0, len(books))
	for _, book := range books {
		persistentID := *book.ITunesPersistentID
		itunesPath := itunesLocations[persistentID]
		author := ""
		if book.AuthorID != nil {
			if a, aErr := s.Store().GetAuthorByID(*book.AuthorID); aErr == nil && a != nil {
				author = a.Name
			}
		}
		reverseMapped := itunes.ReverseRemapPath(book.FilePath, previewMappings)
		items = append(items, ITunesBookMapping{
			BookID:             book.ID,
			Title:              book.Title,
			Author:             author,
			ITunesPersistentID: persistentID,
			LocalPath:          book.FilePath,
			ITunesPath:         itunesPath,
			PathDiffers:        reverseMapped != itunesPath,
		})
	}

	httputil.RespondWithOK(c, ITunesWriteBackPreviewResponse{
		Items: items,
		Total: len(items),
	})
}

// handleListITunesBooks returns paginated books that have iTunes persistent IDs.
func (s *Server) handleListITunesBooks(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	p := httputil.ParsePaginationParams(c)
	search := p.Search
	limit, offset := p.Limit, p.Offset

	var allBooks []database.Book
	var err error
	if search != "" {
		allBooks, err = s.Store().SearchBooks(search, 0, 0)
	} else {
		allBooks, err = s.Store().GetAllBooks(0, 0)
	}
	if err != nil {
		httputil.InternalError(c, "failed to list books", err)
		return
	}

	var filtered []database.Book
	for _, book := range allBooks {
		if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" {
			filtered = append(filtered, book)
		}
	}

	total := len(filtered)

	if offset >= len(filtered) {
		filtered = nil
	} else {
		end := offset + limit
		if end > len(filtered) {
			end = len(filtered)
		}
		filtered = filtered[offset:end]
	}

	items := make([]ITunesBookMapping, 0, len(filtered))
	for _, book := range filtered {
		author := ""
		if book.AuthorID != nil {
			if a, aErr := s.Store().GetAuthorByID(*book.AuthorID); aErr == nil && a != nil {
				author = a.Name
			}
		}
		items = append(items, ITunesBookMapping{
			BookID:             book.ID,
			Title:              book.Title,
			Author:             author,
			ITunesPersistentID: *book.ITunesPersistentID,
			LocalPath:          book.FilePath,
		})
	}

	httputil.RespondWithOK(c, gin.H{
		"items": items,
		"count": total,
	})
}

// handleITunesImportStatus returns the status of an iTunes import operation.
func (s *Server) handleITunesImportStatus(c *gin.Context) {
	if !s.itunesEnabledOrError(c) {
		return
	}
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	opID := c.Param("id")
	op, err := s.Store().GetOperationByID(opID)
	if err != nil || op == nil {
		httputil.RespondWithNotFound(c, "operation", opID)
		return
	}

	progress := calculatePercent(op.Progress, op.Total)
	snapshot := s.itunesSvc.Importer.GetStatus(op.ID)

	httputil.RespondWithOK(c, ITunesImportStatusResponse{
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

func (s *Server) handleITunesImportStatusBulk(c *gin.Context) {
	if !s.itunesEnabledOrError(c) {
		return
	}
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	var req struct {
		IDs []string `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	snapshots := s.itunesSvc.Importer.GetStatusBulk(req.IDs)

	results := make(map[string]ITunesImportStatusResponse, len(req.IDs))
	for _, opID := range req.IDs {
		op, err := s.Store().GetOperationByID(opID)
		if err != nil || op == nil {
			continue
		}
		progress := calculatePercent(op.Progress, op.Total)
		snapshot := snapshots[opID]
		if snapshot == nil {
			snapshot = &itunesservice.ImportStatusSnapshot{}
		}
		results[opID] = ITunesImportStatusResponse{
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
		}
	}

	httputil.RespondWithOK(c, gin.H{"statuses": results})
}

// handleITunesLibraryStatus returns the current status of an iTunes library file.
func (s *Server) handleITunesLibraryStatus(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		httputil.RespondWithBadRequest(c, "path query parameter required")
		return
	}

	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}

	rec, err := s.Store().GetLibraryFingerprint(path)
	if err != nil {
		httputil.InternalError(c, "failed to get library fingerprint", err)
		return
	}

	stat, statErr := os.Stat(path)
	fileExists := statErr == nil

	if rec == nil {
		httputil.RespondWithOK(c, gin.H{
			"path":        path,
			"exists":      fileExists,
			"last_synced": nil,
			"changed":     fileExists,
		})
		return
	}

	changed := false
	if fileExists {
		changed = stat.Size() != rec.Size || !stat.ModTime().Equal(rec.ModTime)
	}

	httputil.RespondWithOK(c, gin.H{
		"path":        path,
		"exists":      fileExists,
		"last_synced": rec.ModTime,
		"size":        rec.Size,
		"changed":     changed,
	})
}

// handleITunesSync triggers an incremental sync from iTunes Library.xml.
func (s *Server) handleITunesSync(c *gin.Context) {
	if !s.itunesEnabledOrError(c) {
		return
	}
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	if s.queue == nil {
		httputil.RespondWithInternalError(c, "operation queue not initialized")
		return
	}

	var req ITunesSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = ITunesSyncRequest{}
	}

	libraryPath := req.LibraryPath
	if libraryPath == "" {
		libraryPath = config.AppConfig.ITunesLibraryReadPath
	}
	if libraryPath == "" {
		libraryPath = s.itunesSvc.Importer.DiscoverLibraryPath()
	}
	if libraryPath == "" {
		httputil.RespondWithBadRequest(c, "no iTunes library path configured or provided")
		return
	}

	if _, err := os.Stat(libraryPath); os.IsNotExist(err) {
		httputil.RespondWithBadRequest(c, "iTunes library file not found")
		return
	}

	if !req.Force {
		if rec, err := s.Store().GetLibraryFingerprint(libraryPath); err == nil && rec != nil {
			if info, statErr := os.Stat(libraryPath); statErr == nil {
				if info.Size() == rec.Size && info.ModTime().Equal(rec.ModTime) {
					httputil.RespondWithOK(c, gin.H{"message": "no changes detected — use force:true to sync anyway", "operation_id": ""})
					return
				}
			}
		}
	}

	opID := ulid.Make().String()
	op, err := s.Store().CreateOperation(opID, "itunes_sync", &libraryPath)
	if err != nil {
		httputil.InternalError(c, "failed to create operation", err)
		return
	}

	pathMappings := req.PathMappings
	if len(pathMappings) == 0 {
		for _, m := range config.AppConfig.ITunesPathMappings {
			pathMappings = append(pathMappings, itunes.PathMapping{From: m.From, To: m.To})
		}
	}

	lp := libraryPath
	pm := pathMappings
	actFn := s.itunesActivityFn
	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.itunesSvc.Importer.Sync(ctx, lp, pm, actFn, operations.LoggerFromReporter(progress))
	}

	if err := s.queue.Enqueue(op.ID, "itunes_sync", operations.PriorityNormal, operationFunc); err != nil {
		httputil.InternalError(c, "failed to enqueue operation", err)
		return
	}

	httputil.RespondWithSuccess(c, http.StatusAccepted, ITunesSyncResponse{
		OperationID: op.ID,
		Message:     "iTunes sync operation queued",
	})
}

// --- small helpers ---

func calculatePercent(current, total int) int {
	if total <= 0 {
		return 0
	}
	pct := (current * 100) / total
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

// handleITunesLibraryStats reads the configured ITL file and reports
// low-level structural counts useful for verifying orphan-cleanup
// progress: master-track count and dangling playlist→track refs
// (mtph items pointing at TrackIDs not present in the master list).
//
// Cheap to call: parses the binary directly with ITL helpers, no
// full library-object materialization.
func (s *Server) handleITunesLibraryStats(c *gin.Context) {
	if !s.itunesEnabledOrError(c) {
		return
	}
	itlPath := config.AppConfig.ITunesLibraryWritePath
	if itlPath == "" {
		httputil.RespondWithBadRequest(c, "no ITL library path configured")
		return
	}
	if _, err := os.Stat(itlPath); err != nil {
		httputil.RespondWithBadRequest(c, fmt.Sprintf("ITL not accessible: %v", err))
		return
	}

	data, err := os.ReadFile(itlPath)
	if err != nil {
		httputil.InternalError(c, "read ITL", err)
		return
	}
	dec, decErr := itunes.DecryptAndInflateITL(data)
	if decErr != nil {
		httputil.InternalError(c, "decrypt/inflate ITL", decErr)
		return
	}

	masterTIDs := itunes.CollectMasterTrackIDsLE(dec)
	dangling := itunes.FindDanglingMtphRefsLE(dec, masterTIDs)

	httputil.RespondWithOK(c, gin.H{
		"success":        true,
		"itl_path":       itlPath,
		"itl_size_bytes": len(data),
		"master_tracks":  len(masterTIDs),
		"dangling_mtph":  len(dangling),
		"itl_size_mb":    fmt.Sprintf("%.2f", float64(len(data))/(1024*1024)),
	})
}
