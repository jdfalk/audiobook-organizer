<!-- file: HANDOFF.md -->
<!-- version: 2.0.0 -->
<!-- guid: 9a8b7c6d-5e4f-3d2c-1b0a-9f8e7d6c5b4a -->

# LibraryFolder → ImportPath Refactoring Handoff

**Date**: November 23, 2025
**Project**: audiobook-organizer
**Task**: Comprehensive API and Type Rename
**Branch Strategy**: Work in separate branch, rebase periodically against main

## 🎯 Executive Summary

This document provides complete instructions for renaming `LibraryFolder` to `ImportPath` throughout the entire codebase to resolve confusing terminology where "library folders" actually refers to "import paths" (monitored folders for new content, NOT the main library).

**Scope**: ~150+ occurrences across Go backend, TypeScript frontend, database schemas, API endpoints, tests, and documentation.

**Estimated Effort**: Large refactoring requiring careful attention to maintain functionality.

---

## 📚 Glossary and Terminology

### Current Terminology (Established in README.md)

**Library / Library Folder / Library Path**:

- The main root directory (`root_dir`) configured in `config.yaml`
- Where audiobooks are permanently stored in organized structure
- Example: `/Users/jdfalk/ao-library/library/`
- This is your primary, organized collection
- NOT stored in the `library_folders` database table

**Import Path / Import Folder / Monitored Folder**:

- External directories scanned for new audiobook files
- NOT part of your organized library
- Temporary staging locations (like Downloads folders)
- Where the app looks for new content to import into library
- THESE are what's stored in `library_folders` table (confusing name!)
- Example: `/Users/jdfalk/Downloads/test_books`

**The Problem**:
The database table `library_folders` and Go type `LibraryFolder` actually store *import paths*, not library folders. This creates significant confusion in the codebase and API.

**The Solution**:
Rename everything from `LibraryFolder`/`library_folders`/`library/folders` to `ImportPath`/`import_paths`/`import-paths` for clarity and consistency.

---

## 🔄 Periodic Rebasing Requirement

**CRITICAL**: To minimize merge conflicts when this work is complete, you MUST periodically rebase your working branch against `main`.

**Recommended Schedule**:

- Rebase at the end of each major section (database, server, frontend)
- Rebase at least once per day during active development
- Rebase immediately before submitting final PR

**Rebase Commands**:

```bash
# Update main
git checkout main
git pull origin main

# Rebase your branch
git checkout your-refactoring-branch
git rebase main

# Resolve conflicts if any
# ... fix conflicts ...
git rebase --continue

# Force push (your branch only!)
git push --force-with-lease origin your-refactoring-branch
```

---

## 🏗️ Architecture Context

### Database Layer

- **PebbleDB** (primary): Key-value store with JSON serialization
  - Keys: `library:<id>` → will become `import_path:<id>`
  - Counter: `counter:library` → will become `counter:import_path`
- **SQLite** (alternative): Relational store with SQL schemas
  - Table: `library_folders` → will become `import_paths`
  - Index: `idx_library_folders_path` → will become `idx_import_paths_path`

### API Layer

- **Endpoints**: `/api/v1/library/folders` → will become `/api/v1/import-paths`
- **HTTP Methods**: GET (list), POST (create), DELETE (remove)
- **Handler Functions**: `listLibraryFolders` → will become `listImportPaths`

### Frontend Layer

- **TypeScript Interface**: `LibraryFolder` → will become `ImportPath`
- **API Client**: `getLibraryFolders()` → will become `getImportPaths()`
- **React Components**: References in Settings, FileManager, Library, Dashboard

---

## 🎯 Where We Are

### ✅ Major Accomplishments (Nov 21-22)

**Metadata Extraction - FIXED**: The core problem was solved. Books were showing `{narrator}` and `{series}` placeholders because tag extraction was completely broken. Now working perfectly.

**What Was Fixed**:
1. Case-insensitive raw tag lookups (was missing "Publisher" when looking for "publisher")
2. Release-group tag filtering (skips bracketed tags like `[PZG]`)
3. Roman numeral volume detection (Vol. IV → 4)
4. Series extraction from multiple sources (tags, album patterns, title patterns)
5. Narrator extraction priority chain (raw.narrator → raw.reader → raw.artist → raw.album_artist → tag.Artist)
6. Publisher extraction from raw tags

**Diagnostics CLI - NEW**: Created `diagnostics` command with:
- `cleanup-invalid`: Finds and removes books with placeholder values
- `query`: Lists books or shows raw Pebble database entries

**Database - CLEANED**: Purged 8 corrupted records, rescanned to get clean data.

**Scan Progress - IMPLEMENTED BUT NOT TESTED**: Added code to:
- Pre-scan all folders to count total files (like rsync)
- Track library vs import book counts separately
- Show "Scanning: X/Y files" instead of "0/0"
- Display "Library: 4 books, Import paths: 4 books (Total: 8)" on completion

### 🔴 What's Broken

1. **Web UI not showing books** - Books exist in DB and return via API, but Library page is empty
2. **EventSource drops every ~17 seconds** - `/api/events` connection closes, causing reconnect loop
3. **Health endpoint 404** - Frontend polls `/api/v1/health` but server only has `/api/health`
4. **Scan progress not tested** - Code is in `internal/server/server.go` v1.26.0 but needs rebuild and test

## 📋 What To Do Next

### Step 1: Test Scan Progress (5 minutes)

```bash
# Build
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go build -o ~/audiobook-organizer-embedded

# Restart server
killall audiobook-organizer-embedded
~/audiobook-organizer-embedded serve --port 8888 --debug

# Trigger scan
curl -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true"

# Check logs
tail -f /Users/jdfalk/ao-library/logs/latest.log
```

**Expected**: Progress shows "Scanning: 4/8 files" instead of "0/0", completion message shows "Library: 4 books, Import paths: 4 books (Total: 8)"

### Step 2: Fix Web UI Display (15-30 minutes)

**Problem**: Books show in API but not in frontend Library page.

**Investigation Steps**:
1. Open http://localhost:8888 in browser
2. Go to Library page
3. Open browser console (F12) - look for errors
4. Check Network tab - does `/api/v1/audiobooks` return data?
5. Hard refresh (Cmd+Shift+R)

**Likely Causes**:
- Frontend is filtering books based on some criteria
- State management issue (books loaded but not rendered)
- API response format changed and frontend expects old format

**Files to Check**:
- `web/src/pages/Library.tsx` - book listing logic
- `web/src/api/api.ts` - API client
- Browser console for React errors

### Step 3: Fix EventSource Stability (30-60 minutes)

**Problem**: `/api/events` connection closes after ~17 seconds, causing constant reconnects.

**Server-Side Investigation** (`internal/realtime/events.go`):
- Check for read/write timeouts in Gin server config
- Verify heartbeat is being sent correctly
- Look for context deadline or connection lifecycle issues

**Client-Side Fix** (`web/src/pages/Library.tsx`, `web/src/pages/Dashboard.tsx`):
- Implement exponential backoff: 3s → 6s → 12s → 24s (cap at 30s)
- Reset to 3s on successful connection
- Consider single shared EventSource across pages

### Step 4: Fix Health Endpoint (10 minutes)

**Option A** (Preferred): Add `/api/v1/health` route in `internal/server/server.go`

```go
// In setupRoutes():
v1.GET("/health", s.healthCheck)
```

**Option B**: Change frontend to poll `/api/health` instead

**Also Update**: `web/src/components/ConnectionStatus.tsx` to auto-refresh page when health check succeeds after failure

## 🗂️ Key Files Reference

### Backend (Go)
- `cmd/diagnostics.go` v1.0.0 - CLI diagnostics commands
- `internal/metadata/metadata.go` v1.7.0 - Metadata extraction logic
- `internal/metadata/volume.go` v1.1.0 - Volume/series detection
- `internal/server/server.go` v1.26.0 - HTTP API server, scan operation
- `internal/realtime/events.go` v1.1.0 - SSE event broadcasting

### Frontend (TypeScript/React)
- `web/src/pages/Library.tsx` v1.17.0 - Library page with book list
- `web/src/pages/Dashboard.tsx` - Dashboard with system stats
- `web/src/components/ConnectionStatus.tsx` - Reconnect overlay
- `web/src/api/api.ts` - API client

### Test Commands
```bash
# List books via API
curl http://localhost:8888/api/v1/audiobooks | jq

# Check system status
curl http://localhost:8888/api/v1/system/status | jq

# Diagnostics - list books
~/audiobook-organizer-embedded diagnostics query

# Diagnostics - cleanup invalid (dry run)
~/audiobook-organizer-embedded diagnostics cleanup-invalid

# Inspect single file metadata
go run . inspect-metadata "/path/to/file.m4b"
```

## 📊 Current Database State

- **Location**: `/Users/jdfalk/ao-library/audiobooks.pebble/`
- **Books**: 8 total (4 in library, 4 in import paths)
- **Metadata Quality**: All 4 library books have correct narrator, series, publisher, series_position
- **Clean**: No corrupted/placeholder records

## 🎓 Context for AI

**User Environment**:
- Server running from: `/Users/jdfalk/ao-library`
- Library folder: `/Users/jdfalk/ao-library/library/`
- Import path: `/Users/jdfalk/Downloads/test_books/`
- Binary location: `~/audiobook-organizer-embedded`
- Port: 8888

**Testing Workflow**:
1. Make code changes
2. Build: `go build -o ~/audiobook-organizer-embedded`
3. Restart: `killall audiobook-organizer-embedded && ~/audiobook-organizer-embedded serve --port 8888 --debug`
4. Test via curl or web UI at http://localhost:8888

**Key Principles**:
- Never suggest deleting database - must recover from any state
- Always use VS Code tasks when available (see `.vscode/tasks.json`)
- Follow file header conventions (file path, version, guid)
- Increment version on every file change
- Add comprehensive tests for new functionality

## 🔗 Related Documentation

- `CURRENT_SESSION.md` - Detailed session notes with full context
- `TODO.md` - Project-wide TODO list with priorities
- `AGENTS.md` - Pointer to `.github/` instructions
- `.github/copilot-instructions.md` - Coding standards and workflow
- `.github/instructions/` - Language-specific coding rules

## ⚡ Quick Wins Available

1. **Health endpoint** - 10 minutes, unblocks reconnect overlay
2. **Dashboard count separation** - 20 minutes, improves data visibility
3. **EventSource backoff** - 15 minutes client-side only, reduces log spam

Start with testing scan progress (Step 1), then tackle web UI display issue (Step 2) as it's user-facing and critical.
