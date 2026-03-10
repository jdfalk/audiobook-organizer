// file: internal/ai/openai_batch.go
// version: 1.1.0
// guid: b3c4d5e6-f7a8-9b0c-1d2e-3f4a5b6c7d8e

package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
)

// BatchRequest represents a single request in a JSONL batch file.
type BatchRequest struct {
	CustomID string                 `json:"custom_id"`
	Method   string                 `json:"method"`
	URL      string                 `json:"url"`
	Body     map[string]interface{} `json:"body"`
}

// BatchResponse represents a single response line from a batch result file.
type BatchResponse struct {
	ID       string `json:"id"`
	CustomID string `json:"custom_id"`
	Response struct {
		StatusCode int             `json:"status_code"`
		Body       json.RawMessage `json:"body"`
	} `json:"response"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// CreateBatchAuthorDedup creates a batch job for author dedup (full mode).
// Returns the batch ID for polling.
func (p *OpenAIParser) CreateBatchAuthorDedup(ctx context.Context, inputs []AuthorDiscoveryInput) (string, error) {
	if !p.enabled {
		return "", fmt.Errorf("OpenAI parser is not enabled")
	}
	if len(inputs) == 0 {
		return "", fmt.Errorf("no inputs provided")
	}

	// Build the system and user prompts (same as discoverAuthorBatch)
	systemPrompt := `You are an expert audiobook metadata reviewer. You will receive a list of authors with their IDs, book counts, and sample book titles. Find groups of authors that are likely the same person (different name formats, typos, abbreviations, last-name-first, etc).

CRITICAL RULES:
- COMPOUND NAMES: Many author entries contain multiple people separated by commas, ampersands, "and", or semicolons. When you find a compound entry that matches an individual author entry, suggest a merge with the individual as canonical.
- Use sample_titles to distinguish authors from narrators.
- NEVER merge two genuinely different people.
- Only merge when names clearly refer to the same person.
- If unsure, use action "skip".
- Identify narrators or publishers incorrectly listed as authors.
- INITIALS FORMATTING: Always use spaces after periods in initials: "C. B. Lee" not "C.B. Lee".
- PEN NAMES & ALIASES: When names are clearly pen names or handles, use action "alias" instead of "merge".

Return ONLY valid JSON: {"suggestions": [{"author_ids": [1, 42], "action": "merge|rename|split|skip|alias", "canonical_name": "Correct Name", "reason": "brief explanation", "confidence": "high|medium|low", "is_narrator": [ids], "is_publisher": [ids]}]}

Only include groups where you find actual duplicates or issues.`

	batchJSON, err := json.Marshal(inputs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal inputs: %w", err)
	}

	userPrompt := fmt.Sprintf("Find duplicate authors in this list:\n\n%s", string(batchJSON))

	// Build JSONL with a single request
	req := BatchRequest{
		CustomID: "author-dedup-full",
		Method:   "POST",
		URL:      "/v1/chat/completions",
		Body: map[string]interface{}{
			"model": p.model,
			"messages": []map[string]string{
				{"role": "system", "content": systemPrompt},
				{"role": "user", "content": userPrompt},
			},
			"max_completion_tokens": 16000,
			"response_format":      map[string]string{"type": "json_object"},
		},
	}

	var buf bytes.Buffer
	line, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal batch request: %w", err)
	}
	buf.Write(line)
	buf.WriteByte('\n')

	// Upload the JSONL file
	file, err := p.client.Files.New(ctx, openai.FileNewParams{
		File:    &buf,
		Purpose: openai.FilePurposeBatch,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload batch file: %w", err)
	}

	// Create the batch
	batch, err := p.client.Batches.New(ctx, openai.BatchNewParams{
		InputFileID:      file.ID,
		Endpoint:         openai.BatchNewParamsEndpointV1ChatCompletions,
		CompletionWindow: openai.BatchNewParamsCompletionWindow24h,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create batch: %w", err)
	}

	return batch.ID, nil
}

// CheckBatchStatus checks the status of a batch job.
func (p *OpenAIParser) CheckBatchStatus(ctx context.Context, batchID string) (string, string, error) {
	if !p.enabled {
		return "", "", fmt.Errorf("OpenAI parser is not enabled")
	}

	batch, err := p.client.Batches.Get(ctx, batchID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get batch: %w", err)
	}

	return string(batch.Status), batch.OutputFileID, nil
}

// CancelBatch cancels a pending or in-progress batch job.
func (p *OpenAIParser) CancelBatch(ctx context.Context, batchID string) error {
	if !p.enabled {
		return fmt.Errorf("OpenAI parser is not enabled")
	}

	_, err := p.client.Batches.Cancel(ctx, batchID)
	if err != nil {
		return fmt.Errorf("failed to cancel batch %s: %w", batchID, err)
	}
	return nil
}

// DownloadBatchResults downloads and parses results from a completed batch.
func (p *OpenAIParser) DownloadBatchResults(ctx context.Context, outputFileID string) ([]AuthorDiscoverySuggestion, error) {
	if !p.enabled {
		return nil, fmt.Errorf("OpenAI parser is not enabled")
	}

	resp, err := p.client.Files.Content(ctx, outputFileID)
	if err != nil {
		return nil, fmt.Errorf("failed to download batch results: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read batch results: %w", err)
	}

	var allSuggestions []AuthorDiscoverySuggestion

	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line
	for scanner.Scan() {
		var batchResp BatchResponse
		if err := json.Unmarshal(scanner.Bytes(), &batchResp); err != nil {
			continue
		}
		if batchResp.Error != nil {
			continue
		}
		if batchResp.Response.StatusCode != 200 {
			continue
		}

		// Parse the chat completion response
		var completion struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(batchResp.Response.Body, &completion); err != nil {
			continue
		}
		if len(completion.Choices) == 0 {
			continue
		}

		var result struct {
			Suggestions []AuthorDiscoverySuggestion `json:"suggestions"`
		}
		if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &result); err != nil {
			continue
		}
		allSuggestions = append(allSuggestions, result.Suggestions...)
	}

	return allSuggestions, nil
}

// CreateBatchAuthorReview creates a batch job for author dedup groups (groups mode).
// Returns the batch ID for polling.
func (p *OpenAIParser) CreateBatchAuthorReview(ctx context.Context, groups []AuthorDedupInput) (string, error) {
	if !p.enabled {
		return "", fmt.Errorf("OpenAI parser is not enabled")
	}
	if len(groups) == 0 {
		return "", fmt.Errorf("no groups provided")
	}

	// Reuse the same system prompt as reviewAuthorBatch
	systemPrompt := `You are an expert audiobook metadata reviewer. You will receive groups of potentially duplicate author names. For each group, determine the correct action:

- "merge": The variants are the same author with different name formats. Provide the correct canonical name.
- "split": The names represent different people incorrectly grouped together.
- "rename": The canonical name needs correction.
- "skip": The group is fine as-is or you're unsure.
- "reclassify": Entry is not an author at all (narrator/publisher misclassified as author).
- "alias": Pen names or stage names for the same person.

INITIALS FORMATTING: Always use spaces after periods in initials: "C. B. Lee" not "C.B. Lee".

Return ONLY valid JSON: {"suggestions": [{"group_index": N, "action": "merge|split|rename|skip|alias|reclassify", "canonical_name": "Correct Name", "reason": "brief explanation", "confidence": "high|medium|low", "is_narrator": [indices], "is_publisher": [indices]}]}`

	batchJSON, err := json.Marshal(groups)
	if err != nil {
		return "", fmt.Errorf("failed to marshal groups: %w", err)
	}

	userPrompt := fmt.Sprintf("Review these duplicate author groups:\n\n%s", string(batchJSON))

	req := BatchRequest{
		CustomID: "author-dedup-groups",
		Method:   "POST",
		URL:      "/v1/chat/completions",
		Body: map[string]interface{}{
			"model": p.model,
			"messages": []map[string]string{
				{"role": "system", "content": systemPrompt},
				{"role": "user", "content": userPrompt},
			},
			"max_completion_tokens": 32000,
			"response_format":      map[string]string{"type": "json_object"},
		},
	}

	var buf bytes.Buffer
	line, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal batch request: %w", err)
	}
	buf.Write(line)
	buf.WriteByte('\n')

	file, err := p.client.Files.New(ctx, openai.FileNewParams{
		File:    &buf,
		Purpose: openai.FilePurposeBatch,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload batch file: %w", err)
	}

	batch, err := p.client.Batches.New(ctx, openai.BatchNewParams{
		InputFileID:      file.ID,
		Endpoint:         openai.BatchNewParamsEndpointV1ChatCompletions,
		CompletionWindow: openai.BatchNewParamsCompletionWindow24h,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create batch: %w", err)
	}

	return batch.ID, nil
}

// DownloadBatchGroupsResults downloads and parses results from a completed groups batch.
func (p *OpenAIParser) DownloadBatchGroupsResults(ctx context.Context, outputFileID string) ([]AuthorDedupSuggestion, error) {
	if !p.enabled {
		return nil, fmt.Errorf("OpenAI parser is not enabled")
	}

	resp, err := p.client.Files.Content(ctx, outputFileID)
	if err != nil {
		return nil, fmt.Errorf("failed to download batch results: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read batch results: %w", err)
	}

	var allSuggestions []AuthorDedupSuggestion

	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		var batchResp BatchResponse
		if err := json.Unmarshal(scanner.Bytes(), &batchResp); err != nil {
			continue
		}
		if batchResp.Error != nil {
			continue
		}
		if batchResp.Response.StatusCode != 200 {
			continue
		}

		var completion struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(batchResp.Response.Body, &completion); err != nil {
			continue
		}
		if len(completion.Choices) == 0 {
			continue
		}

		var result struct {
			Suggestions []AuthorDedupSuggestion `json:"suggestions"`
		}
		if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &result); err != nil {
			continue
		}
		allSuggestions = append(allSuggestions, result.Suggestions...)
	}

	return allSuggestions, nil
}

// BuildBatchAuthorDedupRequest creates the messages for a batch-compatible request.
// Used by both real-time and batch paths.
func (p *OpenAIParser) BuildAuthorDedupMessages(inputs []AuthorDiscoveryInput) (system string, user string, model shared.ChatModel) {
	system = `You are an expert audiobook metadata reviewer. Find groups of authors that are likely the same person.

Return ONLY valid JSON: {"suggestions": [{"author_ids": [1, 42], "action": "merge|rename|split|skip|alias", "canonical_name": "Correct Name", "reason": "brief explanation", "confidence": "high|medium|low"}]}`

	batchJSON, _ := json.Marshal(inputs)
	user = fmt.Sprintf("Find duplicate authors in this list:\n\n%s", string(batchJSON))
	model = shared.ChatModel(p.model)
	return
}
