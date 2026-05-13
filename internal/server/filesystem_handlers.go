// file: internal/server/filesystem_handlers.go
// version: 2.5.0
// guid: 565db679-19ba-4518-b63e-6892663be41b
// last-edited: 2026-05-10
//
// Filesystem HTTP handlers split out of server.go: home directory,
// filesystem browse, exclusion add/remove, import-path CRUD, and the
// on-demand single-file import endpoint.

package server

import (
	"context"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/importer"
	"github.com/jdfalk/audiobook-organizer/internal/organizer"
	"github.com/jdfalk/audiobook-organizer/internal/plugin"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
	ulid "github.com/oklog/ulid/v2"
)

// filesystemHandlerDeps documents the narrow Server surface needed by the
// filesystem-browse and import-path handlers in this file. *Server satisfies
// this interface automatically via its exported accessor methods.
type filesystemHandlerDeps interface {
	Store() database.Store
	FilesystemService() *fileops.FilesystemService
	ImportPathService() *importer.ImportPathService
	ImportService() *importer.ImportService
	DedupEngine() *dedup.Engine
	publishEvent(ctx context.Context, event plugin.Event)
}

var _ filesystemHandlerDeps = (*Server)(nil)

// getHomeDirectory returns the server user's home directory path.
func (s *Server) getHomeDirectory(c *gin.Context) {
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		httputil.RespondWithInternalError(c, "failed to determine home directory")
		return
	}

	httputil.RespondWithOK(c, gin.H{"path": homeDir})
}

func (s *Server) browseFilesystem(c *gin.Context) {
	path := c.Query("path")
	result, err := s.filesystemService.BrowseDirectory(path)
	if err != nil {
		if errors.Is(err, fileops.ErrPathNotAllowed) {
			httputil.RespondWithForbidden(c, err.Error())
			return
		}
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	httputil.RespondWithOK(c, result)
}

func (s *Server) createExclusion(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if err := s.filesystemService.CreateExclusion(req.Path); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	httputil.RespondWithCreated(c, gin.H{"message": "exclusion created"})
}

func (s *Server) removeExclusion(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	if err := s.filesystemService.RemoveExclusion(req.Path); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	httputil.RespondWithNoContent(c)
}

func (s *Server) listImportPaths(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	folders, err := s.Store().GetAllImportPaths()
	if err != nil {
		httputil.InternalError(c, "failed to list import paths", err)
		return
	}

	// Ensure we never return null - always return empty array
	if folders == nil {
		folders = []database.ImportPath{}
	}

	// Refresh BookCount with the live value from the books table. The
	// stored ImportPath.BookCount is only updated when an auto-scan
	// completes (folder_autoscan_op.go) or when a path is first added
	// (addImportPath, below); without this refresh a path can sit at
	// "0 books found" indefinitely if the user added the path but
	// never triggered a scan, or if a scan failed partway, or if books
	// got created via a different path that happens to overlap.
	for i := range folders {
		if cnt, cerr := s.Store().CountBooksByPathPrefix(folders[i].Path); cerr == nil {
			folders[i].BookCount = cnt
		}
	}

	httputil.RespondWithOK(c, gin.H{"importPaths": folders, "count": len(folders)})
}

func (s *Server) addImportPath(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	var req struct {
		Path    string `json:"path" binding:"required"`
		Name    string `json:"name" binding:"required"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	createdPath, err := s.importPathService.CreateImportPath(req.Path, req.Name)
	if err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}
	folder := createdPath
	if req.Enabled != nil && !*req.Enabled {
		folder.Enabled = false
		if err := s.Store().UpdateImportPath(folder.ID, folder); err != nil {
			// Non-fatal; return created folder anyway with note
			httputil.RespondWithCreated(c, gin.H{"importPath": folder, "warning": "created but could not update enabled flag"})
			return
		}
	}

	// Auto-scan the newly added folder if enabled and the v2 op registry is available.
	if folder.Enabled && s.opRegistry != nil {
		opID := ulid.Make().String()
		folderPath := folder.Path
		_, err := s.Store().CreateOperation(opID, "scan", &folderPath)
		if err == nil {
			params := folderAutoScanOpParams{
				LegacyOpID: opID,
				FolderPath: folderPath,
				FolderID:   folder.ID,
			}
			if _, enqErr := s.opRegistry.EnqueueOp(c.Request.Context(), "library.folder-auto-scan", params); enqErr == nil {
				httputil.RespondWithCreated(c, gin.H{"importPath": folder, "scan_operation_id": opID})
				return
			}
		}
	}

	// Fallback: if enabled but op registry unavailable OR operation creation failed, run synchronous scan.
	if folder.Enabled && s.opRegistry == nil {
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
								scanner.ApplyOrganizedFileMetadata(dbBook, newPath)
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

	httputil.RespondWithCreated(c, gin.H{"importPath": folder})
}

func (s *Server) removeImportPath(c *gin.Context) {
	if s.Store() == nil {
		httputil.RespondWithInternalError(c, "database not initialized")
		return
	}
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httputil.RespondWithBadRequest(c, "invalid import path id")
		return
	}
	if err := s.Store().DeleteImportPath(id); err != nil {
		httputil.InternalError(c, "failed to remove import path", err)
		return
	}
	httputil.RespondWithNoContent(c)
}

func (s *Server) importFile(c *gin.Context) {
	var req importer.ImportFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	result, err := s.importService.ImportFile(&req)
	if err != nil {
		httputil.RespondWithBadRequest(c, err.Error())
		return
	}

	s.publishEvent(c.Request.Context(), plugin.NewEvent(plugin.EventBookImported, result.ID, map[string]any{
		"file_path": result.FilePath,
		"source":    "import",
	}))

	httputil.RespondWithCreated(c, result)
}
