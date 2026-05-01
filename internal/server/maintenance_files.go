// file: internal/server/maintenance_files.go
// version: 1.0.0
// guid: 8863c612-6680-401e-9f8d-0a32fb296e3b
// last-edited: 2026-05-01

package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
	"github.com/oklog/ulid/v2"
)

// ---------------------------------------------------------------------------
// Backfill book_files
// ---------------------------------------------------------------------------

// bookFilesBackfillResult describes one book processed during the backfill.
type bookFilesBackfillResult struct {
	BookID       string   `json:"book_id"`
	BookTitle    string   `json:"book_title"`
	FilePath     string   `json:"file_path"`
	FilesCreated int      `json:"files_created"`
	FilePaths    []string `json:"file_paths"`
	Skipped      bool     `json:"skipped,omitempty"`
	SkipReason   string   `json:"skip_reason,omitempty"`
	Missing      bool     `json:"missing,omitempty"`
	Applied      bool     `json:"applied"`
	Error        string   `json:"error,omitempty"`
}

// handleBackfillBookFiles scans all books and creates book_files rows where
// none exist yet.
//
// Query params:
//   - dry_run=true  (default) — report what would be created without modifying
//   - dry_run=false — actually create the rows
func (s *Server) handleBackfillBookFiles(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") == "true"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Fetch all books (0,0 = no pagination).
	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	var results []bookFilesBackfillResult
	totalFiles := 0

	for i := range allBooks {
		book := &allBooks[i]

		// Check whether book_files rows already exist for this book.
		existing, bfErr := store.GetBookFiles(book.ID)
		if bfErr != nil {
			results = append(results, bookFilesBackfillResult{
				BookID:    book.ID,
				BookTitle: book.Title,
				FilePath:  book.FilePath,
				Error:     fmt.Sprintf("GetBookFiles: %v", bfErr),
			})
			continue
		}
		if len(existing) > 0 {
			results = append(results, bookFilesBackfillResult{
				BookID:     book.ID,
				BookTitle:  book.Title,
				FilePath:   book.FilePath,
				Skipped:    true,
				SkipReason: fmt.Sprintf("already has %d book_file row(s)", len(existing)),
			})
			continue
		}

		// Determine what files to create rows for.
		var filesToCreate []string
		var isMissing bool

		if book.FilePath == "" {
			results = append(results, bookFilesBackfillResult{
				BookID:     book.ID,
				BookTitle:  book.Title,
				Skipped:    true,
				SkipReason: "empty file_path",
			})
			continue
		}

		fi, statErr := os.Stat(book.FilePath)
		if statErr != nil {
			// Path doesn't exist — create one row marked missing.
			filesToCreate = []string{book.FilePath}
			isMissing = true
		} else if fi.IsDir() {
			// Directory: glob for audio files using the shared audioFilesInDir helper.
			filesToCreate = metafetch.AudioFilesInDir(book.FilePath)
			if len(filesToCreate) == 0 {
				results = append(results, bookFilesBackfillResult{
					BookID:     book.ID,
					BookTitle:  book.Title,
					FilePath:   book.FilePath,
					Skipped:    true,
					SkipReason: "directory contains no recognised audio files",
				})
				continue
			}
		} else {
			// Single file.
			filesToCreate = []string{book.FilePath}
		}

		result := bookFilesBackfillResult{
			BookID:       book.ID,
			BookTitle:    book.Title,
			FilePath:     book.FilePath,
			FilesCreated: len(filesToCreate),
			FilePaths:    filesToCreate,
			Missing:      isMissing,
		}

		if !dryRun {
			createErr := createBookFilesForBook(store, book, filesToCreate, isMissing)
			if createErr != nil {
				result.Error = createErr.Error()
				log.Printf("[WARN] backfill-book-files: book %s (%q): %v", book.ID, book.Title, createErr)
			} else {
				result.Applied = true
				// If file_path pointed directly at a file (not a directory), normalise
				// book.file_path to the parent directory.
				if !isMissing && fi != nil && !fi.IsDir() && len(filesToCreate) == 1 {
					current, getErr := store.GetBookByID(book.ID)
					if getErr == nil && current != nil {
						current.FilePath = filepath.Dir(book.FilePath)
						if _, upErr := store.UpdateBook(book.ID, current); upErr != nil {
							log.Printf("[WARN] backfill-book-files: normalise file_path for book %s: %v", book.ID, upErr)
						}
					}
				}
				log.Printf("[INFO] backfill-book-files: created %d book_file row(s) for book %s (%q)",
					len(filesToCreate), book.ID, book.Title)
			}
		}

		results = append(results, result)
		totalFiles += len(filesToCreate)
	}

	// Compute summary counts.
	applied := 0
	skipped := 0
	errors := 0
	for _, r := range results {
		switch {
		case r.Error != "":
			errors++
		case r.Skipped:
			skipped++
		case r.Applied || dryRun:
			applied++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":       dryRun,
		"books_total":   len(allBooks),
		"books_found":   len(results) - skipped,
		"books_skipped": skipped,
		"files_total":   totalFiles,
		"applied":       applied,
		"errors":        errors,
		"results":       results,
	})
}

// ---------------------------------------------------------------------------
// Empty folder cleanup
// ---------------------------------------------------------------------------

// emptyFolderResult describes a directory that was (or would be) removed.
type emptyFolderResult struct {
	Path    string `json:"path"`
	Removed bool   `json:"removed"`
	Error   string `json:"error,omitempty"`
}

// handleCleanupEmptyFolders walks the audiobook root directory, finds empty
// directories (no files; only empty subdirectories), and removes them
// bottom-up (deepest first).
//
// Query params:
//   - dry_run=true  (default) — report what would be removed without deleting
//   - dry_run=false — actually delete the directories
func (s *Server) handleCleanupEmptyFolders(c *gin.Context) {
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

	// Collect all directories (depth-first, pre-order). We reverse the list
	// afterward so we process deepest entries first (bottom-up).
	var dirs []string
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			// Non-fatal: log and continue.
			log.Printf("[WARN] cleanup-empty-folders: walk error at %q: %v", path, walkErr)
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		if path == rootDir {
			return nil // Never remove the root itself.
		}
		// Skip hidden directories (starting with a dot).
		if strings.HasPrefix(filepath.Base(path), ".") {
			return filepath.SkipDir
		}
		dirs = append(dirs, path)
		return nil
	})
	if err != nil {
		internalError(c, "failed to walk root directory", err)
		return
	}

	// Reverse so deepest directories come first.
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})

	var results []emptyFolderResult
	removedCount := 0

	for _, dir := range dirs {
		empty, checkErr := isDirEmpty(dir)
		if checkErr != nil {
			results = append(results, emptyFolderResult{
				Path:  dir,
				Error: fmt.Sprintf("stat error: %v", checkErr),
			})
			continue
		}
		if !empty {
			continue
		}

		result := emptyFolderResult{Path: dir}

		if !dryRun {
			if removeErr := os.Remove(dir); removeErr != nil {
				result.Error = removeErr.Error()
				log.Printf("[WARN] cleanup-empty-folders: failed to remove %q: %v", dir, removeErr)
			} else {
				result.Removed = true
				removedCount++
				log.Printf("[INFO] cleanup-empty-folders: removed empty directory %q", dir)
			}
		} else {
			removedCount++
		}

		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":         dryRun,
		"root_dir":        rootDir,
		"folders_found":   len(results),
		"folders_removed": removedCount,
		"folders":         results,
	})
}

// isDirEmpty reports whether dir contains no files or non-hidden subdirectories.
// It reads only the immediate children of dir.
func isDirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		// Any non-hidden entry means the directory is not empty.
		if !strings.HasPrefix(e.Name(), ".") {
			return false, nil
		}
	}
	return true, nil
}

// ---------------------------------------------------------------------------
// Garbage directory detection (cleanup-organize-mess)
// ---------------------------------------------------------------------------

// garbageDirResult describes a directory that looks like a file-fragment garbage
// directory left behind by a failed or partial organize run.
type garbageDirResult struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// isGarbageDirectory returns a non-empty reason string if the directory name
// looks like a file fragment rather than a real book/author/series directory.
// Examples of garbage:
//   - "04_ Intro"     — starts with digits and underscore (chapter file fragment)
//   - "04 - Intro"    — starts with digits and space-dash (chapter fragment)
//   - "Hero's Trial - 04 - Intro"  — contains double-nested path fragment
//   - Very short names (1-2 chars) that are not normal
func isGarbageDirectory(name string) string {
	if name == "" {
		return ""
	}

	// Pattern: starts with 2-3 digits followed by underscore or space-dash
	// e.g. "04_", "04 -", "004_", "1 -"
	chapterFragmentRe := regexp.MustCompile(`^\d{1,3}[_ ][_\-\s]`)
	if chapterFragmentRe.MatchString(name) {
		return "starts with chapter number fragment"
	}

	// Pattern: purely numeric name (e.g. "04", "004")
	pureNumericRe := regexp.MustCompile(`^\d+$`)
	if pureNumericRe.MatchString(name) {
		return "purely numeric directory name"
	}

	// Pattern: contains " - NN - " which looks like a double-nested segment
	// e.g. "Hero's Trial - 04 - Intro"
	doubleSegmentRe := regexp.MustCompile(` - \d{1,3} - `)
	if doubleSegmentRe.MatchString(name) {
		return "contains double-nested chapter segment pattern"
	}

	// Pattern: very short name (1 or 2 non-whitespace chars) that isn't a known
	// abbreviation — typically leftover from a bad path split.
	trimmed := strings.TrimSpace(name)
	if len([]rune(trimmed)) <= 2 && !allAlpha(trimmed) {
		return "suspiciously short non-alphabetic directory name"
	}

	return ""
}

// allAlpha returns true if every rune in s is a letter (handles Unicode).
func allAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return len(s) > 0
}

// handleCleanupOrganizeMess walks the audiobook root directory and reports
// (or removes) directories that look like garbage left behind by a partial or
// broken organize run, as well as empty directories.
//
// Query params:
//   - dry_run=true  (default) — report what would be removed without deleting
//   - dry_run=false — actually delete empty directories; garbage dirs with files
//     are always just reported (manual review required for non-empty garbage dirs)
func (s *Server) handleCleanupOrganizeMess(c *gin.Context) {
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

	var dirs []string
	walkErr := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("[WARN] cleanup-organize-mess: walk error at %q: %v", path, err)
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		if path == rootDir {
			return nil
		}
		if strings.HasPrefix(filepath.Base(path), ".") {
			return filepath.SkipDir
		}
		dirs = append(dirs, path)
		return nil
	})
	if walkErr != nil {
		internalError(c, "failed to walk root directory", walkErr)
		return
	}

	// Process deepest directories first (bottom-up).
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})

	var emptyResults []emptyFolderResult
	var garbageResults []garbageDirResult
	emptyRemoved := 0

	for _, dir := range dirs {
		name := filepath.Base(dir)

		// Check for garbage name pattern first.
		if reason := isGarbageDirectory(name); reason != "" {
			garbageResults = append(garbageResults, garbageDirResult{
				Path:   dir,
				Reason: reason,
			})
			// Garbage directories with files are NOT auto-removed — log for manual review.
			// If they are also empty, they will be caught below and removed if !dryRun.
		}

		// Check emptiness.
		empty, checkErr := isDirEmpty(dir)
		if checkErr != nil {
			emptyResults = append(emptyResults, emptyFolderResult{
				Path:  dir,
				Error: fmt.Sprintf("stat error: %v", checkErr),
			})
			continue
		}
		if !empty {
			continue
		}

		result := emptyFolderResult{Path: dir}
		if !dryRun {
			if removeErr := os.Remove(dir); removeErr != nil {
				result.Error = removeErr.Error()
				log.Printf("[WARN] cleanup-organize-mess: failed to remove %q: %v", dir, removeErr)
			} else {
				result.Removed = true
				emptyRemoved++
				log.Printf("[INFO] cleanup-organize-mess: removed empty directory %q", dir)
			}
		} else {
			emptyRemoved++
		}
		emptyResults = append(emptyResults, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":               dryRun,
		"root_dir":              rootDir,
		"empty_folders_found":   len(emptyResults),
		"empty_folders_removed": emptyRemoved,
		"garbage_dirs_found":    len(garbageResults),
		"garbage_dirs_note":     "Non-empty garbage directories require manual review; only empty ones are removed.",
		"empty_folders":         emptyResults,
		"garbage_dirs":          garbageResults,
	})
}

func createBookFilesForBook(store maintenanceStore, book *database.Book, filePaths []string, missing bool) error {
	for _, fp := range filePaths {
		ext := strings.ToLower(filepath.Ext(fp))
		// Strip leading dot from extension for the format field.
		format := strings.TrimPrefix(ext, ".")

		var fileSize int64
		if !missing {
			if info, err := os.Stat(fp); err == nil {
				fileSize = info.Size()
			}
		}

		bf := &database.BookFile{
			ID:               ulid.Make().String(),
			BookID:           book.ID,
			FilePath:         fp,
			OriginalFilename: filepath.Base(fp),
			Format:           format,
			FileSize:         fileSize,
			Missing:          missing,
		}
		if err := store.CreateBookFile(bf); err != nil {
			return fmt.Errorf("CreateBookFile(%q): %w", fp, err)
		}
	}
	return nil
}

func isAuthorDirectory(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}

	bookSubdirs := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subPath := filepath.Join(dir, e.Name())
		if len(metafetch.AudioFilesInDir(subPath)) > 0 {
			bookSubdirs++
			if bookSubdirs >= 2 {
				return true
			}
		}
	}
	return false
}

// bestMatchSubdir returns the subdirectory of parent whose name best matches
// title.  It uses word-overlap scoring; returns "" if no reasonable match is
// found.
func bestMatchSubdir(parent, title string) string {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return ""
	}

	titleWords := longWords(strings.ToLower(extractCoreTitle(title)))

	bestPath := ""
	bestScore := 0

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Only consider subdirs that actually contain audio files.
		sub := filepath.Join(parent, e.Name())
		if len(metafetch.AudioFilesInDir(sub)) == 0 {
			continue
		}

		dirWords := longWords(strings.ToLower(e.Name()))
		score := 0
		for w := range titleWords {
			if dirWords[w] {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestPath = sub
		}
	}

	// Require at least one matching word.
	if bestScore == 0 {
		return ""
	}
	return bestPath
}

// fixAuthorDirPath updates the book's file_path to the given subdir, then
// rebuilds book_files rows from that directory.
func fixAuthorDirPath(store maintenanceStore, book *database.Book, subdir string) error {
	// Re-fetch to avoid stale data (UpdateBook does FULL column replacement).
	current, err := store.GetBookByID(book.ID)
	if err != nil {
		return fmt.Errorf("GetBookByID: %w", err)
	}
	if current == nil {
		return fmt.Errorf("book %s not found", book.ID)
	}

	current.FilePath = subdir

	if _, err = store.UpdateBook(book.ID, current); err != nil {
		return fmt.Errorf("UpdateBook: %w", err)
	}

	// Delete existing book_files for this book and rebuild from the new directory.
	if err = store.DeleteBookFilesForBook(book.ID); err != nil {
		return fmt.Errorf("DeleteBookFilesForBook: %w", err)
	}

	newFiles := metafetch.AudioFilesInDir(subdir)
	if len(newFiles) == 0 {
		// No audio files found — leave book_files empty for now (not an error).
		return nil
	}

	return createBookFilesForBook(store, current, newFiles, false)
}

// ---------------------------------------------------------------------------
// Enrich book_files — track numbers, file sizes, formats, original filenames
// ---------------------------------------------------------------------------

// enrichBookFileResult describes one book_files row that was (or would be)
// enriched.
type enrichBookFileResult struct {
	FileID          string `json:"file_id"`
	BookID          string `json:"book_id"`
	FilePath        string `json:"file_path"`
	TrackNumberOld  int    `json:"track_number_old,omitempty"`
	TrackNumberNew  int    `json:"track_number_new,omitempty"`
	TrackCountOld   int    `json:"track_count_old,omitempty"`
	TrackCountNew   int    `json:"track_count_new,omitempty"`
	FileSizeOld     int64  `json:"file_size_old,omitempty"`
	FileSizeNew     int64  `json:"file_size_new,omitempty"`
	FormatOld       string `json:"format_old,omitempty"`
	FormatNew       string `json:"format_new,omitempty"`
	OrigFilenameSet bool   `json:"original_filename_set,omitempty"`
	Changed         bool   `json:"changed"`
	Applied         bool   `json:"applied"`
	Error           string `json:"error,omitempty"`
	Warning         string `json:"warning,omitempty"`
}

// handleEnrichBookFiles iterates all book_files rows and fills in missing or
// zero-valued fields:
//   - track_number: parsed from leading digits in the filename
//   - track_count:  total number of files for the owning book
//   - file_size:    from os.Stat when currently 0 or suspiciously small (<1000 bytes)
//   - format:       from filepath.Ext when empty
//   - original_filename: from filepath.Base when empty
//
// Also detects book_files where file_path points to a directory (not an audio
// file) and flags them with a warning.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update the database
func (s *Server) handleEnrichBookFiles(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Fetch all books so we can iterate book_files per book.
	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	var results []enrichBookFileResult
	totalChanged := 0
	totalApplied := 0
	totalErrors := 0

	for i := range allBooks {
		book := &allBooks[i]

		files, bfErr := store.GetBookFiles(book.ID)
		if bfErr != nil {
			log.Printf("[WARN] enrich-book-files: GetBookFiles book %s: %v", book.ID, bfErr)
			continue
		}
		if len(files) == 0 {
			continue
		}

		trackCount := len(files)

		for j := range files {
			f := &files[j]
			result := enrichBookFileResult{
				FileID:   f.ID,
				BookID:   f.BookID,
				FilePath: f.FilePath,
			}

			changed := false

			// --- 1. original_filename ----------------------------------------
			if f.OriginalFilename == "" {
				f.OriginalFilename = filepath.Base(f.FilePath)
				result.OrigFilenameSet = true
				changed = true
			}

			// --- 2. format from extension -------------------------------------
			if f.Format == "" {
				ext := strings.ToLower(filepath.Ext(f.FilePath))
				if ext != "" {
					newFmt := strings.TrimPrefix(ext, ".")
					result.FormatOld = f.Format
					result.FormatNew = newFmt
					f.Format = newFmt
					changed = true
				}
			}

			// --- 3. file_size from os.Stat ------------------------------------
			// Fix sizes that are zero, suspiciously small (< 1000 bytes, likely
			// a directory inode size), or where the file_path points to a
			// directory instead of an actual audio file.
			if !f.Missing {
				needsSizeFix := f.FileSize == 0 || f.FileSize < 1000
				if info, statErr := os.Stat(f.FilePath); statErr == nil {
					if info.IsDir() {
						// file_path points to a directory, not a file — flag
						// it so it can be fixed. We can't determine the real
						// size without knowing the actual file.
						result.Warning = "file_path is a directory, not an audio file"
						result.FileSizeOld = f.FileSize
						changed = true
					} else if needsSizeFix {
						newSize := info.Size()
						if newSize > 0 && newSize != f.FileSize {
							result.FileSizeOld = f.FileSize
							result.FileSizeNew = newSize
							f.FileSize = newSize
							changed = true
						}
					}
				}
			}

			// --- 4. track_number from filename --------------------------------
			if f.TrackNumber == 0 {
				parsed := parseTrackNumberFromFilename(f.FilePath)
				if parsed > 0 {
					result.TrackNumberOld = f.TrackNumber
					result.TrackNumberNew = parsed
					f.TrackNumber = parsed
					changed = true
				}
			}

			// --- 5. track_count -----------------------------------------------
			if f.TrackCount != trackCount {
				result.TrackCountOld = f.TrackCount
				result.TrackCountNew = trackCount
				f.TrackCount = trackCount
				changed = true
			}

			result.Changed = changed

			if changed {
				totalChanged++
				if !dryRun {
					if upErr := store.UpdateBookFile(f.ID, f); upErr != nil {
						result.Error = upErr.Error()
						totalErrors++
						log.Printf("[WARN] enrich-book-files: UpdateBookFile %s: %v", f.ID, upErr)
					} else {
						result.Applied = true
						totalApplied++
					}
				}
				results = append(results, result)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":       dryRun,
		"books_scanned": len(allBooks),
		"files_changed": totalChanged,
		"applied":       totalApplied,
		"errors":        totalErrors,
		"results":       results,
	})
}

// parseTrackNumberFromFilename extracts a leading track number from an audio
// filename.  Supported patterns (case-insensitive for the "Track" prefix):
//
//	"01 Chapter.mp3"      → 1
//	"02_Head of Dragon.m4b" → 2
//	"003-Epilogue.mp3"    → 3
//	"Track 05.mp3"        → 5
//
// Returns 0 if no number can be determined.
func parseTrackNumberFromFilename(filePath string) int {
	base := filepath.Base(filePath)
	// Remove extension for cleaner matching.
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// Pattern A: optional "Track " prefix, then 1-3 leading digits followed by
	// a non-digit separator (space, underscore, dash, dot) or end-of-string.
	reLeading := regexp.MustCompile(`(?i)^(?:track\s*)?(\d{1,3})(?:[\s_\-.]|$)`)
	if m := reLeading.FindStringSubmatch(name); len(m) > 1 {
		n, err := strconv.Atoi(m[1])
		if err == nil && n > 0 {
			return n
		}
	}

	// Pattern B: entire stem is a number (e.g. "05.mp3").
	reOnly := regexp.MustCompile(`^(\d{1,3})$`)
	if m := reOnly.FindStringSubmatch(name); len(m) > 1 {
		n, err := strconv.Atoi(m[1])
		if err == nil && n > 0 {
			return n
		}
	}

	return 0
}

// ---------------------------------------------------------------------------
// Fix Book File Paths (directory → individual audio files)
// ---------------------------------------------------------------------------

// fixBookFilePathsResult describes the outcome for one book_files row whose
// file_path pointed to a directory (or whose file_size was suspiciously small).
type fixBookFilePathsResult struct {
	FileID      string   `json:"file_id"`
	BookID      string   `json:"book_id"`
	OldPath     string   `json:"old_path"`
	Action      string   `json:"action"`              // "updated", "expanded", "marked_missing", "size_fixed"
	NewPath     string   `json:"new_path,omitempty"`  // for "updated"
	NewPaths    []string `json:"new_paths,omitempty"` // for "expanded"
	FileSizeOld int64    `json:"file_size_old,omitempty"`
	FileSizeNew int64    `json:"file_size_new,omitempty"`
	Applied     bool     `json:"applied"`
	Error       string   `json:"error,omitempty"`
}

// handleFixBookFilePaths iterates every book_files row and:
//
//  1. If file_path doesn't exist but a file with the same stem prefix exists in
//     the same directory, updates the row to the real filename (repairs truncated
//     filenames left by old path-length limiting logic).
//
//  2. If file_path is a directory, globs for audio files inside it:
//     - 1 file found  → update the row's file_path to that file
//     - N>1 files     → create new book_file rows (one per file), delete the directory row
//     - 0 files found → mark the row missing=true
//
//  3. If file_path is a real file and file_size < 100 bytes (likely measured
//     from a directory inode), re-reads the size with os.Stat.
//
// For new/updated rows the handler also populates file_size, format, and
// original_filename from the actual file on disk.
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update/create/delete rows
func (s *Server) handleFixBookFilePaths(c *gin.Context) {
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

	var results []fixBookFilePathsResult
	totalChanged := 0
	totalApplied := 0
	totalErrors := 0

	for i := range allBooks {
		book := &allBooks[i]

		files, bfErr := store.GetBookFiles(book.ID)
		if bfErr != nil {
			log.Printf("[WARN] fix-book-file-paths: GetBookFiles book %s: %v", book.ID, bfErr)
			continue
		}

		for j := range files {
			f := &files[j]

			info, statErr := os.Stat(f.FilePath)
			if statErr != nil {
				// File doesn't exist on disk. Try to find the real file by
				// prefix-matching the truncated stem against siblings in the
				// same directory (handles filenames truncated by old organizer
				// runs that used the wrong path-length calculation).
				dir := filepath.Dir(f.FilePath)
				base := filepath.Base(f.FilePath)
				ext := filepath.Ext(base)
				stem := strings.TrimSuffix(base, ext)

				dirEntries, readErr := os.ReadDir(dir)
				var match string
				if readErr == nil {
					for _, de := range dirEntries {
						if de.IsDir() {
							continue
						}
						name := de.Name()
						nameExt := filepath.Ext(name)
						nameStem := strings.TrimSuffix(name, nameExt)
						// Match: same extension and full name starts with truncated stem
						// Only accept if the real file's name starts with the truncated
						// stem AND the extra characters don't begin with a space.
						// A genuine truncation cuts mid-word, so the suffix should
						// start with a non-space character (e.g. "Unabri" → "Unabridged").
						// A space prefix means we found a different longer variant
						// (e.g. "Book" → "Book 2"), which is not a truncation.
						if strings.EqualFold(nameExt, ext) &&
							strings.HasPrefix(nameStem, stem) &&
							name != base &&
							len(nameStem) > len(stem) &&
							nameStem[len(stem)] != ' ' {
							match = filepath.Join(dir, name)
							break
						}
					}
				}
				if match == "" {
					continue
				}

				fi, statErr2 := os.Stat(match)
				res := fixBookFilePathsResult{
					FileID:      f.ID,
					BookID:      f.BookID,
					OldPath:     f.FilePath,
					NewPath:     match,
					Action:      "truncated_name_repaired",
					FileSizeOld: f.FileSize,
				}
				if statErr2 == nil {
					res.FileSizeNew = fi.Size()
				}
				totalChanged++
				if !dryRun {
					f.FilePath = match
					f.OriginalFilename = filepath.Base(match)
					if statErr2 == nil {
						f.FileSize = fi.Size()
					}
					f.Missing = false
					if upErr := store.UpdateBookFile(f.ID, f); upErr != nil {
						res.Error = upErr.Error()
						totalErrors++
					} else {
						res.Applied = true
						totalApplied++
					}
				}
				results = append(results, res)
				continue
			}

			if info.IsDir() {
				// file_path points to a directory — find real audio files.
				audioFiles := metafetch.AudioFilesInDir(f.FilePath)

				switch len(audioFiles) {
				case 0:
					// No audio files found — mark as missing.
					res := fixBookFilePathsResult{
						FileID:  f.ID,
						BookID:  f.BookID,
						OldPath: f.FilePath,
						Action:  "marked_missing",
					}
					totalChanged++
					if !dryRun {
						f.Missing = true
						if upErr := store.UpdateBookFile(f.ID, f); upErr != nil {
							res.Error = upErr.Error()
							totalErrors++
						} else {
							res.Applied = true
							totalApplied++
						}
					}
					results = append(results, res)

				case 1:
					// Exactly one file — update the existing row.
					audioPath := audioFiles[0]
					fi2, statErr2 := os.Stat(audioPath)
					res := fixBookFilePathsResult{
						FileID:  f.ID,
						BookID:  f.BookID,
						OldPath: f.FilePath,
						NewPath: audioPath,
						Action:  "updated",
					}
					totalChanged++
					if !dryRun {
						f.FilePath = audioPath
						f.OriginalFilename = filepath.Base(audioPath)
						ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(audioPath), "."))
						if ext != "" {
							f.Format = ext
						}
						if statErr2 == nil {
							res.FileSizeOld = f.FileSize
							res.FileSizeNew = fi2.Size()
							f.FileSize = fi2.Size()
						}
						f.Missing = false
						if upErr := store.UpdateBookFile(f.ID, f); upErr != nil {
							res.Error = upErr.Error()
							totalErrors++
						} else {
							res.Applied = true
							totalApplied++
						}
					}
					results = append(results, res)

				default:
					// Multiple files — expand into individual rows.
					res := fixBookFilePathsResult{
						FileID:   f.ID,
						BookID:   f.BookID,
						OldPath:  f.FilePath,
						NewPaths: audioFiles,
						Action:   "expanded",
					}
					totalChanged++
					if !dryRun {
						applyErr := false
						for _, audioPath := range audioFiles {
							fi3, statErr3 := os.Stat(audioPath)
							newFile := &database.BookFile{
								ID:               ulid.Make().String(),
								BookID:           f.BookID,
								FilePath:         audioPath,
								OriginalFilename: filepath.Base(audioPath),
								Format:           strings.ToLower(strings.TrimPrefix(filepath.Ext(audioPath), ".")),
								Missing:          statErr3 != nil,
							}
							if statErr3 == nil {
								newFile.FileSize = fi3.Size()
							}
							if crErr := store.CreateBookFile(newFile); crErr != nil {
								res.Error = fmt.Sprintf("CreateBookFile %s: %v", audioPath, crErr)
								totalErrors++
								applyErr = true
								break
							}
						}
						if !applyErr {
							if delErr := store.DeleteBookFile(f.ID); delErr != nil {
								res.Error = fmt.Sprintf("DeleteBookFile %s: %v", f.ID, delErr)
								totalErrors++
							} else {
								res.Applied = true
								totalApplied++
							}
						}
					}
					results = append(results, res)
				}
				continue
			}

			// file_path is a real file — check for suspiciously small file_size
			// (< 100 bytes likely came from os.Stat on a directory inode).
			if !f.Missing && f.FileSize < 100 {
				realSize := info.Size()
				if realSize != f.FileSize {
					res := fixBookFilePathsResult{
						FileID:      f.ID,
						BookID:      f.BookID,
						OldPath:     f.FilePath,
						Action:      "size_fixed",
						FileSizeOld: f.FileSize,
						FileSizeNew: realSize,
					}
					totalChanged++
					if !dryRun {
						f.FileSize = realSize
						if upErr := store.UpdateBookFile(f.ID, f); upErr != nil {
							res.Error = upErr.Error()
							totalErrors++
						} else {
							res.Applied = true
							totalApplied++
						}
					}
					results = append(results, res)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":       dryRun,
		"books_scanned": len(allBooks),
		"files_changed": totalChanged,
		"applied":       totalApplied,
		"errors":        totalErrors,
		"results":       results,
	})
}
