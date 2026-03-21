# Code Review Report — 2026-03-21

## 1. Executive Summary

Four parallel analysis agents reviewed the audiobook-organizer codebase on 2026-03-21:

| Agent | Scope | Issues Found |
|-------|-------|-------------|
| Go Backend | `metadata_fetch_service.go`, `metadata.go`, `taglib_support.go`, `enhanced.go`, `embed_cover.go`, `cover.go` | 6 (2 critical, 4 important) |
| React Frontend | `TagComparison.tsx`, `ChangeLog.tsx`, `BookDetail.tsx`, `api.ts` | 9 (5 critical, 4 important) |
| Tag Round-Trip | Write/read pipeline across taglib, CLI fallbacks, and provenance | 5 gaps identified |
| Test Runner | `go test ./...` and `npx tsc --noEmit` | All 33 Go packages pass; TypeScript compiles clean |

**Total: 20 issues across all agents.** The most severe are data-loss bugs in the metadata pipeline (partial `UpdateBook` wipe, series-index overwrite) and dead UI code paths (conflict-overwrite no-op, `loadChangelog` reference error).

---

## 2. Go Backend Issues

Reviewed files: `internal/server/metadata_fetch_service.go`, `internal/metadata/metadata.go`, `internal/metadata/taglib_support.go`, `internal/metadata/enhanced.go`, `internal/tagger/embed_cover.go`, `internal/metadata/cover.go`.

### Critical

#### 2.1 Partial `UpdateBook` call destroys all book fields (Confidence: 100)

**File:** `metadata_fetch_service.go`, line 321

After downloading cover art in `FetchMetadataForBook`, the code calls:

```go
mfs.db.UpdateBook(id, &database.Book{CoverURL: &localCoverURL})
```

Since `UpdateBook` does full column replacement, this wipes every other field (title, author, series, ISBN, etc.) on the book record immediately after they were successfully fetched and applied.

**Fix:** Remove the destructive second call; the in-memory `updatedBook` was already mutated on the line above.

#### 2.2 `SeriesIndex` read from tags is unconditionally overwritten by title heuristic (Confidence: 95)

**File:** `metadata.go`, line 350

`SERIES_INDEX`/`MVIN` tags are read at lines 290-296, then unconditionally replaced by `DetectVolumeNumber(metadata.Title)` at line 350. This breaks the tag round-trip: a file tagged with `SERIES_INDEX=3` will have that value overwritten by whatever number appears in the title string.

**Fix:** Only call `DetectVolumeNumber` when `metadata.SeriesIndex == 0`.

### Important

#### 2.3 `writeBackMetadata` omits narrator, language, description, series, series_index (Confidence: 90)

**File:** `metadata_fetch_service.go`, lines 703-795

The automatic write-back path (`config.WriteBackMetadata = true`) uses `writeBackMetadata` which builds a tag map missing half the fields that the manual `WriteBackMetadataForBook` endpoint writes via `buildFullTagMap`. The two code paths are inconsistent.

**Fix:** Refactor `writeBackMetadata` to delegate to `buildFullTagMap`.

#### 2.4 Version-sibling skip incorrectly increments `writtenCount` (Confidence: 90)

**File:** `metadata_fetch_service.go`, lines 1993-1995

When `filterUnchangedTags` returns an empty map (nothing to write), the code still increments `writtenCount`, causing spurious `SetLastWrittenAt` stamps and unnecessary rescans.

**Fix:** Remove `writtenCount++` from the empty-map branch (line 1994).

#### 2.5 `EmbedCoverArt` may strip custom tags from M4B files (Confidence: 85)

**File:** `embed_cover.go`, lines 92-101

The ffmpeg command uses `-map 0:a` (audio only) with the `ipod` muxer, which may silently drop non-standard metadata atoms written by taglib. Since `embedCoverInBookFiles` runs immediately after tag writing (lines 2023-2026), custom tags can be permanently erased.

**Fix:** Call cover embedding before writing metadata tags, or verify `-map_metadata 0` preserves custom atoms.

#### 2.6 `filterUnchangedTags` does not compare custom fields (Confidence: 85)

**File:** `metadata_fetch_service.go`, lines 2161-2182

Fields like `book_id`, `description`, `asin`, `open_library_id`, `hardcover_id`, `google_books_id`, `edition`, `print_year` are not in `currentVals`, so they are always rewritten regardless of whether they changed.

---

## 3. React Frontend Issues

Reviewed files: `web/src/components/TagComparison.tsx`, `web/src/components/ChangeLog.tsx`, `web/src/pages/BookDetail.tsx`, `web/src/services/api.ts`.

### Critical

#### 3.1 `loadChangelog()` called before declaration -- ReferenceError (Confidence: 95)

**File:** `ChangeLog.tsx`, line 63

`handleRevert` (line 49) calls `loadChangelog()` which is declared at line 74. `const` bindings are not hoisted, so this throws `ReferenceError` at runtime when a user clicks Revert.

#### 3.2 HTTP errors from revert silently ignored (Confidence: 90)

**File:** `ChangeLog.tsx`, lines 53-60

The `fetch()` calls to `revert-metadata` and `write-back` have no `response.ok` checks. HTTP 4xx/5xx errors are silently treated as success.

#### 3.3 Double `loadTags()` on snapshot change (Confidence: 88)

**File:** `TagComparison.tsx`, lines 85-95

When `snapshotTimestamp` changes, two `useEffect` hooks fire simultaneously, issuing duplicate `getBookTags` requests. Out-of-order responses may overwrite each other.

#### 3.4 Genre field renders `book.quality` (wrong field) (Confidence: 92)

**File:** `BookDetail.tsx`, line 1291 / `api.ts` Book interface

```ts
{ label: 'Genre', value: book.quality }
```

The `Book` interface in `api.ts` does not include a `genre` field at all, despite migration 36 adding the column. The UI displays audio quality under the "Genre" label.

#### 3.5 `handleConflictOverwrite` always sends `null` (Confidence: 90)

**File:** `BookDetail.tsx`, lines 776-779, 803-816

`pendingUpdate` is never set to actual form data before the conflict dialog opens. The guard `if (!book || !pendingUpdate) return` causes the Overwrite button to silently do nothing.

### Important

#### 3.6 Resize listeners leaked on unmount during active drag (Confidence: 83)

**File:** `TagComparison.tsx`, lines 124-157

#### 3.7 Version-expand reset on every version-count change (Confidence: 82)

**File:** `BookDetail.tsx`, lines 540-544

#### 3.8 Tags preload effect re-runs on every `versionFileTags` update (Confidence: 80)

**File:** `BookDetail.tsx`, lines 590-618

#### 3.9 "Compare snapshot" click fires `onCompareSnapshot` twice (Confidence: 85)

**File:** `ChangeLog.tsx`, lines 154-165

---

## 4. Tag Round-Trip Audit

### Comparison Table

| Field (tagMap key) | Written by taglib? | Written by M4B CLI? | Written by MP3 CLI? | Written by FLAC CLI? | Read from file? | Shown in provenance? | Gap? |
|---|---|---|---|---|---|---|---|
| **title** | TITLE | yes | yes | TITLE | Yes | Yes | -- |
| **artist** | ALBUMARTIST, ARTIST, COMPOSER | yes | yes | ARTIST | Yes | Yes | -- |
| **album** | ALBUM | yes | yes | ALBUM | Yes | Yes | -- |
| **genre** | GENRE | yes | yes | GENRE | Yes | Yes | -- |
| **year** | DATE | yes | yes | DATE | Yes | Yes | -- |
| **narrator** | NARRATOR | comment only | TXXX | NARRATOR | Yes | Yes | **M4B CLI lossy** |
| **language** | LANGUAGE | **No** | **No** | **No** | Yes | Yes | **CLI fallbacks never write** |
| **publisher** | PUBLISHER | **No** | **No** | **No** | Yes | Yes | **CLI fallbacks never write** |
| **description** | DESCRIPTION + COMMENT | **No** | **No** | **No** | **No** | **No** | **Written but never read back** |
| **series** | SERIES + MVNM | **No** | **No** | **No** | Yes | Yes | **CLI fallbacks never write** |
| **series_index** | SERIES_INDEX + MVIN | **No** | **No** | **No** | Yes | Yes | **CLI fallbacks never write** |
| **isbn10** | custom tag | **No** | TXXX | Vorbis | Yes | Yes | **M4B CLI never writes** |
| **isbn13** | custom tag | **No** | TXXX | Vorbis | Yes | Yes | **M4B CLI never writes** |
| **asin** | custom tag | **No** | TXXX | Vorbis | Yes | Yes | **M4B CLI never writes** |
| **book_id** | custom tag | **No** | TXXX | Vorbis | Yes | **No** | **M4B CLI never writes; not in provenance** |
| **open_library_id** | custom tag | **No** | TXXX | Vorbis | Yes | **No** | **M4B CLI never writes; not in provenance** |
| **hardcover_id** | custom tag | **No** | TXXX | Vorbis | Yes | **No** | **M4B CLI never writes; not in provenance** |
| **google_books_id** | custom tag | **No** | TXXX | Vorbis | Yes | **No** | **M4B CLI never writes; not in provenance** |
| **edition** | custom tag | **No** | TXXX | Vorbis | Yes | Yes | **M4B CLI never writes** |
| **print_year** | custom tag | **No** | TXXX | Vorbis | Yes | Yes | **M4B CLI never writes** |

### Key Gaps

1. **`description` -- written but never read back (CRITICAL).** `writeMetadataWithTaglib` writes `DESCRIPTION` and `COMMENT` tags. `BuildMetadataFromTag` reads `COMM`/`comment` into `metadata.Comments`, but the `DESCRIPTION` key is never searched. Neither field appears in provenance.

2. **M4B CLI fallback is severely limited.** It only writes title, artist, album, narrator (embedded in comment), genre, year, track. All enrichment data (language, publisher, series, ISBNs, custom tags) is silently lost when taglib fails on M4B files.

3. **Narrator round-trip is lossy via M4B CLI.** TagLib writes a dedicated `NARRATOR` tag. M4B CLI writes `--comment "Narrator: ..."`. The reader looks for the raw `NARRATOR` key, not a "Narrator:" prefix in comments.

4. **Several custom tag fields not shown in provenance.** `book_id`, `open_library_id`, `hardcover_id`, `google_books_id`, and `AUDIOBOOK_ORGANIZER_VERSION` are read/written but never appear in `buildMetadataProvenance`.

5. **`CustomTags` struct is stale.** Lacks `Edition` and `PrintYear` fields; `ToMap()` omits those constants.

---

## 5. Test Results

**Go backend:** All 33 packages pass (`go test -count=1 ./...`). One transient failure was observed in `TestEmbedCoverArt_MissingFFmpeg` (flaky -- depends on ffmpeg availability on the build host), but it passed on re-run.

| Package | Duration |
|---------|----------|
| `internal/ai` | 57.8s |
| `internal/server` | 36.0s |
| `internal/realtime` | 34.4s |
| `internal/database` | 10.3s |
| `internal/backup` | 9.9s |
| All others | < 5s each |

**Frontend TypeScript:** `npx tsc --noEmit` completes with zero errors.

No code fixes were required to make tests pass -- the codebase was in a clean state.

---

## 6. Issues Fixed During This Session

Commits from 2026-03-20 to 2026-03-21 (newest first):

| Commit | Description |
|--------|-------------|
| `ac90990` | test(metadata): add custom tag consistency tests for write/read pipeline |
| `bb6201c` | fix(ui): prevent double API call when snapshot comparison activates |
| `26c8afd` | fix: resolve static analysis warnings |
| `8b6bc1d` | fix: add Edition and PrintYear to CustomTags struct and ToMap |
| `6857a2c` | fix: sync all metadata fields to library copies, add roundtrip test script |
| `c51c1ee` | fix(tests): update BookDetail tests for current UI, add tag labels |
| `bb269e5` | fix(tags): complete tag round-trip for all metadata fields |
| `1b23f44` | feat(covers): deduplicate archived covers with SHA-256 hashing |
| `026e4a8` | fix(tags): preserve custom tags during cover embed, read all tags back |
| `1ab28fb` | feat(ui): transpose tag comparison table, add dismiss for snapshot |
| `f4d9ea4` | fix(tagger): preserve M4B chapters when embedding cover art |
| `b5b9df4` | fix(tagger): fix ffmpeg cover embed for M4B files with data streams |
| `dee2886` | feat(ui): version group chip green by default, red if files missing |
| `06dfeb2` | feat(covers): always overwrite embedded cover art, archive old version |
| `9fc987a` | feat(tags): embed cover art during write-back, remove cover_url tag |
| `023661f` | feat(tags): write all DB fields to audio files as custom tags |
| `c4fa5f4` | fix(tags): write ASIN to files and show all tags in write-back dialog |
| `cec495d` | feat(ui): resizable columns and hideable rows in tag comparison table |
| `e04565e` | feat(ui): redesign iTunes Linked panel with rich book data |
| `fc7bd93` | test(ui): cover files history and refresh router tests |

---

*Generated by Claude Code on 2026-03-21. Agents used: claude-sonnet-4-6 (backend, frontend, tag audit), claude-opus-4-6 (test runner, tag audit).*
