<!-- file: docs/superpowers/bot-tasks/2026-04-30-fe-1-filter-panel.md -->
<!-- version: 1.0.0 -->
<!-- guid: a9b0c1d2-e3f4-5678-abcd-901234567ef8 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: FE-1 — Extract FilterPanel from Library.tsx

**TODO ID:** FE-1
**Audience:** burndown bot
**Branch:** `refactor/library-filter-panel`
**PR title:** `refactor(web): extract FilterPanel component from Library.tsx`

---

## What This Task Does

Extracts the search/filter sidebar or toolbar from `web/src/pages/Library.tsx`
into a new `web/src/components/FilterPanel.tsx` component. This is the first of
three PRs to split `Library.tsx`.

---

## What NOT to Do

- **Do NOT change** any filtering logic or state — only move code.
- **Do NOT change** the CSS classes or Material-UI component props.
- **Do NOT merge** the FE-2 (BookGrid) or FE-3 (BatchToolbar) work into this PR.
- **Do NOT break** the library page — it must render identically after the refactor.

---

## Read First

1. Read `web/src/pages/Library.tsx` (the full file). Identify:
   - The JSX block that renders the search input, genre filter, author filter,
     sort controls, and any other filter UI.
   - The props/state that the filter UI reads and writes.
2. Identify the state variables and callbacks that the filter panel needs:
   - `searchQuery`, `setSearchQuery` (or similar)
   - `selectedGenre`, `setSelectedGenre`
   - `sortBy`, `setSortBy`
   - Any other filter-related state

---

## Steps

### Step 1 — Identify the filter panel JSX block

In `Library.tsx`, find the JSX that renders the filter/search UI. It's typically
a `<Box>` or `<Drawer>` or `<Paper>` containing search and filter controls.
Note the start and end JSX tags.

### Step 2 — Define the FilterPanel props interface

Create `web/src/components/FilterPanel.tsx`:

```tsx
import React from 'react';
// ... import MUI components used by the filter panel

interface FilterPanelProps {
  searchQuery: string;
  onSearchChange: (query: string) => void;
  selectedGenre: string;
  onGenreChange: (genre: string) => void;
  sortBy: string;
  onSortChange: (sort: string) => void;
  // add any other props needed
}

export const FilterPanel: React.FC<FilterPanelProps> = ({
  searchQuery, onSearchChange,
  selectedGenre, onGenreChange,
  sortBy, onSortChange,
}) => {
  return (
    // Paste the extracted JSX here
  );
};
```

Adjust the props to match exactly what the extracted JSX needs.

### Step 3 — Replace in Library.tsx

Replace the extracted JSX block in `Library.tsx` with:
```tsx
<FilterPanel
  searchQuery={searchQuery}
  onSearchChange={setSearchQuery}
  selectedGenre={selectedGenre}
  onGenreChange={setSelectedGenre}
  sortBy={sortBy}
  onSortChange={setSortBy}
/>
```

Add the import at the top of `Library.tsx`:
```tsx
import { FilterPanel } from '../components/FilterPanel';
```

### Step 4 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval
npm run build --prefix web 2>&1 | tail -20
# or:
cd web && npx tsc --noEmit 2>&1 | tail -20
```

Visually verify the library page renders the same if possible.

### Step 5 — Commit and open PR

```bash
git checkout -b refactor/library-filter-panel
git add web/src/components/FilterPanel.tsx web/src/pages/Library.tsx
git commit -m "refactor(web): extract FilterPanel component from Library.tsx

Moves the search/filter sidebar JSX into a new FilterPanel component.
Props-only interface — no logic moved. Library page renders identically.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/library-filter-panel
gh pr create \
  --title "refactor(web): extract FilterPanel component from Library.tsx" \
  --body "First of 3 PRs splitting Library.tsx. Extracts filter UI into FilterPanel. No logic changes. FE-1."
```

---

## Checklist

- [ ] `web/src/components/FilterPanel.tsx` created
- [ ] `FilterPanel` has a typed `FilterPanelProps` interface
- [ ] Extracted JSX moved into FilterPanel, not duplicated
- [ ] `Library.tsx` uses `<FilterPanel>` with all required props
- [ ] No filter state or logic moved (remains in Library.tsx)
- [ ] `npx tsc --noEmit` passes with no new errors
- [ ] `npm run build` succeeds
- [ ] PR opened with correct branch and title
