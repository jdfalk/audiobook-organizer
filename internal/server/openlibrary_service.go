// file: internal/server/openlibrary_service.go
// version: 2.1.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f90

package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/openlibrary"
)

// OpenLibraryService manages the Open Library data dump lifecycle.
type OpenLibraryService struct {
	store     *openlibrary.OLStore
	tracker   *openlibrary.DownloadTracker
	mu        sync.Mutex
	importing map[string]bool
}

// getOLDumpDir returns the configured dump directory, falling back to {RootDir}/openlibrary-dumps.
func getOLDumpDir() string {
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
		tracker:   openlibrary.NewDownloadTracker(),
		importing: make(map[string]bool),
	}

	storePath := filepath.Join(getOLDumpDir(), "oldb")
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
			svc.store = store
		}
	}

	return svc
}

// Store returns the underlying OLStore (may be nil).
func (svc *OpenLibraryService) Store() *openlibrary.OLStore {
	return svc.store
}

// Close closes the underlying store.
func (svc *OpenLibraryService) Close() {
	if svc.store != nil {
		svc.store.Close()
	}
}

// --- HTTP Handlers ---

// uploadedFileInfo describes a dump file on disk that hasn't been imported yet.
type uploadedFileInfo struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	ModTime  string `json:"mod_time"`
}

func (s *Server) getOLStatus(c *gin.Context) {
	svc := s.olService
	resp := gin.H{
		"enabled":   config.AppConfig.OpenLibraryDumpEnabled,
		"downloads": svc.tracker.GetAll(),
	}

	if svc.store != nil {
		status, err := svc.store.GetStatus()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		resp["status"] = status
	}

	// Check for uploaded dump files on disk
	dumpDir := getOLDumpDir()
	if dumpDir != "" {
		files := map[string]uploadedFileInfo{}
		for _, dumpType := range []string{"editions", "authors", "works"} {
			path := filepath.Join(dumpDir, openlibrary.DumpFilename(dumpType))
			if info, err := os.Stat(path); err == nil {
				files[dumpType] = uploadedFileInfo{
					Filename: info.Name(),
					Size:     info.Size(),
					ModTime:  info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
				}
			}
		}
		if len(files) > 0 {
			resp["uploaded_files"] = files
		}
	}

	c.JSON(http.StatusOK, resp)
}

type olDownloadRequest struct {
	Types []string `json:"types"`
}

var validDumpTypes = map[string]bool{"editions": true, "authors": true, "works": true}

func (s *Server) startOLDownload(c *gin.Context) {
	var req olDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Types) == 0 {
		req.Types = []string{"editions", "authors", "works"}
	}

	for _, t := range req.Types {
		if !validDumpTypes[t] {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid dump type: %s", t)})
			return
		}
	}

	targetDir := getOLDumpDir()
	if targetDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "openlibrary_dump_dir not configured"})
		return
	}

	tracker := s.olService.tracker
	go func() {
		for _, dumpType := range req.Types {
			log.Printf("[INFO] Starting OL dump download: %s", dumpType)
			err := openlibrary.DownloadDump(dumpType, targetDir, tracker)
			if err != nil {
				log.Printf("[ERROR] OL dump download failed for %s: %v", dumpType, err)
			} else {
				log.Printf("[INFO] OL dump download complete: %s", dumpType)
			}
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": "download started", "types": req.Types})
}

func (s *Server) startOLImport(c *gin.Context) {
	var req olDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Types) == 0 {
		req.Types = []string{"editions", "authors", "works"}
	}

	targetDir := getOLDumpDir()
	if targetDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "openlibrary_dump_dir not configured"})
		return
	}

	svc := s.olService
	svc.mu.Lock()
	if svc.store == nil {
		storePath := filepath.Join(targetDir, "oldb")
		store, err := openlibrary.NewOLStore(storePath)
		if err != nil {
			svc.mu.Unlock()
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to open store: %v", err)})
			return
		}
		svc.store = store
	}
	svc.mu.Unlock()

	go func() {
		var importWg sync.WaitGroup
		for _, dumpType := range req.Types {
			svc.mu.Lock()
			if svc.importing[dumpType] {
				svc.mu.Unlock()
				log.Printf("[WARN] OL import already in progress for %s", dumpType)
				continue
			}
			svc.importing[dumpType] = true
			svc.mu.Unlock()

			importWg.Add(1)
			go func(dt string) {
				defer importWg.Done()
				defer func() {
					svc.mu.Lock()
					delete(svc.importing, dt)
					svc.mu.Unlock()
				}()

				filePath := filepath.Join(targetDir, openlibrary.DumpFilename(dt))
				log.Printf("[INFO] Starting OL dump import: %s from %s", dt, filePath)

				err := svc.store.ImportDump(dt, filePath, func(count int) {
					if count%100000 == 0 {
						log.Printf("[INFO] OL %s import progress: %d records", dt, count)
					}
				})

				if err != nil {
					log.Printf("[ERROR] OL dump import failed for %s: %v", dt, err)
				} else {
					log.Printf("[INFO] OL dump import complete: %s", dt)
				}
			}(dumpType)
		}
		importWg.Wait()
		log.Printf("[INFO] All OL dump imports complete")
	}()

	c.JSON(http.StatusAccepted, gin.H{"message": "import started", "types": req.Types})
}

func (s *Server) uploadOLDump(c *gin.Context) {
	log.Printf("[DEBUG] uploadOLDump: Content-Type=%s, ContentLength=%d", c.ContentType(), c.Request.ContentLength)
	dumpType := c.PostForm("type")
	log.Printf("[DEBUG] uploadOLDump: dumpType=%q", dumpType)
	if !validDumpTypes[dumpType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type must be one of: editions, authors, works"})
		return
	}

	targetDir := getOLDumpDir()
	if targetDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "openlibrary_dump_dir not configured"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	if !strings.HasSuffix(header.Filename, ".gz") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file must be a .gz dump file"})
		return
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create dump dir"})
		return
	}

	targetPath := filepath.Join(targetDir, openlibrary.DumpFilename(dumpType))
	out, err := os.Create(targetPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create target file"})
		return
	}
	defer out.Close()

	written, err := io.Copy(out, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}

	log.Printf("[INFO] OL dump uploaded: %s (%d bytes) -> %s", header.Filename, written, targetPath)
	c.JSON(http.StatusOK, gin.H{
		"message":  "dump file uploaded",
		"type":     dumpType,
		"filename": header.Filename,
		"size":     written,
	})
}

func (s *Server) deleteOLData(c *gin.Context) {
	svc := s.olService
	if svc == nil {
		c.JSON(http.StatusOK, gin.H{"message": "no data to delete"})
		return
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()

	if svc.store != nil {
		svc.store.Close()
		svc.store = nil
	}

	targetDir := getOLDumpDir()
	if targetDir != "" {
		if err := os.RemoveAll(targetDir); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to delete data: %v", err)})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "data deleted"})
}
