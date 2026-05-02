<!-- file: docs/superpowers/plans/2026-04-23-envelope-migration-parallel.md -->
<!-- version: 3.0.0 -->
<!-- guid: 3c9d4e5f-6a7b-8c9d-0e1f-2a3b4c5d6e7f -->
<!-- last-edited: 2026-04-24 -->

# Parallel Envelope Migration Plan (TODO 4.15)

Author: Claude (Opus 4.7)
Status: ready for parallel execution
Successor of: CHANGELOG entries April 23, 2026 (PRs #425, #426, #427, #428)

## 1. Goal

Finish migrating every Gin handler in `internal/server/` from raw
`c.JSON(http.Status..., gin.H{...})` calls to the `RespondWith*` helpers
in `internal/server/error_handler.go`, so every successful response is
enveloped as `{"data": ...}` and every error as `{"error","code","status"}`.

This plan splits the remaining ~960 callsites across ~20 handler files
into small, well-scoped units that can be executed by **Haiku sub-agents
running in parallel** with a minimum of merge conflicts and review
overhead.

## 2. Background (read first)

Before dispatching any agent, read these reference points so you
understand the pattern:

- **Helpers**: `internal/server/error_handler.go` — `RespondWithOK`,
  `RespondWithCreated`, `RespondWithBadRequest`, `RespondWithNotFound`,
  `RespondWithInternalError`, `RespondWithConflict`,
  `RespondWithUnauthorized`, `RespondWithForbidden`. `internalError`
  (lowercase) already exists and is kept for 500s that log the `err`.
- **Reference PRs** (all merged, all on main):
  - #425 — backend-only pilot (`entity_tag_handlers.go`, `user_handlers.go`)
  - #426 — coupled slice with api.ts unwrap (`update_handlers.go`)
  - #427 — void-return callers (`quarantine_handlers.go`)
  - #428 — multi-endpoint with mixed gin.H / struct responses
    (`organize_handlers.go`)

**The single most important pattern discovered during the pilots:**

> Put the `.data` unwrap **inside the API-service-layer function** in
> `web/src/services/*.ts`, not in the React component. Components keep
> their existing contract — they still receive `UpdateInfo`, not
> `{ data: UpdateInfo }`. This turns an endpoint migration into a
> **single TypeScript edit** instead of hunting down every component.

Example (copy this pattern verbatim):

```ts
// Before
export async function foo(): Promise<Foo> {
  const response = await fetch(`${API_BASE}/foo`);
  if (!response.ok) throw await buildApiError(response, 'Failed to get foo');
  return response.json();
}

// After
export async function foo(): Promise<Foo> {
  const response = await fetch(`${API_BASE}/foo`);
  if (!response.ok) throw await buildApiError(response, 'Failed to get foo');
  const body = await response.json();
  return body.data;
}
```

**Do not** unwrap in components. **Do not** change the `Promise<Foo>`
return type. **Do not** touch `buildApiError` or error-path code.

## 3. Constraints the plan has to satisfy

| Constraint | How this plan addresses it |
|---|---|
| Haiku is faster + cheaper but less capable. It needs tight scopes, exact paths, and a mechanical pattern. | Each agent gets **one handler file** + a listed set of frontend caller locations. No judgment calls. |
| Agents must not edit the same file simultaneously. | Agents are grouped so **no two agents in the same wave touch the same file**. `web/src/services/api.ts` is shared, so waves are staged. |
| `web/src/services/api.ts` is 3000+ LOC and touched by many endpoints. | Git's line-based merge handles non-adjacent edits cleanly. Agents operate on non-overlapping function definitions. If conflicts happen, merge order (Section 7) resolves them. |
| Response-shape changes must ship backend + frontend together. | Each agent's PR bundles both. No split PRs. |
| Review burden must stay manageable. | ~18 small PRs, each following an identical template. Reviewer skims the diff against the reference PRs. |
| Integration tests may decode response bodies. | Each agent must `grep` for tests referencing its endpoints and update any that inspect JSON shape. |

## 4. Handler inventory (remaining work)

Sorted by callsite count (from `grep -cE 'c\.JSON\(http\.Status'`).
Status reflects the current state on `main` as of 2026-04-23.

| Handler file | Callsites | API-service file(s) | UI page(s) | Risk |
|---|---:|---|---|---|
| `audiobooks_handlers.go` | 87 | `api.ts` | Library, BookDetail | **XL** |
| `entities_handlers.go` | 87 | `api.ts` | Authors, Series | **XL** |
| `operations_handlers.go` | 56 | `api.ts` | Dashboard, Library, dialogs | **L** |
| `ai_handlers.go` | 53 | `api.ts` | BookDedup | **L** |
| `metadata_handlers.go` | 52 | `api.ts` | BookDetail, Metadata dialogs | **L** |
| `dedup_handlers.go` | 52 | `api.ts` | BookDedup | **L** |
| `itunes_handlers.go` | 51 | `api.ts` | ITunes settings + pages | **L** |
| `system_handlers.go` | 45 | `api.ts` | Dashboard, SystemInfo, Storage, Logs | **L** |
| `versions_handlers.go` | 44 | `versionApi.ts` | VersionsPanel, TrashedVersions | **M** |
| `auth_handlers.go` | 43 | `api.ts` | Login, AuthContext | **M** (careful with cookies) |
| `duplicates_handlers.go` | 42 | `api.ts` | BookDedup duplicates tab | **M** |
| `playlist_handlers.go` | 34 | `playlistApi.ts` | Playlists, PlaylistDetail | **M** |
| `diagnostics_handlers.go` | 29 | `api.ts` | Diagnostics page + e2e | **M** |
| `apikey_handlers.go` | 23 | `api.ts` | Settings | **S** |
| `filesystem_handlers.go` | 22 | `api.ts` | FileManager, WelcomeWizard | **S** |
| `plugins_handlers.go` | 19 | `api.ts` | PluginsTab | **S** |
| `reading_handlers.go` | 16 | `readingApi.ts` | BookDetail, AudioPlayer | **S** |
| `activity_handlers.go` | 11 | `activityApi.ts` | ActivityLog, ChangeLog | **S** |
| `file_ops_handlers.go` | 2 | `fileOpsApi.ts` | MainLayout | **XS** |

**Total: ~807 backend callsites remaining, ~20 files.**

## 5. Parallel execution waves

Execution is staged in waves to keep conflict risk low and catch pattern
drift before it spreads. **Do not start a later wave until the previous
wave's PRs are all merged to main.** Within a wave, all agents run
concurrently.

### Wave 1 — Isolated api-service files (6 agents, parallel)

These handlers have their own dedicated `*Api.ts` file, so there is
**zero `web/src/services/api.ts` contention**. Safest wave; validates the
template.

| Agent | Handler | API file | Expected PRs |
|---|---|---|---|
| A1 | `file_ops_handlers.go` | `fileOpsApi.ts` | 1 |
| A2 | `activity_handlers.go` | `activityApi.ts` | 1 |
| A3 | `reading_handlers.go` | `readingApi.ts` | 1 |
| A4 | `versions_handlers.go` | `versionApi.ts` | 1 |
| A5 | `playlist_handlers.go` | `playlistApi.ts` | 1 |
| A6 | *(reserved — no matching service file)* | — | — |

### Wave 2 — Small `api.ts` consumers (4 agents, parallel)

Each touches `api.ts` but in distinct sections. Conflict risk low.

| Agent | Handler |
|---|---|
| B1 | `apikey_handlers.go` |
| B2 | `filesystem_handlers.go` |
| B3 | `plugins_handlers.go` |
| B4 | `diagnostics_handlers.go` |

### Wave 3 — Mid-size `api.ts` consumers (4 agents, parallel)

| Agent | Handler |
|---|---|
| C1 | `system_handlers.go` |
| C2 | `auth_handlers.go` |
| C3 | `duplicates_handlers.go` |
| C4 | `dedup_handlers.go` |

### Wave 4 — Heavy `api.ts` consumers (4 agents, parallel)

| Agent | Handler |
|---|---|
| D1 | `operations_handlers.go` |
| D2 | `ai_handlers.go` |
| D3 | `metadata_handlers.go` |
| D4 | `itunes_handlers.go` |

### Wave 5 — The two giants (serial, Opus or human)

**Do not dispatch to Haiku.** Each has 87 callsites, crosses many UI
pages, and needs judgment. Split into sub-slices (list/get/create/update/delete
groupings) and run each sub-slice as its own PR with a real review.

| Handler | Suggested split |
|---|---|
| `entities_handlers.go` | Authors CRUD / Series CRUD / Tag ops / Search |
| `audiobooks_handlers.go` | List+search / Single-book / Batch ops / Sync+covers |

## 5c. Wave 2 post-mortem (2026-04-24) — single-PR-per-wave is the default

Wave 2 (PR #435) shipped 4 handler migrations in one consolidated PR
instead of four separate PRs. Outcome: 1 merge vs. 5 rebases + 5 merges
in Wave 1. Zero CHANGELOG conflicts. ~10x less coordinator overhead.

**New default for Waves 3+:**
- Dispatch all wave agents in parallel, each in a worktree.
- Agents edit files only — no git, no CHANGELOG (same as Wave 2).
- Coordinator consolidates all 4 agents' edits into ONE branch +
  ONE commit + ONE PR per wave.
- CHANGELOG entry covers the whole wave in a single block.

Use per-agent PRs only if: (a) wave files have heavy test-file overlap
that would make a consolidated diff hard to review, or (b) one agent's
work needs to ship ahead of the others for feature reasons. Neither
applies to Waves 3–4.

## 5b. Wave 1 post-mortem (2026-04-24) — REQUIRED READING before dispatch

Wave 1 completed (PRs #430–#434) but the original dispatch had three defects.
Waves 2+ MUST incorporate these corrections:

1. **Worktree isolation is NOT enough.** Even with `isolation: "worktree"`,
   if prompts give Haiku absolute paths (`/Users/.../internal/server/...`)
   the Edit tool writes to those absolute paths and bypasses the worktree.
   Haiku's edits bled into the coordinator's main working tree.
   **Fix:** use coordinator-driven git. Agents are forbidden from
   running any git/gh command. They only edit files. Coordinator handles
   every branch, commit, push, rebase, and PR.
2. **Sub-agents cannot run git/gh reliably.** 4 of 5 Wave-1 agents
   completed the refactor but were blocked on bash for commit/push/PR.
   **Fix:** bake coordinator-driven git into the template (see Section 6).
3. **Endpoint-path grep, not function-name grep.** A4 missed a test file
   (`server_extra_test.go`) because it only searched for version handler
   names. The test decoded responses by URL, not by function name.
   **Fix:** agents must grep every endpoint URL path across the entire
   `internal/server/*_test.go` tree, not just the obvious test file.

## 6. Agent task template

Every Haiku agent gets **exactly** this prompt template, with the
bracketed parts filled in. Keep it short and mechanical; Haiku does best
with explicit steps.

```
ROLE
You are a Go + TypeScript refactoring agent. You MUST follow the pattern
exactly. No creative interpretation.

TASK
Migrate `internal/server/<HANDLER_FILE>.go` to use the response-envelope
helpers from `internal/server/error_handler.go`. Update any matching
TypeScript callers.

READ FIRST (REQUIRED)
1. internal/server/error_handler.go — the helpers
2. internal/server/organize_handlers.go — reference for the pattern
3. web/src/services/api.ts lines 2981-3002 — reference for the TS adapter

STEP 1 — BACKEND
Open internal/server/<HANDLER_FILE>.go. For every `c.JSON(...)` call:
  - 200 with gin.H or struct:          RespondWithOK(c, <payload>)
  - 201 with gin.H or struct:          RespondWithCreated(c, <payload>)
  - 400 with gin.H{"error": msg}:      RespondWithBadRequest(c, msg)
  - 404 with gin.H{"error": "X not found"}:  RespondWithNotFound(c, "X", id)
  - 409:                               RespondWithConflict(c, msg)
  - 401:                               RespondWithUnauthorized(c, msg)
  - 403:                               RespondWithForbidden(c, msg)
  - 500 with gin.H{"error": msg}:      RespondWithInternalError(c, msg)
     (if the code already calls `internalError(c, msg, err)`, LEAVE IT —
      that one logs and already does the right thing.)

Remove the now-unused `"net/http"` import if nothing else needs it.

STEP 2 — FRONTEND
grep for every endpoint this handler owns in web/src/. For each caller
that returns data (not Promise<void>), change:
    return response.json();
to:
    const body = await response.json();
    return body.data;

DO NOT touch callers that return Promise<void>. DO NOT touch error-path
code. DO NOT touch component files.

STEP 3 — TESTS
grep the repo for test files that decode the endpoints' responses. If
any test decodes `{"foo": ...}`, wrap it in `{"data": {"foo": ...}}` —
see entity_tag_handlers_test.go or user_handlers_test.go on main for the
pattern.

STEP 4 — VERIFY
  go build ./...
  go vet ./internal/server/...
  cd web && npx tsc --noEmit
  go test ./internal/server/ -run '<RELEVANT_PATTERN>' -count=1 -timeout 120s

All four MUST be clean. If anything fails, fix it — do NOT weaken tests.

STEP 5 — REPORT (NO GIT)
Do NOT run `git`, `gh`, or any branch/commit/push/PR command. Do NOT
touch `CHANGELOG.md`. The coordinator owns all git + CHANGELOG edits.

Your final message MUST list, in this exact format:
  Handler file modified: <path>
  API service file modified: <path>
  Test files modified: <path>, <path>, ...
  Backend callsites migrated: N
  Frontend callers unwrapped: M
  Endpoint paths touched: /api/v1/foo, /api/v1/bar, ...
  Verification: go build=PASS, go vet=PASS, tsc=PASS, go test -run X=PASS

If any verification failed, list the error and stop — do NOT claim
success.

STEP 6 — CHANGELOG
Add a bullet under the existing "HTTP Response Envelope Migration"
section of CHANGELOG.md (Unreleased). Keep it to 2-3 lines.

OUT OF SCOPE (explicitly forbidden)
- Do not rename functions, types, routes, or URLs.
- Do not refactor error handling beyond the helper swap.
- Do not touch any handler file other than the one assigned.
- Do not migrate nil-slice guards to EnsureNotNil (separate TODO item).
- Do not touch component files in web/src/pages or web/src/components.
- Do not introduce new dependencies.
```

## 7. Merge coordination

**Repo policy: rebase or fast-forward only. No squash, ever.** Every merge
in this plan uses `gh pr merge <n> --rebase`. Haiku agents must not pass
`--squash` or `--merge` and must not attempt to self-merge.

**Rule 1 — serial merge.** Even though agents work in parallel, PRs merge
**one at a time** via `gh pr merge <n> --rebase`. This prevents cascading
rebase wars on `api.ts` and `CHANGELOG.md`.

**Rule 2 — rebase before merge.** Each PR is rebased on latest main
immediately before merging. If a rebase conflict hits, resolve it
manually — almost always it's `CHANGELOG.md` (trivial) or
`web/src/services/api.ts` (trivial).

**Rule 3 — merge order within a wave.** Sort by PR number ascending;
merge the oldest first. The CHANGELOG accumulates bullets in order.

**Rule 4 — CI gate.** A PR cannot merge without green CI. If a PR's CI
fails, close the branch, spawn a replacement agent with the failure log
as additional context.

**Rule 5 — no merge across waves.** Wait for all PRs in wave N to merge
before dispatching wave N+1. This means every agent in wave N+1 branches
from fully-migrated wave-N main.

## 8. Agent dispatch mechanics

Use the Task tool with `subagent_type: "general-purpose"` and
`model: "haiku"`. Run agents concurrently by calling the Task tool
**multiple times in a single message** (one per agent).

Each agent dispatch must include:
- Model pinned to Haiku
- The exact template from Section 6 with placeholders filled
- An explicit list of endpoint paths the agent owns (so it greps correctly)
- The branch name to use (pre-assigned)

Suggested branch names:
```
refactor/envelope-file-ops
refactor/envelope-activity
refactor/envelope-reading
refactor/envelope-versions
refactor/envelope-playlists
refactor/envelope-apikey
refactor/envelope-filesystem
refactor/envelope-plugins
refactor/envelope-diagnostics
refactor/envelope-system
refactor/envelope-auth
refactor/envelope-duplicates
refactor/envelope-dedup
refactor/envelope-operations
refactor/envelope-ai
refactor/envelope-metadata
refactor/envelope-itunes
```

## 9. Coordinator (Opus) responsibilities

The Opus instance that dispatches the agents is responsible for:

1. Picking the wave and dispatching all its agents in one Task-tool call.
2. Monitoring PRs (e.g. `gh pr list --search 'envelope in:title'`).
3. Reviewing each PR diff against the reference PRs for shape drift.
   Skim: is every raw `c.JSON` gone? Are the TS unwraps only inside
   api-service functions?
4. Rebase-merging PRs in order, waiting for CI green.
5. Running the full test suite after each wave completes
   (`make test-all`), not after each PR.
6. Handling Wave 5 personally (entities, audiobooks) — splitting each
   into sub-slice PRs.
7. Updating TODO 4.15 status after each wave.

## 10. Stop conditions

Stop the parallel rollout and escalate to human review if any of these
happens:

- A Haiku agent's PR fails CI twice after replacement.
- A wave's merged PRs break the e2e suite.
- An agent requests scope expansion (it's trying to be helpful; deny).
- A response shape conflict surfaces (e.g., a component that the plan
  missed reads response bodies directly). Pause, add the component to the
  plan, re-dispatch.

## 11. Estimated effort

- Wave 1 (6 agents, ~30 min wall clock each): ~1h with coordinator merge
  overhead
- Wave 2 (4 agents): ~45min
- Wave 3 (4 agents): ~1h
- Wave 4 (4 agents): ~1h15
- Wave 5 (giants, Opus): ~2-4h split across sessions

Total: **half a day of wall clock** for the 17 Haiku-eligible files,
plus a dedicated follow-up session for the two giants.

## 12. Definition of done

- Zero `c.JSON(http.Status` calls remain in `internal/server/` outside
  of `error_handler.go` itself (verify with
  `grep -rn 'c\.JSON(http\.Status' internal/server/ | grep -v error_handler.go`).
- All `web/src/services/*.ts` callers either unwrap `.data` or are
  documented as `Promise<void>`.
- `make test-all` green.
- CHANGELOG has one bullet per merged PR under the envelope-migration
  section.
- TODO 4.15 marked `[x]` with a summary of PRs.
