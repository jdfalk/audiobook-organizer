// file: internal/server/filesystem_handlers.go
// version: 1.2.0
// guid: 565db679-19ba-4518-b63e-6892663be41b
//
// Filesystem HTTP handlers split out of server.go: home directory,
// filesystem browse, exclusion add/remove, import-path CRUD, and the
// on-demand single-file import endpoint.

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/plugin"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	ulid "github.com/oklog/ulid/v2"
)

// getHomeDirectory returns the server user's home directory path.
func (s *Server) getHomeDirectory(c *gin.Context) {
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to determine home directory"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"path": homeDir})
}

func (s *Server) browseFilesystem(c *gin.Context) {
	path := c.Query("path")
	result, err := s.filesystemService.BrowseDirectory(path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) createExclusion(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.filesystemService.CreateExclusion(req.Path); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "exclusion created"})
}

func (s *Server) removeExclusion(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := s.filesystemService.RemoveExclusion(req.Path); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) listImportPaths(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	folders, err := s.Store().GetAllImportPaths()
	if err != nil {
		internalError(c, "failed to list import paths", err)
		return
	}

	// Ensure we never return null - always return empty array
	if folders == nil {
		folders = []database.ImportPath{}
	}

	c.JSON(http.StatusOK, gin.H{"importPaths": folders, "count": len(folders)})
}

func (s *Server) addImportPath(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var req struct {
		Path    string `json:"path" binding:"required"`
		Name    string `json:"name" binding:"required"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	createdPath, err := s.importPathService.CreateImportPath(req.Path, req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	folder := createdPath
	if req.Enabled != nil && !*req.Enabled {
		folder.Enabled = false
		if err := s.Store().UpdateImportPath(folder.ID, folder); err != nil {
			// Non-fatal; return created folder anyway with note
			c.JSON(http.StatusCreated, gin.H{"importPath": folder, "warning": "created but could not update enabled flag"})
			return
		}
	}

	// Auto-scan the newly added folder if enabled and operation queue is available
	if folder.Enabled && s.queue != nil {
		opID := ulid.Make().String()
		folderPath := folder.Path
		op, err := s.Store().CreateOperation(opID, "scan", &folderPath)
		if err == nil {
			// Create scan operation function
			operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
				_ = progress.Log("info", fmt.Sprintf("Auto-scanning newly added folder: %s", folderPath), nil)

				// Check if folder exists
				if _, err := os.Stat(folderPath); os.IsNotExist(err) {
					return fmt.Errorf("folder does not exist: %s", folderPath)
				}

				// Scan directory for audiobook files (parallel)
				workers := config.AppConfig.ConcurrentScans
				if workers < 1 {
					workers = 4
				}
				scanLog := operations.LoggerFromReporter(progress)
				books, err := scanner.ScanDirectoryParallel(folderPath, workers, scanLog)
				if err != nil {
					return fmt.Errorf("failed to scan folder: %w", err)
				}

				scanLog.Info("Found %d audiobook files", len(books))

				// Process the books to extract metadata (parallel)
				if len(books) > 0 {
					scanLog.Info("Processing metadata for %d books using %d workers", len(books), workers)
					if err := scanner.ProcessBooksParallel(ctx, books, workers, nil, scanLog); err != nil {
						return fmt.Errorf("failed to process books: %w", err)
					}
					// Auto-organize if enabled
					if config.AppConfig.AutoOrganize && config.AppConfig.RootDir != "" {
						org := organizer.NewOrganizer(&config.AppConfig)
						organized := 0
						for _, b := range books {
							// Lookup DB book by file path
							dbBook, err := s.Store().GetBookByFilePath(b.FilePath)
							if err != nil || dbBook == nil {
								continue
							}
							newPath, _, err := org.OrganizeBook(dbBook)
							if err != nil {
								_ = progress.Log("warn", fmt.Sprintf("Organize failed for %s: %v", dbBook.Title, err), nil)
								continue
							}
							if newPath != dbBook.FilePath {
								dbBook.FilePath = newPath
								applyOrganizedFileMetadata(dbBook, newPath)
								if _, err := s.Store().UpdateBook(dbBook.ID, dbBook); err != nil {
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

				// Trigger dedup check on newly scanned books
				if s.dedupEngine != nil && len(books) > 0 {
					go func() {
						for _, b := range books {
							dbBook, err := s.Store().GetBookByFilePath(b.FilePath)
							if err != nil || dbBook == nil {
								continue
							}
							if _, err := s.dedupEngine.CheckBook(context.Background(), dbBook.ID); err != nil {
								log.Printf("[WARN] dedup check failed for scanned book %s: %v", dbBook.ID, err)
							}
						}
					}()
				}

				// Update book count for this import path
				folder.BookCount = len(books)
				now := time.Now()
				folder.LastScan = &now
				if err := s.Store().UpdateImportPath(folder.ID, folder); err != nil {
					_ = progress.Log("warn", fmt.Sprintf("Failed to update book count: %v", err), nil)
				}

				_ = progress.Log("info", fmt.Sprintf("Auto-scan completed. Total books: %d", len(books)), nil)
				return nil
			}

			// Enqueue the scan operation with normal priority
			_ = s.queue.Enqueue(op.ID, "scan", operations.PriorityNormal, operationFunc)

			c.JSON(http.StatusCreated, gin.H{"importPath": folder, "scan_operation_id": op.ID})
			return
		}
	}

	// Fallback: if enabled but queue unavailable OR operation creation failed, run synchronous scan
	if folder.Enabled && s.queue == nil {
		// Basic scan without progress reporter
		if _, err := os.Stat(folder.Path); err == nil {
			books, err := scanner.ScanDirectory(folder.Path, nil)
			if err == nil {
				if len(books) > 0 {
					_ = scanner.ProcessBooks(books, nil) // ignore individual processing errors (already logged internally)
					// Auto-organize if enabled
					if config.AppConfig.AutoOrganize && config.AppConfig.RootDir != "" {
						org := organizer.NewOrganizer(&config.AppConfig)
						for _, b := range books {
							dbBook, err := s.Store().GetBookByFilePath(b.FilePath)
							if err != nil || dbBook == nil {
								continue
							}
							newPath, _, err := org.OrganizeBook(dbBook)
							if err != nil {
								continue
							}
							if newPath != dbBook.FilePath {
								dbBook.FilePath = newPath
								applyOrganizedFileMetadata(dbBook, newPath)
								_, _ = s.Store().UpdateBook(dbBook.ID, dbBook)
							}
						}
					} else if config.AppConfig.AutoOrganize && config.AppConfig.RootDir == "" {
						log.Printf("auto-organize enabled but root_dir not set")
					}
				}
				folder.BookCount = len(books)
				now := time.Now()
				folder.LastScan = &now
				_ = s.Store().UpdateImportPath(folder.ID, folder)
			}
		}
	}

	c.JSON(http.StatusCreated, gin.H{"importPath": folder})
}

func (s *Server) removeImportPath(c *gin.Context) {
	if s.Store() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid import path id"})
		return
	}
	if err := s.Store().DeleteImportPath(id); err != nil {
		internalError(c, "failed to remove import path", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) importFile(c *gin.Context) {
	var req ImportFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := s.importService.ImportFile(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s.publishEvent(c.Request.Context(), plugin.NewEvent(plugin.EventBookImported, result.ID, map[string]any{
		"file_path": result.FilePath,
		"source":    "import",
	}))

	c.JSON(http.StatusCreated, result)
}
