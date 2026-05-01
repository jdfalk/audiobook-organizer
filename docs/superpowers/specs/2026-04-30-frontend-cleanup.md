<!-- file: docs/superpowers/specs/2026-04-30-frontend-cleanup.md -->
<!-- version: 1.0.0 -->
<!-- guid: b3c4d5e6-f7a8-9b0c-1d2e-3f4a5b6c7d8e -->
<!-- last-edited: 2026-04-30 -->

# Frontend Code Quality Cleanup

**Status:** Draft — awaiting implementation
**Scope:** `web/src/`
**Related specs:** [`2026-04-30-library-component-split.md`](./2026-04-30-library-component-split.md), [`2026-04-30-settings-component-split.md`](./2026-04-30-settings-component-split.md)

---

## Problem

Six frontend code quality issues were identified in the 2026-04-30 audit:

**F-4 — 108 `console.log` calls in production code:**
Debug `console.log` calls leak internal state to any user who opens DevTools.
They also flood browser consoles, making legitimate error output harder to find.

**F-5 — No per-route error boundaries:**
A JavaScript error in any page component propagates to the root ErrorBoundary (if one
exists) and unmounts the entire application. Users see a blank page with no recovery
path. Per-route boundaries limit blast radius to the failing page.

**F-6 — localStorage key strings duplicated:**
localStorage keys like `'library_page'`, `'library_view_mode'` appear as bare string
literals in multiple files. A typo in one place silently breaks persistence.

**F-8 — Coverage thresholds too low:**
`web/vite.config.ts` has coverage thresholds at `statements: 15, lines: 15, branches: 10,
functions: 15`. These are so low that most new code is effectively untested.

---

## Core Rule / Goal

> **Production frontend code must have zero debug `console.log` calls.
> Every route must be wrapped in an error boundary.
> localStorage keys must be centralised constants.
> Coverage thresholds must reflect actual quality targets.**

---

## Approach

### FE-7 — Remove console.log

Remove all `console.log`, `console.warn`, `console.error` from non-test production code
except intentional error reporting in catch blocks. Do NOT remove `console.error` in
error boundary `componentDidCatch` or similar error-reporting code.

### FE-8 — Per-route error boundaries

Wrap each route's component in a `<PageErrorBoundary>` in `App.tsx`. Create a
reusable `PageErrorBoundary` component if one doesn't already exist.

### FE-9 — Centralise localStorage keys

Create `web/src/constants/storageKeys.ts` with a `STORAGE_KEYS` const object.
Replace all bare string literals in `localStorage.getItem/setItem/removeItem` calls
with the constants.

### FE-10 — Raise coverage thresholds

Raise Vitest coverage thresholds from 15%/10% to 40%/30%. Add appropriate
`exclude` patterns for generated files and test utilities.

---

## Acceptance Criteria

- [ ] `grep -rn 'console\.log' web/src/ | grep -v test | wc -l` returns 0.
- [ ] Every route in `App.tsx` is wrapped in an error boundary.
- [ ] `web/src/constants/storageKeys.ts` exists with all key constants.
- [ ] No bare localStorage key strings remain in `Library.tsx` or `Settings.tsx`.
- [ ] Vitest coverage thresholds are at least `statements: 40, lines: 40, branches: 30, functions: 40`.
- [ ] `npx --prefix web tsc --noEmit` passes.
- [ ] `make test-frontend` passes (or documents the coverage gap clearly).

---

## Related Bot-Tasks

- [`2026-04-30-fe-7-console-log.md`](../bot-tasks/2026-04-30-fe-7-console-log.md) — FE-7
- [`2026-04-30-fe-8-error-boundaries.md`](../bot-tasks/2026-04-30-fe-8-error-boundaries.md) — FE-8
- [`2026-04-30-fe-9-localstorage-keys.md`](../bot-tasks/2026-04-30-fe-9-localstorage-keys.md) — FE-9
- [`2026-04-30-fe-10-coverage.md`](../bot-tasks/2026-04-30-fe-10-coverage.md) — FE-10
