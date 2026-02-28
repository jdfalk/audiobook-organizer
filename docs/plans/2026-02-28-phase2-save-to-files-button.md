<!-- file: docs/plans/2026-02-28-phase2-save-to-files-button.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890 -->
<!-- last-edited: 2026-02-28 -->

# Phase 2: Save to Files Button & Write-back Endpoint

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add explicit "Save to Files" button that writes DB metadata to audio files with per-segment numbering.
**Architecture:** New POST endpoint, enhanced tag writing with track numbers, frontend button with confirmation.
**Tech Stack:** Go, React/TypeScript, AtomicParsley/ffmpeg (CLI tools)

---

## Overview

Currently, file write-back only happens automatically during `FetchMetadataForBook` when
`config.WriteBackMetadata` is true. Manual edits via `PUT /audiobooks/:id` never write to
disk. This plan adds an on-demand "Save to Files" button so users can explicitly push DB
metadata into audio tags at any time.

### What will be built

| # | Component | File(s) touched |
|---|-----------|-----------------|
| 1 | Track-number support in `WriteMetadataToFile` | `internal/metadata/enhanced.go` |
| 2 | `WriteBackMetadataForBook` service method | `internal/server/metadata_fetch_service.go` |
| 3 | HTTP handler `writeBackAudiobookMetadata` | `internal/server/server.go` (handler added to same file) |
| 4 | Route registration | `internal/server/server.go` |
| 5 | API client function `writeBackMetadata` | `web/src/services/api.ts` |
| 6 | `SaveIcon` import and state variables | `web/src/pages/BookDetail.tsx` |
| 7 | Confirmation dialog and button in action bar | `web/src/pages/BookDetail.tsx` |
| 8 | Unit test for service method | `internal/server/metadata_fetch_service_test.go` |
| 9 | Integration test for HTTP endpoint | `internal/server/server_write_back_test.go` (new file) |

---

## Prerequisites / assumptions

- `AtomicParsley` is available in PATH for M4B files (`brew install atomicparsley`).
- `eyeD3` is available for MP3 files (`pip install eyeD3`).
- The `database.Book.UpdatedAt` field is populated by `db.UpdateBook`. No schema change is needed.
- The `BookSegment.TrackNumber` and `BookSegment.TotalTracks` fields already exist in the
  `database.BookSegment` struct (confirmed at `internal/database/store.go:453-454`).
- No new DB column for `last_written_at` is added in this phase (avoids migration complexity).
  The "disable if current" logic is deferred to a follow-up.

---

## Task 1 — Add track-number support to `WriteMetadataToFile`

**File:** `internal/metadata/enhanced.go`
**Version bump:** `1.3.0` → `1.4.0`

### What to change

Inside `writeM4BMetadata`, after the `genre` argument block and before the `exec.Command`
call, add a handler for the `"track"` key:

```go
// In writeM4BMetadata, after the genre block (around line 275):
if track, ok := metadata["track"].(string); ok && track != "" {
    args = append(args, "--tracknum", track)
}
```

Inside `writeMP3Metadata`, after the `year` argument block and before
`args = append(args, filePath)`, add:

```go
// In writeMP3Metadata, after the year block (around line 327):
if track, ok := metadata["track"].(string); ok && track != "" {
    args = append(args, "--track-num", track)
}
```

Inside `writeFLACMetadata`, after the `year` block and before
`args = append(args, filePath)`, add:

```go
// In writeFLACMetadata, after the year block (around line 385):
if track, ok := metadata["track"].(string); ok && track != "" {
    args = append(args, "--set-tag=TRACKNUMBER="+track)
}
```

### Exact diff for `writeM4BMetadata` (lines ~272-278 before edit)

```go
// BEFORE (line ~273):
	if year, ok := metadata["year"].(int); ok && year > 0 {
		args = append(args, "--year", fmt.Sprintf("%d", year))
	}

	cmd := exec.Command("AtomicParsley", args...)

// AFTER:
	if year, ok := metadata["year"].(int); ok && year > 0 {
		args = append(args, "--year", fmt.Sprintf("%d", year))
	}
	if track, ok := metadata["track"].(string); ok && track != "" {
		args = append(args, "--tracknum", track)
	}

	cmd := exec.Command("AtomicParsley", args...)
```

### Exact diff for `writeMP3Metadata` (lines ~325-329 before edit)

```go
// BEFORE (line ~326):
	if year, ok := metadata["year"].(int); ok && year > 0 {
		args = append(args, "--release-year", fmt.Sprintf("%d", year))
	}
	args = append(args, filePath)

// AFTER:
	if year, ok := metadata["year"].(int); ok && year > 0 {
		args = append(args, "--release-year", fmt.Sprintf("%d", year))
	}
	if track, ok := metadata["track"].(string); ok && track != "" {
		args = append(args, "--track-num", track)
	}
	args = append(args, filePath)
```

### Exact diff for `writeFLACMetadata` (lines ~383-387 before edit)

```go
// BEFORE (line ~384):
	if year, ok := metadata["year"].(int); ok && year > 0 {
		args = append(args, fmt.Sprintf("--set-tag=DATE=%d", year))
	}
	args = append(args, filePath)

// AFTER:
	if year, ok := metadata["year"].(int); ok && year > 0 {
		args = append(args, fmt.Sprintf("--set-tag=DATE=%d", year))
	}
	if track, ok := metadata["track"].(string); ok && track != "" {
		args = append(args, "--set-tag=TRACKNUMBER="+track)
	}
	args = append(args, filePath)
```

### Header update

Change line 3 from `// version: 1.3.0` to `// version: 1.4.0`.

---

## Task 2 — Add `WriteBackMetadataForBook` to the service

**File:** `internal/server/metadata_fetch_service.go`
**Version bump:** `3.0.0` → `3.1.0`

### What to add

Add a new exported method on `*MetadataFetchService` after the existing `writeBackMetadata`
private method (after line 511). This method is called by the HTTP handler.

```go
// WriteBackMetadataForBook reads current DB metadata for the book, resolves authors and
// narrators, writes comprehensive tags to all active audio file segments, and records a
// history entry. It is called by POST /api/v1/audiobooks/:id/write-back.
func (mfs *MetadataFetchService) WriteBackMetadataForBook(id string) (int, error) {
	book, err := mfs.db.GetBookByID(id)
	if err != nil || book == nil {
		return 0, fmt.Errorf("audiobook not found: %s", id)
	}

	// --- Resolve author names ---
	var authorNames []string
	bookAuthors, err := mfs.db.GetBookAuthors(id)
	if err == nil && len(bookAuthors) > 0 {
		for _, ba := range bookAuthors {
			if author, aerr := mfs.db.GetAuthorByID(ba.AuthorID); aerr == nil && author != nil {
				authorNames = append(authorNames, author.Name)
			}
		}
	} else if book.AuthorID != nil {
		if author, aerr := mfs.db.GetAuthorByID(*book.AuthorID); aerr == nil && author != nil {
			authorNames = append(authorNames, author.Name)
		}
	}
	artistStr := strings.Join(authorNames, " & ")

	// --- Resolve narrator names ---
	var narratorNames []string
	bookNarrators, err := mfs.db.GetBookNarrators(id)
	if err == nil && len(bookNarrators) > 0 {
		for _, bn := range bookNarrators {
			if narrator, nerr := mfs.db.GetNarratorByID(bn.NarratorID); nerr == nil && narrator != nil {
				narratorNames = append(narratorNames, narrator.Name)
			}
		}
	} else if book.Narrator != nil && *book.Narrator != "" {
		narratorNames = append(narratorNames, *book.Narrator)
	}
	narratorStr := strings.Join(narratorNames, " & ")

	// --- Determine year ---
	year := 0
	if book.AudiobookReleaseYear != nil && *book.AudiobookReleaseYear > 0 {
		year = *book.AudiobookReleaseYear
	} else if book.PrintYear != nil && *book.PrintYear > 0 {
		year = *book.PrintYear
	}

	opConfig := fileops.OperationConfig{VerifyChecksums: true}

	// --- Collect active segments ---
	bookNumericID := int(crc32.ChecksumIEEE([]byte(book.ID)))
	segments, segErr := mfs.db.ListBookSegments(bookNumericID)
	if segErr != nil {
		segments = nil
	}
	// Filter to active only
	var activeSegments []database.BookSegment
	for _, seg := range segments {
		if seg.Active {
			activeSegments = append(activeSegments, seg)
		}
	}

	totalTracks := len(activeSegments)
	writtenCount := 0

	if totalTracks > 1 {
		// Multi-file: write to each segment with per-track title and numbering
		digits := len(fmt.Sprintf("%d", totalTracks))
		trackFmt := fmt.Sprintf("%%0%dd", digits)
		for i, seg := range activeSegments {
			trackNum := i + 1
			segTitle := fmt.Sprintf(trackFmt+" - %s", trackNum, book.Title)
			trackStr := fmt.Sprintf("%d/%d", trackNum, totalTracks)
			tagMap := mfs.buildTagMap(book.Title, segTitle, artistStr, narratorStr, year, trackStr)
			if err := metadata.WriteMetadataToFile(seg.FilePath, tagMap, opConfig); err != nil {
				log.Printf("[WARN] write-back failed for segment %s: %v", seg.FilePath, err)
			} else {
				writtenCount++
			}
		}
	} else {
		// Single-file or no segments: write to book.FilePath
		tagMap := mfs.buildTagMap(book.Title, book.Title, artistStr, narratorStr, year, "")
		if err := metadata.WriteMetadataToFile(book.FilePath, tagMap, opConfig); err != nil {
			log.Printf("[WARN] write-back failed for %s: %v", book.FilePath, err)
		} else {
			writtenCount++
		}
	}

	// --- Record history entry ---
	now := time.Now()
	summaryVal := fmt.Sprintf("%q (wrote %d file(s))", book.Title, writtenCount)
	summaryJSON := jsonEncodeString(summaryVal)
	record := &database.MetadataChangeRecord{
		BookID:     book.ID,
		Field:      "write_back",
		NewValue:   &summaryJSON,
		ChangeType: "write-back",
		Source:     "manual",
		ChangedAt:  now,
	}
	if err := mfs.db.RecordMetadataChange(record); err != nil {
		log.Printf("[WARN] failed to record write-back history for %s: %v", book.ID, err)
	}

	return writtenCount, nil
}

// buildTagMap constructs the tag map shared by all write-back paths.
func (mfs *MetadataFetchService) buildTagMap(
	albumTitle, trackTitle, artist, narrator string, year int, track string,
) map[string]interface{} {
	tagMap := make(map[string]interface{})
	tagMap["title"] = trackTitle
	tagMap["album"] = albumTitle
	if artist != "" {
		tagMap["artist"] = artist
	}
	if narrator != "" {
		tagMap["narrator"] = narrator
	}
	if year > 0 {
		tagMap["year"] = year
	}
	tagMap["genre"] = "Audiobook"
	if track != "" {
		tagMap["track"] = track
	}
	return tagMap
}
```

### Header update

Change line 3 from `// version: 3.0.0` to `// version: 3.1.0`.

### Import check

The new method uses:
- `fmt` — already imported
- `strings` — already imported
- `time` — already imported
- `log` — already imported
- `hash/crc32` — already imported
- `github.com/jdfalk/audiobook-organizer/internal/database` — already imported
- `github.com/jdfalk/audiobook-organizer/internal/fileops` — already imported
- `github.com/jdfalk/audiobook-organizer/internal/metadata` — already imported

No new imports needed.

---

## Task 3 — Add HTTP handler `writeBackAudiobookMetadata`

**File:** `internal/server/server.go`
**Version bump:** `1.72.0` → `1.73.0`

### Where to add

Add a new method on `*Server` immediately after the `fetchAudiobookMetadata` handler.
Search for `func (s *Server) fetchAudiobookMetadata` in `server.go` to locate the insertion
point. Add the new handler directly after the closing brace of `fetchAudiobookMetadata`.

```go
// writeBackAudiobookMetadata handles POST /api/v1/audiobooks/:id/write-back.
// It writes current DB metadata for the book to all active audio files on disk.
func (s *Server) writeBackAudiobookMetadata(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "book id is required"})
		return
	}

	svc := NewMetadataFetchService(s.db)
	writtenCount, err := svc.WriteBackMetadataForBook(id)
	if err != nil {
		if err.Error() == fmt.Sprintf("audiobook not found: %s", id) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		log.Printf("[ERROR] write-back failed for book %s: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       fmt.Sprintf("metadata written to %d file(s)", writtenCount),
		"written_count": writtenCount,
	})
}
```

### Header update

Change line 3 from `// version: 1.72.0` to `// version: 1.73.0`.

---

## Task 4 — Register the new route

**File:** `internal/server/server.go` (same file, route registration block)

Find the existing line (around line 1135):

```go
protected.POST("/audiobooks/:id/fetch-metadata", s.fetchAudiobookMetadata)
```

Add the new route directly after it:

```go
protected.POST("/audiobooks/:id/fetch-metadata", s.fetchAudiobookMetadata)
protected.POST("/audiobooks/:id/write-back", s.writeBackAudiobookMetadata)
```

---

## Task 5 — API client function

**File:** `web/src/services/api.ts`
**Version bump:** `1.24.0` → `1.25.0`

### Add a response interface and function

Find the `fetchBookMetadata` function (at line ~1235). Add the following immediately
after its closing brace:

```typescript
export interface WriteBackMetadataResponse {
  message: string;
  written_count: number;
}

export async function writeBackMetadata(
  bookId: string
): Promise<WriteBackMetadataResponse> {
  const response = await fetch(
    `${API_BASE}/audiobooks/${bookId}/write-back`,
    {
      method: 'POST',
    }
  );
  if (!response.ok) {
    throw await buildApiError(response, 'Failed to write metadata to files');
  }
  return response.json();
}
```

### Header update

Change line 4 from `// version: 1.24.0` to `// version: 1.25.0`.

---

## Task 6 — Frontend state and imports in BookDetail.tsx

**File:** `web/src/pages/BookDetail.tsx`
**Version bump:** `1.15.0` → `1.16.0`

### 6a — Add SaveIcon import

Find the existing icon imports (around lines 33-41). Add after `StorageIcon`:

```typescript
import SaveIcon from '@mui/icons-material/Save.js';
```

### 6b — Add state variables

Find the block of existing state declarations near the top of the `BookDetail` component
(around lines 55-80). Add after the `parsingWithAI` state:

```typescript
const [writingToFiles, setWritingToFiles] = useState(false);
const [writeBackDialogOpen, setWriteBackDialogOpen] = useState(false);
```

### Header update

Change line 3 from `// version: 1.15.0` to `// version: 1.16.0`.

---

## Task 7 — Confirmation dialog and button

**File:** `web/src/pages/BookDetail.tsx`

### 7a — Handler function

Find `handleFetchMetadata` (around line 264). Add the following handler after its closing
brace (after line 285):

```typescript
const handleWriteBackMetadata = async () => {
  if (!book) return;
  setWritingToFiles(true);
  setWriteBackDialogOpen(false);
  try {
    const result = await api.writeBackMetadata(book.id);
    toast(result.message || 'Metadata written to files.', 'success');
  } catch (error: unknown) {
    console.error('Failed to write metadata to files', error);
    const msg =
      error instanceof Error ? error.message : 'Write to files failed.';
    toast(msg, 'error');
  } finally {
    setWritingToFiles(false);
  }
};
```

### 7b — "Save to Files" button in the action bar

Find the "Fetch Metadata" button block in the action bar (around line 840-853):

```tsx
            <Button
              variant="outlined"
              startIcon={
                fetchingMetadata ? (
                  <CircularProgress size={20} />
                ) : (
                  <CloudDownloadIcon />
                )
              }
              onClick={handleFetchMetadata}
              disabled={fetchingMetadata || actionLoading}
            >
              {fetchingMetadata ? 'Fetching...' : 'Fetch Metadata'}
            </Button>
```

Add the new button **immediately after** the closing `</Button>` of "Fetch Metadata" and
before the "Parse with AI" button:

```tsx
            <Button
              variant="outlined"
              startIcon={
                writingToFiles ? (
                  <CircularProgress size={20} />
                ) : (
                  <SaveIcon />
                )
              }
              onClick={() => setWriteBackDialogOpen(true)}
              disabled={writingToFiles || actionLoading}
            >
              {writingToFiles ? 'Writing...' : 'Save to Files'}
            </Button>
```

### 7c — Confirmation dialog

Find the existing `deleteDialogOpen` Dialog component in BookDetail.tsx (search for
`setDeleteDialogOpen(false)` inside a Dialog to locate it). Add the write-back
confirmation dialog anywhere in the JSX return before the final closing tag, after the
delete dialog:

```tsx
      {/* Write-back confirmation dialog */}
      <Dialog
        open={writeBackDialogOpen}
        onClose={() => setWriteBackDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Save Metadata to Files</DialogTitle>
        <DialogContent>
          <Typography variant="body1" gutterBottom>
            This will write the following metadata from the database directly
            into the audio file tags on disk:
          </Typography>
          <Box component="ul" sx={{ mt: 1 }}>
            <li>
              <strong>Title</strong> — {book?.title}
            </li>
            <li>
              <strong>Album</strong> — {book?.title} (groups tracks in players)
            </li>
            <li>
              <strong>Artist</strong> —{' '}
              {book?.authors?.map((a) => a.name).join(' & ') ||
                book?.author_name ||
                '(none)'}
            </li>
            <li>
              <strong>Narrator</strong> —{' '}
              {book?.narrators?.map((n) => n.name).join(' & ') ||
                book?.narrator ||
                '(none)'}
            </li>
            <li>
              <strong>Year</strong> —{' '}
              {book?.audiobook_release_year || book?.print_year || '(none)'}
            </li>
            <li>
              <strong>Genre</strong> — Audiobook
            </li>
            <li>
              <strong>Track numbers</strong> — written for multi-file books
            </li>
          </Box>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
            A backup of each file is created before writing and removed on
            success. The original file is restored automatically if writing
            fails.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setWriteBackDialogOpen(false)}>Cancel</Button>
          <Button
            variant="contained"
            startIcon={<SaveIcon />}
            onClick={handleWriteBackMetadata}
          >
            Write to Files
          </Button>
        </DialogActions>
      </Dialog>
```

---

## Task 8 — Unit test for `WriteBackMetadataForBook`

**File:** `internal/server/metadata_fetch_service_test.go`

Find the existing test file (it should already exist given the pattern). Add a new test
function. If the file does not exist, create it with the package header first.

The test uses `mocks.NewMockStore(t)` as per the project patterns noted in MEMORY.md.

```go
func TestWriteBackMetadataForBook_NotFound(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	mockStore.On("GetBookByID", "missing-id").Return(nil, nil)

	svc := NewMetadataFetchService(mockStore)
	_, err := svc.WriteBackMetadataForBook("missing-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audiobook not found")
}

func TestWriteBackMetadataForBook_SingleFile(t *testing.T) {
	// Uses a real temp file so WriteMetadataToFile can be tested end-to-end
	// only if AtomicParsley / eyeD3 are available; otherwise this tests the
	// tag-map construction path.
	//
	// Create a minimal book in a mock store.
	authorID := 42
	year := 2022
	book := &database.Book{
		ID:                   "01JTEST000000000000000001",
		Title:                "Test Book",
		AuthorID:             &authorID,
		AudiobookReleaseYear: &year,
		FilePath:             "/tmp/test-nonexistent.m4b",
	}
	author := &database.Author{ID: 42, Name: "Test Author"}

	mockStore := mocks.NewMockStore(t)
	mockStore.On("GetBookByID", book.ID).Return(book, nil)
	mockStore.On("GetBookAuthors", book.ID).Return([]database.BookAuthor{
		{BookID: book.ID, AuthorID: 42, Role: "author", Position: 0},
	}, nil)
	mockStore.On("GetAuthorByID", 42).Return(author, nil)
	mockStore.On("GetBookNarrators", book.ID).Return([]database.BookNarrator{}, nil)
	mockStore.On("ListBookSegments", mock.AnythingOfType("int")).Return([]database.BookSegment{}, nil)
	mockStore.On("RecordMetadataChange", mock.AnythingOfType("*database.MetadataChangeRecord")).Return(nil)

	svc := NewMetadataFetchService(mockStore)
	// WriteMetadataToFile will fail because the file doesn't exist, but the
	// function should return (0, nil) not an error — failures are logged not returned.
	count, err := svc.WriteBackMetadataForBook(book.ID)
	require.NoError(t, err)
	// writtenCount is 0 because the temp file does not exist; no fatal error.
	assert.Equal(t, 0, count)
}
```

**Important:** Add necessary imports to the test file:
```go
import (
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"

    "github.com/jdfalk/audiobook-organizer/internal/database"
    "github.com/jdfalk/audiobook-organizer/internal/database/mocks"
)
```

---

## Task 9 — Integration test for the HTTP endpoint

**File:** `internal/server/server_write_back_test.go` (new file)

```go
// file: internal/server/server_write_back_test.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f23456789012

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteBackEndpoint_NotFound(t *testing.T) {
	ts := setupTestServer(t) // uses real SQLite per project patterns (MEMORY.md)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/audiobooks/nonexistent-id/write-back", nil)
	w := httptest.NewRecorder()
	ts.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestWriteBackEndpoint_ExistingBook_NoFiles(t *testing.T) {
	ts := setupTestServer(t)

	// Insert a book that has no real file on disk
	book := insertTestBook(t, ts.DB(), "Write-back Test", "/tmp/no-such-file.m4b")

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/audiobooks/"+book.ID+"/write-back",
		nil,
	)
	w := httptest.NewRecorder()
	ts.Router().ServeHTTP(w, req)

	// Endpoint returns 200 even when files fail — failures are warnings not errors
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "written_count")
}
```

**Note:** `setupTestServer`, `insertTestBook`, and similar helpers follow the patterns
established across the existing test suite. Check `internal/server/server_test.go` for
the exact signatures of `setupTestServer` and `insertTestBook` before writing this file.
Adapt parameter names to match what already exists.

---

## Build and test commands

```bash
# 1. Compile backend only (fast iteration)
make build-api

# 2. Run all Go tests
make test

# 3. Run only the new unit test
go test ./internal/server/... -run TestWriteBack -v

# 4. Run only the new integration test
go test ./internal/server/... -run TestWriteBackEndpoint -v

# 5. Run frontend tests
cd web && npm test -- --run

# 6. Full build (embeds frontend)
make build

# 7. Full test suite with coverage gate
make ci
```

---

## Manual verification steps

1. Start the server: `make run`
2. Open the browser at `http://localhost:8080`
3. Navigate to any book detail page that has a real `.m4b` or `.mp3` file on disk.
4. Verify the "Save to Files" button appears in the action bar between "Fetch Metadata"
   and "Parse with AI".
5. Click "Save to Files" — a confirmation dialog must appear listing title, artist,
   narrator, year, genre, and track number notes.
6. Click "Write to Files" — a success toast must appear: "metadata written to N file(s)".
7. Verify with `AtomicParsley /path/to/file.m4b -t` or
   `ffprobe -show_entries format_tags /path/to/file.m4b` that tags were updated.
8. Click "Save to Files" on a multi-segment book and verify each segment received a
   title like `"01 - Book Title"`, `"02 - Book Title"`, and a track tag `"1/2"`, `"2/2"`.
9. Verify a write-back history entry appears in the "History" tab with
   `change_type = "write-back"` and `source = "manual"`.

---

## Edge cases handled

| Scenario | Behavior |
|----------|----------|
| File does not exist on disk | `WriteMetadataToFile` returns error; logged as `[WARN]`; `writtenCount` stays 0; HTTP 200 returned with `written_count: 0` |
| `AtomicParsley` not in PATH | `WriteMetadataToFile` returns error; same warn-and-continue behavior |
| Book not in DB | HTTP 404 |
| Book has no author | `artist` tag is omitted from tag map (empty string check in `buildTagMap`) |
| Book has no narrator | `narrator` / `comment` tag is omitted |
| Single-segment book (one `BookSegment` active) | Treated as single-file: title not prefixed, no track tag written |
| Zero active segments | Falls through to single-file path using `book.FilePath` |
| 10+ segments | Zero-padded to two digits: `"01 - Title"` through `"10 - Title"` |
| 100+ segments | Zero-padded to three digits automatically via `fmt.Sprintf` digit count |

---

## Files changed summary

| File | Change type | Version |
|------|-------------|---------|
| `internal/metadata/enhanced.go` | Edit — add `track` tag support to M4B, MP3, FLAC writers | `1.3.0` → `1.4.0` |
| `internal/server/metadata_fetch_service.go` | Edit — add `WriteBackMetadataForBook` + `buildTagMap` | `3.0.0` → `3.1.0` |
| `internal/server/server.go` | Edit — add handler + route registration | `1.72.0` → `1.73.0` |
| `web/src/services/api.ts` | Edit — add `WriteBackMetadataResponse` interface + `writeBackMetadata` function | `1.24.0` → `1.25.0` |
| `web/src/pages/BookDetail.tsx` | Edit — add `SaveIcon`, state, handler, button, dialog | `1.15.0` → `1.16.0` |
| `internal/server/metadata_fetch_service_test.go` | Edit — add two unit tests | bump version |
| `internal/server/server_write_back_test.go` | Create — integration tests for HTTP endpoint | `1.0.0` |
