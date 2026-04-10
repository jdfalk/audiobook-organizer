// file: internal/ai/embedding_client.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// EmbeddingClient handles OpenAI embedding generation
type EmbeddingClient struct {
	client *openai.Client
	model  string
}

// NewEmbeddingClient creates a new embedding client using the given API key.
// Default model is text-embedding-3-large.
func NewEmbeddingClient(apiKey string) *EmbeddingClient {
	clientOptions := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		clientOptions = append(clientOptions, option.WithBaseURL(baseURL))
	}

	client := openai.NewClient(clientOptions...)
	return &EmbeddingClient{
		client: &client,
		model:  "text-embedding-3-large",
	}
}

// EmbedBatch sends up to 100 texts to the OpenAI Embeddings API and returns one
// []float32 per input in the same order. Retries up to 3 times with exponential
// backoff (1s, 4s).
func (c *EmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
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
