// file: internal/server/server_title_helpers.go
// version: 1.0.0
// guid: b4b0048c-d778-43c9-871e-21f9a9b6705d
// last-edited: 2026-05-01

package server

import (
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func computeSeriesPrunePreview(store database.Store) (*seriesPrunePreviewResult, error) {
	allSeries, err := store.GetAllSeries()
	if err != nil {
		return nil, fmt.Errorf("failed to get series: %w", err)
	}

	// Group by LOWER(TRIM(name)) + author_id
	type groupKey struct {
		name     string
		authorID int // 0 means nil
	}
	groups := make(map[groupKey][]database.Series)
	for _, s := range allSeries {
		aid := 0
		if s.AuthorID != nil {
			aid = *s.AuthorID
		}
		key := groupKey{name: strings.ToLower(strings.TrimSpace(s.Name)), authorID: aid}
		groups[key] = append(groups[key], s)
	}

	result := &seriesPrunePreviewResult{}

	// Find duplicate groups (>1 entry with same normalized name + author_id)
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}

		// Pick canonical: most books attached, then lowest ID
		canonicalIdx := 0
		canonicalBookCount := 0
		for i, s := range group {
			books, err := store.GetBooksBySeriesID(s.ID)
			if err != nil {
				continue
			}
			bc := len(books)
			if bc > canonicalBookCount || (bc == canonicalBookCount && s.ID < group[canonicalIdx].ID) {
				canonicalIdx = i
				canonicalBookCount = bc
			}
		}

		var mergeIDs []int
		totalBooks := 0
		for i, s := range group {
			if i == canonicalIdx {
				continue
			}
			mergeIDs = append(mergeIDs, s.ID)
			books, _ := store.GetBooksBySeriesID(s.ID)
			totalBooks += len(books)
		}
		books, _ := store.GetBooksBySeriesID(group[canonicalIdx].ID)
		totalBooks += len(books)

		result.Groups = append(result.Groups, seriesPrunePreviewGroup{
			Name:        group[canonicalIdx].Name,
			CanonicalID: group[canonicalIdx].ID,
			MergeIDs:    mergeIDs,
			BookCount:   totalBooks,
			Type:        "duplicate",
		})
		result.DuplicateCount += len(mergeIDs)
	}

	// Find orphan series with 0 books
	for _, s := range allSeries {
		books, err := store.GetBooksBySeriesID(s.ID)
		if err != nil {
			continue
		}
		if len(books) == 0 {
			result.Groups = append(result.Groups, seriesPrunePreviewGroup{
				Name:        s.Name,
				CanonicalID: s.ID,
				MergeIDs:    nil,
				BookCount:   0,
				Type:        "orphan",
			})
			result.OrphanCount++
		}
	}

	result.TotalCount = result.DuplicateCount + result.OrphanCount
	return result, nil
}

func stripChapterFromTitle(title string) string {
	cleaned := title

	// Strip leading track/disc number prefixes from filenames
	// e.g. "01 - Title", "01. Title", "1 - Title", "123 - Title"
	trackNumPrefix := regexp.MustCompile(`^\d{1,3}\s*[-–.]\s*`)
	cleaned = trackNumPrefix.ReplaceAllString(cleaned, "")
	// e.g. "01 Title" (bare number prefix followed by non-numeric text)
	bareNumPrefix := regexp.MustCompile(`^\d{1,3}\s+`)
	if stripped := strings.TrimSpace(bareNumPrefix.ReplaceAllString(cleaned, "")); stripped != "" {
		cleaned = stripped
	}
	// e.g. "Track 01 - Title", "Track01 - Title"
	trackWordPrefix := regexp.MustCompile(`(?i)^[Tt]rack\s*\d+\s*[-–.]\s*`)
	cleaned = trackWordPrefix.ReplaceAllString(cleaned, "")
	// e.g. "Disc 1 - Title", "Disc01 - Title"
	discWordPrefix := regexp.MustCompile(`(?i)^[Dd]is[ck]\s*\d+\s*[-–.]\s*`)
	cleaned = discWordPrefix.ReplaceAllString(cleaned, "")

	// Strip leading bracketed series info like "[The Expanse 9.0]" or "[Series Name]"
	bracketPrefix := regexp.MustCompile(`^\[.*?\]\s*[-–]?\s*`)
	cleaned = bracketPrefix.ReplaceAllString(cleaned, "")

	// Strip trailing bracketed info like "Title [Unabridged]"
	bracketSuffix := regexp.MustCompile(`\s*\[.*?\]\s*$`)
	cleaned = bracketSuffix.ReplaceAllString(cleaned, "")

	// Common patterns for chapters/books/parts/volumes
	patterns := []string{
		`(?i)[,:\s]*-?\s*(?:Book|Chapter|Part|Volume|Vol\.?|Pt\.?)\s*\d+[\.\d]*\s*$`,
		`(?i)\s*\((?:Book|Chapter|Part|Volume)\s*\d+[\.\d]*\)`,
		`(?i)\s*#\d+[\.\d]*\s*$`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		cleaned = re.ReplaceAllString(cleaned, "")
	}

	// Strip audiobook qualifiers like "(Unabridged)", "(Abridged)", etc.
	qualifiers := regexp.MustCompile(`(?i)\s*\((un)?abridged\)`)
	cleaned = qualifiers.ReplaceAllString(cleaned, "")

	// Strip leading/trailing " - " artifacts from removals
	cleaned = strings.TrimLeft(cleaned, " -–")
	cleaned = strings.TrimRight(cleaned, " -–")
	cleaned = strings.TrimSpace(cleaned)

	// If stripping removed everything, return the original title
	if cleaned == "" {
		return strings.TrimSpace(title)
	}
	return cleaned
}

func stripSubtitle(title string) string {
	// Try colon separator first: "Title: Subtitle"
	if idx := strings.Index(title, ": "); idx > 0 {
		return strings.TrimSpace(title[:idx])
	}
	// Try dash separator: "Title - Subtitle"
	if idx := strings.Index(title, " - "); idx > 0 {
		return strings.TrimSpace(title[:idx])
	}
	// Try em-dash: "Title — Subtitle"
	if idx := strings.Index(title, " — "); idx > 0 {
		return strings.TrimSpace(title[:idx])
	}
	return title
}

func extractTitleFromSegmentFilename(filename string) string {
	// Strip extension
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Try to find title after " - " separator (common pattern)
	if idx := strings.Index(name, " - "); idx >= 0 {
		title := strings.TrimSpace(name[idx+3:])
		if title != "" {
			return title
		}
	}

	// Try after " – " (em dash)
	if idx := strings.Index(name, " – "); idx >= 0 {
		title := strings.TrimSpace(name[idx+len(" – "):])
		if title != "" {
			return title
		}
	}

	// Strip leading track numbers like "01 ", "01. "
	stripped := regexp.MustCompile(`^\d{1,3}[\s.\-]+`).ReplaceAllString(name, "")
	if stripped != "" {
		return strings.TrimSpace(stripped)
	}

	return name
}

func reassignExternalIDsForFiles(sourceBookID, targetBookID string, files []database.BookFile) {
	eidStore := asExternalIDStore(database.GetGlobalStore())
	if eidStore == nil {
		return
	}

	mappings, err := eidStore.GetExternalIDsForBook(sourceBookID)
	if err != nil || len(mappings) == 0 {
		return
	}

	// Build lookup sets from the moved files
	movedPaths := make(map[string]bool, len(files))
	movedPIDs := make(map[string]bool, len(files))
	for _, f := range files {
		if f.FilePath != "" {
			movedPaths[f.FilePath] = true
		}
		if f.ITunesPersistentID != "" {
			movedPIDs[f.ITunesPersistentID] = true
		}
	}

	// Collect only the mappings that belong to the moved files
	var toMove []database.ExternalIDMapping
	for _, m := range mappings {
		if (m.FilePath != "" && movedPaths[m.FilePath]) ||
			(m.ExternalID != "" && movedPIDs[m.ExternalID]) {
			toMove = append(toMove, m)
		}
	}
	if len(toMove) == 0 {
		return
	}

	// Reassign each mapping: delete old reverse key, update primary, add new reverse key
	for _, m := range toMove {
		oldReverseKey := fmt.Sprintf("ext_id:book:%s:%s:%s", sourceBookID, m.Source, m.ExternalID)
		_ = database.GetGlobalStore().DeleteRaw(oldReverseKey)

		m.BookID = targetBookID
		if createErr := eidStore.CreateExternalIDMapping(&m); createErr != nil {
			log.Printf("[WARN] reassignExternalIDsForFiles: failed to reassign %s:%s to %s: %v",
				m.Source, m.ExternalID, targetBookID, createErr)
		}
	}

	log.Printf("[INFO] reassigned %d external ID mapping(s) from book %s to %s",
		len(toMove), sourceBookID, targetBookID)
}
