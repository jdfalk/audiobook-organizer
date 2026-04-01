// file: internal/server/organize_service.go
// version: 1.15.0
// guid: c3d4e5f6-a7b8-c9d0-e1f2-a3b4c5d6e7f8

package server

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/backup"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	ulid "github.com/oklog/ulid/v2"
)

type OrganizeService struct {
	db database.Store
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
}

type OrganizeStats struct {
	Organized int
	Skipped   int
	Failed    int
	Total     int
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

	// Get ALL books by paginating through the database
	var allBooks []database.Book
	const fetchPageSize = 1000
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
	booksToOrganize := orgSvc.filterBooksNeedingOrganization(allBooks, log)

	logMsg = fmt.Sprintf("Found %d books that need organizing (out of %d total)", len(booksToOrganize), len(allBooks))
	log.Info("%s", logMsg)
	log.Debug("Organize: %s", logMsg)

	// Perform organization
	stats := orgSvc.organizeBooks(ctx, booksToOrganize, log, req.OperationID)

	// Trigger automatic rescan if any books were organized
	if stats.Organized > 0 {
		orgSvc.triggerAutomaticRescan(ctx, log)
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
	libraryPath := discoverITunesLibraryPath()
	if libraryPath == "" {
		log.Info("Skipping iTunes sync: no library found")
		return
	}

	log.Info("Running iTunes sync before organize: %s", libraryPath)

	if err := executeITunesSync(ctx, log, libraryPath, nil); err != nil {
		log.Warn("iTunes pre-sync failed (continuing with organize): %s", err.Error())
		return
	}

	log.Info("iTunes sync completed successfully")
}

func (orgSvc *OrganizeService) filterBooksNeedingOrganization(allBooks []database.Book, log logger.Logger) []database.Book {
	booksToOrganize := make([]database.Book, 0)
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
				log.Debug("Organize: Skipping book already correctly organized: %s", book.FilePath)
				continue
			}
			log.Info("Organize: Book in RootDir needs re-organization: %s", book.Title)
			// Fall through to include in organize list
		}
		// Quick check: skip if file_path is empty
		if book.FilePath == "" {
			continue
		}
		// Defer os.Stat to the actual organize phase — stat calls on 140K+ files
		// during scanning are the main bottleneck. Only check existence for books
		// that aren't already in RootDir (they need to be copied, so source must exist).
		if config.AppConfig.RootDir == "" || !strings.HasPrefix(book.FilePath, config.AppConfig.RootDir) {
			info, err := os.Stat(book.FilePath)
			if os.IsNotExist(err) {
				log.Debug("Organize: Skipping non-existent file: %s", book.FilePath)
				continue
			}
			// For directory-based (multi-file) books outside RootDir, verify files exist
			if err == nil && info.IsDir() {
				bookFiles, bfErr := orgSvc.db.GetBookFiles(book.ID)
				if bfErr == nil && len(bookFiles) > 0 {
					missingCount := 0
					for _, bf := range bookFiles {
						if bf.FilePath == "" || bf.Missing {
							continue
						}
						if _, serr := os.Stat(bf.FilePath); os.IsNotExist(serr) {
							missingCount++
					}
				}
				if missingCount > 0 {
					log.Debug("Organize: Skipping %s — %d book file(s) missing on disk", book.Title, missingCount)
					skippedMissingFiles++
					continue
				}
			}
		}
		} // end of non-RootDir stat check
		booksToOrganize = append(booksToOrganize, book)
	}
	if skippedDeleted > 0 {
		log.Info("Organize: Skipped %d soft-deleted book(s)", skippedDeleted)
	}
	if skippedMissingFiles > 0 {
		log.Info("Organize: Skipped %d book(s) with missing book files", skippedMissingFiles)
	}
	return booksToOrganize
}

// bookNeedsReOrganize checks whether a book already in RootDir needs to be
// moved because its current path doesn't match the target path derived from
// current metadata.
func (orgSvc *OrganizeService) bookNeedsReOrganize(book *database.Book, log logger.Logger) (bool, error) {
	org := organizer.NewOrganizer(&config.AppConfig)

	info, err := os.Stat(book.FilePath)
	if err != nil {
		return false, err
	}

	if info.IsDir() {
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
	org := organizer.NewOrganizer(&config.AppConfig)
	oldPath := book.FilePath

	info, err := os.Stat(oldPath)
	if err != nil {
		return "", fmt.Errorf("cannot stat %s: %w", oldPath, err)
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
		return targetPath, nil
	}

	// Create parent directory for target
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create target directory: %w", err)
	}

	// Rename (move) the file or directory
	if err := os.Rename(oldPath, targetPath); err != nil {
		return "", fmt.Errorf("failed to rename %s -> %s: %w", oldPath, targetPath, err)
	}

	// Update the book record
	book.FilePath = targetPath
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

func (orgSvc *OrganizeService) organizeBooks(ctx context.Context, booksToOrganize []database.Book, log logger.Logger, operationID string) *OrganizeStats {
	org := organizer.NewOrganizer(&config.AppConfig)
	stats := &OrganizeStats{Total: len(booksToOrganize)}

	// Track location changes for iTunes ITL write-back
	var itlUpdates []itunes.ITLLocationUpdate

	for i, book := range booksToOrganize {
		if log.IsCanceled() {
			log.Info("Organize canceled")
			break
		}

		log.UpdateProgress(i, len(booksToOrganize), fmt.Sprintf("Organizing %s...", book.Title))

		oldPath := book.FilePath
		isDir := false
		if info, err := os.Stat(oldPath); err == nil && info.IsDir() {
			isDir = true
		}

		// If book is already in RootDir, re-organize via rename instead of copy
		alreadyInRoot := config.AppConfig.RootDir != "" && strings.HasPrefix(oldPath, config.AppConfig.RootDir)

		var newPath string
		var err error

		if alreadyInRoot {
			newPath, err = orgSvc.reOrganizeInPlace(&book, log)
		} else if isDir {
			// Multi-file book: organize each segment file into the target directory
			newPath, err = orgSvc.organizeDirectoryBook(org, &book, log)
		} else {
			newPath, err = org.OrganizeBook(&book)
		}

		if err != nil {
			log.Warn("Failed to organize %s: %s", book.Title, err.Error())
			stats.Failed++

			// Track failed books
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
			continue
		}

		if oldPath == newPath {
			log.Info("Skipped %s: already in correct location", book.Title)
			stats.Skipped++

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
			continue
		}

		if alreadyInRoot {
			// Re-organized in place — no new version needed, just record the rename
			log.Info("Re-organized %s: %s → %s", book.Title, oldPath, newPath)
			stats.Organized++

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
			}
		} else {
			// Version-aware organize: create a new book record for the organized copy,
			// keep the original record pointing at the source (e.g. iTunes).
			// Link them as versions with the organized copy as primary.
			createdBook, err := orgSvc.createOrganizedVersion(org, &book, newPath, isDir, operationID, log)
			if err != nil {
				stats.Failed++
				continue
			}

			log.Info("Organized %s: created version %s → %s (original kept at %s)",
				book.Title, createdBook.ID, newPath, oldPath)

			stats.Organized++
		}

		// Collect ITL update if this book came from iTunes
		if book.ITunesPersistentID != nil {
			itlUpdates = append(itlUpdates, itunes.ITLLocationUpdate{
				PersistentID: *book.ITunesPersistentID,
				NewLocation:  newPath,
			})
		}
	}

	// Write back location changes to iTunes Library.itl
	if len(itlUpdates) > 0 && config.AppConfig.ITLWriteBackEnabled && config.AppConfig.ITunesLibraryWritePath != "" {
		orgSvc.writeBackITLLocations(itlUpdates, log)
	}

	summary := fmt.Sprintf("Organization completed: %d organized, %d skipped, %d failed (of %d total)",
		stats.Organized, stats.Skipped, stats.Failed, stats.Total)
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
			NewValue:    fmt.Sprintf("organized:%d skipped:%d failed:%d total:%d", stats.Organized, stats.Skipped, stats.Failed, stats.Total),
		})
	}

	return stats
}

// organizeDirectoryBook handles organizing a multi-file book where file_path is a directory.
// It finds all book files, organizes them into the target directory, and returns the new directory path.
func (orgSvc *OrganizeService) organizeDirectoryBook(org *organizer.Organizer, book *database.Book, log logger.Logger) (string, error) {
	// Get book files from DB
	bookFiles, err := orgSvc.db.GetBookFiles(book.ID)

	var segmentPaths []string
	if err == nil && len(bookFiles) > 0 {
		for _, bf := range bookFiles {
			if bf.FilePath != "" && !bf.Missing {
				segmentPaths = append(segmentPaths, bf.FilePath)
			}
		}
	}

	// If no book files in DB, scan the directory for audio files
	if len(segmentPaths) == 0 {
		entries, err := os.ReadDir(book.FilePath)
		if err != nil {
			return "", fmt.Errorf("failed to read directory: %w", err)
		}
		audioExts := map[string]bool{
			".m4b": true, ".m4a": true, ".mp3": true, ".aac": true,
			".ogg": true, ".opus": true, ".flac": true, ".wma": true,
			".wav": true,
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if audioExts[ext] {
				segmentPaths = append(segmentPaths, filepath.Join(book.FilePath, entry.Name()))
			}
		}
	}

	if len(segmentPaths) == 0 {
		return "", fmt.Errorf("no audio files found in directory: %s", book.FilePath)
	}

	log.Info("Organizing %d segment files for %s", len(segmentPaths), book.Title)

	targetDir, _, err := org.OrganizeBookDirectory(book, segmentPaths)
	if err != nil {
		return "", err
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
		if !isDir {
			os.Remove(newPath)
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
			// For directory books, update file paths to point to the organized location
			if isDir && bf.FilePath != "" {
				fileName := filepath.Base(bf.FilePath)
				newBF.FilePath = filepath.Join(newPath, fileName)
				newBF.ITunesPath = computeITunesPath(newBF.FilePath)
			}
			_ = orgSvc.db.CreateBookFile(&newBF)
		}
	}

	// Update original book: set version group, mark as non-primary
	book.VersionGroupID = &versionGroupID
	book.IsPrimaryVersion = &isNotPrimary
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

func (orgSvc *OrganizeService) writeBackITLLocations(updates []itunes.ITLLocationUpdate, log logger.Logger) {
	itlPath := config.AppConfig.ITunesLibraryWritePath

	// Create backup before modifying
	backupPath := itlPath + ".bak"
	srcData, err := os.ReadFile(itlPath)
	if err != nil {
		log.Warn("ITL write-back: failed to read %s: %s", itlPath, err.Error())
		return
	}
	if err := os.WriteFile(backupPath, srcData, 0644); err != nil {
		log.Warn("ITL write-back: failed to create backup: %s", err.Error())
		return
	}

	result, err := itunes.UpdateITLLocations(itlPath, itlPath, updates)
	if err != nil {
		log.Warn("ITL write-back failed: %s", err.Error())
		// Restore backup on failure
		if restoreErr := os.WriteFile(itlPath, srcData, 0644); restoreErr != nil {
			log.Error("ITL restore from backup also failed: %s", restoreErr.Error())
		}
		return
	}

	// Validate the written file
	if err := itunes.ValidateITL(itlPath); err != nil {
		log.Warn("ITL validation failed after write-back: %s", err.Error())
		// Restore backup
		if restoreErr := os.WriteFile(itlPath, srcData, 0644); restoreErr != nil {
			log.Error("ITL restore from backup also failed: %s", restoreErr.Error())
		}
		return
	}

	log.Info("ITL write-back: updated %d/%d locations in %s", result.UpdatedCount, len(updates), itlPath)
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

	if err := operations.GlobalQueue.Enqueue(scanOp.ID, "scan", operations.PriorityLow, scanFunc); err != nil {
		log.Warn("Failed to enqueue rescan: %s", err.Error())
	} else {
		log.Info("Rescan operation queued successfully")
	}
}
