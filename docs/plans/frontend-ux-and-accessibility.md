<!-- file: docs/plans/frontend-ux-and-accessibility.md -->
<!-- version: 2.0.0 -->
<!-- guid: c8d9e0f1-a2b3-4c5d-6e7f-8a9b0c1d2e3f -->
<!-- last-edited: 2026-01-31 -->

# Frontend, UX, and Accessibility

## Overview

UI polish, new interactive components, accessibility compliance, and
internationalization. Covers everything from dark mode to mobile responsiveness
to screen reader support.

---

## P1 — High Priority

### Delete/Purge Flow Refinement

Revisit Book Detail delete workflows for correctness and safety:

- Soft delete + block-hash verification end-to-end
- State transition validation (imported → organized → deleted)

---

## P2 — Medium Priority UX Polish

### Global Notification / Toast System

Consistent success and error feedback across all pages. Currently each page
handles feedback independently with local `<Snackbar>` state (see
`BookDetail.tsx` line 60–63 and `Library.tsx`). The existing Zustand store at
`web/src/stores/useAppStore.ts` already defines a `Notification` interface and
`addNotification`/`removeNotification` actions but nothing renders them.

**File: `web/src/components/toast/ToastProvider.tsx`** — create this file.

```tsx
// file: web/src/components/toast/ToastProvider.tsx
import React, { createContext, useContext, useCallback, ReactNode } from 'react';
import { Snackbar, Alert, Stack } from '@mui/material';
import { useAppStore } from '../../stores/useAppStore';

// ---------------------------------------------------------------
// Context: exposes a single `toast()` function to any component.
// ---------------------------------------------------------------
interface ToastContextType {
  toast: (message: string, severity?: 'success' | 'error' | 'warning' | 'info') => void;
}

const ToastContext = createContext<ToastContextType>({ toast: () => {} });

export function useToast() {
  return useContext(ToastContext);
}

// ---------------------------------------------------------------
// Provider: reads notifications[] from Zustand, renders a stacked
// Snackbar.  Place this ONCE, wrapping <App /> in main.tsx.
// ---------------------------------------------------------------
interface ToastProviderProps {
  children: ReactNode;
}

export function ToastProvider({ children }: ToastProviderProps) {
  const notifications = useAppStore((s) => s.notifications);
  const addNotification = useAppStore((s) => s.addNotification);
  const removeNotification = useAppStore((s) => s.removeNotification);

  const toast = useCallback(
    (message: string, severity: 'success' | 'error' | 'warning' | 'info' = 'info') => {
      addNotification(message, severity);
    },
    [addNotification]
  );

  return (
    <ToastContext.Provider value={{ toast }}>
      {children}

      {/* Anchor: bottom-left stack.  Each notification is its own Snackbar
          so they don't clobber each other.  The anchorOrigin + style below
          produces a vertical stack growing upward. */}
      <Stack
        spacing={1}
        sx={{ position: 'fixed', bottom: 16, left: 16, zIndex: 1400 }}
      >
        {notifications.map((n, index) => (
          <Snackbar
            key={n.id}
            open
            autoHideDuration={4500}
            onClose={() => removeNotification(n.id)}
            // Stack offset: each toast shifts up by ~60px
            sx={{ position: 'relative', bottom: 'auto', left: 'auto' }}
          >
            <Alert
              severity={n.severity}
              onClose={() => removeNotification(n.id)}
              variant="filled"
              sx={{ width: '100%', minWidth: 280, maxWidth: 420 }}
            >
              {n.message}
            </Alert>
          </Snackbar>
        ))}
      </Stack>
    </ToastContext.Provider>
  );
}
```

**File: `web/src/main.tsx`** — wrap `<App />` with `<ToastProvider>`.
Insert the import and wrap the existing `app` constant:

```tsx
// Add import at top:
import { ToastProvider } from './components/toast/ToastProvider';

// Wrap the existing app tree — place ToastProvider inside ThemeProvider
// so the Alert component picks up the MUI theme palette:
const app = (
  <ErrorBoundary>
    <BrowserRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        <ToastProvider>
          <App />
        </ToastProvider>
      </ThemeProvider>
    </BrowserRouter>
  </ErrorBoundary>
);
```

**Usage — in any page or component:**

```tsx
import { useToast } from '../components/toast/ToastProvider';

export function Settings() {
  const { toast } = useToast();

  const handleSave = async () => {
    try {
      await api.updateConfig(pendingConfig);
      toast('Settings saved successfully', 'success');   // green
    } catch (err) {
      toast('Failed to save settings', 'error');         // red
    }
  };
  // ...
}
```

Replace every standalone `<Snackbar>` + local state pattern in
`BookDetail.tsx` (the `alert` state at line 60) and `Library.tsx` with calls
to `useToast()`. The local alert state and its JSX can be removed entirely once
all usages migrate.

---

### Dark Mode

The current theme is a single dark-mode `createTheme` in `web/src/theme.ts`
(hardcoded `palette.mode: 'dark'`). To add a toggleable dark/light mode:

**File: `web/src/theme.ts`** — convert the static export into a factory
function that accepts a mode parameter. Keep the existing dark palette colors
as the dark branch; add a light branch.

```ts
// file: web/src/theme.ts
import { createTheme, PaletteMode } from '@mui/material/styles';

export function createAppTheme(mode: PaletteMode = 'dark') {
  return createTheme({
    palette: {
      mode,
      primary:   { main: '#1976d2' },
      secondary: { main: '#dc004e' },
      background: mode === 'dark'
        ? { default: '#0a1929', paper: '#1e2a38' }
        : { default: '#f5f5f5', paper: '#ffffff' },
    },
    typography: {
      fontFamily: [
        '-apple-system', 'BlinkMacSystemFont', '"Segoe UI"', 'Roboto',
        '"Helvetica Neue"', 'Arial', 'sans-serif',
      ].join(','),
    },
    components: {
      MuiAppBar: {
        styleOverrides: { root: { backgroundImage: 'none' } },
      },
      MuiDrawer: {
        styleOverrides: { paper: { backgroundImage: 'none' } },
      },
    },
  });
}

// Keep a default export for backwards compatibility during migration:
export const theme = createAppTheme('dark');
```

**File: `web/src/main.tsx`** — replace the static `theme` import with a
stateful theme that reads the persisted preference:

```tsx
import React, { useState, useEffect, useMemo } from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { CssBaseline, ThemeProvider, PaletteMode } from '@mui/material';
import App from './App';
import { createAppTheme } from './theme';
import { ErrorBoundary } from './components/ErrorBoundary';
import { ToastProvider } from './components/toast/ToastProvider';

const THEME_KEY = 'audiobook_organizer_theme_mode';

function ThemeRoot() {
  const [mode, setMode] = useState<PaletteMode>(() => {
    // Read persisted preference; fall back to 'dark' (current default)
    return (localStorage.getItem(THEME_KEY) as PaletteMode) || 'dark';
  });

  // Expose setMode globally so a toggle anywhere can call it.
  // A dedicated React context or Zustand slice is preferred in the long run;
  // this pattern keeps the change minimal.
  useEffect(() => {
    (window as any).__setThemeMode = (m: PaletteMode) => {
      setMode(m);
      localStorage.setItem(THEME_KEY, m);
    };
  }, []);

  const theme = useMemo(() => createAppTheme(mode), [mode]);

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <ToastProvider>
        <App />
      </ToastProvider>
    </ThemeProvider>
  );
}

const app = (
  <ErrorBoundary>
    <BrowserRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <ThemeRoot />
    </BrowserRouter>
  </ErrorBoundary>
);

ReactDOM.createRoot(document.getElementById('root')!).render(
  import.meta.env.DEV ? <React.StrictMode>{app}</React.StrictMode> : app
);
```

**File: `web/src/components/layout/TopBar.tsx`** — add a theme toggle
button next to the existing menu icon. Import MUI's `DarkMode` / `LightMode`
icons and call the global setter:

```tsx
import DarkModeIcon from '@mui/icons-material/DarkMode';
import LightModeIcon from '@mui/icons-material/LightMode';
import { useTheme } from '@mui/material/styles';

// Inside the <Toolbar>, after the menu IconButton:
const muiTheme = useTheme();
<IconButton color="inherit" onClick={() => {
  const next = muiTheme.palette.mode === 'dark' ? 'light' : 'dark';
  (window as any).__setThemeMode?.(next);
}} aria-label="toggle theme">
  {muiTheme.palette.mode === 'dark' ? <LightModeIcon /> : <DarkModeIcon />}
</IconButton>
```

Persistence is via `localStorage` key `audiobook_organizer_theme_mode`. If a
future config API endpoint is preferred, replace the read/write in `ThemeRoot`
with `api.getConfig()` / `api.updateConfig({ theme_mode })` and add
`theme_mode` to the Go `Config` struct in `internal/config/config.go`.

---

### Keyboard Shortcuts

**File: `web/src/hooks/useKeyboardShortcuts.ts`** — create this file.

```ts
// file: web/src/hooks/useKeyboardShortcuts.ts
import { useEffect, useRef } from 'react';

export interface ShortcutDefinition {
  key: string;            // single character or key name, e.g. '/' or 'Escape'
  label: string;          // human-readable, shown in help panel
  description: string;    // longer explanation for the help panel
  action: () => void;
}

/**
 * Registers global keyboard shortcuts.
 *
 * Design rules that prevent conflict with text input:
 *   1. If the event target is an <input>, <textarea>, or [contentEditable],
 *      the shortcut is suppressed — the keystroke belongs to the field.
 *   2. Shortcuts fire on `keydown` so they feel immediate.
 *   3. All shortcuts call `event.preventDefault()` + `event.stopPropagation()`
 *      so downstream handlers (e.g. the browser's `/` quick-find) don't fire.
 */
export function useKeyboardShortcuts(shortcuts: ShortcutDefinition[]) {
  // Store shortcuts in a ref so the effect closure never goes stale.
  const shortcutsRef = useRef(shortcuts);
  shortcutsRef.current = shortcuts;

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      // Rule 1: ignore when focus is inside an editable element.
      const target = e.target as HTMLElement;
      if (
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable
      ) {
        return;
      }

      const match = shortcutsRef.current.find((s) => s.key === e.key);
      if (match) {
        e.preventDefault();
        e.stopPropagation();
        match.action();
      }
    };

    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);
}
```

**File: `web/src/pages/Library.tsx`** — wire the hook at the top of the
`Library` component. The Library page already has `searchInputRef` (or a
`SearchBar` component); add a ref to focus it:

```tsx
import { useKeyboardShortcuts } from '../hooks/useKeyboardShortcuts';

// Inside Library component body, after existing state declarations:
const searchInputRef = useRef<HTMLInputElement | null>(null);

useKeyboardShortcuts([
  {
    key: '/',
    label: '/',
    description: 'Focus the search bar',
    action: () => searchInputRef.current?.focus(),
  },
  {
    key: 'o',
    label: 'O',
    description: 'Start organize operation',
    action: () => {
      // Call the same handler wired to the Organize button:
      api.startOrganize().then(() => toast('Organize started', 'info'));
    },
  },
  {
    key: 's',
    label: 'S',
    description: 'Scan all import paths',
    action: () => {
      api.startScan().then(() => toast('Scan started', 'info'));
    },
  },
]);
```

Pass `searchInputRef` down to `<SearchBar>` via a new `inputRef` prop so it
can attach to the underlying `<TextField inputRef={inputRef}>`.

**File: `web/src/components/shortcuts/ShortcutsHelp.tsx`** — create this file.

```tsx
// file: web/src/components/shortcuts/ShortcutsHelp.tsx
import React, { useState } from 'react';
import {
  Dialog, DialogTitle, DialogContent,
  Table, TableBody, TableRow, TableCell,
  Chip, Typography, IconButton, Tooltip,
} from '@mui/material';
import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import type { ShortcutDefinition } from '../../hooks/useKeyboardShortcuts';

interface ShortcutsHelpProps {
  shortcuts: ShortcutDefinition[];
}

export function ShortcutsHelp({ shortcuts }: ShortcutsHelpProps) {
  const [open, setOpen] = useState(false);

  return (
    <>
      <Tooltip title="Keyboard shortcuts">
        <IconButton onClick={() => setOpen(true)} aria-label="keyboard shortcuts help">
          <HelpOutlineIcon />
        </IconButton>
      </Tooltip>

      <Dialog open={open} onClose={() => setOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Keyboard Shortcuts</DialogTitle>
        <DialogContent>
          <Table size="small">
            <TableBody>
              {shortcuts.map((s) => (
                <TableRow key={s.key}>
                  <TableCell sx={{ width: 80 }}>
                    <Chip label={s.label} size="small" variant="outlined" />
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2">{s.description}</Typography>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DialogContent>
      </Dialog>
    </>
  );
}
```

Place `<ShortcutsHelp shortcuts={shortcuts} />` in the Library page toolbar
(next to the existing filter/upload buttons). Pass the same `shortcuts` array
used by `useKeyboardShortcuts`.

---

### Welcome Wizard (Enrichment of Existing Component)

The wizard already exists at `web/src/components/wizard/WelcomeWizard.tsx`
(version 1.0.3) and is wired into `App.tsx` lines 30–68. It uses MUI
`<Stepper>`, `<Step>`, `<StepLabel>` with three steps:

1. **Library Path** — uses `<ServerFileBrowser>` to pick a directory; saves
   via `api.updateConfig({ root_dir })`.
2. **AI Setup (Optional)** — a `<TextField>` for the OpenAI API key with a
   "Test Connection" button that calls `api.testAIConnection(key)`. The result
   is stored in local state (`keyTestResult`).
3. **Import Folders** — adds one or more paths via `<ServerFileBrowser>` and
   calls `api.addImportPath(path, name)` for each.

**First-run detection logic** (in `App.tsx`):

```tsx
// Lines 37–48 of App.tsx:
const config = await api.getConfig();
const setupComplete =
  Boolean(config.root_dir && config.root_dir.trim()) ||
  Boolean(config.setup_complete);
setShowWizard(!setupComplete);
if (setupComplete) {
  localStorage.setItem('welcome_wizard_completed', 'true');
}
```

The wizard is considered complete when either `root_dir` is non-empty or
`setup_complete` is `true` in the server config. `localStorage` key
`welcome_wizard_completed` is a client-side fallback used when the API is
unreachable (line 50–54).

**Completion flag storage** — at the end of the wizard's final step, it calls:

```tsx
await api.updateConfig({ root_dir: libraryPath, setup_complete: true });
```

This sets `config.AppConfig.SetupComplete = true` on the server (see
`internal/server/server.go` line 3109–3112) and persists via
`config.SaveConfigToDatabase()`. The `App.tsx` parent then calls
`handleWizardComplete()` which sets `showWizard = false`.

**To extend the wizard** (e.g. add a new step), add an entry to the `steps`
array (line 59 of WelcomeWizard.tsx), increment `activeStep`, and render a
new `<Box>` guarded by `activeStep === N`. The "Next" / "Back" buttons
already increment/decrement `activeStep`. The "Finish" button on the last
step triggers the config save shown above.

---

### Progressive Loading Skeletons

**File: `web/src/components/audiobooks/AudiobookGridSkeleton.tsx`** — create
this file. It renders the same grid column count as `AudiobookGrid` but with
MUI `<Skeleton>` placeholders.

```tsx
// file: web/src/components/audiobooks/AudiobookGridSkeleton.tsx
import React from 'react';
import { Grid, Box, Skeleton } from '@mui/material';

/**
 * Skeleton placeholder that matches the AudiobookGrid layout exactly.
 * `count` defaults to 12 (one full viewport of cards at typical widths).
 */
export function AudiobookGridSkeleton({ count = 12 }: { count?: number }) {
  return (
    <Grid container spacing={2}>
      {Array.from({ length: count }).map((_, i) => (
        <Grid item xs={12} sm={6} md={4} lg={3} key={i}>
          <Box sx={{ p: 1 }}>
            {/* Card height placeholder — matches AudiobookCard */}
            <Skeleton variant="rectangular" width="100%" height={180} sx={{ borderRadius: 1 }} />
            <Box sx={{ pt: 1 }}>
              <Skeleton variant="text" sx={{ fontSize: '1.2rem', width: '80%' }} />
              <Skeleton variant="text" sx={{ width: '60%' }} />
              <Skeleton variant="text" sx={{ width: '45%' }} />
            </Box>
          </Box>
        </Grid>
      ))}
    </Grid>
  );
}
```

**File: `web/src/components/audiobooks/AudiobookGrid.tsx`** — replace the
current `CircularProgress` loading state (lines 35–46) with the skeleton:

```tsx
import { AudiobookGridSkeleton } from './AudiobookGridSkeleton';

// Replace the existing loading block:
if (loading) {
  return <AudiobookGridSkeleton />;
}
```

**File: `web/src/pages/Library.tsx`** — pass the `loading` prop to
`<AudiobookGrid>`. The component already receives `loading` from local state;
make sure it is set to `true` before the fetch and `false` after resolution.
No structural change needed here — the skeleton renders automatically when
`loading={true}`.

---

### Virtualized Audiobook List

For collections exceeding ~1,000 books, the flat DOM list becomes expensive.
Use `react-window` (lightweight, no peer-dep issues).

**Step 1:** `npm install react-window` and `npm install -D @types/react-window`.

**File: `web/src/components/audiobooks/VirtualizedAudiobookList.tsx`** — create
this file.

```tsx
// file: web/src/components/audiobooks/VirtualizedAudiobookList.tsx
import React, { useRef, useEffect } from 'react';
import { FixedSizeList } from 'react-window';
import { Box } from '@mui/material';
import { AudiobookCard } from './AudiobookCard';
import type { Audiobook } from '../../types';

const ROW_HEIGHT = 220; // Must match the rendered height of one AudiobookCard row

interface VirtualizedAudiobookListProps {
  audiobooks: Audiobook[];
  onEdit?: (audiobook: Audiobook) => void;
  onDelete?: (audiobook: Audiobook) => void;
  onClick?: (audiobook: Audiobook) => void;
  selectedIds?: Set<string>;
  onToggleSelect?: (audiobook: Audiobook) => void;
  // Container height in px; caller can derive from window.innerHeight - toolbar.
  containerHeight: number;
  containerWidth: number;
}

export function VirtualizedAudiobookList({
  audiobooks,
  onEdit,
  onDelete,
  onClick,
  selectedIds,
  onToggleSelect,
  containerHeight,
  containerWidth,
}: VirtualizedAudiobookListProps) {
  const listRef = useRef<FixedSizeList>(null);

  // Scroll to top when the list data changes (e.g. after a search/filter).
  useEffect(() => {
    listRef.current?.scrollTo(0);
  }, [audiobooks]);

  // Row renderer — called by react-window for each visible row only.
  const Row = React.memo(({ index, style }: { index: number; style: React.CSSProperties }) => {
    const book = audiobooks[index];
    return (
      <Box style={style} sx={{ px: 1, py: 0.5 }}>
        <AudiobookCard
          audiobook={book}
          onEdit={() => onEdit?.(book)}
          onDelete={() => onDelete?.(book)}
          onClick={() => onClick?.(book)}
          selected={selectedIds?.has(book.id)}
          onToggleSelect={() => onToggleSelect?.(book)}
        />
      </Box>
    );
  });

  return (
    <FixedSizeList
      ref={listRef}
      height={containerHeight}
      width={containerWidth}
      itemCount={audiobooks.length}
      itemSize={ROW_HEIGHT}
    >
      {Row}
    </FixedSizeList>
  );
}
```

**File: `web/src/pages/Library.tsx`** — add a view-mode toggle (or auto-select
based on count). When the total book count exceeds 1,000 (or user picks list
mode), render `<VirtualizedAudiobookList>` instead of `<AudiobookGrid>`:

```tsx
import { VirtualizedAudiobookList } from '../components/audiobooks/VirtualizedAudiobookList';

// Inside render, replace the current grid/list branch:
{audiobooks.length > 1000 || viewMode === 'list' ? (
  <VirtualizedAudiobookList
    audiobooks={audiobooks}
    containerHeight={window.innerHeight - 200} // subtract toolbar + padding
    containerWidth={containerRef.current?.clientWidth ?? 800}
    onEdit={...}
    onDelete={...}
    onClick={...}
    selectedIds={selectedIds}
    onToggleSelect={...}
  />
) : (
  <AudiobookGrid audiobooks={audiobooks} loading={loading} ... />
)}
```

---

### Advanced Filters

Filter audiobooks by: bitrate range, codec, quality tier, duration bucket.
Complements existing search and sort.

---

## First-Run Experience

(See Welcome Wizard section above for full implementation detail.)

---

## Backlog — Components & Views

### Library Enhancements

- Inline author/series quick-create dialog from the edit form
- Book detail modal with expanded metadata and version timeline

### New Visualization Components

- Timeline visualization for operation history
- Quality comparison chart between versions of the same book
- Folder tree viewer for import paths with status badges
- Standalone log tail component (filter by level, live search)

---

## Accessibility (a11y)

### Skip-to-Content Link

**File: `web/src/components/layout/SkipLink.tsx`** — create this file.

```tsx
// file: web/src/components/layout/SkipLink.tsx
import React from 'react';
import { Box } from '@mui/material';

/**
 * A visually-hidden link that becomes visible on focus.
 * Screen readers and keyboard users hit Tab first; this skips the
 * sidebar navigation and jumps straight to main content.
 *
 * The target must be an element with id="main-content" — add that id
 * to the <Box component="main"> in MainLayout.tsx.
 */
export function SkipLink() {
  return (
    <Box
      component="a"
      href="#main-content"
      tabIndex={0}
      sx={{
        position: 'fixed',
        top: -100,
        left: 16,
        zIndex: 9999,
        px: 2, py: 1,
        bgcolor: 'primary.main',
        color: '#fff',
        borderRadius: 1,
        fontWeight: 600,
        textDecoration: 'none',
        transition: 'top 0.2s',
        '&:focus': { top: 8 },
      }}
    >
      Skip to content
    </Box>
  );
}
```

**File: `web/src/components/layout/MainLayout.tsx`** — add `<SkipLink />` as
the first child and add `id="main-content"` to the `<Box component="main">`:

```tsx
import { SkipLink } from './SkipLink';

// Inside MainLayout return:
<Box sx={{ display: 'flex', width: '100%' }}>
  <SkipLink />                       {/* ← insert here */}
  <TopBar ... />
  <Sidebar ... />
  <Box
    component="main"
    id="main-content"                {/* ← add this id */}
    tabIndex={-1}                    {/* allows programmatic focus */}
    sx={{ /* existing styles */ }}
  >
    {children}
  </Box>
</Box>
```

### ARIA Attributes for Existing Components

Apply the following targeted changes to improve screen-reader announcements
without restructuring components:

| File | Element | Add |
|---|---|---|
| `AudiobookGrid.tsx` | Root `<Box>` when empty | `role="status"` `aria-live="polite"` `aria-label="No audiobooks found"` |
| `AudiobookCard.tsx` | Card `<Paper>` | `role="article"` `aria-label={`Audiobook: ${audiobook.title}`}` |
| `SearchBar.tsx` | `<TextField>` | `aria-label="Search audiobooks"` (already likely present via `label` prop; verify) |
| `FilterSidebar.tsx` | Outer `<Box>` | `role="navigation"` `aria-label="Filter audiobooks"` |
| `TopBar.tsx` | Menu `<IconButton>` | Already has `aria-label="open drawer"` — no change needed |
| `Settings.tsx` | Each `<Tab>` | `aria-label` matching the tab name (e.g. `aria-label="General settings"`) |

### High Contrast Theme Option

Add a third palette mode option (`'highcontrast'`) in `createAppTheme` that
uses pure white backgrounds, black text, and saturated accent colors. Wire it
as a third option in the theme toggle (cycle: dark -> light -> high-contrast ->
dark).

---

## Internationalization (i18n)

- Extract UI strings into translation files
- Language switcher in Settings
- Date/time and number format localization

## Mobile & PWA

- PWA manifest and offline shell
- Add to Home Screen guidance
- Basic offline read-only browsing of cached metadata
- Mobile responsive layout improvements (grid collapse, drawer navigation)

---

## Dependencies

- Dark mode: `createAppTheme` factory must replace the static `theme` export
  before the toggle can work. The `ThemeRoot` wrapper in `main.tsx` is the
  single source of truth for the active mode.
- Virtualized list depends on stable sort/filter API responses and requires
  `react-window` in `package.json`.
- Keyboard shortcuts hook must be instantiated per-page (not globally) so that
  page-specific actions (e.g. the search ref) are available in closure.
- i18n requires all UI strings to be extracted first.

## References

- Frontend source: `web/src/`
- Zustand store (notifications): `web/src/stores/useAppStore.ts`
- MUI theme factory: `web/src/theme.ts`
- App entry point: `web/src/main.tsx`
- Layout components: `web/src/components/layout/`
- Wizard: `web/src/components/wizard/WelcomeWizard.tsx`
- Current E2E tests: `web/tests/e2e/`
