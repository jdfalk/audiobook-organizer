<!-- file: docs/superpowers/bot-tasks/2026-04-30-fe-5-settings-paths.md -->
<!-- version: 1.0.0 -->
<!-- guid: e3f4a5b6-c7d8-9012-efab-345678901cd2 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: FE-5 — Extract PathsSettingsTab from Settings.tsx

**TODO ID:** FE-5
**Audience:** burndown bot
**Branch:** `refactor/settings-paths-tab`
**PR title:** `refactor(web): extract PathsSettingsTab from Settings.tsx`

**Prerequisite:** FE-4 must be merged first.

---

## What This Task Does

Extracts the "Paths" (library paths / import directories) settings tab content
from `web/src/pages/Settings.tsx` into
`web/src/components/settings/PathsSettingsTab.tsx`.

---

## What NOT to Do

- **Do NOT change** path-management logic or API calls.
- **Do NOT merge** the metadata tab into this PR.
- **Do NOT change** how paths are added or removed.

---

## Read First

1. Read `web/src/pages/Settings.tsx`. Find the Paths tab panel (library root
   path, import paths list, add/remove path controls).
2. Identify state and handlers the Paths tab needs:
   - `libraryPath: string`, `setLibraryPath`
   - `importPaths: string[]`, `addImportPath`, `removeImportPath`

---

## Steps

### Step 1 — Create PathsSettingsTab.tsx

```tsx
// web/src/components/settings/PathsSettingsTab.tsx
import React from 'react';
import { TextField, Button, List, ListItem, IconButton, Box } from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';

interface PathsSettingsTabProps {
  libraryPath: string;
  onLibraryPathChange: (path: string) => void;
  importPaths: string[];
  onAddImportPath: (path: string) => void;
  onRemoveImportPath: (index: number) => void;
}

export const PathsSettingsTab: React.FC<PathsSettingsTabProps> = (props) => {
  return (
    <Box>
      {/* Paste the extracted Paths tab JSX here */}
    </Box>
  );
};
```

Adjust props to match the actual extracted JSX.

### Step 2 — Replace in Settings.tsx

```tsx
<PathsSettingsTab
  libraryPath={libraryPath}
  onLibraryPathChange={setLibraryPath}
  importPaths={importPaths}
  onAddImportPath={handleAddImportPath}
  onRemoveImportPath={handleRemoveImportPath}
/>
```

Add the import:
```tsx
import { PathsSettingsTab } from '../components/settings/PathsSettingsTab';
```

### Step 3 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval/web
npx tsc --noEmit 2>&1 | tail -20
npm run build 2>&1 | tail -20
```

### Step 4 — Commit and open PR

```bash
git checkout -b refactor/settings-paths-tab
git add web/src/components/settings/PathsSettingsTab.tsx web/src/pages/Settings.tsx
git commit -m "refactor(web): extract PathsSettingsTab from Settings.tsx

Moves Paths tab panel JSX into PathsSettingsTab component.
Path management handlers remain in Settings.tsx.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/settings-paths-tab
gh pr create \
  --title "refactor(web): extract PathsSettingsTab from Settings.tsx" \
  --body "Second of 3 PRs splitting Settings.tsx. Extracts Paths tab. Depends on FE-4. FE-5."
```

---

## Checklist

- [ ] `PathsSettingsTab.tsx` created in `web/src/components/settings/`
- [ ] Props interface typed correctly
- [ ] Add/remove path handlers remain in Settings.tsx (passed as props)
- [ ] `Settings.tsx` uses `<PathsSettingsTab>` with all required props
- [ ] `npx tsc --noEmit` passes
- [ ] `npm run build` succeeds
- [ ] PR opened with correct branch and title
