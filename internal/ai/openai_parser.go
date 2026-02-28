// file: internal/ai/openai_parser.go
// version: 1.4.0
// guid: 9a0b1c2d-3e4f-5a6b-7c8d-9e0f1a2b3c4d

package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

// ParsedMetadata represents structured metadata extracted from a filename
type ParsedMetadata struct {
	Title      string `json:"title"`
	Author     string `json:"author"`
	Series     string `json:"series,omitempty"`
	SeriesNum  int    `json:"series_number,omitempty"`
	Narrator   string `json:"narrator,omitempty"`
	Publisher  string `json:"publisher,omitempty"`
	Year       int    `json:"year,omitempty"`
	Confidence string `json:"confidence"` // high, medium, low
}

// OpenAIParser handles AI-powered metadata parsing using OpenAI
type OpenAIParser struct {
	client     *openai.Client
	model      string
	maxRetries int
	enabled    bool
}

// NewOpenAIParser creates a new OpenAI parser
func NewOpenAIParser(apiKey string, enabled bool) *OpenAIParser {
	if !enabled || apiKey == "" {
		return &OpenAIParser{enabled: false}
	}

	clientOptions := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		clientOptions = append(clientOptions, option.WithBaseURL(baseURL))
	}

	client := openai.NewClient(clientOptions...)

	return &OpenAIParser{
		client:     &client,
		model:      "gpt-4o-mini", // Fast and cost-effective
		maxRetries: 2,
		enabled:    true,
	}
}

// IsEnabled returns whether the parser is enabled
func (p *OpenAIParser) IsEnabled() bool {
	return p.enabled
}

// ParseFilename uses OpenAI to parse a filename into structured metadata
// Uses prompt caching by setting system prompt as a cached message
func (p *OpenAIParser) ParseFilename(ctx context.Context, filename string) (*ParsedMetadata, error) {
	if !p.enabled {
		return nil, fmt.Errorf("OpenAI parser is not enabled")
	}

	// Create the system prompt (will be cached by OpenAI)
	systemPrompt := `You are an expert at parsing audiobook filenames. Extract structured metadata from the filename.

Common patterns:
- "Title - Author" or "Author - Title"
- "Author - Series Name Book N - Title"
- "Title (Series Name #N)" or "Title (Series Name, Book N)"
- May include narrator: "Title - Author - Narrator"
- May include year: "Title (2020)" or "Title - Author (2020)"

Return ONLY valid JSON with these fields (omit if not found):
{
  "title": "book title",
  "author": "author name",
  "series": "series name",
  "series_number": 1,
  "narrator": "narrator name",
  "publisher": "publisher name",
  "year": 2020,
  "confidence": "high|medium|low"
}

Set confidence based on clarity of the filename structure.`

	// User prompt with the actual filename
	userPrompt := fmt.Sprintf("Parse this audiobook filename:\n\n%s", filename)

	// Create chat completion with response format for JSON
	jsonObjectFormat := shared.NewResponseFormatJSONObjectParam()

	completion, err := p.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Model:       shared.ChatModel(p.model),
		Temperature: param.NewOpt(0.1),
		MaxTokens:   param.NewOpt[int64](500),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &jsonObjectFormat,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("OpenAI API call failed: %w", err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	// Parse the JSON response
	content := completion.Choices[0].Message.Content
	return parseMetadataFromJSON(content)
}

// AudiobookContext provides rich context for AI parsing beyond just a filename.
type AudiobookContext struct {
	FilePath      string `json:"file_path"`                // Full path including folder hierarchy
	Title         string `json:"title,omitempty"`           // Existing title from DB
	AuthorName    string `json:"author_name,omitempty"`     // Existing author from DB
	Narrator      string `json:"narrator,omitempty"`        // Existing narrator from DB
	FileCount     int    `json:"file_count,omitempty"`      // Number of files in the book
	TotalDuration int    `json:"total_duration,omitempty"`  // Total duration in seconds
}

// ParseAudiobook uses OpenAI to parse audiobook metadata from rich context
// (full file path, existing metadata, file count, duration) rather than just a filename.
func (p *OpenAIParser) ParseAudiobook(ctx context.Context, abCtx AudiobookContext) (*ParsedMetadata, error) {
	if !p.enabled {
		return nil, fmt.Errorf("OpenAI parser is not enabled")
	}

	systemPrompt := `You are an expert at identifying audiobook metadata. Extract structured metadata from ALL available context: folder hierarchy, existing metadata, file details.

Key strategies:
- Folder paths often encode: /Author/Series/Title/ or /Author/Title/
- Existing metadata may be partially correct — improve or correct it
- Multi-file books (file_count > 1) are common; individual filenames like "01 Part 1.mp3" are not useful
- When author and narrator are the same, still include both
- If multiple authors or narrators, separate them with " & " (ampersand with spaces)

Return ONLY valid JSON with these fields (omit if not found):
{
  "title": "book title",
  "author": "author name (use ' & ' to separate multiple)",
  "series": "series name",
  "series_number": 1,
  "narrator": "narrator name (use ' & ' to separate multiple)",
  "publisher": "publisher name",
  "year": 2020,
  "confidence": "high|medium|low"
}

Set confidence based on how much context was available and how unambiguous it is.`

	// Build a rich user prompt with all available context
	userPrompt := fmt.Sprintf("Parse this audiobook's metadata from the following context:\n\nFull file path: %s", abCtx.FilePath)

	if abCtx.Title != "" {
		userPrompt += fmt.Sprintf("\nExisting title: %s", abCtx.Title)
	}
	if abCtx.AuthorName != "" {
		userPrompt += fmt.Sprintf("\nExisting author: %s", abCtx.AuthorName)
	}
	if abCtx.Narrator != "" {
		userPrompt += fmt.Sprintf("\nExisting narrator: %s", abCtx.Narrator)
	}
	if abCtx.FileCount > 0 {
		userPrompt += fmt.Sprintf("\nFile count: %d files", abCtx.FileCount)
	}
	if abCtx.TotalDuration > 0 {
		hours := abCtx.TotalDuration / 3600
		minutes := (abCtx.TotalDuration % 3600) / 60
		userPrompt += fmt.Sprintf("\nTotal duration: %dh %dm", hours, minutes)
	}

	jsonObjectFormat := shared.NewResponseFormatJSONObjectParam()

	completion, err := p.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Model:       shared.ChatModel(p.model),
		Temperature: param.NewOpt(0.1),
		MaxTokens:   param.NewOpt[int64](500),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &jsonObjectFormat,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("OpenAI API call failed: %w", err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	content := completion.Choices[0].Message.Content
	return parseMetadataFromJSON(content)
}

// ParseBatch parses multiple filenames in a single request (more efficient).
// Retries with exponential backoff on failure to handle rate limiting.
func (p *OpenAIParser) ParseBatch(ctx context.Context, filenames []string) ([]*ParsedMetadata, error) {
	if !p.enabled {
		return nil, fmt.Errorf("OpenAI parser is not enabled")
	}

	if len(filenames) == 0 {
		return []*ParsedMetadata{}, nil
	}

	// Limit batch size
	const maxBatchSize = 20
	if len(filenames) > maxBatchSize {
		filenames = filenames[:maxBatchSize]
	}

	systemPrompt := `You are an expert at parsing audiobook filenames. Extract structured metadata from each filename.

Common patterns:
- "Title - Author" or "Author - Title"
- "Author - Series Name Book N - Title"
- "Title (Series Name #N)" or "Title (Series Name, Book N)"
- May include narrator: "Title - Author - Narrator"
- May include year: "Title (2020)" or "Title - Author (2020)"

Return ONLY valid JSON with a "results" key containing an array with these fields for each file (omit if not found):
{"results": [
  {
    "title": "book title",
    "author": "author name",
    "series": "series name",
    "series_number": 1,
    "narrator": "narrator name",
    "publisher": "publisher name",
    "year": 2020,
    "confidence": "high|medium|low"
  }
]}

Set confidence based on clarity of the filename structure.`

	userPrompt := "Parse these audiobook filenames:\n\n"
	for i, filename := range filenames {
		userPrompt += fmt.Sprintf("%d. %s\n", i+1, filename)
	}

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
			Model:       shared.ChatModel(p.model),
			Temperature: param.NewOpt(0.1),
			MaxTokens:   param.NewOpt[int64](2000),
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
		return parseBatchMetadataFromJSON(content)
	}

	return nil, lastErr
}

// TestConnection tests the OpenAI API connection
func (p *OpenAIParser) TestConnection(ctx context.Context) error {
	if !p.enabled {
		return fmt.Errorf("OpenAI parser is not enabled")
	}

	// Set timeout for test
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Simple test parse
	_, err := p.ParseFilename(ctx, "The Hobbit - J.R.R. Tolkien")
	return err
}

// parseMetadataFromJSON parses a single metadata object from JSON string
func parseMetadataFromJSON(content string) (*ParsedMetadata, error) {
	var metadata ParsedMetadata
	if err := json.Unmarshal([]byte(content), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}
	return &metadata, nil
}

// parseBatchMetadataFromJSON parses batch results from JSON.
// Accepts either {"results": [...]} or a bare array [...].
func parseBatchMetadataFromJSON(content string) ([]*ParsedMetadata, error) {
	// Try wrapped format first (JSON Object mode returns objects)
	var wrapped struct {
		Results []*ParsedMetadata `json:"results"`
	}
	if err := json.Unmarshal([]byte(content), &wrapped); err == nil && len(wrapped.Results) > 0 {
		return wrapped.Results, nil
	}

	// Fall back to bare array
	var results []*ParsedMetadata
	if err := json.Unmarshal([]byte(content), &results); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}
	return results, nil
}
