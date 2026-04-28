<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-core-3-discovery.md -->
<!-- version: 1.0.0 -->
<!-- guid: c9d0e1f2-a3b4-5678-cdef-901234567890 -->

# BOT TASK: Unified Maintenance — Frontend API Client

**TODO ID:** ASYNC-CORE-3  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-2` — dispatcher handler must be live

```bash
count=$(gh pr list --label "task:ASYNC-CORE-2" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-2"; exit 0; }
```

## Branch

```
feat/async-core-3-maintenance-api-client
```

## Label

```bash
gh label create "task:ASYNC-CORE-3" --color "0075ca" --description "Bot task: maintenance frontend API client" 2>/dev/null || true
```

## Files to Edit

1. `web/src/services/api.ts` — add two new functions

## Step 1 — Add to `web/src/services/api.ts`

Find the existing maintenance-related API calls and add after them:

```typescript
export interface MaintenanceJobDef {
  id: string;
  name: string;
  description: string;
  category: string;
  default_params: Record<string, unknown>;
  can_resume: boolean;
}

/** Returns all registered maintenance jobs from the server. */
export async function listMaintenanceJobs(): Promise<MaintenanceJobDef[]> {
  const response = await fetch(`${API_BASE}/maintenance/jobs`);
  if (!response.ok) throw await buildApiError(response, 'Failed to list maintenance jobs');
  const data = await response.json();
  return data.jobs ?? [];
}

/** Runs a registered maintenance job and returns the operation ID. */
export async function runMaintenanceJob(
  jobId: string,
  params: Record<string, unknown> = {}
): Promise<{ operation_id: string }> {
  const response = await fetch(`${API_BASE}/maintenance/jobs/${encodeURIComponent(jobId)}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params),
  });
  if (!response.ok) throw await buildApiError(response, `Failed to run maintenance job ${jobId}`);
  return response.json();
}
```

Bump the file version header.

## Step 2 — Verify

```bash
cd web && npx tsc --noEmit
```

Zero new type errors.

## Definition of Done

- `listMaintenanceJobs()` and `runMaintenanceJob()` exported from `api.ts`
- TypeScript compiles clean
- No other files changed

## PR Instructions

```bash
gh label create "task:ASYNC-CORE-3" --color "0075ca" --description "Bot task: maintenance frontend API client" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): add frontend API client for unified job system" \
  --body "Adds listMaintenanceJobs() and runMaintenanceJob() to api.ts. Prerequisite for the dynamic maintenance UI (ASYNC-CORE-4). (ASYNC-CORE-3)" \
  --label "task:ASYNC-CORE-3"
```
