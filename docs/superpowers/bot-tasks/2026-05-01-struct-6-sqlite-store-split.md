<!-- file: docs/superpowers/bot-tasks/2026-05-01-struct-6-sqlite-store-split.md -->
<!-- version: 1.0.0 -->
<!-- guid: a7b8c9d0-e1f2-3456-abcd-789012345678 -->
<!-- last-edited: 2026-05-01 -->

# STRUCT-6 — Split `sqlite_store.go` (6976 lines → 7 files)

**Priority:** High  
**Effort:** Large (mechanical move — no logic changes)  
**Branch:** `refactor/struct-6-sqlite-store-split`

---

## Why This Matters

`internal/database/sqlite_store.go` is **6976 lines** — probably the largest file
in the codebase. All SQLite store methods live in a single file, making it extremely
difficult to navigate, review, or find any given method.

**Evidence:**
```bash
wc -l internal/database/sqlite_store.go
# ~6976
```

---

## What This Task Does

Split `sqlite_store.go` into 7 files by domain. **No logic changes** — only move
functions. Package name stays `package database`.

---

## What NOT to Do

- **Do NOT** change any method signatures or logic.
- **Do NOT** rename any functions.
- **Do NOT** touch the `Store` interface in `store.go` — it does not change.
- **Do NOT** touch test files.
- **IMPORTANT:** Three `Book.ITunesPath` nil-check usages (deprecated field, lines
  ~259, ~2838, ~2996 in current file) must stay exactly as-is — they will be
  removed in a separate DEP-1e migration task.

---

## Target File Layout

### File 1: `internal/database/sqlite_store_core.go`
Struct definition, constructor, table creation, schema migrations. Lines ~1–960.
Functions to move:
- `NewSQLiteStore` (~361–410)
- `createTables` (~411–748)
- `deduplicateSeries` (~749–814)
- `ensureExtendedBookColumns` (~815–914)
- `ensureExtendedBookFileColumns` (~915–960)
- `Close` (~961–965)

Also move any top-level `type`, `const`, and `var` declarations that the above
methods depend on (e.g., `type rowScanner interface`, `const` select-column lists,
`type SQLiteStore struct`, `const bookFileCols`).

### File 2: `internal/database/sqlite_store_users.go`
User accounts, sessions, API keys, invites, preferences. Lines ~967–1280.
Functions to move:
- `CreateUser`, `scanUser`, `GetUserByID`, `GetUserByUsername`, `GetUserByEmail`
- `UpdateUser`
- `CreateSession`, `GetSession`, `RevokeSession`, `ListUserSessions`, `DeleteExpiredSessions`
- `ListUsers`, `CountUsers`
- `GetRoleByID`, `GetRoleByName`, `ListRoles`, `CreateRole`, `UpdateRole`, `DeleteRole`
- `CreateAPIKey`, `GetAPIKey`, `GetAPIKeyByHash`, `ListAPIKeysForUser`, `ListAllAPIKeys`
- `RevokeAPIKey`, `SetAPIKeyStatus`, `TouchAPIKeyLastUsed`
- `CreateInvite`, `GetInvite`, `ListActiveInvites`, `DeleteInvite`, `ConsumeInvite`
- `SetUserPreferenceForUser`, `GetUserPreferenceForUser`, `GetAllPreferencesForUser`
- `GetUserPreference`, `SetUserPreference`, `GetAllUserPreferences` (lines ~4152–4188)

### File 3: `internal/database/sqlite_store_books.go`
All book, author, series, narrator, work, and book-file CRUD. Lines ~1280–3345.
Functions to move:
- User positions and book state setters (lines ~1147–1169)
- Book version methods (lines ~1170–1184)
- Book segment methods (lines ~1278–1423)
- `GetAllAuthors`, `GetAuthorByID`, `GetAuthorByName`, `CreateAuthor`, `DeleteAuthor`,
  `UpdateAuthorName`, `GetAuthorAliases`, `GetAllAuthorAliases`, `CreateAuthorAlias`,
  `DeleteAuthorAlias`, `FindAuthorByAlias`
- All series methods: `GetAllSeries`, `DeleteSeries`, `UpdateSeriesName`,
  `GetAllSeriesBookCounts`, `GetAllSeriesFileCounts`, `GetSeriesByID`, `GetSeriesByName`,
  `CreateSeries`
- Work methods: `GetAllWorks`, `GetWorkByID`, `CreateWork`, `UpdateWork`, `DeleteWork`,
  `GetBooksByWorkID`
- Core book methods: `GetAllBooks`, `GetAllBookSummaries`, `GetDistinctGenres`,
  `GetDistinctLanguages`, `GetBookByID`, `GetBookByFilePath`, `GetBookByITunesPersistentID`
- Book hash lookups: `GetBookByFileHash`, `GetBookByOriginalHash`, `GetBookByOrganizedHash`,
  `GetBookBySegmentFileHash`, `GetBooksByMetadataSourceHash`
- Duplicate finders: `GetDuplicateBooks`, `GetBooksByTitleInDir`, `GetFolderDuplicates`,
  `GetDuplicateBooksByMetadata`
- Book relations: `GetBooksBySeriesID`, `GetBooksByAuthorID`, `GetBookAuthors`, `SetBookAuthors`,
  `GetBooksByAuthorIDWithRole`, `GetAllAuthorBookCounts`, `GetAllAuthorFileCounts`
- Narrator methods: `CreateNarrator`, `GetNarratorByID`, `GetNarratorByName`, `ListNarrators`,
  `GetBookNarrators`, `SetBookNarrators`
- Core write methods: `CreateBook`, `UpdateBook`, `UpdateBookRating`, `SetLastWrittenAt`,
  `MarkITunesSynced`, `GetITunesPurgePendingBooks`, `GetITunesDirtyBooks`, `DeleteBook`
- Search and count: `SearchBooks`, `CountBooks`, `CountFiles`
- External IDs / tags / alternative titles (lines ~4579–4903)
- Blocked hashes, fingerprints (lines ~4426–4495)
- Deferred iTunes updates (lines ~4497–4577)
- Book tags: `GetBookUserTags`, `SetBookUserTags`, `AddBookUserTag`, `RemoveBookUserTag`
- Alternative titles: `GetBookAlternativeTitles`, `AddBookAlternativeTitle`,
  `RemoveBookAlternativeTitle`, `SetBookAlternativeTitles`
- All book-file methods (lines ~6001–6499): `GetBookFiles`, `GetAllBookFiles`,
  `GetBookFilesNeedingDelugeImport`, `GetBookFileByPID`, `GetBookFileByPath`,
  `GetBookFileByAcoustID`, `GetBookFileByAcoustIDFuzzy`, `DeleteBookFile`,
  `DeleteBookFilesForBook`, `UpsertBookFile`, `BatchUpsertBookFiles`, `GetBookFileByID`,
  `MoveBookFilesToBook`, `GetQuarantinedBooks`, `CountQuarantinedBooks`, `MergeChapterBooks`,
  `FlagMetadataHashDuplicate`

### File 4: `internal/database/sqlite_store_playlists.go`
Playlists and playback. Functions to move:
- `CreatePlaylist`, `GetPlaylistByID`, `GetPlaylistBySeriesID`, `AddPlaylistItem`, `GetPlaylistItems`
- `AddPlaybackEvent`, `ListPlaybackEvents`, `UpdatePlaybackProgress`, `GetPlaybackProgress`
- `IncrementBookPlayStats`, `GetBookStats`, `IncrementUserListenStats`, `GetUserStats`

### File 5: `internal/database/sqlite_store_metadata.go`
Metadata field states and change history. Functions to move:
- `GetMetadataFieldStates`, `UpsertMetadataFieldState`, `DeleteMetadataFieldState`
- `RecordMetadataChange`, `GetMetadataChangeHistory`, `GetBookChangeHistory`
- `AddMetadataRejection`, `GetMetadataRejections`, `DeleteMetadataRejections`

### File 6: `internal/database/sqlite_store_activity.go`
Operations, operation logs, raw KV, operation results/state/params. Functions to move:
- `CreateOperation`, `GetOperationByID`, `GetRecentOperations`, `ListOperations`
- `UpdateOperationStatus`, `UpdateOperationError`, `AddOperationLog`, `GetOperationLogs`
- `SaveOperationSummaryLog`, `GetOperationSummaryLog`, `ListOperationSummaryLogs`
- `SetRaw`, `GetRaw`, `DeleteRaw`, `ScanPrefix`, `CountPrefix`
- `CreateOperationResult`, `GetOperationResults`, `GetOperationResultsPage`
- `GetRecentCompletedOperations`, `ensureOpStateTable`
- `SaveOperationState`, `GetOperationState`, `SaveOperationParams`, `GetOperationParams`
- `DeleteOperationState`, `DeleteOperationsByStatus`, `UpdateOperationResultData`
- `GetInterruptedOperations`, `truncateActivity`
- `GetScanFailCount`, `IncrScanFailCount`, `ResetScanFailCount`

### File 7: `internal/database/sqlite_store_util.go`
Scan helpers, fuzzy matching, utility functions. Functions to move:
- `scanBookSummary`, `scanBook`
- `nullableString`, `nullableInt`, `nullableFloat`
- `normalizeTitle`, `jaroWinkler`
- `fuzzyRankBooks`, `sanitizeFTS5Query`
- `boolToInt`
- `scanOperationChanges`, `bookFileScan`
- `nullableStringVal`, `nullableIntVal`, `nullableInt64Val`, `nullableTimeVal`
- `TableRowCounts`, `SQLitePageSizeBytes`
- `BookPathPrefix` type, `GetBookPathPrefixes`
- `GetAuthorsByBookIDs`, `GetNarratorsByBookIDs`

---

## Steps

### Step 1 — Baseline check

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go build ./internal/database/...
go test ./internal/database/... -timeout 180s 2>&1 | grep -E 'FAIL|ok'
wc -l internal/database/sqlite_store.go
```

### Step 2 — Identify shared declarations to put in sqlite_store_core.go

```bash
grep -n '^type\|^const\|^var' internal/database/sqlite_store.go | head -30
```

These top-level declarations (`type rowScanner interface`, `type SQLiteStore struct`,
`const bookFileCols`, etc.) must go into `sqlite_store_core.go`. Functions that
USE them can go in any file since they're package-level.

### Step 3 — Create the 7 new files one at a time

Start with `sqlite_store_core.go` (structs and constructors), then proceed in order.
For each file:
1. Create it with the version header.
2. Copy the functions listed above.
3. Add imports needed by those functions.
4. Run `go build ./internal/database/...` to check — fix errors.

Header template:
```go
// file: internal/database/sqlite_store_XXX.go
// version: 1.0.0
// guid: <generate-a-new-uuid>
// last-edited: 2026-05-01

package database
```

### Step 4 — Remove functions from sqlite_store.go

After all 7 files build cleanly alongside the original, remove the moved function
bodies from `sqlite_store.go`.

**CRITICAL:** Do NOT touch these three lines (deprecated Book.ITunesPath usages —
they will be removed in DEP-1e):
```bash
grep -n 'ITunesPath' internal/database/sqlite_store.go
```

Leave any lines at those locations untouched.

After removal, `sqlite_store.go` should be essentially empty (just package declaration
plus any declarations that didn't fit above). Delete it if empty; or keep it with
just a comment pointing to the sub-files.

### Step 5 — Final build + test

```bash
go build ./internal/database/...
go build ./...
go test ./internal/database/... -timeout 180s 2>&1 | grep -E 'FAIL|ok|---'
```

All must pass.

### Step 6 — Commit and open PR

```bash
git checkout -b refactor/struct-6-sqlite-store-split
git add internal/database/sqlite_store*.go
git commit -m "refactor(database): split sqlite_store.go into 7 domain files

Splits the 6976-line sqlite_store.go into:
- sqlite_store_core.go (struct, constructor, schema)
- sqlite_store_users.go (users, sessions, API keys)
- sqlite_store_books.go (books, authors, files, series)
- sqlite_store_playlists.go (playlists, playback)
- sqlite_store_metadata.go (field states, change history)
- sqlite_store_activity.go (operations, KV, results)
- sqlite_store_util.go (scan helpers, fuzzy, utils)

No logic changes. Structure audit STRUCT-6.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin refactor/struct-6-sqlite-store-split
gh pr create \
  --title "refactor(database): split sqlite_store.go into 7 domain files" \
  --body "Splits 6976-line file into 7 focused files. No logic changes. Structure audit STRUCT-6."
```

---

## Checklist

- [ ] 7 new files created with version headers
- [ ] `SQLiteStore` struct and `rowScanner` interface in `sqlite_store_core.go`
- [ ] Deprecated `Book.ITunesPath` usages (3 lines) left untouched
- [ ] `go build ./internal/database/...` clean
- [ ] `go build ./...` clean
- [ ] `go test ./internal/database/...` passes
- [ ] PR opened on branch `refactor/struct-6-sqlite-store-split`
