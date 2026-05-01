<!-- file: docs/superpowers/bot-tasks/2026-04-30-fe-6-settings-metadata.md -->
<!-- version: 1.0.0 -->
<!-- guid: f4a5b6c7-d8e9-0123-fabc-456789012de3 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: FE-6 — Extract MetadataSettingsTab from Settings.tsx

**TODO ID:** FE-6
**Audience:** burndown bot
**Branch:** `refactor/settings-metadata-tab`
**PR title:** `refactor(web): extract MetadataSettingsTab from Settings.tsx`

**Prerequisite:** FE-5 must be merged first.

---

## What This Task Does

Extracts the "Metadata" settings tab content from `web/src/pages/Settings.tsx`
into `web/src/components/settings/MetadataSettingsTab.tsx`. This is the third and
final PR to split Settings.tsx.

---

## What NOT to Do

- **Do NOT change** metadata-fetch logic or API calls.
- **Do NOT merge** anything else into this PR.
- **Do NOT break** the metadata provider toggles.

---

## Read First

1. Read `web/src/pages/Settings.tsx`. Find the Metadata tab (Open Library toggle,
   OpenAI API key, auto-fetch on import toggle, etc.).
2. Identify the props needed.

---

## Steps

### Step 1 — Create MetadataSettingsTab.tsx

```tsx
// web/src/components/settings/MetadataSettingsTab.tsx
import React from 'react';
import { Switch, FormControlLabel, TextField, Box } from '@mui/material';

interface MetadataSettingsTabProps {
  openLibraryEnabled: boolean;
  onOpenLibraryToggle: (enabled: boolean) => void;
  openAIKey: string;
  onOpenAIKeyChange: (key: string) => void;
  autoFetchOnImport: boolean;
  onAutoFetchToggle: (enabled: boolean) => void;
  // add other props as found in the actual JSX
}

export const MetadataSettingsTab: React.FC<MetadataSettingsTabProps> = (props) => {
  return (
    <Box>
      {/* Paste the extracted Metadata tab JSX here */}
    </Box>
  );
};
```

### Step 2 — Replace in Settings.tsx

```tsx
<MetadataSettingsTab
  openLibraryEnabled={openLibraryEnabled}
  onOpenLibraryToggle={setOpenLibraryEnabled}
  openAIKey={openAIKey}
  onOpenAIKeyChange={setOpenAIKey}
  autoFetchOnImport={autoFetchOnImport}
  onAutoFetchToggle={setAutoFetchOnImport}
/>
```

Add the import:
```tsx
import { MetadataSettingsTab } from '../components/settings/MetadataSettingsTab';
```

### Step 3 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval/web
npx tsc --noEmit 2>&1 | tail -20
npm run build 2>&1 | tail -20
```

### Step 4 — Commit and open PR

```bash
git checkout -b refactor/settings-metadata-tab
git add web/src/components/settings/MetadataSettingsTab.tsx web/src/pages/Settings.tsx
git commit -m "refactor(web): extract MetadataSettingsTab from Settings.tsx

Final PR splitting Settings.tsx. Moves Metadata tab into its own
component. Settings logic remains in Settings.tsx.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/settings-metadata-tab
gh pr create \
  --title "refactor(web): extract MetadataSettingsTab from Settings.tsx" \
  --body "Third and final PR splitting Settings.tsx. Extracts Metadata tab. Depends on FE-4 and FE-5. FE-6."
```

---

## Checklist

- [ ] `MetadataSettingsTab.tsx` created in `web/src/components/settings/`
- [ ] Props interface typed correctly
- [ ] Metadata handlers remain in Settings.tsx (passed as props)
- [ ] `Settings.tsx` uses `<MetadataSettingsTab>` with all required props
- [ ] `npx tsc --noEmit` passes
- [ ] `npm run build` succeeds
- [ ] PR opened with correct branch and title
