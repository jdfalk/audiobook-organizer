// file: internal/ai/embedding_batch.go
// version: 1.0.0
// guid: a7b8c9d0-e1f2-3456-7890-abcdef012345
// last-edited: 2026-05-17

package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/openai/openai-go"
)

// EmbedBatchItem is one book→text pair submitted for async embedding.
type EmbedBatchItem struct {
	BookID string
	Text   string
}

// EmbedBatchResult holds a single result returned from a completed embedding batch.
type EmbedBatchResult struct {
	BookID string
	Vector []float32
}

// CreateEmbeddingBatch submits a list of book texts to the OpenAI Batch API
// using the /v1/embeddings endpoint. Returns the batch ID for polling.
func (c *EmbeddingClient) CreateEmbeddingBatch(ctx context.Context, items []EmbedBatchItem) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("embedding client not initialised")
	}
	if len(items) == 0 {
		return "", fmt.Errorf("no items to embed")
	}

	// Build JSONL — one request per item.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, it := range items {
		req := map[string]any{
			"custom_id": "book-" + it.BookID,
			"method":    "POST",
			"url":       "/v1/embeddings",
			"body": map[string]any{
				"model":           c.model,
				"input":           it.Text,
				"encoding_format": "float",
			},
		}
		if err := enc.Encode(req); err != nil {
			return "", fmt.Errorf("encode batch request: %w", err)
		}
	}

	// Upload the JSONL file.
	file, err := c.client.Files.New(ctx, openai.FileNewParams{
		File:    bytes.NewReader(buf.Bytes()),
		Purpose: openai.FilePurposeBatch,
	})
	if err != nil {
		return "", fmt.Errorf("upload embedding batch file: %w", err)
	}

	// Create the batch job.
	batch, err := c.client.Batches.New(ctx, openai.BatchNewParams{
		InputFileID:      file.ID,
		Endpoint:         openai.BatchNewParamsEndpointV1Embeddings,
		CompletionWindow: openai.BatchNewParamsCompletionWindow24h,
		Metadata: map[string]string{
			"project": "audiobook-organizer",
			"type":    "embed_async",
		},
	})
	if err != nil {
		return "", fmt.Errorf("create embedding batch: %w", err)
	}
	return batch.ID, nil
}

// DownloadEmbeddingBatchResults downloads the output file for a completed
// embedding batch and returns the parsed vectors keyed by book ID.
func (c *EmbeddingClient) DownloadEmbeddingBatchResults(ctx context.Context, outputFileID string) ([]EmbedBatchResult, error) {
	if c.client == nil {
		return nil, fmt.Errorf("embedding client not initialised")
	}

	resp, err := c.client.Files.Content(ctx, outputFileID)
	if err != nil {
		return nil, fmt.Errorf("download embedding batch output: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embedding batch output: %w", err)
	}

	return parseEmbeddingBatchOutput(data)
}

// parseEmbeddingBatchOutput parses JSONL output from an embedding batch.
// Each line has custom_id "book-<ID>" and a response body with data[0].embedding.
func parseEmbeddingBatchOutput(data []byte) ([]EmbedBatchResult, error) {
	var results []EmbedBatchResult
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry struct {
			CustomID string `json:"custom_id"`
			Response struct {
				StatusCode int `json:"status_code"`
				Body       struct {
					Data []struct {
						Embedding []float64 `json:"embedding"`
					} `json:"data"`
				} `json:"body"`
			} `json:"response"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Error != nil || entry.Response.StatusCode != 200 {
			continue
		}
		if len(entry.Response.Body.Data) == 0 || len(entry.Response.Body.Data[0].Embedding) == 0 {
			continue
		}

		bookID := strings.TrimPrefix(entry.CustomID, "book-")
		vec64 := entry.Response.Body.Data[0].Embedding
		vec := make([]float32, len(vec64))
		for i, v := range vec64 {
			vec[i] = float32(v)
		}
		results = append(results, EmbedBatchResult{BookID: bookID, Vector: vec})
	}
	return results, scanner.Err()
}
