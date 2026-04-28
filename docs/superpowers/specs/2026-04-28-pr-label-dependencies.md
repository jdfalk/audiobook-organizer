<!-- file: docs/superpowers/specs/2026-04-28-pr-label-dependencies.md -->
<!-- version: 1.0.0 -->
<!-- guid: b2c3d4e5-f6a7-8901-bcde-f23456789012 -->

# Design: PR Label Dependency System

**Status:** Spec written — applies to all burndown bot tasks going forward  
**Audience:** Burndown bot implementors, future spec writers

---

## Problem

Multi-wave work (like the ASYNC series) has ordering constraints: Wave 2 can't start
before Wave 1's foundation merges. We need a way to express and check these
dependencies across bots running on different machines.

**Rejected approach — status files in git:**  
Writing `docs/superpowers/status/ASYNC-CORE-1.json` and checking it in doesn't work
across concurrent bot instances. Bot A finishes and commits the file; Bot B starts
before pulling — it sees stale state. Race conditions and flaky dependencies.

**Chosen approach — GitHub PR labels:**  
GitHub's API is a shared database. Any machine querying
`gh pr list --label "task:ASYNC-CORE-1" --state merged` gets authoritative state
instantly, no pull required.

---

## Label Convention

Every bot-task PR gets a label applied when it's created:

```
task:ASYNC-CORE-1
task:ASYNC-W1-1
task:ASYNC-CLEAN-1
```

Format: `task:{TASK-ID}` where TASK-ID matches the `**TODO ID:**` field in the bot-task file.

Labels must exist in the repo before they can be applied. The bot creates them
if missing:

```bash
gh label create "task:ASYNC-CORE-1" --color "0075ca" --description "Bot task ASYNC-CORE-1" 2>/dev/null || true
```

---

## Bot-Task File Format

Each bot-task file MUST include a `## Prerequisites` section:

```markdown
## Prerequisites

None — this task has no dependencies.
```

or:

```markdown
## Prerequisites

All of the following PRs must be merged before starting this task:

- `task:ASYNC-CORE-1` — maintenance interface + registry
- `task:ASYNC-CORE-2` — dispatcher handler

Check with:
\`\`\`bash
gh pr list --label "task:ASYNC-CORE-1" --state merged --json number | jq 'length > 0'
gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length > 0'
\`\`\`

If either returns `false`, abort and try again later.
```

---

## Bot Behavior (prerequisite check)

At the TOP of every bot-task execution (before any file edits), the bot runs:

```bash
# For each prerequisite label:
READY=$(gh pr list --label "task:ASYNC-CORE-1" --state merged --json number | jq 'length > 0')
if [ "$READY" != "true" ]; then
  echo "Prerequisite task:ASYNC-CORE-1 not yet merged. Aborting."
  exit 0  # Exit 0 so the bot doesn't mark the task as failed
fi
```

---

## Bot Behavior (label creation + PR creation)

When the bot creates its PR:

```bash
# 1. Create label if missing
gh label create "task:ASYNC-W1-1" \
  --color "e4e669" \
  --description "Bot task: fix-read-by-narrator conversion" \
  2>/dev/null || true

# 2. Create PR with label
gh pr create \
  --title "feat(maintenance): convert fix-read-by-narrator to MaintenanceJob" \
  --body "..." \
  --label "task:ASYNC-W1-1"
```

---

## Checking All Prerequisites at Once

Helper script the bot can use:

```bash
check_prereqs() {
  local labels=("$@")
  for label in "${labels[@]}"; do
    count=$(gh pr list --label "$label" --state merged --json number | jq 'length')
    if [ "$count" -eq 0 ]; then
      echo "UNMET: $label"
      return 1
    fi
  done
  echo "ALL PREREQUISITES MET"
  return 0
}

check_prereqs "task:ASYNC-CORE-1" "task:ASYNC-CORE-2" || exit 0
```

---

## Discoverability

- `gh pr list --label "task:ASYNC-CORE-1"` shows all PRs for that task
- `gh label list | grep "^task:"` shows all task labels in the repo
- `gh pr list --label "task:ASYNC-CORE-1" --state merged` confirms completion

The GitHub UI shows labels on each PR card, making the dependency chain visible
without navigating to files.

---

## Applying This Pattern to Future Specs

For any multi-wave spec, the spec author should:

1. Assign a `TASK-ID` to each bot-task (e.g., `FEAT-X-1`, `FEAT-X-2`)
2. List dependencies in each bot-task's `## Prerequisites` section
3. Include the `gh label create` + `gh pr create --label` commands in the bot-task
4. Include the prerequisite check block at the start of the bot-task

No other coordination infrastructure needed.
