<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p7-02-admin-opt-in-toggle.md -->
<!-- version: 1.0.0 -->
<!-- guid: bce3953e-a051-4c9d-ad59-8e95190d8498 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Settings UI + endpoint for auto-merge opt-in

**Pipeline phase:** P7
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P7-01` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P7-01" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P7-01"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p7-02-admin-opt-in-toggle
```

## Label

```bash
gh label create "task:PIPE-P7-02" --color "1d76db" --description "Bot task: Settings UI + endpoint for auto-merge opt-in" 2>/dev/null || true
```

## What This Does

Adds a single boolean toggle in the Settings → Dedup page that flips
`settings.dedup.auto_merge_enabled`. Adds a confirmation modal warning the
user that ≥0.90-score groups will be merged without per-action approval, and
that all merges are reversible via the match log.

## Definition of Done

- [ ] Toggle persists via existing `SettingsStore`
- [ ] Confirmation modal shown on enable
- [ ] Audit log entry written on toggle (existing `system_activity_log`)


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): settings ui + endpoint for auto-merge opt-in (PIPE-P7-02)" \
  --body "Implements PIPE-P7-02 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P7-02"
```
