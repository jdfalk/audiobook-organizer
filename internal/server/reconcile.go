// file: internal/server/reconcile.go
// version: 1.7.0
// guid: e7f8a9b0-c1d2-3e4f-5a6b-7c8d9e0f1a2b

package server

import (
	"context"
	"encoding/json"
	"fmt"
	stdlog "log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	"github.com/oklog/ulid/v2"
	"net/http"
)

// ReconcileMatch represents a potential match between a broken DB record and an untracked file.
type ReconcileMatch struct {
	BookID     string  `json:"book_id"`
	BookTitle  string  `json:"book_title"`
	OldPath    string  `json:"old_path"`
	NewPath    string  `json:"new_path"`
	MatchType  string  `json:"match_type"` // "hash", "original_hash", "filename"
	Confidence string  `json:"confidence"` // "high", "medium", "low"
	Score      float64 `json:"score"`
}

// ReconcilePreviewResult is the full preview of what reconciliation would do.
type ReconcilePreviewResult struct {
	BrokenRecords  []ReconcileBrokenRecord `json:"broken_records"`
	UntrackedFiles []string                `json:"untracked_files"`
	Matches        []ReconcileMatch        `json:"matches"`
	UnmatchedBooks []ReconcileBrokenRecord `json:"unmatched_books"`
}

// ReconcileBrokenRecord represents a book whose file_path no longer exists on disk.
type ReconcileBrokenRecord struct {
	BookID   string  `json:"book_id"`
	Title    string  `json:"title"`
	FilePath string  `json:"file_path"`
	FileHash *string `json:"file_hash,omitempty"`
}

// ReconcileApplyRequest specifies which matches to apply.
type ReconcileApplyRequest struct {
	Matches []ReconcileApplyItem `json:"matches"`
}

// ReconcileApplyItem is a single match the user confirmed.
type ReconcileApplyItem struct {
	BookID  string `json:"book_id"`
	NewPath string `json:"new_path"`
}

// ReconcileApplyResult reports what was done.
type ReconcileApplyResult struct {
	Applied int      `json:"applied"`
	Skipped int      `json:"skipped"`
	Errors  []string `json:"errors,omitempty"`
}

// reconcilePreview handles GET /api/v1/operations/reconcile/preview (sync, kept for backward compat)
func (s *Server) reconcilePreview(c *gin.Context) {
	store := database.GlobalStore
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	result, err := buildReconcilePreview(store)
	if err != nil {
		internalError(c, "failed to build reconcile preview", err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// startReconcileScan handles POST /api/v1/operations/reconcile/scan — async background scan
func (s *Server) startReconcileScan(c *gin.Context) {
	store := database.GlobalStore
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	id := ulid.Make().String()
	op, err := store.CreateOperation(id, "reconcile_scan", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return s.runReconcileScan(ctx, id, progress)
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "reconcile_scan", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// runReconcileScan executes the reconcile preview build and persists results.
// Read-only over DB and filesystem — safe to re-run on restart with no
// checkpoint, the same shape as runIsbnEnrichment / runMetadataRefreshScan.
func (s *Server) runReconcileScan(ctx context.Context, opID string, progress operations.ProgressReporter) error {
	store := database.GlobalStore
	if store == nil {
		return fmt.Errorf("database not initialized")
	}
	result, err := buildReconcilePreviewWithProgress(store, operations.LoggerFromReporter(progress))
	if err != nil {
		return fmt.Errorf("reconcile scan failed: %w", err)
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal scan results: %w", err)
	}
	if err := store.UpdateOperationResultData(opID, string(resultJSON)); err != nil {
		return fmt.Errorf("failed to store scan results: %w", err)
	}
	return nil
}

// latestReconcileScan handles GET /api/v1/operations/reconcile/scan/latest
func (s *Server) latestReconcileScan(c *gin.Context) {
	store := database.GlobalStore
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Find the most recent reconcile_scan operation
	ops, _, err := store.ListOperations(50, 0)
	if err != nil {
		internalError(c, "failed to list operations", err)
		return
	}

	for _, op := range ops {
		if op.Type != "reconcile_scan" {
			continue
		}
		// Return the operation with its result_data if completed
		if op.Status == "completed" && op.ResultData != nil {
			var preview ReconcilePreviewResult
			if err := json.Unmarshal([]byte(*op.ResultData), &preview); err == nil {
				c.JSON(http.StatusOK, gin.H{
					"operation": op,
					"preview":   preview,
				})
				return
			}
		}
		// Return op status if still running or failed
		c.JSON(http.StatusOK, gin.H{
			"operation": op,
			"preview":   nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"operation": nil, "preview": nil})
}

// startReconcile handles POST /api/v1/operations/reconcile
func (s *Server) startReconcile(c *gin.Context) {
	store := database.GlobalStore
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	if operations.GlobalQueue == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "operation queue not initialized"})
		return
	}

	var req struct {
		Matches []ReconcileApplyItem `json:"matches"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Matches) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no matches provided"})
		return
	}

	id := ulid.Make().String()
	op, err := store.CreateOperation(id, "reconcile", nil)
	if err != nil {
		internalError(c, "failed to create operation", err)
		return
	}

	matches := req.Matches
	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return executeReconcile(ctx, store, id, matches, operations.LoggerFromReporter(progress))
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "reconcile", operations.PriorityNormal, operationFunc); err != nil {
		internalError(c, "failed to enqueue operation", err)
		return
	}

	c.JSON(http.StatusAccepted, op)
}

// buildReconcilePreview builds the full reconciliation preview (sync, no progress).
func buildReconcilePreview(store database.Store) (*ReconcilePreviewResult, error) {
	return buildReconcilePreviewWithProgress(store, nil)
}

// buildReconcilePreviewWithProgress builds the full reconciliation preview with
// progress reporting for background operations. log may be nil.
func buildReconcilePreviewWithProgress(store database.Store, log logger.Logger) (*ReconcilePreviewResult, error) {
	if log == nil {
		log = logger.New("reconcile")
	}
	report := func(current, total int, msg string) {
		log.UpdateProgress(current, total, msg)
	}
	logMsg := func(level, msg string) {
		switch level {
		case "info":
			log.Info("reconcile: %s", msg)
		case "warn":
			log.Warn("reconcile: %s", msg)
		case "error":
			log.Error("reconcile: %s", msg)
		default:
			log.Debug("reconcile: %s", msg)
		}
	}

	result := &ReconcilePreviewResult{
		BrokenRecords:  []ReconcileBrokenRecord{},
		UntrackedFiles: []string{},
		Matches:        []ReconcileMatch{},
		UnmatchedBooks: []ReconcileBrokenRecord{},
	}

	// Step 1: Find broken DB records
	report(0, 100, "Loading all books from database...")
	books, err := store.GetAllBooks(100000, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list books: %w", err)
	}
	logMsg("info", fmt.Sprintf("Loaded %d books from database", len(books)))

	// Build set of all known file paths for quick lookup
	knownPaths := make(map[string]bool, len(books))
	var brokenBooks []database.Book

	report(5, 100, fmt.Sprintf("Checking file paths for %d books...", len(books)))
	for i, book := range books {
		if book.FilePath == "" {
			continue
		}
		knownPaths[book.FilePath] = true
		if _, err := os.Stat(book.FilePath); err != nil {
			brokenBooks = append(brokenBooks, book)
			result.BrokenRecords = append(result.BrokenRecords, ReconcileBrokenRecord{
				BookID:   book.ID,
				Title:    book.Title,
				FilePath: book.FilePath,
				FileHash: book.FileHash,
			})
		}
		if i%1000 == 0 && i > 0 {
			pct := 5 + (i*15)/len(books)
			report(pct, 100, fmt.Sprintf("Checked %d / %d book paths (%d broken so far)...", i, len(books), len(brokenBooks)))
		}
	}

	logMsg("info", fmt.Sprintf("Found %d broken records out of %d books", len(brokenBooks), len(books)))

	if len(brokenBooks) == 0 {
		report(100, 100, fmt.Sprintf("All %d books have valid file paths", len(books)))
		return result, nil
	}

	// Step 2: Scan directories for untracked audio files
	report(20, 100, "Scanning directories for untracked audio files...")
	logMsg("info", "Scanning library, import paths, and iTunes directories for untracked files")
	untrackedFiles, err := findUntrackedFiles(store, knownPaths)
	if err != nil {
		return nil, fmt.Errorf("failed to scan for untracked files: %w", err)
	}
	result.UntrackedFiles = untrackedFiles
	logMsg("info", fmt.Sprintf("Found %d untracked audio files on disk", len(untrackedFiles)))

	if len(untrackedFiles) == 0 {
		result.UnmatchedBooks = result.BrokenRecords
		report(100, 100, fmt.Sprintf("No untracked files found. %d broken records remain unmatched.", len(brokenBooks)))
		return result, nil
	}

	// Step 3: Hash untracked files for matching
	report(40, 100, fmt.Sprintf("Hashing %d untracked files...", len(untrackedFiles)))
	hashIndex := make(map[string]string)
	for i, fp := range untrackedFiles {
		h, err := scanner.ComputeSegmentFileHash(fp)
		if err != nil {
			continue
		}
		hashIndex[h] = fp
		if i%100 == 0 && i > 0 {
			pct := 40 + (i*20)/len(untrackedFiles)
			report(pct, 100, fmt.Sprintf("Hashed %d / %d untracked files...", i, len(untrackedFiles)))
		}
	}
	logMsg("info", fmt.Sprintf("Computed hashes for %d untracked files", len(hashIndex)))

	matchedBooks := make(map[string]bool)
	matchedFiles := make(map[string]bool)

	// Step 3a: Match by file hash
	report(60, 100, "Matching by file hash...")
	for _, book := range brokenBooks {
		if book.FileHash != nil && *book.FileHash != "" {
			if fp, ok := hashIndex[*book.FileHash]; ok && !matchedFiles[fp] {
				result.Matches = append(result.Matches, ReconcileMatch{
					BookID:     book.ID,
					BookTitle:  book.Title,
					OldPath:    book.FilePath,
					NewPath:    fp,
					MatchType:  "hash",
					Confidence: "high",
					Score:      1.0,
				})
				matchedBooks[book.ID] = true
				matchedFiles[fp] = true
			}
		}
	}

	// Step 3b: Match by original_file_hash
	for _, book := range brokenBooks {
		if matchedBooks[book.ID] {
			continue
		}
		if book.OriginalFileHash != nil && *book.OriginalFileHash != "" {
			if fp, ok := hashIndex[*book.OriginalFileHash]; ok && !matchedFiles[fp] {
				result.Matches = append(result.Matches, ReconcileMatch{
					BookID:     book.ID,
					BookTitle:  book.Title,
					OldPath:    book.FilePath,
					NewPath:    fp,
					MatchType:  "original_hash",
					Confidence: "high",
					Score:      0.95,
				})
				matchedBooks[book.ID] = true
				matchedFiles[fp] = true
			}
		}
	}
	logMsg("info", fmt.Sprintf("Hash matching found %d matches", len(result.Matches)))

	// Step 4: Match by filename pattern
	report(75, 100, "Matching by filename patterns...")
	filenameIndex := make(map[string][]string)
	for _, fp := range untrackedFiles {
		if matchedFiles[fp] {
			continue
		}
		base := normalizeFilename(filepath.Base(fp))
		filenameIndex[base] = append(filenameIndex[base], fp)
	}

	for _, book := range brokenBooks {
		if matchedBooks[book.ID] {
			continue
		}
		bookBase := normalizeFilename(filepath.Base(book.FilePath))
		if candidates, ok := filenameIndex[bookBase]; ok && len(candidates) > 0 {
			for _, fp := range candidates {
				if !matchedFiles[fp] {
					result.Matches = append(result.Matches, ReconcileMatch{
						BookID:     book.ID,
						BookTitle:  book.Title,
						OldPath:    book.FilePath,
						NewPath:    fp,
						MatchType:  "filename",
						Confidence: "low",
						Score:      0.5,
					})
					matchedBooks[book.ID] = true
					matchedFiles[fp] = true
					break
				}
			}
		}
	}

	// Step 4b: Try matching by title contained in filename
	report(85, 100, "Matching by title in filename...")
	for _, book := range brokenBooks {
		if matchedBooks[book.ID] {
			continue
		}
		normalizedTitle := normalizeFilename(book.Title)
		if normalizedTitle == "" {
			continue
		}
		for _, fp := range untrackedFiles {
			if matchedFiles[fp] {
				continue
			}
			normalizedBase := normalizeFilename(filepath.Base(fp))
			if strings.Contains(normalizedBase, normalizedTitle) ||
				strings.Contains(normalizedTitle, normalizedBase) {
				result.Matches = append(result.Matches, ReconcileMatch{
					BookID:     book.ID,
					BookTitle:  book.Title,
					OldPath:    book.FilePath,
					NewPath:    fp,
					MatchType:  "filename",
					Confidence: "low",
					Score:      0.3,
				})
				matchedBooks[book.ID] = true
				matchedFiles[fp] = true
				break
			}
		}
	}

	// Collect unmatched books
	for _, book := range brokenBooks {
		if !matchedBooks[book.ID] {
			result.UnmatchedBooks = append(result.UnmatchedBooks, ReconcileBrokenRecord{
				BookID:   book.ID,
				Title:    book.Title,
				FilePath: book.FilePath,
				FileHash: book.FileHash,
			})
		}
	}

	summary := fmt.Sprintf("Scan complete: %d matches (%d hash, %d filename), %d unmatched out of %d broken",
		len(result.Matches),
		countMatchType(result.Matches, "hash")+countMatchType(result.Matches, "original_hash"),
		countMatchType(result.Matches, "filename"),
		len(result.UnmatchedBooks),
		len(brokenBooks))
	logMsg("info", summary)
	report(100, 100, summary)

	return result, nil
}

// findUntrackedFiles walks directories in priority order (library root first,
// then import paths, then iTunes paths), collecting audio files not in the DB.
// Priority matters: if the same file exists in the library and an import path,
// the library copy is preferred for matching.
func findUntrackedFiles(store database.Store, knownPaths map[string]bool) ([]string, error) {
	var dirs []string

	// Priority 1: Library root (our organized folder — always preferred)
	if config.AppConfig.RootDir != "" {
		dirs = append(dirs, config.AppConfig.RootDir)
	}

	// Priority 2: Import paths
	importPaths, err := store.GetAllImportPaths()
	if err != nil {
		stdlog.Printf("[WARN] reconcile: failed to get import paths: %v", err)
	} else {
		for _, ip := range importPaths {
			if ip.Enabled {
				dirs = append(dirs, ip.Path)
			}
		}
	}

	// Priority 3: iTunes library paths (lowest priority — never modify these)
	if config.AppConfig.ITunesLibraryReadPath != "" {
		itunesMedia := filepath.Dir(config.AppConfig.ITunesLibraryReadPath)
		// Walk up to find the iTunes Media/Audiobooks folder
		audiobooks := filepath.Join(itunesMedia, "iTunes Media", "Audiobooks")
		if _, err := os.Stat(audiobooks); err == nil {
			dirs = append(dirs, audiobooks)
		}
	}

	if len(dirs) == 0 {
		return nil, nil
	}

	// Build set of supported extensions
	extSet := make(map[string]bool)
	for _, ext := range config.AppConfig.SupportedExtensions {
		extSet[strings.ToLower(ext)] = true
	}
	// Fallback if no extensions configured
	if len(extSet) == 0 {
		for _, ext := range []string{".m4b", ".mp3", ".m4a", ".flac", ".aac", ".ogg", ".wma"} {
			extSet[ext] = true
		}
	}

	var untracked []string
	seen := make(map[string]bool)

	for _, dir := range dirs {
		if _, err := os.Stat(dir); err != nil {
			stdlog.Printf("[WARN] reconcile: directory does not exist: %s", dir)
			continue
		}
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip errors
			}
			if info.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if !extSet[ext] {
				return nil
			}
			absPath, _ := filepath.Abs(path)
			if absPath == "" {
				absPath = path
			}
			if !knownPaths[absPath] && !knownPaths[path] && !seen[absPath] {
				untracked = append(untracked, absPath)
				seen[absPath] = true
			}
			return nil
		})
		if err != nil {
			stdlog.Printf("[WARN] reconcile: error walking %s: %v", dir, err)
		}
	}

	return untracked, nil
}

// executeReconcile applies confirmed matches: updates DB file_path and records OperationChanges.
func executeReconcile(ctx context.Context, store database.Store, operationID string, matches []ReconcileApplyItem, log logger.Logger) error {
	result := &ReconcileApplyResult{
		Errors: []string{},
	}

	total := len(matches)
	for i, m := range matches {
		if log.IsCanceled() {
			break
		}

		log.UpdateProgress(i+1, total, fmt.Sprintf("Updating %s", m.BookID))

		book, err := store.GetBookByID(m.BookID)
		if err != nil || book == nil {
			result.Errors = append(result.Errors, fmt.Sprintf("book %s: not found", m.BookID))
			result.Skipped++
			continue
		}

		// Verify the new path exists
		if _, err := os.Stat(m.NewPath); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("book %s: new path does not exist: %s", m.BookID, m.NewPath))
			result.Skipped++
			continue
		}

		oldPath := book.FilePath

		// Record the change for undo support
		change := &database.OperationChange{
			ID:          ulid.Make().String(),
			OperationID: operationID,
			BookID:      book.ID,
			ChangeType:  "file_path_update",
			FieldName:   "file_path",
			OldValue:    oldPath,
			NewValue:    m.NewPath,
		}
		if err := store.CreateOperationChange(change); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("book %s: failed to record change: %v", m.BookID, err))
			result.Skipped++
			continue
		}

		// Update the book's file path
		book.FilePath = m.NewPath
		if _, err := store.UpdateBook(book.ID, book); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("book %s: failed to update: %v", m.BookID, err))
			result.Skipped++
			continue
		}

		log.Info("Updated book %s: %s -> %s", book.ID, oldPath, m.NewPath)
		result.Applied++
	}

	// Store result data on the operation
	resultJSON, _ := json.Marshal(result)
	_ = store.UpdateOperationResultData(operationID, string(resultJSON))
	log.UpdateProgress(total, total, fmt.Sprintf("Reconciliation complete: %d applied, %d skipped", result.Applied, result.Skipped))

	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			log.Error("%s", e)
		}
		return fmt.Errorf("completed with %d errors: %s", len(result.Errors), result.Errors[0])
	}
	return nil
}

// normalizeFilename strips extension, lowercases, and removes non-alphanumeric chars for comparison.
func normalizeFilename(name string) string {
	// Strip extension
	ext := filepath.Ext(name)
	if ext != "" {
		name = name[:len(name)-len(ext)]
	}
	name = strings.ToLower(name)
	// Replace common separators with space
	name = strings.NewReplacer("-", " ", "_", " ", ".", " ").Replace(name)
	// Remove extra whitespace
	parts := strings.Fields(name)
	return strings.Join(parts, " ")
}

// countMatchType counts matches of a given type.
func countMatchType(matches []ReconcileMatch, matchType string) int {
	n := 0
	for _, m := range matches {
		if m.MatchType == matchType {
			n++
		}
	}
	return n
}

// VersionGroupCleanupResult holds the result of pruning duplicate version groups.
type VersionGroupCleanupResult struct {
	GroupsChecked     int `json:"groups_checked"`
	GroupsCleaned     int `json:"groups_cleaned"`
	DuplicatesRemoved int `json:"duplicates_removed"`
	FilesDeleted      int `json:"files_deleted"`
}

// cleanupDuplicateVersionGroups finds version groups with more than 2 members
// (1 original + 1 organized) and removes the extra organized copies that were
// created by the organize-reprocessing bug.
func cleanupDuplicateVersionGroups(store database.Store, rootDir string, dryRun bool) (*VersionGroupCleanupResult, error) {
	result := &VersionGroupCleanupResult{}

	// Fetch all books and group by version_group_id
	versionGroups := make(map[string][]database.Book)
	const pageSize = 1000
	for offset := 0; ; offset += pageSize {
		books, err := store.GetAllBooks(pageSize, offset)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch books: %w", err)
		}
		for _, b := range books {
			if b.VersionGroupID != nil && *b.VersionGroupID != "" {
				versionGroups[*b.VersionGroupID] = append(versionGroups[*b.VersionGroupID], b)
			}
		}
		if len(books) < pageSize {
			break
		}
	}

	for groupID, members := range versionGroups {
		result.GroupsChecked++
		if len(members) <= 2 {
			continue // normal: 1 original + 1 organized
		}

		// Separate into originals (non-primary, outside library) and organized copies (in library)
		var originals, libraryCopies []database.Book
		for _, m := range members {
			if rootDir != "" && strings.HasPrefix(m.FilePath, rootDir) {
				libraryCopies = append(libraryCopies, m)
			} else {
				originals = append(originals, m)
			}
		}

		if len(libraryCopies) <= 1 {
			continue // only one library copy, nothing to prune
		}

		// Keep the oldest library copy (first created = lowest ULID), remove the rest
		// Sort by ID (ULIDs sort chronologically)
		keepIdx := 0
		for i := 1; i < len(libraryCopies); i++ {
			if libraryCopies[i].ID < libraryCopies[keepIdx].ID {
				keepIdx = i
			}
		}

		result.GroupsCleaned++
		for i, dup := range libraryCopies {
			if i == keepIdx {
				continue
			}

			stdlog.Printf("[INFO] version-group cleanup: removing duplicate %s (%s) from group %s", dup.ID, dup.FilePath, groupID)

			if !dryRun {
				// Delete the file if it exists and is in the library
				if rootDir != "" && strings.HasPrefix(dup.FilePath, rootDir) {
					if _, err := os.Stat(dup.FilePath); err == nil {
						if err := os.Remove(dup.FilePath); err != nil {
							stdlog.Printf("[WARN] failed to delete duplicate file %s: %v", dup.FilePath, err)
						} else {
							result.FilesDeleted++
						}
					}
				}
				// Delete the book record
				if err := store.DeleteBook(dup.ID); err != nil {
					stdlog.Printf("[WARN] failed to delete duplicate book record %s: %v", dup.ID, err)
				}
			}
			result.DuplicatesRemoved++
		}

		// Ensure the kept library copy is primary and the original(s) are non-primary
		if !dryRun {
			isPrimary := true
			isNotPrimary := false
			kept := libraryCopies[keepIdx]
			kept.IsPrimaryVersion = &isPrimary
			store.UpdateBook(kept.ID, &kept)
			for _, orig := range originals {
				orig.IsPrimaryVersion = &isNotPrimary
				store.UpdateBook(orig.ID, &orig)
			}
		}
	}

	return result, nil
}

// cleanupDuplicateVersionGroupsHandler is the HTTP handler for POST /api/v1/operations/cleanup-version-groups
func (s *Server) cleanupDuplicateVersionGroupsHandler(c *gin.Context) {
	dryRun := c.Query("dry_run") == "true"
	result, err := cleanupDuplicateVersionGroups(database.GlobalStore, config.AppConfig.RootDir, dryRun)
	if err != nil {
		internalError(c, "failed to cleanup version groups", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"dry_run": dryRun,
		"result":  result,
	})
}

// BrokenSegmentResult describes books with missing segment files.
type BrokenSegmentResult struct {
	BooksChecked    int                  `json:"books_checked"`
	BrokenBooks     int                  `json:"broken_books"`
	MarkedForReview int                  `json:"marked_for_review"`
	Details         []BrokenSegmentEntry `json:"details"`
}

// BrokenSegmentEntry describes one book with missing segment files.
type BrokenSegmentEntry struct {
	BookID          string   `json:"book_id"`
	Title           string   `json:"title"`
	FilePath        string   `json:"file_path"`
	TotalSegments   int      `json:"total_segments"`
	MissingSegments int      `json:"missing_segments"`
	MissingPaths    []string `json:"missing_paths"`
}

// findBrokenSegmentBooks finds books whose segment files don't exist on disk
// and optionally marks them as needs_review.
func findBrokenSegmentBooks(store database.Store, dryRun bool) (*BrokenSegmentResult, error) {
	allBooks, err := store.GetAllBooks(100000, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get books: %w", err)
	}

	result := &BrokenSegmentResult{}
	now := time.Now()
	needsReview := "needs_review"

	for _, book := range allBooks {
		// Only check directory-based books in import paths (not in library)
		if config.AppConfig.RootDir != "" && strings.HasPrefix(book.FilePath, config.AppConfig.RootDir) {
			continue
		}
		info, serr := os.Stat(book.FilePath)
		if serr != nil || !info.IsDir() {
			continue
		}

		files, segErr := store.GetBookFiles(book.ID)
		if segErr != nil || len(files) == 0 {
			continue
		}

		result.BooksChecked++
		var missingPaths []string
		activeCount := 0
		for _, f := range files {
			if f.Missing || f.FilePath == "" {
				continue
			}
			activeCount++
			if _, ferr := os.Stat(f.FilePath); os.IsNotExist(ferr) {
				missingPaths = append(missingPaths, f.FilePath)
			}
		}

		if len(missingPaths) == 0 {
			continue
		}

		entry := BrokenSegmentEntry{
			BookID:          book.ID,
			Title:           book.Title,
			FilePath:        book.FilePath,
			TotalSegments:   activeCount,
			MissingSegments: len(missingPaths),
			MissingPaths:    missingPaths,
		}
		result.Details = append(result.Details, entry)
		result.BrokenBooks++

		if !dryRun {
			book.LibraryState = &needsReview
			book.MarkedForDeletion = boolPtr(true)
			book.MarkedForDeletionAt = &now
			if _, uerr := store.UpdateBook(book.ID, &book); uerr != nil {
				stdlog.Printf("[WARN] failed to mark broken book %s: %v", book.ID, uerr)
			} else {
				result.MarkedForReview++
			}
		}
	}

	return result, nil
}

// markBrokenSegmentBooksHandler handles POST /api/v1/operations/mark-broken-segments
func (s *Server) markBrokenSegmentBooksHandler(c *gin.Context) {
	dryRun := c.Query("dry_run") == "true"
	result, err := findBrokenSegmentBooks(database.GlobalStore, dryRun)
	if err != nil {
		internalError(c, "failed to find broken segments", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"dry_run": dryRun,
		"result":  result,
	})
}

// MergeDuplicatesResult describes the outcome of merging no-VG duplicates into existing version groups.
type MergeDuplicatesResult struct {
	TotalNoVG        int                   `json:"total_no_vg"`
	MatchedToVG      int                   `json:"matched_to_vg"`
	SelfDuplicates   int                   `json:"self_duplicates"`
	MetadataMerged   int                   `json:"metadata_merged"`
	SoftDeleted      int                   `json:"soft_deleted"`
	RemainingOrphans int                   `json:"remaining_orphans"`
	Errors           int                   `json:"errors"`
	Details          []MergeDuplicateEntry `json:"details,omitempty"`
}

// MergeDuplicateEntry describes one merge action.
type MergeDuplicateEntry struct {
	DuplicateID  string   `json:"duplicate_id"`
	PrimaryID    string   `json:"primary_id"`
	Title        string   `json:"title"`
	FieldsMerged []string `json:"fields_merged,omitempty"`
	Action       string   `json:"action"`
}

// mergeNoVGDuplicates finds no-VG books that match VG books by title, merges metadata, and soft-deletes.
// It also deduplicates among the remaining no-VG orphans (keeping one per title, soft-deleting extras).
func mergeNoVGDuplicates(store database.Store, rootDir string, dryRun bool) (*MergeDuplicatesResult, error) {
	result := &MergeDuplicatesResult{}

	// Load all books in pages
	var allBooks []database.Book
	pageSize := 5000
	for offset := 0; ; offset += pageSize {
		books, err := store.GetAllBooks(pageSize, offset)
		if err != nil {
			return nil, fmt.Errorf("failed to get books: %w", err)
		}
		allBooks = append(allBooks, books...)
		if len(books) < pageSize {
			break
		}
	}

	// Index VG primary books by normalized title
	vgPrimaryByTitle := make(map[string]*database.Book)
	var noVGBooks []database.Book

	for i := range allBooks {
		b := &allBooks[i]
		if b.VersionGroupID != nil && *b.VersionGroupID != "" {
			if b.IsPrimaryVersion != nil && *b.IsPrimaryVersion {
				normTitle := strings.TrimSpace(strings.ToLower(b.Title))
				vgPrimaryByTitle[normTitle] = b
			}
		} else {
			if rootDir != "" && strings.HasPrefix(b.FilePath, rootDir) {
				noVGBooks = append(noVGBooks, *b)
			}
		}
	}

	result.TotalNoVG = len(noVGBooks)
	now := time.Now()
	deletedState := "deleted"

	// Helper to soft-delete a book
	softDelete := func(book *database.Book) error {
		book.MarkedForDeletion = boolPtr(true)
		book.MarkedForDeletionAt = &now
		book.LibraryState = &deletedState
		_, err := store.UpdateBook(book.ID, book)
		return err
	}

	// Phase 1: Match no-VG books against existing VG primaries
	// Track which no-VG books have been handled
	handled := make(map[string]bool)

	for i := range noVGBooks {
		dupe := &noVGBooks[i]
		normTitle := strings.TrimSpace(strings.ToLower(dupe.Title))
		primary, found := vgPrimaryByTitle[normTitle]
		if !found {
			continue
		}

		handled[dupe.ID] = true
		result.MatchedToVG++
		entry := MergeDuplicateEntry{
			DuplicateID: dupe.ID,
			PrimaryID:   primary.ID,
			Title:       dupe.Title,
		}

		merged := mergeBookMetadata(primary, dupe)
		entry.FieldsMerged = merged

		if !dryRun {
			if len(merged) > 0 {
				if _, err := store.UpdateBook(primary.ID, primary); err != nil {
					stdlog.Printf("[WARN] merge-dupes: failed to update primary %s: %v", primary.ID, err)
					entry.Action = "error"
					result.Errors++
					result.Details = append(result.Details, entry)
					continue
				}
				result.MetadataMerged++
			}
			if err := softDelete(dupe); err != nil {
				stdlog.Printf("[WARN] merge-dupes: failed to soft-delete %s: %v", dupe.ID, err)
				entry.Action = "error"
				result.Errors++
			} else {
				result.SoftDeleted++
				if len(merged) > 0 {
					entry.Action = "merged_and_deleted"
				} else {
					entry.Action = "deleted_no_merge_needed"
				}
			}
		} else {
			result.SoftDeleted++
			if len(merged) > 0 {
				entry.Action = "would_merge_and_delete"
				result.MetadataMerged++
			} else {
				entry.Action = "would_delete_no_merge_needed"
			}
		}
		result.Details = append(result.Details, entry)
	}

	// Phase 2: Deduplicate among remaining no-VG orphans (same title → keep one, delete rest)
	// Group remaining orphans by normalized title
	orphansByTitle := make(map[string][]*database.Book)
	for i := range noVGBooks {
		b := &noVGBooks[i]
		if handled[b.ID] {
			continue
		}
		normTitle := strings.TrimSpace(strings.ToLower(b.Title))
		orphansByTitle[normTitle] = append(orphansByTitle[normTitle], b)
	}

	for _, group := range orphansByTitle {
		if len(group) <= 1 {
			continue
		}

		// Pick the best keeper: prefer directory-based (multi-file) paths, then longest path
		bestIdx := 0
		for i := 1; i < len(group); i++ {
			// Prefer books whose path is a directory (multi-file book) over individual segment files
			bestIsDir := !strings.Contains(filepath.Base(group[bestIdx].FilePath), ".")
			currIsDir := !strings.Contains(filepath.Base(group[i].FilePath), ".")
			if currIsDir && !bestIsDir {
				bestIdx = i
			} else if currIsDir == bestIsDir {
				// Prefer shorter path (usually the directory, not a segment file)
				if len(group[i].FilePath) < len(group[bestIdx].FilePath) {
					bestIdx = i
				}
			}
		}

		keeper := group[bestIdx]
		for i, dupe := range group {
			if i == bestIdx {
				continue
			}

			result.SelfDuplicates++
			entry := MergeDuplicateEntry{
				DuplicateID: dupe.ID,
				PrimaryID:   keeper.ID,
				Title:       dupe.Title,
			}

			merged := mergeBookMetadata(keeper, dupe)
			entry.FieldsMerged = merged

			if !dryRun {
				if len(merged) > 0 {
					if _, err := store.UpdateBook(keeper.ID, keeper); err != nil {
						stdlog.Printf("[WARN] merge-self-dupes: failed to update keeper %s: %v", keeper.ID, err)
					} else {
						result.MetadataMerged++
					}
				}
				if err := softDelete(dupe); err != nil {
					stdlog.Printf("[WARN] merge-self-dupes: failed to soft-delete %s: %v", dupe.ID, err)
					entry.Action = "error"
					result.Errors++
				} else {
					result.SoftDeleted++
					entry.Action = "self_dupe_deleted"
				}
			} else {
				result.SoftDeleted++
				entry.Action = "would_delete_self_dupe"
				if len(merged) > 0 {
					result.MetadataMerged++
				}
			}
			result.Details = append(result.Details, entry)
		}
	}

	// Count remaining orphans (unique no-VG books with no match)
	remaining := 0
	for _, group := range orphansByTitle {
		if len(group) >= 1 {
			remaining++
		}
	}
	result.RemainingOrphans = remaining

	return result, nil
}

// mergeBookMetadata copies non-empty metadata fields from src to dst where dst field is empty.
// Returns list of field names that were merged.
func mergeBookMetadata(dst, src *database.Book) []string {
	var merged []string

	if dst.Narrator == nil && src.Narrator != nil {
		dst.Narrator = src.Narrator
		merged = append(merged, "narrator")
	}
	if dst.NarratorsJSON == nil && src.NarratorsJSON != nil {
		dst.NarratorsJSON = src.NarratorsJSON
		merged = append(merged, "narrators_json")
	}
	if dst.Description == nil && src.Description != nil {
		dst.Description = src.Description
		merged = append(merged, "description")
	}
	if dst.Language == nil && src.Language != nil {
		dst.Language = src.Language
		merged = append(merged, "language")
	}
	if dst.Publisher == nil && src.Publisher != nil {
		dst.Publisher = src.Publisher
		merged = append(merged, "publisher")
	}
	if dst.PrintYear == nil && src.PrintYear != nil {
		dst.PrintYear = src.PrintYear
		merged = append(merged, "print_year")
	}
	if dst.AudiobookReleaseYear == nil && src.AudiobookReleaseYear != nil {
		dst.AudiobookReleaseYear = src.AudiobookReleaseYear
		merged = append(merged, "audiobook_release_year")
	}
	if dst.ISBN10 == nil && src.ISBN10 != nil {
		dst.ISBN10 = src.ISBN10
		merged = append(merged, "isbn10")
	}
	if dst.ISBN13 == nil && src.ISBN13 != nil {
		dst.ISBN13 = src.ISBN13
		merged = append(merged, "isbn13")
	}
	if dst.ASIN == nil && src.ASIN != nil {
		dst.ASIN = src.ASIN
		merged = append(merged, "asin")
	}
	if dst.OpenLibraryID == nil && src.OpenLibraryID != nil {
		dst.OpenLibraryID = src.OpenLibraryID
		merged = append(merged, "open_library_id")
	}
	if dst.HardcoverID == nil && src.HardcoverID != nil {
		dst.HardcoverID = src.HardcoverID
		merged = append(merged, "hardcover_id")
	}
	if dst.GoogleBooksID == nil && src.GoogleBooksID != nil {
		dst.GoogleBooksID = src.GoogleBooksID
		merged = append(merged, "google_books_id")
	}
	if dst.Edition == nil && src.Edition != nil {
		dst.Edition = src.Edition
		merged = append(merged, "edition")
	}
	if dst.CoverURL == nil && src.CoverURL != nil {
		dst.CoverURL = src.CoverURL
		merged = append(merged, "cover_url")
	}
	if dst.Duration == nil && src.Duration != nil {
		dst.Duration = src.Duration
		merged = append(merged, "duration")
	}
	if dst.Bitrate == nil && src.Bitrate != nil {
		dst.Bitrate = src.Bitrate
		merged = append(merged, "bitrate")
	}
	if dst.Codec == nil && src.Codec != nil {
		dst.Codec = src.Codec
		merged = append(merged, "codec")
	}
	if dst.SampleRate == nil && src.SampleRate != nil {
		dst.SampleRate = src.SampleRate
		merged = append(merged, "sample_rate")
	}
	if dst.Channels == nil && src.Channels != nil {
		dst.Channels = src.Channels
		merged = append(merged, "channels")
	}
	if dst.FileHash == nil && src.FileHash != nil {
		dst.FileHash = src.FileHash
		merged = append(merged, "file_hash")
	}
	if dst.FileSize == nil && src.FileSize != nil {
		dst.FileSize = src.FileSize
		merged = append(merged, "file_size")
	}
	// iTunes fields
	if dst.ITunesPersistentID == nil && src.ITunesPersistentID != nil {
		dst.ITunesPersistentID = src.ITunesPersistentID
		merged = append(merged, "itunes_persistent_id")
	}
	if dst.ITunesPlayCount == nil && src.ITunesPlayCount != nil {
		dst.ITunesPlayCount = src.ITunesPlayCount
		merged = append(merged, "itunes_play_count")
	}
	if dst.ITunesRating == nil && src.ITunesRating != nil {
		dst.ITunesRating = src.ITunesRating
		merged = append(merged, "itunes_rating")
	}
	if dst.ITunesBookmark == nil && src.ITunesBookmark != nil {
		dst.ITunesBookmark = src.ITunesBookmark
		merged = append(merged, "itunes_bookmark")
	}
	// Author/Series: only merge if dst has none
	if dst.AuthorID == nil && src.AuthorID != nil {
		dst.AuthorID = src.AuthorID
		merged = append(merged, "author_id")
	}
	if dst.SeriesID == nil && src.SeriesID != nil {
		dst.SeriesID = src.SeriesID
		merged = append(merged, "series_id")
	}
	if dst.SeriesSequence == nil && src.SeriesSequence != nil {
		dst.SeriesSequence = src.SeriesSequence
		merged = append(merged, "series_sequence")
	}

	return merged
}

// mergeNoVGDuplicatesHandler handles POST /api/v1/operations/merge-novg-duplicates
func (s *Server) mergeNoVGDuplicatesHandler(c *gin.Context) {
	dryRun := c.Query("dry_run") == "true"
	result, err := mergeNoVGDuplicates(database.GlobalStore, config.AppConfig.RootDir, dryRun)
	if err != nil {
		internalError(c, "failed to merge duplicates", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"dry_run": dryRun,
		"result":  result,
	})
}

// AssignVGResult holds the result of assigning version groups to orphan library books.
type AssignVGResult struct {
	TotalChecked int `json:"total_checked"`
	Assigned     int `json:"assigned"`
	AlreadyHasVG int `json:"already_has_vg"`
	NotInLibrary int `json:"not_in_library"`
	Errors       int `json:"errors"`
}

// assignOrphanVGs finds books in the library directory that have no version group,
// creates a VG for each, and marks them as primary. This fixes books that were
// organized before a DB wipe and re-scanned without linkage.
func assignOrphanVGs(store database.Store, rootDir string) (*AssignVGResult, error) {
	result := &AssignVGResult{}

	var allBooks []database.Book
	pageSize := 5000
	for offset := 0; ; offset += pageSize {
		books, err := store.GetAllBooks(pageSize, offset)
		if err != nil {
			return nil, fmt.Errorf("failed to get books: %w", err)
		}
		allBooks = append(allBooks, books...)
		if len(books) < pageSize {
			break
		}
	}

	for i := range allBooks {
		b := &allBooks[i]
		result.TotalChecked++

		// Skip books that already have a VG
		if b.VersionGroupID != nil && *b.VersionGroupID != "" {
			result.AlreadyHasVG++
			continue
		}

		// Only process books in the library directory
		if rootDir == "" || !strings.HasPrefix(b.FilePath, rootDir) {
			result.NotInLibrary++
			continue
		}

		// Create a version group and mark as primary
		vgID := fmt.Sprintf("vg-%s", ulid.Make().String())
		b.VersionGroupID = &vgID
		isPrimary := true
		b.IsPrimaryVersion = &isPrimary
		organizedState := "organized"
		b.LibraryState = &organizedState

		if _, err := store.UpdateBook(b.ID, b); err != nil {
			result.Errors++
			continue
		}
		result.Assigned++
	}

	return result, nil
}

// assignOrphanVGsHandler handles POST /api/v1/operations/assign-orphan-vgs
func (s *Server) assignOrphanVGsHandler(c *gin.Context) {
	result, err := assignOrphanVGs(database.GlobalStore, config.AppConfig.RootDir)
	if err != nil {
		internalError(c, "failed to assign version groups", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}
