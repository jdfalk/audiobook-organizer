<!-- file: docs/superpowers/plans/2026-04-17-store-interface-segregation.md -->
<!-- version: 1.0.0 -->
<!-- guid: 46d32d6c-606d-473b-89a7-f32ee81b3231 -->

# Store Interface Segregation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split `internal/database/store.go`'s ~281-method `Store` interface into ~41 focused sub-interfaces (hybrid read/write split) without breaking any implementation or caller. Migrate 3 proof-point services to narrow interfaces, then hand off the remaining 58 files to a follow-on agent via a detailed migration catalog.

**Architecture:** Define one sub-interface per logical domain in new `iface_<domain>.go` files. Read/write split for hot domains (Book, Author, Series, User); single interface per domain everywhere else. Top-level `Store` becomes a pure embedding block. `PebbleStore` satisfies everything via Go's structural typing — no implementation changes. Mockery regenerates per-interface mocks alongside the existing `mocks.Store`.

**Tech Stack:** Go 1.26, `pgregory.net/rapid` (existing), mockery v3 (existing). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-04-17-store-interface-segregation-design.md` — read first for the full migration catalog.

---

## Execution model

Each task = **one PR** via the Quick Fix Workflow in `CLAUDE.md`. Steps 6 and 7 can parallelize after task 2 merges. Task 1 must land before any task 3–6.

| # | PR | Depends on |
|---|---|---|
| 1 | Define all sub-interfaces + refactor `Store` to embed them | — |
| 2 | `.mockery.yaml` additions + regen new mock files | 1 |
| 3 | Proof-point: migrate `playlist_evaluator.go` | 1 (task 2 optional) |
| 4 | Proof-point: migrate `audiobook_service.go` | 1 |
| 5 | Proof-point: migrate `reconcile.go` | 1 |
| 6 | Write follow-on migration plan (catalog → task list) | 1, 3, 4, 5 |

---

## Task 1: Define sub-interfaces and refactor `Store`

**Goal:** Ship all ~41 sub-interfaces defined, with `Store` reduced to an embedding block. `*PebbleStore` must still satisfy `database.Store` unchanged.

**Files:**
- Create: `internal/database/iface_book.go`
- Create: `internal/database/iface_author.go`
- Create: `internal/database/iface_series.go`
- Create: `internal/database/iface_user.go`
- Create: `internal/database/iface_tags.go`
- Create: `internal/database/iface_itunes.go`
- Create: `internal/database/iface_ops.go`
- Create: `internal/database/iface_misc.go`
- Modify: `internal/database/store.go` (shrink the `Store` interface block to pure embedding)

- [ ] **Step 1.1: Worktree + branch**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git fetch origin main
git worktree add .worktrees/iface-segregation -b refactor/store-iface-segregation origin/main
cd .worktrees/iface-segregation
```

- [ ] **Step 1.2: Create `iface_book.go` with Book read/write split**

Generate a fresh GUID: `uuidgen | tr '[:upper:]' '[:lower:]'`.

```go
// file: internal/database/iface_book.go
// version: 1.0.0
// guid: <your-guid>

package database

import "time"

// BookReader is the read-only slice of Store for callers that only
// read books. See spec 2026-04-17-store-interface-segregation-design.md.
type BookReader interface {
	GetBookByID(id string) (*Book, error)
	GetAllBooks(limit, offset int) ([]Book, error)
	GetBookByFilePath(path string) (*Book, error)
	GetBookByITunesPersistentID(persistentID string) (*Book, error)
	GetBookByFileHash(hash string) (*Book, error)
	GetBookByOriginalHash(hash string) (*Book, error)
	GetBookByOrganizedHash(hash string) (*Book, error)
	GetDuplicateBooks() ([][]Book, error)
	GetFolderDuplicates() ([][]Book, error)
	GetDuplicateBooksByMetadata(threshold float64) ([][]Book, error)
	GetBooksByTitleInDir(normalizedTitle, dirPath string) ([]Book, error)
	GetBooksBySeriesID(seriesID int) ([]Book, error)
	GetBooksByAuthorID(authorID int) ([]Book, error)
	GetBooksByVersionGroup(groupID string) ([]Book, error)
	SearchBooks(query string, limit, offset int) ([]Book, error)
	CountBooks() (int, error)
	ListSoftDeletedBooks(limit, offset int, olderThan *time.Time) ([]Book, error)
	GetBookSnapshots(id string, limit int) ([]BookSnapshot, error)
	GetBookAtVersion(id string, ts time.Time) (*Book, error)
	GetBookTombstone(id string) (*Book, error)
	ListBookTombstones(limit int) ([]Book, error)
	GetITunesDirtyBooks() ([]Book, error)
}

// BookWriter is the write-only slice of Store for callers that only
// mutate books.
type BookWriter interface {
	CreateBook(book *Book) (*Book, error)
	UpdateBook(id string, book *Book) (*Book, error)
	DeleteBook(id string) error
	SetLastWrittenAt(id string, t time.Time) error
	MarkITunesSynced(bookIDs []string) (int64, error)
	RevertBookToVersion(id string, ts time.Time) (*Book, error)
	PruneBookSnapshots(id string, keepCount int) (int, error)
	CreateBookTombstone(book *Book) error
	DeleteBookTombstone(id string) error
}

// BookStore combines BookReader and BookWriter for callers that need both.
type BookStore interface {
	BookReader
	BookWriter
}
```

- [ ] **Step 1.3: Create `iface_author.go`**

```go
// file: internal/database/iface_author.go
// version: 1.0.0
// guid: <fresh-uuid>

package database

// AuthorReader is the read-only author slice (authors + aliases + book-author joins).
type AuthorReader interface {
	GetAllAuthors() ([]Author, error)
	GetAuthorByID(id int) (*Author, error)
	GetAuthorByName(name string) (*Author, error)
	GetAuthorAliases(authorID int) ([]AuthorAlias, error)
	GetAllAuthorAliases() ([]AuthorAlias, error)
	FindAuthorByAlias(aliasName string) (*Author, error)
	GetBookAuthors(bookID string) ([]BookAuthor, error)
	GetBooksByAuthorIDWithRole(authorID int) ([]Book, error)
	GetAllAuthorBookCounts() (map[int]int, error)
	GetAllAuthorFileCounts() (map[int]int, error)
	GetAuthorTombstone(oldID int) (int, error)
}

// AuthorWriter is the write-only author slice.
type AuthorWriter interface {
	CreateAuthor(name string) (*Author, error)
	DeleteAuthor(id int) error
	UpdateAuthorName(id int, name string) error
	CreateAuthorAlias(authorID int, aliasName string, aliasType string) (*AuthorAlias, error)
	DeleteAuthorAlias(id int) error
	SetBookAuthors(bookID string, authors []BookAuthor) error
	CreateAuthorTombstone(oldID, canonicalID int) error
	ResolveTombstoneChains() (int, error)
}

// AuthorStore combines both halves.
type AuthorStore interface {
	AuthorReader
	AuthorWriter
}
```

- [ ] **Step 1.4: Create `iface_series.go`**

```go
// file: internal/database/iface_series.go
// version: 1.0.0
// guid: <fresh-uuid>

package database

// SeriesReader is the read-only series slice.
type SeriesReader interface {
	GetAllSeries() ([]Series, error)
	GetSeriesByID(id int) (*Series, error)
	GetSeriesByName(name string, authorID *int) (*Series, error)
	GetAllSeriesBookCounts() (map[int]int, error)
	GetAllSeriesFileCounts() (map[int]int, error)
}

// SeriesWriter is the write-only series slice.
type SeriesWriter interface {
	CreateSeries(name string, authorID *int) (*Series, error)
	DeleteSeries(id int) error
	UpdateSeriesName(id int, name string) error
}

// SeriesStore combines both halves.
type SeriesStore interface {
	SeriesReader
	SeriesWriter
}
```

- [ ] **Step 1.5: Create `iface_user.go`**

```go
// file: internal/database/iface_user.go
// version: 1.0.0
// guid: <fresh-uuid>

package database

// UserReader is the read-only user slice.
type UserReader interface {
	GetUserByID(id string) (*User, error)
	GetUserByUsername(username string) (*User, error)
	GetUserByEmail(email string) (*User, error)
	ListUsers() ([]User, error)
	CountUsers() (int, error)
}

// UserWriter is the write-only user slice.
type UserWriter interface {
	CreateUser(username, email, passwordHashAlgo, passwordHash string, roles []string, status string) (*User, error)
	UpdateUser(user *User) error
}

// UserStore combines both halves.
type UserStore interface {
	UserReader
	UserWriter
}
```

- [ ] **Step 1.6: Create `iface_tags.go`**

```go
// file: internal/database/iface_tags.go
// version: 1.0.0
// guid: <fresh-uuid>

package database

// TagStore covers book/author/series tag operations (source-tracked).
// Matches the "Tags" section of the legacy Store interface.
type TagStore interface {
	// Book tags
	AddBookTag(bookID, tag string) error
	AddBookTagWithSource(bookID, tag, source string) error
	RemoveBookTag(bookID, tag string) error
	RemoveBookTagsByPrefix(bookID, prefix, source string) error
	GetBookTags(bookID string) ([]string, error)
	GetBookTagsDetailed(bookID string) ([]BookTag, error)
	SetBookTags(bookID string, tags []string) error
	ListAllTags() ([]TagWithCount, error)
	GetBooksByTag(tag string) ([]string, error)

	// Author tags
	AddAuthorTag(authorID int, tag string) error
	AddAuthorTagWithSource(authorID int, tag, source string) error
	RemoveAuthorTag(authorID int, tag string) error
	RemoveAuthorTagsByPrefix(authorID int, prefix, source string) error
	GetAuthorTags(authorID int) ([]string, error)
	GetAuthorTagsDetailed(authorID int) ([]BookTag, error)
	SetAuthorTags(authorID int, tags []string) error
	ListAllAuthorTags() ([]TagWithCount, error)
	GetAuthorsByTag(tag string) ([]int, error)

	// Series tags
	AddSeriesTag(seriesID int, tag string) error
	AddSeriesTagWithSource(seriesID int, tag, source string) error
	RemoveSeriesTag(seriesID int, tag string) error
	RemoveSeriesTagsByPrefix(seriesID int, prefix, source string) error
	GetSeriesTags(seriesID int) ([]string, error)
	GetSeriesTagsDetailed(seriesID int) ([]BookTag, error)
	SetSeriesTags(seriesID int, tags []string) error
	ListAllSeriesTags() ([]TagWithCount, error)
	GetSeriesByTag(tag string) ([]int, error)
}

// UserTagStore covers free-form per-book user tags (the *BookUserTag* variants).
type UserTagStore interface {
	GetBookUserTags(bookID string) ([]string, error)
	SetBookUserTags(bookID string, tags []string) error
	AddBookUserTag(bookID string, tag string) error
	RemoveBookUserTag(bookID string, tag string) error
}
```

- [ ] **Step 1.7: Create `iface_itunes.go`**

```go
// file: internal/database/iface_itunes.go
// version: 1.0.0
// guid: <fresh-uuid>

package database

import "time"

// ITunesStateStore covers iTunes library fingerprints and deferred updates.
type ITunesStateStore interface {
	SaveLibraryFingerprint(path string, size int64, modTime time.Time, crc32 uint32) error
	GetLibraryFingerprint(path string) (*LibraryFingerprintRecord, error)
	CreateDeferredITunesUpdate(bookID, persistentID, oldPath, newPath, updateType string) error
	GetPendingDeferredITunesUpdates() ([]DeferredITunesUpdate, error)
	MarkDeferredITunesUpdateApplied(id int) error
	GetDeferredITunesUpdatesByBookID(bookID string) ([]DeferredITunesUpdate, error)
}

// ExternalIDStore covers ExternalIDMapping CRUD + tombstones.
type ExternalIDStore interface {
	CreateExternalIDMapping(mapping *ExternalIDMapping) error
	GetBookByExternalID(source, externalID string) (string, error)
	GetExternalIDsForBook(bookID string) ([]ExternalIDMapping, error)
	IsExternalIDTombstoned(source, externalID string) (bool, error)
	TombstoneExternalID(source, externalID string) error
	ReassignExternalIDs(oldBookID, newBookID string) error
	BulkCreateExternalIDMappings(mappings []ExternalIDMapping) error
	MarkExternalIDRemoved(source, externalID string) error
	SetExternalIDProvenance(source, externalID, provenance string) error
	GetRemovedExternalIDs(source string) ([]ExternalIDMapping, error)
}

// PathHistoryStore covers file rename/move history.
type PathHistoryStore interface {
	RecordPathChange(change *BookPathChange) error
	GetBookPathHistory(bookID string) ([]BookPathChange, error)
}
```

- [ ] **Step 1.8: Create `iface_ops.go`**

```go
// file: internal/database/iface_ops.go
// version: 1.0.0
// guid: <fresh-uuid>

package database

import "time"

// OperationStore covers the full operation-tracking surface:
// Operation + logs + state + results + changes + summary + retention.
type OperationStore interface {
	// Operation CRUD
	CreateOperation(id, opType string, folderPath *string) (*Operation, error)
	GetOperationByID(id string) (*Operation, error)
	GetRecentOperations(limit int) ([]Operation, error)
	ListOperations(limit, offset int) ([]Operation, int, error)
	UpdateOperationStatus(id, status string, progress, total int, message string) error
	UpdateOperationError(id, errorMessage string) error
	UpdateOperationResultData(id string, resultData string) error

	// State persistence (resumable operations)
	SaveOperationState(opID string, state []byte) error
	GetOperationState(opID string) ([]byte, error)
	SaveOperationParams(opID string, params []byte) error
	GetOperationParams(opID string) ([]byte, error)
	DeleteOperationState(opID string) error
	GetInterruptedOperations() ([]Operation, error)

	// Change tracking (undo/rollback)
	CreateOperationChange(change *OperationChange) error
	GetOperationChanges(operationID string) ([]*OperationChange, error)
	GetBookChanges(bookID string) ([]*OperationChange, error)
	RevertOperationChanges(operationID string) error

	// Logs
	AddOperationLog(operationID, level, message string, details *string) error
	GetOperationLogs(operationID string) ([]OperationLog, error)

	// Summary logs (persistent across restarts)
	SaveOperationSummaryLog(op *OperationSummaryLog) error
	GetOperationSummaryLog(id string) (*OperationSummaryLog, error)
	ListOperationSummaryLogs(limit, offset int) ([]OperationSummaryLog, error)

	// Per-book result rows
	CreateOperationResult(result *OperationResult) error
	GetOperationResults(operationID string) ([]OperationResult, error)
	GetRecentCompletedOperations(limit int) ([]Operation, error)

	// Retention
	PruneOperationLogs(olderThan time.Time) (int, error)
	PruneOperationChanges(olderThan time.Time) (int, error)
	DeleteOperationsByStatus(statuses []string) (int, error)
}
```

- [ ] **Step 1.9: Create `iface_misc.go` with the remaining single-interfaces**

This file collects the 25 single-interfaces that are too small or stable to warrant their own file. Use the list below verbatim.

```go
// file: internal/database/iface_misc.go
// version: 1.0.0
// guid: <fresh-uuid>

package database

import "time"

// LifecycleStore covers store startup/teardown.
type LifecycleStore interface {
	Close() error
	Reset() error
}

// NarratorStore covers narrators + book-narrator joins.
type NarratorStore interface {
	CreateNarrator(name string) (*Narrator, error)
	GetNarratorByID(id int) (*Narrator, error)
	GetNarratorByName(name string) (*Narrator, error)
	ListNarrators() ([]Narrator, error)
	GetBookNarrators(bookID string) ([]BookNarrator, error)
	SetBookNarrators(bookID string, narrators []BookNarrator) error
}

// WorkStore covers Work CRUD.
type WorkStore interface {
	GetAllWorks() ([]Work, error)
	GetWorkByID(id string) (*Work, error)
	CreateWork(work *Work) (*Work, error)
	UpdateWork(id string, work *Work) (*Work, error)
	DeleteWork(id string) error
	GetBooksByWorkID(workID string) ([]Book, error)
}

// SessionStore covers authenticated session CRUD.
type SessionStore interface {
	CreateSession(userID, ip, userAgent string, ttl time.Duration) (*Session, error)
	GetSession(id string) (*Session, error)
	RevokeSession(id string) error
	ListUserSessions(userID string) ([]Session, error)
	DeleteExpiredSessions(now time.Time) (int, error)
}

// RoleStore covers Role CRUD.
type RoleStore interface {
	GetRoleByID(id string) (*Role, error)
	GetRoleByName(name string) (*Role, error)
	ListRoles() ([]Role, error)
	CreateRole(role *Role) (*Role, error)
	UpdateRole(role *Role) error
	DeleteRole(id string) error
}

// APIKeyStore covers APIKey CRUD and revocation.
type APIKeyStore interface {
	CreateAPIKey(key *APIKey) (*APIKey, error)
	GetAPIKey(id string) (*APIKey, error)
	ListAPIKeysForUser(userID string) ([]APIKey, error)
	RevokeAPIKey(id string) error
	TouchAPIKeyLastUsed(id string, at time.Time) error
}

// InviteStore covers Invite CRUD and atomic consume.
type InviteStore interface {
	CreateInvite(invite *Invite) (*Invite, error)
	GetInvite(token string) (*Invite, error)
	ListActiveInvites() ([]Invite, error)
	DeleteInvite(token string) error
	ConsumeInvite(token, passwordHashAlgo, passwordHash string) (*User, error)
}

// UserPreferenceStore covers both global and per-user preferences.
type UserPreferenceStore interface {
	GetUserPreference(key string) (*UserPreference, error)
	SetUserPreference(key, value string) error
	GetAllUserPreferences() ([]UserPreference, error)
	SetUserPreferenceForUser(userID, key, value string) error
	GetUserPreferenceForUser(userID, key string) (*UserPreferenceKV, error)
	GetAllPreferencesForUser(userID string) ([]UserPreferenceKV, error)
}

// UserPositionStore covers per-user position + derived book state.
type UserPositionStore interface {
	SetUserPosition(userID, bookID, segmentID string, positionSeconds float64) error
	GetUserPosition(userID, bookID string) (*UserPosition, error)
	ListUserPositionsForBook(userID, bookID string) ([]UserPosition, error)
	ClearUserPositions(userID, bookID string) error
	SetUserBookState(state *UserBookState) error
	GetUserBookState(userID, bookID string) (*UserBookState, error)
	ListUserBookStatesByStatus(userID, status string, limit, offset int) ([]UserBookState, error)
	ListUserPositionsSince(userID string, t time.Time) ([]UserPosition, error)
}

// BookVersionStore covers version CRUD, lifecycle, and lookups.
type BookVersionStore interface {
	CreateBookVersion(v *BookVersion) (*BookVersion, error)
	GetBookVersion(id string) (*BookVersion, error)
	GetBookVersionsByBookID(bookID string) ([]BookVersion, error)
	GetActiveVersionForBook(bookID string) (*BookVersion, error)
	UpdateBookVersion(v *BookVersion) error
	DeleteBookVersion(id string) error
	GetBookVersionByTorrentHash(hash string) (*BookVersion, error)
	ListTrashedBookVersions() ([]BookVersion, error)
	ListPurgedBookVersions() ([]BookVersion, error)
}

// BookFileStore covers the canonical BookFile surface.
type BookFileStore interface {
	CreateBookFile(file *BookFile) error
	UpdateBookFile(id string, file *BookFile) error
	GetBookFiles(bookID string) ([]BookFile, error)
	GetBookFileByID(bookID, fileID string) (*BookFile, error)
	GetBookFileByPID(itunesPID string) (*BookFile, error)
	GetBookFileByPath(filePath string) (*BookFile, error)
	DeleteBookFile(id string) error
	DeleteBookFilesForBook(bookID string) error
	UpsertBookFile(file *BookFile) error
	BatchUpsertBookFiles(files []*BookFile) error
	MoveBookFilesToBook(fileIDs []string, sourceBookID, targetBookID string) error
}

// BookSegmentStore covers the deprecated segment surface, kept until
// the segment-removal PR.
type BookSegmentStore interface {
	CreateBookSegment(bookNumericID int, segment *BookSegment) (*BookSegment, error)
	UpdateBookSegment(segment *BookSegment) error
	ListBookSegments(bookNumericID int) ([]BookSegment, error)
	MergeBookSegments(bookNumericID int, newSegment *BookSegment, supersedeIDs []string) error
	GetBookSegmentByID(segmentID string) (*BookSegment, error)
	MoveSegmentsToBook(segmentIDs []string, targetBookNumericID int) error
}

// PlaylistStore covers the legacy series-playlist auto-generator.
type PlaylistStore interface {
	CreatePlaylist(name string, seriesID *int, filePath string) (*Playlist, error)
	GetPlaylistByID(id int) (*Playlist, error)
	GetPlaylistBySeriesID(seriesID int) (*Playlist, error)
	AddPlaylistItem(playlistID, bookID, position int) error
	GetPlaylistItems(playlistID int) ([]PlaylistItem, error)
}

// UserPlaylistStore covers smart + static user playlists (spec 3.4).
type UserPlaylistStore interface {
	CreateUserPlaylist(pl *UserPlaylist) (*UserPlaylist, error)
	GetUserPlaylist(id string) (*UserPlaylist, error)
	GetUserPlaylistByName(name string) (*UserPlaylist, error)
	GetUserPlaylistByITunesPID(pid string) (*UserPlaylist, error)
	ListUserPlaylists(playlistType string, limit, offset int) ([]UserPlaylist, int, error)
	UpdateUserPlaylist(pl *UserPlaylist) error
	DeleteUserPlaylist(id string) error
	ListDirtyUserPlaylists() ([]UserPlaylist, error)
}

// ImportPathStore covers managed import path CRUD.
type ImportPathStore interface {
	GetAllImportPaths() ([]ImportPath, error)
	GetImportPathByID(id int) (*ImportPath, error)
	GetImportPathByPath(path string) (*ImportPath, error)
	CreateImportPath(path, name string) (*ImportPath, error)
	UpdateImportPath(id int, importPath *ImportPath) error
	DeleteImportPath(id int) error
}

// MetadataStore covers MetadataFieldState, change history, and
// alternative titles.
type MetadataStore interface {
	GetMetadataFieldStates(bookID string) ([]MetadataFieldState, error)
	UpsertMetadataFieldState(state *MetadataFieldState) error
	DeleteMetadataFieldState(bookID, field string) error
	RecordMetadataChange(record *MetadataChangeRecord) error
	GetMetadataChangeHistory(bookID string, field string, limit int) ([]MetadataChangeRecord, error)
	GetBookChangeHistory(bookID string, limit int) ([]MetadataChangeRecord, error)
	GetBookAlternativeTitles(bookID string) ([]BookAlternativeTitle, error)
	AddBookAlternativeTitle(bookID, title, source, language string) error
	RemoveBookAlternativeTitle(bookID, title string) error
	SetBookAlternativeTitles(bookID string, titles []BookAlternativeTitle) error
}

// HashBlocklistStore covers DoNotImport entries.
type HashBlocklistStore interface {
	IsHashBlocked(hash string) (bool, error)
	AddBlockedHash(hash, reason string) error
	RemoveBlockedHash(hash string) error
	GetAllBlockedHashes() ([]DoNotImport, error)
	GetBlockedHashByHash(hash string) (*DoNotImport, error)
}

// RawKVStore covers the low-level key-value escape hatch.
type RawKVStore interface {
	SetRaw(key string, value []byte) error
	GetRaw(key string) ([]byte, error)
	DeleteRaw(key string) error
	ScanPrefix(prefix string) ([]KVPair, error)
}

// PlaybackStore covers playback events, progress, and stats.
type PlaybackStore interface {
	AddPlaybackEvent(event *PlaybackEvent) error
	ListPlaybackEvents(userID string, bookNumericID int, limit int) ([]PlaybackEvent, error)
	UpdatePlaybackProgress(progress *PlaybackProgress) error
	GetPlaybackProgress(userID string, bookNumericID int) (*PlaybackProgress, error)
	IncrementBookPlayStats(bookNumericID int, seconds int) error
	GetBookStats(bookNumericID int) (*BookStats, error)
	IncrementUserListenStats(userID string, seconds int) error
	GetUserStats(userID string) (*UserStats, error)
}

// SettingsStore covers persistent encrypted configuration.
type SettingsStore interface {
	GetSetting(key string) (*Setting, error)
	SetSetting(key, value, typ string, isSecret bool) error
	GetAllSettings() ([]Setting, error)
	DeleteSetting(key string) error
}

// StatsStore covers aggregate counts and dashboard metrics.
type StatsStore interface {
	CountFiles() (int, error)
	CountAuthors() (int, error)
	CountSeries() (int, error)
	GetBookCountsByLocation(rootDir string) (library, import_ int, err error)
	GetBookSizesByLocation(rootDir string) (librarySize, importSize int64, err error)
	GetDashboardStats() (*DashboardStats, error)
}

// MaintenanceStore covers database maintenance and scan-cache.
type MaintenanceStore interface {
	Optimize() error
	GetScanCacheMap() (map[string]ScanCacheEntry, error)
	UpdateScanCache(bookID string, mtime int64, size int64) error
	MarkNeedsRescan(bookID string) error
	GetDirtyBookFolders() ([]string, error)
}

// SystemActivityStore covers cross-cutting system activity log.
type SystemActivityStore interface {
	AddSystemActivityLog(source, level, message string) error
	GetSystemActivityLogs(source string, limit int) ([]SystemActivityLog, error)
	PruneSystemActivityLogs(olderThan time.Time) (int, error)
}
```

- [ ] **Step 1.10: Refactor `Store` in `store.go` to pure embedding**

Read the current `Store` interface (lines 15–446 of `internal/database/store.go`). Replace the entire method-listing block with an embedding-only definition. The file stays in place, but drops from ~430 lines to ~35 for the interface block — shared type definitions below line 446 stay untouched.

Edit `internal/database/store.go`:

```go
// Store defines the full database surface. Most services should depend
// on a narrower sub-interface defined in iface_*.go; Store itself is
// used by the server bootstrap and test fixtures that genuinely need
// wide access. See docs/superpowers/specs/2026-04-17-store-interface-segregation-design.md.
type Store interface {
	LifecycleStore
	BookStore
	AuthorStore
	SeriesStore
	UserStore
	NarratorStore
	WorkStore
	SessionStore
	RoleStore
	APIKeyStore
	InviteStore
	UserPreferenceStore
	UserPositionStore
	BookVersionStore
	BookFileStore
	BookSegmentStore
	PlaylistStore
	UserPlaylistStore
	ImportPathStore
	OperationStore
	TagStore
	UserTagStore
	MetadataStore
	HashBlocklistStore
	ITunesStateStore
	PathHistoryStore
	ExternalIDStore
	RawKVStore
	PlaybackStore
	SettingsStore
	StatsStore
	MaintenanceStore
	SystemActivityStore
}
```

Bump the `// version:` header on `store.go` one minor (e.g. `2.55.0` → `2.56.0`).

- [ ] **Step 1.11: Build-and-vet gate**

```bash
go build ./...
go vet ./...
```

Expected: both clean. If the build fails with "Duplicate method" errors, a method is defined in more than one sub-interface — fix by picking one home for it (tag methods, preferences, and position methods are the common pitfalls).

- [ ] **Step 1.12: Compile-time assertion that PebbleStore still satisfies Store**

Add to the bottom of `internal/database/pebble_store.go` (or create a tiny `internal/database/iface_assert_test.go` — your call; the spec prefers a regular non-test file so the check runs on `go build`):

```go
// iface_assert.go (new file, same package) — OR append to pebble_store.go
var _ Store = (*PebbleStore)(nil)
var _ BookStore = (*PebbleStore)(nil)
var _ AuthorStore = (*PebbleStore)(nil)
var _ SeriesStore = (*PebbleStore)(nil)
var _ UserStore = (*PebbleStore)(nil)
```

Prefer creating a new file `internal/database/iface_assert.go` that holds all these assertions in one place:

```go
// file: internal/database/iface_assert.go
// version: 1.0.0
// guid: <fresh-uuid>

package database

// Compile-time proof that PebbleStore satisfies every sub-interface
// defined in iface_*.go. If a method is ever removed from PebbleStore
// (or renamed) the compile fails here — long before any caller does.

var (
	_ Store                = (*PebbleStore)(nil)
	_ LifecycleStore       = (*PebbleStore)(nil)
	_ BookStore            = (*PebbleStore)(nil)
	_ AuthorStore          = (*PebbleStore)(nil)
	_ SeriesStore          = (*PebbleStore)(nil)
	_ UserStore            = (*PebbleStore)(nil)
	_ NarratorStore        = (*PebbleStore)(nil)
	_ WorkStore            = (*PebbleStore)(nil)
	_ SessionStore         = (*PebbleStore)(nil)
	_ RoleStore            = (*PebbleStore)(nil)
	_ APIKeyStore          = (*PebbleStore)(nil)
	_ InviteStore          = (*PebbleStore)(nil)
	_ UserPreferenceStore  = (*PebbleStore)(nil)
	_ UserPositionStore    = (*PebbleStore)(nil)
	_ BookVersionStore     = (*PebbleStore)(nil)
	_ BookFileStore        = (*PebbleStore)(nil)
	_ BookSegmentStore     = (*PebbleStore)(nil)
	_ PlaylistStore        = (*PebbleStore)(nil)
	_ UserPlaylistStore    = (*PebbleStore)(nil)
	_ ImportPathStore      = (*PebbleStore)(nil)
	_ OperationStore       = (*PebbleStore)(nil)
	_ TagStore             = (*PebbleStore)(nil)
	_ UserTagStore         = (*PebbleStore)(nil)
	_ MetadataStore        = (*PebbleStore)(nil)
	_ HashBlocklistStore   = (*PebbleStore)(nil)
	_ ITunesStateStore     = (*PebbleStore)(nil)
	_ PathHistoryStore     = (*PebbleStore)(nil)
	_ ExternalIDStore      = (*PebbleStore)(nil)
	_ RawKVStore           = (*PebbleStore)(nil)
	_ PlaybackStore        = (*PebbleStore)(nil)
	_ SettingsStore        = (*PebbleStore)(nil)
	_ StatsStore           = (*PebbleStore)(nil)
	_ MaintenanceStore     = (*PebbleStore)(nil)
	_ SystemActivityStore  = (*PebbleStore)(nil)
)
```

- [ ] **Step 1.13: Run the test suite**

```bash
go test ./... -count=1 2>&1 | tail -30
```

Expected: all passing. No test logic changed — this is pure type refactoring. If anything fails, a method probably moved to the wrong sub-interface and `PebbleStore` no longer satisfies the assertion — re-read the compile error and relocate.

- [ ] **Step 1.14: Commit**

```bash
git add internal/database/iface_*.go internal/database/store.go
git commit -m "$(cat <<'EOF'
refactor: split Store into focused sub-interfaces (ISP)

Define ~41 sub-interfaces covering every method of the previously
monolithic Store: Reader/Writer split for Book/Author/Series/User
hot domains, single interface per domain for everything else. Store
is now an embedding-only umbrella. PebbleStore satisfies every
sub-interface (compile-time assertion in iface_assert.go).

No behavior change. Callers unchanged — PebbleStore continues to
satisfy every consumer's type. Proof-point migrations ship in
subsequent PRs per the implementation plan.

Spec: docs/superpowers/specs/2026-04-17-store-interface-segregation-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 1.15: Push + PR + merge**

```bash
git push -u origin refactor/store-iface-segregation
gh pr create --title "refactor: split Store into focused sub-interfaces (ISP)" --body "$(cat <<'EOF'
## Summary

Defines ~41 sub-interfaces across new `iface_*.go` files. The top-level \`Store\` is now a pure embedding block — zero method definitions of its own. \`PebbleStore\` satisfies every sub-interface (enforced by \`iface_assert.go\`).

## Scope

- **Added:** 8 new interface files (\`iface_book.go\`, \`iface_author.go\`, \`iface_series.go\`, \`iface_user.go\`, \`iface_tags.go\`, \`iface_itunes.go\`, \`iface_ops.go\`, \`iface_misc.go\`), 1 compile-time assertion file (\`iface_assert.go\`)
- **Modified:** \`store.go\` \`Store\` interface shrunk from ~430 lines to ~35 (pure embedding)
- **Unchanged:** Every caller. PebbleStore implementation. Mocks (regenerated in follow-up PR).

## Test plan

- [x] \`go build ./...\` clean
- [x] \`go vet ./...\` clean
- [x] \`go test ./... -count=1\` green

Spec: \`docs/superpowers/specs/2026-04-17-store-interface-segregation-design.md\`
EOF
)"

gh pr merge <num> --rebase --admin
```

---

## Task 2: Mockery config + regen mocks

**Goal:** Generate per-interface mocks for every new sub-interface. Preserve the existing full `mocks.Store` for tests that still want it.

**Files:**
- Modify: `.mockery.yaml`
- Generated (don't hand-edit): `internal/database/mocks/mock_*.go` (~41 new files)

- [ ] **Step 2.1: Branch**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git fetch origin main
git worktree add .worktrees/iface-mocks -b refactor/store-iface-mocks origin/main
cd .worktrees/iface-mocks
```

- [ ] **Step 2.2: Update `.mockery.yaml`**

Find the existing `github.com/jdfalk/audiobook-organizer/internal/database:` block. Keep the existing `Store:` entry. Add one entry per sub-interface defined in task 1.

Edit `.mockery.yaml` so the block reads:

```yaml
  github.com/jdfalk/audiobook-organizer/internal/database:
    interfaces:
      Store:
      LifecycleStore:
      BookReader:
      BookWriter:
      BookStore:
      AuthorReader:
      AuthorWriter:
      AuthorStore:
      SeriesReader:
      SeriesWriter:
      SeriesStore:
      UserReader:
      UserWriter:
      UserStore:
      NarratorStore:
      WorkStore:
      SessionStore:
      RoleStore:
      APIKeyStore:
      InviteStore:
      UserPreferenceStore:
      UserPositionStore:
      BookVersionStore:
      BookFileStore:
      BookSegmentStore:
      PlaylistStore:
      UserPlaylistStore:
      ImportPathStore:
      OperationStore:
      TagStore:
      UserTagStore:
      MetadataStore:
      HashBlocklistStore:
      ITunesStateStore:
      PathHistoryStore:
      ExternalIDStore:
      RawKVStore:
      PlaybackStore:
      SettingsStore:
      StatsStore:
      MaintenanceStore:
      SystemActivityStore:
```

Bump the `.mockery.yaml` `# version:` header one minor.

- [ ] **Step 2.3: Regenerate mocks**

```bash
make mocks
```

Expected: creates ~41 new files under `internal/database/mocks/`, each named `mock_<interface>.go`. If `make mocks` fails, read the mockery error — it usually means an interface name in `.mockery.yaml` doesn't match the Go source.

- [ ] **Step 2.4: Run `mocks-check`**

```bash
make mocks-check
```

Expected: clean. This CI gate (backlog 5.9) ensures generated mocks match the interface — it will catch any mistakes where the generated file doesn't reflect the current interface source.

- [ ] **Step 2.5: Run full test suite**

```bash
go test ./... -count=1 2>&1 | tail -10
```

Expected: green. New mocks are unused by any existing test, so this step verifies generation didn't corrupt existing mocks.

- [ ] **Step 2.6: Commit + PR + merge**

```bash
git add .mockery.yaml internal/database/mocks/
git commit -m "$(cat <<'EOF'
chore: generate per-interface mocks for sub-interfaces

Adds mockery entries for every sub-interface defined in the ISP
refactor. Full Store mock preserved for tests that still need it;
new focused mocks (mock_book_reader.go, mock_tag_store.go, etc.)
available for service tests that narrow their dependency.

No source code or behavior change. mocks-check CI gate stays green.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git push -u origin refactor/store-iface-mocks
gh pr create --title "chore: generate per-interface mocks for sub-interfaces" --body "Mockery additions for the ISP sub-interfaces. Full Store mock preserved. Depends on PR #<task-1-pr>.

## Test plan
- [x] \`make mocks\` succeeds
- [x] \`make mocks-check\` clean
- [x] \`go test ./...\` green"
gh pr merge <num> --rebase --admin
```

---

## Task 3: Proof-point — migrate `playlist_evaluator.go`

**Goal:** Narrow three free-function `store database.Store` parameters to the minimum interface each needs. Smallest surface of the three proof-points — start here.

**Files:**
- Modify: `internal/server/playlist_evaluator.go`
- **Unchanged:** `internal/server/playlist_evaluator_test.go`, `internal/server/playlist_evaluator_prop_test.go` (they use real `*PebbleStore`, which still satisfies every sub-interface — the tests exercise the narrowed types for free)

- [ ] **Step 3.1: Branch**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git fetch origin main
git worktree add .worktrees/iface-playlist-eval -b refactor/iface-playlist-eval origin/main
cd .worktrees/iface-playlist-eval
```

- [ ] **Step 3.2: Verify which methods are actually called**

```bash
grep -nE "store\.[A-Z]" internal/server/playlist_evaluator.go
```

Expected output (exactly two call sites):

```
128:		state, _ := store.GetUserBookState(userID, id)
263:		b, _ := store.GetBookByID(id)
```

`GetUserBookState` is on `UserPositionStore`. `GetBookByID` is on `BookReader`. No other methods are called — so the narrow interface is `UserPositionStore + BookReader`.

- [ ] **Step 3.3: Narrow the function signatures**

Edit `internal/server/playlist_evaluator.go`. Three function signatures change:

1. `EvaluateSmartPlaylist` (around line 63) — calls both `GetBookByID` (via `sortBookIDs`) and `GetUserBookState` (via `applyPerUserFilters`). Change parameter from `store database.Store` to an inline anonymous interface.
2. `applyPerUserFilters` (around line 117) — calls `GetUserBookState` only.
3. `sortBookIDs` (around line 245) — calls `GetBookByID` only.

Replace the three signatures with:

```go
// Type alias above these functions for readability:
type playlistEvalStore interface {
	database.BookReader
	database.UserPositionStore
}

func EvaluateSmartPlaylist(
	store playlistEvalStore,
	idx *search.BleveIndex,
	query string,
	sortJSON string,
	limit int,
	userID string,
) ([]string, error) {
	// body unchanged
}

func applyPerUserFilters(
	store database.UserPositionStore,
	ids []string,
	filters []search.PerUserFilter,
	userID string,
) []string {
	// body unchanged
}

func sortBookIDs(store database.BookReader, ids []string, sortJSON string) ([]string, error) {
	// body unchanged
}
```

The two helper functions get the even-narrower single interfaces because each only needs one. The public entry point takes the combined anonymous interface.

Bump the file's `// version:` header.

- [ ] **Step 3.4: Build + vet + test**

```bash
go build ./...
go vet ./...
go test ./internal/server/ -run TestProp_.*Playlist -count=1 -v
go test ./internal/server/ -run Playlist -count=1 -v
```

Expected: all green. Property tests from the previous session exercise `EvaluateSmartPlaylist` against random inputs — they pass unchanged because `*PebbleStore` satisfies `playlistEvalStore`.

- [ ] **Step 3.5: Commit + PR + merge**

```bash
git add internal/server/playlist_evaluator.go
git commit -m "$(cat <<'EOF'
refactor: narrow playlist_evaluator Store deps (ISP proof-point 1/3)

First of three proof-point migrations for the Store ISP refactor.
EvaluateSmartPlaylist, applyPerUserFilters, and sortBookIDs now
accept narrow interfaces instead of full database.Store:

- EvaluateSmartPlaylist: BookReader + UserPositionStore
- applyPerUserFilters: UserPositionStore only
- sortBookIDs: BookReader only

Demonstrates the inline-anonymous-interface pattern for multi-
domain consumers. Tests unchanged — they use real *PebbleStore
which satisfies every sub-interface.

Plan: docs/superpowers/plans/2026-04-17-store-interface-segregation.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git push -u origin refactor/iface-playlist-eval
gh pr create --title "refactor: narrow playlist_evaluator Store deps (ISP proof-point 1/3)" --body "First proof-point migration. Depends on PR #<task-1-pr>.

## Test plan
- [x] go build ./... clean
- [x] go vet ./... clean
- [x] Existing playlist_evaluator + prop tests green"
gh pr merge <num> --rebase --admin
```

---

## Task 4: Proof-point — migrate `audiobook_service.go`

**Goal:** Narrow the widest-surface service in the codebase. This is the hardest of the three proof-points; success proves the pattern scales.

**Files:**
- Modify: `internal/server/audiobook_service.go`
- **Unchanged:** the four audiobook_service_*_test.go files (they use real store fixtures)

- [ ] **Step 4.1: Branch**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git fetch origin main
git worktree add .worktrees/iface-audiobook-svc -b refactor/iface-audiobook-svc origin/main
cd .worktrees/iface-audiobook-svc
```

- [ ] **Step 4.2: Enumerate the actual method calls**

```bash
grep -nE "svc\.store\.[A-Z]" internal/server/audiobook_service.go | sort -u
```

The survey listed these interfaces needed: `BookStore` (reads+writes), `AuthorReader`, `AuthorWriter`, `SeriesReader`, `SeriesWriter`, `NarratorStore`, `StatsStore`, `HashBlocklistStore`, `TagStore`, `BookFileStore`. Verify by reading the grep output — every `svc.store.XYZ(` call should map to exactly one of those interfaces. If a method shows up that isn't covered, either the taxonomy is wrong (rare — add to an existing interface) or a method was missed in task 1 (fix task 1's interface files in this PR).

- [ ] **Step 4.3: Define the composite type at the top of the file**

Add just below the imports, above `type AudiobookService struct`:

```go
// audiobookStore is the narrow slice of database.Store that
// AudiobookService actually needs. Declared as a named composite so
// the service's dependencies are inspectable in one place rather
// than inlined into every method signature that forwards the store.
type audiobookStore interface {
	database.BookStore
	database.AuthorReader
	database.AuthorWriter
	database.SeriesReader
	database.SeriesWriter
	database.NarratorStore
	database.StatsStore
	database.HashBlocklistStore
	database.TagStore
	database.BookFileStore
}
```

- [ ] **Step 4.4: Narrow the struct field**

Change the existing `store database.Store` field (line 28) to:

```go
type AudiobookService struct {
	store           audiobookStore
	bookCache       *cache.Cache[*database.Book]
	listCache       *cache.Cache[[]database.Book]
	activityService *ActivityService
	searchIndex     *search.BleveIndex
}
```

- [ ] **Step 4.5: Narrow the constructor signature**

Change `func NewAudiobookService(store database.Store)` to:

```go
func NewAudiobookService(store audiobookStore) *AudiobookService {
	return &AudiobookService{
		store:     store,
		bookCache: cache.New[*database.Book](30 * time.Second),
		listCache: cache.New[[]database.Book](10 * time.Second),
	}
}
```

Bump the file's `// version:` header.

- [ ] **Step 4.6: Build + vet + test**

```bash
go build ./...
go vet ./...
go test ./internal/server/ -count=1 -v -run Audiobook 2>&1 | tail -30
go test ./... -count=1 -short 2>&1 | tail -20
```

Expected: all green. If anything fails, one of three things happened:
1. A method is called through `svc.store.XYZ` that you missed — add the owning interface to `audiobookStore`.
2. The struct is embedded in another type that also calls the store field directly — grep for `\.store\.` references and ensure they all fit.
3. A handler passes `svc.store` as a `database.Store` to a helper — narrow that helper's signature too, or use a type assertion at the call site.

- [ ] **Step 4.7: Commit + PR + merge**

```bash
git add internal/server/audiobook_service.go
git commit -m "$(cat <<'EOF'
refactor: narrow AudiobookService Store deps (ISP proof-point 2/3)

Second of three proof-point migrations. AudiobookService's store
field is now a named composite interface covering exactly the 10
sub-interfaces it uses: BookStore, AuthorReader/Writer, SeriesReader/
Writer, NarratorStore, StatsStore, HashBlocklistStore, TagStore,
BookFileStore.

Demonstrates the named-composite pattern for services with wider
(but still not full-Store) surfaces. Tests unchanged — *PebbleStore
satisfies the composite.

Plan: docs/superpowers/plans/2026-04-17-store-interface-segregation.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git push -u origin refactor/iface-audiobook-svc
gh pr create --title "refactor: narrow AudiobookService Store deps (ISP proof-point 2/3)" --body "Second proof-point. Depends on PR #<task-1-pr>.

## Test plan
- [x] go build ./... clean
- [x] go test ./internal/server/ green"
gh pr merge <num> --rebase --admin
```

---

## Task 5: Proof-point — migrate `reconcile.go`

**Goal:** Migrate a moderate-surface service whose shape (multi-domain read+write with OperationStore) is representative of the 58 files remaining.

**Files:**
- Modify: `internal/server/reconcile.go`

- [ ] **Step 5.1: Branch**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git fetch origin main
git worktree add .worktrees/iface-reconcile -b refactor/iface-reconcile origin/main
cd .worktrees/iface-reconcile
```

- [ ] **Step 5.2: Enumerate method calls**

```bash
grep -nE "store\.[A-Z]" internal/server/reconcile.go | sort -u | awk -F'[.(]' '{print $2}' | sort -u
```

Expected unique method list: `CreateOperation`, `CreateOperationChange`, `DeleteBook`, `GetAllBooks`, `GetAllImportPaths`, `GetBookByID`, `GetBookFiles`, `ListOperations`, `UpdateBook`, `UpdateOperationResultData`.

Mapping:
- `CreateOperation`, `CreateOperationChange`, `UpdateOperationResultData`, `ListOperations` → `OperationStore`
- `GetAllBooks`, `GetBookByID` → `BookReader`
- `DeleteBook`, `UpdateBook` → `BookWriter` (→ `BookStore` combined)
- `GetAllImportPaths` → `ImportPathStore`
- `GetBookFiles` → `BookFileStore`

- [ ] **Step 5.3: Identify the free-functions that take `store`**

`reconcile.go` is a file of free functions (not a struct-based service like `audiobook_service`). Each `func X(store database.Store, ...)` takes the store as a parameter. Grep:

```bash
grep -nE "^func .*store database\.Store" internal/server/reconcile.go
```

Narrow each signature individually to the minimum interface that function's body needs. The inline anonymous-interface pattern works cleanly here.

Example for a function that only reads books and writes operations:

```go
func reconcileBooks(
	store interface {
		database.BookStore
		database.OperationStore
	},
	/* other params */
) error { /* body unchanged */ }
```

Per function, identify the used methods via `grep -nE "store\.[A-Z]" <function-body>` and pick the narrowest composition.

- [ ] **Step 5.4: Cross-function composite for the most-used shape**

If most functions share the same 3–4 interfaces, define a file-local alias for readability (like `audiobookStore` in task 4):

```go
// reconcileStore is the wide shape used by most reconcile helpers.
type reconcileStore interface {
	database.BookStore
	database.BookFileStore
	database.ImportPathStore
	database.OperationStore
}
```

Functions that need only a subset (e.g., a helper that only reads books) still take the narrow type. Don't force every function through `reconcileStore` if it doesn't need it — that would defeat the point of the refactor.

Bump the file's `// version:` header.

- [ ] **Step 5.5: Build + vet + test**

```bash
go build ./...
go vet ./...
go test ./internal/server/ -count=1 -v -run Reconcile 2>&1 | tail -30
go test ./... -count=1 -short 2>&1 | tail -10
```

Expected: green.

- [ ] **Step 5.6: Commit + PR + merge**

```bash
git add internal/server/reconcile.go
git commit -m "$(cat <<'EOF'
refactor: narrow reconcile.go Store deps (ISP proof-point 3/3)

Third of three proof-point migrations. Reconcile helpers now take
narrow composites of BookStore, BookFileStore, ImportPathStore,
and OperationStore. Most functions take the narrowest subset they
use; a shared reconcileStore alias covers helpers that touch the
full set.

Closes the proof-point set — the pattern scales from a 2-interface
narrow (playlist_evaluator) through a 10-interface composite
(audiobook_service) to moderate multi-domain (reconcile).

Plan: docs/superpowers/plans/2026-04-17-store-interface-segregation.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git push -u origin refactor/iface-reconcile
gh pr create --title "refactor: narrow reconcile.go Store deps (ISP proof-point 3/3)" --body "Third and final proof-point. Depends on PR #<task-1-pr>.

## Test plan
- [x] go build ./... clean
- [x] go test ./internal/server/ green"
gh pr merge <num> --rebase --admin
```

---

## Task 6: Write the follow-on migration plan

**Goal:** Produce a second implementation plan under `docs/superpowers/plans/` listing the 58 remaining files, each with concrete target interfaces, commit scripts, and PR boilerplate. A follow-on agent dispatched later should be able to execute this plan end-to-end without asking the human to re-classify any file.

**Files:**
- Create: `docs/superpowers/plans/2026-04-17-store-iface-sweep.md`

- [ ] **Step 6.1: Branch**

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git fetch origin main
git worktree add .worktrees/iface-sweep-plan -b docs/iface-sweep-plan origin/main
cd .worktrees/iface-sweep-plan
```

- [ ] **Step 6.2: Generate the follow-on plan file**

The catalog already lives in `docs/superpowers/specs/2026-04-17-store-interface-segregation-design.md` section 6. The follow-on plan is that catalog converted into an executable task list.

Create `docs/superpowers/plans/2026-04-17-store-iface-sweep.md` with:

```markdown
# Store Interface Sweep — Follow-on Migration Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Foundation is already merged — this plan migrates the remaining 58 files one-by-one.

**Goal:** Migrate every non-proof-point consumer of `database.Store` to the narrow interface(s) listed in the spec's migration catalog. Remove the unused `Store` field from the 18 "noop" consumers.

**Prerequisites:**
- Foundation PR (task 1 of the ISP plan) merged — sub-interfaces defined.
- Mockery PR (task 2) merged — per-interface mocks available for test migrations.
- Proof-point PRs (3, 4, 5) merged — pattern validated on three representative services.

## Execution model

Each row of the migration catalog in the spec is one PR. Dispatch this plan via superpowers:subagent-driven-development — the agent reads the catalog, picks an unclaimed file, migrates it, opens a PR, merges, moves on. Files with the same target interface can be grouped into a single PR if they're in the same package (≤ 5 files per PR keeps diffs reviewable).

## Per-file workflow (template — apply to every eligible file)

1. `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && git fetch origin main`
2. `git worktree add .worktrees/iface-sweep-<slug> -b refactor/iface-sweep-<slug> origin/main`
3. `cd .worktrees/iface-sweep-<slug>`
4. Read the row in the spec's catalog for `<file>`. Note: Class (read-only / write-only / read-write / test / noop), Interfaces (the target list), Notes.
5. `grep -nE "store\.[A-Z]" <file>` (or the relevant variable name) and sanity-check against the target list. If a method call doesn't map to one of the target interfaces, either add that interface to the list (common: the spec missed a method) or flag the file as needing human review.
6. Narrow the struct field or parameter signatures using the pattern chosen in proof-point task 3/4/5 that matches the file's shape:
   - Free functions → inline anonymous interfaces per function, as in `playlist_evaluator.go`.
   - Struct with multi-domain store → named composite type at top of file, as in `audiobook_service.go`.
   - Shared shape across many functions in one file → file-local alias, as in `reconcile.go`.
7. Bump the file's `// version:` header.
8. `go build ./... && go vet ./...`
9. `go test ./<package>/ -count=1 -v 2>&1 | tail -20`
10. Commit with prefix `refactor:` and body explaining which interfaces replaced `database.Store`.
11. `git push -u origin refactor/iface-sweep-<slug> && gh pr create ... && gh pr merge <n> --rebase --admin`

## Special cases (read first)

- **`internal/server/indexed_store.go`** — wraps `Store` and forwards book-CRUD through a bleve-indexed layer. The wrapper's field type must be wide enough to forward every method it defines. Narrow both the struct field AND the forwarded method set to the same `BookStore` composite. If the wrapper also forwards methods outside `BookStore`, add those interfaces to its field's type.
- **`internal/logger/operation.go`** — defines a *local* `OperationStore` interface for log injection. Rename the local interface (e.g. to `logOpStore`) to avoid collision with the new `database.OperationStore`. Update call sites.
- **The 18 noop consumers** (field-but-no-calls) — these get a different treatment: delete the unused field entirely, update the constructor to drop the parameter, fix callers. Do this **after** all legitimate migrations finish, as one bundled cleanup PR.
- **`internal/server/server.go`** — the server bootstrap legitimately needs full `Store` access. Leave as-is.

## Migration table

(Copy the full per-package table verbatim from spec section 6. Every row is one migration unit.)

## Verification per batch

After every 5 PRs merged, run the full test suite on main:

```bash
git checkout main && git pull
go test ./... -count=1
```

Expected: always green. If a regression appears, the last PR's narrowing was incorrect — revert via `gh pr revert <n>` and re-classify.

## Definition of done

- All 58 eligible files migrated (58 → 0 in the catalog's remaining column).
- 18 noop consumers cleaned up via the bundled cleanup PR.
- `grep -rn "database\.Store\b" internal/ cmd/ | wc -l` drops from 79 to ~12 (just the legitimate wide-access consumers: `server.go`, mocks, tests that intentionally use full fixtures).
- Full test suite green.
- `mocks-check` CI gate green.
```

(The final plan file should include the full per-package table from the spec. It's identical content — duplicating it here would double this plan's size for no benefit. The agent executing task 6 reads the spec section 6 and pastes it into the new plan under "Migration table".)

- [ ] **Step 6.3: Verify the plan file is complete**

```bash
wc -l docs/superpowers/plans/2026-04-17-store-iface-sweep.md
grep -c "^|" docs/superpowers/plans/2026-04-17-store-iface-sweep.md
```

Expected: file > 300 lines, table row count matches the spec's catalog (≥ 79 rows counting headers).

- [ ] **Step 6.4: Commit + PR + merge**

```bash
git add docs/superpowers/plans/2026-04-17-store-iface-sweep.md
git commit -m "$(cat <<'EOF'
docs: follow-on migration plan for remaining 58 Store consumers

Turns the spec's migration catalog into an executable per-PR task
list. A follow-on agent dispatched via
superpowers:subagent-driven-development can work this plan end-to-
end without re-classifying files. Includes special-case handling
for indexed_store wrapper, logger interface collision, and the 18
noop consumers.

Companion to docs/superpowers/plans/2026-04-17-store-interface-
segregation.md (tasks 1-5 of the main plan).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
git push -u origin docs/iface-sweep-plan
gh pr create --title "docs: follow-on migration plan for remaining 58 Store consumers" --body "Executable task list derived from the ISP spec's catalog. Ready to hand off to a follow-on agent."
gh pr merge <num> --rebase --admin
```

---

## Self-review (for the plan writer)

- **Spec coverage.** Every spec section maps to a task: §3.1 → Task 1 steps 1.2–1.5; §3.2 → Task 1 steps 1.6–1.9; §3.3 file layout → Task 1 step 1.10; §3.4 composition → Task 4 step 4.3 (named composite), Task 5 step 5.3 (inline anonymous); §3.5 mocks → Task 2; §4 proof-points → Tasks 3/4/5; §6 catalog → Task 6.
- **No placeholders.** Every step names an exact file or command. Code blocks have full content, not "..." elisions. Commit messages are complete.
- **Type consistency.** `audiobookStore`, `reconcileStore`, `playlistEvalStore` names match across plan sections. Every sub-interface referenced in proof-point tasks is defined in task 1.
- **Gotchas called out.** Duplicate-method compile errors (step 1.11), method-miss during proof-point (step 4.6), `indexed_store.go` wrapper (task 6 special cases), logger collision (task 6 special cases).

## Success criteria (overall)

- PRs 1–6 merged to main.
- `go test ./... -count=1` green after every merge.
- `make mocks-check` green on main.
- 3 proof-point services migrated; pattern documented in the follow-on plan.
- Follow-on plan complete enough for a cold agent to execute without human clarification.
