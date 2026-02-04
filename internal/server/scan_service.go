// file: internal/server/scan_service.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
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

// PerformScan executes the multi-folder scan operation
func (ss *ScanService) PerformScan(ctx context.Context, req *ScanRequest, progress operations.ProgressReporter) error {
	forceUpdate := req.ForceUpdate != nil && *req.ForceUpdate
	if forceUpdate {
		log.Printf("[DEBUG] ScanService: Force update enabled - will update all book file paths in database")
	}

	// Determine which folders to scan
	foldersToScan, err := ss.determineFoldersToScan(req.FolderPath, forceUpdate, progress)
	if err != nil {
		return err
	}

	if len(foldersToScan) == 0 {
		_ = progress.Log("warn", "No folders to scan", nil)
		return nil
	}

	// First pass: count total files across all folders
	totalFilesAcrossFolders := ss.countFilesAcrossFolders(foldersToScan, progress)
	_ = progress.Log("info", fmt.Sprintf("Total audiobook files across all folders: %d", totalFilesAcrossFolders), nil)
	if totalFilesAcrossFolders == 0 {
		_ = progress.Log("warn", "No audiobook files detected during pre-scan; totals will update as files are processed", nil)
	}

	// Scan each folder
	stats := &ScanStats{}
	var processedFiles atomic.Int32

	for folderIdx, folderPath := range foldersToScan {
		if progress.IsCanceled() {
			_ = progress.Log("info", "Scan canceled", nil)
			return fmt.Errorf("scan canceled")
		}

		err := ss.scanFolder(ctx, folderIdx, folderPath, foldersToScan, totalFilesAcrossFolders, &processedFiles, stats, progress)
		if err != nil {
			_ = progress.Log("error", fmt.Sprintf("Error scanning folder %s: %v", folderPath, err), nil)
			continue
		}
	}

	// Report completion
	ss.reportCompletion(totalFilesAcrossFolders, int(processedFiles.Load()), stats, progress)
	return nil
}

func (ss *ScanService) determineFoldersToScan(folderPath *string, forceUpdate bool, progress operations.ProgressReporter) ([]string, error) {
	var foldersToScan []string

	if folderPath != nil && *folderPath != "" {
		// Scan specific folder
		foldersToScan = []string{*folderPath}
		_ = progress.Log("info", fmt.Sprintf("Starting scan of folder: %s", *folderPath), nil)
	} else {
		// Full scan: include RootDir if force_update enabled, then all import paths
		if forceUpdate && config.AppConfig.RootDir != "" {
			foldersToScan = append(foldersToScan, config.AppConfig.RootDir)
			_ = progress.Log("info", fmt.Sprintf("Full rescan: including library path %s", config.AppConfig.RootDir), nil)
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
		_ = progress.Log("info", fmt.Sprintf("Scanning %d total folders (%d import paths)", len(foldersToScan), len(folders)), nil)
	}

	return foldersToScan, nil
}

func (ss *ScanService) countFilesAcrossFolders(foldersToScan []string, progress operations.ProgressReporter) int {
	totalFilesAcrossFolders := 0
	for _, folderPath := range foldersToScan {
		if _, err := os.Stat(folderPath); os.IsNotExist(err) {
			_ = progress.Log("warn", fmt.Sprintf("Folder does not exist: %s", folderPath), nil)
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
		_ = progress.Log("info", fmt.Sprintf("Folder %s: Found %d audiobook files", folderPath, fileCount), nil)
		totalFilesAcrossFolders += fileCount
	}
	return totalFilesAcrossFolders
}

func (ss *ScanService) scanFolder(ctx context.Context, folderIdx int, folderPath string, foldersToScan []string, totalFilesAcrossFolders int, processedFiles *atomic.Int32, stats *ScanStats, progress operations.ProgressReporter) error {
	currentProcessed := int(processedFiles.Load())
	displayTotal := totalFilesAcrossFolders
	if currentProcessed > displayTotal {
		displayTotal = currentProcessed
	}
	_ = progress.UpdateProgress(currentProcessed, displayTotal, fmt.Sprintf("Scanning folder %d/%d: %s", folderIdx+1, len(foldersToScan), folderPath))
	_ = progress.Log("info", fmt.Sprintf("Scanning folder: %s", folderPath), nil)

	// Check if folder exists
	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		_ = progress.Log("warn", fmt.Sprintf("Folder does not exist: %s", folderPath), nil)
		return nil
	}

	// Scan directory for audiobook files (parallel)
	workers := config.AppConfig.ConcurrentScans
	if workers < 1 {
		workers = 4
	}
	books, err := scanner.ScanDirectoryParallel(folderPath, workers)
	if err != nil {
		return fmt.Errorf("failed to scan folder: %w", err)
	}

	_ = progress.Log("info", fmt.Sprintf("Found %d audiobook files in %s", len(books), folderPath), nil)
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
		_ = progress.UpdateProgress(int(current), displayTotal, message)
	}

	// Process the books to extract metadata (parallel)
	if len(books) > 0 {
		_ = progress.Log("info", fmt.Sprintf("Processing metadata for %d books using %d workers", len(books), workers), nil)
		if err := scanner.ProcessBooksParallel(ctx, books, workers, progressCallback); err != nil {
			_ = progress.Log("error", fmt.Sprintf("Failed to process books: %v", err), nil)
		} else {
			_ = progress.Log("info", fmt.Sprintf("Successfully processed %d books", len(books)), nil)
		}

			// Auto-organize if enabled
		ss.autoOrganizeScannedBooks(ctx, books, progress)
	}

	// Update book count for this import path
	ss.updateImportPathBookCount(folderPath, len(books), progress)

	return nil
}

func (ss *ScanService) autoOrganizeScannedBooks(ctx context.Context, books []scanner.Book, progress operations.ProgressReporter) {
	if config.AppConfig.AutoOrganize && config.AppConfig.RootDir != "" {
		org := organizer.NewOrganizer(&config.AppConfig)
		organized := 0
		for i := range books {
			if progress.IsCanceled() {
				break
			}
			// Lookup DB book by file path
			dbBook, err := ss.db.GetBookByFilePath(books[i].FilePath)
			if err != nil || dbBook == nil {
				continue
			}
			newPath, err := org.OrganizeBook(dbBook)
			if err != nil {
				_ = progress.Log("warn", fmt.Sprintf("Organize failed for %s: %v", dbBook.Title, err), nil)
				continue
			}
			// Update DB path if changed
			if newPath != dbBook.FilePath {
				dbBook.FilePath = newPath
				applyOrganizedFileMetadata(dbBook, newPath)
				if _, err := ss.db.UpdateBook(dbBook.ID, dbBook); err != nil {
					_ = progress.Log("warn", fmt.Sprintf("Failed to update path for %s: %v", dbBook.Title, err), nil)
				} else {
					organized++
				}
			}
		}
		_ = progress.Log("info", fmt.Sprintf("Auto-organize complete: %d organized", organized), nil)
	} else if config.AppConfig.AutoOrganize && config.AppConfig.RootDir == "" {
		_ = progress.Log("warn", "Auto-organize enabled but root_dir not set", nil)
	}
}

func (ss *ScanService) updateImportPathBookCount(folderPath string, bookCount int, progress operations.ProgressReporter) {
	folders, _ := ss.db.GetAllImportPaths()
	for _, folder := range folders {
		if folder.Path == folderPath {
			folder.BookCount = bookCount
			if err := ss.db.UpdateImportPath(folder.ID, &folder); err != nil {
				_ = progress.Log("warn", fmt.Sprintf("Failed to update book count for folder %s: %v", folderPath, err), nil)
			}
			break
		}
	}
}

func (ss *ScanService) reportCompletion(totalFilesAcrossFolders int, finalProcessed int, stats *ScanStats, progress operations.ProgressReporter) {
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
	_ = progress.UpdateProgress(finalProcessed, finalTotal, completionMsg)
	_ = progress.Log("info", completionMsg, nil)
}
