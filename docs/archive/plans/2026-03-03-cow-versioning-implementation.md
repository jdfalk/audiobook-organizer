# Copy-on-Write Versioning + Path Format + Smart Apply Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add copy-on-write book versioning to PebbleDB, a unified path format template engine, and a smart metadata apply pipeline that generates segment titles, renames files, writes tags, and verifies.

**Architecture:** UpdateBook atomically appends immutable `book_ver:{id}:{ts}` entries alongside the current `book:{id}`. A format engine resolves `{author}/{title}/{track_title}.{ext}` templates using book+segment data. The metadata apply pipeline chains: snapshot → apply → generate titles → rename → write tags → verify → record.

**Tech Stack:** Go 1.24, PebbleDB, React/TypeScript/MUI frontend

---

## Phase 1: Copy-on-Write Book Versioning

### Task 1: Add BookVersion type and Store interface methods

**Files:**
- Modify: `internal/database/store.go`

**Step 1: Add BookVersion struct and interface methods**

Add after the existing `MetadataChangeRecord` struct (around line 537):

```go
// BookVersion represents an immutable snapshot of a book at a point in time.
type BookVersion struct {
	BookID    string    `json:"book_id"`
	Timestamp time.Time `json:"timestamp"`
	Data      []byte    `json:"data"` // Full JSON-serialized Book
}
```

Add to Store interface after `UpdateBook` (around line 77):

```go
	// Book Version History (copy-on-write)
	GetBookVersions(id string, limit int) ([]BookVersion, error)
	GetBookAtVersion(id string, ts time.Time) (*Book, error)
	RevertBookToVersion(id string, ts time.Time) (*Book, error)
	PruneBookVersions(id string, keepCount int) (int, error)
```

Bump version header.

**Step 2: Verify compilation fails (methods not implemented)**

Run: `go build ./internal/database/...`
Expected: FAIL — PebbleStore and SQLiteStore don't implement new methods.

**Step 3: Commit**

```bash
git add internal/database/store.go
git commit -m "feat: add BookVersion type and Store interface methods for CoW versioning"
```

---

### Task 2: Implement CoW in PebbleStore.UpdateBook

**Files:**
- Modify: `internal/database/pebble_store.go`
- Test: `internal/database/pebble_store_test.go`

**Step 1: Write failing test for version creation on update**

Add to `pebble_store_test.go`:

```go
func TestPebbleUpdateBookCreatesVersion(t *testing.T) {
	store := setupTestPebbleStore(t)
	defer store.Close()

	book := &database.Book{Title: "Original Title", FilePath: "/test/book.m4b"}
	created, err := store.CreateBook(book)
	require.NoError(t, err)

	// Update the book
	created.Title = "Updated Title"
	updated, err := store.UpdateBook(created.ID, created)
	require.NoError(t, err)
	require.Equal(t, "Updated Title", updated.Title)

	// Should have exactly one version (the pre-update snapshot)
	versions, err := store.GetBookVersions(created.ID, 10)
	require.NoError(t, err)
	require.Len(t, versions, 1)

	// Version should contain the OLD state
	var oldBook database.Book
	err = json.Unmarshal(versions[0].Data, &oldBook)
	require.NoError(t, err)
	require.Equal(t, "Original Title", oldBook.Title)
}
```

**Step 2: Run test, verify it fails**

Run: `go test ./internal/database/ -run TestPebbleUpdateBookCreatesVersion -v`
Expected: FAIL — GetBookVersions not implemented.

**Step 3: Modify UpdateBook to append version**

In `pebble_store.go`, modify `UpdateBook()` (line 1078). Before the existing batch operations, add version snapshot of the OLD book:

```go
func (p *PebbleStore) UpdateBook(id string, book *Book) (*Book, error) {
	oldBook, err := p.GetBookByID(id)
	if err != nil {
		return nil, err
	}
	if oldBook == nil {
		return nil, fmt.Errorf("book not found")
	}

	// --- CoW: snapshot old state before mutation ---
	oldData, err := json.Marshal(oldBook)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal old book for version: %w", err)
	}
	versionKey := []byte(fmt.Sprintf("book_ver:%s:%d", id, time.Now().UnixNano()))
	// --- end CoW preamble ---

	book.ID = id
	if oldBook.CreatedAt != nil {
		book.CreatedAt = oldBook.CreatedAt
	}
	now := time.Now()
	book.UpdatedAt = &now

	data, err := json.Marshal(book)
	if err != nil {
		return nil, err
	}

	batch := p.db.NewBatch()

	// CoW: write version snapshot
	if err := batch.Set(versionKey, oldData, nil); err != nil {
		batch.Close()
		return nil, err
	}

	// Update main key (rest of existing code unchanged)
	key := []byte(fmt.Sprintf("book:%s", id))
	if err := batch.Set(key, data, nil); err != nil {
		batch.Close()
		return nil, err
	}
	// ... rest of existing index update code stays the same ...
```

**Step 4: Implement GetBookVersions**

Add to `pebble_store.go`:

```go
func (p *PebbleStore) GetBookVersions(id string, limit int) ([]BookVersion, error) {
	if limit <= 0 {
		limit = 50
	}
	prefix := fmt.Sprintf("book_ver:%s:", id)
	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(prefix),
		UpperBound: []byte(prefix + "\xff"),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var versions []BookVersion
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		// Parse timestamp from key: book_ver:{id}:{nanoTimestamp}
		parts := strings.SplitN(key, ":", 3)
		if len(parts) != 3 {
			continue
		}
		nsec, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			continue
		}
		dataCopy := make([]byte, len(iter.Value()))
		copy(dataCopy, iter.Value())
		versions = append(versions, BookVersion{
			BookID:    id,
			Timestamp: time.Unix(0, nsec),
			Data:      dataCopy,
		})
	}
	// Reverse for newest-first
	for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
		versions[i], versions[j] = versions[j], versions[i]
	}
	if len(versions) > limit {
		versions = versions[:limit]
	}
	return versions, nil
}
```

**Step 5: Run test, verify it passes**

Run: `go test ./internal/database/ -run TestPebbleUpdateBookCreatesVersion -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/database/pebble_store.go internal/database/pebble_store_test.go
git commit -m "feat: CoW versioning - UpdateBook appends immutable version on every write"
```

---

### Task 3: Implement GetBookAtVersion, RevertBookToVersion, PruneBookVersions

**Files:**
- Modify: `internal/database/pebble_store.go`
- Test: `internal/database/pebble_store_test.go`

**Step 1: Write failing tests**

```go
func TestPebbleGetBookAtVersion(t *testing.T) {
	store := setupTestPebbleStore(t)
	defer store.Close()

	book := &database.Book{Title: "V1", FilePath: "/test/v1.m4b"}
	created, err := store.CreateBook(book)
	require.NoError(t, err)

	// Update twice
	created.Title = "V2"
	store.UpdateBook(created.ID, created)
	time.Sleep(time.Millisecond) // ensure different timestamps
	created.Title = "V3"
	store.UpdateBook(created.ID, created)

	versions, err := store.GetBookVersions(created.ID, 10)
	require.NoError(t, err)
	require.Len(t, versions, 2)

	// Get the first version (oldest = V1)
	oldBook, err := store.GetBookAtVersion(created.ID, versions[1].Timestamp)
	require.NoError(t, err)
	require.Equal(t, "V1", oldBook.Title)
}

func TestPebbleRevertBookToVersion(t *testing.T) {
	store := setupTestPebbleStore(t)
	defer store.Close()

	book := &database.Book{Title: "Original", FilePath: "/test/orig.m4b"}
	created, err := store.CreateBook(book)
	require.NoError(t, err)

	created.Title = "Modified"
	store.UpdateBook(created.ID, created)

	versions, err := store.GetBookVersions(created.ID, 10)
	require.NoError(t, err)
	require.Len(t, versions, 1)

	// Revert to original
	reverted, err := store.RevertBookToVersion(created.ID, versions[0].Timestamp)
	require.NoError(t, err)
	require.Equal(t, "Original", reverted.Title)

	// Current book should now be "Original"
	current, err := store.GetBookByID(created.ID)
	require.NoError(t, err)
	require.Equal(t, "Original", current.Title)

	// Revert itself should create a new version (of the "Modified" state)
	versions2, err := store.GetBookVersions(created.ID, 10)
	require.NoError(t, err)
	require.Len(t, versions2, 3) // original snapshot + revert snapshot + revert creates version
}

func TestPebblePruneBookVersions(t *testing.T) {
	store := setupTestPebbleStore(t)
	defer store.Close()

	book := &database.Book{Title: "V1", FilePath: "/test/prune.m4b"}
	created, err := store.CreateBook(book)
	require.NoError(t, err)

	for i := 2; i <= 6; i++ {
		created.Title = fmt.Sprintf("V%d", i)
		store.UpdateBook(created.ID, created)
		time.Sleep(time.Millisecond)
	}

	versions, _ := store.GetBookVersions(created.ID, 100)
	require.Len(t, versions, 5) // 5 updates = 5 versions

	pruned, err := store.PruneBookVersions(created.ID, 2)
	require.NoError(t, err)
	require.Equal(t, 3, pruned) // removed 3, kept 2

	remaining, _ := store.GetBookVersions(created.ID, 100)
	require.Len(t, remaining, 2)
}
```

**Step 2: Run tests, verify they fail**

Run: `go test ./internal/database/ -run "TestPebbleGetBookAtVersion|TestPebbleRevertBookToVersion|TestPebblePruneBookVersions" -v`
Expected: FAIL

**Step 3: Implement the three methods**

```go
func (p *PebbleStore) GetBookAtVersion(id string, ts time.Time) (*Book, error) {
	key := []byte(fmt.Sprintf("book_ver:%s:%d", id, ts.UnixNano()))
	value, closer, err := p.db.Get(key)
	if err == pebble.ErrNotFound {
		return nil, fmt.Errorf("version not found")
	}
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var book Book
	if err := json.Unmarshal(value, &book); err != nil {
		return nil, err
	}
	return &book, nil
}

func (p *PebbleStore) RevertBookToVersion(id string, ts time.Time) (*Book, error) {
	oldBook, err := p.GetBookAtVersion(id, ts)
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	// UpdateBook will automatically create a CoW version of current state
	oldBook.ID = id
	return p.UpdateBook(id, oldBook)
}

func (p *PebbleStore) PruneBookVersions(id string, keepCount int) (int, error) {
	if keepCount < 0 {
		keepCount = 0
	}
	versions, err := p.GetBookVersions(id, 0) // 0 = no limit internally, get all
	if err != nil {
		return 0, err
	}
	// versions are newest-first; keep the first keepCount, delete the rest
	if len(versions) <= keepCount {
		return 0, nil
	}
	toDelete := versions[keepCount:]
	batch := p.db.NewBatch()
	for _, v := range toDelete {
		key := []byte(fmt.Sprintf("book_ver:%s:%d", id, v.Timestamp.UnixNano()))
		if err := batch.Delete(key, nil); err != nil {
			batch.Close()
			return 0, err
		}
	}
	if err := batch.Commit(pebble.Sync); err != nil {
		return 0, err
	}
	return len(toDelete), nil
}
```

Note: Fix `GetBookVersions` to allow limit=0 meaning "all" — change the guard:
```go
// In GetBookVersions, after collecting all versions:
if limit > 0 && len(versions) > limit {
    versions = versions[:limit]
}
```

**Step 4: Run tests, verify they pass**

Run: `go test ./internal/database/ -run "TestPebbleGetBookAtVersion|TestPebbleRevertBookToVersion|TestPebblePruneBookVersions" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/database/pebble_store.go internal/database/pebble_store_test.go
git commit -m "feat: CoW versioning - GetBookAtVersion, RevertBookToVersion, PruneBookVersions"
```

---

### Task 4: Implement CoW methods for SQLiteStore and MockStore

**Files:**
- Modify: `internal/database/sqlite_store.go`
- Modify: `internal/database/mock_store.go`
- Modify: `internal/database/mocks/mock_store.go`

**Step 1: Add SQLiteStore stubs (no-op for now, SQLite is opt-in)**

```go
func (s *SQLiteStore) GetBookVersions(id string, limit int) ([]BookVersion, error) {
	return nil, nil // SQLite CoW not yet implemented
}

func (s *SQLiteStore) GetBookAtVersion(id string, ts time.Time) (*Book, error) {
	return nil, fmt.Errorf("book versioning not supported in SQLite store")
}

func (s *SQLiteStore) RevertBookToVersion(id string, ts time.Time) (*Book, error) {
	return nil, fmt.Errorf("book versioning not supported in SQLite store")
}

func (s *SQLiteStore) PruneBookVersions(id string, keepCount int) (int, error) {
	return 0, nil
}
```

**Step 2: Add MockStore methods**

In `internal/database/mock_store.go`, add simple map-backed implementations that satisfy the interface.

**Step 3: Regenerate testify mocks**

Run: `cd internal/database/mocks && go generate ./...` (or manually add the 4 methods matching the pattern of existing mock methods).

**Step 4: Verify full build**

Run: `go build ./...`
Expected: PASS — all Store implementations satisfy the interface.

**Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/database/sqlite_store.go internal/database/mock_store.go internal/database/mocks/mock_store.go
git commit -m "feat: CoW versioning - SQLite stubs and mock implementations"
```

---

### Task 5: Remove saveBookSnapshot hack from metadata_fetch_service.go

**Files:**
- Modify: `internal/server/metadata_fetch_service.go`

**Step 1: Remove the saveBookSnapshot method and all calls to it**

- Delete the `saveBookSnapshot` method (around line 471-489)
- Delete the `recordSearchEvent` method (not needed as separate method — keep the search event recording inline in SearchMetadataForBook)
- Delete the `RevertBookToSnapshot` method (replaced by DB-layer `RevertBookToVersion`)
- Remove calls to `saveBookSnapshot` in `ApplyMetadataCandidate` and `FetchMetadataForBook`
- The `__snapshot__` metadata change records are no longer written — CoW handles this natively

Keep the `recordChangeHistory` fix (called BEFORE apply) and the `persistFetchedMetadata` call in `ApplyMetadataCandidate` — those are correct fixes.

**Step 2: Remove revertAudiobookMetadata handler from server.go**

Replace it with a new handler that uses `store.RevertBookToVersion`:

```go
func (s *Server) revertAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")
	if database.GlobalStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database not initialized"})
		return
	}
	var body struct {
		Timestamp string `json:"timestamp"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Timestamp == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "timestamp is required (RFC3339Nano)"})
		return
	}
	ts, err := time.Parse(time.RFC3339Nano, body.Timestamp)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid timestamp format"})
		return
	}
	book, err := database.GlobalStore.RevertBookToVersion(id, ts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Book reverted", "book": book})
}
```

**Step 3: Add version history endpoint**

```go
// GET /api/v1/audiobooks/:id/versions-history
func (s *Server) getAudiobookVersionHistory(c *gin.Context) {
	id := c.Param("id")
	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	versions, err := database.GlobalStore.GetBookVersions(id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Return versions with parsed book data for display
	type versionResponse struct {
		Timestamp string `json:"timestamp"`
		Title     string `json:"title"`
		Author    string `json:"author_name"`
	}
	var resp []versionResponse
	for _, v := range versions {
		var b database.Book
		if err := json.Unmarshal(v.Data, &b); err != nil {
			continue
		}
		resp = append(resp, versionResponse{
			Timestamp: v.Timestamp.Format(time.RFC3339Nano),
			Title:     b.Title,
		})
	}
	c.JSON(http.StatusOK, gin.H{"versions": resp})
}
```

Register route: `protected.GET("/audiobooks/:id/versions-history", s.getAudiobookVersionHistory)`

**Step 4: Verify build and tests**

Run: `go build ./... && go test ./internal/server/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/server/metadata_fetch_service.go internal/server/server.go
git commit -m "refactor: replace snapshot hack with native CoW versioning from DB layer"
```

---

## Phase 2: Path Format Template Engine

### Task 6: Create path format engine with tests

**Files:**
- Create: `internal/server/path_format.go`
- Create: `internal/server/path_format_test.go`

**Step 1: Write failing tests**

```go
package server

import (
	"testing"
	"github.com/stretchr/testify/require"
)

func TestFormatPath_BasicTemplate(t *testing.T) {
	vars := FormatVars{
		Author:      "James S.A. Corey",
		Title:       "Leviathan Falls",
		Series:      "The Expanse",
		SeriesPos:   "9",
		Track:       1,
		TotalTracks: 51,
		Ext:         "mp3",
		Lang:        "en",
	}
	result := FormatPath("{author}/{series_prefix}{title}/{track_title}.{ext}", vars)
	require.Equal(t, "James S.A. Corey/The Expanse 9 - Leviathan Falls/Leviathan Falls - 1_51.mp3", result)
}

func TestFormatSegmentTitle(t *testing.T) {
	tests := []struct {
		format   string
		title    string
		track    int
		total    int
		expected string
	}{
		{"{title} - {track}/{total_tracks}", "Leviathan Falls", 15, 51, "Leviathan Falls - 15/51"},
		{"{title} - {track} of {total_tracks}", "Leviathan Falls", 15, 51, "Leviathan Falls - 15 of 51"},
		{"{title} - Part {track}", "Leviathan Falls", 15, 51, "Leviathan Falls - Part 15"},
		{"{track:02d} - {title}", "Leviathan Falls", 3, 51, "03 - Leviathan Falls"},
	}
	for _, tt := range tests {
		result := FormatSegmentTitle(tt.format, tt.title, tt.track, tt.total)
		require.Equal(t, tt.expected, result, "format: %s", tt.format)
	}
}

func TestFormatPath_EmptyVariablesCollapse(t *testing.T) {
	vars := FormatVars{
		Author: "Author",
		Title:  "Title",
		Ext:    "m4b",
		// No series, no lang
	}
	result := FormatPath("{author}/{series_prefix}{title}.{lang}.{ext}", vars)
	require.Equal(t, "Author/Title.m4b", result)
}

func TestFormatPath_LanguageSuffix(t *testing.T) {
	vars := FormatVars{
		Author: "Author",
		Title:  "Title",
		Lang:   "de",
		Ext:    "m4b",
	}
	result := FormatPath("{author}/{title}.{lang}.{ext}", vars)
	require.Equal(t, "Author/Title.de.m4b", result)
}

func TestSanitizePathComponent(t *testing.T) {
	require.Equal(t, "James S.A. Corey", sanitizePathComponent("James S.A. Corey"))
	require.Equal(t, "Title - Subtitle", sanitizePathComponent("Title: Subtitle"))
	require.Equal(t, "No Slash Here", sanitizePathComponent("No/Slash/Here"))
}
```

**Step 2: Run tests, verify they fail**

Run: `go test ./internal/server/ -run "TestFormat" -v`
Expected: FAIL — FormatPath not defined.

**Step 3: Implement the format engine**

```go
// file: internal/server/path_format.go

package server

import (
	"fmt"
	"regexp"
	"strings"
)

// FormatVars holds all variables available for path/title formatting.
type FormatVars struct {
	Author      string
	Title       string
	Series      string
	SeriesPos   string
	Year        int
	Narrator    string
	Lang        string // ISO 639-1
	Track       int
	TotalTracks int
	TrackTitle  string // pre-computed segment title
	Ext         string
}

var formatVarPattern = regexp.MustCompile(`\{(\w+)(?::([^}]+))?\}`)

// FormatSegmentTitle formats a segment title using the segment_title_format template.
func FormatSegmentTitle(format string, title string, track, totalTracks int) string {
	result := format
	result = strings.ReplaceAll(result, "{title}", title)
	result = strings.ReplaceAll(result, "{total_tracks}", fmt.Sprintf("%d", totalTracks))

	// Handle {track} with optional format spec like {track:02d}
	result = formatVarPattern.ReplaceAllStringFunc(result, func(match string) string {
		parts := formatVarPattern.FindStringSubmatch(match)
		name := parts[1]
		spec := parts[2]
		if name == "track" {
			if spec != "" {
				return fmt.Sprintf("%"+spec, track)
			}
			return fmt.Sprintf("%d", track)
		}
		return match
	})
	return result
}

// FormatPath formats a full file path using the path_format template.
func FormatPath(format string, vars FormatVars) string {
	// Compute segment title if track info present
	trackTitle := vars.TrackTitle
	if trackTitle == "" && vars.Track > 0 {
		trackTitle = FormatSegmentTitle(
			DefaultSegmentTitleFormat,
			vars.Title, vars.Track, vars.TotalTracks,
		)
	}

	// Compute series_prefix
	seriesPrefix := ""
	if vars.Series != "" {
		seriesPrefix = vars.Series
		if vars.SeriesPos != "" {
			seriesPrefix += " " + vars.SeriesPos
		}
		seriesPrefix += " - "
	}

	// Replace all variables
	result := format
	result = strings.ReplaceAll(result, "{author}", vars.Author)
	result = strings.ReplaceAll(result, "{title}", vars.Title)
	result = strings.ReplaceAll(result, "{series}", vars.Series)
	result = strings.ReplaceAll(result, "{series_position}", vars.SeriesPos)
	result = strings.ReplaceAll(result, "{series_prefix}", seriesPrefix)
	result = strings.ReplaceAll(result, "{year}", func() string {
		if vars.Year > 0 { return fmt.Sprintf("%d", vars.Year) }
		return ""
	}())
	result = strings.ReplaceAll(result, "{narrator}", vars.Narrator)
	result = strings.ReplaceAll(result, "{lang}", vars.Lang)
	result = strings.ReplaceAll(result, "{track_title}", trackTitle)
	result = strings.ReplaceAll(result, "{ext}", vars.Ext)

	// Handle {track} with optional format spec
	result = formatVarPattern.ReplaceAllStringFunc(result, func(match string) string {
		parts := formatVarPattern.FindStringSubmatch(match)
		name := parts[1]
		spec := parts[2]
		if name == "track" {
			if spec != "" {
				return fmt.Sprintf("%"+spec, vars.Track)
			}
			return fmt.Sprintf("%d", vars.Track)
		}
		if name == "total_tracks" {
			return fmt.Sprintf("%d", vars.TotalTracks)
		}
		return match
	})

	// Collapse empty segments:
	// - Remove consecutive dots (Title..mp3 → Title.mp3)
	// - Remove empty path segments (Author//Title → Author/Title)
	// - Remove leading/trailing separators in segments
	result = collapseEmptySegments(result)

	// Sanitize each path component
	parts := strings.Split(result, "/")
	for i, part := range parts {
		// Don't sanitize the extension part of the last component
		parts[i] = sanitizePathComponent(part)
	}
	return strings.Join(parts, "/")
}

// collapseEmptySegments cleans up paths with empty variable substitutions.
func collapseEmptySegments(path string) string {
	// Remove double dots (from empty {lang}: "title..ext" → "title.ext")
	for strings.Contains(path, "..") {
		path = strings.ReplaceAll(path, "..", ".")
	}
	// Remove double slashes
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	// Remove trailing dots before slashes ("title./ext" shouldn't happen but safety)
	path = strings.ReplaceAll(path, "./", "/")
	// Remove leading dots after slashes
	path = strings.ReplaceAll(path, "/.", "/")
	// Trim leading/trailing slashes and dots
	path = strings.Trim(path, "/.")
	return path
}

// sanitizePathComponent removes filesystem-unsafe characters from a single path component.
func sanitizePathComponent(s string) string {
	// Replace characters illegal in most filesystems
	replacer := strings.NewReplacer(
		"/", " ",
		"\\", " ",
		":", " -",
		"*", "",
		"?", "",
		"\"", "'",
		"<", "",
		">", "",
		"|", " -",
	)
	s = replacer.Replace(s)
	// Collapse multiple spaces
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

const (
	DefaultPathFormat          = "{author}/{series_prefix}{title}/{track_title}.{ext}"
	DefaultSegmentTitleFormat  = "{title} - {track}/{total_tracks}"
)
```

**Step 4: Run tests, verify they pass**

Run: `go test ./internal/server/ -run "TestFormat|TestSanitize" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/server/path_format.go internal/server/path_format_test.go
git commit -m "feat: path format template engine with variable substitution and collapse"
```

---

### Task 7: Add path_format and segment_title_format to config

**Files:**
- Modify: `internal/config/config.go`

**Step 1: Add fields to Config struct**

```go
	PathFormat          string `json:"path_format"`
	SegmentTitleFormat  string `json:"segment_title_format"`
	AutoRenameOnApply   bool   `json:"auto_rename_on_apply"`
	AutoWriteTagsOnApply bool  `json:"auto_write_tags_on_apply"`
	VerifyAfterWrite    bool   `json:"verify_after_write"`
```

**Step 2: Set defaults**

In `LoadConfig()`:
```go
	viper.SetDefault("path_format", "{author}/{series_prefix}{title}/{track_title}.{ext}")
	viper.SetDefault("segment_title_format", "{title} - {track}/{total_tracks}")
	viper.SetDefault("auto_rename_on_apply", true)
	viper.SetDefault("auto_write_tags_on_apply", true)
	viper.SetDefault("verify_after_write", true)
```

**Step 3: Wire into config loading**

```go
	AppConfig.PathFormat = viper.GetString("path_format")
	AppConfig.SegmentTitleFormat = viper.GetString("segment_title_format")
	AppConfig.AutoRenameOnApply = viper.GetBool("auto_rename_on_apply")
	AppConfig.AutoWriteTagsOnApply = viper.GetBool("auto_write_tags_on_apply")
	AppConfig.VerifyAfterWrite = viper.GetBool("verify_after_write")
```

**Step 4: Verify build and config tests**

Run: `go build ./... && go test ./internal/config/...`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/config.go
git commit -m "feat: add path_format, segment_title_format, and apply pipeline config flags"
```

---

## Phase 3: Smart Apply Pipeline (Steps 3-7)

### Task 8: Generate segment titles on metadata apply

**Files:**
- Modify: `internal/server/metadata_fetch_service.go`

**Step 1: Add generateSegmentTitles method**

After metadata apply, compute and update segment titles based on the template:

```go
func (mfs *MetadataFetchService) generateSegmentTitles(bookID string, bookTitle string) error {
	segments, err := mfs.db.ListBookSegments(bookNumericID(bookID))
	if err != nil || len(segments) == 0 {
		return nil // no segments = single-file book, skip
	}

	titleFormat := config.AppConfig.SegmentTitleFormat
	if titleFormat == "" {
		titleFormat = DefaultSegmentTitleFormat
	}

	// Sort segments by track number, then filepath
	sort.Slice(segments, func(i, j int) bool {
		ti, tj := 0, 0
		if segments[i].TrackNumber != nil { ti = *segments[i].TrackNumber }
		if segments[j].TrackNumber != nil { tj = *segments[j].TrackNumber }
		if ti != tj { return ti < tj }
		return segments[i].FilePath < segments[j].FilePath
	})

	// Auto-assign track numbers if missing
	for i := range segments {
		if segments[i].TrackNumber == nil {
			n := i + 1
			segments[i].TrackNumber = &n
		}
	}
	total := len(segments)
	for i := range segments {
		segments[i].TotalTracks = &total
	}

	// Generate titles and update segments
	for _, seg := range segments {
		track := 0
		if seg.TrackNumber != nil { track = *seg.TrackNumber }
		newTitle := FormatSegmentTitle(titleFormat, bookTitle, track, total)
		// Store the computed title - we'll use it for write-back
		seg.SegmentTitle = &newTitle
		if err := mfs.db.UpdateBookSegment(&seg); err != nil {
			log.Printf("[WARN] failed to update segment title for %s: %v", seg.ID, err)
		}
	}
	return nil
}
```

Note: This requires adding `SegmentTitle *string` to the `BookSegment` struct in `store.go`.

**Step 2: Call from ApplyMetadataCandidate after the book is updated**

Add after `persistFetchedMetadata`:
```go
	// Generate segment titles from template
	if err := mfs.generateSegmentTitles(id, updatedBook.Title); err != nil {
		log.Printf("[WARN] failed to generate segment titles for %s: %v", id, err)
	}
```

**Step 3: Verify build and tests**

Run: `go build ./... && go test ./internal/server/...`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/database/store.go internal/server/metadata_fetch_service.go
git commit -m "feat: generate segment titles from template on metadata apply"
```

---

### Task 9: File rename pipeline step

**Files:**
- Create: `internal/server/file_pipeline.go`
- Create: `internal/server/file_pipeline_test.go`

This task implements the rename step. Given a book and its segments, compute the target paths from the path_format template and rename files.

**Step 1: Write test for ComputeTargetPaths**

```go
func TestComputeTargetPaths(t *testing.T) {
	book := &database.Book{
		ID:       "test-id",
		Title:    "Leviathan Falls",
		FilePath: "/library/old-path",
	}
	segments := []database.BookSegment{
		{ID: "s1", FilePath: "/library/old-path/01.mp3", TrackNumber: intPtr(1), TotalTracks: intPtr(3)},
		{ID: "s2", FilePath: "/library/old-path/02.mp3", TrackNumber: intPtr(2), TotalTracks: intPtr(3)},
		{ID: "s3", FilePath: "/library/old-path/03.mp3", TrackNumber: intPtr(3), TotalTracks: intPtr(3)},
	}
	vars := FormatVars{
		Author: "James S.A. Corey",
		Title:  "Leviathan Falls",
		Series: "The Expanse",
		SeriesPos: "9",
		Ext: "mp3",
	}

	paths := ComputeTargetPaths("/library", "{author}/{series_prefix}{title}/{track_title}.{ext}",
		"{title} - {track}/{total_tracks}", book, segments, vars)

	require.Len(t, paths, 3)
	require.Contains(t, paths[0].TargetPath, "James S.A. Corey/The Expanse 9 - Leviathan Falls/")
	require.Contains(t, paths[0].TargetPath, "Leviathan Falls - 1/3.mp3")
}
```

**Step 2: Implement ComputeTargetPaths and RenameFiles**

The implementation computes target paths, creates directories, and does atomic renames (rename to temp, verify, rename to final). Records old→new path mappings for rollback.

**Step 3: Run tests**

Run: `go test ./internal/server/ -run TestComputeTargetPaths -v`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/server/file_pipeline.go internal/server/file_pipeline_test.go
git commit -m "feat: file rename pipeline - compute target paths and atomic rename"
```

---

### Task 10: Wire pipeline into ApplyMetadataCandidate

**Files:**
- Modify: `internal/server/metadata_fetch_service.go`

**Step 1: Add pipeline execution after segment title generation**

```go
	// Steps 3-6 of the smart apply pipeline
	if config.AppConfig.AutoRenameOnApply {
		if err := mfs.runApplyPipeline(id, updatedBook); err != nil {
			log.Printf("[WARN] apply pipeline failed for %s: %v", id, err)
			// Pipeline failure is non-fatal — book data is already saved,
			// CoW version exists for recovery
		}
	}
```

The `runApplyPipeline` method orchestrates: generate titles → rename files → write tags → verify.

**Step 2: Verify build and tests**

Run: `go build ./... && go test ./internal/server/...`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/server/metadata_fetch_service.go
git commit -m "feat: wire smart apply pipeline into ApplyMetadataCandidate"
```

---

## Phase 4: Frontend — Version History + Tag Editor

### Task 11: Add version history API calls to frontend

**Files:**
- Modify: `web/src/services/api.ts`

**Step 1: Add API functions**

```typescript
export interface BookVersionEntry {
  timestamp: string;
  title: string;
  author_name: string;
}

export async function getBookVersionHistory(bookId: string, limit = 50): Promise<BookVersionEntry[]> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/versions-history?limit=${limit}`);
  if (!response.ok) throw await buildApiError(response, 'Failed to fetch version history');
  const data = await response.json();
  return data.versions || [];
}

export async function revertBookToVersion(bookId: string, timestamp: string): Promise<{ message: string; book: Book }> {
  const response = await fetch(`${API_BASE}/audiobooks/${bookId}/revert-metadata`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ timestamp }),
  });
  if (!response.ok) throw await buildApiError(response, 'Failed to revert');
  return response.json();
}
```

**Step 2: Update MetadataHistory component to show CoW versions**

Add a "Snapshots" tab or section that lists `BookVersionEntry` items with "Revert to this version" buttons, showing timestamp + title at that point.

**Step 3: TypeScript check**

Run: `cd web && npx tsc --noEmit`
Expected: PASS

**Step 4: Commit**

```bash
git add web/src/services/api.ts web/src/components/MetadataHistory.tsx
git commit -m "feat: frontend version history with revert support"
```

---

### Task 12: Add path format settings to Settings page

**Files:**
- Modify: `web/src/pages/Settings.tsx`
- Modify: `web/src/services/api.ts`

**Step 1: Add format template fields to Settings page**

Add a "File Naming" section with:
- Path Format text input (with preview showing example output)
- Segment Title Format text input (with preview)
- Auto-rename on apply toggle
- Auto-write tags on apply toggle
- Verify after write toggle

**Step 2: Wire to settings API**

Use existing `api.setSetting(key, value)` for each field.

**Step 3: TypeScript check and commit**

---

### Task 13: Tag editor inline editing (multi-select unified view)

**Files:**
- Modify: `web/src/pages/BookDetail.tsx`

This is the largest frontend task. Key changes:

**Step 1: Multi-select unified view**

When multiple files selected:
- Top: book-level fields (author, title, series, year, etc.) — each is an editable text field
- Mixed values show `< mixed >` placeholder
- Bottom: per-file table with track#, computed segment title, filename

**Step 2: Inline editing for single-select**

Replace static text cells with click-to-edit inputs. Tab navigation between fields.

**Step 3: Save button with pipeline preview**

"Preview" toggle shows computed filenames. "Save" triggers the apply pipeline.

**Step 4: TypeScript check and E2E smoke test**

Run: `cd web && npx tsc --noEmit`

**Step 5: Commit**

---

## Execution Order Summary

| Phase | Tasks | Estimated Scope |
|-------|-------|----------------|
| 1: CoW Versioning | Tasks 1-5 | DB layer + cleanup |
| 2: Path Format Engine | Tasks 6-7 | New module + config |
| 3: Smart Apply Pipeline | Tasks 8-10 | Backend pipeline |
| 4: Frontend | Tasks 11-13 | API + UI |

Each task is independently testable and committable. Phase 1 is the critical foundation — it prevents data loss immediately.
