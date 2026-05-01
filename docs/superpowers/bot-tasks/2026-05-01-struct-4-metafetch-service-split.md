<!-- file: docs/superpowers/bot-tasks/2026-05-01-struct-4-metafetch-service-split.md -->
<!-- version: 1.0.0 -->
<!-- guid: f6a7b8c9-d0e1-2345-fabc-678901234567 -->
<!-- last-edited: 2026-05-01 -->

# STRUCT-4 — Split `metafetch/service.go` (3932 lines → 8 files)

**Priority:** High  
**Effort:** Large (mechanical move — no logic changes)  
**Branch:** `refactor/struct-4-metafetch-service-split`

---

## Why This Matters

`internal/metafetch/service.go` is **3932 lines** in a single file. Functions for
wiring, fetching, search, scoring, normalisation, writeback, and file I/O are all
mixed together. Splitting them makes the code navigable and reviewable.

**Evidence:**
```bash
wc -l internal/metafetch/service.go
# 3932+
```

---

## What This Task Does

Split `service.go` into 8 files by logical domain. **No logic changes** — only move
functions. The package name stays `package metafetch`.

---

## What NOT to Do

- **Do NOT** change any function signatures or logic.
- **Do NOT** rename any functions.
- **Do NOT** change exported function names (they may be called from other packages).
- **Do NOT** touch test files.

---

## Target File Layout

### File 1: `internal/metafetch/service_wiring.go`
Constructor and dependency injection. Functions to move:
- `NewService`
- `SetOverrideSources`
- `SetActivityService`
- `SetWriteBackBatcher`
- `SetSafeWriteDeps`
- `SetOLStore`
- `SetDedupEngine`
- `SetMetadataScorer`
- `SetMetadataLLMScorer`
- `SetISBNEnrichment`
- `ISBNEnrichment`

### File 2: `internal/metafetch/service_fetch.go`
Fetch entry points. Functions to move:
- `queueISBNEnrichment`
- `FetchMetadataForBook`
- `FetchMetadataForBookByTitle`

### File 3: `internal/metafetch/service_search.go`
Search context and source chain. Functions to move:
- `buildSearchContext`
- `BuildSourceChain`
- `SearchMetadataForBook`
- `SearchMetadataForBookWithOptions`

### File 4: `internal/metafetch/service_apply.go`
Apply / persist / candidate application. Functions to move:
- `ApplyMetadataToBook`
- `RecordChangeHistory`
- `syncMetadataToLibraryCopy`
- `ensureLibraryCopy`
- `persistFetchedMetadata`
- `ApplyMetadataCandidate`
- `checkMetadataSourceHashDuplicates`
- `ApplyMetadataSystemTags`
- `MarkNoMatch`

### File 5: `internal/metafetch/service_scoring.go`
Scoring and ranking. Functions to move:
- `IsGarbageValue`
- `IsBetterValue`
- `IsBetterStringPtr`
- `computeF1Base`
- `ApplyNonBaseAdjustments`
- `durationScoreMultiplier`
- `computeDurationScore`
- `pickBestMatchFromScored`
- `ScoreOneResult`
- `ScoreBaseCandidates`
- `bestTitleMatchForBook`
- `RerankTopK`
- `ApplySeriesPositionFilter`
- `BestTitleMatch`
- `BestTitleMatchWithContext`

### File 6: `internal/metafetch/service_normalize.go`
Normalisation and parsing. Functions to move:
- `derefString`
- `derefIntAsString`
- `jsonEncodeString`
- `NormalizeMetaSeries`
- `ParseSeriesFromTitle`
- `SignificantWords`
- `isCompilation`
- `extractTrailingNumber`
- `normalizeSeriesNumber`

### File 7: `internal/metafetch/service_writeback.go`
Write-back, tag maps, segment titles, apply pipeline. Functions to move:
- `writeBackMetadata`
- `MetadataSourceTag`
- `MetadataLanguageTag`
- `BuildTagMap`
- `BuildFullTagMap`
- `FilterUnchangedTags`
- `generateSegmentTitles`
- `runApplyPipeline`
- `WriteBackMetadataForBook`

### File 8: `internal/metafetch/service_files.go`
File I/O, iTunes path computation. Functions to move:
- `AudioFilesInDir`
- `backupFileBeforeWrite`
- `ApplyMetadataFileIO`
- `ComputeITunesPath`
- `removeEmptyDirs`

---

## Steps

### Step 1 — Baseline check

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go build ./internal/metafetch/...
go test ./internal/metafetch/... -timeout 120s 2>&1 | grep -E 'FAIL|ok'
wc -l internal/metafetch/service.go
```

### Step 2 — Keep `service.go` for the `Service` struct and field declarations

Open `service.go` and identify the `type Service struct { ... }` declaration and
any `const`/`var` at file scope. These stay in `service.go` — do NOT move them.

```bash
grep -n 'type Service struct\|^const\|^var\|^type ' internal/metafetch/service.go | head -30
```

### Step 3 — Create the 8 new files

For each file above:
1. Create the file with the version header and `package metafetch`.
2. Copy (not cut) the functions.
3. Add only the imports needed by the copied functions.
4. Run `go build ./internal/metafetch/...` — fix any import errors.

Header template:
```go
// file: internal/metafetch/service_XXX.go
// version: 1.0.0
// guid: <generate-a-new-uuid>
// last-edited: 2026-05-01

package metafetch
```

### Step 4 — Remove functions from service.go

After all 8 files build cleanly alongside the original, delete the moved functions
from `service.go`. Only the struct definition, top-level consts/vars, and any
functions not listed above should remain.

### Step 5 — Final build + test

```bash
go build ./internal/metafetch/...
go build ./...
go test ./internal/metafetch/... -timeout 120s 2>&1 | grep -E 'FAIL|ok|---'
```

All must pass. Also run `go build ./...` to catch cross-package references.

### Step 6 — Bump version headers on all changed files

### Step 7 — Commit and open PR

```bash
git checkout -b refactor/struct-4-metafetch-service-split
git add internal/metafetch/
git commit -m "refactor(metafetch): split service.go into 8 domain files

Splits the 3932-line service.go into:
- service_wiring.go (constructor, DI)
- service_fetch.go (fetch entry points)
- service_search.go (search context, source chain)
- service_apply.go (apply, persist, candidate)
- service_scoring.go (scoring, ranking)
- service_normalize.go (normalisation, parsing)
- service_writeback.go (writeback, tags, pipeline)
- service_files.go (file I/O, iTunes path)

No logic changes. Structure audit STRUCT-4.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/struct-4-metafetch-service-split
gh pr create \
  --title "refactor(metafetch): split service.go into 8 domain files" \
  --body "Splits 3932-line file into 8 focused files. No logic changes. Structure audit STRUCT-4."
```

---

## Checklist

- [ ] 8 new files created, each with header and `package metafetch`
- [ ] `Service` struct and file-scope declarations remain in `service.go`
- [ ] `go build ./internal/metafetch/...` clean
- [ ] `go build ./...` clean (cross-package check)
- [ ] `go test ./internal/metafetch/...` passes
- [ ] No function renamed or logic changed
- [ ] PR opened on branch `refactor/struct-4-metafetch-service-split`
