// file: internal/server/itunes_handlers.go
// version: 2.8.0
// guid: 7f2e1a4c-8b3d-4e5f-9a1b-2c3d4e5f6a7b
// last-edited: 2026-05-10

// iTunes HTTP handlers. All business logic lives in internal/itunes/service.
// Handlers that call s.itunesSvc.Importer.* guard with itunesEnabledOrError
// so they return 503 when iTunes is disabled rather than panicking.

package server

import (
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
//
// Four path columns surface the full picture so users can see exactly what
// is currently in iTunes vs what AO has on disk vs what AO would write back:
//
//   - ITunesPath               — what iTunes currently has, e.g. W:/foo/bar.m4b
//   - ITunesPathTranslated     — local equivalent of ITunesPath after applying
//                                forward path mappings (so users can stat it)
//   - AOPath                   — where AO has the file on disk (book.FilePath)
//   - AOITunesTranslatedPath   — what AO will write into the iTunes ITL when
//                                write-back runs (ReverseRemapPath of AOPath)
//
// PathDiffers is true iff AOITunesTranslatedPath != ITunesPath — i.e. the
// thing AO wants to write does not match what iTunes already has.
//
// Backwards compatibility: LocalPath is preserved as an alias of AOPath so
// older clients keep working through the migration. Remove once no caller
// reads it.
type ITunesBookMapping struct {
	BookID                 string `json:"book_id"`
	Title                  string `json:"title"`
	Author                 string `json:"author"`
	ITunesPersistentID     string `json:"itunes_persistent_id"`
	ITunesPath             string `json:"itunes_path,omitempty"`
	ITunesPathTranslated   string `json:"itunes_path_translated,omitempty"`
	AOPath                 string `json:"ao_path"`
	AOITunesTranslatedPath string `json:"ao_itunes_translated_path,omitempty"`
	PathDiffers            bool   `json:"path_differs,omitempty"`

	// LocalPath duplicates AOPath for backwards compatibility with the
	// previous response shape. Will be removed once no caller reads it.
	LocalPath string `json:"local_path"`
}

// ITunesWriteBackPreviewRequest is the wire type for POST /itunes/write-back-preview.
//
// LibraryPath is now optional — when empty, the handler uses the configured
// ITunesLibraryReadPath. The dialog used to require the user to type this
// path on every preview, which was confusing because the actual write-back
// always targets the configured ITunesLibraryWritePath (.itl) regardless.
type ITunesWriteBackPreviewRequest struct {
	LibraryPath string   `json:"library_path,omitempty"`
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
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
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

	params := itunesImportOpParams{LegacyOpID: op.ID, Request: svcReq}
	if _, enqErr := s.opRegistry.EnqueueOp(c.Request.Context(), "itunes.import", params); enqErr != nil {
		httputil.InternalError(c, "failed to enqueue operation", enqErr)
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

	// Fall back to the configured read path when the request omits one.
	// The dialog no longer requires users to type the .xml path — they
	// configure it once in Settings and the preview endpoint uses it
	// directly. The actual write-back always targets the configured
	// ITunesLibraryWritePath (.itl) regardless of this read path.
	libraryPath := strings.TrimSpace(req.LibraryPath)
	if libraryPath == "" {
		libraryPath = config.AppConfig.ITunesLibraryReadPath
	}
	if libraryPath == "" {
		httputil.RespondWithBadRequest(c, "no iTunes library path configured (set ITunesLibraryReadPath in settings)")
		return
	}

	if _, err := os.Stat(libraryPath); os.IsNotExist(err) {
		httputil.RespondWithBadRequest(c, "iTunes library file not found")
		return
	}

	library, err := itunes.ParseLibrary(libraryPath)
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
	// Forward-mapper for translating an iTunes location into its local
	// equivalent. Wraps the existing ImportOptions.RemapPath because that
	// is the canonical forward direction; the receiver pattern is
	// historical and not worth refactoring here.
	forwardOpts := itunes.ImportOptions{PathMappings: previewMappings}

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
		aoITunesTranslated := itunes.ReverseRemapPath(book.FilePath, previewMappings)
		itunesTranslated := ""
		if itunesPath != "" {
			itunesTranslated = forwardOpts.RemapPath(itunesPath)
		}
		items = append(items, ITunesBookMapping{
			BookID:                 book.ID,
			Title:                  book.Title,
			Author:                 author,
			ITunesPersistentID:     persistentID,
			ITunesPath:             itunesPath,
			ITunesPathTranslated:   itunesTranslated,
			AOPath:                 book.FilePath,
			AOITunesTranslatedPath: aoITunesTranslated,
			PathDiffers:            aoITunesTranslated != itunesPath,
			LocalPath:              book.FilePath,
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
	if s.opRegistry == nil {
		httputil.RespondWithInternalError(c, "operation registry not initialized")
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

	syncParams := itunesSyncOpParams{LegacyOpID: op.ID, LibraryPath: libraryPath, PathMappings: pathMappings}
	if _, enqErr := s.opRegistry.EnqueueOp(c.Request.Context(), "itunes.sync", syncParams); enqErr != nil {
		httputil.InternalError(c, "failed to enqueue operation", enqErr)
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
