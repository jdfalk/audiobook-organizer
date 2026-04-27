<!-- file: docs/superpowers/specs/2026-04-27-activity-batcher-followups-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 13b5d68c-026 7-5175-f283-567835f2cdde -->

# Activity Batcher — Follow-up Tasks

**TODO ID:** ACT-BATCH-FU
**Audience:** human reviewer
**Parent feature:** Activity batcher shipped April 27, 2026 (PRs #477–#481).
**Companion bot recipes:** see [Bot tasks](#bot-tasks).
**Size:** Each sub-task is S — one small PR.

## Status

The 15-second semantic batcher is in production. Scanner per-file flood logs are gone. Two follow-ups remain:

1. **No test for context-cancel flush.** The batcher promises that pending entries flush on shutdown. There's no automated proof.
2. **Scanner doesn't yet emit via `LogBatch`.** The `LogBatch` API (`internal/activity/`) exists per PR #481, but scanner.go still uses the old per-file `log.Printf` (the structured-batch API is opt-in and no callers have opted in yet).

These are mechanical follow-ups, perfect for the burndown bot.

## Why this matters

The cancel-flush guarantee is the harder one to mentally verify than to test. A regression here would silently drop the last 15 seconds of activity on shutdown — invisible until someone looks for it. A test pins the contract.

The scanner conversion is the *first* real consumer of the new structured-batch API. Until something uses it, the API has no real-world validation.

## Design decisions

**One PR per follow-up.** No reason to bundle.

**Scanner conversion is conservative.** Only swap log lines that produce per-file repetition (the 5,000-events-per-scan flood). One-shot logs (scan started, scan finished) stay as-is. The point of `LogBatch` is repetition collapse.

## Out of scope

- New batcher features (e.g. cross-key merging, dynamic window sizing). Future work.
- UI changes. Already shipped.

## Bot tasks {#bot-tasks}

| ID | Title | Bot recipe |
|---|---|---|
| **ACT-BATCH-FU-1** | LogBatch context-cancel flush test | [`bot-tasks/2026-04-27-activity-batcher-flush-test.md`](../bot-tasks/2026-04-27-activity-batcher-flush-test.md) |
| **ACT-BATCH-FU-2** | Convert scanner per-file logs to LogBatch | [`bot-tasks/2026-04-27-activity-batcher-scanner-convert.md`](../bot-tasks/2026-04-27-activity-batcher-scanner-convert.md) |
