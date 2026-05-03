<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p7-01-trust-ladder-runner.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7abef450-c31f-48b2-b2b9-c19230e0d53e -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Trust-ladder runner: emits suggestions / auto-merges per match group

**Pipeline phase:** P7
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P3-01` — must be merged before this task starts
- `task:P4-01` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P3-01" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P3-01"; exit 0; }
count=$(gh pr list --label "task:P4-01" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P4-01"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p7-01-trust-ladder-runner
```

## Label

```bash
gh label create "task:PIPE-P7-01" --color "1d76db" --description "Bot task: Trust-ladder runner: emits suggestions / auto-merges per match group" 2>/dev/null || true
```

## What This Does

A new job `trust-ladder` that walks open match groups and:

- 0.50 ≤ score < 0.75: emit a "needs review" notification via realtime hub.
- 0.75 ≤ score < 0.90: emit a "default-yes" notification.
- score ≥ 0.90: if `settings.dedup.auto_merge_enabled` is true, call the
  v2 merge endpoint internally with `decided_by = "auto"` and write a
  `LogMatch` entry with `decision = "auto-merge"`. Otherwise emit a "would
  auto-merge" notification only.

## Definition of Done

- [ ] Auto-merge path is gated by the global setting (default off)
- [ ] Every action persists a `fingerprint_match_log` row for forensics
- [ ] Test: with the setting on, score 0.95 group results in members merged


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): trust-ladder runner: emits suggestions / auto-merges per match group (PIPE-P7-01)" \
  --body "Implements PIPE-P7-01 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P7-01"
```
