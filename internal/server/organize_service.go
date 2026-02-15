// file: internal/server/organize_service.go
// version: 1.1.0
// guid: c3d4e5f6-a7b8-c9d0-e1f2-a3b4c5d6e7f8

package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
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
}

type OrganizeStats struct {
	Organized int
	Failed    int
	Total     int
}

// PerformOrganize executes the library organization operation
func (orgSvc *OrganizeService) PerformOrganize(ctx context.Context, req *OrganizeRequest, progress operations.ProgressReporter) error {
	_ = progress.Log("info", "Starting file organization", nil)

	// Get books to organize
	allBooks, err := orgSvc.db.GetAllBooks(1000, 0)
	if err != nil {
		errDetails := err.Error()
		_ = progress.Log("error", "Failed to fetch books", &errDetails)
		return fmt.Errorf("failed to fetch books: %w", err)
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

		// Re-fetch books since metadata may have changed
		allBooks, err = orgSvc.db.GetAllBooks(1000, 0)
		if err != nil {
			return fmt.Errorf("failed to re-fetch books after metadata: %w", err)
		}
	}

	// Filter books that need organizing
	booksToOrganize := orgSvc.filterBooksNeedingOrganization(allBooks, progress)

	logMsg = fmt.Sprintf("Found %d books that need organizing (out of %d total)", len(booksToOrganize), len(allBooks))
	_ = progress.Log("info", logMsg, nil)
	log.Printf("[DEBUG] Organize: %s", logMsg)

	// Perform organization
	stats := orgSvc.organizeBooks(ctx, booksToOrganize, progress)

	// Trigger automatic rescan if any books were organized
	if stats.Organized > 0 {
		orgSvc.triggerAutomaticRescan(ctx, progress)
	}

	return nil
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

func (orgSvc *OrganizeService) organizeBooks(ctx context.Context, booksToOrganize []database.Book, progress operations.ProgressReporter) *OrganizeStats {
	org := organizer.NewOrganizer(&config.AppConfig)
	stats := &OrganizeStats{Total: len(booksToOrganize)}

	for i, book := range booksToOrganize {
		if progress.IsCanceled() {
			_ = progress.Log("info", "Organize canceled", nil)
			break
		}

		_ = progress.UpdateProgress(i, len(booksToOrganize), fmt.Sprintf("Organizing %s...", book.Title))

		newPath, err := org.OrganizeBook(&book)
		if err != nil {
			errDetails := fmt.Sprintf("Failed to organize %s: %s", book.Title, err.Error())
			_ = progress.Log("warn", errDetails, nil)
			stats.Failed++
			continue
		}

		// Update book's file path and state in database
		book.FilePath = newPath
		book.LibraryState = stringPtr("organized")
		applyOrganizedFileMetadata(&book, newPath)
		if _, err := orgSvc.db.UpdateBook(book.ID, &book); err != nil {
			errDetails := fmt.Sprintf("Failed to update book path: %s", err.Error())
			_ = progress.Log("warn", errDetails, nil)
		} else {
			stats.Organized++
		}
	}

	summary := fmt.Sprintf("Organization completed: %d organized, %d failed", stats.Organized, stats.Failed)
	_ = progress.Log("info", summary, nil)

	return stats
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
		books, err := scanner.ScanDirectoryParallel(config.AppConfig.RootDir, workers)
		if err != nil {
			return fmt.Errorf("failed to rescan root directory: %w", err)
		}

		_ = scanProgress.Log("info", fmt.Sprintf("Found %d books in root directory", len(books)), nil)

		// Process the books to extract metadata
		if len(books) > 0 {
			if err := scanner.ProcessBooksParallel(ctx, books, workers, nil); err != nil {
				return fmt.Errorf("failed to process books: %w", err)
			}
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
