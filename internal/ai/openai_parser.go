// file: internal/ai/openai_parser.go
// version: 1.2.0
// guid: 9a0b1c2d-3e4f-5a6b-7c8d-9e0f1a2b3c4d

package ai

import (
	"context"
	"encoding/json"
	"fmt"
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

	client := openai.NewClient(option.WithAPIKey(apiKey))

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
	var metadata ParsedMetadata
	content := completion.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}

	return &metadata, nil
}

// ParseBatch parses multiple filenames in a single request (more efficient)
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

	// Create the system prompt (will be cached by OpenAI)
	systemPrompt := `You are an expert at parsing audiobook filenames. Extract structured metadata from each filename.

Common patterns:
- "Title - Author" or "Author - Title"
- "Author - Series Name Book N - Title"
- "Title (Series Name #N)" or "Title (Series Name, Book N)"
- May include narrator: "Title - Author - Narrator"
- May include year: "Title (2020)" or "Title - Author (2020)"

Return ONLY valid JSON array with these fields for each file (omit if not found):
[
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
]

Set confidence based on clarity of the filename structure.`

	// Build user prompt with all filenames
	userPrompt := "Parse these audiobook filenames:\n\n"
	for i, filename := range filenames {
		userPrompt += fmt.Sprintf("%d. %s\n", i+1, filename)
	}

	// Create chat completion
	jsonObjectFormat := shared.NewResponseFormatJSONObjectParam()

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
		return nil, fmt.Errorf("OpenAI API call failed: %w", err)
	}

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	// Parse the JSON response
	var results []*ParsedMetadata
	content := completion.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), &results); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI response: %w", err)
	}

	return results, nil
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
