// file: internal/server/dedup_engine.go
// version: 1.2.0
// guid: 8f3a1c6e-d472-4b9a-a5e1-7c2d9f0b3e84

package server

import (
	"context"
	"fmt"
	"log"
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
func (de *DedupEngine) CheckBook(ctx context.Context, bookID string) (bool, error) {
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
		if err := de.EmbedBook(ctx, bookID); err != nil {
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

// checkExactTitle checks all books by the same author for near-identical titles.
func (de *DedupEngine) checkExactTitle(book *database.Book, authorName string) error {
	if book.AuthorID == nil {
		return nil
	}

	others, err := de.bookStore.GetBooksByAuthorID(*book.AuthorID)
	if err != nil {
		return fmt.Errorf("get books by author: %w", err)
	}

	normTitle := normalizeTitle(book.Title)
	for i := range others {
		other := &others[i]
		if other.ID == book.ID {
			continue
		}
		dist := levenshteinDistance(normTitle, normalizeTitle(other.Title))
		if dist < 3 {
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
	}
	return nil
}

// findSimilarBooks runs Layer 2 embedding similarity search for a book.
func (de *DedupEngine) findSimilarBooks(ctx context.Context, bookID string) error {
	emb, err := de.embedStore.Get("book", bookID)
	if err != nil || emb == nil {
		return fmt.Errorf("no embedding for book %s", bookID)
	}

	// Load the query book once so we can consult its version_group_id when
	// filtering candidates. If the lookup fails we proceed without the
	// filter — skipping pairs is an optimisation, not a correctness need.
	queryBook, _ := de.bookStore.GetBookByID(bookID)

	results, err := de.embedStore.FindSimilar("book", emb.Vector, float32(de.BookLowThreshold), 20)
	if err != nil {
		return err
	}

	for _, r := range results {
		if r.EntityID == bookID {
			continue
		}
		// Drop candidates that are already siblings in the same version
		// group. The version-group system already knows these are the same
		// logical book in different formats, so surfacing them as dedup
		// candidates is just noise.
		if queryBook != nil && queryBook.VersionGroupID != nil && *queryBook.VersionGroupID != "" {
			otherBook, err := de.bookStore.GetBookByID(r.EntityID)
			if err == nil && otherBook != nil && otherBook.VersionGroupID != nil &&
				*otherBook.VersionGroupID == *queryBook.VersionGroupID {
				continue
			}
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

// EmbedBook generates and stores an embedding for the given book.
// Skips re-embedding if the text hash has not changed.
//
// Non-primary versions (members of a version group that are not the primary
// representative) are skipped entirely: their embedding would be a duplicate
// of the primary's by construction, and surfacing them as dedup candidates
// just clutters the UI with noise. Any existing embedding for a non-primary
// book is deleted on the spot so historical rows from earlier backfills get
// cleaned up as we walk the library.
func (de *DedupEngine) EmbedBook(ctx context.Context, bookID string) error {
	if de.embedClient == nil {
		return fmt.Errorf("no embedding client configured")
	}

	book, err := de.bookStore.GetBookByID(bookID)
	if err != nil {
		return fmt.Errorf("get book %s: %w", bookID, err)
	}
	if book == nil {
		return fmt.Errorf("book %s not found", bookID)
	}

	// Skip non-primary version-group members. If the book was previously
	// embedded (stale data from a pre-fix backfill), remove that row now.
	if book.IsPrimaryVersion != nil && !*book.IsPrimaryVersion {
		if err := de.embedStore.Delete("book", bookID); err != nil {
			log.Printf("dedup: delete stale embedding for non-primary %s: %v", bookID, err)
		}
		return nil
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

	// Check if existing embedding already has this hash — skip if so
	existing, err := de.embedStore.Get("book", bookID)
	if err == nil && existing != nil && existing.TextHash == hash {
		return nil // already up to date
	}

	vec, err := de.embedClient.EmbedOne(ctx, text)
	if err != nil {
		return fmt.Errorf("embed text: %w", err)
	}

	return de.embedStore.Upsert(database.Embedding{
		EntityType: "book",
		EntityID:   bookID,
		TextHash:   hash,
		Vector:     vec,
		Model:      "text-embedding-3-large",
	})
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
			if err := de.EmbedBook(ctx, book.ID); err != nil {
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

	candidates, _, err := de.embedStore.ListCandidates(database.CandidateFilter{
		EntityType: "book",
		Limit:      100000,
	})
	if err != nil {
		return 0, fmt.Errorf("list candidates: %w", err)
	}

	// Memoise book lookups so a book referenced by many candidates is only
	// fetched once per purge run.
	type bookMeta struct {
		isNonPrimary   bool
		versionGroupID string
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
		if b.VersionGroupID != nil {
			m.versionGroupID = *b.VersionGroupID
		}
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
		case a.versionGroupID != "" && a.versionGroupID == b.versionGroupID:
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
func normalizeTitle(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	// Collapse multiple spaces to one
	parts := strings.Fields(title)
	return strings.Join(parts, " ")
}

// derefStr is defined in audiobook_service.go
