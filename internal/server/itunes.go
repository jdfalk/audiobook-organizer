// file: internal/server/itunes.go
// version: 2.21.0
// guid: 719912e9-7b5f-48e1-afa6-1b0b7f57c2fa

package server

import (
	"context"
	"fmt"
	stdlog "log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/oklog/ulid/v2"
)

const (
	itunesImportProgressBatch = 100
	itunesImportErrorLimit    = 50
)

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
	LibraryPath        string               `json:"library_path" binding:"required"`
	ImportMode         string               `json:"import_mode" binding:"required,oneof=organized import organize"`
	PreserveLocation   bool                 `json:"preserve_location"`
	ImportPlaylists    bool                 `json:"import_playlists"`
	SkipDuplicates     bool                 `json:"skip_duplicates"`
	FetchMetadata      bool                 `json:"fetch_metadata"`
	PathMappings       []itunes.PathMapping `json:"path_mappings,omitempty"`
}

// ITunesImportResponse acknowledges an iTunes import operation.
type ITunesImportResponse struct {
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}

// ITunesWriteBackRequest represents a write-back request for iTunes ITL updates.
// LibraryPath is kept for backward compatibility but is no longer used (XML write-back removed).
type ITunesWriteBackRequest struct {
	LibraryPath  string               `json:"library_path"`
	AudiobookIDs []string             `json:"audiobook_ids"`
	PathMappings []itunes.PathMapping `json:"path_mappings,omitempty"` // Used to reverse-map local paths to iTunes paths
}

// ITunesWriteBackResponse summarizes write-back results.
type ITunesWriteBackResponse struct {
	Success      bool   `json:"success"`
	UpdatedCount int    `json:"updated_count"`
	Message      string `json:"message"`
}

// ITunesBookMapping represents a book with its iTunes path comparison.
type ITunesBookMapping struct {
	BookID             string `json:"book_id"`
	Title              string `json:"title"`
	Author             string `json:"author"`
	ITunesPersistentID string `json:"itunes_persistent_id"`
	LocalPath          string `json:"local_path"`
	ITunesPath         string `json:"itunes_path"`
	PathDiffers        bool   `json:"path_differs"`
}

// ITunesWriteBackPreviewRequest is the request body for the preview endpoint.
type ITunesWriteBackPreviewRequest struct {
	LibraryPath string   `json:"library_path" binding:"required"`
	BookIDs     []string `json:"book_ids,omitempty"`
}

// ITunesWriteBackPreviewResponse contains the preview results.
type ITunesWriteBackPreviewResponse struct {
	Items []ITunesBookMapping `json:"items"`
	Total int                 `json:"total"`
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
	Linked    int // existing books that had iTunes metadata or VG linked
	Failed    int
	Errors    []string
}

// itunesActivityRecorder is a package-level hook for dual-writing iTunes sync
// changes to the unified activity log. Set by server.go.
var itunesActivityRecorder func(entry database.ActivityEntry)

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

	stdlog.Printf("iTunes validate: library=%s, mappings=%d", req.LibraryPath, len(req.PathMappings))

	opts := itunes.ImportOptions{
		LibraryPath:    req.LibraryPath,
		ImportMode:     itunes.ImportModeImport,
		SkipDuplicates: false, // Don't hash files during validation - just check existence
		PathMappings:   req.PathMappings,
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

	// Limit missing paths to first 100 to avoid huge responses
	missingPaths := result.MissingPaths
	if len(missingPaths) > 100 {
		missingPaths = missingPaths[:100]
	}

	stdlog.Printf("iTunes validate complete: %d audiobooks, %d found, %d missing, prefixes=%v",
		result.AudiobookTracks, result.FilesFound, result.FilesMissing, result.PathPrefixes)

	response := ITunesValidateResponse{
		TotalTracks:     result.TotalTracks,
		AudiobookTracks: result.AudiobookTracks,
		AudiobookCount:  result.AudiobookCount,
		FilesFound:      result.FilesFound,
		FilesMissing:    result.FilesMissing,
		MissingPaths:    missingPaths,
		PathPrefixes:    result.PathPrefixes,
		DuplicateCount:  duplicateCount,
		EstimatedTime:   result.EstimatedTime,
	}

	c.JSON(http.StatusOK, response)
}

// ITunesTestMappingRequest tests a single path mapping against the library.
type ITunesTestMappingRequest struct {
	LibraryPath string `json:"library_path" binding:"required"`
	From        string `json:"from" binding:"required"`
	To          string `json:"to" binding:"required"`
}

// ITunesTestMappingResponse returns sample results from testing a mapping.
type ITunesTestMappingResponse struct {
	Tested int                    `json:"tested"`
	Found  int                    `json:"found"`
	Examples []ITunesTestExample  `json:"examples"`
}

// ITunesTestExample is a single found file example.
type ITunesTestExample struct {
	Title string `json:"title"`
	Path  string `json:"path"`
}

// handleITunesTestMapping tests a single path mapping against a few tracks.
func (s *Server) handleITunesTestMapping(c *gin.Context) {
	var req ITunesTestMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	library, err := itunes.ParseLibrary(req.LibraryPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to parse library: %v", err)})
		return
	}

	stdlog.Printf("iTunes test-mapping: from=%q to=%q", req.From, req.To)
	mapping := itunes.PathMapping{From: req.From, To: req.To}
	opts := itunes.ImportOptions{PathMappings: []itunes.PathMapping{mapping}}

	response := ITunesTestMappingResponse{Examples: []ITunesTestExample{}}
	for _, track := range library.Tracks {
		if !itunes.IsAudiobook(track) {
			continue
		}
		// Only test tracks that match this prefix
		if !strings.HasPrefix(track.Location, req.From) {
			continue
		}
		if response.Tested >= 20 {
			break
		}
		response.Tested++

		location := opts.RemapPath(track.Location)
		path, err := itunes.DecodeLocation(location)
		if err != nil {
			stdlog.Printf("  [%d/20] decode error for %q: %v", response.Tested, track.Name, err)
			continue
		}
		if _, err := os.Stat(path); err == nil {
			response.Found++
			stdlog.Printf("  [%d/20] FOUND: %q → %s", response.Tested, track.Name, path)
			if len(response.Examples) < 3 {
				response.Examples = append(response.Examples, ITunesTestExample{
					Title: track.Name,
					Path:  path,
				})
			}
		} else {
			stdlog.Printf("  [%d/20] MISSING: %q → %s", response.Tested, track.Name, path)
		}
	}

	stdlog.Printf("iTunes test-mapping: tested=%d found=%d examples=%d", response.Tested, response.Found, len(response.Examples))
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
		internalError(c, "failed to create operation", err)
		return
	}

	status := &itunesImportStatus{}
	itunesImportStatuses.Store(op.ID, status)

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return executeITunesImport(ctx, operations.LoggerFromReporter(progress), op.ID, req)
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "itunes_import", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, ITunesImportResponse{
		OperationID: op.ID,
		Status:      "queued",
		Message:     "iTunes import operation queued",
	})
}

// handleITunesWriteBack updates the iTunes ITL binary with new file paths.
// XML write-back has been removed; only ITL write-back is supported.
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

	if !config.AppConfig.ITLWriteBackEnabled || config.AppConfig.ITunesLibraryWritePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ITL write-back is not enabled in config"})
		return
	}

	// Build path mappings for reverse-mapping local paths to iTunes paths.
	// Prefer request-supplied mappings, then config-stored mappings.
	pathMappings := req.PathMappings
	if len(pathMappings) == 0 {
		for _, m := range config.AppConfig.ITunesPathMappings {
			pathMappings = append(pathMappings, itunes.PathMapping{From: m.From, To: m.To})
		}
	}

	var itlUpdates []itunes.ITLLocationUpdate
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

		itunesPath := itunes.ReverseRemapPath(book.FilePath, pathMappings)
		itlUpdates = append(itlUpdates, itunes.ITLLocationUpdate{
			PersistentID: *book.ITunesPersistentID,
			NewLocation:  itunesPath,
		})
	}

	if len(itlUpdates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no audiobooks with iTunes persistent IDs found"})
		return
	}

	itlPath := config.AppConfig.ITunesLibraryWritePath
	itlResult, itlErr := itunes.UpdateITLLocations(itlPath, itlPath+".tmp", itlUpdates)
	if itlErr != nil {
		stdlog.Printf("[WARN] ITL write-back failed: %v", itlErr)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("ITL write-back failed: %v", itlErr),
		})
		return
	}

	// Atomic replace
	if renameErr := itunes.RenameITLFile(itlPath+".tmp", itlPath); renameErr != nil {
		stdlog.Printf("[WARN] ITL rename failed: %v", renameErr)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("ITL rename failed: %v", renameErr),
		})
		return
	}

	stdlog.Printf("[INFO] ITL write-back: updated %d tracks", itlResult.UpdatedCount)
	c.JSON(http.StatusOK, ITunesWriteBackResponse{
		Success:      true,
		UpdatedCount: itlResult.UpdatedCount,
		Message:      fmt.Sprintf("Successfully updated %d audiobook locations in ITL", itlResult.UpdatedCount),
	})
}

// handleITunesWriteBackAll writes ALL books with iTunes persistent IDs back to
// the ITL binary file in a single bulk operation. This is useful when the ITL
// file needs a full refresh (e.g., after restoring from backup or initial setup).
// collectITLUpdates iterates all books and builds ITL location updates
// from book_files (per-file itunes_path + itunes_persistent_id).
//
// The page scan is parallelised across 4 workers. Each worker processes a
// disjoint range of pages, then results are merged.  GetAllBooks is safe to
// call concurrently (both PebbleStore and SQLiteStore hold no write lock
// during reads).
func collectITLUpdates(store database.Store) []itunes.ITLLocationUpdate {
	const (
		pageSize  = 10000
		numWorkers = 4
	)

	// First pass: count total books so we can split work evenly.
	// We do a single sequential scan here; the per-book work below is where
	// the parallelism pays off.
	type pageRange struct {
		start int
		end   int // exclusive; -1 means "until empty page"
	}

	// Collect all pages sequentially (offset arithmetic only), then dispatch
	// to workers.  Each worker handles a contiguous slice of page offsets.
	//
	// We don't know the total count up front, so workers each own a subset of
	// page offsets and stop when GetAllBooks returns an empty page.

	// Build a channel of page offsets; workers pull from it.
	pageCh := make(chan int, 256)
	go func() {
		offset := 0
		for {
			// Send the next page offset
			pageCh <- offset
			offset += pageSize
			// We keep sending; workers will stop when pages come back empty.
			// We cap at a reasonable upper bound (50M books) to avoid an
			// infinite goroutine in pathological cases.
			if offset > 50_000_000 {
				break
			}
		}
		close(pageCh)
	}()

	type result struct {
		updates []itunes.ITLLocationUpdate
	}
	resultCh := make(chan result, numWorkers)

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var local []itunes.ITLLocationUpdate
			for offset := range pageCh {
				books, err := store.GetAllBooks(pageSize, offset)
				if err != nil || len(books) == 0 {
					break
				}
				for i := range books {
					// Only collect from PRIMARY versions to avoid duplicate PIDs.
					// Both the original (imported) and organized copy have book_files
					// with the same PID but different paths. We want the primary's path.
					if books[i].IsPrimaryVersion != nil && !*books[i].IsPrimaryVersion {
						continue
					}
					files, _ := store.GetBookFiles(books[i].ID)
					if len(files) > 0 {
						for _, f := range files {
							if f.ITunesPersistentID != "" && f.ITunesPath != "" {
								local = append(local, itunes.ITLLocationUpdate{
									PersistentID: f.ITunesPersistentID,
									NewLocation:  f.ITunesPath,
								})
							}
						}
					} else if books[i].ITunesPersistentID != nil && *books[i].ITunesPersistentID != "" &&
						books[i].ITunesPath != nil && *books[i].ITunesPath != "" {
						local = append(local, itunes.ITLLocationUpdate{
							PersistentID: *books[i].ITunesPersistentID,
							NewLocation:  *books[i].ITunesPath,
						})
					}
				}
				if len(books) < pageSize {
					break
				}
			}
			resultCh <- result{updates: local}
		}()
	}

	// Close resultCh once all workers finish
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var updates []itunes.ITLLocationUpdate
	for r := range resultCh {
		updates = append(updates, r.updates...)
	}
	return updates
}

func (s *Server) handleITunesWriteBackAll(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	if !config.AppConfig.ITLWriteBackEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ITL write-back is not enabled in config"})
		return
	}

	itlPath := config.AppConfig.ITunesLibraryWritePath
	if itlPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no ITL library path configured"})
		return
	}

	if _, err := os.Stat(itlPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ITL file not found at configured path"})
		return
	}

	// Paginate through all books and collect those with iTunes PIDs
	store := database.GlobalStore
	var itlUpdates []itunes.ITLLocationUpdate
	const pageSize = 10000
	offset := 0

	for {
		books, err := store.GetAllBooks(pageSize, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to query books: %v", err)})
			return
		}
		if len(books) == 0 {
			break
		}
		for i := range books {
			book := &books[i]
			files, _ := store.GetBookFiles(book.ID)
			if len(files) > 0 {
				// Use per-file persistent IDs and paths from book_files
				for _, f := range files {
					if f.ITunesPersistentID != "" && f.ITunesPath != "" {
						itlUpdates = append(itlUpdates, itunes.ITLLocationUpdate{
							PersistentID: f.ITunesPersistentID,
							NewLocation:  f.ITunesPath,
						})
					}
				}
			} else {
				// Fallback: use book-level iTunes fields for books without book_files rows
				if book.ITunesPersistentID == nil || *book.ITunesPersistentID == "" {
					continue
				}
				if book.ITunesPath == nil || *book.ITunesPath == "" {
					continue
				}
				itlUpdates = append(itlUpdates, itunes.ITLLocationUpdate{
					PersistentID: *book.ITunesPersistentID,
					NewLocation:  *book.ITunesPath,
				})
			}
		}
		if len(books) < pageSize {
			break
		}
		offset += pageSize
	}

	if len(itlUpdates) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success":       true,
			"updated_count": 0,
			"message":       "no books with iTunes persistent IDs found",
		})
		return
	}

	itlResult, itlErr := itunes.UpdateITLLocations(itlPath, itlPath+".tmp", itlUpdates)
	if itlErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("ITL write-back failed: %v", itlErr),
		})
		return
	}

	if renameErr := itunes.RenameITLFile(itlPath+".tmp", itlPath); renameErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("ITL rename failed: %v", renameErr),
		})
		return
	}

	stdlog.Printf("[INFO] Bulk ITL write-back: updated %d tracks out of %d candidates", itlResult.UpdatedCount, len(itlUpdates))

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"updated_count": itlResult.UpdatedCount,
		"total_books":   len(itlUpdates),
		"message":       fmt.Sprintf("ITL write-back complete: %d tracks updated out of %d books with iTunes PIDs", itlResult.UpdatedCount, len(itlUpdates)),
	})
}

// handleITunesWriteBackPreview returns a comparison of local paths vs iTunes paths.
func (s *Server) handleITunesWriteBackPreview(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req ITunesWriteBackPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if _, err := os.Stat(req.LibraryPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "iTunes library file not found"})
		return
	}

	// Parse iTunes library to build persistent ID -> location map
	library, err := itunes.ParseLibrary(req.LibraryPath)
	if err != nil {
		internalError(c, "failed to parse iTunes library", err)
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

	// Get books - either specific IDs or all with iTunes persistent IDs
	var books []database.Book
	if len(req.BookIDs) > 0 {
		for _, id := range req.BookIDs {
			book, bErr := database.GlobalStore.GetBookByID(id)
			if bErr != nil || book == nil {
				continue
			}
			if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" {
				books = append(books, *book)
			}
		}
	} else {
		// Get all books and filter to those with iTunes persistent IDs
		allBooks, bErr := database.GlobalStore.GetAllBooks(0, 0)
		if bErr != nil {
			internalError(c, "failed to list books", bErr)
			return
		}
		for _, book := range allBooks {
			if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" {
				books = append(books, book)
			}
		}
	}

	// Build path mappings for reverse-mapping in preview
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
			if a, aErr := database.GlobalStore.GetAuthorByID(*book.AuthorID); aErr == nil && a != nil {
				author = a.Name
			}
		}
		// Show what would be written: reverse-map local path to iTunes format
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

	c.JSON(http.StatusOK, ITunesWriteBackPreviewResponse{
		Items: items,
		Total: len(items),
	})
}

// handleListITunesBooks returns paginated books that have iTunes persistent IDs.
func (s *Server) handleListITunesBooks(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	search := c.Query("search")
	limit := 50
	offset := 0
	if l := c.Query("limit"); l != "" {
		if v, err := fmt.Sscanf(l, "%d", &limit); err != nil || v == 0 {
			limit = 50
		}
	}
	if o := c.Query("offset"); o != "" {
		if v, err := fmt.Sscanf(o, "%d", &offset); err != nil || v == 0 {
			offset = 0
		}
	}

	var allBooks []database.Book
	var err error
	if search != "" {
		allBooks, err = database.GlobalStore.SearchBooks(search, 0, 0)
	} else {
		allBooks, err = database.GlobalStore.GetAllBooks(0, 0)
	}
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	// Filter to books with iTunes persistent IDs
	var filtered []database.Book
	for _, book := range allBooks {
		if book.ITunesPersistentID != nil && *book.ITunesPersistentID != "" {
			filtered = append(filtered, book)
		}
	}

	total := len(filtered)

	// Apply pagination
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
			if a, aErr := database.GlobalStore.GetAuthorByID(*book.AuthorID); aErr == nil && a != nil {
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

	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"count": total,
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

func (s *Server) handleITunesImportStatusBulk(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		IDs []string `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results := make(map[string]ITunesImportStatusResponse, len(req.IDs))
	for _, opID := range req.IDs {
		op, err := database.GlobalStore.GetOperationByID(opID)
		if err != nil || op == nil {
			continue
		}
		progress := calculatePercent(op.Progress, op.Total)
		snapshot := snapshotITunesImportStatus(op.ID)
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

	c.JSON(http.StatusOK, gin.H{"statuses": results})
}

// albumGroup holds tracks belonging to the same album (book).
type albumGroup struct {
	key    string // "Artist|Album"
	tracks []*itunes.Track
}

func executeITunesImport(ctx context.Context, log logger.Logger, opID string, req ITunesImportRequest) error {
	store := database.GlobalStore

	// Persist operation parameters for resume
	pathMappings := make(map[string]string)
	for _, pm := range req.PathMappings {
		pathMappings[pm.From] = pm.To
	}
	_ = operations.SaveParams(store, opID, operations.ITunesImportParams{
		LibraryXMLPath: req.LibraryPath,
		LibraryPath:    req.LibraryPath,
		ImportMode:     req.ImportMode,
		PathMappings:   pathMappings,
		SkipDuplicates: req.SkipDuplicates,
		EnrichMetadata: req.FetchMetadata,
		AutoOrganize:   !req.PreserveLocation,
	})

	// Load any existing checkpoint from a previous interrupted run
	checkpoint, _ := operations.LoadCheckpoint(store, opID)
	resumeIndex := 0
	if checkpoint != nil && checkpoint.Phase == "importing" {
		resumeIndex = checkpoint.PhaseIndex
		log.Info("Resuming import from album %d/%d", resumeIndex, checkpoint.PhaseTotal)
	}

	status := loadITunesImportStatus(opID)
	log.UpdateProgress(0, 0, "Parsing iTunes XML library...")
	log.Info("Parsing iTunes XML library: %s", req.LibraryPath)

	library, err := itunes.ParseLibrary(req.LibraryPath)
	if err != nil {
		recordITunesImportError(status, fmt.Sprintf("failed to parse library: %v", err))
		operations.ClearState(store, opID)
		return fmt.Errorf("failed to parse library: %w", err)
	}

	log.UpdateProgress(0, 0, fmt.Sprintf("Parsed %d tracks, grouping into albums...", len(library.Tracks)))
	log.Info("Parsed %d tracks, grouping into albums...", len(library.Tracks))

	// Phase 1: Group audiobook tracks by Artist|Album
	groups := groupTracksByAlbum(library)

	totalGroups := len(groups)
	setITunesImportTotal(status, totalGroups)

	log.UpdateProgress(0, totalGroups, fmt.Sprintf("Found %d audiobook albums, starting import...", totalGroups))
	log.Info("Found %d audiobook albums to import (from grouped tracks)", totalGroups)
	if totalGroups == 0 {
		log.UpdateProgress(0, 0, "No audiobooks found in library")
		operations.ClearState(store, opID)
		return nil
	}

	importMode := resolveITunesImportMode(req.ImportMode)
	importOpts := itunes.ImportOptions{
		LibraryPath:  req.LibraryPath,
		PathMappings: req.PathMappings,
	}

	// Phase 2: Quick import — file path matching + new book creation (no hashing)
	var newBookIDs []string // track IDs of newly created books for hash validation
	processed := 0
	for i, group := range groups {
		// Skip already-processed groups on resume
		if i < resumeIndex {
			processed++
			continue
		}
		if log.IsCanceled() {
			log.Info("iTunes import canceled")
			return nil
		}

		processed++
		updateITunesProcessed(status, processed)

		book, err := buildBookFromAlbumGroup(group, req.LibraryPath, importOpts)
		if err != nil {
			recordITunesFailure(status, err.Error())
			log.Error("%s", err.Error())
			updateITunesProgress(log, status, processed, totalGroups, group.key)
			continue
		}

		// Use first track for author/series assignment
		assignAuthorAndSeries(book, group.tracks[0])

		// Resolve first track's actual file path (book.FilePath may be a directory for multi-track albums)
		firstTrackPath := book.FilePath
		if len(group.tracks) > 0 {
			loc := importOpts.RemapPath(group.tracks[0].Location)
			if decoded, decErr := itunes.DecodeLocation(loc); decErr == nil {
				firstTrackPath = decoded
			}
		}

		// Check external ID map first (authoritative for PID lookups)
		if eidStore := asExternalIDStore(store); eidStore != nil && len(group.tracks) > 0 {
			firstPID := group.tracks[0].PersistentID
			if firstPID != "" {
				// Check if tombstoned — skip reimport of intentionally deleted book
				if tombstoned, _ := eidStore.IsExternalIDTombstoned("itunes", firstPID); tombstoned {
					updateITunesProgress(log, status, processed, totalGroups, book.Title)
					continue
				}
				// Check if we already have a mapping for this PID
				if bookID, err := eidStore.GetBookByExternalID("itunes", firstPID); err == nil && bookID != "" {
					if existing, err := database.GlobalStore.GetBookByID(bookID); err == nil && existing != nil {
						linkITunesMetadata(existing, book, group.tracks[0], log)
						updateITunesLinked(status)
						updateITunesProgress(log, status, processed, totalGroups, book.Title)
						continue
					}
				}
			}
		}

		if req.SkipDuplicates {
			// Fast check: file path match (no disk I/O, just DB lookup)
			if existing, err := database.GlobalStore.GetBookByFilePath(book.FilePath); err == nil && existing != nil {
				linkITunesMetadata(existing, book, group.tracks[0], log)
				updateITunesLinked(status)
				updateITunesProgress(log, status, processed, totalGroups, book.Title)
				continue
			}
		}

		book.LibraryState = stringPtr(importLibraryState(importMode))

		// New book — create a version group
		vgID := fmt.Sprintf("vg-%s", ulid.Make().String())
		book.VersionGroupID = stringPtr(vgID)
		isPrimary := false // iTunes original is non-primary; organized copy will be primary
		book.IsPrimaryVersion = &isPrimary

		// Try to extract embedded cover art from first track's actual file
		coverPath, coverErr := metadata.ExtractCoverArt(firstTrackPath)
		if coverErr == nil && coverPath != "" {
			coverFilename := filepath.Base(coverPath)
			book.CoverURL = stringPtr("/api/v1/covers/local/" + coverFilename)
		}

		created, err := database.GlobalStore.CreateBook(book)
		if err != nil {
			recordITunesFailure(status, fmt.Sprintf("Failed to save '%s': %v", book.Title, err))
			log.Error("Failed to save '%s': %v", book.Title, err)
			updateITunesProgress(log, status, processed, totalGroups)
			continue
		}

		updateITunesImported(status)
		newBookIDs = append(newBookIDs, created.ID)

		// Register all track PIDs in the external ID map
		if eidStore := asExternalIDStore(store); eidStore != nil {
			for _, albumTrack := range group.tracks {
				if albumTrack.PersistentID != "" {
					trackNum := albumTrack.TrackNumber
					trackLoc := importOpts.RemapPath(albumTrack.Location)
					trackPath, _ := itunes.DecodeLocation(trackLoc)
					_ = eidStore.CreateExternalIDMapping(&database.ExternalIDMapping{
						Source:      "itunes",
						ExternalID:  albumTrack.PersistentID,
						BookID:      created.ID,
						TrackNumber: &trackNum,
						FilePath:    trackPath,
					})
				}
			}
		}

		// Create BookFiles for multi-track albums
		if len(group.tracks) > 1 {
			totalTracks := len(group.tracks)
			for _, track := range group.tracks {
				trackLoc := importOpts.RemapPath(track.Location)
				trackPath, decErr := itunes.DecodeLocation(trackLoc)
				if decErr != nil {
					continue
				}
				trackFormat := strings.TrimPrefix(strings.ToLower(filepath.Ext(trackPath)), ".")
				bf := &database.BookFile{
					ID:                 ulid.Make().String(),
					BookID:             created.ID,
					FilePath:           trackPath,
					ITunesPersistentID: track.PersistentID,
					Format:             trackFormat,
					FileSize:           track.Size,
					Duration:           int(track.TotalTime), // already ms
					TrackNumber:        track.TrackNumber,
					TrackCount:         totalTracks,
				}
				if segHash, hashErr := scanner.ComputeSegmentFileHash(trackPath); hashErr == nil {
					bf.FileHash = segHash
				}
				if createErr := database.GlobalStore.CreateBookFile(bf); createErr != nil {
					log.Warn("Failed to create book file for track %d of '%s': %v", track.TrackNumber, book.Title, createErr)
				}
			}
		}

		// Populate book_authors junction table (multi-author aware)
		if created.AuthorID != nil && len(book.Authors) > 0 {
			for i := range book.Authors {
				book.Authors[i].BookID = created.ID
			}
			_ = database.GlobalStore.SetBookAuthors(created.ID, book.Authors)
		} else if created.AuthorID != nil {
			_ = database.GlobalStore.SetBookAuthors(created.ID, []database.BookAuthor{
				{BookID: created.ID, AuthorID: *created.AuthorID, Role: "author", Position: 0},
			})
		}

		if req.ImportPlaylists {
			// Use first track for playlist tag extraction
			tags := itunes.ExtractPlaylistTags(group.tracks[0].TrackID, library.Playlists)
			if len(tags) > 0 {
				log.Info("Playlist tags for '%s': %s", book.Title, strings.Join(tags, ", "))
			}
		}

		updateITunesProgress(log, status, processed, totalGroups, book.Title)

		// Checkpoint every 10 groups
		if processed%10 == 0 {
			_ = operations.SaveCheckpoint(store, opID, "itunes_import", "importing", processed, totalGroups)
		}
	}

	quickSummary := buildITunesSummary(status)
	log.UpdateProgress(totalGroups, totalGroups, "Quick import done: "+quickSummary)
	log.Info("Quick import completed: %s", quickSummary)

	// Phase 3: Hash validation — compute hashes for new books and link any hash matches
	if len(newBookIDs) > 0 && req.SkipDuplicates {
		_ = operations.SaveCheckpoint(store, opID, "itunes_import", "hash_validation", 0, len(newBookIDs))
		log.UpdateProgress(totalGroups, totalGroups, fmt.Sprintf("Hash validation: checking %d new books...", len(newBookIDs)))
		log.Info("Starting hash validation for %d new books...", len(newBookIDs))

		hashLinked := 0
		hashBlocked := 0
		for hi, bookID := range newBookIDs {
			if log.IsCanceled() {
				log.Info("Hash validation canceled")
				break
			}

			book, err := database.GlobalStore.GetBookByID(bookID)
			if err != nil || book == nil {
				continue
			}

			// Hash the book's file
			hash, err := scanner.ComputeFileHash(book.FilePath)
			if err != nil {
				log.Warn("Hash validation: failed to hash %s: %v", book.FilePath, err)
				continue
			}
			if hash == "" {
				continue
			}

			book.FileHash = stringPtr(hash)
			book.OriginalFileHash = stringPtr(hash)
			if importMode == itunes.ImportModeOrganized {
				book.OrganizedFileHash = stringPtr(hash)
			}

			// Check for blocked hash
			if blocked, err := database.GlobalStore.IsHashBlocked(hash); err == nil && blocked {
				log.Warn("Hash validation: blocked hash for %s, soft-deleting", book.Title)
				marked := true
				now := time.Now()
				book.MarkedForDeletion = &marked
				book.MarkedForDeletionAt = &now
				database.GlobalStore.UpdateBook(book.ID, book)
				hashBlocked++
				continue
			}

			// Check for existing book with same hash — merge into its VG
			if existing, err := database.GlobalStore.GetBookByFileHash(hash); err == nil && existing != nil && existing.ID != book.ID {
				// This new book is a duplicate — link it to the existing book's VG
				if existing.VersionGroupID != nil && *existing.VersionGroupID != "" {
					book.VersionGroupID = existing.VersionGroupID
					isPrimary := false
					book.IsPrimaryVersion = &isPrimary
				}
				hashLinked++
				log.Info("Hash validation: linked %s → %s via hash", book.Title, existing.ID)
			}

			// Save the hash (and any VG changes) to the book
			if _, err := database.GlobalStore.UpdateBook(book.ID, book); err != nil {
				log.Warn("Hash validation: failed to update %s: %v", book.ID, err)
			}

			if (hi+1)%100 == 0 || hi+1 == len(newBookIDs) {
				msg := fmt.Sprintf("Hash validation: %d/%d checked (%d linked, %d blocked)",
					hi+1, len(newBookIDs), hashLinked, hashBlocked)
				log.UpdateProgress(totalGroups, totalGroups, msg)
			}
		}
		log.Info("Hash validation completed: %d linked, %d blocked out of %d new books", hashLinked, hashBlocked, len(newBookIDs))
	}

	// Phase 4: Metadata enrichment (if requested) — runs before organize
	// so that author/title are accurate for folder structure
	if req.FetchMetadata {
		_ = operations.SaveCheckpoint(store, opID, "itunes_import", "enriching", 0, 0)
		log.Info("Starting metadata enrichment phase...")
		enrichITunesImportedBooks(log, status)
	}

	// Phase 5: Organize (if requested) — runs after enrichment
	if importMode == itunes.ImportModeOrganize && !req.PreserveLocation {
		_ = operations.SaveCheckpoint(store, opID, "itunes_import", "organizing", 0, 0)
		log.Info("Starting organize phase...")
		organizeImportedBooks(log, status)
	}

	// Clear checkpoint on successful completion
	_ = operations.ClearState(store, opID)

	// Save library fingerprint for change detection
	if fp, err := itunes.ComputeFingerprint(req.LibraryPath); err == nil {
		_ = store.SaveLibraryFingerprint(fp.Path, fp.Size, fp.ModTime, fp.CRC32)
	}

	summary := buildITunesSummary(status)
	log.UpdateProgress(totalGroups, totalGroups, summary)
	log.Info("%s", summary)
	_ = ctx
	return nil
}

// groupTracksByAlbum groups audiobook tracks by Artist|Album key.
// Tracks within each group are sorted by disc number then track number.
func groupTracksByAlbum(library *itunes.Library) []albumGroup {
	groupMap := make(map[string]*albumGroup)
	var groupOrder []string

	for _, track := range library.Tracks {
		if !itunes.IsAudiobook(track) {
			continue
		}

		artist := strings.TrimSpace(track.Artist)
		album := strings.TrimSpace(track.Album)

		// If no album, use the track name as a standalone book
		if album == "" {
			album = strings.TrimSpace(track.Name)
		}

		key := artist + "|" + album
		if _, exists := groupMap[key]; !exists {
			groupMap[key] = &albumGroup{key: key}
			groupOrder = append(groupOrder, key)
		}
		groupMap[key].tracks = append(groupMap[key].tracks, track)
	}

	// Sort tracks within each group by disc then track number
	result := make([]albumGroup, 0, len(groupOrder))
	for _, key := range groupOrder {
		g := groupMap[key]
		sort.Slice(g.tracks, func(i, j int) bool {
			if g.tracks[i].DiscNumber != g.tracks[j].DiscNumber {
				return g.tracks[i].DiscNumber < g.tracks[j].DiscNumber
			}
			return g.tracks[i].TrackNumber < g.tracks[j].TrackNumber
		})
		result = append(result, *g)
	}

	return result
}

// enrichITunesImportedBooks fetches metadata for recently imported books
// to normalize author names and get cover art before organizing.
func enrichITunesImportedBooks(log logger.Logger, status *itunesImportStatus) {
	mfs := NewMetadataFetchService(database.GlobalStore)

	// Get all imported books (library_state = 'imported')
	books, err := database.GlobalStore.GetAllBooks(10000, 0)
	if err != nil {
		log.Error("Failed to list books for enrichment: %v", err)
		return
	}

	enriched := 0
	consecutiveErrors := 0
	for i, book := range books {
		if book.LibraryState == nil || *book.LibraryState != "imported" {
			continue
		}
		if book.ITunesImportSource == nil {
			continue
		}

		resp, err := mfs.FetchMetadataForBook(book.ID)
		if err != nil {
			log.Debug("No metadata found for '%s': %v", book.Title, err)
			consecutiveErrors++
			// Back off if we're hitting rate limits (many consecutive failures)
			if consecutiveErrors >= 5 {
				log.Info("Rate limit detected, pausing 10s...")
				time.Sleep(10 * time.Second)
				consecutiveErrors = 0
			}
			continue
		}

		consecutiveErrors = 0
		enriched++
		// If metadata found a new author, add to book_authors without clobbering existing multi-author links
		if resp.Book != nil && resp.Book.AuthorID != nil {
			existing, _ := database.GlobalStore.GetBookAuthors(book.ID)
			if len(existing) <= 1 {
				// Only one or zero authors — safe to replace with metadata result
				_ = database.GlobalStore.SetBookAuthors(book.ID, []database.BookAuthor{
					{BookID: book.ID, AuthorID: *resp.Book.AuthorID, Role: "author", Position: 0},
				})
			}
			// If multiple authors already linked, don't overwrite — keep the split authors
		}

		// Rate limit: pause every 10 enrichments to avoid hammering external APIs
		if enriched%10 == 0 {
			log.Info("Enriched %d books so far (processing %d/%d)...", enriched, i+1, len(books))
			time.Sleep(2 * time.Second)
		}
	}

	log.Info("Metadata enrichment complete: %d books enriched", enriched)
}

// organizeImportedBooks moves all imported books into the organized folder structure.
// Runs as a separate phase after metadata enrichment so author/title are accurate.
func organizeImportedBooks(log logger.Logger, status *itunesImportStatus) {
	books, err := database.GlobalStore.GetAllBooks(100000, 0)
	if err != nil {
		log.Error("Failed to list books for organize: %v", err)
		return
	}

	organized := 0
	for i := range books {
		book := &books[i]
		if book.LibraryState == nil || *book.LibraryState != "imported" {
			continue
		}
		if book.ITunesImportSource == nil {
			continue
		}

		oldPath := book.FilePath
		if err := organizeImportedBook(book, log); err != nil {
			recordITunesFailure(status, fmt.Sprintf("Failed to organize '%s': %v", book.Title, err))
			log.Warn("Failed to organize '%s': %v", book.Title, err)
		} else {
			book.LibraryState = stringPtr("organized")
			if _, err := database.GlobalStore.UpdateBook(book.ID, book); err != nil {
				log.Error("Failed to update organized path for '%s': %v — rolling back", book.Title, err)
				if book.FilePath != oldPath {
					if rbErr := os.Rename(book.FilePath, oldPath); rbErr != nil {
						log.Error("CRITICAL: rollback failed for %s: file at %s, DB expects %s", book.ID, book.FilePath, oldPath)
					} else {
						book.FilePath = oldPath
					}
				}
			} else {
				organized++
			}
		}
	}

	log.Info("Organize phase complete: %d books organized", organized)
}

// buildBookFromAlbumGroup creates a single Book from a group of tracks
// that belong to the same album. For single-track groups, it behaves
// like the old buildBookFromTrack. For multi-track groups, it uses the
// album name as the title and sums durations/sizes.
// linkITunesMetadata copies iTunes-specific fields (play count, rating, bookmark,
// persistent ID, date added) from an import book onto an existing book that was
// matched by file path. Also ensures the existing book has a version group.
func linkITunesMetadata(existing *database.Book, importBook *database.Book, track *itunes.Track, log logger.Logger) {
	changed := false
	if existing.ITunesPersistentID == nil && importBook.ITunesPersistentID != nil {
		existing.ITunesPersistentID = importBook.ITunesPersistentID
		changed = true
	}
	if existing.ITunesPlayCount == nil && importBook.ITunesPlayCount != nil {
		existing.ITunesPlayCount = importBook.ITunesPlayCount
		changed = true
	}
	if existing.ITunesRating == nil && importBook.ITunesRating != nil {
		existing.ITunesRating = importBook.ITunesRating
		changed = true
	}
	if existing.ITunesBookmark == nil && importBook.ITunesBookmark != nil {
		existing.ITunesBookmark = importBook.ITunesBookmark
		changed = true
	}
	if existing.ITunesDateAdded == nil && importBook.ITunesDateAdded != nil {
		existing.ITunesDateAdded = importBook.ITunesDateAdded
		changed = true
	}
	if existing.ITunesImportSource == nil && importBook.ITunesImportSource != nil {
		existing.ITunesImportSource = importBook.ITunesImportSource
		changed = true
	}
	// Ensure it has a version group and is marked as primary
	if existing.VersionGroupID == nil || *existing.VersionGroupID == "" {
		vgID := fmt.Sprintf("vg-%s", ulid.Make().String())
		existing.VersionGroupID = &vgID
		changed = true
	}
	if existing.IsPrimaryVersion == nil || !*existing.IsPrimaryVersion {
		isPrimary := true
		existing.IsPrimaryVersion = &isPrimary
		changed = true
	}
	if changed {
		if _, err := database.GlobalStore.UpdateBook(existing.ID, existing); err != nil {
			log.Warn("Failed to link iTunes metadata to %s: %v", existing.ID, err)
		}
	}
}

// linkAsVersion creates the import book as a non-primary version linked to the
// existing book's version group. The existing book (organized copy) stays primary.
func linkAsVersion(existing *database.Book, importBook *database.Book, track *itunes.Track, log logger.Logger) {
	// Ensure the existing book has a version group
	if existing.VersionGroupID == nil || *existing.VersionGroupID == "" {
		vgID := fmt.Sprintf("vg-%s", ulid.Make().String())
		existing.VersionGroupID = &vgID
		isPrimary := true
		existing.IsPrimaryVersion = &isPrimary
		if _, err := database.GlobalStore.UpdateBook(existing.ID, existing); err != nil {
			log.Warn("Failed to set VG on existing book %s: %v", existing.ID, err)
			return
		}
	}

	// Create the iTunes book as a non-primary version in the same VG
	importBook.VersionGroupID = existing.VersionGroupID
	isPrimary := false
	importBook.IsPrimaryVersion = &isPrimary
	importBook.LibraryState = stringPtr("imported")

	created, err := database.GlobalStore.CreateBook(importBook)
	if err != nil {
		log.Warn("Failed to create version link for %s: %v", importBook.Title, err)
		return
	}

	// Copy iTunes metadata to the existing primary if it's missing
	linkITunesMetadata(existing, importBook, track, log)

	log.Info("Created version link: %s (iTunes) → %s (primary) in %s", created.ID, existing.ID, *existing.VersionGroupID)
}

func buildBookFromAlbumGroup(group albumGroup, libraryPath string, opts itunes.ImportOptions) (*database.Book, error) {
	if len(group.tracks) == 0 {
		return nil, fmt.Errorf("album group has no tracks")
	}

	firstTrack := group.tracks[0]

	// Resolve file path for first track (used as the book's primary file path)
	location := opts.RemapPath(firstTrack.Location)
	filePath, err := itunes.DecodeLocation(location)
	if err != nil {
		return nil, fmt.Errorf("failed to decode location: %w", err)
	}
	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	// For multi-track albums, use the common parent directory as FilePath
	// and the Album as the title. For single-track, use the file itself.
	title := strings.TrimSpace(firstTrack.Album)
	bookFilePath := filePath
	if len(group.tracks) > 1 && title != "" {
		// Find common parent directory of all tracks
		bookFilePath = commonParentDir(group.tracks, opts)
	}
	if title == "" {
		title = strings.TrimSpace(firstTrack.Name)
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}

	// Sum durations and sizes across all tracks
	var totalDurationMs int64
	var totalSize int64
	for _, t := range group.tracks {
		totalDurationMs += t.TotalTime
		totalSize += t.Size
	}

	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
	var duration *int
	if totalDurationMs > 0 {
		seconds := int(totalDurationMs / 1000)
		duration = &seconds
	}
	var releaseYear *int
	if firstTrack.Year > 0 {
		releaseYear = intPtr(firstTrack.Year)
	}
	var persistentID *string
	if firstTrack.PersistentID != "" {
		persistentID = stringPtr(firstTrack.PersistentID)
	}

	book := &database.Book{
		Title:                title,
		FilePath:             bookFilePath,
		Format:               format,
		Duration:             duration,
		OriginalFilename:     stringPtr(filepath.Base(filePath)),
		AudiobookReleaseYear: releaseYear,
		ITunesPersistentID:   persistentID,
		ITunesPlayCount:      intPtr(firstTrack.PlayCount),
		ITunesRating:         intPtr(firstTrack.Rating),
		ITunesBookmark:       int64Ptr(firstTrack.Bookmark),
		ITunesImportSource:   stringPtr(libraryPath),
	}

	if !firstTrack.DateAdded.IsZero() {
		book.ITunesDateAdded = &firstTrack.DateAdded
	}
	if firstTrack.PlayDate > 0 {
		lastPlayed := time.Unix(firstTrack.PlayDate, 0)
		book.ITunesLastPlayed = &lastPlayed
	}
	if firstTrack.AlbumArtist != "" && firstTrack.AlbumArtist != firstTrack.Artist {
		book.Narrator = stringPtr(firstTrack.AlbumArtist)
	}
	// Comments field typically contains the book description/synopsis
	if firstTrack.Comments != "" {
		book.Description = stringPtr(firstTrack.Comments)
	}
	if totalSize > 0 {
		book.FileSize = &totalSize
	}

	if len(group.tracks) > 1 {
		stdlog.Printf("iTunes import: grouped %d tracks into album %q", len(group.tracks), title)
	}

	return book, nil
}

// commonParentDir finds the common parent directory for all tracks in a group.
func commonParentDir(tracks []*itunes.Track, opts itunes.ImportOptions) string {
	if len(tracks) == 0 {
		return ""
	}

	// Decode all paths
	var paths []string
	for _, t := range tracks {
		location := opts.RemapPath(t.Location)
		p, err := itunes.DecodeLocation(location)
		if err != nil {
			continue
		}
		paths = append(paths, filepath.Dir(p))
	}
	if len(paths) == 0 {
		return ""
	}

	// Find common parent directory (must match on path boundaries, not substring)
	common := paths[0]
	for _, p := range paths[1:] {
		for common != p && !strings.HasPrefix(p, common+string(filepath.Separator)) {
			common = filepath.Dir(common)
			if common == "/" || common == "." {
				return common
			}
		}
	}
	return common
}

// assignAuthorAndSeries resolves the track's Artist into one or more author
// records (splitting composites like "A / B") and links them to the book.
// The first author becomes the primary AuthorID; all are stored in book_authors.
// This is called both during import and sync for new books.
func assignAuthorAndSeries(book *database.Book, track *itunes.Track) {
	if book == nil || track == nil {
		return
	}

	if track.Artist != "" {
		ids, err := ensureAuthorIDs(track.Artist)
		if err == nil && len(ids) > 0 {
			book.AuthorID = &ids[0]
			// Store multi-author links (needs book.ID set — caller must
			// call this after CreateBook for the sync path, or we defer
			// to the post-create hook)
			book.Authors = make([]database.BookAuthor, 0, len(ids))
			for i, id := range ids {
				book.Authors = append(book.Authors, database.BookAuthor{
					AuthorID: id,
					Role:     "author",
					Position: i,
				})
			}
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

// ensureAuthorIDs resolves an author name string (which may be composite like
// "Author1 / Author2") into individual author records. Returns all author IDs
// with the first being the primary. If the name is not composite, returns a
// single-element slice.
func ensureAuthorIDs(name string) ([]int, error) {
	parts := SplitCompositeAuthorName(name)
	if len(parts) == 0 {
		// Not composite — treat as single author
		parts = []string{name}
	}

	var ids []int
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		part = NormalizeAuthorName(part)
		author, err := database.GlobalStore.GetAuthorByName(part)
		if err != nil {
			return nil, err
		}
		if author == nil {
			author, err = database.GlobalStore.CreateAuthor(part)
			if err != nil {
				return nil, err
			}
		}
		ids = append(ids, author.ID)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no valid author names in %q", name)
	}
	return ids, nil
}

// ensureAuthorID resolves a (possibly composite) author name and returns the
// primary author ID. For backwards compatibility with callers that only need one ID.
func ensureAuthorID(name string) (*int, error) {
	ids, err := ensureAuthorIDs(name)
	if err != nil {
		return nil, err
	}
	return &ids[0], nil
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

func organizeImportedBook(book *database.Book, log logger.Logger) error {
	if book == nil {
		return fmt.Errorf("book is nil")
	}
	if config.AppConfig.RootDir == "" {
		return fmt.Errorf("root_dir is not configured")
	}

	org := organizer.NewOrganizer(&config.AppConfig)
	newPath, _, err := org.OrganizeBook(book)
	if err != nil {
		return err
	}
	if newPath != "" && newPath != book.FilePath {
		book.FilePath = newPath
		applyOrganizedFileMetadata(book, newPath)
		log.Info("Organized '%s' to %s", book.Title, newPath)
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

func snapshotITunesImportStatus(opID string) *itunesImportStatus {
	status := loadITunesImportStatus(opID)
	status.mu.Lock()
	defer status.mu.Unlock()

	snapshot := &itunesImportStatus{
		Total:     status.Total,
		Processed: status.Processed,
		Imported:  status.Imported,
		Skipped:   status.Skipped,
		Linked:    status.Linked,
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

func updateITunesLinked(status *itunesImportStatus) {
	status.mu.Lock()
	status.Linked++
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

func updateITunesProgress(log logger.Logger, status *itunesImportStatus, processed, total int, currentTitle ...string) {
	status.mu.Lock()
	current := status.Processed
	imported := status.Imported
	linked := status.Linked
	skipped := status.Skipped
	failed := status.Failed
	status.mu.Unlock()

	if processed%itunesImportProgressBatch != 0 && processed != total {
		return
	}

	title := ""
	if len(currentTitle) > 0 {
		title = currentTitle[0]
	}

	message := fmt.Sprintf(
		"Book %d/%d — %d new, %d linked, %d skipped, %d failed",
		current,
		total,
		imported,
		linked,
		skipped,
		failed,
	)
	if title != "" {
		message += fmt.Sprintf(" — %s", title)
	}
	log.UpdateProgress(processed, total, message)
}

func buildITunesSummary(status *itunesImportStatus) string {
	status.mu.Lock()
	defer status.mu.Unlock()
	return fmt.Sprintf(
		"Import completed: %d new, %d linked, %d skipped, %d failed",
		status.Imported,
		status.Linked,
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

// handleITunesLibraryStatus returns the current status of an iTunes library file.
func (s *Server) handleITunesLibraryStatus(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path query parameter required"})
		return
	}

	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	rec, err := database.GlobalStore.GetLibraryFingerprint(path)
	if err != nil {
		internalError(c, "failed to get library fingerprint", err)
		return
	}

	response := gin.H{
		"path":                 path,
		"configured":           true,
		"fingerprint_stored":   rec != nil,
		"changed_since_import": false,
	}

	if rec != nil {
		response["last_imported"] = rec.UpdatedAt

		// Quick mtime+size check (no CRC32 for polling)
		if info, err := os.Stat(path); err == nil {
			if info.Size() != rec.Size || !info.ModTime().Equal(rec.ModTime) {
				response["changed_since_import"] = true
				response["last_external_change"] = info.ModTime()
			}
		}
	}

	// Also check fsnotify watcher if available
	if s.libraryWatcher != nil && s.libraryWatcher.HasChanged() {
		response["changed_since_import"] = true
		if changedAt := s.libraryWatcher.ChangedAt(); !changedAt.IsZero() {
			response["last_external_change"] = changedAt
		}
	}

	c.JSON(http.StatusOK, response)
}

// ITunesSyncRequest represents a request to sync from iTunes Library.xml.
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

// handleITunesSync triggers an incremental sync from iTunes Library.xml.
func (s *Server) handleITunesSync(c *gin.Context) {
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	var req ITunesSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body — will discover library path from DB
		req = ITunesSyncRequest{}
	}

	// Discover library path if not provided
	libraryPath := req.LibraryPath
	if libraryPath == "" {
		libraryPath = config.AppConfig.ITunesLibraryReadPath
	}
	if libraryPath == "" {
		libraryPath = discoverITunesLibraryPath()
	}
	if libraryPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no iTunes library path configured or provided"})
		return
	}

	if _, err := os.Stat(libraryPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "iTunes library file not found"})
		return
	}

	// Check fingerprint — skip if unchanged (unless forced)
	if !req.Force {
		if rec, err := database.GlobalStore.GetLibraryFingerprint(libraryPath); err == nil && rec != nil {
			if info, statErr := os.Stat(libraryPath); statErr == nil {
				if info.Size() == rec.Size && info.ModTime().Equal(rec.ModTime) {
					c.JSON(http.StatusOK, gin.H{"message": "no changes detected — use force:true to sync anyway", "operation_id": ""})
					return
				}
			}
		}
	}

	opID := ulid.Make().String()
	op, err := database.GlobalStore.CreateOperation(opID, "itunes_sync", &libraryPath)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	pathMappings := req.PathMappings
	// Fall back to configured path mappings if none in the request
	if len(pathMappings) == 0 {
		for _, m := range config.AppConfig.ITunesPathMappings {
			pathMappings = append(pathMappings, itunes.PathMapping{From: m.From, To: m.To})
		}
	}
	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return executeITunesSync(ctx, operations.LoggerFromReporter(progress), libraryPath, pathMappings)
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "itunes_sync", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, ITunesSyncResponse{
		OperationID: op.ID,
		Message:     "iTunes sync operation queued",
	})
}

// discoverITunesLibraryPath finds the library path from the most recent imported book.
func discoverITunesLibraryPath() string {
	if database.GlobalStore == nil {
		return ""
	}
	books, err := database.GlobalStore.GetAllBooks(100, 0)
	if err != nil {
		return ""
	}
	for _, book := range books {
		if book.ITunesImportSource != nil && *book.ITunesImportSource != "" {
			return *book.ITunesImportSource
		}
	}
	return ""
}

// executeITunesSync re-reads an iTunes Library.xml and updates changed fields
// or imports new audiobooks.
func executeITunesSync(ctx context.Context, log logger.Logger, libraryPath string, pathMappings []itunes.PathMapping) error {
	store := database.GlobalStore

	log.UpdateProgress(0, 0, "Parsing iTunes library XML...")
	log.Info("Starting iTunes sync from %s", libraryPath)

	library, err := itunes.ParseLibrary(libraryPath)
	if err != nil {
		return fmt.Errorf("failed to parse library: %w", err)
	}
	trackCount := len(library.Tracks)
	log.Info("Parsed %d tracks from iTunes library", trackCount)
	log.UpdateProgress(0, 0, fmt.Sprintf("Grouping %d tracks by album...", trackCount))

	groups := groupTracksByAlbum(library)
	totalGroups := len(groups)
	log.Info("Found %d audiobook groups from %d tracks", totalGroups, trackCount)
	if totalGroups == 0 {
		log.UpdateProgress(0, 0, "No audiobooks found in library")
		log.Warn("No audiobooks found in library")
		return nil
	}

	// Apply any deferred iTunes updates (e.g., from transcodes while write-back was disabled)
	if config.AppConfig.ITLWriteBackEnabled && config.AppConfig.ITunesLibraryWritePath != "" {
		pending, _ := store.GetPendingDeferredITunesUpdates()
		if len(pending) > 0 {
			updates := make([]itunes.ITLLocationUpdate, len(pending))
			for i, p := range pending {
				updates[i] = itunes.ITLLocationUpdate{
					PersistentID: p.PersistentID,
					NewLocation:  p.NewPath,
				}
			}
			itlPath := config.AppConfig.ITunesLibraryWritePath
			tmpPath := itlPath + ".deferred-update.tmp"
			result, itlErr := itunes.UpdateITLLocations(itlPath, tmpPath, updates)
			if itlErr == nil && result.UpdatedCount > 0 {
				_ = itunes.RenameITLFile(tmpPath, itlPath)
				for _, p := range pending {
					_ = store.MarkDeferredITunesUpdateApplied(p.ID)
				}
				log.Info("Applied %d deferred iTunes updates", result.UpdatedCount)
			} else if itlErr != nil {
				log.Warn("Failed to apply deferred iTunes updates: %v", itlErr)
				_ = os.Remove(tmpPath)
			}
		}
	}

	importOpts := itunes.ImportOptions{
		LibraryPath:  libraryPath,
		PathMappings: pathMappings,
	}

	// Pre-build persistent ID → Book index to avoid O(n) scan per group
	log.UpdateProgress(0, 0, "Building persistent ID index...")
	allBooks, err := store.GetAllBooks(100000, 0)
	if err != nil {
		return fmt.Errorf("failed to load books for index: %w", err)
	}
	pidIndex := make(map[string]*database.Book, len(allBooks))
	pathIndex := make(map[string]*database.Book, len(allBooks))
	titleIndex := make(map[string]*database.Book, len(allBooks))
	for i := range allBooks {
		if allBooks[i].ITunesPersistentID != nil && *allBooks[i].ITunesPersistentID != "" {
			pidIndex[*allBooks[i].ITunesPersistentID] = &allBooks[i]
		}
		pathIndex[allBooks[i].FilePath] = &allBooks[i]
		titleIndex[strings.ToLower(allBooks[i].Title)] = &allBooks[i]
	}
	log.Info("Indexed %d books (%d with iTunes persistent IDs)", len(allBooks), len(pidIndex))

	// pendingFiles collects book_files to be batch-upserted. We flush every
	// itunesBatchFlushSize groups to bound memory usage while amortising the
	// per-transaction overhead of individual UpsertBookFile calls.
	const itunesBatchFlushSize = 500
	var pendingFiles []*database.BookFile

	flushPendingFiles := func() {
		if len(pendingFiles) == 0 {
			return
		}
		if err := store.BatchUpsertBookFiles(pendingFiles); err != nil {
			log.Error("BatchUpsertBookFiles failed (continuing): %v", err)
		}
		pendingFiles = pendingFiles[:0]
	}

	var updated, newBooks, unchanged int
	for i, group := range groups {
		if log.IsCanceled() {
			log.Info("iTunes sync canceled")
			return nil
		}

		if len(group.tracks) == 0 {
			continue
		}

		firstTrack := group.tracks[0]
		persistentID := firstTrack.PersistentID
		if persistentID == "" {
			continue
		}

		existing := pidIndex[persistentID]

		// Fallback: match by title (case-insensitive) — fast, no I/O
		if existing == nil {
			title := strings.TrimSpace(firstTrack.Album)
			if title == "" {
				title = strings.TrimSpace(firstTrack.Name)
			}
			if title != "" {
				existing = titleIndex[strings.ToLower(title)]
			}
		}

		// Fallback: match by file path (requires building book, has os.Stat)
		if existing == nil {
			book, err := buildBookFromAlbumGroup(group, libraryPath, importOpts)
			if err == nil {
				if match := pathIndex[book.FilePath]; match != nil {
					existing = match
				}
			}
		}

		// Backfill PersistentID on matched book so future imports match directly
		if existing != nil && (existing.ITunesPersistentID == nil || *existing.ITunesPersistentID == "") {
			existing.ITunesPersistentID = stringPtr(persistentID)
			pidIndex[persistentID] = existing
		}

		if existing != nil {
			// Compare fields and update if changed
			changed := false

			newPlayCount := intPtr(firstTrack.PlayCount)
			if existing.ITunesPlayCount == nil || *existing.ITunesPlayCount != *newPlayCount {
				existing.ITunesPlayCount = newPlayCount
				changed = true
			}

			newRating := intPtr(firstTrack.Rating)
			if existing.ITunesRating == nil || *existing.ITunesRating != *newRating {
				existing.ITunesRating = newRating
				changed = true
			}

			newBookmark := int64Ptr(firstTrack.Bookmark)
			if existing.ITunesBookmark == nil || *existing.ITunesBookmark != *newBookmark {
				existing.ITunesBookmark = newBookmark
				changed = true
			}

			if firstTrack.PlayDate > 0 {
				lastPlayed := time.Unix(firstTrack.PlayDate, 0)
				if existing.ITunesLastPlayed == nil || !existing.ITunesLastPlayed.Equal(lastPlayed) {
					existing.ITunesLastPlayed = &lastPlayed
					changed = true
				}
			}

			// Store the iTunes file location URL for write-back
			if firstTrack.Location != "" {
				loc := firstTrack.Location
				if existing.ITunesPath == nil || *existing.ITunesPath != loc {
					existing.ITunesPath = &loc
					changed = true
					log.Info("Set itunes_path for %s: %s", existing.Title, loc[:min(80, len(loc))])
				}
			} else {
				log.Debug("No Location for PID %s (%s)", persistentID, existing.Title)
			}

			if changed {
				if _, err := store.UpdateBook(existing.ID, existing); err != nil {
					log.Error("Failed to update '%s': %v", existing.Title, err)
				} else {
					updated++
					// Dual-write to unified activity log
					if itunesActivityRecorder != nil {
						itunesActivityRecorder(database.ActivityEntry{
							Tier:    "change",
							Type:    "itunes_sync",
							Level:   "info",
							Source:  "scheduler",
							BookID:  existing.ID,
							Summary: fmt.Sprintf("iTunes sync updated: %s", existing.Title),
							Tags:    []string{"itunes"},
						})
					}
				}
			} else {
				unchanged++
			}

			// Collect book_files for tracks that are new or changed; skip
			// unchanged ones (same PID + same iTunes path).
			for _, track := range group.tracks {
				if track.PersistentID == "" {
					continue
				}
				// Check if this track already exists with same data
				existingFile, _ := store.GetBookFileByPID(track.PersistentID)
				if existingFile != nil && existingFile.ITunesPath == track.Location {
					continue // unchanged — skip
				}

				remappedPath := importOpts.RemapPath(track.Location)
				decodedPath, _ := itunes.DecodeLocation(remappedPath)
				if decodedPath == "" {
					decodedPath = remappedPath
				}
				// Safety: if the decoded path is still a Windows path (e.g. "X:/..."),
				// try remapping the decoded path directly against each configured mapping.
				decodedPath = remapWindowsPath(decodedPath, importOpts)
				pendingFiles = append(pendingFiles, &database.BookFile{
					BookID:             existing.ID,
					FilePath:           decodedPath,
					ITunesPath:         track.Location,
					ITunesPersistentID: track.PersistentID,
					TrackNumber:        track.TrackNumber,
					TrackCount:         track.TrackCount,
					DiscNumber:         track.DiscNumber,
					DiscCount:          track.DiscCount,
					Title:              track.Name,
					Format:             strings.TrimPrefix(filepath.Ext(decodedPath), "."),
					Duration:           int(track.TotalTime),
					FileSize:           track.Size,
				})
			}
		} else {
			// Import as new book
			book, err := buildBookFromAlbumGroup(group, libraryPath, importOpts)
			if err != nil {
				log.Warn("Failed to build book from group '%s': %v", group.key, err)
				continue
			}
			assignAuthorAndSeries(book, firstTrack)
			book.LibraryState = stringPtr("imported")

			created, err := store.CreateBook(book)
			if err != nil {
				log.Error("Failed to create '%s': %v", book.Title, err)
			} else {
				newBooks++
				// Set up book_authors junction table
				if created.AuthorID != nil && len(book.Authors) > 0 {
					for i := range book.Authors {
						book.Authors[i].BookID = created.ID
					}
					_ = store.SetBookAuthors(created.ID, book.Authors)
				} else if created.AuthorID != nil {
					_ = store.SetBookAuthors(created.ID, []database.BookAuthor{
						{BookID: created.ID, AuthorID: *created.AuthorID, Role: "author", Position: 0},
					})
				}

				// Collect book_files for every track in the group
				for _, track := range group.tracks {
					remappedPath := importOpts.RemapPath(track.Location)
					decodedPath, _ := itunes.DecodeLocation(remappedPath)
					if decodedPath == "" {
						decodedPath = remappedPath
					}
					// Safety: if the decoded path is still a Windows path (e.g. "X:/..."),
					// try remapping the decoded path directly against each configured mapping.
					decodedPath = remapWindowsPath(decodedPath, importOpts)
					pendingFiles = append(pendingFiles, &database.BookFile{
						BookID:             created.ID,
						FilePath:           decodedPath,
						ITunesPath:         track.Location,
						ITunesPersistentID: track.PersistentID,
						TrackNumber:        track.TrackNumber,
						TrackCount:         track.TrackCount,
						DiscNumber:         track.DiscNumber,
						DiscCount:          track.DiscCount,
						Title:              track.Name,
						Format:             strings.TrimPrefix(filepath.Ext(decodedPath), "."),
						Duration:           int(track.TotalTime),
						FileSize:           track.Size,
					})
				}
			}
		}

		// Flush collected files every itunesBatchFlushSize groups
		if len(pendingFiles) >= itunesBatchFlushSize {
			flushPendingFiles()
		}

		processed := i + 1
		if processed%itunesImportProgressBatch == 0 || processed == totalGroups {
			message := fmt.Sprintf("Syncing book %d of %d (updated %d, new %d, unchanged %d)",
				processed, totalGroups, updated, newBooks, unchanged)
			log.UpdateProgress(processed, totalGroups, message)
		}
	}

	// Flush any remaining pending files after the loop
	flushPendingFiles()

	// Save fingerprint after sync
	if fp, err := itunes.ComputeFingerprint(libraryPath); err == nil {
		_ = store.SaveLibraryFingerprint(fp.Path, fp.Size, fp.ModTime, fp.CRC32)
	}

	summary := fmt.Sprintf("Sync completed: %d updated, %d new, %d unchanged (from %d tracks, %d groups)",
		updated, newBooks, unchanged, trackCount, totalGroups)
	log.UpdateProgress(totalGroups, totalGroups, summary)
	log.Info("%s", summary)

	// Note: ITL write-back no longer runs after sync — only after organize.
	// Sync imports existing iTunes paths which don't need writing back.
	// Write-back is triggered by POST /itunes/write-back-all or after organize.

	return nil
}

// remapWindowsPath is a last-resort helper that detects raw Windows paths
// (e.g. "X:/books/itunes/...") that survived RemapPath+DecodeLocation unchanged
// and tries each configured path mapping to convert them to Linux paths.
// If no mapping matches, the original path is returned.
func remapWindowsPath(p string, opts itunes.ImportOptions) string {
	if len(p) < 2 || p[1] != ':' {
		return p // not a Windows drive-letter path
	}
	normalized := strings.ReplaceAll(p, "\\", "/")
	for _, m := range opts.PathMappings {
		from := strings.ReplaceAll(m.From, "\\", "/")
		if from == "" || m.To == "" {
			continue
		}
		// Strip any file:// prefix from the mapping so we can compare plain paths.
		plainFrom := from
		if strings.HasPrefix(plainFrom, "file://localhost/") {
			plainFrom = plainFrom[len("file://localhost/"):]
		} else if strings.HasPrefix(plainFrom, "file:///") {
			plainFrom = plainFrom[len("file:///"):]
		}
		if plainFrom == "" {
			continue
		}
		if strings.HasPrefix(normalized, plainFrom) {
			return m.To + normalized[len(plainFrom):]
		}
		// Case-insensitive fallback.
		if strings.HasPrefix(strings.ToLower(normalized), strings.ToLower(plainFrom)) {
			return m.To + normalized[len(plainFrom):]
		}
	}
	return p
}
