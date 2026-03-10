# AI Author Dedup Model Comparison Test Harness

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a standalone CLI test tool that runs the same author dedup data through multiple GPT models, prompt variations, and parameter tweaks, saving all raw request/response data for comparison analysis.

**Architecture:** A new `dedup-bench` cobra subcommand reads authors from the live database, constructs test configurations (model + prompt + params), sends each to OpenAI, saves raw JSON I/O to timestamped output directories, then generates a summary comparison report. Each "test run" is one model+prompt+params combination.

**Tech Stack:** Go, OpenAI Go SDK (`github.com/openai/openai-go`), existing `internal/ai` and `internal/database` packages, JSON file I/O.

---

## Test Matrix

### Dimension 1: Models (same prompt)
| Model | Notes |
|-------|-------|
| `gpt-4o` | Flagship, high quality |
| `gpt-4o-mini` | Cheap, fast |
| `gpt-5` | Latest flagship |
| `gpt-5-mini` | Currently used |
| `o3-mini` | Reasoning model, cheap |
| `o4-mini` | Newest reasoning model |

### Dimension 2: Prompt Variations (on best model from Dim 1)
| Variant | Description |
|---------|-------------|
| `baseline` | Current production prompt from `reviewAuthorBatch` / `discoverAuthorBatch` |
| `lookup` | Adds instruction to validate authors against known real-world data before deciding |
| `chain-of-thought` | Adds "Think step by step" reasoning instructions before JSON output |

### Dimension 3: Parameter Tweaks
| Param | Values |
|-------|--------|
| `temperature` | 0.0, 0.3, 0.7 (default is 1.0 for most models) |
| `top_p` | 0.5, 1.0 |

### Execution Strategy
- All runs use the **same frozen author data** (extracted once, saved to JSON)
- Both modes tested: `groups` (heuristic pre-grouped) and `full` (all authors)
- Each run saves: input JSON, raw API request params, raw API response, parsed suggestions, timing, token usage
- Low-priority batch API used where supported (models that support it) for cost savings
- Real-time API used as fallback

---

## Output Structure

```
testdata/dedup-bench/
  YYYY-MM-DDTHH-MM-SS/           # Run timestamp
    authors.json                   # Frozen author data (shared across runs)
    groups.json                    # Frozen heuristic groups (shared across runs)
    runs/
      gpt-5-mini_baseline_t0.0/
        config.json                # Model, prompt variant, params
        request.json               # Exact API request body
        response.json              # Raw API response
        suggestions.json           # Parsed suggestions
        stats.json                 # Timing, tokens, cost estimate
      gpt-4o_baseline_t0.0/
        ...
      gpt-5-mini_lookup_t0.0/
        ...
    summary.json                   # Cross-run comparison
    summary.md                     # Human-readable report
```

---

### Task 1: Create the dedup-bench command skeleton

**Files:**
- Create: `cmd/dedup_bench.go`

**Step 1: Write the command file**

```go
// file: cmd/dedup_bench.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/server"
	"github.com/spf13/cobra"
)

var dedupBenchCmd = &cobra.Command{
	Use:   "dedup-bench",
	Short: "Run AI author dedup comparison across models and prompts",
	Long: `Runs the same author data through multiple GPT models, prompt variations,
and parameter tweaks. Saves all raw request/response data for analysis.

Requires OPENAI_API_KEY or openai_api_key in config.`,
	RunE: runDedupBench,
}

var (
	benchOutputDir string
	benchModels    []string
	benchMode      string // "groups", "full", or "both"
	benchDryRun    bool
)

func init() {
	rootCmd.AddCommand(dedupBenchCmd)
	dedupBenchCmd.Flags().StringVar(&benchOutputDir, "output", "testdata/dedup-bench", "Output directory for results")
	dedupBenchCmd.Flags().StringSliceVar(&benchModels, "models", []string{
		"gpt-4o", "gpt-4o-mini", "gpt-5", "gpt-5-mini", "o3-mini", "o4-mini",
	}, "Models to test")
	dedupBenchCmd.Flags().StringVar(&benchMode, "mode", "both", "Mode: groups, full, or both")
	dedupBenchCmd.Flags().BoolVar(&benchDryRun, "dry-run", false, "Extract data only, don't call API")
}

func runDedupBench(cmd *cobra.Command, args []string) error {
	// Initialize config and database
	config.InitConfig(cfgFile)

	apiKey := config.AppConfig.OpenAIAPIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return fmt.Errorf("OpenAI API key required (set openai_api_key in config or OPENAI_API_KEY env var)")
	}

	if err := initializeStore(
		config.AppConfig.DatabaseType,
		config.AppConfig.DatabasePath,
		config.AppConfig.EnableSQLite,
	); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer closeStore()

	store := database.GlobalStore

	// Create timestamped output directory
	ts := time.Now().Format("2006-01-02T15-04-05")
	runDir := filepath.Join(benchOutputDir, ts)
	if err := os.MkdirAll(filepath.Join(runDir, "runs"), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	log.Printf("Dedup bench output: %s", runDir)

	// Step 1: Extract and freeze author data
	authorData, err := extractAuthorData(store)
	if err != nil {
		return fmt.Errorf("failed to extract author data: %w", err)
	}
	if err := writeJSON(filepath.Join(runDir, "authors.json"), authorData.Authors); err != nil {
		return err
	}

	// Step 2: Compute heuristic groups and freeze
	groups := server.FindDuplicateAuthors(authorData.Authors, authorData.BookCounts)
	if err := writeJSON(filepath.Join(runDir, "groups.json"), groups); err != nil {
		return err
	}

	log.Printf("Extracted %d authors, %d heuristic groups", len(authorData.Authors), len(groups))

	if benchDryRun {
		log.Println("Dry run — data extracted, skipping API calls")
		return nil
	}

	// Step 3: Build test configurations
	configs := buildTestConfigs(benchModels)

	// Step 4: Run each configuration
	var allResults []BenchRunResult
	for i, tc := range configs {
		log.Printf("[%d/%d] Running: model=%s prompt=%s temp=%.1f",
			i+1, len(configs), tc.Model, tc.PromptVariant, tc.Temperature)

		modes := []string{}
		switch benchMode {
		case "groups":
			modes = []string{"groups"}
		case "full":
			modes = []string{"full"}
		default:
			modes = []string{"groups", "full"}
		}

		for _, mode := range modes {
			result, err := executeBenchRun(cmd.Context(), apiKey, tc, authorData, groups, mode, runDir)
			if err != nil {
				log.Printf("  ERROR: %v", err)
				result = &BenchRunResult{
					Config: tc,
					Mode:   mode,
					Error:  err.Error(),
				}
			}
			allResults = append(allResults, *result)
		}
	}

	// Step 5: Generate summary
	summary := generateSummary(allResults)
	if err := writeJSON(filepath.Join(runDir, "summary.json"), summary); err != nil {
		return err
	}
	if err := writeSummaryMarkdown(filepath.Join(runDir, "summary.md"), summary); err != nil {
		return err
	}

	log.Printf("Benchmark complete. Results in %s", runDir)
	return nil
}
```

**Step 2: Verify it compiles (will fail — missing types/functions)**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go build ./cmd/...`
Expected: Compile errors for missing types/functions (expected at this stage)

**Step 3: Commit skeleton**

```bash
git add cmd/dedup_bench.go
git commit -m "feat: add dedup-bench command skeleton"
```

---

### Task 2: Add data extraction and types

**Files:**
- Create: `cmd/dedup_bench_types.go`

**Step 1: Write types and data extraction**

```go
// file: cmd/dedup_bench_types.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f23456789012

package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/server"
)

// TestConfig describes a single test run configuration.
type TestConfig struct {
	Model         string  `json:"model"`
	PromptVariant string  `json:"prompt_variant"` // baseline, lookup, chain-of-thought
	Temperature   float64 `json:"temperature"`
	TopP          float64 `json:"top_p"`
}

// BenchRunResult captures the outcome of a single test run.
type BenchRunResult struct {
	Config        TestConfig                     `json:"config"`
	Mode          string                         `json:"mode"` // groups or full
	DurationMs    int64                          `json:"duration_ms"`
	InputTokens   int64                          `json:"input_tokens"`
	OutputTokens  int64                          `json:"output_tokens"`
	CachedTokens  int64                          `json:"cached_tokens"`
	TotalTokens   int64                          `json:"total_tokens"`
	CostEstimate  float64                        `json:"cost_estimate_usd"`
	NumSuggestions int                           `json:"num_suggestions"`
	ActionCounts  map[string]int                 `json:"action_counts"`
	ConfidenceCounts map[string]int              `json:"confidence_counts"`
	Error         string                         `json:"error,omitempty"`
	Suggestions   []json.RawMessage              `json:"suggestions,omitempty"`
}

// BenchSummary is the cross-run comparison.
type BenchSummary struct {
	Timestamp    string            `json:"timestamp"`
	AuthorCount  int               `json:"author_count"`
	GroupCount   int               `json:"group_count"`
	Runs         []BenchRunResult  `json:"runs"`
}

// AuthorData holds the frozen author data for all runs.
type AuthorData struct {
	Authors    []database.Author `json:"authors"`
	BookCounts map[int]int       `json:"book_counts"`
	SampleBooks map[int][]string `json:"sample_books"` // authorID → up to 3 titles
}

// extractAuthorData loads all authors, book counts, and sample titles from the DB.
func extractAuthorData(store database.Store) (*AuthorData, error) {
	authors, err := store.GetAllAuthors()
	if err != nil {
		return nil, fmt.Errorf("GetAllAuthors: %w", err)
	}

	bookCounts, err := store.GetAllAuthorBookCounts()
	if err != nil {
		return nil, fmt.Errorf("GetAllAuthorBookCounts: %w", err)
	}

	sampleBooks := make(map[int][]string, len(authors))
	for _, a := range authors {
		books, err := store.GetBooksByAuthorIDWithRole(a.ID)
		if err != nil {
			continue
		}
		titles := make([]string, 0, 3)
		for i, b := range books {
			if i >= 3 {
				break
			}
			titles = append(titles, b.Title)
		}
		if len(titles) > 0 {
			sampleBooks[a.ID] = titles
		}
	}

	return &AuthorData{
		Authors:     authors,
		BookCounts:  bookCounts,
		SampleBooks: sampleBooks,
	}, nil
}

// buildGroupsInput converts heuristic groups to AI input format.
func buildGroupsInput(groups []server.AuthorDedupGroup, data *AuthorData) []ai.AuthorDedupInput {
	inputs := make([]ai.AuthorDedupInput, 0, len(groups))
	for i, g := range groups {
		variants := make([]string, 0, len(g.Variants))
		for _, v := range g.Variants {
			variants = append(variants, v.Name)
		}
		bc := 0
		if g.Canonical != nil {
			bc = data.BookCounts[g.Canonical.ID]
		}
		var samples []string
		if g.Canonical != nil {
			samples = data.SampleBooks[g.Canonical.ID]
		}
		input := ai.AuthorDedupInput{
			Index:         i,
			CanonicalName: g.Canonical.Name,
			VariantNames:  variants,
			BookCount:     bc,
			SampleTitles:  samples,
		}
		inputs = append(inputs, input)
	}
	return inputs
}

// buildFullInput converts all authors to AI discovery input format.
func buildFullInput(data *AuthorData) []ai.AuthorDiscoveryInput {
	inputs := make([]ai.AuthorDiscoveryInput, 0, len(data.Authors))
	for _, a := range data.Authors {
		inputs = append(inputs, ai.AuthorDiscoveryInput{
			ID:           a.ID,
			Name:         a.Name,
			BookCount:    data.BookCounts[a.ID],
			SampleTitles: data.SampleBooks[a.ID],
		})
	}
	return inputs
}

// buildTestConfigs creates the full matrix of test configurations.
func buildTestConfigs(models []string) []TestConfig {
	configs := []TestConfig{}

	// Dimension 1: All models with baseline prompt, default params
	for _, m := range models {
		configs = append(configs, TestConfig{
			Model:         m,
			PromptVariant: "baseline",
			Temperature:   0.0,
			TopP:          1.0,
		})
	}

	// Dimension 2: Best model candidates with prompt variations
	// (run all prompts on gpt-5-mini and gpt-5 to compare)
	promptModels := []string{"gpt-5-mini", "gpt-5"}
	for _, m := range promptModels {
		for _, pv := range []string{"lookup", "chain-of-thought"} {
			configs = append(configs, TestConfig{
				Model:         m,
				PromptVariant: pv,
				Temperature:   0.0,
				TopP:          1.0,
			})
		}
	}

	// Dimension 3: Temperature variations on gpt-5-mini baseline
	for _, temp := range []float64{0.3, 0.7} {
		configs = append(configs, TestConfig{
			Model:         "gpt-5-mini",
			PromptVariant: "baseline",
			Temperature:   temp,
			TopP:          1.0,
		})
	}

	return configs
}

// writeJSON marshals v to a JSON file at path.
func writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
```

**Step 2: Verify types are referenced correctly**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go vet ./cmd/...`
Expected: May still fail on missing `executeBenchRun`, `generateSummary`, etc.

**Step 3: Commit**

```bash
git add cmd/dedup_bench_types.go
git commit -m "feat: add dedup-bench types and data extraction"
```

---

### Task 3: Export FindDuplicateAuthors and AuthorDedupGroup

The heuristic grouping function and its return type are currently unexported or internal. We need to verify they're accessible from `cmd/`.

**Files:**
- Modify: `internal/server/author_dedup.go` (if types need exporting)

**Step 1: Check if AuthorDedupGroup is exported**

Run: `grep -n "type AuthorDedupGroup\|type authorDedupGroup" internal/server/author_dedup.go`

If `AuthorDedupGroup` and `FindDuplicateAuthors` are already exported (capital first letter), no changes needed — skip to commit.

If unexported, export them:
- Rename `authorDedupGroup` → `AuthorDedupGroup`
- Rename `findDuplicateAuthors` → `FindDuplicateAuthors`
- Update all internal references

**Step 2: Verify compilation**

Run: `go build ./...`
Expected: PASS

**Step 3: Commit (if changes made)**

```bash
git add internal/server/author_dedup.go
git commit -m "refactor: export AuthorDedupGroup and FindDuplicateAuthors"
```

---

### Task 4: Implement prompt variants

**Files:**
- Create: `cmd/dedup_bench_prompts.go`

**Step 1: Write prompt variant functions**

```go
// file: cmd/dedup_bench_prompts.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-345678901234

package cmd

// getGroupsSystemPrompt returns the system prompt for groups mode by variant.
func getGroupsSystemPrompt(variant string) string {
	base := `You are an expert audiobook metadata reviewer. You will receive groups of potentially duplicate author names. For each group, determine the correct action:

- "merge": The variants are the same author with different name formats. Provide the correct canonical name.
- "split": The names represent different people incorrectly grouped together.
- "rename": The canonical name needs correction (e.g., "TOLKIEN, J.R.R." → "J.R.R. Tolkien").
- "skip": The group is fine as-is or you're unsure.
- "reclassify": Entry is not an author at all (narrator/publisher misclassified as author).

INITIALS FORMATTING: Always use spaces after periods in initials: "C. B. Lee" not "C.B. Lee", "J. R. R. Tolkien" not "J.R.R. Tolkien".

PEN NAMES & ALIASES: When names are clearly pen names, handles, or stage names for the same person (e.g., "Mark Twain" / "Samuel Clemens"), use action "alias" instead of "merge".

COMPOUND ENTRIES WITH PUBLISHERS:
- "Graphic Audio [John Smith]" → Author: John Smith, Publisher: Graphic Audio
- "Full Cast Audio" → Publisher, not author. Use action "reclassify".

ROLE DECOMPOSITION: For every suggestion, populate the "roles" object to classify each name:
- "author": the actual book author with name variants
- "narrator": a voice actor identified by reading many different authors' books
- "publisher": a production company or publisher

Return ONLY valid JSON: {"suggestions": [{"group_index": N, "action": "merge|split|rename|skip|alias|reclassify", "canonical_name": "Correct Name", "reason": "brief explanation", "confidence": "high|medium|low", "roles": {"author": {"name": "Name", "variants": ["V1"], "reason": "why"}, "narrator": {"name": "Name", "ids": [indices], "reason": "why"}, "publisher": {"name": "Name", "ids": [indices], "reason": "why"}}}]}`

	switch variant {
	case "lookup":
		return base + `

VALIDATION STEP: Before making your final decision on each group, mentally verify:
1. Is the canonical name a real, known author? If you recognize them, use their most commonly published name.
2. For merges: are you confident both names refer to the same real person? Check if the sample book titles are consistent with a single author.
3. For renames: use the author's most widely recognized professional name format.
4. If a name could be either an author or narrator, check the sample titles — narrators tend to read books by many different authors across different genres.
Do NOT fabricate authors. If you don't recognize a name, base your decision purely on name similarity and the provided context.`

	case "chain-of-thought":
		return base + `

REASONING PROCESS: For each group, think through these steps before deciding:
1. List all names in the group and their structural differences (initials vs full, order, punctuation)
2. Check if sample titles suggest same author or different people
3. Consider if any name is a narrator or publisher rather than author
4. Decide the action and confidence level
5. Then output your JSON suggestion

Include your brief reasoning in the "reason" field.`

	default: // baseline
		return base
	}
}

// getFullSystemPrompt returns the system prompt for full mode by variant.
func getFullSystemPrompt(variant string) string {
	base := `You are an expert audiobook metadata reviewer. You will receive a list of authors with their IDs, book counts, and sample book titles. Find groups of authors that are likely the same person (different name formats, typos, abbreviations, last-name-first, etc).

CRITICAL RULES:
- COMPOUND NAMES: Many author entries contain multiple people separated by commas, ampersands, "and", or semicolons. When you find a compound entry that matches an individual author entry, suggest a merge with the individual as canonical.
- Use sample_titles to distinguish authors from narrators. A narrator reads many different authors' books.
- NEVER merge two genuinely different people.
- Only merge when names clearly refer to the same person.
- If unsure, use action "skip" — false negatives are far better than false positives.
- Identify narrators or publishers incorrectly listed as authors.
- INITIALS FORMATTING: Always use spaces after periods in initials: "C. B. Lee" not "C.B. Lee".
- PEN NAMES & ALIASES: When names are clearly pen names or handles, use action "alias" instead of "merge".

COMPOUND ENTRIES WITH PUBLISHERS:
- "Graphic Audio [John Smith]" → Author: John Smith, Publisher: Graphic Audio
- "Full Cast Audio" → Publisher, not author. Use action "reclassify".

ROLE DECOMPOSITION: For every suggestion, populate the "roles" object.

Return ONLY valid JSON: {"suggestions": [{"author_ids": [1, 42], "action": "merge|rename|split|skip|alias|reclassify", "canonical_name": "Correct Name", "reason": "brief explanation", "confidence": "high|medium|low", "roles": {"author": {"name": "Name", "variants": ["V1"], "reason": "why"}, "narrator": {"name": "Name", "ids": [ids], "reason": "why"}, "publisher": {"name": "Name", "ids": [ids], "reason": "why"}}}]}

Only include groups where you find actual duplicates or issues.`

	switch variant {
	case "lookup":
		return base + `

VALIDATION STEP: Before making your final decision on each group:
1. Is the canonical name a real, known author? Use their most commonly published name.
2. For merges: verify both names refer to the same real person using sample titles as evidence.
3. Check if any author is actually a narrator (reads many different authors) or publisher.
4. If a name could be two people (compound entry), verify by checking if sample titles span different genres/series.
Do NOT fabricate authors. Base decisions on name similarity and provided context.`

	case "chain-of-thought":
		return base + `

REASONING PROCESS: For each potential duplicate group:
1. Identify the structural differences between names (initials, order, punctuation, compound)
2. Cross-reference sample titles — do they suggest same author or different people?
3. Check for narrator/publisher misclassification
4. Assess confidence: high = obvious match, medium = likely but uncertain, low = possible
5. Output your JSON suggestion with reasoning in the "reason" field`

	default:
		return base
	}
}
```

**Step 2: Verify compilation**

Run: `go build ./cmd/...`
Expected: May still fail on missing `executeBenchRun` etc.

**Step 3: Commit**

```bash
git add cmd/dedup_bench_prompts.go
git commit -m "feat: add dedup-bench prompt variants (baseline, lookup, chain-of-thought)"
```

---

### Task 5: Implement the bench runner (API calls + raw I/O saving)

**Files:**
- Create: `cmd/dedup_bench_runner.go`

**Step 1: Write the runner**

```go
// file: cmd/dedup_bench_runner.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0123-defa-456789012345

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/ai"
	"github.com/jdfalk/audiobook-organizer/internal/server"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

// executeBenchRun runs a single model+prompt+params combination and saves results.
func executeBenchRun(
	ctx context.Context,
	apiKey string,
	tc TestConfig,
	data *AuthorData,
	groups []server.AuthorDedupGroup,
	mode string,
	runDir string,
) (*BenchRunResult, error) {
	// Create run output directory
	dirName := fmt.Sprintf("%s_%s_t%.1f", tc.Model, tc.PromptVariant, tc.Temperature)
	if mode == "full" {
		dirName += "_full"
	}
	outDir := filepath.Join(runDir, "runs", dirName)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	// Save config
	if err := writeJSON(filepath.Join(outDir, "config.json"), tc); err != nil {
		return nil, err
	}

	// Build the prompt
	var systemPrompt, userPrompt string
	var inputData interface{}

	if mode == "groups" {
		systemPrompt = getGroupsSystemPrompt(tc.PromptVariant)
		inputs := buildGroupsInput(groups, data)
		inputData = inputs
		batchJSON, _ := json.Marshal(inputs)
		userPrompt = fmt.Sprintf("Review these duplicate author groups:\n\n%s", string(batchJSON))
	} else {
		systemPrompt = getFullSystemPrompt(tc.PromptVariant)
		inputs := buildFullInput(data)
		inputData = inputs
		batchJSON, _ := json.Marshal(inputs)
		userPrompt = fmt.Sprintf("Find duplicate authors in this list:\n\n%s", string(batchJSON))
	}

	// Save the request
	request := map[string]interface{}{
		"model":          tc.Model,
		"prompt_variant": tc.PromptVariant,
		"temperature":    tc.Temperature,
		"top_p":          tc.TopP,
		"mode":           mode,
		"system_prompt":  systemPrompt,
		"user_prompt":    userPrompt,
		"input_data":     inputData,
	}
	if err := writeJSON(filepath.Join(outDir, "request.json"), request); err != nil {
		return nil, err
	}

	// Create OpenAI client
	clientOpts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(baseURL))
	}
	client := openai.NewClient(clientOpts...)

	// Build API request params
	maxTokens := int64(32000)
	if mode == "full" {
		maxTokens = 16000
	}

	jsonObjectFormat := shared.NewResponseFormatJSONObjectParam()
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		Model:               shared.ChatModel(tc.Model),
		MaxCompletionTokens: param.NewOpt(maxTokens),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &jsonObjectFormat,
		},
	}

	// Set temperature and top_p (reasoning models like o3/o4 don't support these)
	isReasoningModel := strings.HasPrefix(tc.Model, "o3") || strings.HasPrefix(tc.Model, "o4") || strings.HasPrefix(tc.Model, "o1")
	if !isReasoningModel {
		params.Temperature = param.NewOpt(tc.Temperature)
		params.TopP = param.NewOpt(tc.TopP)
	}

	// Call the API
	start := time.Now()
	completion, err := client.Chat.Completions.New(ctx, params)
	elapsed := time.Since(start)

	if err != nil {
		// Save the error
		errResult := map[string]interface{}{
			"error":       err.Error(),
			"duration_ms": elapsed.Milliseconds(),
		}
		_ = writeJSON(filepath.Join(outDir, "response.json"), errResult)
		return nil, fmt.Errorf("API call failed: %w", err)
	}

	// Save raw response
	rawResp, _ := json.Marshal(completion)
	if err := os.WriteFile(filepath.Join(outDir, "response.json"), rawResp, 0644); err != nil {
		return nil, err
	}

	// Parse suggestions
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	content := completion.Choices[0].Message.Content

	// Save parsed suggestions
	var suggestionsRaw json.RawMessage
	if mode == "groups" {
		var result struct {
			Suggestions []ai.AuthorDedupSuggestion `json:"suggestions"`
		}
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			log.Printf("  Warning: failed to parse suggestions: %v", err)
			_ = writeJSON(filepath.Join(outDir, "suggestions.json"), map[string]string{
				"error":   err.Error(),
				"raw":     content,
			})
		} else {
			suggestionsRaw, _ = json.Marshal(result.Suggestions)
			_ = writeJSON(filepath.Join(outDir, "suggestions.json"), result.Suggestions)
		}
	} else {
		var result struct {
			Suggestions []ai.AuthorDiscoverySuggestion `json:"suggestions"`
		}
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			log.Printf("  Warning: failed to parse suggestions: %v", err)
			_ = writeJSON(filepath.Join(outDir, "suggestions.json"), map[string]string{
				"error":   err.Error(),
				"raw":     content,
			})
		} else {
			suggestionsRaw, _ = json.Marshal(result.Suggestions)
			_ = writeJSON(filepath.Join(outDir, "suggestions.json"), result.Suggestions)
		}
	}

	// Compute stats
	usage := completion.Usage
	actionCounts := countActions(content, mode)
	confidenceCounts := countConfidence(content, mode)
	numSuggestions := countSuggestions(content)

	costEstimate := estimateCost(tc.Model, usage.PromptTokens, usage.CompletionTokens, usage.PromptTokensDetails.CachedTokens)

	stats := BenchRunResult{
		Config:           tc,
		Mode:             mode,
		DurationMs:       elapsed.Milliseconds(),
		InputTokens:      usage.PromptTokens,
		OutputTokens:     usage.CompletionTokens,
		CachedTokens:     usage.PromptTokensDetails.CachedTokens,
		TotalTokens:      usage.TotalTokens,
		CostEstimate:     costEstimate,
		NumSuggestions:   numSuggestions,
		ActionCounts:     actionCounts,
		ConfidenceCounts: confidenceCounts,
		Suggestions:      []json.RawMessage{suggestionsRaw},
	}

	if err := writeJSON(filepath.Join(outDir, "stats.json"), stats); err != nil {
		return nil, err
	}

	log.Printf("  Done: %dms, %d suggestions, ~$%.4f",
		elapsed.Milliseconds(), numSuggestions, costEstimate)

	return &stats, nil
}

// countActions parses suggestions JSON to count actions.
func countActions(content string, mode string) map[string]int {
	counts := map[string]int{}
	if mode == "groups" {
		var r struct{ Suggestions []struct{ Action string } }
		if json.Unmarshal([]byte(content), &r) == nil {
			for _, s := range r.Suggestions {
				counts[s.Action]++
			}
		}
	} else {
		var r struct{ Suggestions []struct{ Action string } }
		if json.Unmarshal([]byte(content), &r) == nil {
			for _, s := range r.Suggestions {
				counts[s.Action]++
			}
		}
	}
	return counts
}

// countConfidence parses suggestions JSON to count confidence levels.
func countConfidence(content string, mode string) map[string]int {
	counts := map[string]int{}
	var r struct{ Suggestions []struct{ Confidence string } }
	if json.Unmarshal([]byte(content), &r) == nil {
		for _, s := range r.Suggestions {
			counts[s.Confidence]++
		}
	}
	return counts
}

// countSuggestions returns the number of suggestions in the response.
func countSuggestions(content string) int {
	var r struct{ Suggestions []json.RawMessage }
	if json.Unmarshal([]byte(content), &r) == nil {
		return len(r.Suggestions)
	}
	return 0
}

// estimateCost estimates the USD cost of a run based on model pricing.
// Prices as of March 2026 — approximate.
func estimateCost(model string, inputTokens, outputTokens, cachedTokens int64) float64 {
	// Per-million token pricing (input, output, cached-input)
	type pricing struct{ input, output, cached float64 }
	prices := map[string]pricing{
		"gpt-4o":      {2.50, 10.00, 1.25},
		"gpt-4o-mini": {0.15, 0.60, 0.075},
		"gpt-5":       {10.00, 30.00, 2.50},  // estimate
		"gpt-5-mini":  {0.30, 1.25, 0.15},    // estimate
		"o3-mini":     {1.10, 4.40, 0.55},
		"o4-mini":     {1.10, 4.40, 0.55},
	}

	p, ok := prices[model]
	if !ok {
		p = pricing{5.0, 15.0, 2.5} // fallback estimate
	}

	uncachedInput := inputTokens - cachedTokens
	cost := float64(uncachedInput)/1_000_000*p.input +
		float64(cachedTokens)/1_000_000*p.cached +
		float64(outputTokens)/1_000_000*p.output

	return cost
}
```

**Step 2: Verify compilation**

Run: `go build ./cmd/...`
Expected: May fail on `server.AuthorDedupGroup` or `generateSummary`/`writeSummaryMarkdown`

**Step 3: Commit**

```bash
git add cmd/dedup_bench_runner.go
git commit -m "feat: add dedup-bench runner with raw I/O saving"
```

---

### Task 6: Implement summary generation

**Files:**
- Create: `cmd/dedup_bench_summary.go`

**Step 1: Write summary generator**

```go
// file: cmd/dedup_bench_summary.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-1234-efab-567890123456

package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// generateSummary creates a cross-run comparison from all results.
func generateSummary(results []BenchRunResult) *BenchSummary {
	return &BenchSummary{
		Timestamp: time.Now().Format(time.RFC3339),
		Runs:      results,
	}
}

// writeSummaryMarkdown generates a human-readable markdown report.
func writeSummaryMarkdown(path string, summary *BenchSummary) error {
	var sb strings.Builder

	sb.WriteString("# AI Author Dedup Benchmark Results\n\n")
	sb.WriteString(fmt.Sprintf("**Run:** %s\n\n", summary.Timestamp))

	// Group by mode
	groupsRuns := []BenchRunResult{}
	fullRuns := []BenchRunResult{}
	for _, r := range summary.Runs {
		if r.Error != "" {
			continue
		}
		if r.Mode == "groups" {
			groupsRuns = append(groupsRuns, r)
		} else {
			fullRuns = append(fullRuns, r)
		}
	}

	for _, mode := range []string{"groups", "full"} {
		runs := groupsRuns
		if mode == "full" {
			runs = fullRuns
		}
		if len(runs) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("## %s Mode\n\n", strings.Title(mode)))

		// Sort by cost
		sort.Slice(runs, func(i, j int) bool {
			return runs[i].CostEstimate < runs[j].CostEstimate
		})

		// Table header
		sb.WriteString("| Model | Prompt | Temp | Suggestions | Merge | Split | Rename | Skip | Alias | Reclassify | Duration | Tokens (in/out) | Cost |\n")
		sb.WriteString("|-------|--------|------|-------------|-------|-------|--------|------|-------|------------|----------|-----------------|------|\n")

		for _, r := range runs {
			sb.WriteString(fmt.Sprintf("| %s | %s | %.1f | %d | %d | %d | %d | %d | %d | %d | %dms | %d/%d | $%.4f |\n",
				r.Config.Model,
				r.Config.PromptVariant,
				r.Config.Temperature,
				r.NumSuggestions,
				r.ActionCounts["merge"],
				r.ActionCounts["split"],
				r.ActionCounts["rename"],
				r.ActionCounts["skip"],
				r.ActionCounts["alias"],
				r.ActionCounts["reclassify"],
				r.DurationMs,
				r.InputTokens,
				r.OutputTokens,
				r.CostEstimate,
			))
		}

		sb.WriteString("\n")

		// Confidence breakdown
		sb.WriteString("### Confidence Distribution\n\n")
		sb.WriteString("| Model | Prompt | High | Medium | Low |\n")
		sb.WriteString("|-------|--------|------|--------|-----|\n")
		for _, r := range runs {
			sb.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %d |\n",
				r.Config.Model,
				r.Config.PromptVariant,
				r.ConfidenceCounts["high"],
				r.ConfidenceCounts["medium"],
				r.ConfidenceCounts["low"],
			))
		}
		sb.WriteString("\n")
	}

	// Error summary
	errorRuns := []BenchRunResult{}
	for _, r := range summary.Runs {
		if r.Error != "" {
			errorRuns = append(errorRuns, r)
		}
	}
	if len(errorRuns) > 0 {
		sb.WriteString("## Errors\n\n")
		for _, r := range errorRuns {
			sb.WriteString(fmt.Sprintf("- **%s** (%s, %s): %s\n",
				r.Config.Model, r.Config.PromptVariant, r.Mode, r.Error))
		}
		sb.WriteString("\n")
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}
```

**Step 2: Full compilation check**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go build ./...`
Expected: PASS (or minor fixups needed for server.AuthorDedupGroup export)

**Step 3: Commit**

```bash
git add cmd/dedup_bench_summary.go
git commit -m "feat: add dedup-bench summary report generation"
```

---

### Task 7: Verify AuthorDedupGroup is accessible and fix compilation

**Files:**
- Modify: `internal/server/author_dedup.go` (if needed)
- Modify: any `cmd/dedup_bench*.go` files that have compile errors

**Step 1: Attempt full build**

Run: `go build ./...`

**Step 2: Fix any compilation errors**

Common issues:
- `server.AuthorDedupGroup` — verify the type name and fields (`Canonical`, `Variants`)
- `server.FindDuplicateAuthors` — verify signature matches
- Reasoning models (o3/o4) may not support `response_format` JSON mode — add a check
- `strings.Title` is deprecated — use `cases.Title` from `golang.org/x/text`

Fix each error, re-run `go build ./...` until clean.

**Step 3: Commit fixes**

```bash
git add -A
git commit -m "fix: resolve dedup-bench compilation issues"
```

---

### Task 8: Add testdata to .gitignore

**Files:**
- Modify: `.gitignore`

**Step 1: Add testdata/dedup-bench to gitignore**

Add this line to `.gitignore`:
```
testdata/dedup-bench/
```

**Step 2: Commit**

```bash
git add .gitignore
git commit -m "chore: gitignore dedup-bench output"
```

---

### Task 9: Run the benchmark (dry run first)

**Step 1: Dry run to extract data**

Run: `go run . dedup-bench --dry-run`
Expected: Should extract authors and groups, save JSON files, then exit without API calls.

**Step 2: Verify output files**

Run: `ls -la testdata/dedup-bench/*/`
Expected: `authors.json` and `groups.json` exist with content.

Run: `wc -l testdata/dedup-bench/*/authors.json`
Expected: Shows number of authors extracted.

**Step 3: Run actual benchmark (groups mode first, cheaper)**

Run: `go run . dedup-bench --mode groups 2>&1 | tee testdata/dedup-bench-log.txt`

This will take a while (6+ model calls). Watch for:
- Each run logging `Done: Xms, Y suggestions, ~$Z.ZZZZ`
- Any errors (model not available, rate limits, etc.)

**Step 4: If any models fail, update the model list**

Some models may not be available or may not support JSON mode. Remove failing models from the default list in `cmd/dedup_bench.go` and re-run.

**Step 5: Run full mode**

Run: `go run . dedup-bench --mode full 2>&1 | tee -a testdata/dedup-bench-log.txt`

**Step 6: Review results**

Run: `cat testdata/dedup-bench/*/summary.md`

This shows the comparison table across all models, prompts, and parameters.

---

### Task 10: Analyze results and iterate

**Step 1: Review the summary markdown**

Look at:
- Which model produces the most suggestions?
- Which has the highest merge/rename ratio vs skip?
- Confidence distribution — more high-confidence = better
- Cost per run — find the sweet spot of quality vs cost
- Duration — is any model unreasonably slow?

**Step 2: Compare suggestion quality**

For each model's groups mode run, diff the suggestions:
```bash
# Compare two models' suggestions side-by-side
diff <(jq -r '.[] | "\(.group_index) \(.action) \(.canonical_name)"' testdata/dedup-bench/*/runs/gpt-5-mini_baseline_t0.0/suggestions.json) \
     <(jq -r '.[] | "\(.group_index) \(.action) \(.canonical_name)"' testdata/dedup-bench/*/runs/gpt-5_baseline_t0.0/suggestions.json)
```

**Step 3: Document findings in the summary**

After analyzing, add notes to the summary.md about which model/prompt/params combination works best.

---

## Notes for the Implementer

1. **OpenAI SDK version**: The project uses `github.com/openai/openai-go v1.12.0`. The `param.NewOpt` function is used for optional parameters. Check the SDK docs if you hit type issues.

2. **Reasoning models** (o3-mini, o4-mini): These may not support `temperature`, `top_p`, or `response_format: json_object`. The runner already handles temperature/top_p, but you may need to remove `ResponseFormat` for these models and parse JSON from markdown code blocks instead.

3. **Rate limits**: If you hit rate limits, add a sleep between runs (e.g., `time.Sleep(5 * time.Second)` between each `executeBenchRun` call).

4. **Cost safety**: The `--dry-run` flag extracts data without API calls. Use it first. The full matrix with both modes is ~24 API calls. At ~$0.01-0.10 per call for mini models, total cost should be under $5. The gpt-5 calls will be more expensive (~$0.50-1.00 each).

5. **`FindDuplicateAuthors` signature**: Check `internal/server/author_dedup.go` for the exact signature. It likely takes `([]database.Author, map[int]int)` and returns `[]AuthorDedupGroup`. The `AuthorDedupGroup` struct has a `Canonical *database.Author` and `Variants []database.Author` (or similar). Adjust `buildGroupsInput` accordingly.
