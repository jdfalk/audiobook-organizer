<!-- file: docs/superpowers/bot-tasks/2026-04-30-fe-3-batch-toolbar.md -->
<!-- version: 1.0.0 -->
<!-- guid: c1d2e3f4-a5b6-7890-cdef-123456789ab0 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: FE-3 — Extract BatchToolbar from Library.tsx

**TODO ID:** FE-3
**Audience:** burndown bot
**Branch:** `refactor/library-batch-toolbar`
**PR title:** `refactor(web): extract BatchToolbar component from Library.tsx`

**Prerequisite:** FE-2 must be merged first.

---

## What This Task Does

Extracts the batch-action toolbar (shown when books are selected) from
`web/src/pages/Library.tsx` into `web/src/components/BatchToolbar.tsx`. This is
the third and final PR to split Library.tsx.

---

## What NOT to Do

- **Do NOT move** the batch action handlers — keep them in Library.tsx, pass as
  callbacks.
- **Do NOT change** what actions are available (delete, move, tag, etc.).
- **Do NOT merge** anything else into this PR.

---

## Read First

1. Read `web/src/pages/Library.tsx` after FE-1 and FE-2 are merged. Find the JSX
   that renders the batch toolbar: typically a `<Toolbar>` or `<Box>` that appears
   when `selectedBookIds.size > 0`.
2. Identify the callbacks the toolbar fires: `onDelete`, `onMove`, `onTag`, etc.
3. Note which MUI components are used (Toolbar, Button, Chip, etc.).

---

## Steps

### Step 1 — Create BatchToolbar.tsx

```tsx
// web/src/components/BatchToolbar.tsx
import React from 'react';
import { Toolbar, Typography, Button, Box } from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
// import other icons as needed

interface BatchToolbarProps {
  selectedCount: number;
  onDelete: () => void;
  onMove: () => void;
  onClearSelection: () => void;
  // add other callbacks as needed
}

export const BatchToolbar: React.FC<BatchToolbarProps> = ({
  selectedCount, onDelete, onMove, onClearSelection
}) => {
  if (selectedCount === 0) return null;

  return (
    <Toolbar sx={{ bgcolor: 'primary.light' }}>
      <Typography sx={{ flex: 1 }}>
        {selectedCount} selected
      </Typography>
      <Button onClick={onDelete} startIcon={<DeleteIcon />}>Delete</Button>
      <Button onClick={onMove}>Move</Button>
      <Button onClick={onClearSelection}>Clear</Button>
    </Toolbar>
  );
};
```

Adjust to match the actual JSX extracted from Library.tsx.

### Step 2 — Replace in Library.tsx

Replace the batch toolbar JSX with:
```tsx
<BatchToolbar
  selectedCount={selectedBookIds.size}
  onDelete={handleBatchDelete}
  onMove={handleBatchMove}
  onClearSelection={() => setSelectedBookIds(new Set())}
/>
```

Add the import:
```tsx
import { BatchToolbar } from '../components/BatchToolbar';
```

### Step 3 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval/web
npx tsc --noEmit 2>&1 | tail -20
npm run build 2>&1 | tail -20
```

### Step 4 — Commit and open PR

```bash
git checkout -b refactor/library-batch-toolbar
git add web/src/components/BatchToolbar.tsx web/src/pages/Library.tsx
git commit -m "refactor(web): extract BatchToolbar component from Library.tsx

Final PR splitting Library.tsx. Moves batch-action toolbar into
BatchToolbar component. Handlers remain in Library.tsx (props only).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/library-batch-toolbar
gh pr create \
  --title "refactor(web): extract BatchToolbar component from Library.tsx" \
  --body "Third and final PR splitting Library.tsx. Extracts batch toolbar. Depends on FE-1 and FE-2. FE-3."
```

---

## Checklist

- [ ] `web/src/components/BatchToolbar.tsx` created
- [ ] `BatchToolbarProps` interface typed correctly
- [ ] Toolbar returns `null` when `selectedCount === 0`
- [ ] Action handlers remain in Library.tsx, passed as callbacks
- [ ] `Library.tsx` uses `<BatchToolbar>` with all required props
- [ ] `npx tsc --noEmit` passes with no new errors
- [ ] `npm run build` succeeds
- [ ] PR opened with correct branch and title
