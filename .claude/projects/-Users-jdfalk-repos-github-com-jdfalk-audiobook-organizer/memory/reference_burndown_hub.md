---
name: burndown-tasks hub
description: Private GitHub repo that centralizes bot task specs across all jdfalk/* repos
type: reference
---

Bot tasks are now GitHub Issues at https://github.com/jdfalk/burndown-tasks (private).

**Labels:**
- `repo:audiobook-organizer` / `repo:migrate-loop` / `repo:ghcommon` — target repo routing
- `priority:high/normal/low` — dispatch order
- `status:ready` / `status:in-progress` / `status:blocked` / `status:needs-review`

**Adding a new target repo:**
```bash
gh label create "repo:<name>" --color "0075ca" --description "Target: jdfalk/<name>" --repo jdfalk/burndown-tasks
```

**Adding a task:** Open an issue with the full spec as the body, add `repo:<target>` + `priority:<level>` + `status:ready` labels.

**Never commit bot task specs to docs/superpowers/bot-tasks/ anymore** — that directory is now gitignored. All new tasks go directly to GitHub Issues in burndown-tasks.
