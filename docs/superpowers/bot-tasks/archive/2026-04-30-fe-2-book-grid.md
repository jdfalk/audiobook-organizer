<!-- file: docs/superpowers/bot-tasks/2026-04-30-fe-2-book-grid.md -->
<!-- version: 1.0.0 -->
<!-- guid: b0c1d2e3-f4a5-6789-bcde-012345678fa9 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: FE-2 — Extract BookGrid from Library.tsx

**TODO ID:** FE-2
**Audience:** burndown bot
**Branch:** `refactor/library-book-grid`
**PR title:** `refactor(web): extract BookGrid component from Library.tsx`

**Prerequisite:** FE-1 must be merged first.

---

## What This Task Does

Extracts the grid/list of book cards from `web/src/pages/Library.tsx` into
`web/src/components/BookGrid.tsx`. This is the second of three PRs to split
Library.tsx.

---

## What NOT to Do

- **Do NOT move** the individual `BookCard` component if it already exists — only
  the grid container/loop.
- **Do NOT change** the MUI Grid props or card layout.
- **Do NOT merge** the FE-3 (BatchToolbar) work into this PR.
- **Do NOT break** book selection or pagination — pass them as props.

---

## Read First

1. Read `web/src/pages/Library.tsx` after FE-1 is merged. Find the JSX that:
   - Renders a `<Grid container>` or `<Box>` of book cards
   - Maps over `books` array to render each card
   - Handles click/selection of books
2. Identify the props BookGrid will need:
   - `books: Book[]` (the page of books to display)
   - `selectedBookIds: Set<string>` (or `string[]`)
   - `onBookSelect: (id: string) => void`
   - `loading: boolean`
   - Any pagination props

---

## Steps

### Step 1 — Create BookGrid.tsx

```tsx
// web/src/components/BookGrid.tsx
import React from 'react';
import { Grid, CircularProgress, Box } from '@mui/material';
import { BookCard } from './BookCard'; // adjust path if needed
import type { Book } from '../types'; // adjust to actual type location

interface BookGridProps {
  books: Book[];
  selectedBookIds: Set<string>;
  onBookSelect: (id: string) => void;
  loading: boolean;
}

export const BookGrid: React.FC<BookGridProps> = ({
  books, selectedBookIds, onBookSelect, loading
}) => {
  if (loading) {
    return <Box display="flex" justifyContent="center"><CircularProgress /></Box>;
  }

  return (
    <Grid container spacing={2}>
      {books.map(book => (
        <Grid item key={book.id} xs={12} sm={6} md={4} lg={3}>
          <BookCard
            book={book}
            selected={selectedBookIds.has(book.id)}
            onSelect={() => onBookSelect(book.id)}
          />
        </Grid>
      ))}
    </Grid>
  );
};
```

Adjust to match the actual JSX extracted from Library.tsx.

### Step 2 — Replace in Library.tsx

Replace the extracted grid JSX with:
```tsx
<BookGrid
  books={books}
  selectedBookIds={selectedBookIds}
  onBookSelect={handleBookSelect}
  loading={loading}
/>
```

Add the import:
```tsx
import { BookGrid } from '../components/BookGrid';
```

### Step 3 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval/web
npx tsc --noEmit 2>&1 | tail -20
npm run build 2>&1 | tail -20
```

### Step 4 — Commit and open PR

```bash
git checkout -b refactor/library-book-grid
git add web/src/components/BookGrid.tsx web/src/pages/Library.tsx
git commit -m "refactor(web): extract BookGrid component from Library.tsx

Moves the book grid JSX into BookGrid component. Props-only extraction,
no logic moved. Depends on FE-1 (FilterPanel).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/library-book-grid
gh pr create \
  --title "refactor(web): extract BookGrid component from Library.tsx" \
  --body "Second of 3 PRs splitting Library.tsx. Extracts book grid into BookGrid. Depends on FE-1. FE-2."
```

---

## Checklist

- [ ] `web/src/components/BookGrid.tsx` created
- [ ] `BookGridProps` interface defined with correct types
- [ ] Loading state handled in BookGrid
- [ ] `Library.tsx` uses `<BookGrid>` with all required props
- [ ] Book selection still works
- [ ] `npx tsc --noEmit` passes with no new errors
- [ ] `npm run build` succeeds
- [ ] PR opened with correct branch and title
