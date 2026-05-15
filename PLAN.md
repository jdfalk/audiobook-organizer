<!-- file: PLAN.md -->
<!-- version: 3.1.0 -->
<!-- guid: 7d8e9f10-2345-4abc-9def-0123456789ab -->
<!-- last-edited: 2026-05-15 -->

# FE-1 / PROJ-1 / PROJ-2: FilterPanel extraction + BookSummary mark-done

## Goal

PROJ-1 and PROJ-2 are already fully implemented (`BookSummary` struct in `store.go`,
`GetAllBookSummaries` in both PebbleStore and SQLiteStore, wired into the audiobooks
service). Mark them done and log them.

FE-1 requires real work: Library.tsx still owns all filter state
(`filterOpen`, `filters`, `selectedTags`, 5×available* arrays, 2×useEffects that
load filter data, 3 handlers). The filter UI components (`FilterPanel.tsx`,
`FilterSidebar.tsx`) were already extracted, but the state was not. Extract it into
`web/src/hooks/useLibraryFilters.ts` to reduce Library.tsx's bulk and make filter
logic reusable.

## Affected files

- `web/src/hooks/useLibraryFilters.ts` — new hook; moves all filter state + loading
- `web/src/pages/Library.tsx` — replace ~15 state vars + 2 useEffects with the hook
- `TODO.md` — mark PROJ-1, PROJ-2, FE-1 done
- `CHANGELOG.md` — add entries

## Steps

1. Create `useLibraryFilters` hook:
   - Accepts `{ searchParams }` for URL-seeded initial state
   - Returns `{ filterOpen, setFilterOpen, filters, setFilters, selectedTags, handleTagFilterChange, handleFiltersChange, refreshTags, availableAuthors, availableSeries, availableGenres, availableLanguages, availableTags, getActiveFilterCount }`
   - Moves the 2 useEffects that load facets + tags

2. Update Library.tsx:
   - Replace the ~15 filter state declarations + useEffects with `const { ... } = useLibraryFilters({ searchParams })`
   - Remove now-dead `getActiveFilterCount` local function (moved into hook)

3. Mark PROJ-1, PROJ-2, FE-1 done in TODO.md

4. Append CHANGELOG entries

5. Commit: `refactor(fe): extract useLibraryFilters hook from Library.tsx (FE-1)` and
   `chore(todo): mark PROJ-1/2 and FE-1 done`

## Test strategy

- `make test-all` (Vitest + Go)
- `cd web && npx tsc --noEmit` to verify no type regressions

## Rollback

- `git reset HEAD~2` in the worktree
