// file: internal/server/scan_service.go
// version: 1.2.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/logger"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
)

type ScanService struct {
	db database.Store
}

func NewScanService(db database.Store) *ScanService {
	return &ScanService{db: db}
}

type ScanRequest struct {
	FolderPath  *string
	Priority    *int
	ForceUpdate *bool
}

type ScanStats struct {
	TotalBooks   int
	LibraryBooks int
	ImportBooks  int
}

// PerformScanWithID executes the multi-folder scan operation with checkpoint support.
func (ss *ScanService) PerformScanWithID(ctx context.Context, opID string, req *ScanRequest, log logger.Logger) error {
	// Save params for resume
	_ = operations.SaveParams(ss.db, opID, operations.ScanParams{
		FolderPath:  req.FolderPath,
		ForceUpdate: req.ForceUpdate != nil && *req.ForceUpdate,
	})
	err := ss.PerformScan(ctx, req, log)
	_ = operations.ClearState(ss.db, opID)
	return err
}

// PerformScan executes the multi-folder scan operation.
// Accepts a logger.Logger for unified logging, progress, and change tracking.
func (ss *ScanService) PerformScan(ctx context.Context, req *ScanRequest, log logger.Logger) error {
	if log == nil {
		log = logger.New("scan")
	}
	forceUpdate := req.ForceUpdate != nil && *req.ForceUpdate
	if forceUpdate {
		log.Debug("ScanService: Force update enabled - will update all book file paths in database")
	}

	// Determine which folders to scan
	foldersToScan, err := ss.determineFoldersToScan(req.FolderPath, forceUpdate, log)
	if err != nil {
		return err
	}

	if len(foldersToScan) == 0 {
		log.Warn("No folders to scan")
		return nil
	}

	// Pre-load scan cache for incremental skip checks.
	var scanCache map[string]database.ScanCacheEntry
	if !forceUpdate {
		cache, err := ss.db.GetScanCacheMap()
		if err != nil {
			log.Warn("Failed to load scan cache, running full scan: %v", err)
		} else {
			scanCache = cache
			log.Info("Loaded scan cache with %d entries", len(cache))
		}
	}

	// Add any folders that have books flagged needs_rescan.
	if !forceUpdate && scanCache != nil {
		dirtyFolders, err := ss.db.GetDirtyBookFolders()
		if err == nil && len(dirtyFolders) > 0 {
			log.Info("Found %d folders with dirty books", len(dirtyFolders))
			folderSet := make(map[string]bool)
			for _, f := range foldersToScan {
				folderSet[f] = true
			}
			for _, df := range dirtyFolders {
				if !folderSet[df] {
					foldersToScan = append(foldersToScan, df)
				}
			}
		}
	}

	// First pass: count total files across all folders.
	// For incremental scans we use the cache size as an approximation to avoid
	// the expensive directory walk.
	var totalFilesAcrossFolders int
	if forceUpdate || scanCache == nil {
		totalFilesAcrossFolders = ss.countFilesAcrossFolders(foldersToScan, log)
		log.Info("Total audiobook files across all folders: %d", totalFilesAcrossFolders)
		if totalFilesAcrossFolders == 0 {
			log.Warn("No audiobook files detected during pre-scan; totals will update as files are processed")
		}
	} else {
		totalFilesAcrossFolders = len(scanCache)
		log.Info("Incremental scan: ~%d known files, checking for changes", totalFilesAcrossFolders)
	}

	// Install scan cache into the scanner package so workers can skip unchanged files.
	scanner.SetScanCache(scanCache)
	defer scanner.ClearScanCache()

	// Scan each folder
	stats := &ScanStats{}
	var processedFiles atomic.Int32

	for folderIdx, folderPath := range foldersToScan {
		if log.IsCanceled() {
			log.Info("Scan canceled")
			return fmt.Errorf("scan canceled")
		}

		err := ss.scanFolder(ctx, folderIdx, folderPath, foldersToScan, totalFilesAcrossFolders, &processedFiles, stats, log)
		if err != nil {
			log.Error("Error scanning folder %s: %v", folderPath, err)
			continue
		}
	}

	// Report completion with change counters
	counters := log.ChangeCounters()
	if counters != nil && (counters["book_create"] > 0 || counters["book_update"] > 0) {
		log.Info("scan changes: %d created, %d updated, %d skipped",
			counters["book_create"], counters["book_update"], counters["book_skip"])
	}
	ss.reportCompletion(totalFilesAcrossFolders, int(processedFiles.Load()), stats, log)
	return nil
}

func (ss *ScanService) determineFoldersToScan(folderPath *string, forceUpdate bool, log logger.Logger) ([]string, error) {
	var foldersToScan []string

	if folderPath != nil && *folderPath != "" {
		// Scan specific folder
		foldersToScan = []string{*folderPath}
		log.Info("Starting scan of folder: %s", *folderPath)
	} else {
		// Full scan: include RootDir if force_update enabled, then all import paths
		if forceUpdate && config.AppConfig.RootDir != "" {
			foldersToScan = append(foldersToScan, config.AppConfig.RootDir)
			log.Info("Full rescan: including library path %s", config.AppConfig.RootDir)
		}

		// Add all import paths
		folders, err := ss.db.GetAllImportPaths()
		if err != nil {
			return nil, fmt.Errorf("failed to get import paths: %w", err)
		}
		for _, folder := range folders {
			if folder.Enabled {
				foldersToScan = append(foldersToScan, folder.Path)
			}
		}
		log.Info("Scanning %d total folders (%d import paths)", len(foldersToScan), len(folders))
	}

	return foldersToScan, nil
}

func (ss *ScanService) countFilesAcrossFolders(foldersToScan []string, log logger.Logger) int {
	totalFilesAcrossFolders := 0
	for _, folderPath := range foldersToScan {
		if _, err := os.Stat(folderPath); os.IsNotExist(err) {
			log.Warn("Folder does not exist: %s", folderPath)
			continue
		}
		fileCount := 0
		_ = filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			for _, supported := range config.AppConfig.SupportedExtensions {
				if ext == supported {
					fileCount++
					break
				}
			}
			return nil
		})
		log.Info("Folder %s: Found %d audiobook files", folderPath, fileCount)
		totalFilesAcrossFolders += fileCount
	}
	return totalFilesAcrossFolders
}

func (ss *ScanService) scanFolder(ctx context.Context, folderIdx int, folderPath string, foldersToScan []string, totalFilesAcrossFolders int, processedFiles *atomic.Int32, stats *ScanStats, log logger.Logger) error {
	currentProcessed := int(processedFiles.Load())
	displayTotal := totalFilesAcrossFolders
	if currentProcessed > displayTotal {
		displayTotal = currentProcessed
	}
	log.UpdateProgress(currentProcessed, displayTotal, fmt.Sprintf("Scanning folder %d/%d: %s", folderIdx+1, len(foldersToScan), folderPath))
	log.Info("Scanning folder: %s", folderPath)

	// Check if folder exists
	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		log.Warn("Folder does not exist: %s", folderPath)
		return nil
	}

	// Scan directory for audiobook files (parallel)
	workers := config.AppConfig.ConcurrentScans
	if workers < 1 {
		workers = 4
	}
	books, err := scanner.ScanDirectoryParallel(folderPath, workers, log.With("scanner"))
	if err != nil {
		return fmt.Errorf("failed to scan folder: %w", err)
	}

	log.Info("Found %d audiobook files in %s", len(books), folderPath)
	stats.TotalBooks += len(books)
	if folderPath == config.AppConfig.RootDir {
		stats.LibraryBooks += len(books)
	} else {
		stats.ImportBooks += len(books)
	}

	// Prepare per-book progress reporting
	targetTotal := totalFilesAcrossFolders
	if targetTotal == 0 {
		targetTotal = len(books)
	}
	progressCallback := func(_ int, _ int, bookPath string) {
		current := processedFiles.Add(1)
		displayTotal := targetTotal
		if int(current) > displayTotal {
			displayTotal = int(current)
		}
		message := fmt.Sprintf("Processed: %d/%d books", current, displayTotal)
		if bookPath != "" {
			message = fmt.Sprintf("Processed: %d/%d books (%s)", current, displayTotal, filepath.Base(bookPath))
		}
		log.UpdateProgress(int(current), displayTotal, message)
	}

	// Process the books to extract metadata (parallel)
	if len(books) > 0 {
		log.Info("Processing metadata for %d books using %d workers", len(books), workers)
		if err := scanner.ProcessBooksParallel(ctx, books, workers, progressCallback, log.With("scanner")); err != nil {
			log.Error("Failed to process books: %v", err)
		} else {
			log.Info("Successfully processed %d books", len(books))
		}

		// Auto-organize if enabled
		ss.autoOrganizeScannedBooks(ctx, books, log)
	}

	// Update book count for this import path
	ss.updateImportPathBookCount(folderPath, len(books), log)

	return nil
}

func (ss *ScanService) autoOrganizeScannedBooks(_ context.Context, books []scanner.Book, log logger.Logger) {
	if len(books) == 0 {
		return
	}
	if config.AppConfig.AutoOrganize && config.AppConfig.RootDir != "" {
		org := organizer.NewOrganizer(&config.AppConfig)
		organized := 0
		for i := range books {
			if log.IsCanceled() {
				break
			}
			// Lookup DB book by file path
			dbBook, err := ss.db.GetBookByFilePath(books[i].FilePath)
			if err != nil || dbBook == nil {
				continue
			}
			newPath, _, err := org.OrganizeBook(dbBook)
			if err != nil {
				log.Warn("Organize failed for %s: %v", dbBook.Title, err)
				continue
			}
			// Update DB path if changed
			if newPath != dbBook.FilePath {
				oldPath := dbBook.FilePath
				dbBook.FilePath = newPath
				applyOrganizedFileMetadata(dbBook, newPath)
				if _, err := ss.db.UpdateBook(dbBook.ID, dbBook); err != nil {
					log.Error("Failed to update path for %s: %v — rolling back", dbBook.Title, err)
					if rbErr := os.Rename(newPath, oldPath); rbErr != nil {
						log.Error("CRITICAL: rollback failed for %s: file at %s, DB expects %s", dbBook.ID, newPath, oldPath)
					}
				} else {
					organized++
				}
			}
		}
		log.Info("Auto-organize complete: %d organized", organized)
	} else if config.AppConfig.AutoOrganize && config.AppConfig.RootDir == "" {
		log.Warn("Auto-organize enabled but root_dir not set")
	}
}

func (ss *ScanService) updateImportPathBookCount(folderPath string, bookCount int, log logger.Logger) {
	folders, _ := ss.db.GetAllImportPaths()
	for _, folder := range folders {
		if folder.Path == folderPath {
			folder.BookCount = bookCount
			if err := ss.db.UpdateImportPath(folder.ID, &folder); err != nil {
				log.Warn("Failed to update book count for folder %s: %v", folderPath, err)
			}
			break
		}
	}
}

func (ss *ScanService) reportCompletion(totalFilesAcrossFolders int, finalProcessed int, stats *ScanStats, log logger.Logger) {
	var completionMsg string
	if stats.LibraryBooks > 0 && stats.ImportBooks > 0 {
		completionMsg = fmt.Sprintf("Scan completed. Library: %d books, Import: %d books (Total: %d)", stats.LibraryBooks, stats.ImportBooks, stats.TotalBooks)
	} else if stats.LibraryBooks > 0 {
		completionMsg = fmt.Sprintf("Scan completed. Library: %d books", stats.LibraryBooks)
	} else if stats.ImportBooks > 0 {
		completionMsg = fmt.Sprintf("Scan completed. Import: %d books", stats.ImportBooks)
	} else {
		completionMsg = "Scan completed. No books found"
	}

	finalTotal := totalFilesAcrossFolders
	if finalProcessed > finalTotal {
		finalTotal = finalProcessed
	}
	log.UpdateProgress(finalProcessed, finalTotal, completionMsg)
	log.Info("%s", completionMsg)
}
