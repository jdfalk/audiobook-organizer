// file: internal/server/dedup_engine.go
// version: 1.0.0
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
	mergeService *MergeService

	// Thresholds (read from config or set directly)
	BookHighThreshold   float64
	BookLowThreshold    float64
	AuthorHighThreshold float64
	AuthorLowThreshold  float64
	AutoMergeEnabled    bool
}

// NewDedupEngine creates a DedupEngine with sensible defaults.
func NewDedupEngine(
	embedStore *database.EmbeddingStore,
	bookStore database.Store,
	embedClient *ai.EmbeddingClient,
	mergeService *MergeService,
) *DedupEngine {
	return &DedupEngine{
		embedStore:          embedStore,
		bookStore:           bookStore,
		embedClient:         embedClient,
		mergeService:        mergeService,
		BookHighThreshold:   0.95,
		BookLowThreshold:    0.85,
		AuthorHighThreshold: 0.92,
		AuthorLowThreshold:  0.80,
		AutoMergeEnabled:    false,
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

	results, err := de.embedStore.FindSimilar("book", emb.Vector, float32(de.BookLowThreshold), 20)
	if err != nil {
		return err
	}

	for _, r := range results {
		if r.EntityID == bookID {
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

// EmbedBook generates and stores an embedding for the given book.
// Skips re-embedding if the text hash has not changed.
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

// FullScan re-embeds stale entities and runs Layer 2 similarity scans for all books.
// The progress callback is invoked periodically with (done, total).
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

		// Embed (skips if hash unchanged)
		if de.embedClient != nil {
			if err := de.EmbedBook(ctx, book.ID); err != nil {
				log.Printf("dedup: full scan embed error for %s: %v", book.ID, err)
			}
		}

		// Layer 2 similarity
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

// getAllBooks fetches all books in batches.
func (de *DedupEngine) getAllBooks() ([]database.Book, error) {
	var all []database.Book
	const batchSize = 500
	offset := 0
	for {
		batch, err := de.bookStore.GetAllBooks(batchSize, offset)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < batchSize {
			break
		}
		offset += batchSize
	}
	return all, nil
}

// RunLLMReview processes ambiguous candidates through LLM review (Layer 3).
// TODO: Implement full LLM review using OpenAI chat completion.
func (de *DedupEngine) RunLLMReview(ctx context.Context) error {
	log.Println("dedup: LLM review not yet implemented")
	return nil
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
