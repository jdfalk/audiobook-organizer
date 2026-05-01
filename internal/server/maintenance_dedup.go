// file: internal/server/maintenance_dedup.go
// version: 1.0.0
// guid: 9afb479b-8aac-4b43-8b40-9b20ca285fa8
// last-edited: 2026-05-01

package server

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/dedup"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

// ---------------------------------------------------------------------------
// Book Deduplication
// ---------------------------------------------------------------------------

// dedupBooksResult summarises the outcome of handleDedupBooks.
type dedupBooksResult struct {
	DryRun                 bool     `json:"dry_run"`
	Phase1JunkDeleted      int      `json:"phase1_junk_deleted"`
	Phase2PathDupesMerged  int      `json:"phase2_path_dupes_merged"`
	Phase3TitleDupesMerged int      `json:"phase3_title_dupes_merged"`
	Phase4VGDupesCleaned   int      `json:"phase4_vg_dupes_cleaned"`
	TotalBooksRemoved      int      `json:"total_books_removed"`
	Errors                 int      `json:"errors"`
	Details                gin.H    `json:"details"`
	ErrorMessages          []string `json:"error_messages,omitempty"`
}

// dedupMergeDetail describes one merge action.
type dedupMergeDetail struct {
	KeeperID    string   `json:"keeper_id"`
	KeeperTitle string   `json:"keeper_title"`
	RemovedIDs  []string `json:"removed_ids"`
	Reason      string   `json:"reason"`
}

// handleDedupBooks runs a 4-phase book deduplication scan (dry_run=true by default).
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually execute
//
// Phases:
//  1. Delete junk "read by narrator" records with no useful metadata
//  2. Merge books pointing to the same file_path (keep most metadata)
//  3. Merge books with same normalised title+author in the same directory
//  4. Remove duplicate entries inside version groups
func (s *Server) handleDedupBooks(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") == "true"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	result := dedupBooksResult{DryRun: dryRun}
	var errorMessages []string

	// Fetch all books in batches (12K+ books in production).
	allBooks, err := fetchAllBooksPaginated(store)
	if err != nil {
		internalError(c, "failed to list books", err)
		return
	}

	// deletedIDs tracks books already removed in earlier phases so later
	// phases can skip them.
	deletedIDs := make(map[string]bool)

	// -----------------------------------------------------------------------
	// Phase 1: Delete junk "read by narrator" records
	// -----------------------------------------------------------------------
	var phase1Details []dedupMergeDetail

	for i := range allBooks {
		book := &allBooks[i]
		if deletedIDs[book.ID] {
			continue
		}
		if !isJunkReadByNarrator(book) {
			continue
		}

		log.Printf("[INFO] dedup-books phase1: junk record %s title=%q", book.ID, book.Title)
		if !dryRun {
			if delErr := softDeleteBook(store, book.ID); delErr != nil {
				errorMessages = append(errorMessages, fmt.Sprintf("phase1 delete %s: %v", book.ID, delErr))
				result.Errors++
				continue
			}
		}
		deletedIDs[book.ID] = true
		result.Phase1JunkDeleted++
		phase1Details = append(phase1Details, dedupMergeDetail{
			KeeperID:    "",
			KeeperTitle: "",
			RemovedIDs:  []string{book.ID},
			Reason:      "junk: title is 'read by narrator' with no useful metadata",
		})
	}

	// -----------------------------------------------------------------------
	// Phase 2: Merge books with the same file_path
	// -----------------------------------------------------------------------
	pathGroups := make(map[string][]database.Book)
	for i := range allBooks {
		book := &allBooks[i]
		if deletedIDs[book.ID] || book.FilePath == "" {
			continue
		}
		pathGroups[book.FilePath] = append(pathGroups[book.FilePath], *book)
	}

	var phase2Details []dedupMergeDetail

	for fp, group := range pathGroups {
		if len(group) < 2 {
			continue
		}

		// Filter out already-deleted.
		live := filterLive(group, deletedIDs)
		if len(live) < 2 {
			continue
		}

		keepIdx := pickKeeperIdx(live)
		keeper := &live[keepIdx]
		var dups []*database.Book
		for i := range live {
			if i != keepIdx {
				dups = append(dups, &live[i])
			}
		}

		detail := dedupMergeDetail{
			KeeperID:    keeper.ID,
			KeeperTitle: keeper.Title,
			Reason:      fmt.Sprintf("same file_path: %s", fp),
		}

		var mergeErrs []string
		for _, dup := range dups {
			if mergeErr := mergeDuplicateBook(store, keeper, dup, dryRun, s.writeBackBatcher); mergeErr != nil {
				msg := fmt.Sprintf("phase2 merge %s->%s: %v", dup.ID, keeper.ID, mergeErr)
				errorMessages = append(errorMessages, msg)
				mergeErrs = append(mergeErrs, mergeErr.Error())
				result.Errors++
				continue
			}
			detail.RemovedIDs = append(detail.RemovedIDs, dup.ID)
			deletedIDs[dup.ID] = true
			result.Phase2PathDupesMerged++
		}
		if len(mergeErrs) > 0 {
			detail.Reason += " [errors: " + strings.Join(mergeErrs, "; ") + "]"
		}
		if len(detail.RemovedIDs) > 0 {
			phase2Details = append(phase2Details, detail)
		}
	}

	// -----------------------------------------------------------------------
	// Phase 3: Merge books with same normalised title + author in same dir
	// -----------------------------------------------------------------------
	type titleAuthorKey struct {
		NormTitle string
		AuthorID  int // 0 if nil
		Dir       string
	}

	taGroups := make(map[titleAuthorKey][]database.Book)
	for i := range allBooks {
		book := &allBooks[i]
		if deletedIDs[book.ID] {
			continue
		}
		normTitle := normalizeDedupTitle(book.Title)
		if normTitle == "" {
			continue
		}
		authorID := 0
		if book.AuthorID != nil {
			authorID = *book.AuthorID
		}
		// Only group books in the same directory (or with empty path).
		dir := ""
		if book.FilePath != "" {
			dir = filepath.Dir(book.FilePath)
		}
		key := titleAuthorKey{NormTitle: normTitle, AuthorID: authorID, Dir: dir}
		taGroups[key] = append(taGroups[key], *book)
	}

	var phase3Details []dedupMergeDetail

	for key, group := range taGroups {
		if len(group) < 2 {
			continue
		}
		live := filterLive(group, deletedIDs)
		if len(live) < 2 {
			continue
		}
		// Skip groups where author is unknown (authorID==0) and there are
		// different actual titles — could be false positives.
		if key.AuthorID == 0 {
			titles := make(map[string]bool)
			for _, b := range live {
				titles[strings.ToLower(strings.TrimSpace(b.Title))] = true
			}
			if len(titles) > 1 {
				continue // Different titles, skip
			}
		}

		keepIdx := pickKeeperIdx(live)
		keeper := &live[keepIdx]
		var dups []*database.Book
		for i := range live {
			if i != keepIdx {
				dups = append(dups, &live[i])
			}
		}

		detail := dedupMergeDetail{
			KeeperID:    keeper.ID,
			KeeperTitle: keeper.Title,
			Reason:      fmt.Sprintf("same title+author dir=%q normTitle=%q", key.Dir, key.NormTitle),
		}

		for _, dup := range dups {
			if mergeErr := mergeDuplicateBook(store, keeper, dup, dryRun, s.writeBackBatcher); mergeErr != nil {
				msg := fmt.Sprintf("phase3 merge %s->%s: %v", dup.ID, keeper.ID, mergeErr)
				errorMessages = append(errorMessages, msg)
				result.Errors++
				continue
			}
			detail.RemovedIDs = append(detail.RemovedIDs, dup.ID)
			deletedIDs[dup.ID] = true
			result.Phase3TitleDupesMerged++
		}
		if len(detail.RemovedIDs) > 0 {
			phase3Details = append(phase3Details, detail)
		}
	}

	// -----------------------------------------------------------------------
	// Phase 4: Clean up version group duplicate entries
	// -----------------------------------------------------------------------
	// Build a map: versionGroupID → []Book
	vgGroups := make(map[string][]database.Book)
	for i := range allBooks {
		book := &allBooks[i]
		if deletedIDs[book.ID] || book.VersionGroupID == nil || *book.VersionGroupID == "" {
			continue
		}
		vgGroups[*book.VersionGroupID] = append(vgGroups[*book.VersionGroupID], *book)
	}

	var phase4Details []dedupMergeDetail

	for vgID, group := range vgGroups {
		// Deduplicate by book ID within the group (the same book ID appearing
		// multiple times in a version group).
		seen := make(map[string]bool)
		var dupeIDs []string
		for _, b := range group {
			if seen[b.ID] {
				dupeIDs = append(dupeIDs, b.ID)
			}
			seen[b.ID] = true
		}
		if len(dupeIDs) == 0 {
			continue
		}

		detail := dedupMergeDetail{
			KeeperID:    "",
			KeeperTitle: "",
			RemovedIDs:  dupeIDs,
			Reason:      fmt.Sprintf("duplicate entries in version group %s", vgID),
		}

		if !dryRun {
			// Unlink duplicate version group entries by nulling the VersionGroupID
			// on the extra copies after keeping one.
			for _, dupID := range dupeIDs {
				current, gbErr := store.GetBookByID(dupID)
				if gbErr != nil || current == nil {
					continue
				}
				current.VersionGroupID = nil
				current.IsPrimaryVersion = nil
				if _, upErr := store.UpdateBook(dupID, current); upErr != nil {
					msg := fmt.Sprintf("phase4 unlink vg %s from book %s: %v", vgID, dupID, upErr)
					errorMessages = append(errorMessages, msg)
					result.Errors++
					continue
				}
				result.Phase4VGDupesCleaned++
			}
		} else {
			result.Phase4VGDupesCleaned += len(dupeIDs)
		}

		phase4Details = append(phase4Details, detail)
	}

	result.TotalBooksRemoved = result.Phase1JunkDeleted + result.Phase2PathDupesMerged + result.Phase3TitleDupesMerged
	result.ErrorMessages = errorMessages
	result.Details = gin.H{
		"phase1_junk":        phase1Details,
		"phase2_path_dupes":  phase2Details,
		"phase3_title_dupes": phase3Details,
		"phase4_vg_dupes":    phase4Details,
	}

	c.JSON(http.StatusOK, result)
}

// fetchAllBooksPaginated retrieves all books in pages of 500 to avoid
// loading 12K+ records in one shot.
func fetchAllBooksPaginated(store maintenanceStore) ([]database.Book, error) {
	const pageSize = 500
	var all []database.Book
	offset := 0
	for {
		page, err := store.GetAllBooks(pageSize, offset)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < pageSize {
			break
		}
		offset += pageSize
	}
	return all, nil
}

// isJunkReadByNarrator returns true if the book title is literally
// "read by narrator" (or a close variant) AND the book has no useful
// metadata that would justify keeping it.
func isJunkReadByNarrator(book *database.Book) bool {
	t := strings.ToLower(strings.TrimSpace(book.Title))
	if t != "read by narrator" {
		return false
	}
	// Has useful data — don't delete.
	if book.AuthorID != nil {
		return false
	}
	if book.SeriesID != nil {
		return false
	}
	if book.Description != nil && strings.TrimSpace(*book.Description) != "" {
		return false
	}
	if book.ISBN10 != nil || book.ISBN13 != nil || book.ASIN != nil {
		return false
	}
	if book.ITunesPersistentID != nil {
		return false
	}
	return true
}

// pickKeeperIdx returns the index of the "best" book to keep from a group.
// Priority:
//  1. Book with the most non-nil pointer fields (most metadata)
//  2. Prefer the one with author_id set
//  3. Prefer the oldest created_at
func pickKeeperIdx(books []database.Book) int {
	best := 0
	for i := 1; i < len(books); i++ {
		if bookScore(&books[i]) > bookScore(&books[best]) {
			best = i
		}
	}
	return best
}

// bookScore returns a comparable quality score for a Book.
// Higher is better / more complete.
func bookScore(b *database.Book) int {
	score := 0
	if b.AuthorID != nil {
		score += 100
	}
	if b.SeriesID != nil {
		score += 20
	}
	if b.Description != nil && *b.Description != "" {
		score += 10
	}
	if b.Narrator != nil && *b.Narrator != "" {
		score += 5
	}
	if b.Duration != nil {
		score += 5
	}
	if b.ISBN10 != nil || b.ISBN13 != nil || b.ASIN != nil {
		score += 10
	}
	if b.ITunesPersistentID != nil {
		score += 10
	}
	if b.Publisher != nil && *b.Publisher != "" {
		score += 3
	}
	if b.Language != nil && *b.Language != "" {
		score += 2
	}
	if b.Genre != nil && *b.Genre != "" {
		score += 2
	}
	if b.CoverURL != nil && *b.CoverURL != "" {
		score += 3
	}
	// Older record is likely the authoritative one.
	if b.CreatedAt != nil {
		// Earlier creation time → higher score (subtract millis since epoch / big divisor)
		score -= int(b.CreatedAt.Unix() / 1_000_000)
	}
	return score
}

// mergeDuplicateBook transfers data from dup into keeper and then soft-deletes dup.
// When dryRun is true the function returns nil without modifying the database.
func mergeDuplicateBook(store maintenanceStore, keeper *database.Book, dup *database.Book, dryRun bool, batcher Enqueuer) error {
	if dryRun {
		return nil
	}

	// Collect dup's iTunes PIDs before reassignment (for ITL removal).
	dupMappings, _ := store.GetExternalIDsForBook(dup.ID)
	var dupPIDs []string
	for _, m := range dupMappings {
		if m.Source == "itunes" && m.ExternalID != "" && !m.Tombstoned {
			dupPIDs = append(dupPIDs, m.ExternalID)
		}
	}

	// Transfer book_files rows.
	files, err := store.GetBookFiles(dup.ID)
	if err == nil {
		for i := range files {
			f := &files[i]
			f.BookID = keeper.ID
			if upErr := store.UpsertBookFile(f); upErr != nil {
				log.Printf("[WARN] dedup-books: UpsertBookFile %s -> keeper %s: %v", f.ID, keeper.ID, upErr)
			}
		}
	}

	// Transfer external ID mappings.
	if reassignErr := store.ReassignExternalIDs(dup.ID, keeper.ID); reassignErr != nil {
		log.Printf("[WARN] dedup-books: ReassignExternalIDs %s -> %s: %v", dup.ID, keeper.ID, reassignErr)
	}

	// Queue ITL removal for the dup's tracks (they now point to the wrong file).
	// The keeper's tracks remain; dup's tracks are redundant entries in iTunes.
	if batcher != nil && len(dupPIDs) > 0 {
		for _, pid := range dupPIDs {
			batcher.EnqueueRemove(pid)
		}
		log.Printf("[INFO] dedup-books: queued %d ITL removals for dup %s", len(dupPIDs), dup.ID)
	}

	// Transfer user tags.
	tags, tagsErr := store.GetBookUserTags(dup.ID)
	if tagsErr == nil && len(tags) > 0 {
		for _, tag := range tags {
			_ = store.AddBookUserTag(keeper.ID, tag)
		}
	}

	// Merge missing metadata fields into keeper.
	current, gbErr := store.GetBookByID(keeper.ID)
	if gbErr != nil {
		return fmt.Errorf("GetBookByID keeper %s: %w", keeper.ID, gbErr)
	}
	if current == nil {
		return fmt.Errorf("keeper book %s not found", keeper.ID)
	}

	mergeBookFields(current, dup)

	if _, upErr := store.UpdateBook(keeper.ID, current); upErr != nil {
		return fmt.Errorf("UpdateBook keeper %s: %w", keeper.ID, upErr)
	}

	// Soft-delete the duplicate.
	return softDeleteBook(store, dup.ID)
}

// mergeBookFields copies non-nil/non-empty fields from src into dst when
// dst's field is currently nil/empty.  Does not overwrite existing data.
func mergeBookFields(dst, src *database.Book) {
	if dst.AuthorID == nil && src.AuthorID != nil {
		dst.AuthorID = src.AuthorID
	}
	if dst.SeriesID == nil && src.SeriesID != nil {
		dst.SeriesID = src.SeriesID
		if dst.SeriesSequence == nil && src.SeriesSequence != nil {
			dst.SeriesSequence = src.SeriesSequence
		}
	}
	if dst.Narrator == nil && src.Narrator != nil && *src.Narrator != "" {
		dst.Narrator = src.Narrator
	}
	if dst.Description == nil && src.Description != nil && *src.Description != "" {
		dst.Description = src.Description
	}
	if dst.Duration == nil && src.Duration != nil {
		dst.Duration = src.Duration
	}
	if dst.Publisher == nil && src.Publisher != nil {
		dst.Publisher = src.Publisher
	}
	if dst.Language == nil && src.Language != nil {
		dst.Language = src.Language
	}
	if dst.Genre == nil && src.Genre != nil {
		dst.Genre = src.Genre
	}
	if dst.ISBN10 == nil && src.ISBN10 != nil {
		dst.ISBN10 = src.ISBN10
	}
	if dst.ISBN13 == nil && src.ISBN13 != nil {
		dst.ISBN13 = src.ISBN13
	}
	if dst.ASIN == nil && src.ASIN != nil {
		dst.ASIN = src.ASIN
	}
	if dst.ITunesPersistentID == nil && src.ITunesPersistentID != nil {
		dst.ITunesPersistentID = src.ITunesPersistentID
	}
	if dst.ITunesDateAdded == nil && src.ITunesDateAdded != nil {
		dst.ITunesDateAdded = src.ITunesDateAdded
	}
	if dst.ITunesPlayCount == nil && src.ITunesPlayCount != nil {
		dst.ITunesPlayCount = src.ITunesPlayCount
	}
	if dst.ITunesRating == nil && src.ITunesRating != nil {
		dst.ITunesRating = src.ITunesRating
	}
	if dst.ITunesBookmark == nil && src.ITunesBookmark != nil {
		dst.ITunesBookmark = src.ITunesBookmark
	}
	if dst.CoverURL == nil && src.CoverURL != nil {
		dst.CoverURL = src.CoverURL
	}
	if dst.OpenLibraryID == nil && src.OpenLibraryID != nil {
		dst.OpenLibraryID = src.OpenLibraryID
	}
	if dst.GoogleBooksID == nil && src.GoogleBooksID != nil {
		dst.GoogleBooksID = src.GoogleBooksID
	}
	if dst.HardcoverID == nil && src.HardcoverID != nil {
		dst.HardcoverID = src.HardcoverID
	}
	if dst.WorkID == nil && src.WorkID != nil {
		dst.WorkID = src.WorkID
	}
	if (dst.VersionGroupID == nil || *dst.VersionGroupID == "") && src.VersionGroupID != nil && *src.VersionGroupID != "" {
		dst.VersionGroupID = src.VersionGroupID
	}
}

// softDeleteBook marks a book as deleted using the MarkedForDeletion flag.
// If UpdateBook fails, falls back to hard-delete via DeleteBook.
func softDeleteBook(store maintenanceStore, bookID string) error {
	current, err := store.GetBookByID(bookID)
	if err != nil {
		return fmt.Errorf("GetBookByID %s: %w", bookID, err)
	}
	if current == nil {
		return nil // Already gone
	}

	t := true
	now := time.Now()
	current.MarkedForDeletion = &t
	current.MarkedForDeletionAt = &now

	if _, upErr := store.UpdateBook(bookID, current); upErr != nil {
		// Fall back to hard delete.
		log.Printf("[WARN] dedup-books: soft-delete failed for %s (%v), falling back to hard delete", bookID, upErr)
		return store.DeleteBook(bookID)
	}
	return nil
}

// normalizeDedupTitle produces a canonical key for title-based duplicate
// detection:
//   - lowercase + trim
//   - strip "(Unabridged)" suffix
//   - strip leading track/number patterns like "(12/85)" or "12."
//   - remove punctuation
//   - collapse whitespace
func normalizeDedupTitle(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	if s == "" {
		return ""
	}

	// Strip "(Unabridged)" anywhere
	s = strings.ReplaceAll(s, "(unabridged)", "")

	// Strip leading numeric patterns: "(12/85) " or "12. " or "12 - "
	reLeadNum := regexp.MustCompile(`^\s*(\(\d+[/\-]\d+\)|\d+[\.\-\s])\s*`)
	s = reLeadNum.ReplaceAllString(s, "")

	// Remove punctuation (keep letters, digits, spaces)
	s = nonAlphanumRE.ReplaceAllString(s, " ")

	// Collapse whitespace
	fields := strings.FieldsFunc(s, unicode.IsSpace)
	return strings.Join(fields, " ")
}

// filterLive filters out books whose IDs are in the deletedIDs set.
func filterLive(books []database.Book, deletedIDs map[string]bool) []database.Book {
	out := books[:0:len(books)]
	for _, b := range books {
		if !deletedIDs[b.ID] {
			out = append(out, b)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Refetch missing authors
// ---------------------------------------------------------------------------

// refetchMissingAuthorsResult describes one book that was examined/fixed.
type refetchMissingAuthorsResult struct {
	BookID       string `json:"book_id"`
	BookTitle    string `json:"book_title"`
	FilePath     string `json:"file_path,omitempty"`
	AuthorFound  string `json:"author_found,omitempty"`
	AuthorSource string `json:"author_source,omitempty"` // e.g. "tag.AlbumArtist (album_artist)", "tag.Artist"
	AuthorID     *int   `json:"author_id,omitempty"`
	Applied      bool   `json:"applied"`
	Skipped      bool   `json:"skipped"`
	SkipReason   string `json:"skip_reason,omitempty"`
	Error        string `json:"error,omitempty"`
}

// handleRefetchMissingAuthors queries books with a NULL author_id and attempts
// to resolve the author by re-reading audio file tags (album_artist > artist).
//
// Query params:
//   - dry_run=true  (default) — report what would change without writing
//   - dry_run=false — actually update the database
func (s *Server) handleRefetchMissingAuthors(c *gin.Context) {
	dryRun := c.DefaultQuery("dry_run", "true") != "false"

	store := s.Store()
	if store == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}

	const batchSize = 500
	offset := 0
	var results []refetchMissingAuthorsResult
	resolvedCount := 0
	skippedCount := 0
	errorCount := 0

	for {
		batch, err := store.GetAllBooks(batchSize, offset)
		if err != nil {
			internalError(c, "failed to list books", err)
			return
		}
		if len(batch) == 0 {
			break
		}

		for i := range batch {
			book := &batch[i]

			// Only consider books with no author and a non-empty title that
			// isn't itself a "read by narrator" leftover.
			if book.AuthorID != nil {
				continue
			}
			if book.Title == "" {
				continue
			}
			if strings.HasPrefix(strings.ToLower(book.Title), "read by ") {
				continue
			}

			result := refetchMissingAuthorsResult{
				BookID:    book.ID,
				BookTitle: book.Title,
				FilePath:  book.FilePath,
			}

			// Determine which file path to read tags from.
			// Prefer the book's own file_path; fall back to the first book_file.
			tagPath := book.FilePath
			if tagPath == "" {
				files, fErr := store.GetBookFiles(book.ID)
				if fErr == nil && len(files) > 0 {
					tagPath = files[0].FilePath
				}
			}

			if tagPath == "" {
				result.Skipped = true
				result.SkipReason = "no file path available"
				skippedCount++
				results = append(results, result)
				continue
			}

			// Extract tags from the audio file.
			meta, mErr := metadata.ExtractMetadata(tagPath, nil)
			if mErr != nil {
				result.Error = fmt.Sprintf("ExtractMetadata: %v", mErr)
				errorCount++
				results = append(results, result)
				continue
			}

			// Resolve the narrator name for this book (used to skip the artist
			// field when it clearly holds the narrator, not the author).
			narratorName := ""
			if book.Narrator != nil {
				narratorName = strings.ToLower(strings.TrimSpace(*book.Narrator))
			}
			if narratorName == "" && meta.Narrator != "" {
				narratorName = strings.ToLower(strings.TrimSpace(meta.Narrator))
			}

			// Apply tag priority: album_artist > artist (skip artist if it
			// matches the known narrator).
			// meta.Artist is already resolved from album_artist > artist > composer
			// by ExtractMetadata. We trust album_artist unconditionally; for
			// artist-only sources we check it doesn't equal the narrator.
			candidateAuthor := strings.TrimSpace(meta.Artist)
			if candidateAuthor == "" {
				result.Skipped = true
				result.SkipReason = "no author found in file tags"
				skippedCount++
				results = append(results, result)
				continue
			}

			// If the resolved author comes from the plain artist tag (not
			// album_artist) and it matches the narrator, skip it.
			lc := strings.ToLower(candidateAuthor)
			if narratorName != "" && lc == narratorName {
				// Only skip when the source was artist (not album_artist).
				// meta.AuthorSource contains the tag source string.
				if !strings.Contains(meta.AuthorSource, "album_artist") {
					result.Skipped = true
					result.SkipReason = "artist tag matches narrator; cannot determine author"
					skippedCount++
					results = append(results, result)
					continue
				}
			}

			normalizedName := dedup.NormalizeAuthorName(candidateAuthor)
			if normalizedName == "" {
				result.Skipped = true
				result.SkipReason = "normalized author name is empty"
				skippedCount++
				results = append(results, result)
				continue
			}

			result.AuthorFound = normalizedName
			result.AuthorSource = meta.AuthorSource

			if dryRun {
				// In dry-run mode, look up (but don't create) the author so
				// the response shows whether they already exist.
				existing, _ := store.GetAuthorByName(normalizedName)
				if existing != nil {
					result.AuthorID = &existing.ID
				}
				resolvedCount++
				results = append(results, result)
				continue
			}

			// Look up or create the author.
			author, lookupErr := store.GetAuthorByName(normalizedName)
			if lookupErr != nil {
				author, lookupErr = store.CreateAuthor(normalizedName)
				if lookupErr != nil {
					result.Error = fmt.Sprintf("CreateAuthor: %v", lookupErr)
					errorCount++
					results = append(results, result)
					continue
				}
			}
			if author == nil {
				result.Error = "author lookup returned nil"
				errorCount++
				results = append(results, result)
				continue
			}

			// Re-fetch book to avoid stale data (UpdateBook does full column replacement).
			current, getErr := store.GetBookByID(book.ID)
			if getErr != nil || current == nil {
				result.Error = fmt.Sprintf("GetBookByID: %v", getErr)
				errorCount++
				results = append(results, result)
				continue
			}

			current.AuthorID = &author.ID
			if _, updateErr := store.UpdateBook(book.ID, current); updateErr != nil {
				result.Error = updateErr.Error()
				errorCount++
				log.Printf("[WARN] refetch-missing-authors: failed to update book %s: %v", book.ID, updateErr)
			} else {
				result.AuthorID = &author.ID
				result.Applied = true
				resolvedCount++
				log.Printf("[INFO] refetch-missing-authors: set author %q (id=%d) on book %s (%q)",
					normalizedName, author.ID, book.ID, book.Title)
			}

			results = append(results, result)
		}

		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}

	c.JSON(http.StatusOK, gin.H{
		"dry_run":        dryRun,
		"total_examined": len(results),
		"resolved":       resolvedCount,
		"skipped":        skippedCount,
		"errors":         errorCount,
		"results":        results,
	})
}
