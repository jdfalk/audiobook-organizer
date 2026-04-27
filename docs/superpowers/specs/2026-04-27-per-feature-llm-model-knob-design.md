<!-- file: docs/superpowers/specs/2026-04-27-per-feature-llm-model-knob-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 8d3c1f4a-9e57-4b21-95c2-7a1d6f843901 -->

# Per-feature LLM Model Knob

**TODO ID:** AI-MODEL-1
**Audience:** human reviewer
**Companion bot recipe:** [`docs/superpowers/bot-tasks/2026-04-27-per-feature-llm-model-knob.md`](../bot-tasks/2026-04-27-per-feature-llm-model-knob.md)
**Size:** S — one PR, ~150 LOC.

## Problem

`"gpt-5-mini"` is a hardcoded string literal in two locations:

- `internal/openai/openai_parser.go` — every Parse* method (filename parse, cover-art author extract, batch parse) sends the same model in the request body.
- `internal/dedup/engine.go` — `RunLLMReview` duplicates the literal.

These four code paths have very different semantic difficulty:

| Path | Semantic difficulty | Cost sensitivity |
|---|---|---|
| Filename parse | low (regex-shaped task) | high — runs per book scanned |
| Series parse | low | high |
| Dedup pair review | high (judgment call) | low — runs at user request |
| Cover-art author extract | medium-high (vision, OCR-ish) | low — runs at user request |

A single knob forces us to either pay `mini` rates for the cheap paths or risk `nano` quality on the expensive ones. The OpenAI Batch API already gives us 50% off across the board, so cost differences here compound.

## Why this matters now (and not three months ago)

`config.Config` got a major rewrite in PR #472 (April 26): every JSON-tagged field is now persisted automatically with zero registration. Adding new config fields used to require touching three places; today it costs four lines per field. The friction that kept this on the backlog is gone.

## Design decisions

**Four knobs, not one with overrides.** A single `LLMModel` with per-feature override map (`map[string]string`) was tempting (DRY) but it makes the config screen weird ("I want to override X but not Y" requires partial-map UX) and it dodges the actual question — *every* feature should think about its model independently.

**Defaults preserve today's behavior.** Each new field defaults to `"gpt-5-mini"`. No silent quality drop on upgrade. Cost-sensitive defaults (e.g. `FilenameParseModel: "gpt-5-nano"`) are a separate tuning PR after a week of usage data.

**Dependency-injected, not env-var.** `NewOpenAIParser` already takes a config-shaped object via the server bootstrap. Adding another arg is mechanical. Env-var lookups inside the parser would bypass the unified config persistence and bleed leak through unit tests.

## Tradeoffs considered

| Alternative | Why rejected |
|---|---|
| Single `LLMModel` env var | Fights the new JSON config; no per-path knob means the cost-quality lever stays missing. |
| Per-call argument (caller picks model) | Explodes the API surface. Every caller now needs to know which model is right. Wrong layer. |
| Read model from request context | Magic. Hard to test. No precedent in the repo. |

## Out of scope

- **Cost tuning** — defaulting `FilenameParseModel` to `nano` belongs in a follow-up after telemetry shows the quality holds.
- **AI-BATCH-1 / AI-BATCH-2** — those are scanner / operation-loop refactors with their own deferral notes; orthogonal.
- **UI for these knobs** — the settings page already renders unknown JSON-tagged config fields generically (verify by loading the page; no code change expected).

## Risk

Low. Existing callers continue to work; the only behavior change is "the model name now goes through a getter." If a caller is missed during the sweep, the old hardcoded path is removed but the test suite covers the four production callsites.

## Bot recipe

The mechanical execution lives in [`docs/superpowers/bot-tasks/2026-04-27-per-feature-llm-model-knob.md`](../bot-tasks/2026-04-27-per-feature-llm-model-knob.md). That doc is the contract for the burndown bot — file paths, exact diffs, test commands, definition of done.
