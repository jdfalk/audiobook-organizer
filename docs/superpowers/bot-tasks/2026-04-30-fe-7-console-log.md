<!-- file: docs/superpowers/bot-tasks/2026-04-30-fe-7-console-log.md -->
<!-- version: 1.0.0 -->
<!-- guid: a5b6c7d8-e9f0-1234-abcd-567890123ef4 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: FE-7 — Remove console.log Calls from Frontend

**TODO ID:** FE-7
**Audience:** burndown bot
**Branch:** `fix/frontend-remove-console-logs`
**PR title:** `fix(web): remove console.log calls from production frontend code`

---

## What This Task Does

Removes all bare `console.log(...)` calls from production frontend TypeScript/JSX
files. `console.error` for caught errors may stay. Debug logging must use a
conditional check (`if (import.meta.env.DEV)`) or be removed.

---

## What NOT to Do

- **Do NOT remove** `console.error(...)` calls in error handlers.
- **Do NOT remove** `console.warn(...)` calls that warn about real issues.
- **Do NOT add** a logging library — just remove or guard the debug logs.
- **Do NOT change** any logic.

---

## Read First

```bash
grep -rn 'console\.log' web/src/ | grep -v node_modules | grep -v '.test.' | head -30
```

Count the occurrences. If > 20, they need systematic removal.

---

## Steps

### Step 1 — List all console.log calls

```bash
grep -rn 'console\.log' web/src/ | grep -v '\.test\.\|\.spec\.'
```

For each, decide:
- Is it debug output? → Remove it.
- Is it a performance measurement? → Remove or guard with `if (import.meta.env.DEV)`.
- Is it an actual error? → Change to `console.error`.

### Step 2 — Remove debug console.log calls

For simple debug logs (e.g., `console.log('fetching books', query)`), delete the
line entirely.

For logs that may be useful in development, guard them:
```ts
// Before:
console.log('API response', data);

// After (if worth keeping for dev):
if (import.meta.env.DEV) {
  console.log('API response', data);
}
```

### Step 3 — Keep console.error in catch blocks

```ts
// This stays:
} catch (err) {
  console.error('Failed to load books', err);
}
```

### Step 4 — Verify no console.log remains in production code

```bash
grep -rn 'console\.log' web/src/ | grep -v '\.test\.\|\.spec\.\|if.*import\.meta\.env\.DEV'
```

This should return zero results (or only guarded ones).

### Step 5 — Build and test

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval/web
npx tsc --noEmit 2>&1 | tail -10
npm run build 2>&1 | tail -10
npm test -- --run 2>&1 | tail -20
```

### Step 6 — Commit and open PR

```bash
git checkout -b fix/frontend-remove-console-logs
git add web/src/
git commit -m "fix(web): remove console.log calls from production frontend code

Removes debug console.log calls from production code. console.error
in catch blocks retained. Debug-only logs guarded with
import.meta.env.DEV where appropriate.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/frontend-remove-console-logs
gh pr create \
  --title "fix(web): remove console.log calls from production frontend code" \
  --body "Cleans up debug console.log calls. console.error retained. Frontend cleanup FE-7."
```

---

## Checklist

- [ ] No bare `console.log` calls in `web/src/` production code
- [ ] `console.error` in catch blocks retained
- [ ] Dev-only logs guarded with `import.meta.env.DEV`
- [ ] `npx tsc --noEmit` passes
- [ ] `npm run build` succeeds
- [ ] `npm test` passes
- [ ] PR opened with correct branch and title
