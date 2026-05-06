<!-- file: docs/superpowers/bot-tasks/2026-04-30-fe-9-localstorage-keys.md -->
<!-- version: 1.0.0 -->
<!-- guid: c7d8e9f0-a1b2-3456-cdef-789012345ab6 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: FE-9 — Centralise localStorage Keys as Constants

**TODO ID:** FE-9
**Audience:** burndown bot
**Branch:** `fix/frontend-localstorage-keys`
**PR title:** `fix(web): centralise localStorage keys as typed constants`

---

## What This Task Does

Finds all inline `localStorage.getItem('...')` and `localStorage.setItem('...', ...)`
calls and replaces the string keys with typed constants from a new
`web/src/lib/storageKeys.ts` file. This prevents key typos and makes it easy to
find all localStorage usage.

---

## What NOT to Do

- **Do NOT change** what is stored or retrieved — only the key strings.
- **Do NOT add** a localStorage abstraction layer — just constants.
- **Do NOT change** session storage (`sessionStorage`) in this PR.

---

## Read First

```bash
grep -rn 'localStorage\.' web/src/ | grep -v node_modules | grep -v test | head -30
```

List all unique key strings used (the first argument to `getItem`/`setItem`/
`removeItem`).

---

## Steps

### Step 1 — Enumerate all localStorage keys

```bash
grep -rn "localStorage\.\(get\|set\|remove\)Item(" web/src/ | \
  grep -oP "(?<=getItem\(|setItem\(|removeItem\()['\"][^'\"]+['\"]" | sort -u
```

Collect all unique key strings.

### Step 2 — Create storageKeys.ts

Create `web/src/lib/storageKeys.ts`:

```ts
// Centralised localStorage key constants.
// All keys are prefixed with 'ao:' (audiobook-organizer) to avoid clashes.
export const STORAGE_KEYS = {
  THEME: 'ao:theme',
  LIBRARY_SORT: 'ao:library:sort',
  LIBRARY_FILTER: 'ao:library:filter',
  PLAYBACK_VOLUME: 'ao:playback:volume',
  PLAYBACK_SPEED: 'ao:playback:speed',
  // Add all discovered keys here, mapping old name → 'ao:...' prefixed name
} as const;

export type StorageKey = typeof STORAGE_KEYS[keyof typeof STORAGE_KEYS];
```

If existing keys already have a prefix (e.g., `audiobook-volume`), keep the same
value string to avoid data loss for existing users.

### Step 3 — Replace inline strings

For each `localStorage.getItem('theme')` (for example), replace with:

```ts
import { STORAGE_KEYS } from '../lib/storageKeys';

// Before:
const theme = localStorage.getItem('theme');

// After:
const theme = localStorage.getItem(STORAGE_KEYS.THEME);
```

Apply to `setItem` and `removeItem` calls too.

### Step 4 — Verify no raw strings remain

```bash
grep -rn "localStorage\.\(get\|set\|remove\)Item(['\"]" web/src/ | \
  grep -v storageKeys | grep -v test
```

This should return zero results.

### Step 5 — Build and test

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval/web
npx tsc --noEmit 2>&1 | tail -10
npm run build 2>&1 | tail -10
npm test -- --run 2>&1 | tail -20
```

### Step 6 — Commit and open PR

```bash
git checkout -b fix/frontend-localstorage-keys
git add web/src/lib/storageKeys.ts web/src/
git commit -m "fix(web): centralise localStorage keys as typed constants

Creates storageKeys.ts with STORAGE_KEYS constants (ao: prefixed).
All localStorage.getItem/setItem calls now use constants.
Prevents key typos and centralises storage key management.
Existing key values are unchanged to preserve user data.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/frontend-localstorage-keys
gh pr create \
  --title "fix(web): centralise localStorage keys as typed constants" \
  --body "Creates STORAGE_KEYS constants. Replaces all inline localStorage key strings. No data migration needed — key values unchanged. Frontend cleanup FE-9."
```

---

## Checklist

- [ ] `web/src/lib/storageKeys.ts` created with all keys as constants
- [ ] Key values unchanged from existing strings (no user data migration)
- [ ] All `getItem`/`setItem`/`removeItem` calls use `STORAGE_KEYS.*`
- [ ] No raw string key literals remain in production code
- [ ] `npx tsc --noEmit` passes
- [ ] `npm run build` succeeds
- [ ] PR opened with correct branch and title
