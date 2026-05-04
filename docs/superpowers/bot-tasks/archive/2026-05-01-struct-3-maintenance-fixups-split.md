<!-- file: docs/superpowers/bot-tasks/2026-05-01-struct-3-maintenance-fixups-split.md -->
<!-- version: 1.0.0 -->
<!-- guid: e5f6a7b8-c9d0-1234-efab-567890123456 -->
<!-- last-edited: 2026-05-01 -->

# STRUCT-3 — Split `maintenance_fixups.go` (6400 lines → 8 files)

**Priority:** High  
**Effort:** Large (mechanical move — no logic changes)  
**Branch:** `refactor/struct-3-maintenance-fixups-split`

---

## Why This Matters

`internal/server/maintenance_fixups.go` is **6400+ lines** in a single file. It is
impossible to navigate, review, or understand. Splitting it into logical sub-files
makes each maintenance job independently reviewable and reduces merge conflicts.

**Evidence:**
```bash
wc -l internal/server/maintenance_fixups.go
# 6400+
```

---

## What This Task Does

Split `maintenance_fixups.go` into 8 files by logical domain. **No logic changes
whatsoever** — only move functions. The package name stays `package server`.

---

## What NOT to Do

- **Do NOT** change any function signatures or logic.
- **Do NOT** rename any functions.
- **Do NOT** extract interfaces or change imports beyond what is forced by the move.
- **Do NOT** touch any other files in `internal/server/`.
- **Do NOT** touch test files.

---

## Target File Layout

### File 1: `internal/server/maintenance_readby.go`
Lines ~78–314 in original. Functions to move:
- `handleFixReadByNarrator`
- `parsePattern1`
- `parsePattern2`
- `parsePattern3`
- `titleFromFilePath`
- `applyReadByFix`
- `caseInsensitiveIndex`
- `stringDeref`
- `countApplied`
- `countErrors`

### File 2: `internal/server/maintenance_series.go`
Lines ~357–614 in original. Functions to move:
- `handleCleanupSeries`
- `unlinkAndDeleteSeries`
- `mergeSeriesGroup`
- `normalizeSeriesName`

### File 3: `internal/server/maintenance_files.go`
Lines ~662–1000, ~1219–1286, ~1713–1927 in original. Functions to move:
- `handleBackfillBookFiles`
- `handleCleanupEmptyFolders`
- `isDirEmpty`
- `isGarbageDirectory`
- `allAlpha`
- `handleCleanupOrganizeMess`
- `createBookFilesForBook`
- `handleEnrichBookFiles`
- `parseTrackNumberFromFilename`
- `handleFixBookFilePaths`
- `isAuthorDirectory`
- `bestMatchSubdir`
- `fixAuthorDirPath`

### File 4: `internal/server/maintenance_author_version.go`
Lines ~1121–1644 in original. Functions to move:
- `handleFixAuthorNarratorSwap`
- `handleFixVersionGroups`
- `extractCoreTitle`
- `findMajorityCore`
- `coreTitlesMatch`
- `longWords`
- `unlinkVersionGroupOutliers`

### File 5: `internal/server/maintenance_dedup.go`
Lines ~2217–2829 in original. Functions to move:
- `handleDedupBooks`
- `fetchAllBooksPaginated`
- `isJunkReadByNarrator`
- `pickKeeperIdx`
- `bookScore`
- `mergeDuplicateBook`
- `mergeBookFields`
- `softDeleteBook`
- `normalizeDedupTitle`
- `filterLive`
- `handleRefetchMissingAuthors`

### File 6: `internal/server/maintenance_wipe.go`
Lines ~3037–3373 in original. Functions to move:
- `handleWipe`
- `dryRunLabel`
- `wipeBookFiles`
- `wipeSegments`
- `wipeBooks`
- `wipeAuthors`
- `wipeSeries`
- `wipeExternalIDs`
- `wipeActivity`

### File 7: `internal/server/maintenance_itunes.go`
Lines ~3404–5168 in original (library states, iTunes paths, composer, relink, repair,
revert, duration, relink report, deluge import). Functions to move:
- `handleFixLibraryStates`
- `handleRecomputeITunesPaths`
- `handleGenerateITLTests`
- `handleCleanupBackups`
- `categorizeComposer`
- `handleScanComposerTags`
- `runComposerTagScan`
- `handleGetComposerScanResults`
- `humanizeBytes`
- `handleRelinkMissingToiTunes`
- `handleRepairMissingFiles`
- `runMissingFileRepair`
- `repairOneMissingFile`
- `handleGetMissingFileRepairResults`
- `handleRevertMetadataFetch`
- `handleScanDurationMismatch`
- `handleRelinkReport`
- `handleBulkDelugeImport`
- `runBulkDelugeImport`

### File 8: `internal/server/maintenance_hashes.go`
Lines ~5927–6388+ in original. Functions to move:
- `handleBackfillMetadataSourceHash`
- `bookMetadataSourceAndID`
- `handleScanChapterGroups`
- `handleMergeChapterGroups`
- `handleScanDuplicateFiles`
- `handleScanMetadataHashDuplicates`
- `handleGetBookFileHashStats`
- `handleBackfillFileHashes`
- `handleGetBookMetadataHashStats`

---

## Steps

### Step 1 — Baseline check

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go build ./internal/server/...
go test ./internal/server/... -run TestMaintenance -timeout 120s 2>&1 | grep -E 'PASS|FAIL|ok'
wc -l internal/server/maintenance_fixups.go
```

Record the line count and whether tests pass.

### Step 2 — Create the 8 new files

For each file listed above:
1. Create the new `.go` file with the standard version header.
2. Add `package server` at the top.
3. Copy (not cut yet) the functions into the new file.
4. Add only the imports that the moved functions actually use.
   - Run `go build ./internal/server/...` to check for missing imports.
   - Fix import errors.

The header template for each new file:
```go
// file: internal/server/maintenance_XXX.go
// version: 1.0.0
// guid: <generate-a-new-uuid>
// last-edited: 2026-05-01

package server
```

### Step 3 — Build after each file

After creating each new file (while originals still exist), run:
```bash
go build ./internal/server/...
```

Fix any "already declared" errors — they mean a helper was accidentally duplicated.

### Step 4 — Remove functions from maintenance_fixups.go

Once ALL 8 new files build cleanly together with the original, delete the moved
function bodies from `internal/server/maintenance_fixups.go`.

Keep at the top of `maintenance_fixups.go` only:
- The package declaration and any imports still needed for remaining functions.
- Any `const` or `var` blocks that are shared across the new files.

### Step 5 — Final build + test

```bash
go build ./internal/server/...
go test ./internal/server/... -timeout 120s 2>&1 | grep -E 'FAIL|ok|---'
```

Both must pass. `maintenance_fixups.go` should be **empty or nearly empty** after
the move (or deleted if truly empty — but check that the route registrations are
not in it first).

### Step 6 — Bump version headers

- All 8 new files: `version: 1.0.0` (already set in header).
- Any file that was changed: bump patch version.

### Step 7 — Commit and open PR

```bash
git checkout -b refactor/struct-3-maintenance-fixups-split
git add internal/server/maintenance_*.go
git commit -m "refactor(server): split maintenance_fixups.go into 8 domain files

Splits the 6400-line maintenance_fixups.go into:
- maintenance_readby.go (read-by/narrator fix)
- maintenance_series.go (series cleanup/merge)
- maintenance_files.go (book file backfill/cleanup)
- maintenance_author_version.go (author swap, version groups)
- maintenance_dedup.go (dedup, merge, refetch)
- maintenance_wipe.go (wipe operations)
- maintenance_itunes.go (iTunes relink, repair, revert)
- maintenance_hashes.go (hash scans, chapter groups)

No logic changes. Structure audit STRUCT-3.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/struct-3-maintenance-fixups-split
gh pr create \
  --title "refactor(server): split maintenance_fixups.go into 8 domain files" \
  --body "Splits 6400-line file into 8 focused files. No logic changes. Structure audit STRUCT-3."
```

---

## Checklist

- [ ] 8 new files created, each with version header and `package server`
- [ ] `go build ./internal/server/...` clean
- [ ] `go test ./internal/server/...` passes
- [ ] Original `maintenance_fixups.go` is empty or deleted
- [ ] No function renamed or logic changed
- [ ] PR opened on branch `refactor/struct-3-maintenance-fixups-split`
