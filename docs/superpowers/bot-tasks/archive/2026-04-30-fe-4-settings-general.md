<!-- file: docs/superpowers/bot-tasks/2026-04-30-fe-4-settings-general.md -->
<!-- version: 1.0.0 -->
<!-- guid: d2e3f4a5-b6c7-8901-defa-234567890bc1 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: FE-4 — Extract GeneralSettingsTab from Settings.tsx

**TODO ID:** FE-4
**Audience:** burndown bot
**Branch:** `refactor/settings-general-tab`
**PR title:** `refactor(web): extract GeneralSettingsTab from Settings.tsx`

---

## What This Task Does

Extracts the "General" settings tab content from `web/src/pages/Settings.tsx` into
`web/src/components/settings/GeneralSettingsTab.tsx`. This is the first of three
PRs to split Settings.tsx.

---

## What NOT to Do

- **Do NOT change** settings logic or API calls.
- **Do NOT merge** the paths or metadata tabs into this PR.
- **Do NOT break** the settings form save/cancel functionality.

---

## Read First

1. Read `web/src/pages/Settings.tsx`. Find the "General" tab panel JSX (e.g.,
   server name, theme, language, update interval — whatever the General tab shows).
2. Identify the state/props the General tab section needs.
3. Note how the settings form submits (e.g., a shared `handleSave` function that
   saves all tabs at once, or per-tab save).

---

## Steps

### Step 1 — Create the directory

```bash
mkdir -p /Users/jdfalk/.worktrees/audiobook-eval/web/src/components/settings
```

### Step 2 — Create GeneralSettingsTab.tsx

```tsx
// web/src/components/settings/GeneralSettingsTab.tsx
import React from 'react';
import { TextField, Switch, FormControlLabel, Box } from '@mui/material';
import type { GeneralSettings } from '../../types/settings'; // adjust as needed

interface GeneralSettingsTabProps {
  settings: GeneralSettings;
  onChange: (updated: Partial<GeneralSettings>) => void;
}

export const GeneralSettingsTab: React.FC<GeneralSettingsTabProps> = ({
  settings, onChange
}) => {
  return (
    <Box>
      {/* Paste the extracted General tab JSX here */}
    </Box>
  );
};
```

Adjust the props to match exactly what the extracted JSX needs.

### Step 3 — Replace in Settings.tsx

Replace the General tab content with:
```tsx
<GeneralSettingsTab
  settings={generalSettings}
  onChange={handleGeneralChange}
/>
```

Add the import:
```tsx
import { GeneralSettingsTab } from '../components/settings/GeneralSettingsTab';
```

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval/web
npx tsc --noEmit 2>&1 | tail -20
npm run build 2>&1 | tail -20
```

### Step 5 — Commit and open PR

```bash
git checkout -b refactor/settings-general-tab
git add web/src/components/settings/GeneralSettingsTab.tsx web/src/pages/Settings.tsx
git commit -m "refactor(web): extract GeneralSettingsTab from Settings.tsx

Moves General tab panel JSX into GeneralSettingsTab component.
Settings logic and save handlers remain in Settings.tsx.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/settings-general-tab
gh pr create \
  --title "refactor(web): extract GeneralSettingsTab from Settings.tsx" \
  --body "First of 3 PRs splitting Settings.tsx. Extracts General tab. FE-4."
```

---

## Checklist

- [ ] `web/src/components/settings/` directory created
- [ ] `GeneralSettingsTab.tsx` created with typed props interface
- [ ] Extracted JSX moved (not duplicated) into GeneralSettingsTab
- [ ] Settings.tsx uses `<GeneralSettingsTab>` with all required props
- [ ] Save/cancel functionality unchanged
- [ ] `npx tsc --noEmit` passes
- [ ] `npm run build` succeeds
- [ ] PR opened with correct branch and title
