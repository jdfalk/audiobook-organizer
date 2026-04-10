// file: internal/ai/dedup_review.go
// version: 1.0.0
// guid: b2e7c3d1-4a58-4f96-9e0b-7d3a1c8f5b24

package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

// DedupEntity describes one side of a duplicate-pair candidate for LLM review.
// Fields are optional — empty strings are elided from the prompt.
type DedupEntity struct {
	ID       string `json:"id"`
	Title    string `json:"title,omitempty"`
	Author   string `json:"author,omitempty"`
	Narrator string `json:"narrator,omitempty"`
	Series   string `json:"series,omitempty"`
	ISBN     string `json:"isbn,omitempty"`
	ASIN     string `json:"asin,omitempty"`
	// For author entities, Title holds the author name and other fields are empty.
}

// DedupPairInput is one pair (A, B) the caller wants the LLM to adjudicate.
// The caller assigns Index so responses can be matched back.
type DedupPairInput struct {
	Index      int         `json:"index"`
	EntityType string      `json:"entity_type"` // "book" or "author"
	A          DedupEntity `json:"a"`
	B          DedupEntity `json:"b"`
	Similarity float64     `json:"similarity"`
}

// DedupPairVerdict is the LLM's judgment for a single pair.
type DedupPairVerdict struct {
	Index       int    `json:"index"`
	IsDuplicate bool   `json:"is_duplicate"`
	Confidence  string `json:"confidence"` // "high" | "medium" | "low"
	Reason      string `json:"reason"`
}

// dedupReviewBatchSize is the maximum number of pairs sent in one chat request.
// Chosen to stay well under output-token limits with typical per-pair payloads.
const dedupReviewBatchSize = 25

// ReviewDedupPairs sends batches of candidate pairs to OpenAI and returns a
// verdict for each. Results are keyed by Index so the caller can reassemble them
// in any order. Inputs are chunked internally; the caller passes all pairs at once.
func (p *OpenAIParser) ReviewDedupPairs(ctx context.Context, inputs []DedupPairInput) ([]DedupPairVerdict, error) {
	if !p.enabled {
		return nil, fmt.Errorf("OpenAI parser is not enabled")
	}
	if len(inputs) == 0 {
		return nil, nil
	}

	var all []DedupPairVerdict
	for start := 0; start < len(inputs); start += dedupReviewBatchSize {
		end := start + dedupReviewBatchSize
		if end > len(inputs) {
			end = len(inputs)
		}
		batch := inputs[start:end]

		verdicts, err := p.reviewDedupBatch(ctx, batch)
		if err != nil {
			return all, fmt.Errorf("dedup review batch [%d:%d]: %w", start, end, err)
		}
		all = append(all, verdicts...)
	}
	return all, nil
}

func (p *OpenAIParser) reviewDedupBatch(ctx context.Context, batch []DedupPairInput) ([]DedupPairVerdict, error) {
	systemPrompt := `You are an expert audiobook metadata reviewer. You will receive a batch of candidate duplicate pairs (either book pairs or author pairs). For each pair, decide whether A and B describe the same thing.

Rules for BOOK pairs:
- Same book: same title (allowing minor punctuation/subtitle differences), same primary author. Narrator differences are allowed — different editions or re-recordings of the same book are still the same book.
- Examples of SAME book: "The Way of Kings" vs "Stormlight Archive 1 - The Way of Kings"; "Dune" vs "Dune (Unabridged)"; same ISBN or ASIN.
- Examples of DIFFERENT books: same series but different volumes; same title but different authors; "Dune" vs "Dune Messiah".

Rules for AUTHOR pairs:
- Same author: the same real person with a name variant (initial spacing, suffix, pen name, case folding).
- Examples of SAME author: "J.R.R. Tolkien" vs "J. R. R. Tolkien"; "Mark Twain" vs "Samuel Clemens" (pen name, same person).
- Examples of DIFFERENT authors: two real people who happen to share initials; a compound entry like "V. A. Lewis, Azrie" is NOT a duplicate of "V. A. Lewis" — it's a compound that needs splitting, but for THIS task, mark it as not a duplicate.

Confidence:
- "high": obvious match or obvious non-match
- "medium": likely but some ambiguity
- "low": genuinely unsure

Return ONLY valid JSON in this exact shape:
{"verdicts": [{"index": N, "is_duplicate": true|false, "confidence": "high|medium|low", "reason": "brief one-sentence explanation"}]}

Include one verdict per input pair, using the same index as the input.`

	batchJSON, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("marshal batch: %w", err)
	}

	userPrompt := fmt.Sprintf("Review these candidate duplicate pairs:\n\n%s", string(batchJSON))

	jsonObjectFormat := shared.NewResponseFormatJSONObjectParam()

	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(attempt*attempt) * 2 * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		completion, err := p.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage(systemPrompt),
				openai.UserMessage(userPrompt),
			},
			Model:               shared.ChatModel(p.model),
			MaxCompletionTokens: param.NewOpt[int64](8000),
			PromptCacheKey:      param.NewOpt("audiobook-dedup-pair-review-v1"),
			ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
				OfJSONObject: &jsonObjectFormat,
			},
		})

		if err != nil {
			lastErr = fmt.Errorf("OpenAI API call failed (attempt %d): %w", attempt+1, err)
			continue
		}

		if len(completion.Choices) == 0 {
			lastErr = fmt.Errorf("no response from OpenAI (attempt %d)", attempt+1)
			continue
		}

		content := completion.Choices[0].Message.Content
		var result struct {
			Verdicts []DedupPairVerdict `json:"verdicts"`
		}
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			lastErr = fmt.Errorf("parse response (attempt %d): %w", attempt+1, err)
			continue
		}
		return result.Verdicts, nil
	}

	return nil, lastErr
}
