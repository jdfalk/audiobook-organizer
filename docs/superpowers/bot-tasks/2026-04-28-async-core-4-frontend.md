<!-- file: docs/superpowers/bot-tasks/2026-04-28-async-core-4-frontend.md -->
<!-- version: 1.0.0 -->
<!-- guid: d0e1f2a3-b4c5-6789-defa-012345678901 -->

# BOT TASK: Unified Maintenance — Dynamic Frontend Section

**TODO ID:** ASYNC-CORE-4  
**Audience:** burndown bot

## Prerequisites

- `task:ASYNC-CORE-3` — frontend API client

```bash
count=$(gh pr list --label "task:ASYNC-CORE-3" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:ASYNC-CORE-3"; exit 0; }
```

## Branch

```
feat/async-core-4-maintenance-frontend
```

## Label

```bash
gh label create "task:ASYNC-CORE-4" --color "0075ca" --description "Bot task: dynamic maintenance UI section" 2>/dev/null || true
```

## Context

`web/src/components/system/MaintenanceTab.tsx` currently has a scheduler-task list
fetched from `api.getRegisteredTasks()`. We are adding a NEW section below the
existing content titled "Manual Fixes" that dynamically renders cards for each
registered `MaintenanceJob`.

Do NOT remove or modify the existing scheduler task section.

## Files to Edit

1. `web/src/components/system/MaintenanceTab.tsx` — add "Manual Fixes" section

## Step 1 — Add the Manual Fixes section

At the top of the file, add imports:
```typescript
import { listMaintenanceJobs, runMaintenanceJob, type MaintenanceJobDef } from '../../services/api';
import { useOperationsStore } from '../../stores/useOperationsStore';
```

Add state inside the component (after existing state declarations):
```typescript
const [manualJobs, setManualJobs] = useState<MaintenanceJobDef[]>([]);
const [jobRunning, setJobRunning] = useState<Record<string, boolean>>({});
const [jobDryRun, setJobDryRun] = useState<Record<string, boolean>>({});
const startPolling = useOperationsStore((state) => state.startPolling);
```

Add fetch in a useEffect (alongside existing fetchTasks):
```typescript
useEffect(() => {
  listMaintenanceJobs().then(setManualJobs).catch(() => {});
}, []);
```

Add handler:
```typescript
const handleRunJob = async (job: MaintenanceJobDef) => {
  setJobRunning((prev) => ({ ...prev, [job.id]: true }));
  try {
    const dryRun = jobDryRun[job.id] ?? (job.default_params?.dry_run as boolean ?? true);
    const result = await runMaintenanceJob(job.id, { dry_run: dryRun });
    startPolling(result.operation_id, job.id);
  } catch (err) {
    console.error('Failed to run job', job.id, err);
  } finally {
    setJobRunning((prev) => ({ ...prev, [job.id]: false }));
  }
};
```

Add rendered section (below the existing maintenance window card, before the closing `</Box>`):

```tsx
{manualJobs.length > 0 && (
  <Box sx={{ mt: 3 }}>
    <Typography variant="h6" gutterBottom>Manual Fixes</Typography>
    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
      One-off maintenance jobs. All default to dry-run; toggle off to apply changes.
    </Typography>
    {(['library', 'files', 'itunes', 'dedup', 'cleanup'] as const).map((category) => {
      const categoryJobs = manualJobs.filter((j) => j.category === category);
      if (categoryJobs.length === 0) return null;
      return (
        <Box key={category} sx={{ mb: 2 }}>
          <Typography variant="subtitle2" sx={{ textTransform: 'capitalize', mb: 1 }}>
            {category}
          </Typography>
          <Stack spacing={1}>
            {categoryJobs.map((job) => {
              const hasDryRun = 'dry_run' in (job.default_params ?? {});
              const dryRun = jobDryRun[job.id] ?? (job.default_params?.dry_run as boolean ?? true);
              return (
                <Paper key={job.id} variant="outlined" sx={{ p: 1.5, display: 'flex', alignItems: 'center', gap: 2 }}>
                  <Box sx={{ flex: 1 }}>
                    <Typography variant="body2" fontWeight={500}>{job.name}</Typography>
                    <Typography variant="caption" color="text.secondary">{job.description}</Typography>
                  </Box>
                  {hasDryRun && (
                    <FormControlLabel
                      control={
                        <Switch
                          size="small"
                          checked={dryRun}
                          onChange={(e) => setJobDryRun((prev) => ({ ...prev, [job.id]: e.target.checked }))}
                        />
                      }
                      label="Dry run"
                      labelPlacement="start"
                    />
                  )}
                  <Button
                    size="small"
                    variant="outlined"
                    disabled={jobRunning[job.id]}
                    onClick={() => handleRunJob(job)}
                    startIcon={jobRunning[job.id] ? <CircularProgress size={14} /> : undefined}
                  >
                    Run
                  </Button>
                </Paper>
              );
            })}
          </Stack>
        </Box>
      );
    })}
  </Box>
)}
```

Add missing MUI imports at top if not already present:
`FormControlLabel, Switch, Stack, Paper`

Bump the file version header.

## Step 2 — Verify

```bash
cd web && npx tsc --noEmit
make web-dev  # start dev server, manually verify the new section appears
```

The "Manual Fixes" section should appear in the Maintenance tab. Since no jobs
are registered yet (wave tasks not done), the section will be empty — that's correct.

## Definition of Done

- "Manual Fixes" section renders below the existing maintenance window UI
- Calls `listMaintenanceJobs()` on mount
- Each job card shows name, description, dry-run toggle (if applicable), Run button
- Clicking Run calls `runMaintenanceJob()` and starts polling via `useOperationsStore`
- TypeScript compiles clean

## PR Instructions

```bash
gh label create "task:ASYNC-CORE-4" --color "0075ca" --description "Bot task: dynamic maintenance UI section" 2>/dev/null || true
gh pr create \
  --title "feat(maintenance): add dynamic Manual Fixes section to MaintenanceTab" \
  --body "Adds a 'Manual Fixes' section to the Maintenance tab that dynamically renders cards for all registered MaintenanceJob implementations. Run button enqueues the job and shows toast + badge progress. (ASYNC-CORE-4)" \
  --label "task:ASYNC-CORE-4"
```
