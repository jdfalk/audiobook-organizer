// file: internal/server/maintenance_itunes.go
// version: 1.0.0
// guid: ed5e1965-36b4-4eac-adef-eec61111d9a0
// last-edited: 2026-05-01

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/deluge"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	ulid "github.com/oklog/ulid/v2"
)

// libraryStateFixResult describes one book that was (or would be) fixed.
type libraryStateFixResult struct {
	BookID       string `json:"book_id"`
	Title        string `json:"title"`
	OldState     string `json:"old_state"`
	NewState     string `json:"new_state"`
	VersionGroup string `json:"version_group"`
	IsPrimary    bool   `json:"is_primary"`
	Applied      bool   `json:"applied"`
	Error        string `json:"error,omitempty"`
}

// handleFixLibraryStates fixes library_state for books that have organized versions.
// Books with library_state = 'imported' AND version_group_id set AND is_primary_version = false
// should have library_state = 'organized_source'.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update the database
func (s *Server) handleFixLibraryStates(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Fetch all books (non-deleted). With ~11K books this is fine.
	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	var results []libraryStateFixResult
	fixCount := 0
	skipCount := 0
	errorCount := 0

	for i := range allBooks {
		book := &allBooks[i]

		// Look for books with library_state = 'imported'
		if book.LibraryState == nil || *book.LibraryState != "imported" {
			skipCount++
			continue
		}

		// Must have a version_group_id
		if book.VersionGroupID == nil || *book.VersionGroupID == "" {
			skipCount++
			continue
		}

		// Must NOT be a primary version
		if book.IsPrimaryVersion == nil || *book.IsPrimaryVersion {
			skipCount++
			continue
		}

		// This book qualifies for fixing: organized source version in imported state
		result := libraryStateFixResult{
			BookID:       book.ID,
			Title:        book.Title,
			OldState:     "imported",
			NewState:     "organized_source",
			VersionGroup: *book.VersionGroupID,
			IsPrimary:    false,
			Applied:      !dryRun,
		}

		if !dryRun {
			// Update the book
			newState := "organized_source"
			book.LibraryState = &newState
			if _, updateErr := store.UpdateBook(book.ID, book); updateErr != nil {
				result.Error = updateErr.Error()
				errorCount++
			} else {
				fixCount++
			}
		} else {
			fixCount++
		}

		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run": dryRun,
		"fixed":   fixCount,
		"skipped": skipCount,
		"errors":  errorCount,
		"results": results,
	})
}

// recomputeITunesPathResult describes one book_file that was (or would be) fixed.
type recomputeITunesPathResult struct {
	BookFileID    string `json:"book_file_id"`
	BookID        string `json:"book_id"`
	FilePath      string `json:"file_path"`
	OldITunesPath string `json:"old_itunes_path"`
	NewITunesPath string `json:"new_itunes_path"`
	Applied       bool   `json:"applied"`
	Error         string `json:"error,omitempty"`
}

// handleRecomputeITunesPaths iterates all book_files on PRIMARY books and
// recomputes itunes_path from file_path whenever they differ.  Books whose
// file_path lives under the audiobook-organizer root but whose itunes_path
// still points at the old iTunes location (e.g. W:/itunes/…) are the primary
// target, but the handler fixes any book_file where the recomputed value
// differs from the stored value.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update the database
func (s *Server) handleRecomputeITunesPaths(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	var (
		fixCount   int
		skipCount  int
		errorCount int
		results    []recomputeITunesPathResult
	)

	for i := range allBooks {
		book := &allBooks[i]
		// Only consider PRIMARY books; originals/non-primaries are not the
		// organized copies and should not have their itunes_path changed here.
		if book.IsPrimaryVersion == nil || !*book.IsPrimaryVersion {
			continue
		}

		bookFiles, bfErr := store.GetBookFiles(book.ID)
		if bfErr != nil || len(bookFiles) == 0 {
			continue
		}

		for _, bf := range bookFiles {
			if bf.FilePath == "" {
				skipCount++
				continue
			}

			want := metafetch.ComputeITunesPath(bf.FilePath)
			if bf.ITunesPath == want {
				skipCount++
				continue
			}

			result := recomputeITunesPathResult{
				BookFileID:    bf.ID,
				BookID:        book.ID,
				FilePath:      bf.FilePath,
				OldITunesPath: bf.ITunesPath,
				NewITunesPath: want,
			}

			if !dryRun {
				bf.ITunesPath = want
				if updateErr := store.UpdateBookFile(bf.ID, &bf); updateErr != nil {
					result.Error = updateErr.Error()
					errorCount++
				} else {
					result.Applied = true
					fixCount++
				}
			} else {
				fixCount++
			}

			results = append(results, result)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run": dryRun,
		"fixed":   fixCount,
		"skipped": skipCount,
		"errors":  errorCount,
		"results": results,
	})
}

// handleGenerateITLTests generates a suite of .itl test files for iTunes testing.
func (s *Server) handleGenerateITLTests(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	outputDir := config.AppConfig.RootDir + "/.itunes-writeback/tests"

	// Wipe existing test data so we get a clean slate
	if err := os.RemoveAll(outputDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to clean output dir: %v", err)})
		return
	}

	// Gather all books and book_files for the full-library test case
	allBooks, err := store.GetAllBooks(100000, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to fetch books: %v", err)})
		return
	}

	var allBookFiles []database.BookFile
	for _, b := range allBooks {
		files, _ := store.GetBookFiles(b.ID)
		allBookFiles = append(allBookFiles, files...)
	}

	if err := itunes.GenerateTestITLSuite(outputDir, allBooks, allBookFiles); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to generate test suite: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"output_dir": outputDir,
		"books":      len(allBooks),
		"book_files": len(allBookFiles),
		"message":    fmt.Sprintf("Generated ITL test suite in %s with %d books and %d book_files", outputDir, len(allBooks), len(allBookFiles)),
	})
}

// backupCleanupResult summarizes a cleanup-backups run.
type backupCleanupResult struct {
	DryRun       bool     `json:"dry_run"`
	RootDir      string   `json:"root_dir"`
	FilesFound   int      `json:"files_found"`
	FilesRemoved int      `json:"files_removed"`
	BytesFreed   int64    `json:"bytes_freed"`
	Errors       []string `json:"errors,omitempty"`
}

// handleCleanupBackups sweeps the library for stale tag-write backup files
// and deletes them. Two patterns are matched:
//
//  1. `*.backup` and `*.backup.*.backup` — created by the older
//     fileops.FileOperation.Execute() path, which retains 5 per file but
//     never garbage-collects when a file stops being written.
//  2. `*.bak-YYYYMMDD-HHMMSS` — created by the write-back path in
//     metadata_fetch_service.backupFileBeforeWrite. That function is now
//     gated on the WriteBackupBeforeTagWrite config flag (default off)
//     so new backups stop accumulating, but the historical pile (tens of
//     thousands of files, multi-TB apparent size) still needs sweeping.
//
// Protected paths:
//   - Every directory whose name starts with `.` is skipped via
//     filepath.SkipDir. This covers the iTunes writeback folder
//     (.itunes-writeback) and the cover dedup store (.covers).
//   - The iTunes Media tree outside the managed library is not walked
//     because we only scan under config.AppConfig.RootDir.
//
// Query params:
//   - dry_run=true  (default) — report what would be removed
//   - dry_run=false — actually delete
func (s *Server) handleCleanupBackups(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") == "true"
	rootDir := config.AppConfig.RootDir
	if rootDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "root_dir is not configured"})
		return
	}
	if _, err := os.Stat(rootDir); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("root_dir not accessible: %v", err)})
		return
	}

	result := backupCleanupResult{
		DryRun:  dryRun,
		RootDir: rootDir,
	}

	// Regex matches a timestamped .bak-YYYYMMDD-HHMMSS suffix anywhere in
	// the filename. Anchored at end-of-string so it doesn't accidentally
	// eat filenames that happen to contain `.bak-1` earlier.
	bakTimestampRe := regexp.MustCompile(`\.bak-[0-9]{8}-[0-9]{6}$`)

	walkErr := filepath.Walk(rootDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			// Non-fatal, keep going — record and continue.
			result.Errors = append(result.Errors, fmt.Sprintf("walk %q: %v", path, walkErr))
			return nil
		}
		if info.IsDir() {
			// Skip any hidden directory. This intentionally catches
			// .itunes-writeback, .covers, and any other dotfolder a user
			// might add later — explicit deny-list is fragile, prefix
			// check is robust. The root itself never starts with `.`
			// so we don't have to guard against skipping it.
			if path != rootDir && strings.HasPrefix(filepath.Base(path), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		name := filepath.Base(path)
		isBackupCopy := strings.HasSuffix(name, ".backup")
		isBakTimestamp := bakTimestampRe.MatchString(name)
		if !isBackupCopy && !isBakTimestamp {
			return nil
		}

		result.FilesFound++
		size := info.Size()

		if dryRun {
			result.BytesFreed += size
			return nil
		}

		if removeErr := os.Remove(path); removeErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("remove %q: %v", path, removeErr))
			log.Printf("[WARN] cleanup-backups: failed to remove %q: %v", path, removeErr)
			return nil
		}
		result.FilesRemoved++
		result.BytesFreed += size
		return nil
	})
	if walkErr != nil {
		internalError(c, "failed to walk root directory", walkErr)
		return
	}

	log.Printf("[INFO] cleanup-backups: dry_run=%v found=%d removed=%d bytes=%d errors=%d",
		dryRun, result.FilesFound, result.FilesRemoved, result.BytesFreed, len(result.Errors))

	c.JSON(http.StatusOK, gin.H{
		"dry_run":       result.DryRun,
		"root_dir":      result.RootDir,
		"files_found":   result.FilesFound,
		"files_removed": result.FilesRemoved,
		"bytes_freed":   result.BytesFreed,
		"human_freed":   humanizeBytes(result.BytesFreed),
		"errors":        result.Errors,
	})
}

// composerTagResult describes the COMPOSER field state for one audio file.
type composerTagResult struct {
	BookID    string `json:"book_id"`
	BookTitle string `json:"book_title"`
	FilePath  string `json:"file_path"`
	// Category is one of: "ok", "composer_equals_author", "composer_equals_narrator",
	// "composer_mismatch", "missing_narrator", "read_error".
	Category  string `json:"category"`
	Composer  string `json:"composer_on_disk"`
	Author    string `json:"author,omitempty"`
	Narrator  string `json:"narrator,omitempty"`
	WillWrite string `json:"will_write,omitempty"`
	Applied   bool   `json:"applied,omitempty"`
	Error     string `json:"error,omitempty"`
}

// categorizeComposer returns the problem category and the value that should
// be written in the given fix_mode ("set_narrator" or "clear").
func categorizeComposer(composer, author, narrator, fixMode string) (category, willWrite string) {
	composerLower := strings.ToLower(strings.TrimSpace(composer))
	authorLower := strings.ToLower(strings.TrimSpace(author))
	narratorLower := strings.ToLower(strings.TrimSpace(narrator))

	if fixMode == "set_narrator" {
		willWrite = strings.TrimSpace(narrator)
	} else {
		willWrite = ""
	}

	if strings.TrimSpace(composer) == "" {
		if fixMode == "set_narrator" && strings.TrimSpace(narrator) != "" {
			return "missing_narrator", strings.TrimSpace(narrator)
		}
		return "ok", ""
	}

	if author != "" && composerLower == authorLower {
		// Old wrong mapping: author ended up in COMPOSER.
		return "composer_equals_author", willWrite
	}
	if narrator != "" && composerLower == narratorLower {
		if fixMode == "set_narrator" {
			return "ok", strings.TrimSpace(narrator) // already correct
		}
		return "composer_equals_narrator", ""
	}
	// Non-empty COMPOSER that matches neither author nor narrator.
	return "composer_mismatch", willWrite
}

// composerScanWork is one unit of work dispatched to the parallel reader pool.
type composerScanWork struct {
	bookID    string
	bookTitle string
	filePath  string
	author    string
	narrator  string
}

// handleScanComposerTags starts an async, resumable COMPOSER-tag scan as a
// queued operation and returns the operation ID immediately (HTTP 202).
//
// The operation bulk-loads all books/authors/files, fans out tag reads across
// 8 goroutines, persists per-file results to the OperationResult table, and
// survives server restarts — on startup resumeInterruptedOperations() picks up
// any interrupted composer_tag_scan and re-enqueues from where it left off.
//
// Query params:
//   - dry_run=true (default) — scan and report without writing
//   - dry_run=false — apply the fix to problematic files
//   - fix_mode=set_narrator (default) — write COMPOSER=narrator; "clear" to always empty it
//
// Poll progress via GET /api/v1/operations/{id}.
// View results via GET /api/v1/maintenance/scan-composer-tags/{id}.
func (s *Server) handleScanComposerTags(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"
	fixMode := c.DefaultQuery("fix_mode", "set_narrator")
	if fixMode != "set_narrator" && fixMode != "clear" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fix_mode must be 'set_narrator' or 'clear'"})
		return
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	opID := ulid.Make().String()
	if _, err := store.CreateOperation(opID, "composer_tag_scan", nil); err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	params := operations.ComposerScanParams{DryRun: dryRun, FixMode: fixMode}
	if err := operations.SaveParams(store, opID, params); err != nil {
		log.Printf("[WARN] scan-composer-tags: failed to save params for %s: %v", opID, err)
	}

	capturedOpID := opID
	capturedParams := params
	opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.runComposerTagScan(ctx, capturedOpID, capturedParams, store, progress)
	}

	if err := s.queue.Enqueue(opID, "composer_tag_scan", operations.PriorityNormal, opFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	log.Printf("[INFO] scan-composer-tags: queued operation %s dry_run=%v fix_mode=%s", opID, dryRun, fixMode)

	c.JSON(http.StatusAccepted, gin.H{
		"operation_id": opID,
		"message":      "composer tag scan started — poll GET /api/v1/operations/" + opID + " for progress",
		"dry_run":      dryRun,
		"fix_mode":     fixMode,
	})
}

// runComposerTagScan is the resumable core of the composer-tag scan. It is
// called both on first run (from handleScanComposerTags) and on resume (from
// resumeInterruptedOperations). Already-processed files are skipped by
// checking existing OperationResult rows, making the function idempotent.
func (s *Server) runComposerTagScan(
	ctx context.Context,
	opID string,
	params operations.ComposerScanParams,
	store database.Store,
	progress operations.ProgressReporter,
) error {
	_ = progress.UpdateProgress(0, 0, "loading library data")

	// --- Bulk load (eliminates N+1 DB queries) ---
	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		return fmt.Errorf("GetAllBooks: %w", err)
	}
	allAuthors, err := store.GetAllAuthors()
	if err != nil {
		return fmt.Errorf("GetAllAuthors: %w", err)
	}
	authorByID := make(map[int]string, len(allAuthors))
	for _, a := range allAuthors {
		authorByID[a.ID] = a.Name
	}
	allFiles, err := store.GetAllBookFiles()
	if err != nil {
		return fmt.Errorf("GetAllBookFiles: %w", err)
	}
	filesByBook := make(map[string][]database.BookFile, len(allFiles))
	for i := range allFiles {
		f := &allFiles[i]
		filesByBook[f.BookID] = append(filesByBook[f.BookID], *f)
	}

	// Load already-processed file paths from a previous (interrupted) run.
	existingResults, _ := store.GetOperationResults(opID)
	done := make(map[string]bool, len(existingResults))
	for _, r := range existingResults {
		done[r.BookID] = true // BookID field stores the file path for this operation
	}

	// Build work queue, skipping already-processed files.
	audioExts := map[string]bool{".m4b": true, ".m4a": true, ".mp3": true, ".flac": true, ".ogg": true}
	var workItems []composerScanWork
	for i := range allBooks {
		b := &allBooks[i]
		author := ""
		if b.AuthorID != nil {
			author = authorByID[*b.AuthorID]
		}
		narrator := ""
		if b.Narrator != nil {
			narrator = *b.Narrator
		}
		for _, f := range filesByBook[b.ID] {
			if f.FilePath == "" || f.Missing {
				continue
			}
			if !audioExts[strings.ToLower(filepath.Ext(f.FilePath))] {
				continue
			}
			if done[f.FilePath] {
				continue // already processed in a previous run
			}
			workItems = append(workItems, composerScanWork{
				bookID:    b.ID,
				bookTitle: b.Title,
				filePath:  f.FilePath,
				author:    author,
				narrator:  narrator,
			})
		}
	}

	totalFiles := len(existingResults) + len(workItems)
	alreadyDone := len(existingResults)
	log.Printf("[INFO] scan-composer-tags %s: %d files total, %d already done, %d to process",
		opID, totalFiles, alreadyDone, len(workItems))
	_ = progress.UpdateProgress(alreadyDone, totalFiles,
		fmt.Sprintf("resuming: %d/%d already processed", alreadyDone, totalFiles))

	if len(workItems) == 0 {
		_ = progress.UpdateProgress(totalFiles, totalFiles, "all files already processed")
		return nil
	}

	// --- Parallel NAS reads ---
	const workers = 8
	workCh := make(chan composerScanWork, len(workItems))
	for _, w := range workItems {
		workCh <- w
	}
	close(workCh)

	var completed int64 = int64(alreadyDone)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range workCh {
				if ctx.Err() != nil {
					return
				}
				if _, statErr := os.Stat(w.filePath); statErr != nil {
					// File missing on disk — record as skipped so it's not retried
					_ = store.CreateOperationResult(&database.OperationResult{
						OperationID: opID,
						BookID:      w.filePath,
						ResultJSON:  `{"category":"missing"}`,
						Status:      "missing",
					})
					atomic.AddInt64(&completed, 1)
					continue
				}

				tags, readErr := metadata.ReadRawTags(w.filePath)
				var r composerTagResult
				if readErr != nil {
					r = composerTagResult{
						BookID: w.bookID, BookTitle: w.bookTitle, FilePath: w.filePath,
						Category: "read_error", Error: readErr.Error(),
					}
				} else {
					composer := ""
					if vs, ok := tags["COMPOSER"]; ok && len(vs) > 0 {
						composer = strings.TrimSpace(vs[0])
					}
					category, willWrite := categorizeComposer(composer, w.author, w.narrator, params.FixMode)
					r = composerTagResult{
						BookID: w.bookID, BookTitle: w.bookTitle, FilePath: w.filePath,
						Category: category, Composer: composer,
						Author: w.author, Narrator: w.narrator, WillWrite: willWrite,
					}
					if !params.DryRun && category != "ok" && willWrite != composer {
						if writeErr := metadata.WriteSingleTag(w.filePath, "COMPOSER", willWrite); writeErr != nil {
							r.Error = writeErr.Error()
							log.Printf("[WARN] scan-composer-tags %s: write failed %s: %v", opID, w.filePath, writeErr)
						} else {
							r.Applied = true
							log.Printf("[INFO] scan-composer-tags %s: COMPOSER %q→%q %s", opID, composer, willWrite, w.filePath)
						}
					}
				}

				resultJSON, _ := json.Marshal(r)
				_ = store.CreateOperationResult(&database.OperationResult{
					OperationID: opID,
					BookID:      w.filePath, // file path as unique key per file
					ResultJSON:  string(resultJSON),
					Status:      r.Category,
				})

				n := atomic.AddInt64(&completed, 1)
				mu.Lock()
				_ = progress.UpdateProgress(int(n), totalFiles,
					fmt.Sprintf("scanned %d/%d files", n, totalFiles))
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	finalCount := atomic.LoadInt64(&completed)
	_ = progress.UpdateProgress(int(finalCount), totalFiles, "scan complete")
	log.Printf("[INFO] scan-composer-tags %s: finished %d/%d files", opID, finalCount, totalFiles)
	return nil
}

// handleGetComposerScanResults returns the aggregated results for a completed
// (or in-progress) composer_tag_scan operation.
func (s *Server) handleGetComposerScanResults(c *gin.Context) {
	opID := c.Param("id")
	if opID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "operation id required"})
		return
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	op, err := store.GetOperationByID(opID)
	if err != nil || op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}
	if op.Type != "composer_tag_scan" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not a composer_tag_scan operation"})
		return
	}

	rawResults, err := store.GetOperationResults(opID)
	if err != nil {
		internalError(c, "failed to load results", err)
		return
	}

	counts := map[string]int{}
	var problems []composerTagResult
	for _, raw := range rawResults {
		var r composerTagResult
		if err := json.Unmarshal([]byte(raw.ResultJSON), &r); err != nil {
			continue
		}
		counts[r.Category]++
		if r.Category != "ok" && r.Category != "missing" {
			problems = append(problems, r)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"operation_id": opID,
		"status":       op.Status,
		"progress":     op.Progress,
		"total":        op.Total,
		"by_category":  counts,
		"problems":     len(problems),
		"details":      problems,
	})
}

// humanizeBytes turns a byte count into a short "1.23 GB" style string.
func humanizeBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ---------------------------------------------------------------------------
// Relink missing organizer books to iTunes source files
// ---------------------------------------------------------------------------

type relinkMissingResult struct {
	BookID     string   `json:"book_id"`
	Title      string   `json:"title"`
	OldPath    string   `json:"old_path"`
	NewPath    string   `json:"new_path,omitempty"`
	Action     string   `json:"action"` // "relinked", "unresolved", "ambiguous"
	Matches    int      `json:"matches,omitempty"`
	MatchPaths []string `json:"match_paths,omitempty"`
	Applied    bool     `json:"applied"`
	Error      string   `json:"error,omitempty"`
}

// handleRelinkMissingToiTunes finds books whose file_path is under the organizer
// root but no longer exists on disk, then searches the iTunes media folder for
// the original source file by author+title and relinks the DB records to it.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update book and book_files rows
//   - itunes_root   — override config.ITunesMediaRoot for this call
func (s *Server) handleRelinkMissingToiTunes(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"
	iTunesRoot := c.DefaultQuery("itunes_root", config.AppConfig.ITunesMediaRoot)
	organizerRoot := config.AppConfig.RootDir

	if iTunesRoot == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "itunes_media_root not configured; pass ?itunes_root=<path> or set itunes_media_root in settings"})
		return
	}
	if organizerRoot == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "root_dir not configured"})
		return
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	audioExts := map[string]bool{".mp3": true, ".m4b": true, ".m4a": true, ".flac": true, ".opus": true, ".ogg": true}

	// findInITunes searches iTunesRoot for iTunes album directories (or single
	// audio files) matching the given author + title. Results are deduplicated
	// by album directory so a 10-track book returns exactly one match, not 10.
	findInITunes := func(authorName, title string) []string {
		// 25-char prefix keeps enough specificity while accommodating iTunes
		// filename truncation (many files are cut off before 40 chars).
		titlePrefix := title
		if len(titlePrefix) > 25 {
			titlePrefix = titlePrefix[:25]
		}
		titlePrefixLower := strings.ToLower(titlePrefix)

		// First significant word of author for loose directory matching.
		authorWord := authorName
		if idx := strings.Index(authorName, " "); idx > 0 {
			authorWord = authorName[:idx]
		}
		authorWordLower := strings.ToLower(authorWord)

		// dirMatches collects unique iTunes album dirs (or single files).
		dirMatches := map[string]struct{}{}

		entries, err := os.ReadDir(iTunesRoot)
		if err != nil {
			return nil
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if !strings.Contains(strings.ToLower(entry.Name()), authorWordLower) {
				continue
			}
			authorDir := filepath.Join(iTunesRoot, entry.Name())

			albumEntries, err := os.ReadDir(authorDir)
			if err != nil {
				continue
			}
			for _, album := range albumEntries {
				albumPath := filepath.Join(authorDir, album.Name())
				if album.IsDir() {
					// Match on album dir name first (fast path).
					if strings.Contains(strings.ToLower(album.Name()), titlePrefixLower) {
						dirMatches[albumPath] = struct{}{}
						continue
					}
					// Fall back: scan files inside the album dir.
					_ = filepath.WalkDir(albumPath, func(path string, d os.DirEntry, err error) error {
						if err != nil || d.IsDir() {
							return nil
						}
						if !audioExts[strings.ToLower(filepath.Ext(path))] {
							return nil
						}
						if strings.Contains(strings.ToLower(filepath.Base(path)), titlePrefixLower) {
							dirMatches[albumPath] = struct{}{}
							return filepath.SkipDir
						}
						return nil
					})
				} else {
					// Single audio file directly under the author dir.
					if !audioExts[strings.ToLower(filepath.Ext(albumPath))] {
						continue
					}
					if strings.Contains(strings.ToLower(album.Name()), titlePrefixLower) {
						dirMatches[albumPath] = struct{}{}
					}
				}
			}
		}

		result := make([]string, 0, len(dirMatches))
		for d := range dirMatches {
			result = append(result, d)
		}
		sort.Strings(result)
		return result
	}

	// leadingNumRE strips leading track numbers like "01 ", "01 - ", "12 " from
	// iTunes filenames before comparing them to the book title.
	leadingNumRE := regexp.MustCompile(`^\d+\s*[-.]?\s*`)

	trailingNumRE := regexp.MustCompile(`\s+\d+$`)

	// disambiguate narrows multiple iTunes matches to a single best match using
	// a scoring heuristic. Returns "" if still ambiguous after scoring.
	disambiguate := func(matches []string, authorName, title string) string {
		titleLower := strings.ToLower(title)

		type candidate struct {
			path  string
			score int
		}
		cands := make([]candidate, 0, len(matches))

		for _, p := range matches {
			base := filepath.Base(p)
			ext := filepath.Ext(base)
			stemRaw := strings.TrimSuffix(base, ext)
			leadingNum := leadingNumRE.FindString(stemRaw)
			stemNoNum := leadingNumRE.ReplaceAllString(stemRaw, "")
			stemLower := strings.ToLower(stemNoNum)
			// Normalize underscores/colons for comparison.
			stemNorm := strings.ReplaceAll(strings.ReplaceAll(stemLower, "_", " "), ":", " ")

			sc := 0

			switch {
			case stemNorm == titleLower:
				// Perfect stem match.
				sc += 100

			case strings.HasPrefix(stemNorm, titleLower):
				// Title is a prefix of the stem — check the trailing rest.
				rest := stemNorm[len(titleLower):] // intentionally NOT TrimSpace
				switch {
				case regexp.MustCompile(`^\s+book\s+\d`).MatchString(rest),
					regexp.MustCompile(`^\s+\d+$`).MatchString(rest):
					// Trailing "book N" or bare " 2" → likely a sequel.
					sc += 20
				default:
					// Subtitle / series tag after the title — acceptable.
					sc += 60
				}

			case strings.HasPrefix(titleLower, stemNorm) && len(stemNorm) >= 10:
				// The stem is a prefix of the title: iTunes truncated the filename
				// mid-word. Only credit this if the match is long enough (≥10 chars)
				// to avoid false positives.
				sc += 80

			case strings.Contains(stemNorm, titleLower):
				sc += 10
			}

			// Penalize stems that end with a plain number: likely "part 1" / "part 2".
			if trailingNumRE.MatchString(stemNorm) {
				sc -= 30
			}

			// Prefer files without a leading track number — they are usually the
			// "album" file, not an individual track.
			if leadingNum == "" {
				sc += 20
			} else {
				// Among tracked files, lower track numbers are preferred.
				// Use integer value so "01" beats "12" by a small margin.
				if n, err := strconv.Atoi(strings.TrimSpace(
					strings.TrimRight(strings.TrimRight(leadingNum, " "), "-."))); err == nil {
					sc -= n * 2
				}
			}

			// Prefer the author dir that best matches the book's stored author.
			authorDir := filepath.Base(filepath.Dir(p))
			if strings.EqualFold(authorDir, authorName) {
				sc += 40 // exact author dir match
			} else if strings.Contains(strings.ToLower(authorDir), strings.ToLower(authorName)) {
				sc += 20 // author name is substring of dir
			}

			// Shorter filenames are less likely to carry extra series/subtitle info.
			sc -= len(base) / 8

			cands = append(cands, candidate{path: p, score: sc})
		}

		sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })

		// If every candidate has the same normalized stem (all are tracks of the
		// same audiobook), pick the one with the lowest track number, which ends
		// up at the top after sorting by score.
		if len(cands) > 1 {
			stemOf := func(p string) string {
				b := filepath.Base(p)
				s := strings.TrimSuffix(b, filepath.Ext(b))
				s = strings.ToLower(leadingNumRE.ReplaceAllString(s, ""))
				s = strings.ReplaceAll(strings.ReplaceAll(s, "_", " "), ":", " ")
				return s
			}
			first := stemOf(cands[0].path)
			allSame := true
			for _, c := range cands[1:] {
				if stemOf(c.path) != first {
					allSame = false
					break
				}
			}
			if allSame {
				return cands[0].path
			}
		}

		// Require a gap of ≥15 before committing to one candidate.
		if len(cands) >= 2 && cands[0].score-cands[1].score >= 15 {
			return cands[0].path
		}
		if len(cands) == 1 {
			return cands[0].path
		}
		return ""
	}

	var results []relinkMissingResult
	relinked, unresolved, ambiguous, skipped := 0, 0, 0, 0

	for i := range allBooks {
		book := &allBooks[i]
		fp := book.FilePath
		if !strings.HasPrefix(fp, organizerRoot) {
			skipped++
			continue
		}
		if _, err := os.Stat(fp); err == nil {
			skipped++
			continue
		}

		// Book path is under organizer root and doesn't exist — candidate.
		// Derive author name from the organizer path (first component after root)
		// so we don't need a DB join. Fall back to DB author lookup if path is
		// ambiguous (e.g. file directly in root).
		rel := strings.TrimPrefix(fp, organizerRoot)
		rel = strings.TrimPrefix(rel, string(os.PathSeparator))
		authorName := strings.SplitN(rel, string(os.PathSeparator), 2)[0]
		if authorName == "" || authorName == filepath.Base(fp) {
			// path is too flat — try DB author
			if book.Author != nil {
				authorName = book.Author.Name
			} else if book.AuthorID != nil {
				if a, err := store.GetAuthorByID(*book.AuthorID); err == nil && a != nil {
					authorName = a.Name
				}
			}
		}
		if authorName == "" {
			results = append(results, relinkMissingResult{
				BookID:  book.ID,
				Title:   book.Title,
				OldPath: fp,
				Action:  "unresolved",
				Error:   "no author name",
			})
			unresolved++
			continue
		}

		matches := findInITunes(authorName, book.Title)

		res := relinkMissingResult{
			BookID:  book.ID,
			Title:   book.Title,
			OldPath: fp,
			Matches: len(matches),
		}

		switch len(matches) {
		case 0:
			res.Action = "unresolved"
			unresolved++
		case 1:
			res.Action = "relinked"
			res.NewPath = matches[0]
			relinked++
			if !dryRun {
				newFP := matches[0]
				fi, _ := os.Stat(newFP)

				// Update book.file_path
				book.FilePath = newFP
				if _, upErr := store.UpdateBook(book.ID, book); upErr != nil {
					res.Error = "UpdateBook: " + upErr.Error()
					res.Action = "unresolved"
					unresolved++
					relinked--
					break
				}

				// Update all book_files that pointed to the old organizer path.
				// newFP may be a directory (multi-file book) or a single audio file.
				bookFiles, bfErr := store.GetBookFiles(book.ID)
				if bfErr == nil {
					for j := range bookFiles {
						bf := &bookFiles[j]
						if !strings.HasPrefix(bf.FilePath, organizerRoot) {
							continue
						}
						bf.FilePath = newFP
						bf.OriginalFilename = filepath.Base(newFP)
						bf.Missing = false
						if fi != nil && !fi.IsDir() {
							bf.FileSize = fi.Size()
							ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(newFP), "."))
							if ext != "" {
								bf.Format = ext
							}
						}
						_ = store.UpdateBookFile(bf.ID, bf)
					}
				}
				res.Applied = true
			}
		default:
			if best := disambiguate(matches, authorName, book.Title); best != "" {
				// Disambiguation picked a winner — treat as single match.
				res.Action = "relinked"
				res.NewPath = best
				res.MatchPaths = matches // keep all matches for auditing
				relinked++
				if !dryRun {
					newFP := best
					fi, _ := os.Stat(newFP)
					book.FilePath = newFP
					if _, upErr := store.UpdateBook(book.ID, book); upErr != nil {
						res.Error = "UpdateBook: " + upErr.Error()
						res.Action = "unresolved"
						unresolved++
						relinked--
						break
					}
					bookFiles, bfErr := store.GetBookFiles(book.ID)
					if bfErr == nil {
						for j := range bookFiles {
							bf := &bookFiles[j]
							if !strings.HasPrefix(bf.FilePath, organizerRoot) {
								continue
							}
							bf.FilePath = newFP
							bf.OriginalFilename = filepath.Base(newFP)
							bf.Missing = false
							if fi != nil && !fi.IsDir() {
								bf.FileSize = fi.Size()
								ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(newFP), "."))
								if ext != "" {
									bf.Format = ext
								}
							}
							_ = store.UpdateBookFile(bf.ID, bf)
						}
					}
					res.Applied = true
				}
			} else {
				res.Action = "ambiguous"
				res.MatchPaths = matches
				ambiguous++
			}
		}

		results = append(results, res)
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":    dryRun,
		"relinked":   relinked,
		"ambiguous":  ambiguous,
		"unresolved": unresolved,
		"skipped":    skipped,
		"results":    results,
	})
}

// ---------------------------------------------------------------------------
// Async resumable missing-file path repair
// ---------------------------------------------------------------------------

// bookFileMeta is a lightweight holder used by runMissingFileRepair to pass
// title and author to the per-file worker without keeping all Book fields alive.
type bookFileMeta struct {
	title  string
	author string
}

type missingFileRepairResult struct {
	FileID  string `json:"file_id"`
	BookID  string `json:"book_id"`
	Title   string `json:"book_title"`
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path,omitempty"`
	// Method values: "pid", "filename", "truncation", "author_title",
	// "skipped", "unresolved", "ambiguous"
	Method  string `json:"method"`
	Matches int    `json:"matches,omitempty"`
	Applied bool   `json:"applied"`
	Error   string `json:"error,omitempty"`
}

// handleRepairMissingFiles starts an async, resumable missing-file path-repair
// operation. For each book_file row whose stored path doesn't exist on disk it
// tries four escalating strategies — PID lookup, exact filename, stem
// truncation, author+title walk — and on a confident single match updates only
// the file_path field of the existing record.
//
// Never creates new Book or BookFile rows, so the dedup pipeline is never
// triggered.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update book_file rows
//
// Poll progress: GET /api/v1/operations/{id}
// View results:  GET /api/v1/maintenance/repair-missing-files/{id}
func (s *Server) handleRepairMissingFiles(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	roots := []string{config.AppConfig.ITunesMediaRoot, config.AppConfig.RootDir}
	var searchRoots []string
	for _, r := range roots {
		if r != "" {
			searchRoots = append(searchRoots, r)
		}
	}

	opID := ulid.Make().String()
	if _, err := store.CreateOperation(opID, "missing-file-repair", nil); err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	params := operations.MissingFileRepairParams{DryRun: dryRun, SearchRoots: searchRoots}
	if err := operations.SaveParams(store, opID, params); err != nil {
		log.Printf("[WARN] repair-missing-files: failed to save params for %s: %v", opID, err)
	}

	capturedOpID := opID
	capturedParams := params
	opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.runMissingFileRepair(ctx, capturedOpID, capturedParams, store, progress)
	}
	if err := s.queue.Enqueue(opID, "missing-file-repair", operations.PriorityNormal, opFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	log.Printf("[INFO] repair-missing-files: queued %s dry_run=%v roots=%v", opID, dryRun, searchRoots)
	c.JSON(http.StatusAccepted, gin.H{
		"operation_id": opID,
		"message":      "missing file repair started — poll GET /api/v1/operations/" + opID,
		"dry_run":      dryRun,
		"search_roots": searchRoots,
	})
}

// runMissingFileRepair is the resumable core. Idempotent: files already
// processed in a prior run are detected via existing OperationResult rows
// (keyed by book_file ID) and skipped.
func (s *Server) runMissingFileRepair(
	ctx context.Context,
	opID string,
	params operations.MissingFileRepairParams,
	store database.Store,
	progress operations.ProgressReporter,
) error {
	_ = progress.UpdateProgress(0, 0, "loading library data")

	allFiles, err := store.GetAllBookFiles()
	if err != nil {
		return fmt.Errorf("GetAllBookFiles: %w", err)
	}

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		return fmt.Errorf("GetAllBooks: %w", err)
	}
	allAuthors, err := store.GetAllAuthors()
	if err != nil {
		return fmt.Errorf("GetAllAuthors: %w", err)
	}
	authorByID := make(map[int]string, len(allAuthors))
	for _, a := range allAuthors {
		authorByID[a.ID] = a.Name
	}
	metaByBook := make(map[string]bookFileMeta, len(allBooks))
	for i := range allBooks {
		b := &allBooks[i]
		author := ""
		if b.AuthorID != nil {
			author = authorByID[*b.AuthorID]
		}
		metaByBook[b.ID] = bookFileMeta{title: b.Title, author: author}
	}

	// Collect candidates: files the DB thinks exist but os.Stat disagrees.
	var candidates []database.BookFile
	for i := range allFiles {
		f := &allFiles[i]
		if f.FilePath == "" || f.Missing {
			continue
		}
		if _, statErr := os.Stat(f.FilePath); statErr == nil {
			continue
		}
		candidates = append(candidates, *f)
	}

	// Skip files already processed in a prior run.
	existingResults, _ := store.GetOperationResults(opID)
	done := make(map[string]bool, len(existingResults))
	for _, r := range existingResults {
		done[r.BookID] = true // BookID stores file_id for this operation
	}
	var work []database.BookFile
	for _, f := range candidates {
		if !done[f.ID] {
			work = append(work, f)
		}
	}

	totalFiles := len(existingResults) + len(work)
	alreadyDone := len(existingResults)
	log.Printf("[INFO] repair-missing-files %s: %d candidates, %d already done, %d to process",
		opID, totalFiles, alreadyDone, len(work))
	_ = progress.UpdateProgress(alreadyDone, totalFiles,
		fmt.Sprintf("resuming: %d/%d already processed", alreadyDone, totalFiles))

	if len(work) == 0 {
		_ = progress.UpdateProgress(totalFiles, totalFiles, "all files already processed")
		return nil
	}

	// Parse iTunes XML once for PID-based lookups.
	pidToLocation := make(map[string]string)
	if xmlPath := config.AppConfig.ITunesLibraryReadPath; xmlPath != "" {
		if lib, parseErr := itunes.ParseLibrary(xmlPath); parseErr != nil {
			log.Printf("[WARN] repair-missing-files %s: iTunes XML parse error: %v", opID, parseErr)
		} else {
			for _, track := range lib.Tracks {
				if track.PersistentID != "" && track.Location != "" {
					pidToLocation[track.PersistentID] = track.Location
				}
			}
			log.Printf("[INFO] repair-missing-files %s: loaded %d PID→location entries", opID, len(pidToLocation))
		}
	}

	itunesOpts := itunes.ImportOptions{PathMappings: make([]itunes.PathMapping, len(config.AppConfig.ITunesPathMappings))}
	for i, m := range config.AppConfig.ITunesPathMappings {
		itunesOpts.PathMappings[i] = itunes.PathMapping{From: m.From, To: m.To}
	}

	audioExts := map[string]bool{".m4b": true, ".m4a": true, ".mp3": true, ".flac": true, ".ogg": true, ".opus": true}

	// Build a basename→paths filename index across all search roots (once, lazily).
	var filenameIdx map[string][]string
	var idxOnce sync.Once
	buildIdx := func() {
		idxOnce.Do(func() {
			_ = progress.UpdateProgress(alreadyDone, totalFiles, "building filename index…")
			idx := make(map[string][]string, 200000)
			for _, root := range params.SearchRoots {
				_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
					if walkErr != nil || d.IsDir() {
						return nil
					}
					if audioExts[strings.ToLower(filepath.Ext(path))] {
						base := filepath.Base(path)
						idx[base] = append(idx[base], path)
					}
					return nil
				})
			}
			filenameIdx = idx
			log.Printf("[INFO] repair-missing-files %s: filename index built (%d unique names)", opID, len(idx))
		})
	}

	var completed int64 = int64(alreadyDone)
	var mu sync.Mutex

	workCh := make(chan database.BookFile, len(work))
	for _, f := range work {
		workCh <- f
	}
	close(workCh)

	var wg sync.WaitGroup
	const workers = 4
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range workCh {
				if ctx.Err() != nil {
					return
				}
				res := s.repairOneMissingFile(f, metaByBook, pidToLocation, itunesOpts,
					params, audioExts, buildIdx, func() map[string][]string {
						mu.Lock()
						defer mu.Unlock()
						return filenameIdx
					}, store, opID)

				resultJSON, _ := json.Marshal(res)
				_ = store.CreateOperationResult(&database.OperationResult{
					OperationID: opID,
					BookID:      f.ID,
					ResultJSON:  string(resultJSON),
					Status:      res.Method,
				})
				n := atomic.AddInt64(&completed, 1)
				mu.Lock()
				_ = progress.UpdateProgress(int(n), totalFiles, fmt.Sprintf("processed %d/%d", n, totalFiles))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	finalCount := atomic.LoadInt64(&completed)
	activity.FlushOperation(s.activityWriter, opID)
	msg := fmt.Sprintf("Repaired %d of %d missing files", finalCount, totalFiles)
	_ = progress.UpdateProgress(int(finalCount), totalFiles, msg)
	log.Printf("[INFO] repair-missing-files %s: finished %d/%d files", opID, finalCount, totalFiles)
	activity.EmitInfo(s.activityWriter, opID, "missing-file-repair", "repair-missing-files", msg,
		activity.TagsIf(finalCount == 0, activity.NoOpTag)...)
	return nil
}

// repairOneMissingFile tries four strategies in order and returns a result.
// It only calls UpdateBookFile — never CreateBook or CreateBookFile.
func (s *Server) repairOneMissingFile(
	f database.BookFile,
	metaByBook map[string]bookFileMeta,
	pidToLocation map[string]string,
	itunesOpts itunes.ImportOptions,
	params operations.MissingFileRepairParams,
	audioExts map[string]bool,
	buildIdx func(),
	getIdx func() map[string][]string,
	store database.Store,
	opID string,
) missingFileRepairResult {
	bm := metaByBook[f.BookID]
	res := missingFileRepairResult{
		FileID:  f.ID,
		BookID:  f.BookID,
		Title:   bm.title,
		OldPath: f.FilePath,
	}

	// Re-check: another goroutine or prior session may have fixed it.
	if _, statErr := os.Stat(f.FilePath); statErr == nil {
		res.Method = "skipped"
		return res
	}

	candidate, method := "", ""

	// Tier 1: iTunes PID → XML Location → RemapPath
	if candidate == "" && f.ITunesPersistentID != "" {
		if loc, ok := pidToLocation[f.ITunesPersistentID]; ok {
			remapped := itunesOpts.RemapPath(loc)
			if remapped != "" && remapped != loc {
				if _, statErr := os.Stat(remapped); statErr == nil {
					candidate, method = remapped, "pid"
				}
			}
		}
	}

	// Tier 2: exact basename search across filename index
	if candidate == "" {
		buildIdx()
		base := filepath.Base(f.FilePath)
		idx := getIdx()
		paths := idx[base]
		switch len(paths) {
		case 1:
			candidate, method = paths[0], "filename"
			res.Matches = 1
		case 0:
			// no match
		default:
			// Multiple — narrow by parent dir name (album folder)
			parentDir := filepath.Base(filepath.Dir(f.FilePath))
			var narrowed []string
			for _, p := range paths {
				if strings.EqualFold(filepath.Base(filepath.Dir(p)), parentDir) {
					narrowed = append(narrowed, p)
				}
			}
			// If still multiple, narrow by grandparent dir containing author's last name.
			// iTunes multi-author dirs ("Amy DuBoff, Michael Anderle") contain the stored
			// author ("Michael Anderle") as a substring; this resolves those cases.
			if len(narrowed) > 1 && bm.author != "" {
				lastName := strings.ToLower(bm.author)
				if i := strings.LastIndex(lastName, " "); i > 0 {
					lastName = lastName[i+1:]
				}
				var n2 []string
				for _, p := range narrowed {
					if strings.Contains(strings.ToLower(filepath.Base(filepath.Dir(filepath.Dir(p)))), lastName) {
						n2 = append(n2, p)
					}
				}
				if len(n2) >= 1 {
					narrowed = n2
				}
			}
			switch len(narrowed) {
			case 1:
				candidate, method = narrowed[0], "filename"
				res.Matches = 1
			case 0:
				// Parent-dir narrowing eliminated all candidates — file likely moved to a
				// different parent dir. Fall through to Tier 3/4 for broader search.
			default:
				res.Method = "ambiguous"
				res.Matches = len(narrowed)
				return res
			}
		}
	}

	// Tier 3: stem-prefix match in the same directory (truncated filename)
	if candidate == "" {
		dir := filepath.Dir(f.FilePath)
		base := filepath.Base(f.FilePath)
		ext := filepath.Ext(base)
		stem := strings.TrimSuffix(base, ext)
		if entries, readErr := os.ReadDir(dir); readErr == nil {
			for _, de := range entries {
				if de.IsDir() {
					continue
				}
				name := de.Name()
				nameExt := filepath.Ext(name)
				nameStem := strings.TrimSuffix(name, nameExt)
				if strings.EqualFold(nameExt, ext) &&
					strings.HasPrefix(nameStem, stem) &&
					name != base &&
					len(nameStem) > len(stem) &&
					nameStem[len(stem)] != ' ' {
					candidate, method = filepath.Join(dir, name), "truncation"
					break
				}
			}
		}
	}

	// Tier 4: author last-name + title-prefixed album dir, then stored basename.
	// Uses the author's last name so it matches both "Michael Anderle" and
	// "Amy DuBoff, Michael Anderle" directories. Matches album dirs whose name
	// starts with the title prefix, then looks for the stored filename within
	// that album dir (avoids false ambiguity from multiple tracks per album).
	if candidate == "" && bm.author != "" && bm.title != "" {
		lastName := bm.author
		if i := strings.LastIndex(bm.author, " "); i > 0 {
			lastName = bm.author[i+1:]
		}
		titlePrefix := bm.title
		if len(titlePrefix) > 30 {
			titlePrefix = titlePrefix[:30]
		}
		storedBase := filepath.Base(f.FilePath)
		var matches []string
		for _, root := range params.SearchRoots {
			entries, rerr := os.ReadDir(root)
			if rerr != nil {
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				if !strings.Contains(strings.ToLower(entry.Name()), strings.ToLower(lastName)) {
					continue
				}
				authorDir := filepath.Join(root, entry.Name())
				albumEntries, aErr := os.ReadDir(authorDir)
				if aErr != nil {
					continue
				}
				for _, album := range albumEntries {
					if !album.IsDir() {
						continue
					}
					if !strings.HasPrefix(strings.ToLower(album.Name()), strings.ToLower(titlePrefix)) {
						continue
					}
					// Prefer the exact stored basename within this album dir.
					exact := filepath.Join(authorDir, album.Name(), storedBase)
					if _, statErr := os.Stat(exact); statErr == nil {
						matches = append(matches, exact)
						continue
					}
					// Fall back: any audio file in the album dir (single-track books).
					albumFiles, _ := os.ReadDir(filepath.Join(authorDir, album.Name()))
					var audioInAlbum []string
					for _, af := range albumFiles {
						if !af.IsDir() && audioExts[strings.ToLower(filepath.Ext(af.Name()))] {
							audioInAlbum = append(audioInAlbum, filepath.Join(authorDir, album.Name(), af.Name()))
						}
					}
					if len(audioInAlbum) == 1 {
						matches = append(matches, audioInAlbum[0])
					}
				}
			}
		}
		switch len(matches) {
		case 1:
			candidate, method = matches[0], "author_title"
			res.Matches = 1
		case 0:
			// no match
		default:
			res.Method = "ambiguous"
			res.Matches = len(matches)
			return res
		}
	}

	// Tier 4b: flat iTunes library — M4B files directly in the author dir (no album subdir).
	// iTunes sometimes consolidates individual MP3 tracks into a single M4B per book, stored
	// flat under the author dir. The stored basename looks like "01 Defending the Lost.mp3";
	// after stripping the leading track number we get "Defending the Lost", which we match
	// against stems of audio files directly under any co-author dir containing the last name.
	if candidate == "" && bm.author != "" {
		lastName := bm.author
		if i := strings.LastIndex(bm.author, " "); i > 0 {
			lastName = bm.author[i+1:]
		}
		storedBase := filepath.Base(f.FilePath)
		storedStem := strings.TrimSuffix(storedBase, filepath.Ext(storedBase))
		// Strip leading "NN " or "NN. " track-number prefix.
		titleFromFile := storedStem
		if i := strings.IndexByte(storedStem, ' '); i > 0 {
			prefix := storedStem[:i]
			isNum := true
			for _, r := range prefix {
				if r < '0' || r > '9' {
					isNum = false
					break
				}
			}
			if isNum {
				titleFromFile = strings.TrimSpace(storedStem[i+1:])
			}
		}

		var matches []string
		for _, root := range params.SearchRoots {
			entries, rerr := os.ReadDir(root)
			if rerr != nil {
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				if !strings.Contains(strings.ToLower(entry.Name()), strings.ToLower(lastName)) {
					continue
				}
				authorDir := filepath.Join(root, entry.Name())
				dirFiles, _ := os.ReadDir(authorDir)
				for _, df := range dirFiles {
					if df.IsDir() || !audioExts[strings.ToLower(filepath.Ext(df.Name()))] {
						continue
					}
					fileStem := strings.TrimSuffix(df.Name(), filepath.Ext(df.Name()))
					if strings.EqualFold(fileStem, titleFromFile) {
						matches = append(matches, filepath.Join(authorDir, df.Name()))
					}
				}
			}
		}
		// Among multiple matches, prefer dirs whose name starts with the stored author
		// (i.e. "Michael Anderle, Justin Sloan" over "Amy DuBoff, Michael Anderle").
		if len(matches) > 1 {
			authorLower := strings.ToLower(bm.author)
			var preferred []string
			for _, m := range matches {
				dirName := strings.ToLower(filepath.Base(filepath.Dir(m)))
				if strings.HasPrefix(dirName, authorLower) {
					preferred = append(preferred, m)
				}
			}
			if len(preferred) == 1 {
				matches = preferred
			}
		}
		switch len(matches) {
		case 1:
			candidate, method = matches[0], "flat_stem"
			res.Matches = 1
		case 0:
			// no match
		default:
			res.Method = "ambiguous"
			res.Matches = len(matches)
			return res
		}
	}

	if candidate == "" {
		res.Method = "unresolved"
		return res
	}

	res.NewPath = candidate
	res.Method = method
	res.Matches = 1

	if params.DryRun {
		return res
	}

	fi, _ := os.Stat(candidate)
	f.FilePath = candidate
	f.OriginalFilename = filepath.Base(candidate)
	f.Missing = false
	if fi != nil {
		f.FileSize = fi.Size()
	}
	if ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(candidate), ".")); ext != "" {
		f.Format = ext
	}
	if upErr := store.UpdateBookFile(f.ID, &f); upErr != nil {
		res.Error = upErr.Error()
		log.Printf("[WARN] repair-missing-files %s: UpdateBookFile %s: %v", opID, f.ID, upErr)
	} else {
		res.Applied = true
		activity.LogBatch(s.activityWriter, opID, "missing-file-repair", "repair-missing-files",
			activity.BatchItem{Name: filepath.Base(res.OldPath), Detail: method + ": " + candidate})
	}
	return res
}

// handleGetMissingFileRepairResults returns aggregated results for a
// missing_file_repair operation (in-progress or completed).
func (s *Server) handleGetMissingFileRepairResults(c *gin.Context) {
	opID := c.Param("id")
	if opID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "operation id required"})
		return
	}
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	op, err := store.GetOperationByID(opID)
	if err != nil || op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}
	if op.Type != "missing-file-repair" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not a missing-file-repair operation"})
		return
	}
	rawResults, err := store.GetOperationResults(opID)
	if err != nil {
		internalError(c, "failed to load results", err)
		return
	}

	byMethod := map[string]int{}
	var problems []missingFileRepairResult
	repaired, unresolved, ambiguous, skipped := 0, 0, 0, 0
	for _, raw := range rawResults {
		var r missingFileRepairResult
		if jsonErr := json.Unmarshal([]byte(raw.ResultJSON), &r); jsonErr != nil {
			continue
		}
		byMethod[r.Method]++
		switch r.Method {
		case "unresolved":
			unresolved++
			problems = append(problems, r)
		case "ambiguous":
			ambiguous++
			problems = append(problems, r)
		case "skipped":
			skipped++
		default:
			repaired++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"operation_id": opID,
		"status":       op.Status,
		"progress":     op.Progress,
		"total":        op.Total,
		"by_method":    byMethod,
		"repaired":     repaired,
		"unresolved":   unresolved,
		"ambiguous":    ambiguous,
		"skipped":      skipped,
		"problems":     problems,
	})
}

// handleRevertMetadataFetch rolls back all DB changes made by one or more
// bulk_metadata_fetch operations. It reads the OperationResult rows to find
// which books were updated, then restores PreviousValue for every
// ChangeType=fetched MetadataChangeRecord recorded after the operation started.
//
// POST /api/v1/maintenance/revert-metadata-fetch
// Body: {"operation_ids": ["01K...", "01K..."]}
func (s *Server) handleRevertMetadataFetch(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	var req struct {
		OperationIDs []string `json:"operation_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.OperationIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "operation_ids required"})
		return
	}

	// Collect the earliest start time across all operations so we only revert
	// changes that were made by this run (not older fetched records).
	var revertAfter time.Time
	bookIDSet := map[string]bool{}

	for _, opID := range req.OperationIDs {
		op, err := store.GetOperationByID(opID)
		if err != nil || op == nil {
			continue
		}
		if op.Type != "bulk_metadata_fetch" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "operation " + opID + " is not a bulk_metadata_fetch"})
			return
		}
		ts := op.CreatedAt
		if op.StartedAt != nil {
			ts = *op.StartedAt
		}
		if revertAfter.IsZero() || ts.Before(revertAfter) {
			revertAfter = ts
		}

		results, err := store.GetOperationResults(opID)
		if err != nil {
			internalError(c, "failed to load results for "+opID, err)
			return
		}
		for _, r := range results {
			if r.Status == "updated" {
				bookIDSet[r.BookID] = true
			}
		}
	}

	log.Printf("[INFO] revert-metadata-fetch: reverting %d books, changes after %s",
		len(bookIDSet), revertAfter.Format(time.RFC3339))

	reverted := 0
	skipped := 0
	errors := 0

	for bookID := range bookIDSet {
		book, err := store.GetBookByID(bookID)
		if err != nil || book == nil {
			errors++
			continue
		}

		history, err := store.GetBookChangeHistory(bookID, 50)
		if err != nil {
			errors++
			continue
		}

		// Gather the most recent fetched change per field after revertAfter.
		// We want the PreviousValue (what the field was before the fetch ran).
		type revertEntry struct {
			field string
			prev  string // empty string means "was empty before"
		}
		// Use a map so we only take the LAST change per field (most recent op).
		byField := map[string]revertEntry{}
		for _, h := range history {
			if h.ChangeType != "fetched" {
				continue
			}
			if h.ChangedAt.Before(revertAfter) {
				continue
			}
			prev := ""
			if h.PreviousValue != nil {
				// PreviousValue is JSON-encoded string: "\"foo\"" → foo
				if err := json.Unmarshal([]byte(*h.PreviousValue), &prev); err != nil {
					prev = *h.PreviousValue
				}
			}
			byField[h.Field] = revertEntry{field: h.Field, prev: prev}
		}

		if len(byField) == 0 {
			skipped++
			continue
		}

		didChange := false
		for _, e := range byField {
			switch e.field {
			case "title":
				book.Title = e.prev
				didChange = true
			case "author_name":
				if e.prev == "" {
					book.AuthorID = nil
				} else {
					if author, aerr := store.GetAuthorByName(e.prev); aerr == nil && author != nil {
						book.AuthorID = &author.ID
						didChange = true
					}
				}
			case "publisher":
				if e.prev == "" {
					book.Publisher = nil
				} else {
					book.Publisher = &e.prev
				}
				didChange = true
			case "language":
				if e.prev == "" {
					book.Language = nil
				} else {
					book.Language = &e.prev
				}
				didChange = true
			case "audiobook_release_year":
				if e.prev == "" {
					book.AudiobookReleaseYear = nil
				} else if yr, yerr := strconv.Atoi(e.prev); yerr == nil {
					book.AudiobookReleaseYear = &yr
				}
				didChange = true
			case "isbn10":
				if e.prev == "" {
					book.ISBN10 = nil
				} else {
					book.ISBN10 = &e.prev
				}
				didChange = true
			case "isbn13":
				if e.prev == "" {
					book.ISBN13 = nil
				} else {
					book.ISBN13 = &e.prev
				}
				didChange = true
			}
		}

		if didChange {
			if _, uerr := store.UpdateBook(bookID, book); uerr != nil {
				log.Printf("[WARN] revert-metadata-fetch: UpdateBook %s: %v", bookID, uerr)
				errors++
			} else {
				reverted++
			}
		} else {
			skipped++
		}
	}

	log.Printf("[INFO] revert-metadata-fetch: done — reverted:%d skipped:%d errors:%d", reverted, skipped, errors)
	c.JSON(http.StatusOK, gin.H{
		"reverted": reverted,
		"skipped":  skipped,
		"errors":   errors,
		"total":    len(bookIDSet),
	})
}

// durationMismatchResult describes one book whose Audible runtime diverges
// significantly from the local file duration.
type durationMismatchResult struct {
	BookID            string `json:"book_id"`
	Title             string `json:"title"`
	ASIN              string `json:"asin,omitempty"`
	FileDurationSec   int    `json:"file_duration_sec"`
	AudibleRuntimeMin int    `json:"audible_runtime_min"`
	AudibleRuntimeSec int    `json:"audible_runtime_sec"`
	DeltaSec          int    `json:"delta_sec"`
}

// handleScanDurationMismatch scans all books that have both a local file
// duration and a stored Audible runtime, and returns those whose delta
// exceeds the configured threshold.
//
// Query params:
//   - max_delta_min=10  (integer, default 10) — threshold in minutes
//
// GET /api/v1/maintenance/scan-duration-mismatch
func (s *Server) handleScanDurationMismatch(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Parse threshold (minutes → seconds). Default = 10 min.
	thresholdMin := 10
	if raw := c.Query("max_delta_min"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			thresholdMin = v
		}
	}
	thresholdSec := thresholdMin * 60

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	var mismatches []durationMismatchResult
	scanned := 0

	for i := range allBooks {
		book := &allBooks[i]
		if book.Duration == nil || *book.Duration <= 0 {
			continue
		}
		if book.AudibleRuntimeMin == nil || *book.AudibleRuntimeMin <= 0 {
			continue
		}
		scanned++

		fileDurSec := *book.Duration
		audibleSec := *book.AudibleRuntimeMin * 60
		delta := fileDurSec - audibleSec
		if delta < 0 {
			delta = -delta
		}
		if delta <= thresholdSec {
			continue
		}

		asin := ""
		if book.ASIN != nil {
			asin = *book.ASIN
		}
		mismatches = append(mismatches, durationMismatchResult{
			BookID:            book.ID,
			Title:             book.Title,
			ASIN:              asin,
			FileDurationSec:   fileDurSec,
			AudibleRuntimeMin: *book.AudibleRuntimeMin,
			AudibleRuntimeSec: audibleSec,
			DeltaSec:          delta,
		})
	}

	// Sort by largest delta first so the worst mismatches appear at the top.
	sort.Slice(mismatches, func(i, j int) bool {
		return mismatches[i].DeltaSec > mismatches[j].DeltaSec
	})

	log.Printf("[INFO] scan-duration-mismatch: scanned=%d threshold=%dmin mismatches=%d",
		scanned, thresholdMin, len(mismatches))

	c.JSON(http.StatusOK, gin.H{
		"threshold_min":  thresholdMin,
		"scanned":        scanned,
		"mismatch_count": len(mismatches),
		"mismatches":     mismatches,
	})
}

// ---------------------------------------------------------------------------
// RELINK-4: dry-run relink report (read-only triage endpoint)
// ---------------------------------------------------------------------------

// relinkReportResolved is a single successfully-resolvable book entry returned
// by handleRelinkReport.
type relinkReportResolved struct {
	BookID  string `json:"book_id"`
	Title   string `json:"title"`
	NewPath string `json:"new_path"`
}

// relinkReportUnresolved is a single unresolvable book entry returned by
// handleRelinkReport, annotated with the reason it could not be relinked.
type relinkReportUnresolved struct {
	BookID        string   `json:"book_id"`
	Title         string   `json:"title"`
	OldPath       string   `json:"old_path"`
	WhyUnresolved string   `json:"why_unresolved"`
	MatchPaths    []string `json:"match_paths,omitempty"` // present when action=="ambiguous"
}

// handleRelinkReport re-runs the relink dry-run logic over ALL books and
// returns which ones would be successfully relinked vs. those that remain
// unresolved (with a why_unresolved annotation for triage).
//
// This endpoint is purely read-only — it never modifies the database.
//
// Query params:
//   - limit=N   (integer, default 0 = all)  — page size
//   - offset=N  (integer, default 0)        — page offset (into unresolved list)
//
// GET /api/v1/maintenance/relink-report
func (s *Server) handleRelinkReport(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	iTunesRoot := c.DefaultQuery("itunes_root", config.AppConfig.ITunesMediaRoot)
	organizerRoot := config.AppConfig.RootDir

	if iTunesRoot == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "itunes_media_root not configured; pass ?itunes_root=<path> or set itunes_media_root in settings"})
		return
	}
	if organizerRoot == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "root_dir not configured"})
		return
	}

	limit := 0
	if raw := c.Query("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}
	offset := 0
	if raw := c.Query("offset"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
			offset = v
		}
	}

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	audioExts := map[string]bool{".mp3": true, ".m4b": true, ".m4a": true, ".flac": true, ".opus": true, ".ogg": true}

	// findInITunes is identical to the one in handleRelinkMissingToiTunes.
	findInITunes := func(authorName, title string) []string {
		titlePrefix := title
		if len(titlePrefix) > 25 {
			titlePrefix = titlePrefix[:25]
		}
		titlePrefixLower := strings.ToLower(titlePrefix)
		authorWord := authorName
		if idx := strings.Index(authorName, " "); idx > 0 {
			authorWord = authorName[:idx]
		}
		authorWordLower := strings.ToLower(authorWord)

		dirMatches := map[string]struct{}{}
		entries, err := os.ReadDir(iTunesRoot)
		if err != nil {
			return nil
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if !strings.Contains(strings.ToLower(entry.Name()), authorWordLower) {
				continue
			}
			authorDir := filepath.Join(iTunesRoot, entry.Name())
			albumEntries, err := os.ReadDir(authorDir)
			if err != nil {
				continue
			}
			for _, album := range albumEntries {
				albumPath := filepath.Join(authorDir, album.Name())
				if album.IsDir() {
					if strings.Contains(strings.ToLower(album.Name()), titlePrefixLower) {
						dirMatches[albumPath] = struct{}{}
						continue
					}
					_ = filepath.WalkDir(albumPath, func(path string, d os.DirEntry, err error) error {
						if err != nil || d.IsDir() {
							return nil
						}
						if !audioExts[strings.ToLower(filepath.Ext(path))] {
							return nil
						}
						if strings.Contains(strings.ToLower(filepath.Base(path)), titlePrefixLower) {
							dirMatches[albumPath] = struct{}{}
							return filepath.SkipDir
						}
						return nil
					})
				} else {
					if !audioExts[strings.ToLower(filepath.Ext(albumPath))] {
						continue
					}
					if strings.Contains(strings.ToLower(album.Name()), titlePrefixLower) {
						dirMatches[albumPath] = struct{}{}
					}
				}
			}
		}

		result := make([]string, 0, len(dirMatches))
		for d := range dirMatches {
			result = append(result, d)
		}
		sort.Strings(result)
		return result
	}

	leadingNumRE := regexp.MustCompile(`^\d+\s*[-.]?\s*`)
	trailingNumRE := regexp.MustCompile(`\s+\d+$`)

	disambiguate := func(matches []string, authorName, title string) string {
		titleLower := strings.ToLower(title)
		type candidate struct {
			path  string
			score int
		}
		cands := make([]candidate, 0, len(matches))
		for _, p := range matches {
			base := filepath.Base(p)
			ext := filepath.Ext(base)
			stemRaw := strings.TrimSuffix(base, ext)
			leadingNum := leadingNumRE.FindString(stemRaw)
			stemNoNum := leadingNumRE.ReplaceAllString(stemRaw, "")
			stemLower := strings.ToLower(stemNoNum)
			stemNorm := strings.ReplaceAll(strings.ReplaceAll(stemLower, "_", " "), ":", " ")
			sc := 0
			switch {
			case stemNorm == titleLower:
				sc += 100
			case strings.HasPrefix(stemNorm, titleLower):
				rest := stemNorm[len(titleLower):]
				switch {
				case regexp.MustCompile(`^\s+book\s+\d`).MatchString(rest),
					regexp.MustCompile(`^\s+\d+$`).MatchString(rest):
					sc += 20
				default:
					sc += 60
				}
			case strings.HasPrefix(titleLower, stemNorm) && len(stemNorm) >= 10:
				sc += 80
			case strings.Contains(stemNorm, titleLower):
				sc += 10
			}
			if trailingNumRE.MatchString(stemNorm) {
				sc -= 30
			}
			if leadingNum == "" {
				sc += 20
			} else {
				if n, err := strconv.Atoi(strings.TrimSpace(
					strings.TrimRight(strings.TrimRight(leadingNum, " "), "-."))); err == nil {
					sc -= n * 2
				}
			}
			authorDir := filepath.Base(filepath.Dir(p))
			if strings.EqualFold(authorDir, authorName) {
				sc += 40
			} else if strings.Contains(strings.ToLower(authorDir), strings.ToLower(authorName)) {
				sc += 20
			}
			sc -= len(base) / 8
			cands = append(cands, candidate{path: p, score: sc})
		}
		sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
		if len(cands) > 1 {
			stemOf := func(p string) string {
				b := filepath.Base(p)
				s := strings.TrimSuffix(b, filepath.Ext(b))
				s = strings.ToLower(leadingNumRE.ReplaceAllString(s, ""))
				s = strings.ReplaceAll(strings.ReplaceAll(s, "_", " "), ":", " ")
				return s
			}
			first := stemOf(cands[0].path)
			allSame := true
			for _, c := range cands[1:] {
				if stemOf(c.path) != first {
					allSame = false
					break
				}
			}
			if allSame {
				return cands[0].path
			}
		}
		if len(cands) >= 2 && cands[0].score-cands[1].score >= 15 {
			return cands[0].path
		}
		if len(cands) == 1 {
			return cands[0].path
		}
		return ""
	}

	var resolved []relinkReportResolved
	var unresolved []relinkReportUnresolved
	skipped := 0

	for i := range allBooks {
		book := &allBooks[i]
		fp := book.FilePath

		// Only consider books whose path is under the organizer root AND missing.
		if !strings.HasPrefix(fp, organizerRoot) {
			skipped++
			continue
		}
		if _, statErr := os.Stat(fp); statErr == nil {
			skipped++
			continue
		}

		// Derive author name the same way handleRelinkMissingToiTunes does.
		rel := strings.TrimPrefix(fp, organizerRoot)
		rel = strings.TrimPrefix(rel, string(os.PathSeparator))
		authorName := strings.SplitN(rel, string(os.PathSeparator), 2)[0]
		if authorName == "" || authorName == filepath.Base(fp) {
			if book.Author != nil {
				authorName = book.Author.Name
			} else if book.AuthorID != nil {
				if a, err := store.GetAuthorByID(*book.AuthorID); err == nil && a != nil {
					authorName = a.Name
				}
			}
		}
		if authorName == "" {
			unresolved = append(unresolved, relinkReportUnresolved{
				BookID:        book.ID,
				Title:         book.Title,
				OldPath:       fp,
				WhyUnresolved: "no author name",
			})
			continue
		}

		matches := findInITunes(authorName, book.Title)

		switch len(matches) {
		case 0:
			unresolved = append(unresolved, relinkReportUnresolved{
				BookID:        book.ID,
				Title:         book.Title,
				OldPath:       fp,
				WhyUnresolved: "no iTunes match found",
			})
		case 1:
			resolved = append(resolved, relinkReportResolved{
				BookID:  book.ID,
				Title:   book.Title,
				NewPath: matches[0],
			})
		default:
			if best := disambiguate(matches, authorName, book.Title); best != "" {
				resolved = append(resolved, relinkReportResolved{
					BookID:  book.ID,
					Title:   book.Title,
					NewPath: best,
				})
			} else {
				unresolved = append(unresolved, relinkReportUnresolved{
					BookID:        book.ID,
					Title:         book.Title,
					OldPath:       fp,
					WhyUnresolved: fmt.Sprintf("ambiguous: %d iTunes matches, none dominant", len(matches)),
					MatchPaths:    matches,
				})
			}
		}
	}

	// Apply pagination to the unresolved list only (resolved list is typically
	// smaller and always useful in full; callers can use offset/limit to page
	// through the unresolved triage queue).
	totalUnresolved := len(unresolved)
	if offset > 0 {
		if offset >= len(unresolved) {
			unresolved = nil
		} else {
			unresolved = unresolved[offset:]
		}
	}
	if limit > 0 && len(unresolved) > limit {
		unresolved = unresolved[:limit]
	}

	log.Printf("[INFO] relink-report: total=%d resolved=%d unresolved=%d skipped=%d (page offset=%d limit=%d)",
		len(allBooks), len(resolved), totalUnresolved, skipped, offset, limit)

	c.JSON(http.StatusOK, gin.H{
		"resolved":         resolved,
		"unresolved":       unresolved,
		"resolved_count":   len(resolved),
		"unresolved_count": totalUnresolved,
		"skipped":          skipped,
		"offset":           offset,
		"limit":            limit,
	})
}

// handleBulkDelugeImport queues a resumable async operation that calls
// importToLibrary for every book_file that has a deluge_hash but has not
// yet been imported (imported_from_deluge_at IS NULL).
//
// Query params:
//   - dry_run=true (default) — report what would be imported without writing
//   - dry_run=false           — actually copy files
//   - max_books=N             — cap the number of files imported per run (0 = unlimited)
//
// POST /api/v1/maintenance/bulk-deluge-import
func (s *Server) handleBulkDelugeImport(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"
	maxBooks := 0
	if v := c.Query("max_books"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxBooks = n
		}
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	client := getDelugeClient()

	opID := ulid.Make().String()
	if _, err := store.CreateOperation(opID, "bulk-deluge-import", nil); err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	params := operations.BulkImportDelugeParams{DryRun: dryRun, MaxBooks: maxBooks}
	if err := operations.SaveParams(store, opID, params); err != nil {
		log.Printf("[WARN] bulk-deluge-import: failed to save params for %s: %v", opID, err)
	}

	capturedOpID := opID
	capturedParams := params
	capturedClient := client
	opFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.runBulkDelugeImport(ctx, capturedOpID, capturedParams, capturedClient, store, progress)
	}
	if err := s.queue.Enqueue(opID, "bulk-deluge-import", operations.PriorityNormal, opFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	log.Printf("[INFO] bulk-deluge-import: queued %s dry_run=%v max_books=%d", opID, dryRun, maxBooks)
	c.JSON(http.StatusAccepted, gin.H{
		"operation_id": opID,
		"message":      "bulk deluge import started — poll GET /api/v1/operations/" + opID,
		"dry_run":      dryRun,
		"max_books":    maxBooks,
	})
}

// runBulkDelugeImport is the resumable core. Files already imported in a
// prior run (imported_from_deluge_at IS NOT NULL) are filtered out by the
// DB query so the operation is inherently idempotent.
func (s *Server) runBulkDelugeImport(
	ctx context.Context,
	opID string,
	params operations.BulkImportDelugeParams,
	client *deluge.Client,
	store database.Store,
	progress operations.ProgressReporter,
) error {
	_ = progress.UpdateProgress(0, 0, "loading pending files")

	pending, err := store.GetBookFilesNeedingDelugeImport()
	if err != nil {
		return fmt.Errorf("GetBookFilesNeedingDelugeImport: %w", err)
	}
	if params.MaxBooks > 0 && len(pending) > params.MaxBooks {
		pending = pending[:params.MaxBooks]
	}

	total := len(pending)
	log.Printf("[INFO] bulk-deluge-import %s: %d files pending (dry_run=%v)", opID, total, params.DryRun)
	_ = progress.UpdateProgress(0, total, fmt.Sprintf("found %d files to import", total))

	imported, failed := 0, 0
	for i := range pending {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		f := &pending[i]
		if params.DryRun {
			resultJSON, _ := json.Marshal(map[string]any{"path": f.FilePath, "action": "dry_run"})
			_ = store.CreateOperationResult(&database.OperationResult{
				OperationID: opID,
				BookID:      f.ID,
				ResultJSON:  string(resultJSON),
				Status:      "dry_run",
			})
			imported++
		} else {
			newPath, importErr := importToLibrary(&config.AppConfig, client, store, f)
			if importErr != nil {
				log.Printf("[WARN] bulk-deluge-import %s: %s: %v", opID, f.FilePath, importErr)
				resultJSON, _ := json.Marshal(map[string]any{"path": f.FilePath, "error": importErr.Error()})
				_ = store.CreateOperationResult(&database.OperationResult{
					OperationID: opID,
					BookID:      f.ID,
					ResultJSON:  string(resultJSON),
					Status:      "error",
				})
				failed++
			} else {
				resultJSON, _ := json.Marshal(map[string]any{"path": f.FilePath, "new_path": newPath})
				_ = store.CreateOperationResult(&database.OperationResult{
					OperationID: opID,
					BookID:      f.ID,
					ResultJSON:  string(resultJSON),
					Status:      "imported",
				})
				imported++
			}
		}
		if (i+1)%100 == 0 || i+1 == total {
			_ = progress.UpdateProgress(i+1, total,
				fmt.Sprintf("imported %d/%d (failed: %d)", imported, total, failed))
		}
	}
	log.Printf("[INFO] bulk-deluge-import %s: done. imported=%d failed=%d", opID, imported, failed)
	return nil
}

// ── MATCH-1: backfill metadata_source_hash ───────────────────────────────────

// metadataHashBackfillResult is one entry in the backfill response.
type metadataHashBackfillResult struct {
	BookID     string `json:"book_id"`
	BookTitle  string `json:"book_title"`
	Hash       string `json:"hash,omitempty"`
	Source     string `json:"source,omitempty"`
	Skipped    bool   `json:"skipped,omitempty"`
	SkipReason string `json:"skip_reason,omitempty"`
	Applied    bool   `json:"applied,omitempty"`
	Error      string `json:"error,omitempty"`
}
