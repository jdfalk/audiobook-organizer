<!-- file: CURRENT_SESSION.md -->
<!-- version: 1.1.0 -->
<!-- guid: 4f8cde73-8a89-4a1c-ba0d-6f2a781eced2 -->

# Current Development Session - November 21, 2025

## 🎯 IMMEDIATE PROBLEM TO FIX

We have **duplicate book records** and **missing metadata extraction**. The
library structure shows:

```text
library/Unknown Author/{series}/
├── My Quiet Blacksmith Life in Another World/
│   └── My Quiet Blacksmith Life in Another World - Unknown Author - read by {narrator}.m4b
├── Reborn as a Space Mercenary/
│   ├── Reborn as a Space Mercenary - Unknown Author - read by {narrator}.m4b
│   └── Reborn as a Space Mercenary - Unknown Author - read by narrator.m4b
```

### Critical Issues

1. **Template variables not replaced**: `{series}` and `{narrator}` in paths
   means metadata extraction completely failed
2. **Volume/Book numbers not detected**: Files contain "Vol. 01" but title
   doesn't reflect this
3. **Rich metadata ignored**: M4B files have perfect metadata in tags:
   - Album: "My Quiet Blacksmith Life in Another World, Vol. 01"
   - Performer: "Greg Chun" (narrator)
   - Composer: "Tamamaru" (author)
   - Publisher: "Podium Audio"
4. **Duplicate detection partially working**: Scanner created duplicates, some
   with proper paths, some with template variables

## 📝 WORK COMPLETED THIS SESSION (Last 4 Hours)

### 1. EventSource Connection Issues (REGRESSED ⚠️)

- **Initial Fix**: Broadcast now sends to clients with no subscriptions +
  frontend auto-reconnects
- **Observed Behavior (Nov 21 logs)**:
  - `/api/events` connections last **~17–18 seconds** before server closes the
    stream (`events.go:247` "connection closed" followed by 200 response with
    16–18s duration)
  - Browser console fills with `EventSource connection lost, reconnecting in
    3s...` while other fetches (`/status`, `/events`, `/health`) fail in lockstep
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

1. **Metadata extraction completely broken** - Returns empty/Unknown for all
   fields despite rich m4b metadata
2. **Volume numbers not extracted** - "Vol. 01" in filename/album not captured
   as series_position
3. **AI parsing not working** - Either not called or failing silently
4. **Template variables in organized paths** - `{series}` and `{narrator}`
   literals in filesystem
5. **Duplicate books created** - Hash-based detection added but untested
6. **EventSource reconnection loop** - `/api/events` drops frequently; need
  backoff + root-cause fix
7. **Health endpoint mismatch** - Frontend polls `/api/v1/health` but server
  only exposes `/api/health`, resulting in perpetual 404s and stuck "Attempt
  73" overlay even when backend is healthy
8. **Incorrect dashboard counts** - API still reports total books = 8
   (library+import). Need separate counts: `library_books`, `import_books`
   (unique) + update Dashboard + Library page. Also import path `total_size`
   returning negative numbers.

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

1. **DEBUG metadata extraction**:
   - Add extensive logging to `metadata.ExtractMetadata()`
   - Verify mediainfo is actually being called
   - Check what values are returned vs what's in the file
   - Test with the actual file:
     `/Users/jdfalk/Downloads/test_books/[PZG] My Quiet Blacksmith Life.../My Quiet Blacksmith Life... [PZG].m4b`

2. **Fix AI parsing integration**:
   - Verify OpenAI key is loaded from config
   - Add logging when AI parser is created/called
   - Check error handling - might be failing silently
   - Confirm AI is getting called when metadata incomplete

3. **Add volume number extraction**:
   - Create regex patterns for common volume formats
   - Extract to series_position field
   - Apply in both filename parsing AND album tag parsing

4. **Fix template replacement**:
   - Organizer should NEVER write literal `{template}` variables
   - Either replace with actual values or use defaults ("Unknown", "narrator",
     etc.)
   - Add validation before writing files

5. **Test duplicate detection**:
   - Delete corrupted database records
   - Run Full Rescan
   - Verify only 4 unique books created (not 8)
6. **Stabilize EventSource**:
   - Add exponential backoff (3s, 6s, 12s...) with cap + reset on success
   - Determine why `/api/events` closes after ~20 seconds (server timeout?
     proxy? heartbeat not read?)
   - Consider single shared EventSource across pages
7. **Expose accurate counts**:
   - Update backend stats endpoint(s) to return `library_book_count`,
     `import_book_count`, `total_book_count`
   - Fix import path size math (negative values)
   - Update Dashboard + Library UI to use new fields
8. **Fix health endpoint + overlay**:
   - Either add `/api/v1/health` route or change frontend polling to
     `/api/health`
   - Reconnect overlay should increment attempts only when fetch fails, and
     reload automatically when the health call succeeds

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
