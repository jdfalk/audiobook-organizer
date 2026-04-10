// file: internal/ai/metadata_scorer.go
// version: 1.0.0
// guid: 53eab597-169d-4fe7-aa46-a451cb89b1ea

package ai

import "context"

// MetadataCandidateScorer ranks candidate metadata search results by how well
// each one matches a query book. It is the abstraction point that lets the
// metadata fetch pipeline swap between embedding cosine similarity, a chat
// LLM judgment, a cross-encoder reranker, or a simple token-overlap fallback
// without the caller knowing which implementation is in use.
//
// Contract for all implementations:
//
//   - Score must return exactly one score per input candidate, in the same
//     order as the input slice.
//   - Scores must be clamped to [0.0, 1.0] where 1.0 means "definitely the
//     same book" and 0.0 means "definitely not."
//   - Implementations must NEVER return a partial result with a nil error.
//     Any failure (API error, missing dependency, empty query) returns
//     (nil, err) so the caller can fall back to the next tier.
//   - An empty cands slice returns (nil, nil) — not an error, just nothing
//     to score.
//   - Name returns a short identifier used in logs and UI badges
//     ("embedding", "llm:gpt-5-mini", "rerank:cohere-v3"). It must be stable
//     across the lifetime of a process so logs stay searchable.
type MetadataCandidateScorer interface {
	Score(ctx context.Context, q Query, cands []Candidate) ([]float64, error)
	Name() string
}

// Query describes the book the caller is searching metadata for. BookID is
// an optional fast-path — if set and the scorer has a pre-computed vector
// for that book in the EmbeddingStore, it can skip the cost of re-embedding
// the query. Scorers that do not have an embedding store should just ignore
// BookID.
type Query struct {
	BookID   string
	Title    string
	Author   string
	Narrator string
}

// Candidate is one search result being scored. Fields mirror the identity
// slice of metadata.BookMetadata — title, author, narrator — because those
// are the three fields that matter for ranking and including more (publisher,
// description, cover URL) just inflates token counts without improving the
// signal.
type Candidate struct {
	Title    string
	Author   string
	Narrator string
}
