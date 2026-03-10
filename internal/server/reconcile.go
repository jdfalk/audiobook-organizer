// file: internal/server/reconcile.go
// version: 1.1.0
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
	MatchType  string  `json:"match_type"`  // "hash", "original_hash", "filename"
	Confidence string  `json:"confidence"`  // "high", "medium", "low"
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		result, err := buildReconcilePreviewWithProgress(store, operations.LoggerFromReporter(progress))
		if err != nil {
			return fmt.Errorf("reconcile scan failed: %w", err)
		}

		resultJSON, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("failed to marshal scan results: %w", err)
		}
		if err := store.UpdateOperationResultData(id, string(resultJSON)); err != nil {
			return fmt.Errorf("failed to store scan results: %w", err)
		}
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "reconcile_scan", operations.PriorityNormal, operationFunc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, op)
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	matches := req.Matches
	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		return executeReconcile(ctx, store, id, matches, operations.LoggerFromReporter(progress))
	}

	if err := operations.GlobalQueue.Enqueue(op.ID, "reconcile", operations.PriorityNormal, operationFunc); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	if config.AppConfig.ITunesLibraryXMLPath != "" {
		itunesMedia := filepath.Dir(config.AppConfig.ITunesLibraryXMLPath)
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
