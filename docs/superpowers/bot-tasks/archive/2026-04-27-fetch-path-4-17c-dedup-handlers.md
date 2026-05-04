<!-- file: docs/superpowers/bot-tasks/2026-04-27-fetch-path-4-17c-dedup-handlers.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7f2a4c81-6d39-4e0b-97ad-ac9f1b283564 -->

# BOT TASK: 4.17c — Audit dedup handlers for service-delegation gaps

**TODO ID:** 4.17c
**Companion human design:** [`docs/superpowers/specs/2026-04-27-fetch-path-audit-design.md`](../specs/2026-04-27-fetch-path-audit-design.md)
**Pattern reference:** identical to [`4.17b`](2026-04-27-fetch-path-4-17b-metadata-handlers.md) — read it first.

This is an **audit-only, docs-only PR**.

## Branch

```
docs/4-17c-dedup-handlers-audit
```

## Files

Audit target — read every handler in:
- `internal/server/dedup_handlers.go` (and any other `dedup_*.go` in the server package)

Service to delegate to:
- `internal/dedup/engine.go` (`Engine` methods)

Append to (must already exist from 4.17b):
- `docs/audits/2026-04-27-handler-service-delegation.md` — append a `## dedup_handlers.go (4.17c)` section

## Step-by-step

Identical to 4.17b but for dedup. Read the 4.17b doc top-to-bottom, then:

1. List handlers: `grep -n "^func (s \*Server)" internal/server/dedup_handlers.go`
2. For each handler, verdict on whether it calls `Engine.*` methods or reimplements (e.g. inline scoring, inline LLM-review prompt building, inline cluster-walk).
3. Common divergences to flag:
   - Inline `gpt-5-*` model name (after AI-MODEL-1 ships, this becomes the dedup config getter)
   - Inline embedding-store calls when `Engine` already has a wrapper
   - Manual cluster-walk loops that duplicate `Engine.RunDedupScan`
4. Append the section to the audit file.
5. File `4.17e+` follow-ups for any divergences.

## Definition of done

- [ ] Audit file gains a populated `## dedup_handlers.go (4.17c)` section
- [ ] All handlers in the file represented
- [ ] Divergences → TODO entries
- [ ] PR title: `docs(audit): dedup handlers delegation audit (4.17c)`
- [ ] CHANGELOG prepended

## When to STOP

NEEDS_REVIEW if:

- The dedup engine API is genuinely thin (mostly utility functions) and "delegate vs reimplement" doesn't apply cleanly. Note this in the report and surface for human review.
- 4.17b's audit file does not exist. Tasks must run in order — surface the missing prerequisite.
