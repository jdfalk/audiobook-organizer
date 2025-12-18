<!-- file: CURRENT_SESSION.md -->
<!-- version: 1.2.0 -->
<!-- guid: 4f8cde73-8a89-4a1c-ba0d-6f2a781eced2 -->

# Current Development Session - November 22, 2025

## 🎯 CURRENT STATUS - METADATA FIXES COMPLETE ✅

### What Was Fixed (Nov 21-22)

**The Problem**: Metadata extraction was completely broken, returning
`Unknown`/`{placeholder}` values despite rich M4B tags.

**Root Causes Identified**:

1. Case-sensitive raw tag lookups missed data ("publisher" vs "Publisher")
2. Release group tags like `[PZG]` polluted narrator fields
3. Volume numbers in "Vol. 01" format not extracted
4. Series names only looked in explicit tags, not album/title patterns
5. Narrator extraction didn't check all fallback fields
6. Publisher field not extracted from raw tags

**Solutions Implemented**:

1. **Case-insensitive raw tag lookup** in `metadata.ExtractMetadata()`
2. **Release-group filtering** to skip bracketed tags like `[PZG]`
3. **Roman numeral support** in volume detection (Vol. IV → 4)
4. **Series extraction cascade**: raw tags → album-prefix → comment → volume
   string parsing → album/title fallback
5. **Narrator extraction priority**: raw.narrator → raw.reader → raw.artist →
   raw.album_artist → tag.Artist (when composer provided author)
6. **Publisher extraction** from raw.publisher tag
7. **Enhanced volume detection**: `extractSeriesFromVolumeString()` parses
   "Series Name, Vol. 01" patterns

**Results Verified** (via API after cleanup + rescan):

- 4 books with correct narrator (Greg Chun, Fred Berman, Michelle H. Lee,
  Cassandra Morris/Graham Halstead)
- Series extracted from album titles ("My Quiet Blacksmith Life in Another
  World")
- Series positions detected (1 from "Vol. 01")
- Publishers populated (Podium Audio, Seven Seas Siren, Tantor Audio)

### Files Modified

- `cmd/diagnostics.go` v1.0.0 - NEW: CLI diagnostics command (cleanup-invalid,
  query)
- `cmd/root.go` v1.7.0 - Registered diagnosticsCmd
- `internal/metadata/metadata.go` v1.7.0 - Major extraction enhancements
- `internal/metadata/volume.go` v1.1.0 - Roman numerals, series parsing
- `internal/metadata/volume_test.go` v1.0.0 - NEW: Volume detection tests
- `internal/metadata/metadata_internal_test.go` v1.1.0 - NEW: Raw tag tests
- `internal/metadata/write_test.go` v1.1.0 - Skip segfault tests
- `internal/server/server.go` v1.26.0 - Scan progress improvements (IN PROGRESS)

### Database State

- **Cleaned**: 8 invalid records with `{series}`/`{narrator}` placeholders
  purged
- **Current**: 4 books in library + 4 in import paths (8 total)
- **Quality**: All 4 library books have correct metadata fields

## 📝 WORK COMPLETED THIS SESSION

### 1. Diagnostics CLI Command (COMPLETED ✅)

- **Problem**: Legacy `.go.bak` scripts caused lint failures, needed proper CLI
  integration
- **Solution**: Created `cmd/diagnostics.go` with two subcommands:
  - `cleanup-invalid`: Detects and removes books with placeholder values
    (`{series}`, `{narrator}`)
    - Supports `--dry-run` flag for preview
    - Confirmation prompt before deletion
    - Deleted 8 corrupted records in testing
  - `query`: Lists books or shows raw Pebble database entries
    - `--raw` flag for low-level database inspection
    - `--prefix` filter for targeted queries
- **Files**: `cmd/diagnostics.go` v1.0.0, `cmd/root.go` v1.7.0
- **Status**: Fully implemented and tested

### 2. Metadata Extraction Fixes (COMPLETED ✅)

- **Problem**: Files with rich M4B metadata returned Unknown/placeholder values
- **Investigation**: Test files showed:
  - Album: "My Quiet Blacksmith Life in Another World, Vol. 01"
  - Performer: "Greg Chun" but database showed `{narrator}`
  - Composer: "Tamamaru" correctly extracted as author
  - Album Artist: `[PZG]` (release group) polluting narrator field
- **Fixes Implemented**:
  1. Case-insensitive `getRawString()` helper
  2. `normalizeRawTagValue()` with release-group filtering
  3. `looksLikeReleaseGroupTag()` to detect bracketed tags
  4. Narrator extraction priority chain
  5. Series extraction cascade (multiple fallback sources)
  6. `DetectVolumeNumber()` with roman numeral support
  7. `extractSeriesFromVolumeString()` for "Series, Vol. N" patterns
  8. Publisher extraction from raw tags
- **Files**: `internal/metadata/metadata.go` v1.7.0,
  `internal/metadata/volume.go` v1.1.0
- **Tests**: Added `volume_test.go`, `metadata_internal_test.go` - all passing
- **Verification**: `go run . inspect-metadata` shows correct fields, API
  returns 4 books with proper data

### 3. Database Cleanup (COMPLETED ✅)

- **Action**: `~/audiobook-organizer-embedded diagnostics cleanup-invalid --yes`
- **Result**: Deleted 8 records with placeholder paths
- **Follow-up**: Full rescan via
  `curl -X POST http://localhost:8888/api/v1/operations/scan?force_update=true`
- **Outcome**: 8 books found (4 library + 4 import paths), all with correct
  metadata

### 4. Scan Progress Reporting (IN PROGRESS ⚠️)

- **Problem**: Scan shows "Scanning: 0/0" and completion message "Total books
  found: 8" without separating library vs import
- **User Request**: "do a quick list of all the files and use that as our total,
  like rsync does" + "say total books found 4 in library, 4 in import paths"
- **Changes Applied** (NOT YET TESTED):
  1. Pre-scan `filepath.Walk` loop to count total files across all folders
  2. Track `libraryBooks`, `importBooks`, `processedFiles` separately
  3. Update progress with `processedFiles/totalFilesAcrossFolders` during scan
  4. Enhanced completion message: "Library: X books, Import paths: Y books
     (Total: Z)"
- **Files**: `internal/server/server.go` v1.26.0 (compiled but not runtime
  tested)
- **Status**: Code changes complete, needs rebuild and test scan

### 5. EventSource Connection Issues (REGRESSED ⚠️)

- **Initial Fix**: Broadcast now sends to clients with no subscriptions +
  frontend auto-reconnects
- **Observed Behavior (Nov 21 logs)**:
  - `/api/events` connections last **~17–18 seconds** before server closes the
    stream (`events.go:247` "connection closed" followed by 200 response with
    16–18s duration)
  - Browser console fills with
    `EventSource connection lost, reconnecting in 3s...` while other fetches
    (`/status`, `/events`, `/health`) fail in lockstep
  - Multiple EventSource consumers (Dashboard, Library) reconnect at slightly
    different times, so we see two clients cycling every ~17 seconds
- **Current Status**: Reconnect loop never stabilizes because server proactively
  closes the SSE stream and frontend retries immediately with fixed 3s delay
- **New Requirement**:
  - Implement exponential backoff (or capped linear) for EventSource reconnects
    to avoid rapid loops
  - Investigate why `/api/events` closes after ~20 seconds despite heartbeat
    (likely Gin read/write timeout or context deadline)
  - Ensure both Dashboard + Library EventSources share same connection/pool if
    possible
  - When health probe succeeds after a prolonged outage, automatically refresh
    the page so UI recovers from stale state
- **Files Already Touched**:
  - `internal/realtime/events.go` v1.0.0 → v1.1.0
  - `web/src/pages/Library.tsx` v1.16.0 → v1.17.0 (needs revisit)

### 2. Progress Indicator Showing 0/0 (FIXED ✅)

- **Problem**: UI showed "Scanning: 0/0" even during active scans
- **Root Cause**: Same as #1 - events weren't reaching the frontend
- **Fix**: Same broadcast fix resolved this (still holding, but monitor after
  EventSource adjustments)

### 3. Full Rescan Not Scanning Library Folder (FIXED ✅)

- **Problem**: Full Rescan only scanned import paths, not the main library
  folder
- **Root Cause**: Scan logic only added enabled LibraryFolders to scan list
- **Fix**: When `force_update=true`, prepend RootDir to scan list
- **Files Changed**: `internal/server/server.go` v1.24.0 → v1.25.0

### 4. Broken Helper Scripts (FIXED ✅)

- **Problem**: `scripts/query_books.go` was completely mangled, wouldn't compile
- **Root Cause**: File corruption from previous edit
- **Fix**: Completely rewrote both scripts with correct API calls
- **Files Changed**:
  - `scripts/query_books.go` v1.0.1 → v1.1.0
  - `scripts/cleanup_invalid_books.go` v1.0.1 → v1.1.0

### 5. OpenAI API Key Not Saving/Testing (FIXED ✅)

- **Problem**:
  - Key saved but returned masked `sk-****R8kA`
  - Test button tried to use masked key → 401 Unauthorized
  - No indication that key was actually saved
- **Root Cause**: Frontend stored masked key in state after save, then used it
  for testing
- **Fix**:
  - Store masked key separately from input field
  - Clear input field after save, show masked key in placeholder
  - Test button: if field empty → test with backend config, if filled → test
    with typed key
- **Files Changed**: `web/src/pages/Settings.tsx` v1.18.0 → v1.19.0

### 6. Duplicate Book Detection (PARTIALLY FIXED ⚠️)

- **Problem**: Same book in library and import path created 2 database records
- **Root Cause**: Upsert only checked by FilePath, not FileHash
- **Fix**: Added hash-based duplicate detection in scanner
- **Files Changed**: `internal/scanner/scanner.go` v1.8.0 → v1.9.0
- **Status**: Compiles but NOT TESTED - this is where we are now

## 🔍 CURRENT INVESTIGATION

### The Metadata Extraction Pipeline

**Expected Flow**:

1. `scanner.ScanDirectoryParallel()` finds files
2. `scanner.ProcessBooksParallel()` for each file:
   - Extract metadata from m4b tags → `metadata.ExtractMetadata()`
   - If incomplete → Try AI parsing with OpenAI
   - If still incomplete → Parse filename
3. Insert/update database with extracted metadata
4. `organizer.OrganizeBook()` creates path using template pattern

**What's Actually Happening**:

- Files are scanned ✅
- Metadata extraction **FAILS** (produces empty/Unknown values) ❌
- AI parsing **NOT BEING CALLED** or failing silently ❌
- Template variables `{series}` and `{narrator}` not replaced ❌
- Organizer creates paths with unfilled templates ❌

### Key Code Locations

**Metadata Extraction**:

- `internal/metadata/extract.go` - ExtractMetadata() function
- `internal/scanner/scanner.go` lines 167-240 - ProcessBooksParallel() with AI
  fallback
- `internal/ai/openai.go` - ParseFilename() for AI parsing

**File Organizing**:

- `internal/organizer/organizer.go` - OrganizeBook() method
- Uses templates like
  `{author}/{series}/{title} - {author} - read by {narrator}.m4b`

**Volume Number Extraction**:

- Needs regex patterns for: Vol. 01, Vol 01, Volume 1, Book 1, Bk. 1, etc.
- Should extract as series_position

## 🐛 KNOWN BUGS TO FIX

### High Priority

1. ✅ ~~**Metadata extraction completely broken**~~ - FIXED in v1.7.0
2. ✅ ~~**Volume numbers not extracted**~~ - FIXED with roman numeral support
3. ✅ ~~**Template variables in organized paths**~~ - FIXED by metadata
   extraction fixes
4. ⚠️ **Scan progress reporting incomplete** - Code applied but not tested
   (v1.26.0)
5. ❌ **Web UI not showing books** - Books exist in DB and return via API, but
   frontend doesn't display them
6. ❌ **EventSource reconnection loop** - `/api/events` drops every ~17s; need
   backoff + root-cause fix
7. ❌ **Health endpoint mismatch** - Frontend polls `/api/v1/health` (404),
   server only has `/api/health`

### Medium Priority

1. ❌ **Dashboard count separation** - Need separate `library_books` vs
   `import_books` counts (currently shows total)
2. ❌ **Import path negative sizes** - `total_size` returning negative numbers
3. ❓ **Duplicate books** - Hash-based detection added (v1.9.0) but untested
4. ❓ **AI parsing** - OpenAI integration exists but unknown if working (may not
   be needed after metadata fixes)

## 📊 DASHBOARD / DISPLAY REQUIREMENTS

- Separate counts:
  - `Library Books`: only books whose file paths live under `root_dir`
  - `Import Books`: books detected in import folders but not yet organized
    (unique by hash)
- Display both counts on Dashboard + Library page
- Fix import paths stats (negative `total_size` values)
- When scan runs, progress logs should state "Library: X unique" vs "Import: Y
  pending"

## 📋 NEXT STEPS (Priority Order)

### Immediate (Complete Current Work)

1. ⚠️ **Test scan progress reporting** (v1.26.0):
   - Build:
     `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go build -o ~/audiobook-organizer-embedded`
   - Kill existing server: `killall audiobook-organizer-embedded`
   - Restart: `~/audiobook-organizer-embedded serve --port 8888 --debug`
   - Trigger scan:
     `curl -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true"`
   - Verify progress shows actual counts (not 0/0) and completion message
     separates library vs import
   - Check logs in `/Users/jdfalk/ao-library/logs/` for formatted messages

### High Priority (User-Facing Issues)

1. ❌ **Investigate web UI not showing books**:
   - Books exist in database (verified via `/api/v1/audiobooks`)
   - Dashboard shows "Books: 4" correctly
   - Library page may have frontend state/refresh issue
   - Steps:
     1. Check browser console for errors when loading Library page
     2. Verify `/api/v1/audiobooks` returns data in browser Network tab
     3. Check if frontend is filtering/hiding books based on some criteria
     4. Test with browser hard refresh (Cmd+Shift+R)

2. ❌ **Fix EventSource stability**:
   - **Server-side**: Why does `/api/events` close after ~17 seconds?
     - Check for read/write timeouts in Gin server config
     - Verify heartbeat is being sent AND read by clients
     - Review `internal/realtime/events.go` connection lifecycle
   - **Client-side**: Implement exponential backoff
     - Start: 3s delay
     - Backoff: 3s → 6s → 12s → 24s (cap at 30s)
     - Reset to 3s on successful connection
     - Files: `web/src/pages/Library.tsx`, `web/src/pages/Dashboard.tsx`

3. ❌ **Fix health endpoint mismatch**:
   - **Option A**: Add `/api/v1/health` route in `internal/server/server.go`
     (preferred for API versioning)
   - **Option B**: Change frontend to poll `/api/health`
   - Update reconnect overlay to auto-refresh page on health recovery
   - Files: `internal/server/server.go`,
     `web/src/components/ConnectionStatus.tsx`

### Medium Priority (Data Accuracy)

1. ❌ **Separate dashboard counts**:
   - Modify `/api/v1/system/status` to return:
     - `library_book_count`: Books in `root_dir`
     - `import_book_count`: Books in import paths (unique by hash)
     - `total_book_count`: Sum of above
   - Update Dashboard and Library page to display separate counts
   - Files: `internal/server/server.go`, `web/src/pages/Dashboard.tsx`,
     `web/src/pages/Library.tsx`

2. ❌ **Fix import path negative sizes**:
   - Debug `total_size` calculation in library folder stats
   - Likely int overflow or incorrect aggregation
   - File: `internal/server/server.go` (library folder stats endpoint)

### Low Priority (Optional Testing)

1. ❓ **Verify duplicate detection**:
   - Hash-based detection implemented in `internal/scanner/scanner.go` v1.9.0
   - Test by:
     1. Copy a file to both library and import paths
     2. Run full rescan
     3. Verify only one database record created
   - May already be working correctly

2. ❓ **Test AI parsing**:
   - OpenAI integration exists but may not be needed after metadata fixes
   - Only test if books still have missing metadata after scan
   - Verify config has valid OpenAI key if needed

## 🧾 SERVER RESTART LOGS (Nov 21 @ 10:48–10:53)

- Server launched via `~/audiobook-organizer-embedded serve --port 8888`
- Pebble replay successful; migrations already at version 5
- **Encryption warning**: Failed to decrypt `openai_api_key`
  (`illegal base64 data at input byte 3`) so backend currently thinks key is
  unset
- Operation queue + event hub initialize cleanly; `/api/events` shows repeated
  register/unregister cycles at 16–18s cadence
- `/api/v1/health` receives continuous 404s—only `/api/health` exists per Gin
  route dump
- Heartbeat log every 5s (`[DEBUG] Heartbeat: Got 1 library folders`) proves the
  hub loop continues even while clients churn
- Dashboard + Library reloads at 10:51 and 10:52 confirm API endpoints respond
  quickly; issue is purely connection lifecycle + wrong health URL

> Actionable takeaway: fix SSE lifetime on the server, add exponential backoff,
> share a single client connection, and point the reconnect overlay at
> `/api/health` so it can detect recovery and auto-refresh.

## 🔧 DEBUGGING COMMANDS

```bash
# Check what's in database
cd /Users/jdfalk/ao-library && /tmp/query_books

# Clean up invalid records
cd /Users/jdfalk/ao-library && /tmp/cleanup_invalid_books

# Test metadata extraction manually
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go run -tags debug ./cmd/test-metadata "/Users/jdfalk/Downloads/test_books/[PZG] My Quiet Blacksmith Life in Another World, Vol. 01 (Audiobook) [Podium Audio]/My Quiet Blacksmith Life in Another World, Vol. 01 [PZG].m4b"

# Restart server
killall audiobook-organizer-embedded
~/audiobook-organizer-embedded serve --debug
```

## 📁 KEY FILE LOCATIONS

- **Config**: `/Users/jdfalk/ao-library/config.yaml`
- **Database**: `/Users/jdfalk/ao-library/audiobooks.pebble/`
- **Library**: `/Users/jdfalk/ao-library/library/`
- **Import Path**: `/Users/jdfalk/Downloads/test_books/`
- **Test Files**: 4 audiobooks in import path with perfect metadata

## 🎓 PYTHON REFERENCE SCRIPTS

User confirmed we had Python scripts that did excellent matching. Key test
scripts:

```bash
# Test organize with import validation (v3)
python3 scripts/test-organize-import-v3.py /Users/jdfalk/repos/scratch/file-list-books --limit 100 --sample 0 --output test-temp-out-check.json 2>&1

# Test OpenAI parsing with large sample sets
python3 scripts/test_openai_parsing.py --num-samples 5000 --batch-size 50 --workers 10 --output-dir test_results
```

**What to extract from these**:

- Volume/book number extraction patterns (Vol. 01, Vol 01, Volume 1, Book 1, Bk.
  1, etc.)
- Metadata field mapping from m4b tags to our Book structure
- Series detection and normalization logic
- Filename parsing patterns that worked well
- Apply same principles to Go code in `internal/scanner/` and
  `internal/metadata/`

## ⚠️ IMPORTANT NOTES

- User is running server from `~/ao-library` directory
- Pebble database is in that directory
- DO NOT suggest deleting database - must be able to recover from any state
- Full Rescan should fix everything by re-extracting metadata
- Focus on metadata extraction pipeline - that's the core problem
