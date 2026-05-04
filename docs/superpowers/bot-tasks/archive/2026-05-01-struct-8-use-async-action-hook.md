<!-- file: docs/superpowers/bot-tasks/2026-05-01-struct-8-use-async-action-hook.md -->
<!-- version: 1.0.0 -->
<!-- guid: g7h8i9j0-k1l2-3456-mnop-789012345678 -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: STRUCT-8 — Add `useAsyncAction` React hook

**TODO ID:** STRUCT-8
**Audience:** burndown bot
**Branch:** `refactor/struct-8-use-async-action-hook`
**PR title:** `refactor(web): add useAsyncAction hook to eliminate loading boilerplate`

---

## What This Task Does

Creates `web/src/hooks/useAsyncAction.ts` with a reusable hook that replaces
148 identical loading-state patterns across React components.

**Evidence:** `grep -rc 'setLoading(true)' web/src/` returns 148 hits.

The repeated pattern:
```tsx
const [loading, setLoading] = useState(false);
const [error, setError] = useState<string | null>(null);

const handleSomething = async () => {
  setLoading(true);
  try {
    await someApiCall();
  } catch (e) {
    setError(e instanceof Error ? e.message : String(e));
  } finally {
    setLoading(false);
  }
};
```

---

## What NOT to Do

- **Do NOT** replace call sites in this PR — just create the hook.
- **Do NOT** modify any existing components.
- **Do NOT** change `web/src/lib/api.ts`.

---

## Step-by-step

### Step 1 — Check existing hooks for naming conflicts

```bash
ls web/src/hooks/
grep -rn 'useAsyncAction\|useAsync\b' web/src/ | grep -v node_modules
```

Expected: no `useAsyncAction` exists yet. If a similar hook exists, extend it.

### Step 2 — Read 2 concrete examples of the pattern

```bash
grep -B2 -A12 'setLoading(true)' web/src/pages/Library.tsx | head -50
grep -B2 -A12 'setLoading(true)' web/src/pages/BookDedup.tsx | head -50
```

This confirms the exact shape of the pattern before writing the hook.

### Step 3 — Create `web/src/hooks/useAsyncAction.ts`

```typescript
// file: web/src/hooks/useAsyncAction.ts
// version: 1.0.0
// last-edited: 2026-05-01
// guid: h8i9j0k1-l2m3-4567-nopq-890123456789

import { useState, useCallback } from 'react';

interface AsyncActionState {
  loading: boolean;
  error: string | null;
}

interface UseAsyncActionReturn<T> extends AsyncActionState {
  run: (...args: Parameters<() => Promise<T>>) => Promise<T | undefined>;
  clearError: () => void;
}

/**
 * useAsyncAction wraps an async function with loading and error state.
 * Eliminates repeated useState(false) / try-finally loading boilerplate.
 *
 * @example
 * const { run: handleSave, loading, error } = useAsyncAction(async () => {
 *   await api.saveBook(book);
 * });
 */
function useAsyncAction<T = void>(
  fn: (...args: unknown[]) => Promise<T>
): UseAsyncActionReturn<T> {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const run = useCallback(
    async (...args: unknown[]): Promise<T | undefined> => {
      setLoading(true);
      setError(null);
      try {
        return await fn(...args);
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
        return undefined;
      } finally {
        setLoading(false);
      }
    },
    [fn]
  );

  const clearError = useCallback(() => setError(null), []);

  return { loading, error, run, clearError };
}

export default useAsyncAction;
export type { AsyncActionState, UseAsyncActionReturn };
```

### Step 4 — Type-check

```bash
npx --prefix web tsc --noEmit 2>&1 | grep -E 'error|Error' | grep -v node_modules | head -20
```

Expected: no new errors introduced.

### Step 5 — Bump version header

File is new at version `1.0.0`. No bump needed.

### Step 6 — Commit and open PR

```bash
git checkout -b refactor/struct-8-use-async-action-hook
git add web/src/hooks/useAsyncAction.ts
git commit -m "refactor(web): add useAsyncAction hook to eliminate loading boilerplate

Adds useAsyncAction.ts hook wrapping async functions with loading/error
state. Replaces 148 repeated useState(false) + try/finally patterns.
Call-site adoption in follow-up STRUCT-8b.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/struct-8-use-async-action-hook
gh pr create \
  --title "refactor(web): add useAsyncAction hook to eliminate loading boilerplate" \
  --body "Adds useAsyncAction.ts. Part of STRUCT-8 structure audit. 148 call sites to adopt in STRUCT-8b."
```

---

## Checklist

- [ ] `web/src/hooks/useAsyncAction.ts` created
- [ ] `npx --prefix web tsc --noEmit` clean
- [ ] No naming conflict with existing hooks
- [ ] PR opened on branch `refactor/struct-8-use-async-action-hook`
