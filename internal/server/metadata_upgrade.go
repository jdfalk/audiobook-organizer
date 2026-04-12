// file: internal/server/metadata_upgrade.go
// version: 1.0.0
// guid: 4a3b2c1d-0e9f-8a7b-6c5d-4e3f2a1b0c9d
//
// Background job that upgrades metadata from lower-quality sources
// (primarily Google Books) to richer ones (Hardcover, Audible/Audnexus)
// when a high-confidence match is available. Backlog 7.4.
//
// The upgrade targets books tagged with `metadata:source:google_books`
// (or any other source considered "lower quality"). For each candidate,
// the job re-runs the full metadata search pipeline against ALL
// configured sources. If the best result comes from a source OTHER
// than the current one and its confidence score exceeds a threshold,
// the upgrade is applied automatically.
//
// The job leverages the metadata fetch cache (PR #250) so re-fetches
// for already-queried sources are free. Only sources that returned
// empty on the initial fetch will actually hit the API.

package server

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// MetadataUpgradeService finds books with low-quality metadata
// sources and attempts to upgrade them to richer sources.
type MetadataUpgradeService struct {
	db      database.Store
	fetcher *MetadataFetchService
}

// NewMetadataUpgradeService creates an upgrade service. The fetcher
// provides the search + apply pipeline; the db provides the tag
// lookup for finding eligible books.
func NewMetadataUpgradeService(db database.Store, fetcher *MetadataFetchService) *MetadataUpgradeService {
	return &MetadataUpgradeService{db: db, fetcher: fetcher}
}

// lowQualitySources lists the metadata sources that are considered
// "lower quality" — books whose metadata came from these sources
// are candidates for upgrade. The tag namespace is
// metadata:source:<slug> (all lowercase, spaces → underscores).
var lowQualitySources = []string{
	"google_books",
	"wikipedia",
}

// UpgradeResult summarizes what the upgrade job did.
type UpgradeResult struct {
	Checked  int `json:"checked"`
	Upgraded int `json:"upgraded"`
	Skipped  int `json:"skipped"`
	Errors   int `json:"errors"`
}

// minUpgradeConfidence is the minimum score a non-current-source
// candidate must achieve to trigger an automatic metadata apply.
// Set conservatively high to avoid upgrading to a worse match.
const minUpgradeConfidence = 0.90

// RunUpgrade scans for books tagged with low-quality metadata
// sources and attempts to find a better match from other sources.
// Respects context cancellation so it can be run as a long-running
// operation with a kill switch.
func (s *MetadataUpgradeService) RunUpgrade(ctx context.Context, limit int) (*UpgradeResult, error) {
	if s.fetcher == nil {
		return nil, fmt.Errorf("metadata fetch service not configured")
	}
	if limit <= 0 {
		limit = 200
	}

	result := &UpgradeResult{}

	for _, sourceSlug := range lowQualitySources {
		tag := "metadata:source:" + sourceSlug
		bookIDs, err := s.db.GetBooksByTag(tag)
		if err != nil {
			log.Printf("[WARN] metadata-upgrade: GetBooksByTag(%s): %v", tag, err)
			continue
		}
		log.Printf("[INFO] metadata-upgrade: found %d books tagged %s", len(bookIDs), tag)

		for _, bookID := range bookIDs {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			if result.Checked >= limit {
				break
			}
			result.Checked++

			upgraded, upgradeErr := s.tryUpgradeBook(ctx, bookID, sourceSlug)
			if upgradeErr != nil {
				log.Printf("[WARN] metadata-upgrade: book %s: %v", bookID, upgradeErr)
				result.Errors++
				continue
			}
			if upgraded {
				result.Upgraded++
			} else {
				result.Skipped++
			}
		}
	}

	return result, nil
}

// tryUpgradeBook re-searches metadata for a single book and
// applies the best non-current-source result if it's confident
// enough. Returns true if an upgrade was applied.
func (s *MetadataUpgradeService) tryUpgradeBook(ctx context.Context, bookID, currentSourceSlug string) (bool, error) {
	book, err := s.db.GetBookByID(bookID)
	if err != nil || book == nil {
		return false, fmt.Errorf("book not found: %s", bookID)
	}

	// Run the full search pipeline — this goes through the
	// metadata fetch cache, so sources that were already queried
	// (and returned non-empty) won't hit the API again. Sources
	// that returned empty last time WILL be retried because the
	// cache only stores non-empty results.
	resp, err := s.fetcher.SearchMetadataForBook(bookID, book.Title)
	if err != nil {
		return false, fmt.Errorf("search failed: %w", err)
	}
	if resp == nil || len(resp.Results) == 0 {
		return false, nil // no results at all
	}

	// Find the best candidate from a source OTHER than the current one.
	var bestCandidate *MetadataCandidate
	for i := range resp.Results {
		c := &resp.Results[i]
		candidateSlug := strings.ToLower(strings.ReplaceAll(c.Source, " ", "_"))
		if strings.HasPrefix(candidateSlug, "audnexus") {
			candidateSlug = "audnexus"
		}
		// Skip candidates from the same source we're trying to upgrade FROM.
		if candidateSlug == currentSourceSlug {
			continue
		}
		if c.Score < minUpgradeConfidence {
			continue
		}
		if bestCandidate == nil || c.Score > bestCandidate.Score {
			bestCandidate = c
		}
	}

	if bestCandidate == nil {
		return false, nil // no better source found above threshold
	}

	// Apply the upgrade. ApplyMetadataCandidate handles:
	// - change history recording
	// - metadata field application
	// - provenance tagging (metadata:source:*, metadata:language:*)
	// - cache invalidation
	// - ISBN enrichment queueing
	// - file I/O queueing (cover embed, tag write, rename)
	_, applyErr := s.fetcher.ApplyMetadataCandidate(bookID, *bestCandidate, nil)
	if applyErr != nil {
		return false, fmt.Errorf("apply failed: %w", applyErr)
	}

	log.Printf("[INFO] metadata-upgrade: upgraded %s from %s → %s (score=%.2f, title=%q)",
		bookID, currentSourceSlug, bestCandidate.Source, bestCandidate.Score, bestCandidate.Title)
	return true, nil
}
