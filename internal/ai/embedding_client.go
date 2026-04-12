// file: internal/ai/embedding_client.go
// version: 1.1.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// EmbeddingCache is the minimal surface EmbeddingClient needs from
// a content-hash cache layer. It's an interface (not a concrete
// type) so internal/ai doesn't need to import internal/database,
// which would create a circular dependency. Production wires the
// *database.EmbeddingStore implementation at server startup.
//
// Get returns (nil, nil) on a cache miss so callers can use the
// two-valued result without needing a sentinel error constant.
// Put is called with the newly-embedded vectors after a successful
// API call; a Put failure is logged but never fatal because the
// cache is an optimization, not a correctness requirement.
//
// Added after the 2026-04-11 OpenAI quota incident: the metadata
// scorer was re-embedding every candidate on every fetch — no
// content-hash cache existed — and burned the entire monthly
// budget in minutes.
type EmbeddingCache interface {
	GetCachedEmbedding(textHash, model string) ([]float32, error)
	PutCachedEmbedding(textHash, model string, vector []float32) error
}

// EmbeddingClient handles OpenAI embedding generation. Optional
// content-hash cache (see EmbeddingCache) short-circuits repeat
// embeds of identical text so a bulk fetch of 10K books only
// hits the API for unique title+author+narrator combinations.
type EmbeddingClient struct {
	client *openai.Client
	model  string
	cache  EmbeddingCache

	// rawEmbed is the underlying API-call function. It defaults
	// to c.embedBatchRaw which hits OpenAI; tests override it
	// with a fake so the cache-partitioning logic can be
	// exercised without a real API key.
	rawEmbed func(ctx context.Context, texts []string) ([][]float32, error)
}

// NewEmbeddingClient creates a new embedding client using the given API key.
// Default model is text-embedding-3-large. The returned client has no cache
// wired up — call WithCache after construction to enable content-hash caching.
func NewEmbeddingClient(apiKey string) *EmbeddingClient {
	clientOptions := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		clientOptions = append(clientOptions, option.WithBaseURL(baseURL))
	}

	client := openai.NewClient(clientOptions...)
	c := &EmbeddingClient{
		client: &client,
		model:  "text-embedding-3-large",
	}
	c.rawEmbed = c.embedBatchRaw
	return c
}

// WithCache attaches a content-hash cache to the client. Safe to
// call at any time — subsequent EmbedBatch calls start using the
// cache immediately. Pass nil to disable caching (reverts to
// uncached behavior).
func (c *EmbeddingClient) WithCache(cache EmbeddingCache) *EmbeddingClient {
	c.cache = cache
	return c
}

// Model returns the embedding model name this client is pinned
// to. Used by cache-aware callers that need to compose cache
// keys.
func (c *EmbeddingClient) Model() string {
	return c.model
}

// EmbedBatch returns one []float32 per input text in the same
// order. If a content-hash cache is attached (see WithCache),
// inputs are partitioned into cache hits and misses: hits are
// served from the cache with zero API cost, misses are sent to
// OpenAI in a single batch and the results are written back to
// the cache before being returned.
//
// Retries up to 3 times with exponential backoff (1s, 4s) on
// API errors. Cache I/O errors are logged but never fail the
// call — the cache is an optimization, not a correctness layer.
func (c *EmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Partition texts into cache hits and misses. `results` keeps
	// the original order; `missIndices` remembers which slots in
	// `results` are still empty and need an API call. `missTexts`
	// is the compact list we actually send to OpenAI.
	results := make([][]float32, len(texts))
	var missIndices []int
	var missTexts []string
	hits := 0

	if c.cache != nil {
		for i, text := range texts {
			hash := TextHash(text)
			vec, err := c.cache.GetCachedEmbedding(hash, c.model)
			if err != nil {
				// Cache read failure — treat as miss, log once.
				log.Printf("[WARN] embedding cache get failed (hash=%s): %v", hash[:8], err)
			}
			if err == nil && vec != nil {
				results[i] = vec
				hits++
				continue
			}
			missIndices = append(missIndices, i)
			missTexts = append(missTexts, text)
		}
	} else {
		missIndices = make([]int, len(texts))
		missTexts = texts
		for i := range texts {
			missIndices[i] = i
		}
	}

	// All cache hits? Return without touching the API at all.
	if len(missTexts) == 0 {
		log.Printf("[DEBUG] embedding cache: %d/%d hits, 0 API calls", hits, len(texts))
		return results, nil
	}

	if c.cache != nil {
		log.Printf("[DEBUG] embedding cache: %d/%d hits, %d misses sent to API",
			hits, len(texts), len(missTexts))
	}

	apiResults, err := c.rawEmbed(ctx, missTexts)
	if err != nil {
		return nil, err
	}
	if len(apiResults) != len(missTexts) {
		return nil, fmt.Errorf("embedding API returned %d results for %d inputs",
			len(apiResults), len(missTexts))
	}

	// Stitch API results back into the original positions and
	// write them to the cache for next time.
	for j, vec := range apiResults {
		origIdx := missIndices[j]
		results[origIdx] = vec
		if c.cache != nil {
			hash := TextHash(missTexts[j])
			if putErr := c.cache.PutCachedEmbedding(hash, c.model, vec); putErr != nil {
				log.Printf("[WARN] embedding cache put failed (hash=%s): %v",
					hash[:8], putErr)
			}
		}
	}
	return results, nil
}

// embedBatchRaw is the actual OpenAI API call with retries —
// EmbedBatch handles the cache partitioning around it. Split out
// so the caching logic can call it with just the miss set.
func (c *EmbeddingClient) embedBatchRaw(ctx context.Context, texts []string) ([][]float32, error) {
	var lastErr error
	delays := []time.Duration{1 * time.Second, 4 * time.Second}

	for attempt := 0; attempt <= 2; attempt++ {
		if attempt > 0 {
			delay := delays[attempt-1]
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := c.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
			Input: openai.EmbeddingNewParamsInputUnion{
				OfArrayOfStrings: texts,
			},
			Model: openai.EmbeddingModel(c.model),
		})
		if err != nil {
			lastErr = fmt.Errorf("embedding attempt %d: %w", attempt+1, err)
			continue
		}

		// Allocate result slice sized to number of returned embeddings
		results := make([][]float32, len(resp.Data))
		for _, item := range resp.Data {
			idx := int(item.Index)
			if idx < 0 || idx >= len(results) {
				return nil, fmt.Errorf("embedding response index %d out of range (len=%d)", idx, len(results))
			}
			f32 := make([]float32, len(item.Embedding))
			for j, v := range item.Embedding {
				f32[j] = float32(v)
			}
			results[idx] = f32
		}
		return results, nil
	}

	return nil, lastErr
}

// EmbedOne is a convenience wrapper that embeds a single text string.
func (c *EmbeddingClient) EmbedOne(ctx context.Context, text string) ([]float32, error) {
	results, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return results[0], nil
}

// BuildEmbeddingText constructs a human-readable text representation of an entity
// suitable for embedding. entityType may be "book" or "author".
func BuildEmbeddingText(entityType, title, author, narrator string) string {
	switch entityType {
	case "book":
		if narrator != "" {
			return fmt.Sprintf("%s by %s narrated by %s", title, author, narrator)
		}
		return fmt.Sprintf("%s by %s", title, author)
	case "author":
		return title
	default:
		return title
	}
}

// TextHash returns the SHA-256 hex digest of the input string (64 characters).
func TextHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}
