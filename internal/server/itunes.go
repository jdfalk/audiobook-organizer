// file: internal/server/itunes.go
// version: 2.5.0
// guid: 719912e9-7b5f-48e1-afa6-1b0b7f57c2fa

package server

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"log"
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
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
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
	LibraryPath  string               `json:"library_path" binding:"required"`
	PathMappings []itunes.PathMapping `json:"path_mappings,omitempty"`
}

// ITunesValidateResponse summarizes validation results for an iTunes library.
type ITunesValidateResponse struct {
	TotalTracks     int      `json:"total_tracks"`
	AudiobookTracks int      `json:"audiobook_tracks"`
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

// ITunesWriteBackRequest represents a write-back request for iTunes updates.
type ITunesWriteBackRequest struct {
	LibraryPath    string   `json:"library_path" binding:"required"`
	AudiobookIDs   []string `json:"audiobook_ids"`
	CreateBackup   bool     `json:"create_backup"`
	ForceOverwrite bool     `json:"force_overwrite"`
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

	log.Printf("iTunes validate: library=%s, mappings=%d", req.LibraryPath, len(req.PathMappings))

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

	log.Printf("iTunes validate complete: %d audiobooks, %d found, %d missing, prefixes=%v",
		result.AudiobookTracks, result.FilesFound, result.FilesMissing, result.PathPrefixes)

	response := ITunesValidateResponse{
		TotalTracks:     result.TotalTracks,
		AudiobookTracks: result.AudiobookTracks,
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

	log.Printf("iTunes test-mapping: from=%q to=%q", req.From, req.To)
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
			log.Printf("  [%d/20] decode error for %q: %v", response.Tested, track.Name, err)
			continue
		}
		if _, err := os.Stat(path); err == nil {
			response.Found++
			log.Printf("  [%d/20] FOUND: %q → %s", response.Tested, track.Name, path)
			if len(response.Examples) < 3 {
				response.Examples = append(response.Examples, ITunesTestExample{
					Title: track.Name,
					Path:  path,
				})
			}
		} else {
			log.Printf("  [%d/20] MISSING: %q → %s", response.Tested, track.Name, path)
		}
	}

	log.Printf("iTunes test-mapping: tested=%d found=%d examples=%d", response.Tested, response.Found, len(response.Examples))
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

	// Load stored fingerprint for conflict detection
	var storedFP *itunes.LibraryFingerprint
	if rec, err := database.GlobalStore.GetLibraryFingerprint(req.LibraryPath); err == nil && rec != nil {
		storedFP = &itunes.LibraryFingerprint{
			Path:    rec.Path,
			Size:    rec.Size,
			ModTime: rec.ModTime,
			CRC32:   rec.CRC32,
		}
	}

	opts := itunes.WriteBackOptions{
		LibraryPath:       req.LibraryPath,
		Updates:           updates,
		CreateBackup:      req.CreateBackup,
		ForceOverwrite:    req.ForceOverwrite,
		StoredFingerprint: storedFP,
	}

	result, err := itunes.WriteBack(opts)
	if err != nil {
		var modErr *itunes.ErrLibraryModified
		if errors.As(err, &modErr) {
			c.JSON(http.StatusConflict, gin.H{
				"error":         "library_modified",
				"message":       modErr.Error(),
				"stored_size":   modErr.Stored.Size,
				"current_size":  modErr.Current.Size,
				"stored_mtime":  modErr.Stored.ModTime,
				"current_mtime": modErr.Current.ModTime,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("write-back failed: %v", err),
		})
		return
	}

	// Update fingerprint after successful write-back
	if fp, err := itunes.ComputeFingerprint(req.LibraryPath); err == nil {
		_ = database.GlobalStore.SaveLibraryFingerprint(fp.Path, fp.Size, fp.ModTime, fp.CRC32)
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

// albumGroup holds tracks belonging to the same album (book).
type albumGroup struct {
	key    string // "Artist|Album"
	tracks []*itunes.Track
}

func executeITunesImport(ctx context.Context, progress operations.ProgressReporter, opID string, req ITunesImportRequest) error {
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
		_ = progress.Log("info", fmt.Sprintf("Resuming import from album %d/%d", resumeIndex, checkpoint.PhaseTotal), nil)
	}

	status := loadITunesImportStatus(opID)
	progressMessage := "Starting iTunes import"
	_ = progress.UpdateProgress(0, 0, progressMessage)
	_ = progress.Log("info", progressMessage, nil)

	library, err := itunes.ParseLibrary(req.LibraryPath)
	if err != nil {
		recordITunesImportError(status, fmt.Sprintf("failed to parse library: %v", err))
		operations.ClearState(store, opID)
		return fmt.Errorf("failed to parse library: %w", err)
	}

	// Phase 1: Group audiobook tracks by Artist|Album
	groups := groupTracksByAlbum(library)

	totalGroups := len(groups)
	setITunesImportTotal(status, totalGroups)

	_ = progress.Log("info", fmt.Sprintf("Found %d audiobook albums to import (from grouped tracks)", totalGroups), nil)
	if totalGroups == 0 {
		_ = progress.UpdateProgress(0, 0, "No audiobooks found")
		operations.ClearState(store, opID)
		return nil
	}

	importMode := resolveITunesImportMode(req.ImportMode)
	importOpts := itunes.ImportOptions{
		LibraryPath:  req.LibraryPath,
		PathMappings: req.PathMappings,
	}

	// Phase 2: Create one book per album group
	processed := 0
	for i, group := range groups {
		// Skip already-processed groups on resume
		if i < resumeIndex {
			processed++
			continue
		}
		if progress.IsCanceled() {
			_ = progress.Log("info", "iTunes import canceled", nil)
			return nil
		}

		processed++
		updateITunesProcessed(status, processed)

		book, err := buildBookFromAlbumGroup(group, req.LibraryPath, importOpts)
		if err != nil {
			recordITunesFailure(status, err.Error())
			_ = progress.Log("error", err.Error(), nil)
			updateITunesProgress(progress, status, processed, totalGroups, group.key)
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

		// Hash the first track file for dedup (use actual file, not directory)
		hash, err := scanner.ComputeFileHash(firstTrackPath)
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
				updateITunesProgress(progress, status, processed, totalGroups, book.Title)
				continue
			}
		}

		if req.SkipDuplicates {
			if existing, err := database.GlobalStore.GetBookByFilePath(book.FilePath); err == nil && existing != nil {
				updateITunesSkipped(status)
				_ = progress.Log("info", fmt.Sprintf("Skipping duplicate file path: %s", book.FilePath), nil)
				updateITunesProgress(progress, status, processed, totalGroups, book.Title)
				continue
			}
			if book.FileHash != nil {
				if existing, err := database.GlobalStore.GetBookByFileHash(*book.FileHash); err == nil && existing != nil {
					updateITunesSkipped(status)
					_ = progress.Log("info", fmt.Sprintf("Skipping duplicate hash: %s", book.Title), nil)
					updateITunesProgress(progress, status, processed, totalGroups)
					continue
				}
			}
		}

		book.LibraryState = stringPtr(importLibraryState(importMode))

		// Try to extract embedded cover art from first track's actual file
		coverPath, coverErr := metadata.ExtractCoverArt(firstTrackPath)
		if coverErr == nil && coverPath != "" {
			coverFilename := filepath.Base(coverPath)
			book.CoverURL = stringPtr("/api/v1/covers/local/" + coverFilename)
		}

		created, err := database.GlobalStore.CreateBook(book)
		if err != nil {
			recordITunesFailure(status, fmt.Sprintf("Failed to save '%s': %v", book.Title, err))
			_ = progress.Log("error", fmt.Sprintf("Failed to save '%s': %v", book.Title, err), nil)
			updateITunesProgress(progress, status, processed, totalGroups)
			continue
		}

		updateITunesImported(status)

		// Create BookSegments for multi-track albums
		if len(group.tracks) > 1 {
			bookNumericID := int(crc32.ChecksumIEEE([]byte(created.ID)))
			for _, track := range group.tracks {
				trackLoc := importOpts.RemapPath(track.Location)
				trackPath, decErr := itunes.DecodeLocation(trackLoc)
				if decErr != nil {
					continue
				}
				trackFormat := strings.TrimPrefix(strings.ToLower(filepath.Ext(trackPath)), ".")
				trackNum := track.TrackNumber
				totalTracks := len(group.tracks)
				segment := &database.BookSegment{
					FilePath:    trackPath,
					Format:      trackFormat,
					SizeBytes:   track.Size,
					DurationSec: int(track.TotalTime / 1000),
					TrackNumber: &trackNum,
					TotalTracks: &totalTracks,
					Active:      true,
				}
				if _, segErr := database.GlobalStore.CreateBookSegment(bookNumericID, segment); segErr != nil {
					_ = progress.Log("warn", fmt.Sprintf("Failed to create segment for track %d of '%s': %v", track.TrackNumber, book.Title, segErr), nil)
				}
			}
		}

		// Populate book_authors junction table
		if created.AuthorID != nil {
			_ = database.GlobalStore.SetBookAuthors(created.ID, []database.BookAuthor{
				{BookID: created.ID, AuthorID: *created.AuthorID, Role: "author", Position: 0},
			})
		}

		if req.ImportPlaylists {
			// Use first track for playlist tag extraction
			tags := itunes.ExtractPlaylistTags(group.tracks[0].TrackID, library.Playlists)
			if len(tags) > 0 {
				_ = progress.Log("info", fmt.Sprintf("Playlist tags for '%s': %s", book.Title, strings.Join(tags, ", ")), nil)
			}
		}

		updateITunesProgress(progress, status, processed, totalGroups, book.Title)

		// Checkpoint every 10 groups
		if processed%10 == 0 {
			_ = operations.SaveCheckpoint(store, opID, "itunes_import", "importing", processed, totalGroups)
		}
	}

	// Phase 3: Metadata enrichment (if requested) — runs before organize
	// so that author/title are accurate for folder structure
	if req.FetchMetadata {
		_ = operations.SaveCheckpoint(store, opID, "itunes_import", "enriching", 0, 0)
		_ = progress.Log("info", "Starting metadata enrichment phase...", nil)
		enrichITunesImportedBooks(progress, status)
	}

	// Phase 4: Organize (if requested) — runs after enrichment
	if importMode == itunes.ImportModeOrganize && !req.PreserveLocation {
		_ = operations.SaveCheckpoint(store, opID, "itunes_import", "organizing", 0, 0)
		_ = progress.Log("info", "Starting organize phase...", nil)
		organizeImportedBooks(progress, status)
	}

	// Clear checkpoint on successful completion
	_ = operations.ClearState(store, opID)

	// Save library fingerprint for change detection
	if fp, err := itunes.ComputeFingerprint(req.LibraryPath); err == nil {
		_ = store.SaveLibraryFingerprint(fp.Path, fp.Size, fp.ModTime, fp.CRC32)
	}

	summary := buildITunesSummary(status)
	_ = progress.UpdateProgress(totalGroups, totalGroups, summary)
	_ = progress.Log("info", summary, nil)
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
func enrichITunesImportedBooks(progress operations.ProgressReporter, status *itunesImportStatus) {
	mfs := NewMetadataFetchService(database.GlobalStore)

	// Get all imported books (library_state = 'imported')
	books, err := database.GlobalStore.GetAllBooks(10000, 0)
	if err != nil {
		_ = progress.Log("error", fmt.Sprintf("Failed to list books for enrichment: %v", err), nil)
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
			_ = progress.Log("debug", fmt.Sprintf("No metadata found for '%s': %v", book.Title, err), nil)
			consecutiveErrors++
			// Back off if we're hitting rate limits (many consecutive failures)
			if consecutiveErrors >= 5 {
				_ = progress.Log("info", "Rate limit detected, pausing 10s...", nil)
				time.Sleep(10 * time.Second)
				consecutiveErrors = 0
			}
			continue
		}

		consecutiveErrors = 0
		enriched++
		if resp.Book != nil && resp.Book.AuthorID != nil {
			_ = database.GlobalStore.SetBookAuthors(book.ID, []database.BookAuthor{
				{BookID: book.ID, AuthorID: *resp.Book.AuthorID, Role: "author", Position: 0},
			})
		}

		// Rate limit: pause every 10 enrichments to avoid hammering external APIs
		if enriched%10 == 0 {
			_ = progress.Log("info", fmt.Sprintf("Enriched %d books so far (processing %d/%d)...", enriched, i+1, len(books)), nil)
			time.Sleep(2 * time.Second)
		}
	}

	_ = progress.Log("info", fmt.Sprintf("Metadata enrichment complete: %d books enriched", enriched), nil)
}

// organizeImportedBooks moves all imported books into the organized folder structure.
// Runs as a separate phase after metadata enrichment so author/title are accurate.
func organizeImportedBooks(progress operations.ProgressReporter, status *itunesImportStatus) {
	books, err := database.GlobalStore.GetAllBooks(100000, 0)
	if err != nil {
		_ = progress.Log("error", fmt.Sprintf("Failed to list books for organize: %v", err), nil)
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

		if err := organizeImportedBook(book, progress); err != nil {
			recordITunesFailure(status, fmt.Sprintf("Failed to organize '%s': %v", book.Title, err))
			_ = progress.Log("warn", fmt.Sprintf("Failed to organize '%s': %v", book.Title, err), nil)
		} else {
			book.LibraryState = stringPtr("organized")
			if _, err := database.GlobalStore.UpdateBook(book.ID, book); err != nil {
				_ = progress.Log("warn", fmt.Sprintf("Failed to update organized path for '%s': %v", book.Title, err), nil)
			} else {
				organized++
			}
		}
	}

	_ = progress.Log("info", fmt.Sprintf("Organize phase complete: %d books organized", organized), nil)
}

// buildBookFromAlbumGroup creates a single Book from a group of tracks
// that belong to the same album. For single-track groups, it behaves
// like the old buildBookFromTrack. For multi-track groups, it uses the
// album name as the title and sums durations/sizes.
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
	if firstTrack.Comments != "" {
		book.Edition = stringPtr(firstTrack.Comments)
	}
	if totalSize > 0 {
		book.FileSize = &totalSize
	}

	if len(group.tracks) > 1 {
		log.Printf("iTunes import: grouped %d tracks into album %q", len(group.tracks), title)
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

	// Find common prefix
	common := paths[0]
	for _, p := range paths[1:] {
		for !strings.HasPrefix(p, common) {
			common = filepath.Dir(common)
			if common == "/" || common == "." {
				return common
			}
		}
	}
	return common
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

func snapshotITunesImportStatus(opID string) *itunesImportStatus {
	status := loadITunesImportStatus(opID)
	status.mu.Lock()
	defer status.mu.Unlock()

	snapshot := &itunesImportStatus{
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

func updateITunesProgress(progress operations.ProgressReporter, status *itunesImportStatus, processed, total int, currentTitle ...string) {
	status.mu.Lock()
	current := status.Processed
	imported := status.Imported
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
		"Book %d of %d (imported %d, skipped %d, failed %d)",
		current,
		total,
		imported,
		skipped,
		failed,
	)
	if title != "" {
		message += fmt.Sprintf(" — %s", title)
	}
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	pathMappings := req.PathMappings
	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return executeITunesSync(ctx, progress, libraryPath, pathMappings)
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "itunes_sync", operations.PriorityNormal, operationFunc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
func executeITunesSync(ctx context.Context, progress operations.ProgressReporter, libraryPath string, pathMappings []itunes.PathMapping) error {
	store := database.GlobalStore

	_ = progress.UpdateProgress(0, 0, "Starting iTunes sync")
	_ = progress.Log("info", "Starting iTunes sync", nil)

	library, err := itunes.ParseLibrary(libraryPath)
	if err != nil {
		return fmt.Errorf("failed to parse library: %w", err)
	}

	groups := groupTracksByAlbum(library)
	totalGroups := len(groups)
	if totalGroups == 0 {
		_ = progress.UpdateProgress(0, 0, "No audiobooks found in library")
		return nil
	}

	importOpts := itunes.ImportOptions{
		LibraryPath:  libraryPath,
		PathMappings: pathMappings,
	}

	var updated, newBooks, unchanged int
	for i, group := range groups {
		if progress.IsCanceled() {
			_ = progress.Log("info", "iTunes sync canceled", nil)
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

		existing, err := store.GetBookByITunesPersistentID(persistentID)
		if err != nil {
			_ = progress.Log("warn", fmt.Sprintf("Error looking up persistent ID %s: %v", persistentID, err), nil)
			continue
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

			if changed {
				if _, err := store.UpdateBook(existing.ID, existing); err != nil {
					_ = progress.Log("error", fmt.Sprintf("Failed to update '%s': %v", existing.Title, err), nil)
				} else {
					updated++
				}
			} else {
				unchanged++
			}
		} else {
			// Import as new book
			book, err := buildBookFromAlbumGroup(group, libraryPath, importOpts)
			if err != nil {
				_ = progress.Log("warn", fmt.Sprintf("Failed to build book from group '%s': %v", group.key, err), nil)
				continue
			}
			assignAuthorAndSeries(book, firstTrack)
			book.LibraryState = stringPtr("imported")

			if _, err := store.CreateBook(book); err != nil {
				_ = progress.Log("error", fmt.Sprintf("Failed to create '%s': %v", book.Title, err), nil)
			} else {
				newBooks++
			}
		}

		processed := i + 1
		if processed%itunesImportProgressBatch == 0 || processed == totalGroups {
			message := fmt.Sprintf("Syncing book %d of %d (updated %d, new %d, unchanged %d)",
				processed, totalGroups, updated, newBooks, unchanged)
			_ = progress.UpdateProgress(processed, totalGroups, message)
		}
	}

	// Save fingerprint after sync
	if fp, err := itunes.ComputeFingerprint(libraryPath); err == nil {
		_ = store.SaveLibraryFingerprint(fp.Path, fp.Size, fp.ModTime, fp.CRC32)
	}

	summary := fmt.Sprintf("Sync completed: %d updated, %d new, %d unchanged", updated, newBooks, unchanged)
	_ = progress.UpdateProgress(totalGroups, totalGroups, summary)
	_ = progress.Log("info", summary, nil)
	_ = ctx
	return nil
}
