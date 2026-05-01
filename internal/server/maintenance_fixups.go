// file: internal/server/maintenance_fixups.go
// version: 2.0.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d
// last-edited: 2026-05-02

package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/activity"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// maintenanceStore is the narrow slice of database.Store that the
// wipe-helper free functions accept.
type maintenanceStore interface {
	database.BookStore
	database.AuthorStore
	database.SeriesStore
	database.BookFileStore
	database.UserTagStore
	database.ExternalIDStore
	database.StatsStore
}

func (s *Server) handleWipe(c *gin.Context) {
	var req struct {
		Targets []string `json:"targets"`
		Confirm string   `json:"confirm"`
		DryRun  bool     `json:"dry_run"`
	}
	// Default dry_run to true before binding.
	req.DryRun = true

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.Confirm != "WIPE" {
		c.JSON(http.StatusBadRequest, gin.H{"error": `must include "confirm": "WIPE" in the request body`})
		return
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	// Expand "all" to every individual target.
	targetSet := make(map[string]bool, len(req.Targets))
	for _, t := range req.Targets {
		targetSet[t] = true
	}
	if targetSet["all"] {
		for _, t := range []string{
			"books", "book_files", "segments", "files",
			"organized_folders", "activity", "authors", "series", "external_ids",
		} {
			targetSet[t] = true
		}
	}

	results := make(map[string]int64)
	dryRun := req.DryRun

	// ── organized_folders ──────────────────────────────────────────────────
	if targetSet["organized_folders"] {
		rootDir := config.AppConfig.RootDir
		keep := map[string]bool{
			".covers":           true,
			".itunes-writeback": true,
			"openlibrary-dumps": true,
		}
		entries, err := os.ReadDir(rootDir)
		if err != nil {
			log.Printf("[WARN] wipe: can't read root dir %q: %v", rootDir, err)
		} else {
			var count int64
			for _, e := range entries {
				// Skip hidden dirs (starting with ".") that are not in the keeplist,
				// but only delete non-hidden dirs or explicitly non-kept hidden dirs.
				if strings.HasPrefix(e.Name(), ".") && !keep[e.Name()] {
					continue // skip unknown hidden dirs
				}
				if keep[e.Name()] {
					continue
				}
				fullPath := filepath.Join(rootDir, e.Name())
				log.Printf("[INFO] wipe: organized_folders: %s %q", dryRunLabel(dryRun), fullPath)
				if !dryRun {
					if err := os.RemoveAll(fullPath); err != nil {
						log.Printf("[WARN] wipe: RemoveAll %q: %v", fullPath, err)
					}
				}
				count++
			}
			results["organized_folders"] = count
		}
	}

	// ── files (disk + db rows) ─────────────────────────────────────────────
	// "files" implies "book_files" as well — collect file paths first, then delete rows.
	if targetSet["files"] {
		rootDir := config.AppConfig.RootDir
		var count int64
		offset := 0
		batchSize := 500
		for {
			books, err := store.GetAllBooks(batchSize, offset)
			if err != nil {
				log.Printf("[WARN] wipe: files: GetAllBooks: %v", err)
				break
			}
			for _, book := range books {
				files, ferr := store.GetBookFiles(book.ID)
				if ferr != nil {
					log.Printf("[WARN] wipe: files: GetBookFiles %s: %v", book.ID, ferr)
					continue
				}
				for _, bf := range files {
					if bf.FilePath == "" {
						continue
					}
					// Only remove files inside the organizer root dir — never iTunes paths.
					if !strings.HasPrefix(filepath.Clean(bf.FilePath), filepath.Clean(rootDir)) {
						continue
					}
					log.Printf("[INFO] wipe: files: %s %q", dryRunLabel(dryRun), bf.FilePath)
					if !dryRun {
						if rerr := os.Remove(bf.FilePath); rerr != nil && !os.IsNotExist(rerr) {
							log.Printf("[WARN] wipe: os.Remove %q: %v", bf.FilePath, rerr)
						}
					}
					count++
				}
			}
			if len(books) < batchSize {
				break
			}
			offset += batchSize
		}
		results["files"] = count
		// "files" also deletes the book_file rows — mark book_files as well.
		targetSet["book_files"] = true
	}

	// ── book_files (db rows only) ──────────────────────────────────────────
	if targetSet["book_files"] {
		n, err := wipeBookFiles(store, dryRun)
		if err != nil {
			log.Printf("[WARN] wipe: book_files: %v", err)
		}
		results["book_files"] = n
	}

	// ── segments ──────────────────────────────────────────────────────────
	if targetSet["segments"] {
		n, err := wipeSegments(store, dryRun)
		if err != nil {
			log.Printf("[WARN] wipe: segments: %v", err)
		}
		results["segments"] = n
	}

	// ── books ──────────────────────────────────────────────────────────────
	if targetSet["books"] {
		n, err := wipeBooks(store, dryRun)
		if err != nil {
			log.Printf("[WARN] wipe: books: %v", err)
		}
		results["books"] = n
	}

	// ── authors ────────────────────────────────────────────────────────────
	if targetSet["authors"] {
		n, err := wipeAuthors(store, dryRun)
		if err != nil {
			log.Printf("[WARN] wipe: authors: %v", err)
		}
		results["authors"] = n
	}

	// ── series ─────────────────────────────────────────────────────────────
	if targetSet["series"] {
		n, err := wipeSeries(store, dryRun)
		if err != nil {
			log.Printf("[WARN] wipe: series: %v", err)
		}
		results["series"] = n
	}

	// ── external_ids ───────────────────────────────────────────────────────
	if targetSet["external_ids"] {
		n, err := wipeExternalIDs(store, dryRun)
		if err != nil {
			log.Printf("[WARN] wipe: external_ids: %v", err)
		}
		results["external_ids"] = n
	}

	// ── activity ──────────────────────────────────────────────────────────
	if targetSet["activity"] {
		if s.activityService != nil {
			n, err := wipeActivity(s.activityService, dryRun)
			if err != nil {
				log.Printf("[WARN] wipe: activity: %v", err)
			}
			results["activity"] = n
		} else {
			log.Printf("[INFO] wipe: activity: activityService not initialized, skipping")
		}
	}

	log.Printf("[INFO] wipe: complete dry_run=%v targets=%v results=%v", dryRun, req.Targets, results)
	c.JSON(http.StatusOK, gin.H{
		"dry_run": dryRun,
		"results": results,
	})
}

// dryRunLabel returns a label for logging.
func dryRunLabel(dryRun bool) string {
	if dryRun {
		return "[dry-run] would delete"
	}
	return "deleting"
}

// wipeBookFiles deletes all book_file rows using the appropriate store backend.
func wipeBookFiles(store maintenanceStore, dryRun bool) (int64, error) {
	if dryRun {
		// Count only.
		n, err := store.CountFiles()
		return int64(n), err
	}
	switch s := store.(type) {
	case *database.SQLiteStore:
		return s.WipeTable("book_files")
	case *database.PebbleStore:
		n, err := s.WipeByPrefixes([]string{"book_file:"})
		return int64(n), err
	default:
		// Fallback: iterate all books and delete via interface.
		var count int64
		offset := 0
		for {
			books, err := store.GetAllBooks(500, offset)
			if err != nil {
				return count, err
			}
			for _, book := range books {
				if err := store.DeleteBookFilesForBook(book.ID); err != nil {
					log.Printf("[WARN] wipeBookFiles: DeleteBookFilesForBook %s: %v", book.ID, err)
				}
				count++ // approximate
			}
			if len(books) < 500 {
				break
			}
			offset += 500
		}
		return count, nil
	}
}

// wipeSegments deletes all book_segment rows using the appropriate store backend.
func wipeSegments(store maintenanceStore, dryRun bool) (int64, error) {
	switch s := store.(type) {
	case *database.SQLiteStore:
		if dryRun {
			return s.CountTableRows("book_segments")
		}
		return s.WipeTable("book_segments")
	case *database.PebbleStore:
		// Pebble segments use "bf:" (primary) and "bfs:" (secondary) prefixes.
		if dryRun {
			n, err := s.CountByPrefix("bf:")
			return int64(n), err
		}
		n, err := s.WipeByPrefixes([]string{"bf:", "bfs:"})
		return int64(n), err
	default:
		return 0, fmt.Errorf("wipeSegments: unsupported store type %T", store)
	}
}

// wipeBooks deletes all book rows using the appropriate store backend.
func wipeBooks(store maintenanceStore, dryRun bool) (int64, error) {
	if dryRun {
		n, err := store.CountBooks()
		return int64(n), err
	}
	switch s := store.(type) {
	case *database.SQLiteStore:
		return s.WipeTable("books")
	case *database.PebbleStore:
		// Book keys: "book:" prefix. Include secondary indexes.
		n, err := s.WipeByPrefixes([]string{"book:"})
		return int64(n), err
	default:
		return 0, fmt.Errorf("wipeBooks: unsupported store type %T", store)
	}
}

// wipeAuthors deletes all author rows using the appropriate store backend.
func wipeAuthors(store maintenanceStore, dryRun bool) (int64, error) {
	if dryRun {
		n, err := store.CountAuthors()
		return int64(n), err
	}
	switch s := store.(type) {
	case *database.SQLiteStore:
		return s.WipeTable("authors")
	case *database.PebbleStore:
		n, err := s.WipeByPrefixes([]string{"author:"})
		return int64(n), err
	default:
		return 0, fmt.Errorf("wipeAuthors: unsupported store type %T", store)
	}
}

// wipeSeries deletes all series rows using the appropriate store backend.
func wipeSeries(store maintenanceStore, dryRun bool) (int64, error) {
	if dryRun {
		n, err := store.CountSeries()
		return int64(n), err
	}
	switch s := store.(type) {
	case *database.SQLiteStore:
		return s.WipeTable("series")
	case *database.PebbleStore:
		n, err := s.WipeByPrefixes([]string{"series:"})
		return int64(n), err
	default:
		return 0, fmt.Errorf("wipeSeries: unsupported store type %T", store)
	}
}

// wipeExternalIDs deletes all external_id_map rows using the appropriate store backend.
func wipeExternalIDs(store maintenanceStore, dryRun bool) (int64, error) {
	switch s := store.(type) {
	case *database.SQLiteStore:
		if dryRun {
			return s.CountTableRows("external_id_map")
		}
		return s.WipeTable("external_id_map")
	case *database.PebbleStore:
		if dryRun {
			n, err := s.CountByPrefix("ext_id:")
			return int64(n), err
		}
		// "ext_id:" covers both "ext_id:<source>:<id>" and "ext_id:book:<bookID>:<source>:<id>"
		n, err := s.WipeByPrefixes([]string{"ext_id:"})
		return int64(n), err
	default:
		return 0, fmt.Errorf("wipeExternalIDs: unsupported store type %T", store)
	}
}

// wipeActivity deletes all activity log entries.
func wipeActivity(svc *activity.Service, dryRun bool) (int64, error) {
	if dryRun {
		entries, total, err := svc.Query(database.ActivityFilter{Limit: 1})
		if err != nil {
			return 0, err
		}
		_ = entries
		return int64(total), nil
	}
	return svc.Store().WipeAllActivity()
}

// composerTagResult describes the COMPOSER field state for one audio file.
type composerTagResult struct {
	BookID    string `json:"book_id"`
	BookTitle string `json:"book_title"`
	FilePath  string `json:"file_path"`
	// Category is one of: "ok", "composer_equals_author", "composer_equals_narrator",
	// "composer_mismatch", "missing_narrator", "read_error".
	Category  string `json:"category"`
	Composer  string `json:"composer_on_disk"`
	Author    string `json:"author,omitempty"`
	Narrator  string `json:"narrator,omitempty"`
	WillWrite string `json:"will_write,omitempty"`
	Applied   bool   `json:"applied,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) handleGetComposerScanResults(c *gin.Context) {
	opID := c.Param("id")
	if opID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "operation id required"})
		return
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	op, err := store.GetOperationByID(opID)
	if err != nil || op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}
	if op.Type != "composer_tag_scan" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not a composer_tag_scan operation"})
		return
	}

	rawResults, err := store.GetOperationResults(opID)
	if err != nil {
		internalError(c, "failed to load results", err)
		return
	}

	counts := map[string]int{}
	var problems []composerTagResult
	for _, raw := range rawResults {
		var r composerTagResult
		if err := json.Unmarshal([]byte(raw.ResultJSON), &r); err != nil {
			continue
		}
		counts[r.Category]++
		if r.Category != "ok" && r.Category != "missing" {
			problems = append(problems, r)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"operation_id": opID,
		"status":       op.Status,
		"progress":     op.Progress,
		"total":        op.Total,
		"by_category":  counts,
		"problems":     len(problems),
		"details":      problems,
	})
}

type missingFileRepairResult struct {
	FileID  string `json:"file_id"`
	BookID  string `json:"book_id"`
	Title   string `json:"book_title"`
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path,omitempty"`
	// Method values: "pid", "filename", "truncation", "author_title",
	// "skipped", "unresolved", "ambiguous"
	Method  string `json:"method"`
	Matches int    `json:"matches,omitempty"`
	Applied bool   `json:"applied"`
	Error   string `json:"error,omitempty"`
}

func (s *Server) handleGetMissingFileRepairResults(c *gin.Context) {
	opID := c.Param("id")
	if opID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "operation id required"})
		return
	}
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	op, err := store.GetOperationByID(opID)
	if err != nil || op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}
	if op.Type != "missing-file-repair" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not a missing-file-repair operation"})
		return
	}
	rawResults, err := store.GetOperationResults(opID)
	if err != nil {
		internalError(c, "failed to load results", err)
		return
	}

	byMethod := map[string]int{}
	var problems []missingFileRepairResult
	repaired, unresolved, ambiguous, skipped := 0, 0, 0, 0
	for _, raw := range rawResults {
		var r missingFileRepairResult
		if jsonErr := json.Unmarshal([]byte(raw.ResultJSON), &r); jsonErr != nil {
			continue
		}
		byMethod[r.Method]++
		switch r.Method {
		case "unresolved":
			unresolved++
			problems = append(problems, r)
		case "ambiguous":
			ambiguous++
			problems = append(problems, r)
		case "skipped":
			skipped++
		default:
			repaired++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"operation_id": opID,
		"status":       op.Status,
		"progress":     op.Progress,
		"total":        op.Total,
		"by_method":    byMethod,
		"repaired":     repaired,
		"unresolved":   unresolved,
		"ambiguous":    ambiguous,
		"skipped":      skipped,
		"problems":     problems,
	})
}

func (s *Server) handleGetBookFileHashStats(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	stats, err := store.GetBookFileHashStats()
	if err != nil {
		internalError(c, "failed to get book file hash stats", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stats})
}

func (s *Server) handleGetBookMetadataHashStats(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	stats, err := store.GetBookMetadataHashStats()
	if err != nil {
		internalError(c, "failed to get book metadata hash stats", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stats})
}
