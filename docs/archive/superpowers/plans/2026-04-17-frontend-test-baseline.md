<!-- file: docs/superpowers/plans/2026-04-17-frontend-test-baseline.md -->
<!-- version: 1.0.0 -->
<!-- guid: 542cd27b-0857-4522-821d-c68f4e681ef4 -->
<!-- last-edited: 2026-04-16 -->

# 5.6: Frontend Test Coverage Baseline — Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Backlog item:** 5.6 — Frontend test coverage baseline
**Spec:** None — this plan is self-contained.
**Depends on:** Nothing

## Overview

The React/TypeScript frontend (`web/src/`) uses Vite + Vitest with jsdom and React Testing Library. The infrastructure is already in place: `vitest` is in devDependencies, `web/vite.config.ts` has a `test` block configured, and `web/src/test/setup.ts` provides mocks for `matchMedia`, `IntersectionObserver`, `localStorage`, `EventSource`, and `fetch`. There are ~14 existing test files covering pages (Library, BookDetail, Works) and utilities (searchParser, columnDefinitions, api, eventSourceManager).

This plan fills the gaps: adds tests for key interactive components (SearchBar, ReadStatusChip, FilterSidebar), adds tests for the Playlists and Dashboard pages, creates shared test utilities, and integrates frontend test coverage into CI.

## Prerequisites

- Existing: `vitest`, `@testing-library/react`, `@testing-library/jest-dom`, `@testing-library/user-event`, `jsdom` all in `web/package.json`
- Existing: `web/src/test/setup.ts` with global mocks
- Existing: `web/vite.config.ts` test config with `globals: true`, `environment: 'jsdom'`, `setupFiles: ['./src/test/setup.ts']`

---

### Task 1: Shared test utilities + render helper (1 PR)

**Goal:** Create reusable test utilities that wrap React Testing Library's `render` with common providers (Router, MUI theme, Zustand stores).

**Files:**
- Create: `web/src/test/renderWithProviders.tsx` — custom render function
- Create: `web/src/test/factories.ts` — factory functions for test data (Book, Author, Series, Playlist)
- Modify: `web/src/test/setup.ts` — add any missing global mocks discovered during this task

`renderWithProviders`:
- [ ] Wraps component in `MemoryRouter` (from react-router-dom) with configurable `initialEntries`
- [ ] Wraps in MUI `ThemeProvider` with the app's theme
- [ ] Accepts optional `route` parameter for URL-dependent components
- [ ] Re-exports everything from `@testing-library/react` plus the custom `render`

Factory functions:
- [ ] `buildBook(overrides?)` — returns a Book object with sensible defaults (title, author, filePath, etc.)
- [ ] `buildAuthor(overrides?)` — returns an Author with name
- [ ] `buildSeries(overrides?)` — returns a Series with name and book count
- [ ] `buildPlaylist(overrides?)` — returns a Playlist (static or smart)
- [ ] All factories use incrementing IDs to avoid collisions

**Acceptance criteria:**
- [ ] `renderWithProviders(<Library />)` renders without errors
- [ ] Factory objects pass TypeScript type checking
- [ ] Existing tests still pass after this change

---

### Task 2: SearchBar component tests (1 PR)

**Goal:** Test the SearchBar component's input handling, search submission, and keyboard shortcuts.

**Files:**
- Create: `web/src/components/audiobooks/SearchBar.test.tsx`

Tests to write:
- [ ] Renders an input field with placeholder text
- [ ] Typing updates the input value
- [ ] Pressing Enter triggers the search callback with the current input value
- [ ] Clearing the input (clicking X or clearing text) triggers search with empty string
- [ ] Debounce: typing rapidly fires the search callback only once after the debounce delay
- [ ] Focus: component receives focus when keyboard shortcut is triggered (if applicable)
- [ ] Empty state: submitting an empty search is handled gracefully

**Acceptance criteria:**
- [ ] All tests pass with `npm run test -- SearchBar`
- [ ] Tests use `userEvent` for realistic interaction simulation
- [ ] No flaky timing-dependent tests (use `vi.useFakeTimers()` for debounce tests)

---

### Task 3: ReadStatusChip component tests (1 PR)

**Goal:** Test the ReadStatusChip displays correct status and handles click interactions.

**Files:**
- Create: `web/src/components/audiobooks/ReadStatusChip.test.tsx`

Tests to write:
- [ ] Renders "Unread" chip for books with no read status
- [ ] Renders "Reading" chip with correct color for in-progress books
- [ ] Renders "Finished" chip with correct color for completed books
- [ ] Clicking the chip opens a status menu/dropdown (if interactive)
- [ ] Selecting a new status fires the onChange callback with the correct value
- [ ] Chip displays correct MUI color variant for each status

**Acceptance criteria:**
- [ ] All status values are covered
- [ ] Tests verify both visual state (text content, colors) and behavior (click handlers)

---

### Task 4: FilterSidebar component tests (1 PR)

**Goal:** Test the FilterSidebar renders filter options and communicates selections back.

**Files:**
- Create: `web/src/components/audiobooks/FilterSidebar.test.tsx`

Tests to write:
- [ ] Renders filter categories (author, series, format, read status, genre)
- [ ] Selecting a filter option calls the onFilterChange callback with the correct filter object
- [ ] Multiple filters can be active simultaneously
- [ ] "Clear all" button resets all filters and calls onFilterChange with empty filter
- [ ] Filter counts are displayed next to each option (if applicable)
- [ ] Collapsed/expanded state of filter sections persists correctly

**Acceptance criteria:**
- [ ] Tests cover the main filter interactions without testing implementation details
- [ ] Uses `renderWithProviders` from task 1

---

### Task 5: Playlists page tests (1 PR)

**Goal:** Test the Playlists page renders playlist list and handles CRUD interactions.

**Files:**
- Create: `web/src/pages/Playlists.test.tsx`

Tests to write:
- [ ] Renders loading state while fetching playlists
- [ ] Renders empty state when no playlists exist ("No playlists yet" message)
- [ ] Renders list of playlists with name, type (static/smart), and book count
- [ ] "Create Playlist" button is present and clickable
- [ ] Clicking a playlist navigates to the PlaylistDetail page (verify router navigation)
- [ ] Delete button on a playlist shows confirmation dialog
- [ ] Error state: displays error message when API call fails

Mock setup:
- [ ] Mock `fetch` to return playlist data from factories
- [ ] Mock `useNavigate` to verify navigation calls

**Acceptance criteria:**
- [ ] Tests cover loading, empty, populated, and error states
- [ ] Navigation assertions use `MemoryRouter` history

---

### Task 6: Dashboard page tests (1 PR)

**Goal:** Test the Dashboard page renders system status and library statistics.

**Files:**
- Create: `web/src/pages/Dashboard.test.tsx`

Tests to write:
- [ ] Renders library stats (book count, author count, series count)
- [ ] Renders system status indicator (ok/degraded/error)
- [ ] Renders recent activity entries (if displayed)
- [ ] Renders storage usage information
- [ ] Loading state shows skeleton/spinner
- [ ] Error state when system status API fails

Mock setup:
- [ ] Mock `/api/v1/system/status` (already partially mocked in setup.ts)
- [ ] Mock additional dashboard-specific endpoints as needed

**Acceptance criteria:**
- [ ] Dashboard renders without errors using mocked API responses
- [ ] Key statistics are visible in the rendered output

---

### Task 7: CI integration + coverage threshold (1 PR)

**Goal:** Ensure `make test-all` runs frontend Vitest tests and enforce a minimum coverage threshold.

**Files:**
- Modify: `Makefile` — verify `test-all` target runs `cd web && npm run test -- --run` (the `--run` flag exits after one pass instead of watch mode)
- Modify: `web/vite.config.ts` — add coverage thresholds
- Modify: `.github/workflows/ci.yml` (or equivalent) — ensure frontend tests run in CI

Changes:
- [ ] Verify `make test-all` runs `npm run test -- --run` in the `web/` directory (add if missing)
- [ ] Add coverage thresholds to `web/vite.config.ts` test.coverage block:
  ```typescript
  coverage: {
    provider: 'v8',
    reporter: ['text', 'json', 'html'],
    thresholds: {
      statements: 15,
      branches: 10,
      functions: 15,
      lines: 15,
    },
  },
  ```
- [ ] Start with low thresholds (15%) to establish a baseline — these will be increased as coverage grows
- [ ] Add a `make test-frontend` target for running frontend tests independently
- [ ] Verify `make ci` includes frontend tests in its pass/fail criteria

**Acceptance criteria:**
- [ ] `make test-all` runs both Go and frontend tests and reports combined pass/fail
- [ ] `make test-frontend` runs only frontend tests
- [ ] Coverage thresholds are enforced — dropping below fails the test run
- [ ] CI pipeline runs frontend tests on every PR

---

### Estimated effort

| Task | Size | Depends on |
|------|------|------------|
| 1 (test utilities) | S | -- |
| 2 (SearchBar) | S | 1 |
| 3 (ReadStatusChip) | S | 1 |
| 4 (FilterSidebar) | S | 1 |
| 5 (Playlists page) | M | 1 |
| 6 (Dashboard page) | M | 1 |
| 7 (CI integration) | S | 1-6 |
| **Total** | ~7 PRs, M overall | |

### Critical path

Task 1 (test utilities) must be done first. Tasks 2-6 are independent and can all run in parallel. Task 7 should be done last to verify all tests work together and set the coverage baseline.

### Existing test inventory (for reference)

These files already exist and should not be duplicated:
- `web/src/App.test.tsx`
- `web/src/pages/Library.helpers.test.ts`
- `web/src/pages/Library.metadata.test.ts`
- `web/src/pages/Library.importFile.test.tsx`
- `web/src/pages/Library.bulkFetch.test.tsx`
- `web/src/pages/Works.test.tsx`
- `web/src/pages/BookDetail.files-history.test.tsx`
- `web/src/pages/BookDetail.unlock.test.tsx`
- `web/src/components/filemanager/ImportPathCard.test.tsx`
- `web/src/services/api.test.ts`
- `web/src/services/eventSourceManager.test.ts`
- `web/src/config/__tests__/columnDefinitions.test.ts`
- `web/src/hooks/__tests__/useColumnConfig.test.ts`
- `web/src/utils/__tests__/searchParser.test.ts`
