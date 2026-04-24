// file: internal/ai/metadata_llm_review.go
// version: 2.0.0
// guid: e4f92b17-3c8a-4d65-a1f3-9b2e07d84c61

package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/ai/aijobs"
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

// MetadataLLMQuery describes the book the caller is searching metadata for.
// It's the AI-package-local view of ai.Query, kept separate so the JSON
// field names used in the LLM prompt are frozen regardless of future changes
// to the public scorer interface.
type MetadataLLMQuery struct {
	Title    string `json:"title"`
	Author   string `json:"author,omitempty"`
	Narrator string `json:"narrator,omitempty"`
}

// MetadataLLMCandidate is one search result the LLM ranks against the query.
type MetadataLLMCandidate struct {
	Index    int    `json:"index"`
	Title    string `json:"title"`
	Author   string `json:"author,omitempty"`
	Narrator string `json:"narrator,omitempty"`
}

// MetadataLLMScore is the LLM's judgment for a single candidate. Score is
// in [0.0, 1.0] where 1.0 means "definitely the same book." Reason is a
// short one-sentence explanation suitable for display in a debug log or UI
// tooltip.
type MetadataLLMScore struct {
	Index  int     `json:"index"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

// metadataLLMBatchSize caps the number of candidates sent per chat request.
// Matches dedupReviewBatchSize in dedup_review.go — 25 is comfortably under
// the structured-JSON token limits with typical per-candidate payloads.
const metadataLLMBatchSize = 25

// metadataLLMSystemPrompt is the system instruction for the LLM scorer.
const metadataLLMSystemPrompt = `You are an expert audiobook metadata reviewer. You will receive one query book and a batch of candidate search results. For each candidate, score how well it matches the query on a scale from 0.0 to 1.0, where:

- 1.0 = definitely the same book (same title and author, allowing minor punctuation/subtitle differences)
- 0.7-0.9 = probably the same book (same title core, same author, minor edition differences)
- 0.4-0.6 = ambiguous (partial title match, unclear author)
- 0.0-0.3 = probably not the same book (different volumes in a series, unrelated titles)

Scoring rules:
- Title identity matters most. "The Way of Kings" and "Stormlight Archive 1: The Way of Kings" are the same book.
- Author match is a strong signal. Same title with a different author is usually a different book.
- Narrator differences do NOT reduce the score — re-recordings of the same book are still the same book.
- Series position mismatches (volume 6 vs volume 3) should score low (~0.2) even if the series name matches.
- Compilations and omnibus editions should score low (~0.3) unless the query is itself an omnibus.

Return ONLY valid JSON in this exact shape:
{"scores": [{"index": N, "score": 0.0-1.0, "reason": "one-sentence explanation"}]}

Include one score per input candidate, using the same index as the input.`

// metadataReviewPayload persists inputs + candidate indices for async processing.
// Used by the callback to return results in the same order as the request.
type metadataReviewPayload struct {
	Inputs []MetadataLLMCandidate `json:"inputs"`
	Query  MetadataLLMQuery       `json:"query"`
}

// MetadataScoreApplier applies LLM metadata scores after a batch completes.
// Implemented by a sink in the metafetch or ai package.
type MetadataScoreApplier interface {
	// ApplyMetadataScores persists or caches scores keyed by query+candidate combo.
	// Returns the number of scores successfully applied.
	ApplyMetadataScores(query MetadataLLMQuery, scores []MetadataLLMScore) int
}

// ScoreMetadataCandidates asks the LLM to rank candidate metadata search
// results against a query book. It batches inputs internally and returns one
// score per candidate, in input order. Indices in the response are used to
// route scores back to their input slot — missing indices default to 0.0
// (the caller should treat them as "LLM didn't rank this one, use the base
// score instead").
//
// Returns (nil, err) on any failure so callers can fall back to the base
// scorer — no partial results with a nil error.
func (p *OpenAIParser) ScoreMetadataCandidates(
	ctx context.Context,
	query MetadataLLMQuery,
	candidates []MetadataLLMCandidate,
) ([]MetadataLLMScore, error) {
	if !p.enabled {
		return nil, fmt.Errorf("OpenAI parser is not enabled")
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// Ensure every candidate carries a sequential index so the LLM can
	// reference them unambiguously. We don't trust the caller to pre-number.
	indexed := make([]MetadataLLMCandidate, len(candidates))
	for i, c := range candidates {
		c.Index = i
		indexed[i] = c
	}

	// Submit to aijobs batch API if available.
	if p.aiJobsStore != nil {
		return p.scoreMetadataViaAIJobs(ctx, query, indexed)
	}

	// Fallback to legacy synchronous mode (should not reach in production).
	return p.scoreMetadataSynchronous(ctx, query, indexed)
}

// scoreMetadataViaAIJobs submits candidates to the batch API and blocks on results.
func (p *OpenAIParser) scoreMetadataViaAIJobs(
	ctx context.Context,
	query MetadataLLMQuery,
	candidates []MetadataLLMCandidate,
) ([]MetadataLLMScore, error) {
	if p.aiJobsStore == nil {
		return nil, fmt.Errorf("aiJobsStore not configured; cannot use batch API for metadata scoring")
	}

	// Build Deps locally with the AIJobsBatchClient.
	deps := aijobs.Deps{
		Store:  p.aiJobsStore,
		Client: &AIJobsBatchClient{Parser: p},
	}

	var all []MetadataLLMScore
	for start := 0; start < len(candidates); start += metadataLLMBatchSize {
		end := start + metadataLLMBatchSize
		if end > len(candidates) {
			end = len(candidates)
		}
		batch := candidates[start:end]

		jobID, err := SubmitMetadataReviewJob(ctx, deps, p.model, query, batch)
		if err != nil {
			return all, fmt.Errorf("submit metadata review job [%d:%d]: %w", start, end, err)
		}

		// Poll for results (this is synchronous from caller's perspective).
		scores, err := p.pollMetadataJobResults(ctx, jobID)
		if err != nil {
			return all, fmt.Errorf("poll metadata job %s: %w", jobID, err)
		}
		all = append(all, scores...)
	}
	return all, nil
}

// scoreMetadataSynchronous is the fallback legacy path using Chat.Completions.New.
func (p *OpenAIParser) scoreMetadataSynchronous(
	ctx context.Context,
	query MetadataLLMQuery,
	candidates []MetadataLLMCandidate,
) ([]MetadataLLMScore, error) {
	var all []MetadataLLMScore
	for start := 0; start < len(candidates); start += metadataLLMBatchSize {
		end := start + metadataLLMBatchSize
		if end > len(candidates) {
			end = len(candidates)
		}
		batch := candidates[start:end]
		scores, err := p.scoreMetadataBatchSync(ctx, query, batch)
		if err != nil {
			return all, fmt.Errorf("metadata LLM batch [%d:%d]: %w", start, end, err)
		}
		all = append(all, scores...)
	}
	return all, nil
}

// SubmitMetadataReviewJob enqueues an aijobs batch for metadata scoring.
// Inputs are split into sub-batches of metadataLLMBatchSize candidates.
// Returns the job ID immediately; results are retrieved asynchronously via pollMetadataJobResults.
func SubmitMetadataReviewJob(
	ctx context.Context,
	deps aijobs.Deps,
	model string,
	query MetadataLLMQuery,
	candidates []MetadataLLMCandidate,
) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("no candidates to review")
	}

	// Persist the payload (query + candidates) so the async callback can reconstruct state.
	payload := metadataReviewPayload{
		Query:  query,
		Inputs: candidates,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	// Calculate sub-batch count for the SubmitRequest.ItemCount.
	subBatchCount := (len(candidates) + metadataLLMBatchSize - 1) / metadataLLMBatchSize

	// Build function that constructs JSONL rows (one per sub-batch).
	buildFn := func(i int) (aijobs.BatchRequest, error) {
		start := i * metadataLLMBatchSize
		end := start + metadataLLMBatchSize
		if end > len(candidates) {
			end = len(candidates)
		}
		batch := candidates[start:end]

		batchPayload := struct {
			Query      MetadataLLMQuery       `json:"query"`
			Candidates []MetadataLLMCandidate `json:"candidates"`
		}{
			Query:      query,
			Candidates: batch,
		}
		batchJSON, err := json.Marshal(batchPayload)
		if err != nil {
			return aijobs.BatchRequest{}, fmt.Errorf("marshal sub-batch %d: %w", i, err)
		}

		userPrompt := fmt.Sprintf("Rank these candidate metadata search results against the query book:\n\n%s", string(batchJSON))

		body := map[string]any{
			"model": model,
			"messages": []map[string]any{
				{"role": "system", "content": metadataLLMSystemPrompt},
				{"role": "user", "content": userPrompt},
			},
			"max_completion_tokens": 8000,
			"response_format":       map[string]any{"type": "json_object"},
		}

		return aijobs.BatchRequest{Body: body, MaxTokens: 8000}, nil
	}

	return aijobs.Submit(ctx, deps, aijobs.SubmitRequest{
		Type:        "metadata_review",
		ItemCount:   subBatchCount,
		PayloadJSON: payloadJSON,
		Build:       buildFn,
	})
}

// metadataReviewCallback is the completion handler for metadata review batches.
// It deserializes the payload, parses scores, and applies via the injected applier.
func metadataReviewCallback(ctx context.Context, itemsJSON []byte, results []aijobs.RowResult) (successCount, errorCount int, rowErrors []database.AIJobRowError, fatalErr error) {
	// Deserialize the payload.
	var payload metadataReviewPayload
	if err := json.Unmarshal(itemsJSON, &payload); err != nil {
		return 0, 0, nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	// Process each row result.
	var allScores []MetadataLLMScore
	for _, r := range results {
		if r.Err != "" {
			errorCount++
			rowErrors = append(rowErrors, database.AIJobRowError{
				CustomID: r.CustomID,
				Error:    r.Err,
			})
			continue
		}

		// Parse the row's JSON content.
		var parsed struct {
			Scores []MetadataLLMScore `json:"scores"`
		}
		if err := json.Unmarshal([]byte(r.Content), &parsed); err != nil {
			errorCount++
			rowErrors = append(rowErrors, database.AIJobRowError{
				CustomID: r.CustomID,
				Error:    fmt.Sprintf("parse: %v", err),
			})
			continue
		}

		allScores = append(allScores, parsed.Scores...)
		successCount++
	}

	// Apply scores through the injected applier if available.
	if metadataReviewApplier != nil {
		applied := metadataReviewApplier.ApplyMetadataScores(payload.Query, allScores)
		log.Printf("[INFO] metadata_review callback: applied %d scores (from %d successful rows, %d errors)", applied, successCount, errorCount)
	}

	return successCount, errorCount, rowErrors, nil
}

// pollMetadataJobResults waits for a metadata review job to complete and returns scores.
// This is a blocking operation from the caller's perspective.
func (p *OpenAIParser) pollMetadataJobResults(ctx context.Context, jobID string) ([]MetadataLLMScore, error) {
	if p.aiJobsStore == nil {
		return nil, fmt.Errorf("aiJobsStore not configured")
	}

	// Poll for job completion (with timeout).
	maxAttempts := 600 // ~10 minutes at 1s intervals
	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
		}

		job, err := p.aiJobsStore.GetAIJob(jobID)
		if err != nil {
			log.Printf("metadata_review: poll job %s (attempt %d): %v", jobID, attempt+1, err)
			continue
		}

		if job.Status == "completed" {
			// Job is done, retrieve scores from the cache.
			cache := getMetadataScoreCache()
			key := job.ID
			if scores, ok := cache.Get(key); ok {
				cache.Delete(key)
				return scores, nil
			}
			// If cache miss, return error (callback may have failed or not run yet).
			return nil, fmt.Errorf("metadata job %s completed but no scores in cache", jobID)
		}

		if job.Status == "failed" {
			return nil, fmt.Errorf("metadata job %s failed: %s", jobID, job.ErrorMsg)
		}

		// Still pending, retry.
	}

	return nil, fmt.Errorf("metadata job %s did not complete within timeout", jobID)
}

// metadataScoreCache stores results from completed jobs temporarily.
// This allows the async callback to store results that are retrieved by pollMetadataJobResults.
type metadataScoreCache struct {
	mu      sync.Mutex
	results map[string][]MetadataLLMScore
}

var (
	metadataReviewApplier MetadataScoreApplier
	scoreCache            = &metadataScoreCache{results: make(map[string][]MetadataLLMScore)}
)

func getMetadataScoreCache() *metadataScoreCache {
	return scoreCache
}

func (c *metadataScoreCache) Set(key string, scores []MetadataLLMScore) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results[key] = scores
}

func (c *metadataScoreCache) Get(key string) ([]MetadataLLMScore, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	scores, ok := c.results[key]
	return scores, ok
}

func (c *metadataScoreCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.results, key)
}

// SetMetadataReviewApplier registers the optional sink for applying scores.
func SetMetadataReviewApplier(applier MetadataScoreApplier) {
	metadataReviewApplier = applier
}

// scoreMetadataBatchSync is the legacy synchronous fallback using direct API calls.
func (p *OpenAIParser) scoreMetadataBatchSync(
	ctx context.Context,
	query MetadataLLMQuery,
	batch []MetadataLLMCandidate,
) ([]MetadataLLMScore, error) {
	// This path is only used if aiJobsStore is not configured.
	// In production, this should not be reached.
	return nil, fmt.Errorf("scoreMetadataBatchSync not implemented; configure aiJobsStore")
}

// init registers the callback at package load time.
func init() {
	aijobs.Register("metadata_review", metadataReviewCallback)
}
