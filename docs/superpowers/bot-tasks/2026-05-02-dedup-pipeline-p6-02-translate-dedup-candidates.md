<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p6-02-translate-dedup-candidates.md -->
<!-- version: 1.0.0 -->
<!-- guid: 5ce543dd-4140-41e5-adb7-5c6cad9ed5b2 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Translate legacy dedup_candidates → dedup_match_groups

**Pipeline phase:** P6
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P2-02` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P2-02" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P2-02"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p6-02-translate-dedup-candidates
```

## Label

```bash
gh label create "task:PIPE-P6-02" --color "1d76db" --description "Bot task: Translate legacy dedup_candidates → dedup_match_groups" 2>/dev/null || true
```

## What This Does

Walks the legacy `dedup_candidates` table (`internal/database/embedding_store.go`)
and emits equivalent rows in `dedup_match_groups`:

- For every `pending` candidate: create a new open group with
  `strongest_kind = "embedding_similarity"` and the original similarity score.
- For every `merged` candidate: create a `state=merged` group.
- For every `dismissed` candidate: create a `state=dismissed` group.

The legacy table is **not** dropped here — it remains for Phase 6-3.

## Definition of Done

- [ ] Idempotent (uses a stable derived ID from the legacy candidate ID)
- [ ] Counts logged per status
- [ ] Spot-check: pick 10 legacy rows and confirm matching v2 rows exist


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): translate legacy dedup_candidates → dedup_match_groups (PIPE-P6-02)" \
  --body "Implements PIPE-P6-02 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P6-02"
```
