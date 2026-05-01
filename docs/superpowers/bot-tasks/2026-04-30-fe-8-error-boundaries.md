<!-- file: docs/superpowers/bot-tasks/2026-04-30-fe-8-error-boundaries.md -->
<!-- version: 1.0.0 -->
<!-- guid: b6c7d8e9-f0a1-2345-bcde-678901234fa5 -->
<!-- last-edited: 2026-04-30 -->

# BOT TASK: FE-8 — Add Error Boundaries to Major Page Components

**TODO ID:** FE-8
**Audience:** burndown bot
**Branch:** `fix/frontend-error-boundaries`
**PR title:** `fix(web): add error boundaries to major page components`

---

## What This Task Does

Wraps the three major page components (`Library`, `Settings`, `Player` or
equivalent) with a React Error Boundary so that an unhandled render error in one
page does not crash the entire app.

---

## What NOT to Do

- **Do NOT use** a third-party library for error boundaries — React's class
  component pattern is sufficient.
- **Do NOT add** error boundaries to every tiny component — only page-level.
- **Do NOT swallow** errors silently — log them with `console.error`.
- **Do NOT change** any page logic.

---

## Read First

1. Find the top-level router or `App.tsx` where pages are rendered:

```bash
grep -rn 'Library\|Settings\|Player\|Route\|<Routes' web/src/ | \
  grep -v node_modules | grep -v test | head -20
```

2. Check if an `ErrorBoundary` component already exists:

```bash
find web/src -name 'ErrorBoundary*' | head -5
```

---

## Steps

### Step 1 — Create the ErrorBoundary component

Create `web/src/components/ErrorBoundary.tsx`:

```tsx
import React, { Component, ReactNode } from 'react';
import { Box, Typography, Button } from '@mui/material';

interface Props {
  children: ReactNode;
  fallbackMessage?: string;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error('ErrorBoundary caught:', error, info.componentStack);
  }

  render() {
    if (this.state.hasError) {
      return (
        <Box p={4} textAlign="center">
          <Typography variant="h5" gutterBottom>
            {this.props.fallbackMessage ?? 'Something went wrong'}
          </Typography>
          <Typography variant="body2" color="text.secondary" gutterBottom>
            {this.state.error?.message}
          </Typography>
          <Button onClick={() => this.setState({ hasError: false, error: null })}>
            Try again
          </Button>
        </Box>
      );
    }
    return this.props.children;
  }
}
```

### Step 2 — Wrap page components in App.tsx (or router)

In `App.tsx` or wherever routes are defined:

```tsx
import { ErrorBoundary } from './components/ErrorBoundary';

// Before:
<Route path="/library" element={<Library />} />

// After:
<Route path="/library" element={
  <ErrorBoundary fallbackMessage="Library failed to load">
    <Library />
  </ErrorBoundary>
} />
```

Apply the same pattern to Settings, Player, and any other top-level page.

### Step 3 — Verify

```bash
cd /Users/jdfalk/.worktrees/audiobook-eval/web
npx tsc --noEmit 2>&1 | tail -10
npm run build 2>&1 | tail -10
```

### Step 4 — Commit and open PR

```bash
git checkout -b fix/frontend-error-boundaries
git add web/src/components/ErrorBoundary.tsx web/src/App.tsx
git commit -m "fix(web): add error boundaries to major page components

Creates ErrorBoundary class component and wraps Library, Settings,
and Player routes. Unhandled render errors now show a fallback UI
instead of crashing the entire app.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/frontend-error-boundaries
gh pr create \
  --title "fix(web): add error boundaries to major page components" \
  --body "Wraps page routes in ErrorBoundary. Render errors no longer crash full app. Frontend cleanup FE-8."
```

---

## Checklist

- [ ] `ErrorBoundary.tsx` created with `getDerivedStateFromError` and `componentDidCatch`
- [ ] Error logged with `console.error` in `componentDidCatch`
- [ ] Fallback UI shows error message and a "Try again" button
- [ ] Library, Settings, and Player pages wrapped with `<ErrorBoundary>`
- [ ] `npx tsc --noEmit` passes
- [ ] `npm run build` succeeds
- [ ] PR opened with correct branch and title
