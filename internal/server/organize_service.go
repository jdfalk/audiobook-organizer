// file: internal/server/organize_service.go
// version: 1.8.0
// guid: c3d4e5f6-a7b8-c9d0-e1f2-a3b4c5d6e7f8

package server

import (
	"context"
	"fmt"
	"hash/crc32"
	"log"
	"os"
	"strings"
	"sync/atomic"

	"path/filepath"

	"github.com/jdfalk/audiobook-organizer/internal/backup"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/itunes"
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
func (orgSvc *OrganizeService) PerformOrganizeWithID(ctx context.Context, opID string, req *OrganizeRequest, progress operations.ProgressReporter) error {
	_ = operations.SaveParams(orgSvc.db, opID, operations.OrganizeParams{})
	req.OperationID = opID
	err := orgSvc.PerformOrganize(ctx, req, progress)
	_ = operations.ClearState(orgSvc.db, opID)
	return err
}

// PerformOrganize executes the library organization operation
func (orgSvc *OrganizeService) PerformOrganize(ctx context.Context, req *OrganizeRequest, progress operations.ProgressReporter) error {
	_ = progress.Log("info", "Starting file organization", nil)

	// Optional: sync iTunes library first to ensure all books are up to date
	if req.SyncITunesFirst {
		orgSvc.syncITunesBeforeOrganize(ctx, progress)
	}

	// Auto-backup database before organizing
	orgSvc.autoBackup(progress)

	// Get ALL books by paginating through the database
	var allBooks []database.Book
	const fetchPageSize = 1000
	for offset := 0; ; offset += fetchPageSize {
		page, fetchErr := orgSvc.db.GetAllBooks(fetchPageSize, offset)
		if fetchErr != nil {
			errDetails := fetchErr.Error()
			_ = progress.Log("error", "Failed to fetch books", &errDetails)
			return fmt.Errorf("failed to fetch books: %w", fetchErr)
		}
		allBooks = append(allBooks, page...)
		if len(page) < fetchPageSize {
			break
		}
	}

	logMsg := fmt.Sprintf("Fetched %d total books from database", len(allBooks))
	_ = progress.Log("info", logMsg, nil)
	log.Printf("[DEBUG] Organize: %s", logMsg)

	// Optional: fetch metadata before organizing to normalize author names
	if req.FetchMetadataFirst {
		_ = progress.Log("info", "Fetching metadata before organizing...", nil)
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
		_ = progress.Log("info", fmt.Sprintf("Metadata enriched for %d books", enriched), nil)

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
	booksToOrganize := orgSvc.filterBooksNeedingOrganization(allBooks, progress)

	logMsg = fmt.Sprintf("Found %d books that need organizing (out of %d total)", len(booksToOrganize), len(allBooks))
	_ = progress.Log("info", logMsg, nil)
	log.Printf("[DEBUG] Organize: %s", logMsg)

	// Perform organization
	stats := orgSvc.organizeBooks(ctx, booksToOrganize, progress, req.OperationID)

	// Trigger automatic rescan if any books were organized
	if stats.Organized > 0 {
		orgSvc.triggerAutomaticRescan(ctx, progress)
	}

	return nil
}

func (orgSvc *OrganizeService) autoBackup(progress operations.ProgressReporter) {
	dbPath := config.AppConfig.DatabasePath
	dbType := config.AppConfig.DatabaseType
	if dbPath == "" {
		_ = progress.Log("warn", "Skipping auto-backup: no database path configured", nil)
		return
	}

	backupConfig := backup.DefaultBackupConfig()
	if !filepath.IsAbs(backupConfig.BackupDir) {
		backupConfig.BackupDir = filepath.Join(filepath.Dir(dbPath), backupConfig.BackupDir)
	}

	info, err := backup.CreateBackup(dbPath, dbType, backupConfig)
	if err != nil {
		errDetails := fmt.Sprintf("Auto-backup failed: %s", err.Error())
		_ = progress.Log("warn", errDetails, nil)
		return
	}
	_ = progress.Log("info", fmt.Sprintf("Auto-backup created: %s (%d bytes)", info.Filename, info.Size), nil)
}

func (orgSvc *OrganizeService) syncITunesBeforeOrganize(ctx context.Context, progress operations.ProgressReporter) {
	libraryPath := discoverITunesLibraryPath()
	if libraryPath == "" {
		_ = progress.Log("info", "Skipping iTunes sync: no library found", nil)
		return
	}

	_ = progress.Log("info", fmt.Sprintf("Running iTunes sync before organize: %s", libraryPath), nil)

	if err := executeITunesSync(ctx, progress, libraryPath, nil); err != nil {
		errDetails := fmt.Sprintf("iTunes pre-sync failed (continuing with organize): %s", err.Error())
		_ = progress.Log("warn", errDetails, nil)
		return
	}

	_ = progress.Log("info", "iTunes sync completed successfully", nil)
}

func (orgSvc *OrganizeService) filterBooksNeedingOrganization(allBooks []database.Book, progress operations.ProgressReporter) []database.Book {
	booksToOrganize := make([]database.Book, 0)
	for _, book := range allBooks {
		// Skip if already in root directory
		if config.AppConfig.RootDir != "" && strings.HasPrefix(book.FilePath, config.AppConfig.RootDir) {
			logMsg := fmt.Sprintf("Skipping book already in RootDir: %s (RootDir: %s)", book.FilePath, config.AppConfig.RootDir)
			log.Printf("[DEBUG] Organize: %s", logMsg)
			continue
		}
		// Skip if file doesn't exist
		if _, err := os.Stat(book.FilePath); os.IsNotExist(err) {
			logMsg := fmt.Sprintf("Skipping non-existent file: %s", book.FilePath)
			log.Printf("[DEBUG] Organize: %s", logMsg)
			continue
		}
		booksToOrganize = append(booksToOrganize, book)
	}
	return booksToOrganize
}

func (orgSvc *OrganizeService) organizeBooks(ctx context.Context, booksToOrganize []database.Book, progress operations.ProgressReporter, operationID string) *OrganizeStats {
	org := organizer.NewOrganizer(&config.AppConfig)
	stats := &OrganizeStats{Total: len(booksToOrganize)}

	// Track location changes for iTunes ITL write-back
	var itlUpdates []itunes.ITLLocationUpdate

	for i, book := range booksToOrganize {
		if progress.IsCanceled() {
			_ = progress.Log("info", "Organize canceled", nil)
			break
		}

		_ = progress.UpdateProgress(i, len(booksToOrganize), fmt.Sprintf("Organizing %s...", book.Title))

		oldPath := book.FilePath
		isDir := false
		if info, err := os.Stat(oldPath); err == nil && info.IsDir() {
			isDir = true
		}

		var newPath string
		var err error

		if isDir {
			// Multi-file book: organize each segment file into the target directory
			newPath, err = orgSvc.organizeDirectoryBook(org, &book, progress)
		} else {
			newPath, err = org.OrganizeBook(&book)
		}

		if err != nil {
			errDetails := fmt.Sprintf("Failed to organize %s: %s", book.Title, err.Error())
			_ = progress.Log("warn", errDetails, nil)
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
			_ = progress.Log("info", fmt.Sprintf("Skipped %s: already in correct location", book.Title), nil)
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

		// Version-aware organize: create a new book record for the organized copy,
		// keep the original record pointing at the source (e.g. iTunes).
		// Link them as versions with the organized copy as primary.
		createdBook, err := orgSvc.createOrganizedVersion(org, &book, newPath, isDir, operationID, progress)
		if err != nil {
			stats.Failed++
			continue
		}

		_ = progress.Log("info", fmt.Sprintf("Organized %s: created version %s → %s (original kept at %s)",
			book.Title, createdBook.ID, newPath, oldPath), nil)

		stats.Organized++

		// Collect ITL update if this book came from iTunes
		if book.ITunesPersistentID != nil {
			itlUpdates = append(itlUpdates, itunes.ITLLocationUpdate{
				PersistentID: *book.ITunesPersistentID,
				NewLocation:  newPath,
			})
		}
	}

	// Write back location changes to iTunes Library.itl
	if len(itlUpdates) > 0 && config.AppConfig.ITLWriteBackEnabled && config.AppConfig.ITunesLibraryITLPath != "" {
		orgSvc.writeBackITLLocations(itlUpdates, progress)
	}

	summary := fmt.Sprintf("Organization completed: %d organized, %d skipped, %d failed (of %d total)",
		stats.Organized, stats.Skipped, stats.Failed, stats.Total)
	_ = progress.Log("info", summary, nil)

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
// It finds all segment files, organizes them into the target directory, and returns the new directory path.
func (orgSvc *OrganizeService) organizeDirectoryBook(org *organizer.Organizer, book *database.Book, progress operations.ProgressReporter) (string, error) {
	// Get segment files from DB
	numericID := int(crc32.ChecksumIEEE([]byte(book.ID)))
	segments, err := orgSvc.db.ListBookSegments(numericID)

	var segmentPaths []string
	if err == nil && len(segments) > 0 {
		for _, seg := range segments {
			if seg.FilePath != "" {
				segmentPaths = append(segmentPaths, seg.FilePath)
			}
		}
	}

	// If no segments in DB, scan the directory for audio files
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

	_ = progress.Log("info", fmt.Sprintf("Organizing %d segment files for %s", len(segmentPaths), book.Title), nil)

	targetDir, _, err := org.OrganizeBookDirectory(book, segmentPaths)
	if err != nil {
		return "", err
	}

	return targetDir, nil
}

// createOrganizedVersion creates a new book record for the organized copy and links it to the original.
func (orgSvc *OrganizeService) createOrganizedVersion(org *organizer.Organizer, book *database.Book, newPath string, isDir bool, operationID string, progress operations.ProgressReporter) (*database.Book, error) {
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
		errDetails := fmt.Sprintf("Failed to create organized book record for %s: %v", book.Title, err)
		_ = progress.Log("error", errDetails, nil)
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

	// Copy segments to the new book with updated paths
	oldNumericID := int(crc32.ChecksumIEEE([]byte(book.ID)))
	newNumericID := int(crc32.ChecksumIEEE([]byte(newBookID)))
	if segments, err := orgSvc.db.ListBookSegments(oldNumericID); err == nil && len(segments) > 0 {
		for _, seg := range segments {
			newSeg := seg
			newSeg.ID = ulid.Make().String()
			newSeg.BookID = newNumericID
			// For directory books, update segment paths to point to the organized location
			if isDir && seg.FilePath != "" {
				fileName := filepath.Base(seg.FilePath)
				newSeg.FilePath = filepath.Join(newPath, fileName)
			}
			_, _ = orgSvc.db.CreateBookSegment(newNumericID, &newSeg)
		}
	}

	// Update original book: set version group, mark as non-primary
	book.VersionGroupID = &versionGroupID
	book.IsPrimaryVersion = &isNotPrimary
	if _, err := orgSvc.db.UpdateBook(book.ID, book); err != nil {
		_ = progress.Log("warn", fmt.Sprintf("Failed to update original book %s version group: %v", book.ID, err), nil)
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

func (orgSvc *OrganizeService) writeBackITLLocations(updates []itunes.ITLLocationUpdate, progress operations.ProgressReporter) {
	itlPath := config.AppConfig.ITunesLibraryITLPath

	// Create backup before modifying
	backupPath := itlPath + ".bak"
	srcData, err := os.ReadFile(itlPath)
	if err != nil {
		errDetails := fmt.Sprintf("ITL write-back: failed to read %s: %s", itlPath, err.Error())
		_ = progress.Log("warn", errDetails, nil)
		return
	}
	if err := os.WriteFile(backupPath, srcData, 0644); err != nil {
		errDetails := fmt.Sprintf("ITL write-back: failed to create backup: %s", err.Error())
		_ = progress.Log("warn", errDetails, nil)
		return
	}

	result, err := itunes.UpdateITLLocations(itlPath, itlPath, updates)
	if err != nil {
		errDetails := fmt.Sprintf("ITL write-back failed: %s", err.Error())
		_ = progress.Log("warn", errDetails, nil)
		// Restore backup on failure
		if restoreErr := os.WriteFile(itlPath, srcData, 0644); restoreErr != nil {
			_ = progress.Log("error", fmt.Sprintf("ITL restore from backup also failed: %s", restoreErr.Error()), nil)
		}
		return
	}

	// Validate the written file
	if err := itunes.ValidateITL(itlPath); err != nil {
		errDetails := fmt.Sprintf("ITL validation failed after write-back: %s", err.Error())
		_ = progress.Log("warn", errDetails, nil)
		// Restore backup
		if restoreErr := os.WriteFile(itlPath, srcData, 0644); restoreErr != nil {
			_ = progress.Log("error", fmt.Sprintf("ITL restore from backup also failed: %s", restoreErr.Error()), nil)
		}
		return
	}

	_ = progress.Log("info", fmt.Sprintf("ITL write-back: updated %d/%d locations in %s", result.UpdatedCount, len(updates), itlPath), nil)
}

func (orgSvc *OrganizeService) triggerAutomaticRescan(ctx context.Context, progress operations.ProgressReporter) {
	if config.AppConfig.RootDir == "" {
		return
	}

	_ = progress.Log("info", "Starting automatic rescan of library path...", nil)

	// Create a new scan operation
	scanID := ulid.Make().String()
	scanOp, err := orgSvc.db.CreateOperation(scanID, "scan", &config.AppConfig.RootDir)
	if err != nil {
		errDetails := fmt.Sprintf("Failed to create rescan operation: %s", err.Error())
		_ = progress.Log("warn", errDetails, nil)
		return
	}

	// Enqueue the scan operation with low priority
	scanFunc := func(ctx context.Context, scanProgress operations.ProgressReporter) error {
		_ = scanProgress.Log("info", fmt.Sprintf("Scanning organized books in: %s", config.AppConfig.RootDir), nil)

		workers := config.AppConfig.ConcurrentScans
		if workers < 1 {
			workers = 4
		}

		_ = scanProgress.Log("info", fmt.Sprintf("Starting directory scan with %d workers", workers), nil)
		books, err := scanner.ScanDirectoryParallel(config.AppConfig.RootDir, workers, nil)
		if err != nil {
			return fmt.Errorf("failed to rescan root directory: %w", err)
		}

		_ = scanProgress.Log("info", fmt.Sprintf("Found %d books in root directory, processing metadata", len(books)), nil)

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
				_ = scanProgress.UpdateProgress(int(current), displayTotal, message)
			}

			_ = scanProgress.Log("info", fmt.Sprintf("Processing metadata for %d books using %d workers", totalBooks, workers), nil)
			if err := scanner.ProcessBooksParallel(ctx, books, workers, progressCallback, nil); err != nil {
				return fmt.Errorf("failed to process books: %w", err)
			}
			_ = scanProgress.Log("info", fmt.Sprintf("Metadata processing complete: %d books processed", processedFiles.Load()), nil)
		}

		_ = scanProgress.Log("info", "Rescan completed successfully", nil)
		return nil
	}

	if err := operations.GlobalQueue.Enqueue(scanOp.ID, "scan", operations.PriorityLow, scanFunc); err != nil {
		errDetails := fmt.Sprintf("Failed to enqueue rescan: %s", err.Error())
		_ = progress.Log("warn", errDetails, nil)
	} else {
		_ = progress.Log("info", "Rescan operation queued successfully", nil)
	}
}
