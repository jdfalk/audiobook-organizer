<!-- file: docs/superpowers/bot-tasks/2026-04-27-per-feature-llm-model-knob.md -->
<!-- version: 1.0.0 -->
<!-- guid: 9e4d2a5b-af68-4c32-86d3-8b2e7a954012 -->

# BOT TASK: Per-feature LLM Model Knob

**TODO ID:** AI-MODEL-1
**Audience:** burndown bot
**Companion human design:** [`docs/superpowers/specs/2026-04-27-per-feature-llm-model-knob-design.md`](../specs/2026-04-27-per-feature-llm-model-knob-design.md)

## Branch

```
fix/ai-model-1-per-feature-knob
```

## Files to edit

1. `internal/config/config.go`
2. `internal/openai/openai_parser.go`
3. `internal/dedup/engine.go`
4. `internal/openai/openai_parser_test.go`
5. Every callsite of `NewOpenAIParser(` (find with: `grep -rn "NewOpenAIParser(" --include="*.go"`)

## Step 1 — Add config fields

In `internal/config/config.go`, inside the `Config` struct, add **after** the existing OpenAI-related fields:

```go
// Per-feature OpenAI model selection. Default to gpt-5-mini for all four
// to preserve historical behavior. See spec docs/superpowers/specs/2026-04-27-per-feature-llm-model-knob-design.md.
DedupReviewModel    string `json:"dedup_review_model"    mapstructure:"dedup_review_model"`
MetadataReviewModel string `json:"metadata_review_model" mapstructure:"metadata_review_model"`
FilenameParseModel  string `json:"filename_parse_model"  mapstructure:"filename_parse_model"`
CoverArtModel       string `json:"cover_art_model"       mapstructure:"cover_art_model"`
```

In the same file, find the function that builds default config (look for `NewDefaultConfig`, `defaultConfig`, or the constructor that other fields use). Add these defaults next to the existing OpenAI ones:

```go
DedupReviewModel:    "gpt-5-mini",
MetadataReviewModel: "gpt-5-mini",
FilenameParseModel:  "gpt-5-mini",
CoverArtModel:       "gpt-5-mini",
```

Bump the file's `<!-- version: -->` header by minor version.

## Step 2 — Change `NewOpenAIParser` signature

In `internal/openai/openai_parser.go`:

**Before:**
```go
func NewOpenAIParser(apiKey, baseURL string) *OpenAIParser {
    return &OpenAIParser{
        apiKey:  apiKey,
        baseURL: baseURL,
        // ...
        defaultModel: "gpt-5-mini",
    }
}
```

**After:**
```go
func NewOpenAIParser(cfg *config.Config, apiKey, baseURL string) *OpenAIParser {
    return &OpenAIParser{
        cfg:     cfg,
        apiKey:  apiKey,
        baseURL: baseURL,
        // ...
    }
}
```

- Add an unexported `cfg *config.Config` field on the `OpenAIParser` struct.
- Delete the `defaultModel` field and any references.
- For each `Parse*` method that previously used `p.defaultModel`, replace with the appropriate getter:
  - `Parse(...)` (filename parsing) → `p.cfg.FilenameParseModel`
  - `ParseBatch(...)` → `p.cfg.FilenameParseModel`
  - `ParseCoverArt(...)` → `p.cfg.CoverArtModel`
  - any metadata-review path in this file → `p.cfg.MetadataReviewModel`
- Add `import "github.com/jdfalk/audiobook-organizer/internal/config"` if not present.

If a method is uncertain which knob to use, prefer `MetadataReviewModel`. Document the choice in a one-line comment.

## Step 3 — Update dedup engine

In `internal/dedup/engine.go`, function `RunLLMReview` (or whatever method literally embeds `"gpt-5-mini"`):

```go
// Before:
model := "gpt-5-mini"

// After:
model := e.cfg.DedupReviewModel
```

`Engine` already holds a `*config.Config` (verify with `grep -n "cfg" internal/dedup/engine.go`). If it does NOT, add it via the constructor — but the constructor likely already takes one, since dedup reads other config values today. **Do not invent a config injection if one doesn't exist** — stop and surface a NEEDS_REVIEW comment instead.

## Step 4 — Update callsites

Run:
```
grep -rn "NewOpenAIParser(" --include="*.go"
```

For every hit (likely 2–4 callsites in `internal/server/server.go` and tests), prepend the `cfg` argument. If the callsite already has a `cfg` in scope, use it. If not, plumb it through (the surrounding function almost certainly has access to `s.cfg` or similar).

## Step 5 — Tests

Add to `internal/openai/openai_parser_test.go`:

```go
func TestOpenAIParser_UsesConfiguredModels(t *testing.T) {
    cfg := &config.Config{
        FilenameParseModel:  "test-filename-model",
        CoverArtModel:       "test-cover-model",
        MetadataReviewModel: "test-metadata-model",
        DedupReviewModel:    "test-dedup-model",
    }
    // Use the existing mock-HTTP-client pattern in this file.
    // For each public Parse* method on OpenAIParser, capture the request body and
    // assert .model matches the configured value.
}
```

If the file has no existing mock-HTTP pattern, look at sibling tests (e.g. `internal/openai/*_test.go`) and copy the pattern. **Do not invent a new mocking framework.**

## Step 6 — Verify

Run, in order. Stop at the first failure and surface diagnostic:

```
go vet ./...
make test
make ci
```

Then run:
```
grep -rn '"gpt-5-mini"' internal/ | grep -v _test.go
```

This MUST return zero hits. If non-test code still has the literal, find the callsite and replace it. Test files MAY keep the literal (they're asserting against fixed expected values).

## Step 7 — Commit

```
feat(ai): per-feature LLM model knob (TODO AI-MODEL-1)

- Adds DedupReviewModel, MetadataReviewModel, FilenameParseModel, CoverArtModel
  to config.Config (defaults to gpt-5-mini, preserves existing behavior).
- Replaces hardcoded "gpt-5-mini" in openai_parser.go and dedup/engine.go
  with config getters.
- Tests assert each Parse* path reads the correct getter.

Spec: docs/superpowers/specs/2026-04-27-per-feature-llm-model-knob-design.md
```

## Definition of done

- [ ] `make ci` green
- [ ] `grep -rn '"gpt-5-mini"' internal/ | grep -v _test.go` → 0 hits
- [ ] PR opened with title `feat(ai): per-feature LLM model knob (TODO AI-MODEL-1)`
- [ ] CHANGELOG.md prepended with one-paragraph entry under `## [Unreleased]` matching commit body
- [ ] TODO.md `AI-MODEL-1` line flipped to `[x]`

## When to STOP and request review

Surface NEEDS_REVIEW with a comment if any of:

- `internal/dedup/engine.go` does not have a `*config.Config` already injected (Step 3 stop condition).
- A `NewOpenAIParser` callsite has no obvious `cfg` in scope (e.g. command-line tool with its own config struct).
- More than 6 callsites of `NewOpenAIParser` (the spec assumed ~3).
- Any test outside the listed files imports the parser — likely a contract test that needs care.

These all break the "mechanical" assumption. A human should look.
