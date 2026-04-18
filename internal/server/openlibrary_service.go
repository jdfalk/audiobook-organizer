// file: internal/server/openlibrary_service.go
// version: 2.6.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f90

package server

import (
	"context"
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
	"github.com/jdfalk/audiobook-organizer/internal/metafetch"
	"github.com/jdfalk/audiobook-organizer/internal/openlibrary"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/oklog/ulid/v2"
)

// --- HTTP Handlers ---

func (s *Server) getOLStatus(c *gin.Context) {
	svc := s.olService
	resp := gin.H{
		"enabled":   config.AppConfig.OpenLibraryDumpEnabled,
		"downloads": svc.Tracker.GetAll(),
	}

	if svc.OLStore != nil {
		status, err := svc.OLStore.GetStatus()
		if err != nil {
			internalError(c, "failed to get OpenLibrary status", err)
			return
		}
		resp["status"] = status
	}

	// Check for uploaded dump files on disk
	dumpDir := metafetch.GetOLDumpDir()
	if dumpDir != "" {
		files := map[string]metafetch.UploadedFileInfo{}
		for _, dumpType := range []string{"editions", "authors", "works"} {
			path := filepath.Join(dumpDir, openlibrary.DumpFilename(dumpType))
			if info, err := os.Stat(path); err == nil {
				files[dumpType] = metafetch.UploadedFileInfo{
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

func (s *Server) startOLDownload(c *gin.Context) {
	var req olDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Types) == 0 {
		req.Types = []string{"editions", "authors", "works"}
	}

	for _, t := range req.Types {
		if !metafetch.ValidDumpTypes[t] {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid dump type: %s", t)})
			return
		}
	}

	targetDir := metafetch.GetOLDumpDir()
	if targetDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "openlibrary_dump_dir not configured"})
		return
	}

	tracker := s.olService.Tracker
	store := s.Store()
	opID := ulid.Make().String()
	folderPath := targetDir
	if store != nil {
		_, _ = store.CreateOperation(opID, "ol_dump_download", &folderPath)
	}

	oq := s.queue
	if oq != nil {
		err := oq.Enqueue(opID, "ol_dump_download", operations.PriorityNormal,
			func(ctx context.Context, progress operations.ProgressReporter) error {
				for i, dumpType := range req.Types {
					if progress != nil && progress.IsCanceled() {
						return fmt.Errorf("download canceled")
					}
					if progress != nil {
						_ = progress.Log("info", fmt.Sprintf("Starting OL dump download: %s", dumpType), nil)
						_ = progress.UpdateProgress(i, len(req.Types), fmt.Sprintf("Downloading %s...", dumpType))
					}
					err := openlibrary.DownloadDump(dumpType, targetDir, tracker)
					if err != nil {
						if progress != nil {
							_ = progress.Log("error", fmt.Sprintf("OL dump download failed for %s: %v", dumpType, err), nil)
						}
						return fmt.Errorf("download failed for %s: %w", dumpType, err)
					}
					if progress != nil {
						_ = progress.Log("info", fmt.Sprintf("OL dump download complete: %s", dumpType), nil)
					}
				}
				if progress != nil {
					_ = progress.UpdateProgress(len(req.Types), len(req.Types), "All downloads complete")
				}
				return nil
			},
		)
		if err != nil {
			log.Printf("[WARN] Failed to enqueue OL download, running directly: %v", err)
			go func() {
				for _, dumpType := range req.Types {
					_ = openlibrary.DownloadDump(dumpType, targetDir, tracker)
				}
			}()
		}
	} else {
		go func() {
			for _, dumpType := range req.Types {
				_ = openlibrary.DownloadDump(dumpType, targetDir, tracker)
			}
		}()
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "download started", "types": req.Types, "operation_id": opID})
}

func (s *Server) startOLImport(c *gin.Context) {
	var req olDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Types) == 0 {
		req.Types = []string{"editions", "authors", "works"}
	}

	targetDir := metafetch.GetOLDumpDir()
	if targetDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "openlibrary_dump_dir not configured"})
		return
	}

	svc := s.olService
	if err := svc.EnsureStore(targetDir); err != nil {
		internalError(c, "failed to open store", err)
		return
	}

	store := s.Store()
	opID := ulid.Make().String()
	folderPath := targetDir
	if store != nil {
		_, _ = store.CreateOperation(opID, "ol_dump_import", &folderPath)
	}

	oq := s.queue
	if oq != nil {
		typesStr := strings.Join(req.Types, ",")
		err := oq.Enqueue(opID, "ol_dump_import", operations.PriorityNormal,
			func(ctx context.Context, progress operations.ProgressReporter) error {
				return s.executeOLImport(ctx, progress, svc, targetDir, req.Types)
			},
		)
		if err != nil {
			log.Printf("[WARN] Failed to enqueue OL import, running directly: %v", err)
			go func() {
				_ = s.executeOLImport(context.Background(), nil, svc, targetDir, req.Types)
			}()
		}
		_ = typesStr // used for logging if needed
	} else {
		// Fallback: no queue, run directly
		go func() {
			_ = s.executeOLImport(context.Background(), nil, svc, targetDir, req.Types)
		}()
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "import started", "types": req.Types, "operation_id": opID})
}

func (s *Server) executeOLImport(ctx context.Context, progress operations.ProgressReporter, svc *metafetch.OpenLibraryService, targetDir string, types []string) error {
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

func (s *Server) uploadOLDump(c *gin.Context) {
	log.Printf("[DEBUG] uploadOLDump: Content-Type=%s, ContentLength=%d", c.ContentType(), c.Request.ContentLength)
	dumpType := c.PostForm("type")
	log.Printf("[DEBUG] uploadOLDump: dumpType=%q", dumpType)
	if !metafetch.ValidDumpTypes[dumpType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type must be one of: editions, authors, works"})
		return
	}

	targetDir := metafetch.GetOLDumpDir()
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

	if err := os.MkdirAll(targetDir, 0o775); err != nil {
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

	svc.Mu.Lock()
	defer svc.Mu.Unlock()

	if svc.OLStore != nil {
		svc.OLStore.Close()
		svc.OLStore = nil
	}

	targetDir := metafetch.GetOLDumpDir()
	if targetDir != "" {
		if err := os.RemoveAll(targetDir); err != nil {
			internalError(c, "failed to delete data", err)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "data deleted"})
}
