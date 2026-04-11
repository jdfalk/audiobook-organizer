// file: internal/server/dedup_engine.go
// version: 1.8.0
// guid: 8f3a1c6e-d472-4b9a-a5e1-7c2d9f0b3e84

package server

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// DedupEngine orchestrates a 3-layer dedup system:
//   - Layer 1: Exact matching (free, instant) — same file hash, ISBN/ASIN, or near-identical titles
//   - Layer 2: Embedding similarity (cheap, ~250ms) — cosine similarity of OpenAI embeddings
//   - Layer 3: LLM review (expensive, batch only) — for ambiguous candidates
type DedupEngine struct {
	embedStore   *database.EmbeddingStore
	bookStore    database.Store
	embedClient  *ai.EmbeddingClient
	llmParser    *ai.OpenAIParser
	mergeService *MergeService

	// Thresholds (read from config or set directly)
	BookHighThreshold   float64
	BookLowThreshold    float64
	AuthorHighThreshold float64
	AuthorLowThreshold  float64
	AutoMergeEnabled    bool

	// Layer 3 ambiguous zones — candidates whose similarity falls inside these
	// ranges (inclusive) are sent to the LLM during RunLLMReview.
	LLMBookLow    float64
	LLMBookHigh   float64
	LLMAuthorLow  float64
	LLMAuthorHigh float64

	// LLMMaxPairsPerRun caps how many pairs a single RunLLMReview invocation will
	// send to the LLM. Zero means "all pending ambiguous candidates".
	LLMMaxPairsPerRun int
}

// NewDedupEngine creates a DedupEngine with sensible defaults.
// llmParser may be nil if Layer 3 LLM review should be disabled.
func NewDedupEngine(
	embedStore *database.EmbeddingStore,
	bookStore database.Store,
	embedClient *ai.EmbeddingClient,
	llmParser *ai.OpenAIParser,
	mergeService *MergeService,
) *DedupEngine {
	return &DedupEngine{
		embedStore:          embedStore,
		bookStore:           bookStore,
		embedClient:         embedClient,
		llmParser:           llmParser,
		mergeService:        mergeService,
		BookHighThreshold:   0.95,
		BookLowThreshold:    0.85,
		AuthorHighThreshold: 0.92,
		AuthorLowThreshold:  0.80,
		AutoMergeEnabled:    false,
		LLMBookLow:          0.80,
		LLMBookHigh:         0.92,
		LLMAuthorLow:        0.75,
		LLMAuthorHigh:       0.85,
		LLMMaxPairsPerRun:   200,
	}
}

// CheckBook runs Layer 1 (exact) and Layer 2 (embedding) dedup checks for a book.
// Returns true if the book was auto-merged (Layer 1 only, when AutoMergeEnabled).
// Honors ctx cancellation so the dedup-on-import hook can bail immediately
// when the server is shutting down, rather than racing Pebble close.
func (de *DedupEngine) CheckBook(ctx context.Context, bookID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	book, err := de.bookStore.GetBookByID(bookID)
	if err != nil {
		return false, fmt.Errorf("get book %s: %w", bookID, err)
	}
	if book == nil {
		return false, fmt.Errorf("book %s not found", bookID)
	}

	// Resolve author name
	authorName := ""
	if book.AuthorID != nil {
		author, err := de.bookStore.GetAuthorByID(*book.AuthorID)
		if err == nil && author != nil {
			authorName = author.Name
		}
	}

	// --- Layer 1: Exact matching ---
	merged, err := de.checkExactFileHash(book, authorName)
	if err != nil {
		log.Printf("dedup: file hash check error for %s: %v", bookID, err)
	}
	if merged {
		return true, nil
	}

	if err := de.checkExactISBN(book); err != nil {
		log.Printf("dedup: ISBN check error for %s: %v", bookID, err)
	}

	if err := de.checkExactTitle(book, authorName); err != nil {
		log.Printf("dedup: title check error for %s: %v", bookID, err)
	}

	// --- Layer 2: Embedding similarity ---
	if de.embedClient != nil {
		if _, err := de.EmbedBook(ctx, bookID); err != nil {
			log.Printf("dedup: embed book error for %s: %v", bookID, err)
		} else {
			if err := de.findSimilarBooks(ctx, bookID); err != nil {
				log.Printf("dedup: similarity search error for %s: %v", bookID, err)
			}
		}
	}

	return false, nil
}

// checkExactFileHash checks if any other book shares a file hash.
// Auto-merges if hashes match AND same normalized author AND same normalized title.
func (de *DedupEngine) checkExactFileHash(book *database.Book, authorName string) (bool, error) {
	// Check book-level file hash
	if book.FileHash != nil && *book.FileHash != "" {
		other, err := de.bookStore.GetBookByFileHash(*book.FileHash)
		if err != nil {
			return false, err
		}
		if other != nil && other.ID != book.ID {
			return de.handleFileHashMatch(book, other, authorName)
		}
	}

	// Also check via book files
	files, err := de.bookStore.GetBookFiles(book.ID)
	if err != nil {
		return false, err
	}
	for _, f := range files {
		if f.FileHash == "" {
			continue
		}
		other, err := de.bookStore.GetBookByFileHash(f.FileHash)
		if err != nil {
			continue
		}
		if other != nil && other.ID != book.ID {
			merged, err := de.handleFileHashMatch(book, other, authorName)
			if err != nil {
				return false, err
			}
			if merged {
				return true, nil
			}
		}
	}
	return false, nil
}

// handleFileHashMatch decides whether to auto-merge or create a candidate for a file hash match.
func (de *DedupEngine) handleFileHashMatch(book, other *database.Book, authorName string) (bool, error) {
	otherAuthorName := ""
	if other.AuthorID != nil {
		otherAuthor, err := de.bookStore.GetAuthorByID(*other.AuthorID)
		if err == nil && otherAuthor != nil {
			otherAuthorName = otherAuthor.Name
		}
	}

	sameAuthor := NormalizeAuthorName(authorName) == NormalizeAuthorName(otherAuthorName)
	sameTitle := normalizeTitle(book.Title) == normalizeTitle(other.Title)

	if sameAuthor && sameTitle && de.AutoMergeEnabled && de.mergeService != nil {
		_, err := de.mergeService.MergeBooks([]string{book.ID, other.ID}, other.ID)
		if err != nil {
			return false, fmt.Errorf("auto-merge failed: %w", err)
		}
		log.Printf("dedup: auto-merged book %s into %s (file hash match)", book.ID, other.ID)
		return true, nil
	}

	// Create candidate even if we don't auto-merge
	sim := 1.0
	return false, de.embedStore.UpsertCandidate(database.DedupCandidate{
		EntityType: "book",
		EntityAID:  book.ID,
		EntityBID:  other.ID,
		Layer:      "exact",
		Similarity: &sim,
		Status:     "pending",
	})
}

// checkExactISBN scans all books for matching ISBN10, ISBN13, or ASIN.
func (de *DedupEngine) checkExactISBN(book *database.Book) error {
	bookISBN10 := derefStr(book.ISBN10)
	bookISBN13 := derefStr(book.ISBN13)
	bookASIN := derefStr(book.ASIN)

	if bookISBN10 == "" && bookISBN13 == "" && bookASIN == "" {
		return nil
	}

	const batchSize = 500
	offset := 0
	for {
		batch, err := de.bookStore.GetAllBooks(batchSize, offset)
		if err != nil {
			return fmt.Errorf("get all books at offset %d: %w", offset, err)
		}
		if len(batch) == 0 {
			break
		}

		for i := range batch {
			other := &batch[i]
			if other.ID == book.ID {
				continue
			}
			matched := false
			if bookISBN10 != "" && derefStr(other.ISBN10) == bookISBN10 {
				matched = true
			}
			if bookISBN13 != "" && derefStr(other.ISBN13) == bookISBN13 {
				matched = true
			}
			if bookASIN != "" && derefStr(other.ASIN) == bookASIN {
				matched = true
			}
			if matched {
				sim := 1.0
				if err := de.embedStore.UpsertCandidate(database.DedupCandidate{
					EntityType: "book",
					EntityAID:  book.ID,
					EntityBID:  other.ID,
					Layer:      "exact",
					Similarity: &sim,
					Status:     "pending",
				}); err != nil {
					log.Printf("dedup: upsert ISBN candidate error: %v", err)
				}
			}
		}

		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}
	return nil
}

// checkExactTitle checks all books by the same author for near-identical
// titles. Near-identical is defined as Levenshtein distance < 3 on the
// normalized titles, WITH a series-volume safety check: if both books
// carry a distinct series position (either on the Book.SeriesSequence
// field or extracted from the title string), they are rejected even when
// the raw Levenshtein falls under the threshold. Without this guard,
// numbered series volumes like "X 3: A LitRPG Adventure (X, Book 3)" vs
// "X 2: A LitRPG Adventure (X, Book 2)" match at distance 2 and get
// incorrectly flagged as exact duplicates.
//
// Books with empty or near-empty titles are also rejected here — a pair
// of empty strings has a Levenshtein distance of 0 and would otherwise
// match every other empty-titled book by the same author.
func (de *DedupEngine) checkExactTitle(book *database.Book, authorName string) error {
	if book.AuthorID == nil {
		return nil
	}
	if !hasUsableTitle(book.Title) {
		return nil
	}

	others, err := de.bookStore.GetBooksByAuthorID(*book.AuthorID)
	if err != nil {
		return fmt.Errorf("get books by author: %w", err)
	}

	normTitle := normalizeTitle(book.Title)
	bookSeriesNum := seriesNumberOf(book)
	for i := range others {
		other := &others[i]
		if other.ID == book.ID {
			continue
		}
		if !hasUsableTitle(other.Title) {
			continue
		}
		otherNormTitle := normalizeTitle(other.Title)
		dist := levenshteinDistance(normTitle, otherNormTitle)
		if dist >= 3 {
			continue
		}
		// Series-volume safety (primary): if both books identify as
		// distinct volumes of the same series via structured metadata or
		// an explicit "Book N" / "bk N" / "Vol N" / "#N" marker in the
		// title, reject the pair. Merging volume 3 into volume 2 would
		// silently destroy user content.
		otherSeriesNum := seriesNumberOf(other)
		if bookSeriesNum != "" && otherSeriesNum != "" && bookSeriesNum != otherSeriesNum {
			continue
		}
		// Series-volume safety (fallback): if the normalized titles differ
		// ONLY in digit characters (same non-digit structure, different
		// numbers) the pair is almost certainly two volumes of a series
		// whose volume marker the regex didn't catch. This is the last-
		// ditch guard for title patterns like "Series Name 3" with no
		// explicit "book"/"bk"/"vol" token. False positives here are
		// limited to two books whose titles genuinely differ only in a
		// number — rare enough that dismissing them manually is much
		// cheaper than accidentally merging a wrong volume.
		if titlesDifferOnlyInDigits(normTitle, otherNormTitle) {
			continue
		}
		sim := 1.0
		if err := de.embedStore.UpsertCandidate(database.DedupCandidate{
			EntityType: "book",
			EntityAID:  book.ID,
			EntityBID:  other.ID,
			Layer:      "exact",
			Similarity: &sim,
			Status:     "pending",
		}); err != nil {
			log.Printf("dedup: upsert title candidate error: %v", err)
		}
	}
	return nil
}

// hasUsableTitle reports whether a title string is meaningful enough to
// drive dedup decisions. Empty strings, whitespace-only strings, and
// extremely short strings (≤ 2 characters after trimming) are rejected
// — their embeddings collapse into a tiny region of the vector space
// where unrelated records spuriously hit 100% cosine similarity, and
// their Levenshtein distance against any other empty-ish title is 0.
func hasUsableTitle(title string) bool {
	trimmed := strings.TrimSpace(title)
	return len([]rune(trimmed)) > 2
}

// seriesNumberOf returns a stable string representation of a book's
// series position, if one can be determined. It prefers the structured
// Book.SeriesSequence field; if that's unset it falls back to extracting
// a trailing book-number token from the title (handling patterns like
// "Title 3", "Title, Book 3", "Title (Book 3)", "Title #3"). Returns ""
// if no position can be determined.
func seriesNumberOf(book *database.Book) string {
	if book.SeriesSequence != nil {
		return strconv.Itoa(*book.SeriesSequence)
	}
	return extractSeriesNumberFromTitle(book.Title)
}

// seriesNumberInTitleRe matches an explicit book-volume marker followed by
// a number. Recognized markers (case-insensitive, optional trailing dot):
//
//	book, bk, volume, vol, number, no, part, pt, episode, ep, #
//
// Examples that match: "Reclaiming Honor bk 6", "Title, Book 3",
// "Title Vol. 12", "Title #4", "Title Ep 7", "title bk.3" (no space).
// The capture group is the digit portion only.
var seriesNumberInTitleRe = regexp.MustCompile(
	`(?i)(?:book|bk|volume|vol|number|no|part|pt|episode|ep|#)\.?\s*(\d+(?:\.\d+)?)`,
)

// extractSeriesNumberFromTitle looks for an explicit volume marker token
// anywhere in the title and returns the matched number as a string.
// Returns "" if none found. It only matches explicit book-volume markers —
// a bare number in a title ("1984") is not treated as a series position.
//
// This is the safety net that stops Layer 1 from merging "Reclaiming
// Honor bk 6" into "Reclaiming Honor bk 7" just because the normalized
// titles differ by two characters and slip under the Levenshtein
// threshold.
func extractSeriesNumberFromTitle(title string) string {
	m := seriesNumberInTitleRe.FindStringSubmatch(title)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// titlesDifferOnlyInDigits reports whether two titles have identical
// structure after stripping all digit characters. When true, the two
// titles are near-certainly two volumes of a series where the volume
// marker token isn't one the regex recognizes — e.g. "Title 3" vs
// "Title 4", "Series Name Six" vs "Series Name Seven" (no, that one
// has letter differences and returns false), "Reclaiming Honor abc6"
// vs "Reclaiming Honor abc7".
//
// The function is intentionally strict: both titles must match exactly
// after digit removal AND at least one digit must be present in each.
// This avoids false rejections for unrelated books with matching
// non-digit content (e.g. two books titled "Untitled" — no digits, so
// the function returns false and the pair is allowed through).
func titlesDifferOnlyInDigits(a, b string) bool {
	// Normalize: replace each digit with a space, then collapse runs of
	// whitespace, then trim. Replacing (rather than dropping) digits
	// avoids "title12" and "title" producing different strippeds when
	// one is "title12sub" and the other is "title sub" — both collapse
	// to "title sub" the right way. Collapsing whitespace handles
	// "title 2" -> "title " -> "title" vs plain "title".
	normalize := func(s string) string {
		var sb strings.Builder
		for _, r := range s {
			if r >= '0' && r <= '9' {
				sb.WriteRune(' ')
				continue
			}
			sb.WriteRune(r)
		}
		return strings.Join(strings.Fields(sb.String()), " ")
	}
	normA := normalize(a)
	normB := normalize(b)
	if normA != normB {
		return false
	}
	// Non-digit content is identical (ignoring whitespace differences
	// where digits used to be). For this to count as a series-volume
	// diff, the digit strings must differ. If both digit strings are
	// identical, it's the same title (e.g. both "Foundation 1" — same
	// book, not a series difference). If one side has no digits and the
	// other has a digit, that's a "Backyard Dungeon" vs "Backyard Dungeon
	// 2" pattern where the first book is volume 1 and the second is
	// volume 2 — a real series-volume diff.
	digitsA := extractDigits(a)
	digitsB := extractDigits(b)
	if digitsA == digitsB {
		return false
	}
	// At least one side must have a digit; otherwise both digit strings
	// are "" and the == check above would have caught it.
	return true
}

// extractDigits returns all digit characters from s concatenated in
// order. "Book 3 part 2" → "32".
func extractDigits(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// findSimilarBooks runs Layer 2 embedding similarity search for a book.
func (de *DedupEngine) findSimilarBooks(ctx context.Context, bookID string) error {
	emb, err := de.embedStore.Get("book", bookID)
	if err != nil || emb == nil {
		return fmt.Errorf("no embedding for book %s", bookID)
	}

	// Load the query book once so we can consult its version_group_id, its
	// title, and its series position when filtering candidates. If the
	// lookup fails we proceed without the filter — skipping pairs is an
	// optimisation, not a correctness need, but a nil queryBook means we
	// must skip the version-group and series-volume checks below.
	queryBook, _ := de.bookStore.GetBookByID(bookID)

	// Guard against embeddings that should never have been created in the
	// first place. If the query book has an empty/near-empty title, its
	// embedding is noise and everything it matches will be garbage —
	// treat this as a no-op rather than creating candidates.
	if queryBook != nil && !hasUsableTitle(queryBook.Title) {
		return nil
	}

	querySeriesNum := ""
	if queryBook != nil {
		querySeriesNum = seriesNumberOf(queryBook)
	}

	results, err := de.embedStore.FindSimilar("book", emb.Vector, float32(de.BookLowThreshold), 20)
	if err != nil {
		return err
	}

	for _, r := range results {
		if r.EntityID == bookID {
			continue
		}
		otherBook, _ := de.bookStore.GetBookByID(r.EntityID)
		if otherBook == nil {
			// Other book no longer exists — skip the candidate.
			continue
		}
		// Drop candidates with no usable title on the other side. Their
		// embedding is noise (same reason as the query-side guard above).
		if !hasUsableTitle(otherBook.Title) {
			continue
		}
		// Drop candidates that are already siblings in the same version
		// group. The version-group system already knows these are the same
		// logical book in different formats, so surfacing them as dedup
		// candidates is just noise.
		if queryBook != nil && queryBook.VersionGroupID != nil && *queryBook.VersionGroupID != "" &&
			otherBook.VersionGroupID != nil &&
			*otherBook.VersionGroupID == *queryBook.VersionGroupID {
			continue
		}
		// Drop candidates that are distinct volumes of a numbered series.
		// Embeddings cannot distinguish "Book 3" from "Book 4" well because
		// the titles are 99% identical — we have to filter these out by
		// structured metadata. The explicit marker check is the primary
		// signal (Book.SeriesSequence or a "bk"/"book"/"vol"/"#" token in
		// the title); the digit-structure check is the fallback for titles
		// like "Series Name 3" that have no explicit marker.
		if querySeriesNum != "" {
			if otherSeriesNum := seriesNumberOf(otherBook); otherSeriesNum != "" && otherSeriesNum != querySeriesNum {
				continue
			}
		}
		if queryBook != nil && titlesDifferOnlyInDigits(normalizeTitle(queryBook.Title), normalizeTitle(otherBook.Title)) {
			continue
		}
		sim := float64(r.Similarity)
		if err := de.embedStore.UpsertCandidate(database.DedupCandidate{
			EntityType: "book",
			EntityAID:  bookID,
			EntityBID:  r.EntityID,
			Layer:      "embedding",
			Similarity: &sim,
			Status:     "pending",
		}); err != nil {
			log.Printf("dedup: upsert embedding candidate error: %v", err)
		}
	}
	return nil
}

// CheckAuthor runs Layer 2 embedding similarity for an author.
func (de *DedupEngine) CheckAuthor(ctx context.Context, authorID int) error {
	author, err := de.bookStore.GetAuthorByID(authorID)
	if err != nil {
		return fmt.Errorf("get author %d: %w", authorID, err)
	}
	if author == nil {
		return fmt.Errorf("author %d not found", authorID)
	}

	if de.embedClient != nil {
		if err := de.EmbedAuthor(ctx, authorID); err != nil {
			return fmt.Errorf("embed author: %w", err)
		}
	}

	entityID := strconv.Itoa(authorID)
	emb, err := de.embedStore.Get("author", entityID)
	if err != nil || emb == nil {
		return nil // no embedding, nothing to compare
	}

	results, err := de.embedStore.FindSimilar("author", emb.Vector, float32(de.AuthorLowThreshold), 20)
	if err != nil {
		return err
	}

	for _, r := range results {
		if r.EntityID == entityID {
			continue
		}
		sim := float64(r.Similarity)
		if err := de.embedStore.UpsertCandidate(database.DedupCandidate{
			EntityType: "author",
			EntityAID:  entityID,
			EntityBID:  r.EntityID,
			Layer:      "embedding",
			Similarity: &sim,
			Status:     "pending",
		}); err != nil {
			log.Printf("dedup: upsert author candidate error: %v", err)
		}
	}
	return nil
}

// EmbedStatus classifies the outcome of a single EmbedBook call so callers
// can count live API usage separately from no-op traversals. The log line
// in runEmbeddingBackfill used to say "N books embedded" for every
// successful return regardless of what actually happened, which made the
// backfill's real cost invisible. With this enum a caller can report:
//
//	Embedded N (cached M, skipped_non_primary P, skipped_empty_title Q)
//
// making it obvious at a glance whether a run actually called the
// embeddings API or just walked the library to validate state.
type EmbedStatus int

const (
	// EmbedStatusEmbedded means the embeddings API was called and the
	// resulting vector was written to the store. This is the only status
	// that costs money.
	EmbedStatusEmbedded EmbedStatus = iota

	// EmbedStatusCached means the book already had an embedding whose
	// text_hash matched the current title/author/narrator, so no API
	// call was made and no row was written. On re-runs of an unchanged
	// library almost every book lands here.
	EmbedStatusCached

	// EmbedStatusSkippedNonPrimary means the book is a non-primary
	// member of a version group (alternate format of another book).
	// Its identity is owned by the primary, so it gets no embedding.
	// Any stale row for the book is deleted on the way out.
	EmbedStatusSkippedNonPrimary

	// EmbedStatusSkippedEmptyTitle means the book has no usable title
	// (empty, whitespace-only, or ≤ 2 characters after trimming).
	// Embedding such a book would collapse into a dense cluster where
	// unrelated records spuriously match at ~100% cosine. Any stale row
	// for the book is deleted on the way out.
	EmbedStatusSkippedEmptyTitle
)

// String returns a short human-readable form of the status, suitable for
// log output.
func (s EmbedStatus) String() string {
	switch s {
	case EmbedStatusEmbedded:
		return "embedded"
	case EmbedStatusCached:
		return "cached"
	case EmbedStatusSkippedNonPrimary:
		return "skipped_non_primary"
	case EmbedStatusSkippedEmptyTitle:
		return "skipped_empty_title"
	default:
		return "unknown"
	}
}

// EmbedBook generates and stores an embedding for the given book.
// Returns a status classifying what actually happened so callers can
// distinguish live API calls from cache hits and skipped-by-policy books.
//
// Non-primary versions (members of a version group that are not the primary
// representative) are skipped entirely: their embedding would be a duplicate
// of the primary's by construction, and surfacing them as dedup candidates
// just clutters the UI with noise. Any existing embedding for a non-primary
// book is deleted on the spot so historical rows from earlier backfills get
// cleaned up as we walk the library. Empty-title books are skipped with
// the same cleanup behavior.
func (de *DedupEngine) EmbedBook(ctx context.Context, bookID string) (EmbedStatus, error) {
	if de.embedClient == nil {
		return 0, fmt.Errorf("no embedding client configured")
	}

	book, err := de.bookStore.GetBookByID(bookID)
	if err != nil {
		return 0, fmt.Errorf("get book %s: %w", bookID, err)
	}
	if book == nil {
		return 0, fmt.Errorf("book %s not found", bookID)
	}

	// Skip non-primary version-group members. If the book was previously
	// embedded (stale data from a pre-fix backfill), remove that row now.
	if book.IsPrimaryVersion != nil && !*book.IsPrimaryVersion {
		if err := de.embedStore.Delete("book", bookID); err != nil {
			log.Printf("dedup: delete stale embedding for non-primary %s: %v", bookID, err)
		}
		return EmbedStatusSkippedNonPrimary, nil
	}

	// Skip books without a usable title. Embedding a blank or near-empty
	// title produces a vector that lives in a dense cluster of other
	// empty-title vectors, matching them all at ~100% cosine — the exact
	// false-positive pattern the user saw in prod. We also delete any
	// stale embedding for the book in case a pre-fix backfill had stored
	// one, so the next similarity scan is clean.
	if !hasUsableTitle(book.Title) {
		if err := de.embedStore.Delete("book", bookID); err != nil {
			log.Printf("dedup: delete stale embedding for empty-title %s: %v", bookID, err)
		}
		return EmbedStatusSkippedEmptyTitle, nil
	}

	authorName := ""
	if book.AuthorID != nil {
		author, err := de.bookStore.GetAuthorByID(*book.AuthorID)
		if err == nil && author != nil {
			authorName = author.Name
		}
	}

	text := ai.BuildEmbeddingText("book", book.Title, authorName, derefStr(book.Narrator))
	hash := ai.TextHash(text)

	// Check if existing embedding already has this hash — skip if so.
	existing, err := de.embedStore.Get("book", bookID)
	if err == nil && existing != nil && existing.TextHash == hash {
		return EmbedStatusCached, nil
	}

	vec, err := de.embedClient.EmbedOne(ctx, text)
	if err != nil {
		return 0, fmt.Errorf("embed text: %w", err)
	}

	if err := de.embedStore.Upsert(database.Embedding{
		EntityType: "book",
		EntityID:   bookID,
		TextHash:   hash,
		Vector:     vec,
		Model:      "text-embedding-3-large",
	}); err != nil {
		return 0, err
	}
	return EmbedStatusEmbedded, nil
}

// EmbedAuthor generates and stores an embedding for the given author.
func (de *DedupEngine) EmbedAuthor(ctx context.Context, authorID int) error {
	if de.embedClient == nil {
		return fmt.Errorf("no embedding client configured")
	}

	author, err := de.bookStore.GetAuthorByID(authorID)
	if err != nil {
		return fmt.Errorf("get author %d: %w", authorID, err)
	}
	if author == nil {
		return fmt.Errorf("author %d not found", authorID)
	}

	text := ai.BuildEmbeddingText("author", author.Name, "", "")
	hash := ai.TextHash(text)
	entityID := strconv.Itoa(authorID)

	existing, err := de.embedStore.Get("author", entityID)
	if err == nil && existing != nil && existing.TextHash == hash {
		return nil
	}

	vec, err := de.embedClient.EmbedOne(ctx, text)
	if err != nil {
		return fmt.Errorf("embed text: %w", err)
	}

	return de.embedStore.Upsert(database.Embedding{
		EntityType: "author",
		EntityID:   entityID,
		TextHash:   hash,
		Vector:     vec,
		Model:      "text-embedding-3-large",
	})
}

// FullScan re-embeds stale entities and runs both Layer 1 (exact) and
// Layer 2 (embedding) dedup checks for every primary book in the library.
// The progress callback is invoked periodically with (done, total).
//
// Layer 1 used to only run on ingest and metadata-apply events, which meant
// the `exact` bucket stayed at zero for libraries that hadn't seen new books
// since the initial backfill. Running it inside FullScan populates the
// bucket with the hash/ISBN/near-title-match candidates that were always
// there but never surfaced.
func (de *DedupEngine) FullScan(ctx context.Context, progress func(done, total int)) error {
	books, err := de.getAllBooks()
	if err != nil {
		return fmt.Errorf("get all books: %w", err)
	}

	total := len(books)
	for i, book := range books {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Resolve author name once — reused by both Layer 1 title check
		// and (indirectly) by Layer 2 via EmbedBook.
		authorName := ""
		if book.AuthorID != nil {
			if author, err := de.bookStore.GetAuthorByID(*book.AuthorID); err == nil && author != nil {
				authorName = author.Name
			}
		}

		// Layer 1 exact checks (file hash, ISBN/ASIN, near-identical title).
		// Errors are logged but non-fatal — one missing field shouldn't
		// abort the whole scan.
		if _, err := de.checkExactFileHash(&book, authorName); err != nil {
			log.Printf("dedup: full scan hash check error for %s: %v", book.ID, err)
		}
		if err := de.checkExactISBN(&book); err != nil {
			log.Printf("dedup: full scan ISBN check error for %s: %v", book.ID, err)
		}
		if err := de.checkExactTitle(&book, authorName); err != nil {
			log.Printf("dedup: full scan title check error for %s: %v", book.ID, err)
		}

		// Layer 2 embedding: re-embed if stale, then similarity scan.
		if de.embedClient != nil {
			if _, err := de.EmbedBook(ctx, book.ID); err != nil {
				log.Printf("dedup: full scan embed error for %s: %v", book.ID, err)
			}
		}
		if err := de.findSimilarBooks(ctx, book.ID); err != nil {
			// Not fatal — just means no embedding yet
			log.Printf("dedup: full scan similarity error for %s: %v", book.ID, err)
		}

		if progress != nil && (i%10 == 0 || i == total-1) {
			progress(i+1, total)
		}
	}
	return nil
}

// PurgeStaleCandidates deletes book-dedup-candidate rows that are no longer
// meaningful under the current rules:
//
//   - either side references a non-primary version-group member (their
//     identity is owned by their group's primary, not the row itself)
//   - both sides belong to the same version_group_id (the version-group
//     system already knows they are the same logical book)
//
// Returns the number of rows deleted. Intended to be called once at startup
// after the backfill completes and again at the start of a user-triggered
// Re-scan so the candidate table stays clean after the Layer 1 + Layer 2
// rules tighten.
func (de *DedupEngine) PurgeStaleCandidates(ctx context.Context) (int, error) {
	if de.embedStore == nil || de.bookStore == nil {
		return 0, nil
	}

	// First, canonicalize existing rows so duplicate-direction pairs
	// collapse into a single logical row. This has to run BEFORE the
	// stale-rule sweep below because otherwise we'd list the same pair
	// twice (once as (A,B), once as (B,A)) and maybe delete one copy
	// based on one rule and leave the other copy to cause confusion.
	if rewritten, deleted, err := de.embedStore.CanonicalizeCandidates(); err != nil {
		log.Printf("dedup: canonicalize candidates: %v", err)
	} else if rewritten > 0 || deleted > 0 {
		log.Printf("dedup: canonicalized %d candidate pair(s), deleted %d duplicate(s)",
			rewritten, deleted)
	}

	// CRITICAL: Only purge PENDING candidates. Merged and dismissed rows
	// are historical records the user explicitly wants to keep — they're
	// what populates the Merged / Dismissed tabs and lets the user see
	// what they've already actioned. Without this filter, every rescan
	// would delete every previously-merged candidate (because merged
	// books share a version_group_id, which is one of the stale-rule
	// conditions below), making the Merged tab useless.
	candidates, _, err := de.embedStore.ListCandidates(database.CandidateFilter{
		EntityType: "book",
		Status:     "pending",
		Limit:      100000,
	})
	if err != nil {
		return 0, fmt.Errorf("list candidates: %w", err)
	}

	// Memoise book lookups so a book referenced by many candidates is only
	// fetched once per purge run.
	type bookMeta struct {
		isNonPrimary   bool
		emptyTitle     bool
		versionGroupID string
		seriesNumber   string
		normTitle      string
		missing        bool
	}
	cache := make(map[string]bookMeta, len(candidates)*2)
	lookup := func(id string) bookMeta {
		if m, ok := cache[id]; ok {
			return m
		}
		b, err := de.bookStore.GetBookByID(id)
		m := bookMeta{}
		if err != nil || b == nil {
			m.missing = true
			cache[id] = m
			return m
		}
		if b.IsPrimaryVersion != nil && !*b.IsPrimaryVersion {
			m.isNonPrimary = true
		}
		if !hasUsableTitle(b.Title) {
			m.emptyTitle = true
		}
		if b.VersionGroupID != nil {
			m.versionGroupID = *b.VersionGroupID
		}
		m.seriesNumber = seriesNumberOf(b)
		m.normTitle = normalizeTitle(b.Title)
		cache[id] = m
		return m
	}

	deleted := 0
	for _, c := range candidates {
		select {
		case <-ctx.Done():
			return deleted, ctx.Err()
		default:
		}

		a := lookup(c.EntityAID)
		b := lookup(c.EntityBID)

		stale := false
		switch {
		case a.missing || b.missing:
			// One side no longer exists — the candidate can't be actioned.
			stale = true
		case a.isNonPrimary || b.isNonPrimary:
			stale = true
		case a.emptyTitle || b.emptyTitle:
			// One or both books have no usable title — their embedding
			// was garbage and any similarity match they produced is noise.
			stale = true
		case a.versionGroupID != "" && a.versionGroupID == b.versionGroupID:
			stale = true
		case a.seriesNumber != "" && b.seriesNumber != "" && a.seriesNumber != b.seriesNumber:
			// Distinct volumes of a numbered series (detected via a
			// structured field or an explicit "book/bk/vol/#" marker).
			stale = true
		case titlesDifferOnlyInDigits(a.normTitle, b.normTitle):
			// Fallback: normalized titles differ only in digit content,
			// meaning they're different volumes of a series whose marker
			// the explicit regex didn't match. "Reclaiming Honor bk 6"
			// vs "Reclaiming Honor bk 7" is the canonical example.
			stale = true
		}
		if !stale {
			continue
		}
		if err := de.embedStore.DeleteCandidate(c.ID); err != nil {
			log.Printf("dedup: purge stale candidate %d: %v", c.ID, err)
			continue
		}
		deleted++
	}
	return deleted, nil
}

// getAllBooks fetches all PRIMARY-version books in batches. Non-primary
// version-group members are filtered out so FullScan never processes them
// (their identity is owned by the primary) and similarity scanning only
// produces primary-vs-primary candidate pairs.
func (de *DedupEngine) getAllBooks() ([]database.Book, error) {
	var all []database.Book
	const batchSize = 500
	offset := 0
	for {
		batch, err := de.bookStore.GetAllBooks(batchSize, offset)
		if err != nil {
			return nil, err
		}
		for _, b := range batch {
			if b.IsPrimaryVersion != nil && !*b.IsPrimaryVersion {
				continue
			}
			all = append(all, b)
		}
		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}
	return all, nil
}

// RunLLMReview processes ambiguous candidates through LLM review (Layer 3).
// Pending book candidates whose similarity falls in [LLMBookLow, LLMBookHigh] and
// pending author candidates in [LLMAuthorLow, LLMAuthorHigh] are fetched, enriched
// with entity metadata, batched, and sent to the OpenAI chat LLM. The verdict is
// persisted via UpdateCandidateLLM (which also sets layer='llm').
//
// Candidates that are already at layer='llm' are skipped — rerunning is cheap in
// bookkeeping but expensive in API calls, so callers should use UpsertCandidate to
// clear the layer back to 'embedding' if they want a re-review.
func (de *DedupEngine) RunLLMReview(ctx context.Context) error {
	if de.llmParser == nil || !de.llmParser.IsEnabled() {
		log.Println("dedup: LLM review skipped — llmParser not configured")
		return nil
	}
	if de.embedStore == nil {
		return fmt.Errorf("dedup: LLM review requires embedStore")
	}

	bookCandidates, err := de.listAmbiguousCandidates("book", de.LLMBookLow, de.LLMBookHigh)
	if err != nil {
		return fmt.Errorf("list ambiguous book candidates: %w", err)
	}
	authorCandidates, err := de.listAmbiguousCandidates("author", de.LLMAuthorLow, de.LLMAuthorHigh)
	if err != nil {
		return fmt.Errorf("list ambiguous author candidates: %w", err)
	}

	allCandidates := append(bookCandidates, authorCandidates...)
	if de.LLMMaxPairsPerRun > 0 && len(allCandidates) > de.LLMMaxPairsPerRun {
		allCandidates = allCandidates[:de.LLMMaxPairsPerRun]
	}
	if len(allCandidates) == 0 {
		log.Println("dedup: LLM review found no pending ambiguous candidates")
		return nil
	}
	log.Printf("dedup: LLM review starting — %d pair(s) queued", len(allCandidates))

	// Build inputs alongside an index→candidate map for verdict routing.
	inputs := make([]ai.DedupPairInput, 0, len(allCandidates))
	byIndex := make(map[int]database.DedupCandidate, len(allCandidates))
	for i, c := range allCandidates {
		input, ok := de.buildPairInput(i, c)
		if !ok {
			log.Printf("dedup: skipping candidate %d — could not load entities", c.ID)
			continue
		}
		inputs = append(inputs, input)
		byIndex[i] = c
	}
	if len(inputs) == 0 {
		return nil
	}

	verdicts, err := de.llmParser.ReviewDedupPairs(ctx, inputs)
	if err != nil {
		// Persist whatever we did get before surfacing the error.
		de.applyVerdicts(verdicts, byIndex)
		return fmt.Errorf("LLM review call: %w", err)
	}
	applied := de.applyVerdicts(verdicts, byIndex)
	log.Printf("dedup: LLM review complete — %d verdict(s) applied", applied)
	return nil
}

// listAmbiguousCandidates returns pending embedding-layer candidates whose
// similarity falls inside [low, high].
func (de *DedupEngine) listAmbiguousCandidates(entityType string, low, high float64) ([]database.DedupCandidate, error) {
	filter := database.CandidateFilter{
		EntityType:    entityType,
		Status:        "pending",
		Layer:         "embedding",
		MinSimilarity: &low,
		MaxSimilarity: &high,
		Limit:         10000,
	}
	candidates, _, err := de.embedStore.ListCandidates(filter)
	return candidates, err
}

// buildPairInput enriches a stored candidate with entity details suitable for
// the LLM prompt. Returns false if either entity could not be loaded.
func (de *DedupEngine) buildPairInput(index int, c database.DedupCandidate) (ai.DedupPairInput, bool) {
	input := ai.DedupPairInput{
		Index:      index,
		EntityType: c.EntityType,
	}
	if c.Similarity != nil {
		input.Similarity = *c.Similarity
	}

	switch c.EntityType {
	case "book":
		a, aOK := de.loadBookEntity(c.EntityAID)
		b, bOK := de.loadBookEntity(c.EntityBID)
		if !aOK || !bOK {
			return input, false
		}
		input.A = a
		input.B = b
	case "author":
		a, aOK := de.loadAuthorEntity(c.EntityAID)
		b, bOK := de.loadAuthorEntity(c.EntityBID)
		if !aOK || !bOK {
			return input, false
		}
		input.A = a
		input.B = b
	default:
		return input, false
	}
	return input, true
}

// loadBookEntity fetches a book and converts it into a DedupEntity. The caller
// may rely on ID always being populated when the second return value is true.
func (de *DedupEngine) loadBookEntity(bookID string) (ai.DedupEntity, bool) {
	book, err := de.bookStore.GetBookByID(bookID)
	if err != nil || book == nil {
		return ai.DedupEntity{}, false
	}
	entity := ai.DedupEntity{ID: book.ID, Title: book.Title}
	if book.AuthorID != nil {
		if author, aerr := de.bookStore.GetAuthorByID(*book.AuthorID); aerr == nil && author != nil {
			entity.Author = author.Name
		}
	}
	if book.Narrator != nil {
		entity.Narrator = *book.Narrator
	}
	if book.ISBN13 != nil && *book.ISBN13 != "" {
		entity.ISBN = *book.ISBN13
	} else if book.ISBN10 != nil {
		entity.ISBN = *book.ISBN10
	}
	if book.ASIN != nil {
		entity.ASIN = *book.ASIN
	}
	return entity, true
}

// loadAuthorEntity fetches an author and converts it into a DedupEntity. The
// Title field carries the author name so the prompt treats both entity types
// uniformly.
func (de *DedupEngine) loadAuthorEntity(entityID string) (ai.DedupEntity, bool) {
	id, err := strconv.Atoi(entityID)
	if err != nil {
		return ai.DedupEntity{}, false
	}
	author, err := de.bookStore.GetAuthorByID(id)
	if err != nil || author == nil {
		return ai.DedupEntity{}, false
	}
	return ai.DedupEntity{ID: entityID, Title: author.Name}, true
}

// applyVerdicts persists each verdict via UpdateCandidateLLM and returns the
// number of rows successfully updated. Errors are logged and skipped so one
// bad row does not abort the whole batch.
func (de *DedupEngine) applyVerdicts(verdicts []ai.DedupPairVerdict, byIndex map[int]database.DedupCandidate) int {
	applied := 0
	for _, v := range verdicts {
		candidate, ok := byIndex[v.Index]
		if !ok {
			log.Printf("dedup: LLM returned unknown index %d", v.Index)
			continue
		}
		verdict := "not_duplicate"
		if v.IsDuplicate {
			verdict = "duplicate"
		}
		reason := v.Reason
		if v.Confidence != "" {
			reason = fmt.Sprintf("[%s] %s", v.Confidence, reason)
		}
		if err := de.embedStore.UpdateCandidateLLM(candidate.ID, verdict, reason); err != nil {
			log.Printf("dedup: failed to update candidate %d: %v", candidate.ID, err)
			continue
		}
		applied++
	}
	return applied
}

// levenshteinDistance computes the Levenshtein edit distance between two strings.
func levenshteinDistance(a, b string) int {
	la := len(a)
	lb := len(b)

	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use two rows instead of full matrix for O(min(m,n)) space.
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = min3(del, ins, sub)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// min3 returns the minimum of three integers.
func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// normalizeTitle lowercases, trims whitespace, and collapses internal whitespace.
// normalizeTitleRe matches anything that isn't alphanumeric or whitespace —
// used to strip punctuation so "Foo: The Bar" and "Foo The Bar" collapse to
// the same normalized form.
var normalizeTitleRe = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)

// normalizeTitleQuoteStripper matches apostrophes / single quotes / smart
// quotes — characters that should be stripped to *nothing* rather than a
// space so "Ender's Game" becomes "enders game" instead of "ender s game".
var normalizeTitleQuoteStripper = regexp.MustCompile("[\u0027\u2018\u2019\u201C\u201D\"]")

// normalizeTitle folds a title to the canonical form used across the dedup
// engine's exact-match layer. It is deliberately aggressive:
//
//   - lowercase
//   - "&" and "+" fold to " and " so "Foundation & Empire" matches
//     "Foundation and Empire"
//   - all non-alphanumeric characters (punctuation, smart quotes, em-dashes)
//     are stripped
//   - leading articles ("the", "a", "an") are dropped so "The Hobbit"
//     matches "Hobbit"
//   - multiple whitespace runs collapse to a single space
//
// These transforms mirror what a human naturally ignores when deciding if
// two titles are "the same book". They are applied uniformly on both sides
// of any title comparison so the folding never produces false positives on
// its own — the caller still has to decide how close is close enough.
func normalizeTitle(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))

	// Strip quotes and apostrophes to *nothing* first, so "Ender's Game"
	// collapses to "enders game" instead of "ender s game". This has to
	// happen before the general punctuation pass because that one
	// replaces with a space.
	title = normalizeTitleQuoteStripper.ReplaceAllString(title, "")

	// Fold ampersands / plus signs to the word "and" BEFORE stripping
	// punctuation, otherwise the regex would eat the "&" and leave the
	// words around it glued together ("Foo & Bar" -> "foo  bar", which
	// normalizes the same as "Foo Bar" and loses the conjunction).
	title = strings.ReplaceAll(title, "&", " and ")
	title = strings.ReplaceAll(title, "+", " and ")

	// Strip everything else that isn't a letter, digit, or whitespace —
	// colons, dashes, parens, etc. all get replaced with a space so the
	// words on either side don't glue together.
	title = normalizeTitleRe.ReplaceAllString(title, " ")

	// Collapse whitespace runs to a single space.
	title = strings.Join(strings.Fields(title), " ")

	// Drop a leading article. Only a leading one — "A Game of Thrones"
	// should match "Game of Thrones" but "Go Set a Watchman" must not
	// turn into "Go Set Watchman".
	for _, article := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(title, article) {
			title = title[len(article):]
			break
		}
	}

	return title
}

// derefStr is defined in audiobook_service.go
