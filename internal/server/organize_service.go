// file: internal/server/organize_service.go
// version: 1.27.0
// guid: c3d4e5f6-a7b8-c9d0-e1f2-a3b4c5d6e7f8

package server

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/backup"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	ulid "github.com/oklog/ulid/v2"
)

type OrganizeService struct {
	db               database.Store
	organizeHooks    organizer.OrganizeHooks
	writeBackBatcher *WriteBackBatcher
	queue            operations.Queue
}

// SetWriteBackBatcher sets the iTunes write-back batcher.
func (orgSvc *OrganizeService) SetWriteBackBatcher(b *WriteBackBatcher) {
	orgSvc.writeBackBatcher = b
}

// SetQueue sets the operation queue for enqueuing background operations.
func (orgSvc *OrganizeService) SetQueue(q operations.Queue) {
	orgSvc.queue = q
}

// SetOrganizeHooks sets optional hooks that are propagated to every
// Organizer instance created by this service.
func (orgSvc *OrganizeService) SetOrganizeHooks(hooks organizer.OrganizeHooks) {
	orgSvc.organizeHooks = hooks
}

// newOrganizer creates an Organizer with the service's hooks pre-wired.
func (orgSvc *OrganizeService) newOrganizer() *organizer.Organizer {
	org := organizer.NewOrganizer(&config.AppConfig)
	if orgSvc.organizeHooks != nil {
		org.SetHooks(orgSvc.organizeHooks)
	}
	return org
}

func NewOrganizeService(db database.Store) *OrganizeService {
	return &OrganizeService{db: db}
}

type OrganizeRequest struct {
	FolderPath         *string
	Priority           *int
	FetchMetadataFirst bool
	SyncITunesFirst    bool
	OperationID        string
	BookIDs            []string // if set, only organize these books
}

type OrganizeStats struct {
	Organized      int
	ReOrganized    int
	AlreadyCorrect int
	Skipped        int // soft-deleted / non-primary / missing file skips
	Failed         int
	Total          int
}

// PerformOrganizeWithID executes organization with checkpoint support.
func (orgSvc *OrganizeService) PerformOrganizeWithID(ctx context.Context, opID string, req *OrganizeRequest, log logger.Logger) error {
	_ = operations.SaveParams(orgSvc.db, opID, operations.OrganizeParams{})
	req.OperationID = opID
	err := orgSvc.PerformOrganize(ctx, req, log)
	_ = operations.ClearState(orgSvc.db, opID)
	return err
}

// PerformOrganize executes the library organization operation
func (orgSvc *OrganizeService) PerformOrganize(ctx context.Context, req *OrganizeRequest, log logger.Logger) error {
	log.Info("Starting file organization")

	// Optional: sync iTunes library first to ensure all books are up to date
	if req.SyncITunesFirst {
		orgSvc.syncITunesBeforeOrganize(ctx, log)
	}

	// Auto-backup database before organizing
	orgSvc.autoBackup(log)

	// Get books — either specific IDs or all books
	const fetchPageSize = 1000
	var allBooks []database.Book
	if len(req.BookIDs) > 0 {
		for _, id := range req.BookIDs {
			book, err := orgSvc.db.GetBookByID(id)
			if err != nil || book == nil {
				log.Warn("Book %s not found, skipping", id)
				continue
			}
			allBooks = append(allBooks, *book)
		}
	} else {
		for offset := 0; ; offset += fetchPageSize {
			page, fetchErr := orgSvc.db.GetAllBooks(fetchPageSize, offset)
			if fetchErr != nil {
				log.Error("Failed to fetch books: %s", fetchErr.Error())
				return fmt.Errorf("failed to fetch books: %w", fetchErr)
			}
			allBooks = append(allBooks, page...)
			if len(page) < fetchPageSize {
				break
			}
		}
	}

	logMsg := fmt.Sprintf("Fetched %d total books from database", len(allBooks))
	log.Info("%s", logMsg)
	log.Debug("Organize: %s", logMsg)

	// Optional: fetch metadata before organizing to normalize author names
	if req.FetchMetadataFirst {
		log.Info("Fetching metadata before organizing...")
		mfs := NewMetadataFetchService(orgSvc.db)
		enriched := 0
		for i := range allBooks {
			book := &allBooks[i]
			if book.CoverURL != nil {
				continue // already enriched
			}
			if _, err := mfs.FetchMetadataForBook(book.ID); err == nil {
				enriched++
			}
		}
		log.Info("Metadata enriched for %d books", enriched)

		// Re-fetch all books since metadata may have changed
		allBooks = nil
		for offset := 0; ; offset += fetchPageSize {
			page, fetchErr := orgSvc.db.GetAllBooks(fetchPageSize, offset)
			if fetchErr != nil {
				return fmt.Errorf("failed to re-fetch books after metadata: %w", fetchErr)
			}
			allBooks = append(allBooks, page...)
			if len(page) < fetchPageSize {
				break
			}
		}
	}

	// Filter books that need organizing
	booksToOrganize, alreadyCorrect := orgSvc.filterBooksNeedingOrganization(allBooks, log)

	logMsg = fmt.Sprintf("Found %d books that need organizing, %d already correct (out of %d total)",
		len(booksToOrganize), len(alreadyCorrect), len(allBooks))
	log.Info("%s", logMsg)
	log.Debug("Organize: %s", logMsg)

	// Perform organization
	stats := orgSvc.organizeBooks(ctx, booksToOrganize, alreadyCorrect, log, req.OperationID)

	// Post-organize auto write-back now rides the batcher.
	// Previously this ran a bulk location-only write via
	// collectITLUpdatesWithBookIDs + UpdateITLLocations inline.
	// That path (a) skipped metadata refresh, (b) only read
	// book.ITunesPath (stale for older organize runs), and
	// (c) raced with the batcher writing to the same file.
	// Each organize worker now calls orgSvc.writeBackBatcher.Enqueue
	// per book; the batcher flushes once after its debounce with
	// both location + metadata updates and calls MarkITunesSynced
	// on success.
	if stats.Organized > 0 || stats.ReOrganized > 0 {
		// Note: auto-rescan disabled — organize already updates all paths and book_files.
		// A rescan after organize can trigger another organize, creating an infinite loop.
		// If a rescan is needed, trigger it manually.
	}

	return nil
}

func (orgSvc *OrganizeService) autoBackup(log logger.Logger) {
	dbPath := config.AppConfig.DatabasePath
	dbType := config.AppConfig.DatabaseType
	if dbPath == "" {
		log.Warn("Skipping auto-backup: no database path configured")
		return
	}

	backupConfig := backup.DefaultBackupConfig()
	if !filepath.IsAbs(backupConfig.BackupDir) {
		backupConfig.BackupDir = filepath.Join(filepath.Dir(dbPath), backupConfig.BackupDir)
	}

	info, err := backup.CreateBackup(dbPath, dbType, backupConfig)
	if err != nil {
		log.Warn("Auto-backup failed: %s", err.Error())
		return
	}
	log.Info("Auto-backup created: %s (%d bytes)", info.Filename, info.Size)
}

func (orgSvc *OrganizeService) syncITunesBeforeOrganize(ctx context.Context, log logger.Logger) {
	libraryPath := discoverITunesLibraryPath(orgSvc.db)
	if libraryPath == "" {
		log.Info("Skipping iTunes sync: no library found")
		return
	}

	log.Info("Running iTunes sync before organize: %s", libraryPath)

	if err := executeITunesSync(ctx, orgSvc.db, log, libraryPath, nil, nil); err != nil {
		log.Warn("iTunes pre-sync failed (continuing with organize): %s", err.Error())
		return
	}

	log.Info("iTunes sync completed successfully")
}

func (orgSvc *OrganizeService) filterBooksNeedingOrganization(allBooks []database.Book, log logger.Logger) ([]database.Book, []database.Book) {
	booksToOrganize := make([]database.Book, 0)
	alreadyCorrect := make([]database.Book, 0)
	skippedMissingFiles := 0
	skippedDeleted := 0
	for i, book := range allBooks {
		// Update progress during filtering so the UI doesn't show 0/0
		if i%500 == 0 || i == len(allBooks)-1 {
			log.UpdateProgress(i, len(allBooks), fmt.Sprintf("Scanning: %d/%d books", i, len(allBooks)))
		}

		// Skip soft-deleted books
		if book.MarkedForDeletion != nil && *book.MarkedForDeletion {
			skippedDeleted++
			continue
		}

		// Skip non-primary versions — unless they're the only version in their VG
		// (i.e., no organized primary copy exists yet)
		if book.IsPrimaryVersion != nil && !*book.IsPrimaryVersion {
			if book.VersionGroupID != nil && *book.VersionGroupID != "" {
				vgBooks, vgErr := orgSvc.db.GetBooksByVersionGroup(*book.VersionGroupID)
				if vgErr == nil {
					hasPrimary := false
					for _, vb := range vgBooks {
						if vb.IsPrimaryVersion != nil && *vb.IsPrimaryVersion {
							hasPrimary = true
							break
						}
					}
					if hasPrimary {
						continue // Has a primary version — skip this non-primary
					}
					// No primary exists yet — allow organize to create one
				}
			} else {
				continue
			}
		}
		// If already in root directory, check if path needs updating based on current metadata
		if config.AppConfig.RootDir != "" && strings.HasPrefix(book.FilePath, config.AppConfig.RootDir) {
			needsReOrganize, err := orgSvc.bookNeedsReOrganize(&book, log)
			if err != nil {
				log.Debug("Organize: Cannot compute target for %s: %s", book.Title, err.Error())
				continue
			}
			if !needsReOrganize {
				// Already in correct location — collect for stamping, don't log individually
				alreadyCorrect = append(alreadyCorrect, book)
				continue
			}
			log.Info("Organize: Book in RootDir needs re-organization: %s", book.Title)
			// Fall through to include in organize list
		}
		// Quick check: skip if file_path is empty
		if book.FilePath == "" {
			continue
		}
		// For books outside RootDir, rely on book_files to determine readiness.
		// Avoid os.Stat on 140K+ paths during filter — that was the main bottleneck.
		// organizeBook() will skip individual missing files when it runs.
		if config.AppConfig.RootDir == "" || !strings.HasPrefix(book.FilePath, config.AppConfig.RootDir) {
			bookFiles, bfErr := orgSvc.db.GetBookFiles(book.ID)
			if bfErr != nil || len(bookFiles) == 0 {
				// No book_files: can't organize without knowing which files to copy.
				log.Debug("Organize: Skipping %s — no book_files in DB", book.Title)
				skippedMissingFiles++
				continue
			}
			// Count how many active (non-missing) book files exist
			activeCount := 0
			for _, bf := range bookFiles {
				if bf.FilePath != "" && !bf.Missing {
					activeCount++
				}
			}
			if activeCount == 0 {
				log.Debug("Organize: Skipping %s — all book_files marked missing", book.Title)
				skippedMissingFiles++
				continue
			}
		}
		booksToOrganize = append(booksToOrganize, book)
	}
	if skippedDeleted > 0 {
		log.Info("Organize: Skipped %d soft-deleted book(s)", skippedDeleted)
	}
	if skippedMissingFiles > 0 {
		log.Info("Organize: Skipped %d book(s) with missing book files", skippedMissingFiles)
	}
	return booksToOrganize, alreadyCorrect
}

// bookNeedsReOrganize checks whether a book already in RootDir needs to be
// moved because its current path doesn't match the target path derived from
// current metadata.
func (orgSvc *OrganizeService) bookNeedsReOrganize(book *database.Book, log logger.Logger) (bool, error) {
	org := orgSvc.newOrganizer()

	// Determine dir vs file by extension — avoids os.Stat (the main scan bottleneck)
	ext := strings.ToLower(filepath.Ext(book.FilePath))
	audioExts := map[string]bool{".m4b": true, ".m4a": true, ".mp3": true, ".flac": true, ".ogg": true, ".opus": true, ".wma": true, ".aac": true}
	isFile := audioExts[ext]

	if !isFile {
		targetDir, err := org.GenerateTargetDirPath(book)
		if err != nil {
			return false, err
		}
		return book.FilePath != targetDir, nil
	}

	targetPath, err := org.GenerateTargetPath(book)
	if err != nil {
		return false, err
	}
	return book.FilePath != targetPath, nil
}

// reOrganizeInPlace renames/moves a book that is already in RootDir to its
// correct location based on current metadata. Returns the new path.
func (orgSvc *OrganizeService) reOrganizeInPlace(book *database.Book, log logger.Logger) (string, error) {
	org := orgSvc.newOrganizer()
	oldPath := book.FilePath

	info, err := os.Stat(oldPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("source path no longer exists: %s — re-scan the library to update tracking", oldPath)
		}
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied reading source: %s — check filesystem permissions and ACLs", oldPath)
		}
		return "", fmt.Errorf("cannot access source %s: %w", oldPath, err)
	}

	var targetPath string
	if info.IsDir() {
		targetPath, err = org.GenerateTargetDirPath(book)
	} else {
		targetPath, err = org.GenerateTargetPath(book)
	}
	if err != nil {
		return "", err
	}

	if oldPath == targetPath {
		// Already in correct location — still stamp as organized
		organizedState := "organized"
		book.LibraryState = &organizedState
		now := time.Now()
		book.LastOrganizedAt = &now
		orgSvc.db.UpdateBook(book.ID, book)
		return targetPath, nil
	}

	// Create parent directory for target
	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0775); err != nil {
		return "", fmt.Errorf("cannot create target directory %s: %w (check parent permissions and disk space)", parentDir, err)
	}

	// Rename (move) the file or directory
	if err := os.Rename(oldPath, targetPath); err != nil {
		return "", fmt.Errorf("cannot move %s -> %s: %w (verify both paths exist, target not in use, same filesystem, write permission)", oldPath, targetPath, err)
	}

	// Update the book record — set path and mark as organized.
	// ITunesPath is kept in sync so iTunes writeback sends the new
	// location. Without this, iTunes keeps pointing at the old path
	// and shows "file missing."
	book.FilePath = targetPath
	newITunesPath := computeITunesPath(targetPath)
	book.ITunesPath = &newITunesPath
	organizedState := "organized"
	book.LibraryState = &organizedState
	now := time.Now()
	book.LastOrganizedAt = &now
	if _, err := orgSvc.db.UpdateBook(book.ID, book); err != nil {
		log.Warn("Failed to update book path for %s: %s", book.ID, err.Error())
	}

	// Update book_files paths if this is a directory book
	if info.IsDir() {
		if bookFiles, bfErr := orgSvc.db.GetBookFiles(book.ID); bfErr == nil {
			for _, bf := range bookFiles {
				if strings.HasPrefix(bf.FilePath, oldPath) {
					bf.FilePath = filepath.Join(targetPath, strings.TrimPrefix(bf.FilePath, oldPath+"/"))
					if bf.FilePath != "" {
						bf.ITunesPath = computeITunesPath(bf.FilePath)
					}
					_ = orgSvc.db.UpdateBookFile(bf.ID, &bf)
				}
			}
		}
	}

	// Try to remove the now-empty parent directory tree
	orgSvc.cleanupEmptyParents(filepath.Dir(oldPath), config.AppConfig.RootDir, log)

	log.Info("Re-organized: %s → %s", oldPath, targetPath)
	return targetPath, nil
}

// cleanupEmptyParents removes empty directories from dir up to (but not
// including) stopAt.
func (orgSvc *OrganizeService) cleanupEmptyParents(dir, stopAt string, log logger.Logger) {
	for dir != stopAt && strings.HasPrefix(dir, stopAt) && dir != "/" {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		if err := os.Remove(dir); err != nil {
			log.Debug("Could not remove empty dir %s: %s", dir, err.Error())
			break
		}
		log.Debug("Removed empty directory: %s", dir)
		dir = filepath.Dir(dir)
	}
}

func (orgSvc *OrganizeService) organizeBooks(ctx context.Context, booksToOrganize []database.Book, alreadyCorrect []database.Book, log logger.Logger, operationID string) *OrganizeStats {
	stats := &OrganizeStats{Total: len(booksToOrganize) + len(alreadyCorrect)}

	// Thread-safe counters and collectors
	var statsMu sync.Mutex
	var progressCounter int64

	const numWorkers = 8
	jobs := make(chan int, numWorkers*2)

	// Start worker goroutines — each handles the FULL pipeline (file ops + DB writes)
	// for one book. Each book's DB operations are independent, so all writes can
	// happen in parallel. This eliminates the serial collector bottleneck.
	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			workerOrg := orgSvc.newOrganizer()

			for i := range jobs {
				book := booksToOrganize[i]
				oldPath := book.FilePath
				isDir := false
				if info, err := os.Stat(oldPath); err == nil && info.IsDir() {
					isDir = true
				}
				alreadyInRoot := config.AppConfig.RootDir != "" && strings.HasPrefix(oldPath, config.AppConfig.RootDir)

				// --- Step 1: File operations (the parallelizable part) ---
				var newPath string
				var err error

				if alreadyInRoot {
					newPath, err = orgSvc.reOrganizeInPlace(&book, log)
				} else if isDir {
					newPath, err = orgSvc.organizeDirectoryBook(workerOrg, &book, log)
				} else {
					newPath, _, err = workerOrg.OrganizeBook(&book)
				}

				// --- Step 2: DB operations (independent per book) ---
				if err != nil {
					log.Warn("Failed to organize %s: %s", book.Title, err.Error())
					statsMu.Lock()
					stats.Failed++
					statsMu.Unlock()

					if operationID != "" {
						_ = orgSvc.db.CreateOperationChange(&database.OperationChange{
							ID:          ulid.Make().String(),
							OperationID: operationID,
							BookID:      book.ID,
							ChangeType:  "organize_failed",
							FieldName:   "file_path",
							OldValue:    oldPath,
							NewValue:    err.Error(),
						})
					}
				} else if oldPath == newPath {
					// Already in correct location — stamp and count
					now := time.Now()
					book.LastOrganizeOperationID = &operationID
					book.LastOrganizedAt = &now
					if _, updateErr := orgSvc.db.UpdateBook(book.ID, &book); updateErr != nil {
						log.Debug("Organize: failed to stamp book %s: %s", book.ID, updateErr.Error())
					}
					statsMu.Lock()
					stats.AlreadyCorrect++
					statsMu.Unlock()

					if operationID != "" {
						_ = orgSvc.db.CreateOperationChange(&database.OperationChange{
							ID:          ulid.Make().String(),
							OperationID: operationID,
							BookID:      book.ID,
							ChangeType:  "organize_skipped",
							FieldName:   "file_path",
							OldValue:    oldPath,
							NewValue:    oldPath,
						})
					}
				} else if alreadyInRoot {
					// Re-organized in place — stamp and record the rename.
					now := time.Now()
					book.LastOrganizeOperationID = &operationID
					book.LastOrganizedAt = &now
					if _, updateErr := orgSvc.db.UpdateBook(book.ID, &book); updateErr != nil {
						log.Debug("Organize: failed to stamp re-organized book %s: %s", book.ID, updateErr.Error())
					}
					log.Info("Re-organized %s: %s → %s", book.Title, oldPath, newPath)
					statsMu.Lock()
					stats.ReOrganized++
					statsMu.Unlock()

					if operationID != "" {
						_ = orgSvc.db.CreateOperationChange(&database.OperationChange{
							ID:          ulid.Make().String(),
							OperationID: operationID,
							BookID:      book.ID,
							ChangeType:  "organize_rename",
							FieldName:   "file_path",
							OldValue:    oldPath,
							NewValue:    newPath,
						})
						oldState := ""
						if book.LibraryState != nil {
							oldState = *book.LibraryState
						}
						_ = orgSvc.db.CreateOperationChange(&database.OperationChange{
							ID:          ulid.Make().String(),
							OperationID: operationID,
							BookID:      book.ID,
							ChangeType:  "metadata_update",
							FieldName:   "library_state",
							OldValue:    oldState,
							NewValue:    "organized",
						})
					}
				} else {
					// Version-aware organize: create a new book record for the organized copy
					createdBook, createErr := orgSvc.createOrganizedVersion(workerOrg, &book, newPath, isDir, operationID, log)
					if createErr != nil {
						statsMu.Lock()
						stats.Failed++
						statsMu.Unlock()
						goto progress
					}

					// Stamp the new organized book record with this operation
					now := time.Now()
					createdBook.LastOrganizeOperationID = &operationID
					createdBook.LastOrganizedAt = &now
					if _, updateErr := orgSvc.db.UpdateBook(createdBook.ID, createdBook); updateErr != nil {
						log.Debug("Organize: failed to stamp new book %s: %s", createdBook.ID, updateErr.Error())
					}

					log.Info("Organized %s: created version %s → %s (original kept at %s)",
						book.Title, createdBook.ID, newPath, oldPath)

					statsMu.Lock()
					stats.Organized++
					statsMu.Unlock()
				}

				// --- Step 3: Enqueue iTunes writeback ---
				// Route through the global batcher so location + metadata
				// updates ride together. The batcher iterates book_files to
				// pick up per-segment PIDs (multi-file books), falling back
				// to book.ITunesPersistentID for single-file books.
				if err == nil && oldPath != newPath && orgSvc.writeBackBatcher != nil {
					orgSvc.writeBackBatcher.Enqueue(book.ID)
				}

			progress:
				// --- Step 4: Progress reporting ---
				count := atomic.AddInt64(&progressCounter, 1)
				if count%50 == 0 || count == int64(len(booksToOrganize)) {
					log.UpdateProgress(int(count), len(booksToOrganize),
						fmt.Sprintf("Organizing: %d/%d books", count, len(booksToOrganize)))
				}
			}
		}()
	}

	// Feed jobs — cancellation checked here.
	for i := range booksToOrganize {
		if log.IsCanceled() {
			log.Info("Organize canceled")
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	// Stamp already-correct books with this operation ID (sequential — bulk stamp)
	if operationID != "" && len(alreadyCorrect) > 0 {
		stampNow := time.Now()
		for i := range alreadyCorrect {
			b := &alreadyCorrect[i]
			b.LastOrganizeOperationID = &operationID
			b.LastOrganizedAt = &stampNow
			if _, updateErr := orgSvc.db.UpdateBook(b.ID, b); updateErr != nil {
				log.Debug("Organize: failed to stamp already-correct book %s: %s", b.ID, updateErr.Error())
			}
		}
		stats.AlreadyCorrect += len(alreadyCorrect)
	}

	// iTunes writeback happens via orgSvc.writeBackBatcher (enqueued per
	// book inside the worker loop above). The batcher handles both
	// location and metadata updates and correctly iterates book_files
	// to pick up per-segment PIDs for multi-file books.

	summary := fmt.Sprintf("Organize complete: %d organized, %d re-organized, %d already correct (stamped), %d skipped",
		stats.Organized, stats.ReOrganized, stats.AlreadyCorrect, stats.Skipped)
	log.Info("%s", summary)

	// Record summary as operation change
	if operationID != "" {
		_ = orgSvc.db.CreateOperationChange(&database.OperationChange{
			ID:          ulid.Make().String(),
			OperationID: operationID,
			BookID:      "",
			ChangeType:  "organize_summary",
			FieldName:   "stats",
			OldValue:    "",
			NewValue: fmt.Sprintf("organized:%d re_organized:%d already_correct:%d skipped:%d failed:%d total:%d",
				stats.Organized, stats.ReOrganized, stats.AlreadyCorrect, stats.Skipped, stats.Failed, stats.Total),
		})
	}

	return stats
}

// organizeDirectoryBook handles organizing a multi-file book where file_path is a directory.
// It always uses book_files from the database — no directory scanning fallback.
// Returns the target directory path.
func (orgSvc *OrganizeService) organizeDirectoryBook(org *organizer.Organizer, book *database.Book, log logger.Logger) (string, error) {
	bookFiles, err := orgSvc.db.GetBookFiles(book.ID)
	if err != nil {
		return "", fmt.Errorf("cannot load segments for %s (%s): %w", book.Title, book.ID, err)
	}
	if len(bookFiles) == 0 {
		return "", fmt.Errorf("no segments tracked for %q (id=%s) — run a library scan to detect files in %s", book.Title, book.ID, book.FilePath)
	}

	var segmentPaths []string
	missingCount := 0
	for _, bf := range bookFiles {
		if bf.FilePath == "" {
			continue
		}
		if bf.Missing {
			missingCount++
			continue
		}
		segmentPaths = append(segmentPaths, bf.FilePath)
	}

	if len(segmentPaths) == 0 {
		return "", fmt.Errorf("all %d segments for %q (id=%s) marked missing on disk — re-scan to verify, or restore from backup", missingCount, book.Title, book.ID)
	}

	log.Info("Organizing %d segment file(s) for %s (from book_files)", len(segmentPaths), book.Title)

	targetDir, pathMap, err := org.OrganizeBookDirectory(book, segmentPaths)
	if err != nil {
		return "", err
	}

	// Verify at least some files were actually copied to the target
	if len(pathMap) == 0 {
		return "", fmt.Errorf("no files were copied for %s — all source files missing", book.Title)
	}

	// Check how many files actually exist in the target directory
	copiedCount := 0
	for _, dstPath := range pathMap {
		if _, statErr := os.Stat(dstPath); statErr == nil {
			copiedCount++
		}
	}
	if copiedCount == 0 {
		return "", fmt.Errorf("organize produced 0 files for %s — all copies failed", book.Title)
	}

	return targetDir, nil
}

// createOrganizedVersion creates a new book record for the organized copy and links it to the original.
func (orgSvc *OrganizeService) createOrganizedVersion(org *organizer.Organizer, book *database.Book, newPath string, isDir bool, operationID string, log logger.Logger) (*database.Book, error) {
	newBookID := ulid.Make().String()
	isPrimary := true
	isNotPrimary := false
	organizedState := "organized"

	// Determine or create version group
	versionGroupID := ""
	if book.VersionGroupID != nil && *book.VersionGroupID != "" {
		versionGroupID = *book.VersionGroupID
	} else {
		versionGroupID = ulid.Make().String()
	}

	// Create the new organized book record (copy of metadata)
	newBook := database.Book{
		ID:                   newBookID,
		Title:                book.Title,
		AuthorID:             book.AuthorID,
		Narrator:             book.Narrator,
		SeriesID:             book.SeriesID,
		SeriesSequence:       book.SeriesSequence,
		FilePath:             newPath,
		Format:               book.Format,
		FileSize:             book.FileSize,
		FileHash:             book.FileHash,
		OriginalFileHash:     book.OriginalFileHash,
		Duration:             book.Duration,
		Bitrate:              book.Bitrate,
		SampleRate:           book.SampleRate,
		Channels:             book.Channels,
		BitDepth:             book.BitDepth,
		Codec:                book.Codec,
		Edition:              book.Edition,
		Language:             book.Language,
		Publisher:            book.Publisher,
		PrintYear:            book.PrintYear,
		AudiobookReleaseYear: book.AudiobookReleaseYear,
		ISBN10:               book.ISBN10,
		ISBN13:               book.ISBN13,
		ASIN:                 book.ASIN,
		CoverURL:             book.CoverURL,
		OpenLibraryID:        book.OpenLibraryID,
		HardcoverID:          book.HardcoverID,
		GoogleBooksID:        book.GoogleBooksID,
		OriginalFilename:     book.OriginalFilename,
		LibraryState:         &organizedState,
		VersionGroupID:       &versionGroupID,
		IsPrimaryVersion:     &isPrimary,
		Quality:              book.Quality,
	}

	if !isDir {
		applyOrganizedFileMetadata(&newBook, newPath)
	}

	createdBook, err := orgSvc.db.CreateBook(&newBook)
	if err != nil {
		log.Error("Failed to create organized book record for %s: %v", book.Title, err)
		// Best-effort cleanup of the files we just placed at newPath so
		// we don't leak orphan files (single-file) or orphan directories
		// (multi-file) with no DB row pointing at them. Every removal is
		// safety-gated on newPath being under RootDir so a broken newPath
		// can't accidentally rm something outside the managed library.
		if newPath != "" && config.AppConfig.RootDir != "" && strings.HasPrefix(newPath, config.AppConfig.RootDir) {
			if isDir {
				if rmErr := os.RemoveAll(newPath); rmErr != nil {
					log.Warn("organize: cleanup of partial directory organize failed (%s): %v", newPath, rmErr)
				} else {
					log.Warn("organize: cleaned up partial directory organize at %s after CreateBook failure", newPath)
				}
			} else {
				if rmErr := os.Remove(newPath); rmErr != nil {
					log.Warn("organize: cleanup of partial single-file organize failed (%s): %v", newPath, rmErr)
				}
			}
		}
		return nil, err
	}
	// Mark both the organized copy and the original for rescan so the next
	// incremental scan picks up the moved/new file location.
	_ = orgSvc.db.MarkNeedsRescan(createdBook.ID)
	_ = orgSvc.db.MarkNeedsRescan(book.ID)

	// Copy book_authors relationships to the new book
	if authors, err := orgSvc.db.GetBookAuthors(book.ID); err == nil && len(authors) > 0 {
		var newAuthors []database.BookAuthor
		for _, ba := range authors {
			newAuthors = append(newAuthors, database.BookAuthor{
				BookID:   newBookID,
				AuthorID: ba.AuthorID,
				Role:     ba.Role,
			})
		}
		_ = orgSvc.db.SetBookAuthors(newBookID, newAuthors)
	}

	// Copy book files to the new book with updated paths
	if bookFiles, err := orgSvc.db.GetBookFiles(book.ID); err == nil && len(bookFiles) > 0 {
		for _, bf := range bookFiles {
			newBF := bf
			newBF.ID = ulid.Make().String()
			newBF.BookID = newBookID
			// Update file paths to point to the organized location
			if isDir && bf.FilePath != "" {
				fileName := filepath.Base(bf.FilePath)
				newBF.FilePath = filepath.Join(newPath, fileName)
			} else if !isDir {
				newBF.FilePath = newPath
			}
			// ALWAYS recompute itunes_path from the new file_path so that
			// files in the audiobook-organizer folder don't keep a stale
			// W:/itunes/... path from the original iTunes source.
			if newBF.FilePath != "" {
				newBF.ITunesPath = computeITunesPath(newBF.FilePath)
			}
			_ = orgSvc.db.CreateBookFile(&newBF)
		}
	}

	// Update original book: set version group, mark as non-primary, update state
	organizedSourceState := "organized_source" // has an organized copy — no longer "needs organizing"
	book.VersionGroupID = &versionGroupID
	book.IsPrimaryVersion = &isNotPrimary
	book.LibraryState = &organizedSourceState
	if _, err := orgSvc.db.UpdateBook(book.ID, book); err != nil {
		log.Warn("Failed to update original book %s version group: %v", book.ID, err)
	}

	// Record operation changes for undo
	if operationID != "" {
		_ = orgSvc.db.CreateOperationChange(&database.OperationChange{
			ID:          ulid.Make().String(),
			OperationID: operationID,
			BookID:      createdBook.ID,
			ChangeType:  "book_create",
			FieldName:   "organized_version",
			OldValue:    "",
			NewValue:    fmt.Sprintf("version_of:%s path:%s", book.ID, newPath),
		})
		_ = orgSvc.db.CreateOperationChange(&database.OperationChange{
			ID:          ulid.Make().String(),
			OperationID: operationID,
			BookID:      book.ID,
			ChangeType:  "metadata_update",
			FieldName:   "version_group_id",
			OldValue:    "",
			NewValue:    versionGroupID,
		})
	}

	return createdBook, nil
}

func (orgSvc *OrganizeService) triggerAutomaticRescan(ctx context.Context, log logger.Logger) {
	if config.AppConfig.RootDir == "" {
		return
	}

	log.Info("Starting automatic rescan of library path...")

	// Create a new scan operation
	scanID := ulid.Make().String()
	scanOp, err := orgSvc.db.CreateOperation(scanID, "scan", &config.AppConfig.RootDir)
	if err != nil {
		log.Warn("Failed to create rescan operation: %s", err.Error())
		return
	}

	// Enqueue the scan operation with low priority
	scanFunc := func(ctx context.Context, scanProgress operations.ProgressReporter) error {
		scanLog := operations.LoggerFromReporter(scanProgress)
		scanLog.Info("Scanning organized books in: %s", config.AppConfig.RootDir)

		workers := config.AppConfig.ConcurrentScans
		if workers < 1 {
			workers = 4
		}

		scanLog.Info("Starting directory scan with %d workers", workers)
		books, err := scanner.ScanDirectoryParallel(config.AppConfig.RootDir, workers, scanLog)
		if err != nil {
			return fmt.Errorf("failed to rescan root directory: %w", err)
		}

		scanLog.Info("Found %d books in root directory, processing metadata", len(books))

		// Process the books to extract metadata with progress reporting
		if len(books) > 0 {
			var processedFiles atomic.Int64
			totalBooks := len(books)

			progressCallback := func(_ int, _ int, bookPath string) {
				current := processedFiles.Add(1)
				displayTotal := totalBooks
				if int(current) > displayTotal {
					displayTotal = int(current)
				}
				message := fmt.Sprintf("Processed: %d/%d books", current, displayTotal)
				if bookPath != "" {
					message = fmt.Sprintf("Processed: %d/%d books (%s)", current, displayTotal, filepath.Base(bookPath))
				}
				scanLog.UpdateProgress(int(current), displayTotal, message)
			}

			scanLog.Info("Processing metadata for %d books using %d workers", totalBooks, workers)
			if err := scanner.ProcessBooksParallel(ctx, books, workers, progressCallback, scanLog); err != nil {
				return fmt.Errorf("failed to process books: %w", err)
			}
			scanLog.Info("Metadata processing complete: %d books processed", processedFiles.Load())
		}

		scanLog.Info("Rescan completed successfully")
		return nil
	}

	if orgSvc.queue != nil {
		if err := orgSvc.queue.Enqueue(scanOp.ID, "scan", operations.PriorityLow, scanFunc); err != nil {
			log.Warn("Failed to enqueue rescan: %s", err.Error())
		} else {
			log.Info("Rescan operation queued successfully")
		}
	}
}
