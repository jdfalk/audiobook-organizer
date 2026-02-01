<!-- file: docs/plans/itunes-integration.md -->
<!-- version: 2.1.0 -->
<!-- guid: c2d3e4f5-a6b7-8c9d-0e1f-2a3b4c5d6e7f -->
<!-- last-edited: 2026-02-01 -->

# iTunes Integration

## Overview

Full bidirectional iTunes integration: import library metadata, organize and
write back updated paths, and eventually sync playcounts. Phase 1 (core
infrastructure) is complete. This is the **primary MVP blocker**.

**Status: 🟡 In Progress — Phase 1-3 complete; Phase 4 tests in progress**

---

## Phase 1 — Core Infrastructure ✅ Complete

- Database migration 11 for iTunes fields
- Audiobook model updated with iTunes fields (persistent ID, date added, play
  count, rating, bookmarks, playlists)
- iTunes Library.xml parser (howett.net/plist)
- Audiobook entry extraction (distinguishes from music/podcasts)
- Import service with validation, conversion, and all import modes
- Write-back support with automatic backup and rollback
- Manual QA guide created

### iTunes fields on `database.Book` (the live struct used by all store ops)

The `database.Book` struct in `internal/database/store.go` is the runtime
representation stored as JSON in PebbleDB under `book:<ULID>`. The iTunes
fields occupy lines 181-187. Full annotated layout of the iTunes section:

```go
// iTunes import fields — preserved verbatim from the Library.xml track entry.
// All fields are optional pointers; nil means "not imported from iTunes".
ITunesPersistentID *string    `json:"itunes_persistent_id,omitempty"`  // 16-char hex; unique across re-exports
ITunesDateAdded    *time.Time `json:"itunes_date_added,omitempty"`     // RFC3339; when track was added to iTunes
ITunesPlayCount    *int       `json:"itunes_play_count,omitempty"`     // cumulative plays; 0 is valid
ITunesLastPlayed   *time.Time `json:"itunes_last_played,omitempty"`    // derived from Track.PlayDate (Unix epoch → time.Time)
ITunesRating       *int       `json:"itunes_rating,omitempty"`         // 0-100 scale (iTunes uses 20/40/60/80/100 internally)
ITunesBookmark     *int64     `json:"itunes_bookmark,omitempty"`       // playback position in milliseconds
ITunesImportSource *string    `json:"itunes_import_source,omitempty"`  // absolute path to Library.xml that produced this record
```

These map 1:1 to `internal/itunes.Track` fields via `buildBookFromTrack()` in
`internal/server/itunes.go`. The `PersistentID` is the only stable cross-export
identifier; it is used as the join key during write-back.

### Migration 11 — SQLite schema (PebbleDB: no-op)

Migration 11 (`migration011Up` in `internal/database/migrations.go`) adds these
columns to the `books` table. For PebbleDB the fields live inside the JSON blob;
no key-schema change is required. SQLite requires explicit ALTER TABLE:

```sql
ALTER TABLE books ADD COLUMN itunes_persistent_id TEXT;
ALTER TABLE books ADD COLUMN itunes_date_added TIMESTAMP;
ALTER TABLE books ADD COLUMN itunes_play_count INTEGER DEFAULT 0;
ALTER TABLE books ADD COLUMN itunes_last_played TIMESTAMP;
ALTER TABLE books ADD COLUMN itunes_rating INTEGER;
ALTER TABLE books ADD COLUMN itunes_bookmark INTEGER;
ALTER TABLE books ADD COLUMN itunes_import_source TEXT;

CREATE INDEX IF NOT EXISTS idx_books_itunes_persistent_id ON books(itunes_persistent_id);
```

No PebbleDB secondary-index key is needed for `itunes_persistent_id` at this
time because write-back iterates a caller-supplied list of book IDs; it does not
need to look up books by persistent ID. If a future "find book by iTunes ID"
query is required, add: `book:itunes:<persistentID> → <bookULID>`.

---

## Phase 2 — API Endpoints

| Endpoint                          | Method | Description                        |
| --------------------------------- | ------ | ---------------------------------- |
| `/api/v1/itunes/validate`         | POST   | Validate an iTunes library file    |
| `/api/v1/itunes/import`           | POST   | Trigger import operation           |
| `/api/v1/itunes/write-back`       | POST   | Update iTunes with new file paths  |
| `/api/v1/itunes/import-status/:id`| GET    | Check import progress              |

All four endpoints are already implemented in `internal/server/itunes.go` and
wired into the router. The following sections document exactly what each does,
how it integrates with the operation queue, and what the frontend receives.

### Route registration

Routes are registered in `internal/server/server.go` inside the `api` group
(Gin router). The iTunes sub-group pattern:

```go
// internal/server/server.go — inside setupRoutes(), after line 749
itunesGroup := api.Group("/itunes")
{
    itunesGroup.POST("/validate",          s.handleITunesValidate)
    itunesGroup.POST("/import",            s.handleITunesImport)
    itunesGroup.POST("/write-back",        s.handleITunesWriteBack)
    itunesGroup.GET("/import-status/:id",  s.handleITunesImportStatus)
}
```

### POST /api/v1/itunes/validate

**Synchronous.** Parses the library, checks file existence, hashes for
duplicates. No operation record is created.

Request:
```go
type ITunesValidateRequest struct {
    LibraryPath string `json:"library_path" binding:"required"`
}
```

Response (200):
```go
type ITunesValidateResponse struct {
    TotalTracks     int      `json:"total_tracks"`
    AudiobookTracks int      `json:"audiobook_tracks"`
    FilesFound      int      `json:"files_found"`
    FilesMissing    int      `json:"files_missing"`
    MissingPaths    []string `json:"missing_paths,omitempty"`
    DuplicateCount  int      `json:"duplicate_count"`
    EstimatedTime   string   `json:"estimated_import_time"`
}
```

Handler logic (abbreviated from `handleITunesValidate`):
1. `os.Stat(req.LibraryPath)` — 404 if missing.
2. Calls `itunes.ValidateImport(opts)` which iterates all tracks, filters by
   `IsAudiobook()`, checks `os.Stat` on each decoded location, and optionally
   hashes files for duplicate detection.
3. Counts duplicate groups where `len(titles) > 1` and sums the excess.

### POST /api/v1/itunes/import

**Asynchronous.** Creates an operation record, enqueues work on the global
`operations.GlobalQueue`, and returns immediately with the operation ID.

Request:
```go
type ITunesImportRequest struct {
    LibraryPath      string `json:"library_path" binding:"required"`
    ImportMode       string `json:"import_mode" binding:"required,oneof=organized import organize"`
    PreserveLocation bool   `json:"preserve_location"`
    ImportPlaylists  bool   `json:"import_playlists"`
    SkipDuplicates   bool   `json:"skip_duplicates"`
}
```

Response (202 Accepted):
```go
type ITunesImportResponse struct {
    OperationID string `json:"operation_id"`   // ULID string
    Status      string `json:"status"`         // "queued"
    Message     string `json:"message"`
}
```

**How the operation is created and tracked** (the pattern every async handler
must follow):

```go
func (s *Server) handleITunesImport(c *gin.Context) {
    // 1. Generate a ULID for the operation
    opID := ulid.Make().String()

    // 2. Persist the operation record in the database (status = "pending")
    op, err := database.GlobalStore.CreateOperation(opID, "itunes_import", &req.LibraryPath)
    // CreateOperation writes: operation:<opID> → { id, type, status:"pending", ... }

    // 3. Create an in-memory status snapshot (thread-safe counters)
    status := &itunesImportStatus{}
    itunesImportStatuses.Store(op.ID, status)  // sync.Map keyed by opID

    // 4. Wrap the actual work in an OperationFunc closure.
    //    The closure receives a context (for cancellation) and a ProgressReporter.
    operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
        return executeITunesImport(ctx, progress, op.ID, req)
    }

    // 5. Enqueue on the global worker pool (2 workers by default)
    operations.GlobalQueue.Enqueue(op.ID, "itunes_import", operations.PriorityNormal, operationFunc)

    // 6. Return 202 immediately
    c.JSON(http.StatusAccepted, ITunesImportResponse{...})
}
```

**Inside `executeITunesImport`** — how progress is reported at each step:

```go
func executeITunesImport(ctx context.Context, progress operations.ProgressReporter, opID string, req ITunesImportRequest) error {
    status := loadITunesImportStatus(opID)

    // Report initial state (current=0, total=0 means "counting")
    progress.UpdateProgress(0, 0, "Starting iTunes import")
    progress.Log("info", "Starting iTunes import", nil)
    // UpdateProgress writes operation:<opID> with status="running"
    // AND calls realtime.GlobalHub.SendOperationProgress() which pushes SSE

    library, _ := itunes.ParseLibrary(req.LibraryPath)

    // Count audiobooks first so total is known
    totalBooks := countAudiobooks(library)
    setITunesImportTotal(status, totalBooks)  // mutex-guarded

    for _, track := range library.Tracks {
        if !itunes.IsAudiobook(track) { continue }
        if progress.IsCanceled() { return nil }  // checks operation:<opID>.status == "canceled"

        processed++
        // Build database.Book from track fields
        book, _ := buildBookFromTrack(track, req.LibraryPath)

        // Hash file, check blocklist, check duplicates
        hash, _ := scanner.ComputeFileHash(book.FilePath)
        // ... skip logic ...

        // Persist
        database.GlobalStore.CreateBook(book)
        updateITunesImported(status)

        // Throttled progress update — only every 10 books (itunesImportProgressBatch)
        updateITunesProgress(progress, status, processed, totalBooks)
        // This calls progress.UpdateProgress(processed, totalBooks, "Processed X/Y ...")
    }

    // Final summary
    progress.UpdateProgress(totalBooks, totalBooks, buildITunesSummary(status))
    return nil
}
```

### POST /api/v1/itunes/write-back

**Synchronous.** Rewrites the iTunes Library.xml file in place (with optional
backup). Does not create an operation record; it completes quickly enough
to block.

Request:
```go
type ITunesWriteBackRequest struct {
    LibraryPath  string   `json:"library_path" binding:"required"`
    AudiobookIDs []string `json:"audiobook_ids"`        // ULID strings
    CreateBackup bool     `json:"create_backup"`
}
```

Response (200):
```go
type ITunesWriteBackResponse struct {
    Success      bool   `json:"success"`
    UpdatedCount int    `json:"updated_count"`
    BackupPath   string `json:"backup_path,omitempty"`
    Message      string `json:"message"`
}
```

Handler logic:
1. For each `audiobook_id`, fetch `database.Book` via `GetBookByID`.
2. Skip books where `ITunesPersistentID` is nil or empty.
3. Build a `[]itunes.WriteBackUpdate` with `{PersistentID, NewPath: book.FilePath}`.
4. Call `itunes.WriteBack(opts)` which: (a) optionally copies Library.xml to
   `Library.xml.backup.<timestamp>`; (b) parses, updates track locations using
   `EncodeLocation(newPath)`, writes back via `writePlist()`; (c) on any
   write error restores the backup.

### GET /api/v1/itunes/import-status/:id

**Synchronous poll endpoint.** The frontend calls this every 2 seconds.

Response (200):
```go
type ITunesImportStatusResponse struct {
    OperationID string   `json:"operation_id"`
    Status      string   `json:"status"`            // pending|running|completed|failed|canceled
    Progress    int      `json:"progress"`          // 0-100 percentage
    Message     string   `json:"message"`
    TotalBooks  int      `json:"total_books,omitempty"`
    Processed   int      `json:"processed,omitempty"`
    Imported    int      `json:"imported,omitempty"`
    Skipped     int      `json:"skipped,omitempty"`
    Failed      int      `json:"failed,omitempty"`
    Errors      []string `json:"errors,omitempty"`  // capped at 50 entries
}
```

The handler reads the persisted `Operation` record from the database
(`operation:<opID>`) for status/progress/message, then overlays the
in-memory `itunesImportStatus` snapshot for the detailed counters.

### SSE progress event shapes

The operation queue's `ProgressReporter.UpdateProgress()` triggers three
real-time events via `realtime.GlobalHub`. These are the exact JSON payloads
the frontend receives over the SSE stream (event type is in the `event:` field
of the SSE frame):

**1. `operation:progress`** — emitted every `itunesImportProgressBatch` (10) books:
```json
{
  "operation_id": "01HXYZ...",
  "current":      45,
  "total":        120,
  "message":      "Processed 45/120 (imported 42, skipped 2, failed 1)",
  "percentage":   37
}
```

**2. `operation:status`** — emitted on terminal state changes (completed/failed/canceled):
```json
{
  "operation_id": "01HXYZ...",
  "status":       "completed",
  "details": {
    "current": 120,
    "total":   120,
    "message": "operation completed"
  }
}
```
On failure the `details` object contains `"error": "<message>"` instead.

**3. `operation:log`** — emitted for every `progress.Log()` call (per-book
errors, skip reasons, organization results):
```json
{
  "operation_id": "01HXYZ...",
  "level":        "info",
  "message":      "Skipping duplicate hash: The Hobbit",
  "details":      null
}
```

The frontend currently polls `/import-status/:id` every 2 seconds rather than
consuming these SSE events. A future improvement would wire the SSE stream
directly into the `ITunesImport` component to eliminate polling latency.

### TypeScript API client — iTunes methods

These functions already exist in `web/src/services/api.ts`. Shown here for
completeness and as the reference contract for any new endpoint additions:

```typescript
// web/src/services/api.ts

// --- Request / Response types (already defined) ---

export interface ITunesValidateRequest  { library_path: string; }
export interface ITunesValidateResponse {
  total_tracks: number;
  audiobook_tracks: number;
  files_found: number;
  files_missing: number;
  missing_paths?: string[];
  duplicate_count: number;
  estimated_import_time: string;
}

export interface ITunesImportRequest {
  library_path: string;
  import_mode: 'organized' | 'import' | 'organize';
  preserve_location: boolean;
  import_playlists: boolean;
  skip_duplicates: boolean;
}

export interface ITunesImportResponse {
  operation_id: string;
  status: string;
  message: string;
}

export interface ITunesWriteBackRequest {
  library_path: string;
  audiobook_ids: string[];   // ULID strings
  create_backup: boolean;
}

export interface ITunesWriteBackResponse {
  success: boolean;
  updated_count: number;
  backup_path?: string;
  message: string;
}

export interface ITunesImportStatus {
  operation_id: string;
  status: string;            // pending|running|completed|failed|canceled
  progress: number;          // 0-100
  message: string;
  total_books?: number;
  processed?: number;
  imported?: number;
  skipped?: number;
  failed?: number;
  errors?: string[];
}

// --- Client functions (follow the same fetch + buildApiError pattern) ---

export async function validateITunesLibrary(payload: ITunesValidateRequest): Promise<ITunesValidateResponse> {
  const response = await fetch(`${API_BASE}/itunes/validate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!response.ok) throw await buildApiError(response, 'Validation failed');
  return response.json();
}

export async function importITunesLibrary(payload: ITunesImportRequest): Promise<ITunesImportResponse> {
  const response = await fetch(`${API_BASE}/itunes/import`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!response.ok) throw await buildApiError(response, 'Import failed');
  return response.json();
}

export async function getITunesImportStatus(operationId: string): Promise<ITunesImportStatus> {
  const response = await fetch(`${API_BASE}/itunes/import-status/${operationId}`);
  if (!response.ok) throw await buildApiError(response, 'Failed to get status');
  return response.json();
}

export async function writeBackITunesLibrary(payload: ITunesWriteBackRequest): Promise<ITunesWriteBackResponse> {
  const response = await fetch(`${API_BASE}/itunes/write-back`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (!response.ok) throw await buildApiError(response, 'Write-back failed');
  return response.json();
}
```

---

## Phase 3 — UI Components

**Status: ✅ Complete (2026-02-01)**

Settings page additions:

- iTunes Import section with file picker for Library.xml
- Validation results display (summary of what will be imported)
- Import options dialog: mode selection, playlist handling, duplicate strategy
- Progress monitoring with real-time updates via SSE
- Write-back confirmation dialog with preview of path changes

### Component architecture

The iTunes UI lives in a single file component:
`web/src/components/settings/ITunesImport.tsx`. It is rendered as a `<Card>`
inside the Settings page. The project uses Material-UI (MUI) for all UI
primitives and Zustand (`useAppStore`) for global notifications/errors.

The component is already fully implemented. The structure below documents
what exists and what would need to change to add write-back confirmation.

### ITunesImport component — state machine

```
Initial State
    │  user enters libraryPath
    ▼
Path Entry (TextField + Browse button)
    │  clicks "Validate Import"
    ▼
Validating (setValidating=true, spinner on button)
    │  validateITunesLibrary() resolves
    ▼
Validation Results (Alert card: success or warning)
    │  clicks "Import Library"
    ▼
Importing (setImporting=true, polls every 2s)
    │  pollImportStatus() loop
    ▼
Progress Display (LinearProgress bar + counters)
    │  status === 'completed' or 'failed'
    ▼
Terminal State (success Alert or error Alert)
```

### Key React patterns used in this component

**1. File path input** — There is no native file picker for server-side paths.
The current implementation uses `window.prompt()`. A production improvement
would use an Electron-style dialog or a typed text field only:

```tsx
// Current pattern (web/src/components/settings/ITunesImport.tsx)
const handleBrowseFile = () => {
  const path = window.prompt('Enter path to iTunes Library.xml:');
  if (path) {
    setSettings((prev) => ({ ...prev, libraryPath: path }));
  }
};

// The TextField uses an endAdornment for the browse button:
<TextField
  label="iTunes Library Path"
  value={settings.libraryPath}
  onChange={(e) => setSettings(prev => ({ ...prev, libraryPath: e.target.value }))}
  fullWidth
  InputProps={{
    endAdornment: (
      <Button startIcon={<FolderOpenIcon />} onClick={handleBrowseFile}>Browse</Button>
    ),
  }}
/>
```

**2. Import mode selection** — RadioGroup with three options matching the
backend's `oneof=organized import organize` constraint:

```tsx
<FormControl component="fieldset">
  <FormLabel component="legend">Import Mode</FormLabel>
  <RadioGroup
    value={settings.importMode}
    onChange={(e) => setSettings(prev => ({
      ...prev,
      importMode: e.target.value as 'organized' | 'import' | 'organize',
    }))}
  >
    <FormControlLabel value="organized" control={<Radio />} label="Files already organized" />
    <FormControlLabel value="import"    control={<Radio />} label="Import metadata only" />
    <FormControlLabel value="organize"  control={<Radio />} label="Import and organize now" />
  </RadioGroup>
</FormControl>
```

**3. Polling loop** — Uses `useRef` to hold the timeout handle so it can be
cleared on unmount (no memory leak):

```tsx
const pollTimeoutRef = useRef<number | null>(null);

useEffect(() => {
  return () => {
    if (pollTimeoutRef.current) window.clearTimeout(pollTimeoutRef.current);
  };
}, []);

const pollImportStatus = async (operationId: string) => {
  const poll = async () => {
    const status = await getITunesImportStatus(operationId);
    setImportStatus(status);

    if (status.status === 'completed' || status.status === 'failed') {
      setImporting(false);
      return;  // stop polling
    }
    // continue polling after 2 seconds
    pollTimeoutRef.current = window.setTimeout(poll, 2000);
  };
  await poll();
};
```

**4. Missing files dialog** — A `<Dialog>` that shows only when
`showMissingFiles` is true. Lists paths from `validationResult.missing_paths`:

```tsx
<Dialog open={showMissingFiles} onClose={() => setShowMissingFiles(false)} maxWidth="md" fullWidth>
  <DialogTitle>Missing Files</DialogTitle>
  <DialogContent>
    <List>
      {validationResult?.missing_paths?.map((path) => (
        <ListItem key={path}>
          <ListItemText primary={path} />
        </ListItem>
      ))}
    </List>
  </DialogContent>
  <DialogActions>
    <Button onClick={() => setShowMissingFiles(false)}>Close</Button>
  </DialogActions>
</Dialog>
```

### Write-back confirmation dialog (completed)

The write-back UI now follows the same Dialog pattern. The confirmation dialog
handles:

1. Accept a list of book IDs (manual entry).
2. Show a preview table: `Title | Current Path | iTunes Persistent ID`.
3. Provide a "Create backup" checkbox (default true).
4. Call `writeBackITunesLibrary(payload)` and render the result summary.

Skeleton:

```tsx
export function WriteBackConfirmDialog({
  open,
  books,       // database.Book[] — filtered to those with itunes_persistent_id
  libraryPath, // string — path to Library.xml
  onClose,
  onSuccess,
}: WriteBackConfirmDialogProps) {
  const [createBackup, setCreateBackup] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [result, setResult] = useState<ITunesWriteBackResponse | null>(null);

  const handleConfirm = async () => {
    setSubmitting(true);
    try {
      const res = await writeBackITunesLibrary({
        library_path: libraryPath,
        audiobook_ids: books.map(b => b.id),
        create_backup: createBackup,
      });
      setResult(res);
      onSuccess(res);
    } catch (err) {
      // surface error via useAppStore().addNotification(...)
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
      <DialogTitle>Write Back to iTunes</DialogTitle>
      <DialogContent>
        <FormControlLabel
          control={<Checkbox checked={createBackup} onChange={e => setCreateBackup(e.target.checked)} />}
          label="Create backup before modifying Library.xml"
        />
        {/* Preview table: Title | FilePath | PersistentID */}
        <Table>
          {/* ... map books to rows ... */}
        </Table>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={submitting}>Cancel</Button>
        <Button variant="contained" onClick={handleConfirm} disabled={submitting}>
          {submitting ? 'Writing...' : `Update ${books.length} entries`}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
```

---

## Phase 4 — Testing

**Status: 🟡 In Progress (unit tests updated 2026-02-01)**

- Unit tests: XML parser, import logic, write-back logic
- Integration tests: full import → organize → write-back cycle
- E2E tests: UI workflow from file selection through import completion

### Existing test coverage

`internal/itunes/parser_test.go` already covers:
- `TestIsAudiobook` — table-driven tests for Kind, Genre, Location heuristics
- `TestDecodeLocation` — URL decoding including `%20` spaces
- `TestEncodeLocation` — round-trip path → file:// URL encoding
- `TestFindLibraryFile` — graceful handling when no iTunes library is present

`internal/itunes/integration_test.go` exists for end-to-end import flow tests.

### Test patterns used in this project

All tests use the standard `testing` package with table-driven subtests
(`t.Run`). The project does not use a third-party assertion library in the
`itunes` package tests — comparisons are done with direct `if` checks and
`t.Errorf` / `t.Fatalf`. The `server` package tests use `testify/assert`.

### Parser unit tests — what to add

```go
// internal/itunes/parser_test.go — additions

// TestIsAudiobook_EdgeCases covers boundary conditions the current tests miss.
func TestIsAudiobook_EdgeCases(t *testing.T) {
    tests := []struct {
        name     string
        track    *Track
        expected bool
    }{
        {
            name:     "nil track returns false",
            track:    nil,
            expected: false,
        },
        {
            name:     "case-insensitive Kind match",
            track:    &Track{Kind: "AUDIOBOOK FILE"},
            expected: true,
        },
        {
            name:     "spoken word in genre",
            track:    &Track{Genre: "Spoken Word Fiction"},
            expected: true,
        },
        {
            name:     "audiobooks subfolder lowercase",
            track:    &Track{Location: "file:///data/audiobooks/novel.m4b"},
            expected: true,
        },
        {
            name:     "podcast is not audiobook",
            track:    &Track{Kind: "Podcast", Genre: "News"},
            expected: false,
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := IsAudiobook(tt.track); got != tt.expected {
                t.Errorf("IsAudiobook() = %v, want %v", got, tt.expected)
            }
        })
    }
}

// TestExtractSeriesFromAlbum validates series name parsing heuristics.
func TestExtractSeriesFromAlbum(t *testing.T) {
    tests := []struct {
        name           string
        album          string
        wantSeries     string
    }{
        {"comma separator",  "Dark Tower, Book 1",    "Dark Tower"},
        {"dash separator",   "Discworld - Book 3",   "Discworld"},
        {"colon separator",  "Foundation: Part 2",   "Foundation"},
        {"no separator",     "Standalone Title",     "Standalone Title"},
        {"empty string",     "",                     ""},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            series, _ := extractSeriesFromAlbum(tt.album)
            if series != tt.wantSeries {
                t.Errorf("extractSeriesFromAlbum(%q) series = %q, want %q", tt.album, series, tt.wantSeries)
            }
        })
    }
}
```

### Import service unit tests — what to add

The `buildBookFromTrack` function in `internal/server/itunes.go` is the core
conversion logic. It needs tests that verify field mapping without touching the
filesystem. Use `os.CreateTemp` to create a dummy file so `os.Stat` succeeds:

```go
// internal/server/itunes_test.go — additions

func TestBuildBookFromTrack(t *testing.T) {
    // Create a temporary file so os.Stat succeeds
    tmp, err := os.CreateTemp(t.TempDir(), "test-*.m4b")
    if err != nil {
        t.Fatal(err)
    }
    tmp.WriteString("fake audio data")
    tmp.Close()
    tmpPath := tmp.Name()

    // Encode the path as an iTunes file:// URL
    location := itunes.EncodeLocation(tmpPath)

    track := &itunes.Track{
        TrackID:      42,
        PersistentID: "AABB1122CC3344DD",
        Name:         "The Great Gatsby",
        Artist:       "F. Scott Fitzgerald",
        AlbumArtist:  "Nick Carraway",  // different from Artist → becomes Narrator
        Album:        "Classics, Book 7",
        Kind:         "Audiobook",
        Year:         2020,
        Location:     location,
        Size:         int64(len("fake audio data")),
        TotalTime:    3600000,  // 1 hour in ms
        DateAdded:    time.Date(2023, 6, 15, 10, 0, 0, 0, time.UTC),
        PlayCount:    5,
        PlayDate:     1700000000,  // 2023-11-14 22:13:20 UTC
        Rating:       80,
        Bookmark:     1800000,  // 30 minutes in ms
        Comments:     "Unabridged",
    }

    book, err := buildBookFromTrack(track, "/path/to/Library.xml")
    if err != nil {
        t.Fatalf("buildBookFromTrack() error = %v", err)
    }

    // Verify iTunes field mapping
    if book.ITunesPersistentID == nil || *book.ITunesPersistentID != "AABB1122CC3344DD" {
        t.Errorf("ITunesPersistentID = %v, want AABB1122CC3344DD", book.ITunesPersistentID)
    }
    if book.ITunesPlayCount == nil || *book.ITunesPlayCount != 5 {
        t.Errorf("ITunesPlayCount = %v, want 5", book.ITunesPlayCount)
    }
    if book.ITunesRating == nil || *book.ITunesRating != 80 {
        t.Errorf("ITunesRating = %v, want 80", book.ITunesRating)
    }
    if book.ITunesBookmark == nil || *book.ITunesBookmark != 1800000 {
        t.Errorf("ITunesBookmark = %v, want 1800000", book.ITunesBookmark)
    }
    if book.ITunesImportSource == nil || *book.ITunesImportSource != "/path/to/Library.xml" {
        t.Errorf("ITunesImportSource = %v, want /path/to/Library.xml", book.ITunesImportSource)
    }

    // Verify narrator extraction (AlbumArtist != Artist)
    if book.Narrator == nil || *book.Narrator != "Nick Carraway" {
        t.Errorf("Narrator = %v, want Nick Carraway", book.Narrator)
    }
    // Verify edition from Comments
    if book.Edition == nil || *book.Edition != "Unabridged" {
        t.Errorf("Edition = %v, want Unabridged", book.Edition)
    }
    // Verify duration conversion (3600000ms → 3600s)
    if book.Duration == nil || *book.Duration != 3600 {
        t.Errorf("Duration = %v, want 3600", book.Duration)
    }
    // Verify PlayDate → ITunesLastPlayed conversion
    if book.ITunesLastPlayed == nil {
        t.Fatal("ITunesLastPlayed is nil, want non-nil")
    }
    expectedLastPlayed := time.Unix(1700000000, 0)
    if !book.ITunesLastPlayed.Equal(expectedLastPlayed) {
        t.Errorf("ITunesLastPlayed = %v, want %v", *book.ITunesLastPlayed, expectedLastPlayed)
    }
}
```

### Write-back unit tests — what to add

```go
// internal/itunes/writeback_test.go — additions

func TestWriteBack_MissingLibrary(t *testing.T) {
    opts := WriteBackOptions{
        LibraryPath: "/nonexistent/Library.xml",
        Updates:     []*WriteBackUpdate{{ITunesPersistentID: "ABC", NewPath: "/new/path.m4b"}},
    }
    _, err := WriteBack(opts)
    if err == nil {
        t.Fatal("WriteBack() expected error for missing library, got nil")
    }
}

func TestWriteBack_EmptyUpdates(t *testing.T) {
    _, err := WriteBack(WriteBackOptions{LibraryPath: "/some/path", Updates: nil})
    if err == nil {
        t.Fatal("WriteBack() expected error for empty updates, got nil")
    }
}

func TestValidateWriteBack_MissingPersistentID(t *testing.T) {
    // Create a minimal Library.xml in a temp dir, parse it, then validate
    // against an update referencing a non-existent persistent ID.
    // Expects a warning in the returned []string.
    // (Implementation requires a valid plist fixture file.)
}
```

### Integration test — full cycle

The integration test in `internal/itunes/integration_test.go` should cover:

1. Create a temporary directory with fake `.m4b` files.
2. Create a minimal `Library.xml` plist referencing those files.
3. Call `ValidateImport()` — assert FilesFound matches.
4. Call the full import flow (requires a real PebbleDB instance via
   `database.NewPebbleStore(t.TempDir())`).
5. Verify books appear in the store with correct iTunes fields.
6. Call `WriteBack()` with updated paths — verify the Library.xml was rewritten.
7. Verify backup was created if `CreateBackup` was true.

---

## Future: Bidirectional iTunes Sync (Post-MVP)

### Playcount Management

- Increment/decrement playcount buttons in Book Detail
- Keyboard shortcuts: `+` to increment, `-` to decrement
- Bulk playcount operations (mark all as played, reset)
- Playcount history tracking (when changed, old/new values)

### Bidirectional Playcount Sync

- Background job polling iTunes Library.xml for changes
- Detect playcount changes in iTunes → sync to audiobook-organizer
- Detect changes in audiobook-organizer → write back to iTunes
- Conflict resolution: prefer most recent, or sum counts
- Sync status and last sync time displayed in Settings
- Manual sync trigger button

**Dependencies**: Requires iTunes import + write-back (MVP phases above) to be
complete first.

---

## References

- iTunes fields on Audiobook model: `internal/models/audiobook.go`
- Manual QA guide: `docs/MANUAL_QA_GUIDE.md`
- MVP critical path: [`mvp-critical-path.md`](mvp-critical-path.md)
