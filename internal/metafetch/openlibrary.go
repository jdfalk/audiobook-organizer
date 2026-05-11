// file: internal/metafetch/openlibrary.go
// version: 1.1.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f90

package metafetch

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/openlibrary"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

// OpenLibraryService manages the Open Library data dump lifecycle.
type OpenLibraryService struct {
	OLStore   *openlibrary.OLStore
	Tracker   *openlibrary.DownloadTracker
	Mu        sync.Mutex
	Importing map[string]bool
}

// GetOLDumpDir returns the configured dump directory, falling back to {RootDir}/openlibrary-dumps.
func GetOLDumpDir() string {
	if config.AppConfig.OpenLibraryDumpDir != "" {
		return config.AppConfig.OpenLibraryDumpDir
	}
	if config.AppConfig.RootDir != "" {
		return filepath.Join(config.AppConfig.RootDir, "openlibrary-dumps")
	}
	return ""
}

// NewOpenLibraryService creates a new service, optionally opening the existing store.
// If an existing oldb directory is found, it auto-enables Open Library dumps in config.
func NewOpenLibraryService() *OpenLibraryService {
	svc := &OpenLibraryService{
		Tracker:   openlibrary.NewDownloadTracker(),
		Importing: make(map[string]bool),
	}

	storePath := filepath.Join(GetOLDumpDir(), "oldb")
	if info, err := os.Stat(storePath); err == nil && info.IsDir() {
		// Auto-enable if store directory exists on disk
		if !config.AppConfig.OpenLibraryDumpEnabled {
			log.Printf("[INFO] Found existing OL dump store at %s, auto-enabling OpenLibraryDumpEnabled", storePath)
			config.AppConfig.OpenLibraryDumpEnabled = true
		}
		store, err := openlibrary.NewOLStore(storePath)
		if err != nil {
			log.Printf("[WARN] Failed to open OL dump store: %v", err)
		} else {
			svc.OLStore = store
		}
	}

	return svc
}

// Store returns the underlying OLStore (may be nil).
func (svc *OpenLibraryService) Store() *openlibrary.OLStore {
	return svc.OLStore
}

// Close closes the underlying store.
func (svc *OpenLibraryService) Close() {
	if svc.OLStore != nil {
		svc.OLStore.Close()
	}
}

// EnsureStore opens or creates the OL store if not already open.
// Returns an error if the store cannot be opened.
func (svc *OpenLibraryService) EnsureStore(targetDir string) error {
	svc.Mu.Lock()
	defer svc.Mu.Unlock()
	if svc.OLStore == nil {
		storePath := filepath.Join(targetDir, "oldb")
		store, err := openlibrary.NewOLStore(storePath)
		if err != nil {
			return err
		}
		svc.OLStore = store
	}
	return nil
}

// UploadedFileInfo describes a dump file on disk that hasn't been imported yet.
type UploadedFileInfo struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
}

// ValidDumpTypes lists the valid dump type names.
var ValidDumpTypes = map[string]bool{"editions": true, "authors": true, "works": true}

// Import performs the actual Open Library dump import logic.
// It imports the specified dump types from targetDir concurrently,
// updating the progress reporter as it goes.
func (svc *OpenLibraryService) Import(ctx context.Context, progress operations.ProgressReporter, targetDir string, types []string) error {
	if progress != nil {
		_ = progress.UpdateProgress(0, len(types), fmt.Sprintf("Starting Open Library import (%d dump types)", len(types)))
	}

	var importWg sync.WaitGroup
	var importErr error
	var mu sync.Mutex

	for i, dumpType := range types {
		svc.Mu.Lock()
		if svc.Importing[dumpType] {
			svc.Mu.Unlock()
			log.Printf("[WARN] OL import already in progress for %s", dumpType)
			continue
		}
		svc.Importing[dumpType] = true
		svc.Mu.Unlock()

		if progress != nil {
			_ = progress.Log("info", fmt.Sprintf("Starting %s import", dumpType), nil)
		}

		importWg.Add(1)
		go func(dt string, idx int) {
			defer importWg.Done()
			defer func() {
				svc.Mu.Lock()
				delete(svc.Importing, dt)
				svc.Mu.Unlock()
			}()

			filePath := filepath.Join(targetDir, openlibrary.DumpFilename(dt))
			log.Printf("[INFO] Starting OL dump import: %s from %s", dt, filePath)

			lastReported := 0
			err := svc.OLStore.ImportDump(dt, filePath, func(count int) {
				if count-lastReported >= 50000 {
					lastReported = count
					if progress != nil {
						msg := fmt.Sprintf("Importing %s: %dk records", dt, count/1000)
						_ = progress.UpdateProgress(idx, len(types), msg)
					}
					log.Printf("[INFO] OL %s import progress: %d records", dt, count)
				}
			})

			if err != nil {
				log.Printf("[ERROR] OL dump import failed for %s: %v", dt, err)
				if progress != nil {
					_ = progress.Log("error", fmt.Sprintf("%s import failed: %v", dt, err), nil)
				}
				mu.Lock()
				importErr = err
				mu.Unlock()
			} else {
				log.Printf("[INFO] OL dump import complete: %s", dt)
				if progress != nil {
					_ = progress.Log("info", fmt.Sprintf("%s import complete", dt), nil)
				}
			}
		}(dumpType, i)
	}
	importWg.Wait()

	if progress != nil {
		if importErr != nil {
			_ = progress.UpdateProgress(len(types), len(types), fmt.Sprintf("Import finished with errors: %v", importErr))
		} else {
			_ = progress.UpdateProgress(len(types), len(types), "All Open Library dump imports complete")
		}
	}
	log.Printf("[INFO] All OL dump imports complete")
	return importErr
}
