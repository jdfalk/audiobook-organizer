// file: internal/metafetch/openlibrary.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f90

package metafetch

import (
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/openlibrary"
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
