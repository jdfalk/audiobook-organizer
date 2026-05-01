// file: internal/server/maintenance_hashes.go
// version: 1.0.0
// guid: a8dac396-ed1c-4e35-8d5a-245c3f89b493
// last-edited: 2026-05-01

package server

import (
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/scanner"
)

// handleBackfillMetadataSourceHash handles
// POST /api/v1/maintenance/backfill-metadata-source-hash
//
// Iterates all books that already have a known ASIN or ISBN-13 (or ISBN-10),
// computes sha256("{source}:{id}"), and stores it in metadata_source_hash.
// Books that already have a hash are skipped unless ?force=true is set.
//
// Query params:
//   - dry_run=true  (default) — report what would change without modifying DB
//   - dry_run=false — actually write the hash
//   - force=true    — overwrite existing hashes
func (s *Server) handleBackfillMetadataSourceHash(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") == "true"
	force := c.Query("force") == "true"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	var results []metadataHashBackfillResult
	applied := 0
	skipped := 0
	errors := 0

	for i := range allBooks {
		book := &allBooks[i]

		// Skip if already has hash and force not requested.
		if book.MetadataSourceHash != nil && *book.MetadataSourceHash != "" && !force {
			results = append(results, metadataHashBackfillResult{
				BookID:     book.ID,
				BookTitle:  book.Title,
				Hash:       *book.MetadataSourceHash,
				Skipped:    true,
				SkipReason: "already has metadata_source_hash",
			})
			skipped++
			continue
		}

		// Derive source and canonical ID.
		source, canonicalID := bookMetadataSourceAndID(book)
		if canonicalID == "" {
			results = append(results, metadataHashBackfillResult{
				BookID:     book.ID,
				BookTitle:  book.Title,
				Skipped:    true,
				SkipReason: "no ASIN, ISBN-13, or ISBN-10 available",
			})
			skipped++
			continue
		}

		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(source+":"+canonicalID)))
		result := metadataHashBackfillResult{
			BookID:    book.ID,
			BookTitle: book.Title,
			Hash:      hash,
			Source:    source,
		}

		if !dryRun {
			current, getErr := store.GetBookByID(book.ID)
			if getErr != nil || current == nil {
				result.Error = fmt.Sprintf("GetBookByID: %v", getErr)
				errors++
				results = append(results, result)
				continue
			}
			current.MetadataSourceHash = &hash
			if _, updateErr := store.UpdateBook(book.ID, current); updateErr != nil {
				result.Error = updateErr.Error()
				log.Printf("[WARN] backfill-metadata-source-hash: book %s (%q): %v", book.ID, book.Title, updateErr)
				errors++
				results = append(results, result)
				continue
			}
			result.Applied = true
			applied++
		} else {
			applied++ // dry-run: count as "would apply"
		}

		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run": dryRun,
		"total":   len(allBooks),
		"applied": applied,
		"skipped": skipped,
		"errors":  errors,
		"results": results,
	})
}

// bookMetadataSourceAndID returns the best (source, canonical_id) pair for a
// book, in priority order: ASIN → ISBN-13 → ISBN-10. Returns ("", "") if none
// available.
func bookMetadataSourceAndID(book *database.Book) (source, id string) {
	if book.ASIN != nil && *book.ASIN != "" {
		return "audible", *book.ASIN
	}
	if book.ISBN13 != nil && *book.ISBN13 != "" {
		return "openlibrary", *book.ISBN13
	}
	if book.ISBN10 != nil && *book.ISBN10 != "" {
		return "openlibrary", *book.ISBN10
	}
	return "", ""
}

// chapterGroupResult describes one detected chapter group returned by the
// scan and (optionally) merge endpoints.
type chapterGroupResult struct {
	PrimaryBookID string   `json:"primary_book_id"` // first book ID in the group
	BookIDs       []string `json:"book_ids"`
	CommonTitle   string   `json:"common_title"`
	TotalDuration float64  `json:"total_duration"`
	FileCount     int      `json:"file_count"`
}

// handleScanChapterGroups detects sequential-chapter book groups without
// making any changes to the database.
//
// GET /api/v1/maintenance/chapter-groups
//
// Query params:
//   - min_files=3            minimum file count for a group to be reported
//   - max_per_file_duration=600  max per-file seconds for the short-file heuristic
//   - path_prefix=...        optional: only consider books whose path starts with this value
func (s *Server) handleScanChapterGroups(c *gin.Context) {
	minFiles, _ := strconv.Atoi(c.DefaultQuery("min_files", "3"))
	maxDur, _ := strconv.Atoi(c.DefaultQuery("max_per_file_duration", "600"))
	pathPrefix := c.Query("path_prefix")

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	filtered := allBooks
	if pathPrefix != "" {
		filtered = filtered[:0]
		for _, b := range allBooks {
			if strings.HasPrefix(b.FilePath, pathPrefix) {
				filtered = append(filtered, b)
			}
		}
	}

	groups := scanner.DetectChapterGroups(filtered, minFiles, maxDur)

	results := make([]chapterGroupResult, 0, len(groups))
	totalAffected := 0
	for _, g := range groups {
		if g.FileCount < minFiles {
			continue
		}
		primaryID := ""
		if len(g.BookIDs) > 0 {
			primaryID = g.BookIDs[0]
		}
		results = append(results, chapterGroupResult{
			PrimaryBookID: primaryID,
			BookIDs:       g.BookIDs,
			CommonTitle:   g.CommonTitle,
			TotalDuration: g.TotalDuration,
			FileCount:     g.FileCount,
		})
		totalAffected += g.FileCount
	}

	c.JSON(http.StatusOK, gin.H{
		"groups":               results,
		"total_books_affected": totalAffected,
	})
}

// handleMergeChapterGroups detects and optionally merges sequential-chapter
// book groups.
//
// POST /api/v1/maintenance/merge-chapter-groups
//
// Query params:
//   - dry_run=true/false       (default false) — report without writing
//   - min_files=3
//   - max_per_file_duration=600
func (s *Server) handleMergeChapterGroups(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "false") != "false"
	minFiles, _ := strconv.Atoi(c.DefaultQuery("min_files", "3"))
	maxDur, _ := strconv.Atoi(c.DefaultQuery("max_per_file_duration", "600"))

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	allBooks, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	groups := scanner.DetectChapterGroups(allBooks, minFiles, maxDur)

	groupsFound := 0
	booksMerged := 0
	booksSkipped := 0
	results := make([]chapterGroupResult, 0, len(groups))

	for _, g := range groups {
		if g.FileCount < minFiles {
			booksSkipped += g.FileCount
			continue
		}
		primaryID := g.BookIDs[0]
		srcIDs := g.BookIDs[1:]

		groupsFound++
		result := chapterGroupResult{
			PrimaryBookID: primaryID,
			BookIDs:       g.BookIDs,
			CommonTitle:   g.CommonTitle,
			TotalDuration: g.TotalDuration,
			FileCount:     g.FileCount,
		}
		results = append(results, result)

		if !dryRun && len(srcIDs) > 0 {
			if mergeErr := store.MergeChapterBooks(primaryID, srcIDs, g.CommonTitle, g.TotalDuration); mergeErr != nil {
				log.Printf("[WARN] merge-chapter-groups: group %q (primary %s): %v", g.CommonTitle, primaryID, mergeErr)
				booksSkipped += g.FileCount
				continue
			}
			booksMerged += g.FileCount
		} else {
			booksMerged += g.FileCount // dry-run: count as would-merge
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":       dryRun,
		"groups_found":  groupsFound,
		"books_merged":  booksMerged,
		"books_skipped": booksSkipped,
		"groups":        results,
	})
}

// handleScanDuplicateFiles returns groups of book_files that share the same
// original_file_hash, indicating identical audio content at multiple paths.
//
// GET /api/v1/maintenance/duplicate-files
//
// Query params:
//   - limit=50  (max groups returned; default 50)
//
// Response: { data: { groups: [...], total_wasted_bytes: N, total_groups: N } }
func (s *Server) handleScanDuplicateFiles(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 {
		limit = 50
	}

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	groups, err := store.GetDuplicateFilesByHash(limit)
	if err != nil {
		internalError(c, "failed to scan duplicate files", err)
		return
	}

	var totalWasted int64
	for _, g := range groups {
		if g.FileCount > 1 {
			perFile := g.TotalSize / int64(g.FileCount)
			totalWasted += perFile * int64(g.FileCount-1)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"groups":             groups,
			"total_wasted_bytes": totalWasted,
			"total_groups":       len(groups),
		},
	})
}

// ── MATCH-4: metadata-hash duplicate scan ─────────────────────────────────────

// metadataHashDupBook is the per-book entry returned in a metadata-hash duplicate group.
type metadataHashDupBook struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	FileCount int    `json:"file_count"`
}

// metadataHashDupGroup is one group of books that share a metadata_source_hash.
type metadataHashDupGroup struct {
	Hash  string                `json:"hash"`
	Books []metadataHashDupBook `json:"books"`
}

// handleScanMetadataHashDuplicates scans all books, groups them by
// metadata_source_hash, and returns groups where count > 1.
//
// GET /api/v1/maintenance/metadata-hash-duplicates
//
// Response: { "groups": [...], "total_duplicate_books": N }
func (s *Server) handleScanMetadataHashDuplicates(c *gin.Context) {
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	all, err := store.GetAllBooks(0, 0)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	grouped := make(map[string][]database.Book)
	for _, b := range all {
		if b.MetadataSourceHash == nil || *b.MetadataSourceHash == "" {
			continue
		}
		if b.MergedIntoBookID != nil {
			continue
		}
		grouped[*b.MetadataSourceHash] = append(grouped[*b.MetadataSourceHash], b)
	}

	fileCountMap := make(map[string]int)
	for hash, books := range grouped {
		if len(books) < 2 {
			delete(grouped, hash)
			continue
		}
		for _, b := range books {
			files, fErr := store.GetBookFiles(b.ID)
			if fErr == nil {
				fileCountMap[b.ID] = len(files)
			}
		}
	}

	result := make([]metadataHashDupGroup, 0, len(grouped))
	totalDup := 0
	for hash, books := range grouped {
		entry := metadataHashDupGroup{Hash: hash}
		for _, b := range books {
			entry.Books = append(entry.Books, metadataHashDupBook{
				ID:        b.ID,
				Title:     b.Title,
				FileCount: fileCountMap[b.ID],
			})
		}
		result = append(result, entry)
		totalDup += len(books)
	}

	c.JSON(http.StatusOK, gin.H{
		"groups":                result,
		"total_duplicate_books": totalDup,
	})
}

// handleGetBookFileHashStats returns aggregate hash-coverage statistics for all book_files.
// GET /api/v1/maintenance/book-file-hash-stats
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

// handleBackfillFileHashes hashes every book_file row that is missing a file_hash.
// Reads the file from disk, computes SHA-256, and updates file_hash + original_file_hash.
// POST /api/v1/maintenance/backfill-file-hashes
func (s *Server) handleBackfillFileHashes(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "false") == "true"
	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	all, err := store.GetAllBookFiles()
	if err != nil {
		internalError(c, "failed to list book files", err)
		return
	}

	type result struct {
		FileID   string `json:"file_id"`
		FilePath string `json:"file_path"`
		Hash     string `json:"hash,omitempty"`
		Skipped  bool   `json:"skipped,omitempty"`
		Error    string `json:"error,omitempty"`
	}

	var results []result
	updated, skipped, failed := 0, 0, 0

	for _, bf := range all {
		if bf.FileHash != "" {
			skipped++
			continue
		}
		if bf.FilePath == "" {
			results = append(results, result{FileID: bf.ID, FilePath: bf.FilePath, Skipped: true})
			skipped++
			continue
		}
		h, herr := scanner.ComputeFileHash(bf.FilePath)
		if herr != nil {
			results = append(results, result{FileID: bf.ID, FilePath: bf.FilePath, Error: herr.Error()})
			failed++
			continue
		}
		if !dryRun {
			if uerr := store.SetBookFileHash(bf.ID, h); uerr != nil {
				results = append(results, result{FileID: bf.ID, FilePath: bf.FilePath, Error: uerr.Error()})
				failed++
				continue
			}
		}
		results = append(results, result{FileID: bf.ID, FilePath: bf.FilePath, Hash: h})
		updated++
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run": dryRun,
		"updated": updated,
		"skipped": skipped,
		"failed":  failed,
		"results": results,
	})
}

// handleGetBookMetadataHashStats returns metadata_source_hash coverage across all books.
// GET /api/v1/maintenance/book-metadata-hash-stats
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
