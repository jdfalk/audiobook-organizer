<!-- file: docs/superpowers/specs/2026-04-29-responses-api-migration-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 8f2a3b4c-5d6e-7f8a-9b0c-1d2e3f4a5b6c -->

# Migrate audiobook-organizer to the OpenAI Responses API

**Status:** Draft, awaiting human review before bot pickup
**TODO ID:** AI-RESP-1 (umbrella) + AI-RESP-1A..1F per phase
**Reference:** [OpenAI Responses migration guide](https://developers.openai.com/api/docs/guides/migrate-to-responses)

## Why now

`/v1/chat/completions` is in long-term maintenance; OpenAI is shipping
new models on `/v1/responses` first (codex-mini already requires it,
gpt-5.4 prefers it, and the o-series gives meaningfully better outputs
through Responses' first-class reasoning controls). We hit this in the
burndown bot — `gpt-5.1-codex-mini` returned `404 — model is only
supported in v1/responses` against Chat Completions.

Responses also gives us four mechanical wins that matter for this
codebase:

1. **Stateful threads via `PreviousResponseID`.** Several of our flows
   are de-facto multi-turn (the metadata-LLM-review reasoner, the
   bench-pass2 dialogue, dedup explanations). Today every "turn" sends
   the full conversation; Responses keeps history server-side. For our
   ~12K-book library this is real money saved on prompt tokens.
2. **Native structured output.** `response_format` becomes a typed
   JSON-schema input rather than the `{"type":"json_object"}` hint we
   pass today. Same outcome, less parsing-failure recovery code.
3. **Built-in tools.** `file_search`, `web_search`, `code_interpreter`
   ride the same call. Future work; not in scope here.
4. **`reasoning.effort` for o-series.** We use o3 / o4-mini for some
   batch jobs today and pay for default reasoning. With Responses we
   can dial it down for cheap bulk ops and up for the metadata
   reasoning paths.

## Scope (what this spec actually covers)

Six call-site clusters in `internal/ai/`. Sequenced from lowest-risk to
highest, so the bot can ship phases independently and we can soak each
before tackling the next.

### Phase A — `metadata_llm_review.go` (smallest, single-call)
`internal/ai/metadata_llm_review.go:153` — one `Chat.Completions.New`
inside `reviewMetadataCandidate`. JSON-object response, no tools.
Drop-in: swap to `Responses.New` with `response_format` schema, walk
`resp.OutputText`. Single function, no callers downstream care.

### Phase B — Triage / single-shot parsers in `openai_parser.go`
Six call sites; all single-turn JSON-shaped responses, mostly with
`response_format` already. Same drop-in pattern as Phase A, repeated.
The trickier ones are the loop variants (lines ~331, ~546, ~680) that
call in a fan-out — these are still single-turn per call, just
batched at the caller.

### Phase C — Embeddings (`embedding_client.go`)
`Embeddings.New` is **NOT** moving — that endpoint stays at
`/v1/embeddings` even after the Responses migration. **Out of scope.**
Document this explicitly in the spec so a bot doesn't try to migrate
it and break the embed pipeline.

### Phase D — Batches API (`openai_batch.go`)
The batch API submits a JSONL file where each line's `body:` is a Chat
Completions request body (line 110:
`"url": "/v1/chat/completions"`). OpenAI now supports `/v1/responses`
URLs in batches too (verify in the docs at submission time —
endpoint-allowlist may have rolled out by the time the bot picks this
up). When supported:
- Change the URL field in each line.
- Update the body schema from Chat Completions to Responses.
- Update the response-parsing in `decodeBatchOutput*` functions.

If the batch endpoint hasn't shipped Responses URL support yet, **skip
this phase** and revisit when it does. Our embedding-dedup batches and
the dedup-review batches are the affected jobs.

### Phase E — Multi-turn flows (`aijobs/aijobs.go` + bench)
The async-job runner in `internal/ai/aijobs/` orchestrates multi-step
conversations. This is where `PreviousResponseID` actually pays off.
Two changes:
1. Replace the message-array building with `PreviousResponseID`
   threading.
2. Persist `lastResponseID` in the job state so a restart resumes the
   thread.

### Phase F — Cleanup
After all phases are deployed and soaked for two weeks, delete the
old Chat Completions code paths.

## Out of scope

- Embeddings — stays on `/v1/embeddings`. (Phase C explicitly.)
- Image generation (DALL-E) — separate API, not affected.
- Whisper / TTS — separate APIs, not affected.
- Anthropic-side code paths — separate provider, separate spec.
- Adding the `web_search` / `file_search` builtin tools — useful but
  separate.

## API mapping cheat sheet

(See the burndown bot's spec at
[`overnight-burndown/docs/specs/2026-04-29-responses-api-migration.md`](https://github.com/jdfalk/overnight-burndown/blob/main/docs/specs/2026-04-29-responses-api-migration.md)
for the full Chat Completions ↔ Responses translation table — same
table applies here, no point duplicating.)

Audiobook-specific notes:

- Our `response_format: {"type":"json_object"}` calls translate to
  Responses' `response_format: {"type":"json_schema", "schema": ...}`
  with the schema declared inline. We already have Go structs for
  every response shape; emit the schema from the struct via
  `invopop/jsonschema` (already a dep — we use it for OpenAI tool
  defs). One helper `JSONSchemaFor[T]() openai.ResponseFormatJSONSchema`
  in `internal/ai/jsonschema.go` covers all six call sites.

- Our batch line bodies hard-code the URL and body shape. The bot
  must update the batch metadata helpers `batchMetadata` /
  `decodeBatchOutput*` together so a half-migrated batch doesn't
  produce parse errors mid-flight.

- The aijobs state schema needs a new column `last_response_id TEXT`
  (Pebble migration). Nullable; old jobs continue with empty string
  meaning "start a fresh thread on next call."

## Implementation plan (per-phase bot-tasks)

Each phase becomes a `docs/superpowers/bot-tasks/2026-04-29-ai-resp-N-*.md`
with:
- Branch name
- Files to modify (specific line ranges)
- Test additions (table-driven mock against
  `httptest.NewServer` intercepting the OpenAI base URL via
  `option.WithBaseURL`)
- Definition of Done
- Cited diffs from the migration guide for the specific call shape

The bot-task files are not in this spec — see the umbrella spec at
[`2026-04-29-ai-responses-umbrella.md`](2026-04-29-ai-responses-umbrella.md)
for the full execution plan and dependency order.

## Risk + rollback

- **Per-phase deployability.** Each phase is independently revertable
  via `git revert` of its merge commit. We don't introduce a
  feature-flag config knob for this — the surface is too spread out
  to gate cleanly without ugly conditionals.
- **Soak between phases.** Do NOT pick up phase N+1 before phase N
  has been on production for ≥3 days with no rollback. The OpenAI
  Responses API has subtle behavior diffs around tool-call output
  ordering and stop conditions that we want to surface one phase at
  a time.
- **JSON Schema vs json_object regressions.** The structured output
  on Responses is *stricter*. Any field we forgot to include in the
  schema will cause a 400. Mitigation: bot-task includes a "compare
  schema vs Go struct via reflection at build time" check.
- **Pebble migration backward compat.** The `last_response_id`
  column adds nullable; old job rows simply use empty string and
  start a fresh thread. No risk to existing data.

## Definition of Done (whole-spec)

- [ ] All six phases shipped + soaked.
- [ ] `internal/ai/jsonschema.go` helper used at every structured
  output call site.
- [ ] `aijobs.last_response_id` column populated for every new job.
- [ ] Telemetry: per-call token usage shows the cached-tokens
  breakdown that Responses exposes more granularly than Chat
  Completions did.
- [ ] CHANGELOG entries per phase.
- [ ] Old `Chat.Completions.New` call sites removed from
  `internal/ai/`.

## References

- OpenAI Responses migration guide:
  https://developers.openai.com/api/docs/guides/migrate-to-responses
- openai-go SDK Responses package — verify availability on the
  current `go.mod` version before phase A pickup.
- Burndown bot's parallel migration spec:
  https://github.com/jdfalk/overnight-burndown/blob/main/docs/specs/2026-04-29-responses-api-migration.md
