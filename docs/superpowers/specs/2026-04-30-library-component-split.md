<!-- file: docs/superpowers/specs/2026-04-30-library-component-split.md -->
<!-- version: 1.0.0 -->
<!-- guid: 0a1b2c3d-4e5f-6a7b-8c9d-0e1f2a3b4c5d -->
<!-- last-edited: 2026-04-30 -->

# Library.tsx Component Split

**Status:** Draft — awaiting implementation
**Scope:** `web/src/pages/Library.tsx`, `web/src/components/library/`
**Related specs:** [`2026-04-30-settings-component-split.md`](./2026-04-30-settings-component-split.md), [`2026-04-30-frontend-cleanup.md`](./2026-04-30-frontend-cleanup.md)

---

## Problem

**F-1 — Library.tsx is 3,372 lines:**
`web/src/pages/Library.tsx` contains 90 `useState` hooks and 17 `useEffect` hooks in
a single component. This makes it:

- Impossible to unit-test individual pieces in isolation.
- Fragile: a small change in one section can break unrelated sections.
- Slow to parse for IDEs and type checkers.
- A merge-conflict magnet: every Library change touches the same file.

**F-3 — No component-level TypeScript tests for Library:**
Because Library.tsx is a monolith, there are no isolated unit tests for filter logic,
book grid rendering, or batch operations.

---

## Core Rule / Goal

> **Each logical UI section of Library.tsx must become its own component in
> `web/src/components/library/`. Library.tsx becomes a thin coordinator that
> composes the sub-components.**

Behaviour must be identical after each extraction. Extract one component per PR.

---

## Sub-Component Breakdown

| Task | Component | What it contains |
|------|-----------|-----------------|
| FE-1 | `FilterPanel` | Search query, genre/author/sort filters, filter sidebar JSX |
| FE-2 | `BookGrid` | Book list/grid rendering JSX, display-only state, selection state |
| FE-3 | `BatchOperationsToolbar` | Batch edit, batch tag, bulk organize, bulk rate UI and handlers |

---

## Approach

For each extraction:

1. Identify all `useState` / `useCallback` / `useMemo` hooks that belong exclusively
   to the component being extracted.
2. Identify the JSX block that renders that section.
3. Create `web/src/components/library/<ComponentName>.tsx`.
4. Move the hooks and JSX into the new file. State that is shared with other sections
   becomes props (passed down from Library.tsx) or is placed in a Zustand slice
   (if one already exists for this concern — follow existing patterns).
5. In Library.tsx, replace the moved JSX block with `<ComponentName ... />`.
6. Run `npx --prefix web tsc --noEmit` to verify zero type errors.

---

## What Does NOT Change

- Filter logic, book display logic, batch operation logic — only the code organisation changes.
- API calls — all fetch logic remains where it is until a separate data-layer refactor.
- URL state / localStorage persistence — these follow the hooks into the sub-component.

---

## Acceptance Criteria

- [ ] `web/src/components/library/FilterPanel.tsx` exists.
- [ ] `web/src/components/library/BookGrid.tsx` exists.
- [ ] `web/src/components/library/BatchOperationsToolbar.tsx` exists.
- [ ] `npx --prefix web tsc --noEmit` passes after each extraction.
- [ ] Library.tsx line count is materially reduced after all three extractions.
- [ ] UI behaviour is identical to before (manual smoke test or existing E2E pass).

---

## Related Bot-Tasks

- [`2026-04-30-fe-1-filter-panel.md`](../bot-tasks/2026-04-30-fe-1-filter-panel.md) — FE-1
- [`2026-04-30-fe-2-book-grid.md`](../bot-tasks/2026-04-30-fe-2-book-grid.md) — FE-2
- [`2026-04-30-fe-3-batch-toolbar.md`](../bot-tasks/2026-04-30-fe-3-batch-toolbar.md) — FE-3
