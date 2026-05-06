<!-- file: docs/superpowers/bot-tasks/2026-05-01-struct-9-frontend-component-splits.md -->
<!-- version: 1.0.0 -->
<!-- guid: c9d0e1f2-a3b4-5678-cdef-901234567890 -->
<!-- last-edited: 2026-05-01 -->

# STRUCT-9 — Split `Library.tsx` and `BookDedup.tsx` into sub-components

**Priority:** Medium  
**Effort:** Large (UI refactor — extract components, no logic changes)  
**Branch:** `refactor/struct-9-frontend-component-splits`

---

## Why This Matters

Two frontend pages are monolithic and impossible to review:
- `web/src/pages/Library.tsx` — **2900+ lines**
- `web/src/pages/BookDedup.tsx` — **2100+ lines**

Both contain many logically independent sub-sections that can be extracted as
sub-components without changing any behaviour.

**Evidence:**
```bash
wc -l web/src/pages/Library.tsx web/src/pages/BookDedup.tsx
```

---

## What This Task Does

Extract named subcomponents into files under `web/src/components/library/` and
`web/src/components/dedup/`. **No logic or state changes** — only lift JSX blocks
into separate components with their props typed explicitly.

---

## What NOT to Do

- **Do NOT** change any state management logic.
- **Do NOT** change API calls or data flow.
- **Do NOT** rename exported page components (`Library`, `BookDedup`).
- **Do NOT** add new features or UI.
- **Do NOT** touch test files.

---

## Part A — `Library.tsx` Extractions

### Target directory: `web/src/components/library/`

#### Component 1: `LibraryToolbar.tsx`
Extract the bulk-action toolbar at lines **~1744–1817** and **~2039–2287**.
Props: selected books array, bulk action callbacks (delete, tag, move, etc.).

#### Component 2: `LibraryBookGrid.tsx`
Extract the main book grid at lines **~2305–2328** (the `<BookGrid />` host plus
sort controls and grid/list toggle).
Props: books array, current sort/view state, event handlers.

#### Component 3: `LibrarySoftDeletedSection.tsx`
Extract the soft-deleted books section at lines **~2371–2474**.
Props: trashed books array, restore/purge callbacks.

#### Component 4: `LibraryDialogs.tsx`
Extract the dialogs/modals section at lines **~2490–2907+**.
Props: dialog open/close states, book being acted on, action callbacks.

---

## Part B — `BookDedup.tsx` Extractions

### Target directory: `web/src/components/dedup/`

#### Component 5: `DedupBookTab.tsx`
Extract the book duplicates tab at lines **~293–516**.
Props: duplicate groups, callbacks.

#### Component 6: `DedupAdvancedScanTab.tsx`
Extract the advanced scan tab at lines **~518–750**.
Props: scan state, trigger callbacks.

#### Component 7: `DedupAuthorTab.tsx`
Extract the author dedup tab at lines **~757–1208**.
Props: author groups, merge callbacks.

#### Component 8: `DedupSeriesTab.tsx`
Extract the series dedup tab at lines **~1210–1500**.
Props: series groups, merge callbacks.

#### Component 9: `DedupReconcileTab.tsx`
Extract the reconcile tab at lines **~2113–end**.
Props: reconcile items, action callbacks.

---

## Steps

### Step 1 — Baseline check

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
npm --prefix web run build 2>&1 | tail -20
npm --prefix web run test 2>&1 | grep -E 'FAIL|PASS|Tests'
```

### Step 2 — Create target directories

```bash
mkdir -p web/src/components/library
mkdir -p web/src/components/dedup
```

### Step 3 — Extract Library subcomponents (one at a time)

For each component in Part A:
1. Create `web/src/components/library/ComponentName.tsx`.
2. Add the standard file header:
   ```typescript
   // file: web/src/components/library/ComponentName.tsx
   // version: 1.0.0
   // guid: <generate-a-new-uuid>
   // last-edited: 2026-05-01
   ```
3. Copy the JSX block from `Library.tsx` into the new component.
4. Identify what props the block needs (state variables and callbacks it references).
5. Define a TypeScript `interface ComponentNameProps { ... }` and type the component.
6. Export the component: `export function ComponentName({ ... }: ComponentNameProps) { ... }`
7. In `Library.tsx`, import the new component and replace the extracted JSX block with `<ComponentName ... />`.
8. Run `npm --prefix web run build` — fix TypeScript errors before proceeding.

### Step 4 — Extract BookDedup subcomponents (one at a time)

Same process as Step 3 but for `web/src/pages/BookDedup.tsx` and components under
`web/src/components/dedup/`.

### Step 5 — Final build + lint

```bash
npm --prefix web run build
npm --prefix web run lint 2>&1 | grep -E 'error|warning' | grep -v 'warning' | head -20
```

Build must be clean. No new TypeScript errors.

### Step 6 — Add version headers

Bump patch version on `Library.tsx` and `BookDedup.tsx`. New component files start
at `1.0.0`.

### Step 7 — Commit and open PR

```bash
git checkout -b refactor/struct-9-frontend-component-splits
git add web/src/
git commit -m "refactor(web): split Library and BookDedup into sub-components

Extracts 9 sub-components from the two largest frontend pages:
Library.tsx → LibraryToolbar, LibraryBookGrid, LibrarySoftDeletedSection,
              LibraryDialogs
BookDedup.tsx → DedupBookTab, DedupAdvancedScanTab, DedupAuthorTab,
                DedupSeriesTab, DedupReconcileTab

No logic or state changes. Structure audit STRUCT-9.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/struct-9-frontend-component-splits
gh pr create \
  --title "refactor(web): split Library and BookDedup into sub-components" \
  --body "Extracts 9 sub-components from 2 monolithic page files. No logic changes. Structure audit STRUCT-9."
```

---

## Checklist

- [ ] `web/src/components/library/` created with 4 component files
- [ ] `web/src/components/dedup/` created with 5 component files
- [ ] `Library.tsx` and `BookDedup.tsx` reduced significantly in size
- [ ] Each extracted component has explicit TypeScript props interface
- [ ] `npm run build` clean
- [ ] No new TypeScript errors
- [ ] PR opened on branch `refactor/struct-9-frontend-component-splits`
