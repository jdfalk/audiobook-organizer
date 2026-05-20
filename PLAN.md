# Memory Leak Fixes - Parallel Execution Plan

**Goal:** Fix all 41 detected memory leaks across 28 files in the React frontend.

**Status:** Parallel subagent execution (Haiku, 4-5 subagents, no git/CHANGELOG per file).

## Leak Categories
- **Untracked setTimeout/setInterval** (~30 issues): Timer doesn't clear on component unmount
- **addEventListener without cleanup** (~7 issues): Event listeners not removed in useEffect cleanup
- **Recursive polling without cancellation** (~2 issues): Polling continues even after component unmount

## Files to Fix (28 total)
1. src/App.tsx (1 issue)
2. src/components/AIJobsPanel.tsx (1 issue)
3. src/components/CacheStatsPanel.tsx (1 issue)
4. src/components/OperationActivityPanel.tsx (1 issue)
5. src/components/TagComparison.tsx (2 issues - addEventListener)
6. src/components/audiobooks/VersionManagement.tsx (1 issue)
7. src/components/common/ConfigurableTable.tsx (2 issues - addEventListener)
8. src/components/settings/ITunesImport.tsx (2 issues)
9. src/components/system/MaintenanceTab.tsx (1 issue)
10. src/contexts/AuthContext.tsx (1 issue)
11. src/hooks/useKeyboardShortcuts.ts (1 issue)
12. src/hooks/usePendingFileOps.ts (1 issue)
13. src/hooks/useUnsavedChangesBlocker.ts (1 issue)
14. src/pages/ActivityLog.tsx (3 issues)
15. src/pages/BookDedup.tsx (1 issue)
16. src/pages/BookDetail.tsx (2 issues)
17. src/pages/Dashboard.tsx (1 issue)
18. src/pages/Library.tsx (5 issues - highest priority)
19. src/pages/Settings.tsx (1 issue)
20. src/pages/Users.tsx (1 issue)
21. src/services/api.ts (2 issues)
22. src/services/eventSourceManager.ts (2 issues)
23. src/stores/useAppStore.ts (1 issue)
24. src/stores/useOperationsStore.ts (2 issues)
25. src/utils/operationPolling.ts (3 issues)

## Subagent Work Batches
- **Batch 1 (Haiku):** src/App.tsx, src/components/{AIJobsPanel, CacheStatsPanel, OperationActivityPanel, TagComparison, VersionManagement}
- **Batch 2 (Haiku):** src/components/{ConfigurableTable, ITunesImport, MaintenanceTab}, src/contexts/AuthContext, src/hooks/useKeyboardShortcuts
- **Batch 3 (Haiku):** src/hooks/{usePendingFileOps, useUnsavedChangesBlocker}, src/pages/{ActivityLog, BookDedup, BookDetail, Dashboard}
- **Batch 4 (Haiku):** src/pages/{Library, Settings, Users}, src/services/{api, eventSourceManager}
- **Batch 5 (Haiku):** src/stores/{useAppStore, useOperationsStore}, src/utils/operationPolling

## Fix Pattern
1. Add `useRef` to React imports if missing
2. For setTimeout/setInterval: Add timerRef + isUnmountedRef, wrap calls, add cleanup
3. For addEventListener: Add removeEventListener in useEffect cleanup
4. Update file version header (bump patch)
5. **NO git commits, NO CHANGELOG per file**

## Consolidation (Coordinator)
- Collect all diffs from subagents
- Create single consolidated PR with all changes
- One commit per file: `fix(memory): clear [timer/listener] on unmount`
- Update CHANGELOG.md once with summary
- Run `make test-all` before merge

## Test Strategy
- `make test-all` after consolidation
- Memory leak scanner verification
- Spot-check 3-4 high-impact files

## Rollback
```bash
git reset --hard origin/main
```
