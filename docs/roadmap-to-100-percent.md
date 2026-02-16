<!-- file: docs/roadmap-to-100-percent.md -->
<!-- version: 1.3.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890 -->
<!-- last-edited: 2026-02-16 -->

# Roadmap to 100% — Everything Needed for Production Release

**Date**: February 16, 2026
**Current state**: ~98% MVP complete, 81.3% Go test coverage, core features implemented and stabilized
**Goal**: Production-ready, publicly releasable, rock-solid audiobook organizer

## Quick Status Snapshot (February 16, 2026)

Use this section for fast status checks.

- ✅ 1. Critical security scan/guardrails in place
- ⚠️ 1.1 Secret history scrub + key rotation still pending (manual Git history operation)
- ✅ 2. Auth/session stack implemented and tested (backend + frontend + E2E)
- ✅ 3. Frontend critical pages implemented (`Login`, `Works`, search/nav integration, error boundaries, theme toggle)
- ✅ 4. Backend hardening completed (config validation, request size limits, TLS fallback, stale ops, scanner loop protection)
- ✅ 5. Build/deploy alignment completed + binary smoke workflow added
- ⚠️ 6. Manual QA checklist execution still pending
- ⚠️ 6. Full E2E breadth expansion still pending
- ⚠️ 7. Full OpenAPI completion still pending
- ⚠️ 8-10. Optional performance/polish/release automation improvements remain

## Progress Update (February 16, 2026)

Implemented in this pass:

- Auth middleware, auth endpoints (`status/setup/login/me/logout/sessions`), and session cleanup wiring
- API and auth rate limiting middleware with configurable limits
- Request body size limits for JSON and upload endpoints
- Config validation and runtime config update hardening with persistence
- HTTPS startup degradation to HTTP-only when TLS files are missing
- Organizer temp-file writes with cleanup on startup and failures
- Scanner inode tracking to prevent symlink loop traversal
- Stale operation detection endpoint and timeout-based failure handling
- Storage usage endpoint (`GET /api/v1/system/storage`) and frontend quota data integration
- Login UI implementation with bootstrap admin setup and authenticated route gating
- Top bar search routing integration and user logout action
- Build/deploy alignment updates (`Dockerfile`, `Dockerfile.test`, coverage thresholds)
- New deployment artifacts (`docker-compose.yml`, `.env.example`, launchd plist, `install.sh`)
- New docs (`docs/qa-checklist.md`, `docs/architecture.md`)
- Middleware unit tests added for auth, rate limiting, and request size limits
- Scanner persistence race hardening for concurrent scans (conflict-aware get-or-create)
- Auth E2E flow coverage added (bootstrap setup, login redirect, invalid credentials)
- Works page replaced with live data view backed by `/api/v1/works`
- New configuration reference (`docs/configuration.md`)
- New CI binary smoke workflow (`.github/workflows/binary-smoke.yml`)

Still pending for full roadmap completion:

- Git history scrubbing for previously committed secrets
- Full OpenAPI completion for all endpoints
- Expanded frontend/E2E coverage goals and full manual QA execution
- Remaining polish/performance optional items (FTS, cache, shortcut system, etc.)

This document covers EVERY remaining task needed to make this application perfect. An AI agent or developer should be able to use this document alone to bring the project to completion.

---

## Table of Contents

1. [CRITICAL SECURITY (Do First)](#1-critical-security)
2. [Authentication & Authorization](#2-authentication--authorization)
3. [Frontend Completion](#3-frontend-completion)
4. [Backend Hardening](#4-backend-hardening)
5. [Build & Deployment Fixes](#5-build--deployment-fixes)
6. [Testing & QA](#6-testing--qa)
7. [Documentation](#7-documentation)
8. [Performance & Scalability](#8-performance--scalability)
9. [Polish & UX](#9-polish--ux)
10. [Release Pipeline](#10-release-pipeline)

---

## 1. CRITICAL SECURITY

### 1.1 Rotate Exposed OpenAI API Key
**Priority**: IMMEDIATE
**Files**: `.env`, git history
**Detail**: The `.env` file contains a real OpenAI API key (`sk-svcacct-...`) that was committed to git. Even though `.env` is in `.gitignore` now, it exists in git history.

**Steps**:
1. Go to OpenAI dashboard, revoke the current key, generate a new one
2. Use `git filter-repo` or BFG to scrub `.env` from entire git history:
   ```bash
   # From the repo root (already have filter-repo set up at .git/filter-repo)
   git filter-repo --path .env --invert-paths
   ```
3. Force-push cleaned history (coordinate with any collaborators)
4. Create `.env.example` with placeholder values:
   ```env
   # OpenAI API Configuration (optional, for AI-powered metadata)
   OPENAI_API_KEY=your_api_key_here
   ENABLE_AI_PARSING=false
   OPENAI_MODEL=gpt-4o-mini
   ```
5. Verify `.env` is in `.gitignore` (it already is at line 171)

### 1.2 Audit for Other Secrets
**Files**: All config files, test fixtures
**Detail**: Scan entire repo for any other leaked credentials, API keys, or tokens.

**Steps**:
1. Run: `grep -rn "sk-\|api[_-]key\|password.*=\|secret.*=" --include="*.go" --include="*.ts" --include="*.yml" --include="*.json" . | grep -v node_modules | grep -v _test.go | grep -v mock`
2. Check `internal/config/config.go` for any hardcoded defaults that look like real credentials
3. Verify test fixtures use obviously-fake values

---

## 2. Authentication & Authorization

### 2.1 Basic HTTP Authentication
**Priority**: P0 (blocks public release)
**Files to create/modify**:
- `internal/server/middleware/auth.go` (NEW) — auth middleware
- `internal/server/middleware/auth_test.go` (NEW) — tests
- `internal/server/server.go` — wire auth middleware into router
- `internal/config/config.go` — add auth config fields
- `web/src/services/api.ts` — add auth headers to all requests
- `web/src/pages/LoginPage.tsx` — implement real login form
- `web/src/contexts/AuthContext.tsx` (NEW) — auth state management

**Design**:
The database already has `users` and `sessions` tables (migration 16) with full CRUD methods. The backend Store interface already has: `CreateUser`, `GetUserByUsername`, `CreateSession`, `GetSession`, `RevokeSession`. Use these.

**Implementation**:

#### Backend Auth Middleware (`internal/server/middleware/auth.go`):
```go
// Pattern: Check for session cookie or Bearer token
// 1. Extract token from Cookie "session_id" or Authorization header
// 2. Call store.GetSession(token) — already implemented
// 3. If valid and not expired, set user in context
// 4. If invalid, return 401
// 5. Apply to all /api/v1/* routes except /api/v1/auth/*
```

#### Auth Endpoints (add to `internal/server/server.go` routes):
- `POST /api/v1/auth/login` — accepts `{username, password}`, calls `store.GetUserByUsername()`, verifies bcrypt hash, creates session via `store.CreateSession()`, returns session token in cookie + body
- `POST /api/v1/auth/logout` — calls `store.RevokeSession()`
- `GET /api/v1/auth/me` — returns current user info
- `POST /api/v1/auth/setup` — first-run only: creates initial admin user (only works if zero users exist)

#### Password hashing:
- Use `golang.org/x/crypto/bcrypt` (already in go.sum as transitive dep)
- Hash on create, verify on login
- The `User` struct in `internal/database/store.go` needs a `PasswordHash string` field added

#### Frontend auth flow:
- `LoginPage.tsx` already exists as a stub — implement real login form
- On app load, call `GET /api/v1/auth/me`. If 401, redirect to login
- Store session in cookie (httpOnly, secure, sameSite=strict)
- Add auth header interceptor in `api.ts` axios instance

#### First-run experience:
- If no users exist in DB, the welcome wizard should include an "Create Admin Account" step
- `POST /api/v1/auth/setup` only works when user count is 0
- After setup, redirect to login

### 2.2 Session Management
**Files**: Already implemented in `internal/database/sqlite_store.go`
**Detail**: The store methods exist. Just need to wire them into the auth middleware.

**Existing methods**:
- `CreateSession(session *Session) error` — insert with TTL
- `GetSession(token string) (*Session, error)` — lookup + expiry check
- `RevokeSession(token string) error` — delete
- `ListUserSessions(userID string) ([]*Session, error)` — list all for user

**Additional work**:
- Add session cleanup goroutine (delete expired sessions periodically)
- Add "active sessions" view in Settings page so user can see/revoke sessions

---

## 3. Frontend Completion

### 3.1 Fix Login Page
**File**: `web/src/pages/LoginPage.tsx`
**Current state**: Stub with placeholder text
**Needed**: Real login form with username/password fields, error handling, redirect on success

### 3.2 Fix Works Page
**File**: `web/src/pages/WorksPage.tsx`
**Current state**: Stub with placeholder content
**Needed**: Either implement as a "series/collections" view or remove from navigation if not needed. Decide: is this for grouping books into series? If so:
- Display series grouped by author
- Allow drag-and-drop reordering
- Show series progress (books read/total)

### 3.3 Dark Mode Toggle
**Files**:
- `web/src/contexts/ThemeContext.tsx` — already exists with theme switching logic
- `web/src/components/layout/Navbar.tsx` — add toggle button
**Detail**: ThemeContext already supports dark/light modes. Just need a toggle button in the navbar (sun/moon icon). Implementation is ~10 lines: import `useTheme` hook, add IconButton with `Brightness4`/`Brightness7` icon, call `toggleTheme()` on click.

### 3.4 QuotaTab Real Data
**File**: `web/src/components/settings/QuotaTab.tsx`
**Current state**: Hardcoded sample data (totalSpace: 500GB, usedSpace: 234.5GB, etc.)
**Needed**:
- Add backend endpoint `GET /api/v1/system/storage` that returns disk usage for the library directory
- Go implementation: use `syscall.Statfs` (Linux/Mac) to get filesystem stats
- Replace hardcoded values with API call results

### 3.5 Search Integration
**Files**:
- `web/src/components/layout/Navbar.tsx` — has search input
- `web/src/services/api.ts` — has `searchBooks()` function
**Detail**: The search bar in the navbar exists but doesn't trigger navigation or display results. Wire it to:
1. On Enter or debounced typing, call `api.searchBooks(query)`
2. Navigate to `/library?search=query` or show dropdown results
3. The backend `GET /api/v1/audiobooks?search=term` already supports search

### 3.6 Remove Unused API Functions
**File**: `web/src/services/api.ts`
**Detail**: Several exported functions are never imported anywhere:
- Audit all exports, remove any that aren't used by any component
- This is cleanup only, no functionality change

### 3.7 Error Boundaries
**Files**: `web/src/components/ErrorBoundary.tsx` (may need creation)
**Detail**: No React error boundaries exist. If a component throws, the entire app crashes. Add:
- A top-level ErrorBoundary wrapping the app
- Display a "Something went wrong" message with a reload button
- Log error details to console

### 3.8 Loading States
**Files**: Various pages
**Detail**: Some pages don't show loading indicators when fetching data. Audit each page component and ensure:
- Skeleton loaders or spinners during initial data fetch
- Error states when API calls fail
- Empty states when no data exists ("No audiobooks found. Import some to get started!")

---

## 4. Backend Hardening

### 4.1 Config Validation
**File**: `internal/config/config.go`
**Detail**: No validation on config values. Add a `Validate()` method to `Config` struct:

```go
func (c *Config) Validate() error {
    // Port range: 1-65535
    // Paths: verify parent directory exists
    // Workers: must be >= 1
    // Timeouts: must be >= 0
    // Naming pattern: validate template syntax
    // Database path: verify parent directory is writable
}
```

Call `Validate()` after loading config in `cmd/root.go` and in the settings API endpoint.

### 4.2 Graceful HTTPS Degradation
**File**: `internal/server/server.go`
**Current behavior**: If TLS cert files don't exist, the server calls `log.Fatal()` and crashes
**Needed**: Log a warning and fall back to HTTP-only. The user may not have certs configured.

**Location**: Look for `log.Fatal` calls related to TLS/HTTPS in the server startup code. Replace with:
```go
if _, err := os.Stat(certFile); os.IsNotExist(err) {
    log.Warn().Msg("TLS cert not found, falling back to HTTP only")
    // Skip HTTPS listener, only start HTTP
}
```

### 4.3 Organizer Partial File Cleanup
**File**: `internal/organizer/organizer.go`
**Detail**: If a file copy is interrupted (e.g., disk full, process killed), partial files are left behind. Add:
1. Write to a `.tmp` suffix first (e.g., `book.m4b.tmp`)
2. On successful copy, rename `.tmp` to final name
3. On error, delete the `.tmp` file
4. On startup, scan for and clean up any `.tmp` files in the organized directory

### 4.4 Scanner Symlink Loop Protection
**File**: `internal/scanner/scanner.go`
**Detail**: If the library contains symlink loops (A -> B -> A), the scanner could loop forever. Add:
1. Track visited inodes using a `map[uint64]bool` (use `os.Stat().Sys().(*syscall.Stat_t).Ino`)
2. Skip any directory already visited
3. Log a warning when a symlink loop is detected

### 4.5 Stuck Operation Detection
**File**: `internal/operations/queue.go`
**Detail**: If a background operation hangs (e.g., metadata fetch to unresponsive server), it stays "in_progress" forever. Add:
1. Operation timeout (configurable, default 30 minutes)
2. Periodic check for operations that have been running longer than timeout
3. Mark timed-out operations as "failed" with reason "operation timed out"
4. Add `GET /api/v1/operations/stale` endpoint to list stuck operations

### 4.6 Rate Limiting
**File**: `internal/server/middleware/ratelimit.go` (NEW)
**Detail**: No rate limiting on any endpoint. For public deployment:
1. Add per-IP rate limiting middleware (use `golang.org/x/time/rate`)
2. Default: 100 requests/minute for API, 10 requests/minute for auth endpoints
3. Return `429 Too Many Requests` when exceeded
4. Configurable in config file

### 4.7 Request Size Limits
**File**: `internal/server/server.go`
**Detail**: No request body size limits. A malicious POST could exhaust memory.
1. Add `http.MaxBytesReader` wrapper to all POST/PUT handlers
2. Default limit: 10MB for file uploads, 1MB for JSON bodies
3. Return `413 Request Entity Too Large` when exceeded

### 4.8 CORS Configuration
**File**: `internal/server/server.go`
**Detail**: Check current CORS settings. For embedded mode, CORS should be restrictive (same-origin). For dev mode with separate Vite server, CORS needs to allow localhost:5173.
1. In embedded mode: no CORS headers needed (same origin)
2. In dev mode: allow `http://localhost:5173` only
3. Never allow `*` in production

---

## 5. Build & Deployment Fixes

### 5.1 Fix Dockerfile.test Go Version
**File**: `Dockerfile.test`, line 6
**Current**: `FROM mcr.microsoft.com/devcontainers/go:1.23-bookworm`
**Fix**: Change to `FROM mcr.microsoft.com/devcontainers/go:1.25-bookworm`

### 5.2 Fix Dockerfile Node Version
**File**: `Dockerfile`, line 36
**Current**: `FROM --platform=$BUILDPLATFORM node:25-alpine AS frontend-builder`
**Fix**: Change to `FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-builder`

### 5.3 Align Coverage Thresholds
**Files**:
- `.github/workflows/ci.yml` line 38: `coverage-threshold: '0'` -> change to `'80'`
- `.github/repository-config.yml` line 98: `threshold: 60` -> change to `80`
- `Makefile` line 134: already 80% (correct)

### 5.4 Create docker-compose.yml
**File**: `docker-compose.yml` (NEW)
**Content**:
```yaml
version: '3.8'
services:
  audiobook-organizer:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data           # Database and config
      - /path/to/audiobooks:/audiobooks  # Library directory
    environment:
      - AO_DIR=/audiobooks
      - AO_DB=/data/audiobook-organizer.db
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/api/v1/health"]
      interval: 30s
      timeout: 10s
      retries: 3
```

### 5.5 Create .env.example
**File**: `.env.example` (NEW)
**Content**: See section 1.1 above

### 5.6 Create macOS launchd Plist
**File**: `deploy/com.audiobook-organizer.plist` (NEW)
**Content**:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.audiobook-organizer</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/audiobook-organizer</string>
        <string>serve</string>
        <string>--dir</string>
        <string>/Users/USERNAME/Audiobooks</string>
        <string>--host</string>
        <string>0.0.0.0</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/usr/local/var/log/audiobook-organizer.log</string>
    <key>StandardErrorPath</key>
    <string>/usr/local/var/log/audiobook-organizer.err</string>
</dict>
</plist>
```

### 5.7 Verify Docker Build End-to-End
**Steps**:
1. `docker build -t audiobook-organizer .`
2. `docker run -p 8080:8080 -v ~/audiobooks:/audiobooks audiobook-organizer serve --dir /audiobooks`
3. Open http://localhost:8080, verify UI loads
4. Import a book, verify scan/organize work inside container
5. Stop container, restart, verify data persists

---

## 6. Testing & QA

### 6.1 Fix Flaky Test
**File**: `internal/scanner/scanner_test.go`
**Test**: `TestScanService_SpecialCharsInFilenames`
**Issue**: Intermittently fails — "Expected 3 books, got 2" or similar. One book occasionally fails to save.
**Likely cause**: Race condition in parallel test execution. Global state (`GlobalStore`, `GlobalQueue`) shared across tests.
**Fix**: Ensure test uses isolated store instance via `testutil.SetupIntegration(t)` and doesn't share globals with other parallel tests.

### 6.2 Manual QA Checklist
Create `docs/qa-checklist.md` with a step-by-step verification list:

```markdown
# QA Checklist

## First Run
- [ ] Fresh binary starts without errors
- [ ] Welcome wizard appears on first visit
- [ ] Can set library path in wizard
- [ ] Can skip optional steps (AI key, iTunes)
- [ ] Dashboard loads after wizard completion

## Import & Scan
- [ ] Can scan a directory with M4B files
- [ ] Can scan a directory with MP3 files
- [ ] Can scan mixed format directories
- [ ] Progress shows in UI via SSE
- [ ] Duplicate files detected and skipped
- [ ] Special characters in filenames handled

## iTunes Import
- [ ] Can browse for Library.xml
- [ ] Validation shows track count
- [ ] Import creates book records
- [ ] Metadata fetch runs after import
- [ ] Files organized into author/title structure

## Library View
- [ ] Books display in grid
- [ ] Books display in list view
- [ ] Pagination works (next/prev)
- [ ] Search filters results
- [ ] Sort by title/author/date works
- [ ] Cover art displays when available

## Book Detail
- [ ] All metadata fields display
- [ ] Can edit metadata fields
- [ ] Multi-author display works
- [ ] Narrator display works
- [ ] File list shows all audio files

## Organize
- [ ] Copy strategy works
- [ ] Hardlink strategy works
- [ ] Naming pattern applied correctly
- [ ] Multi-author books filed under primary author

## Settings
- [ ] Can change library path
- [ ] Can change naming pattern
- [ ] Can toggle AI features
- [ ] Can configure download clients
- [ ] Settings persist across restarts

## Auto-scan (fsnotify)
- [ ] Dropping file in import dir triggers scan within 30s
- [ ] SSE notification fires
- [ ] New book appears in library

## Backup/Restore
- [ ] Can create backup
- [ ] Can restore from backup
- [ ] Auto-cleanup of old backups works

## Download Clients
- [ ] Deluge connection works (if available)
- [ ] qBittorrent connection works (if available)
- [ ] SABnzbd connection works (if available)

## Edge Cases
- [ ] 10,000+ book library performs acceptably
- [ ] Very long filenames handled
- [ ] Unicode filenames handled
- [ ] Network disconnection handled gracefully
- [ ] Disk full handled gracefully
- [ ] Concurrent operations don't conflict
```

### 6.3 Integration Test for Full Workflow
**File**: `internal/integration_test.go` (NEW or extend existing)
**Detail**: End-to-end test that:
1. Creates temp directory with sample audio files
2. Starts server programmatically
3. Hits wizard endpoint to configure
4. Triggers scan via API
5. Verifies books appear in database
6. Triggers organize via API
7. Verifies files moved to correct structure
8. Fetches metadata via API
9. Verifies metadata populated
10. Creates backup, verifies it's valid
11. Tears down

### 6.4 Increase Test Coverage to 85%+
**Current**: 81.3%
**Target**: 85%+
**Focus areas** (packages with lowest coverage):
- `internal/server/` — currently 79.8%, add tests for new auth endpoints, cover proxy, storage endpoint
- `internal/organizer/` — add tests for partial file cleanup, multi-author filing
- `internal/watcher/` — add tests for debounce behavior, audio-only filter
- New user/session/playback code — ensure the migration 16 tables have comprehensive tests

### 6.5 Frontend Test Coverage
**Current**: 23 tests
**Target**: 40+ tests
**Missing tests for**:
- WelcomeWizard iTunes step
- LoginPage (once implemented)
- Dark mode toggle
- Search integration
- Error boundary
- Book detail editing
- Settings persistence

### 6.6 E2E Test for Auth Flow
**File**: `web/tests/e2e/auth-flow.spec.ts` (NEW)
**Test**:
1. Visit app, get redirected to login
2. First-run: create admin account
3. Login with credentials
4. Verify dashboard loads
5. Logout, verify redirected to login
6. Try accessing API without auth, verify 401

---

## 7. Documentation

### 7.1 README Overhaul
**File**: `README.md`
**Current state**: Likely minimal or template. Needs:
- Project description and screenshots
- Quick start (Docker and binary)
- Configuration reference
- API overview (link to OpenAPI spec)
- Development setup guide
- Architecture overview
- License

### 7.2 OpenAPI Spec Completion
**File**: `docs/openapi.yml` or generate from code
**Current**: Only 5 of 71 endpoints documented
**Needed**: Full spec for all 71 endpoints. Consider using `swaggo/swag` to generate from Go doc comments:
1. Add swagger comments to all handler functions
2. Run `swag init` to generate spec
3. Serve spec at `/api/v1/docs`

Alternatively, write by hand for the ~66 undocumented endpoints. Group by:
- Auth (login, logout, me, setup)
- Audiobooks (CRUD, search, batch operations)
- Authors (CRUD, merge)
- Metadata (fetch, providers)
- Scanner (scan, status)
- Organizer (organize, preview)
- Operations (list, status, cancel)
- Config/Settings (get, update)
- Backup (create, restore, list)
- Download clients (configure, test connection)
- iTunes (validate, import)
- System (health, storage, version)
- SSE (events stream)

### 7.3 Environment Variables Documentation
Add to README or create `docs/configuration.md`:
- All CLI flags (`--dir`, `--port`, `--host`, `--db`, etc.)
- All environment variables
- Config file format (JSON) with all fields explained
- Example configs for common setups (single-user, Docker, NAS)

### 7.4 Architecture Documentation
**File**: `docs/architecture.md` (NEW)
**Content**:
- System diagram showing Go backend + embedded React frontend
- Database schema diagram (all 16 migrations)
- API request/response flow
- Background operations model (queue + SSE)
- File organization strategy
- Metadata enrichment pipeline
- Authentication flow

---

## 8. Performance & Scalability

### 8.1 SQLite WAL Mode
**File**: `internal/database/sqlite_store.go`
**Detail**: Ensure WAL (Write-Ahead Logging) mode is enabled for better concurrent read performance:
```go
db.Exec("PRAGMA journal_mode=WAL")
db.Exec("PRAGMA synchronous=NORMAL")
db.Exec("PRAGMA busy_timeout=5000")
```
Check if this is already done. If not, add it in the `Open()` or `NewSQLiteStore()` function.

### 8.2 Query Optimization for Large Libraries
**File**: `internal/database/sqlite_store.go`
**Detail**: For libraries with 10,000+ books:
1. Add composite index on `books(author_id, title)` for sorted queries
2. Add index on `books(created_at)` for recent-first sorting
3. Ensure `LIMIT/OFFSET` pagination uses indexed columns
4. Consider cursor-based pagination for large result sets (use `WHERE id > last_id LIMIT N` instead of `OFFSET`)

### 8.3 In-Memory Cache for Hot Paths
**File**: `internal/cache/cache.go` (NEW, optional)
**Detail**: For frequently accessed data (config, author list, dashboard stats):
1. Simple TTL cache using `sync.Map` or a small LRU
2. Invalidate on writes
3. Cache: config (5 min TTL), author list (1 min TTL), dashboard counts (30s TTL)
4. This is P2 — only needed if SQLite becomes a bottleneck

### 8.4 Full-Text Search
**File**: `internal/database/sqlite_store.go`
**Detail**: Currently uses `LIKE '%term%'` which doesn't use indexes. For better search:
1. Create FTS5 virtual table: `CREATE VIRTUAL TABLE books_fts USING fts5(title, content=books, content_rowid=rowid)`
2. Add triggers to keep FTS table in sync with books table
3. Replace LIKE queries with FTS5 MATCH queries
4. This gives much faster search + relevance ranking
5. Add as migration 17

### 8.5 Concurrent Request Handling
**File**: `internal/server/server.go`
**Detail**: Verify that the server handles concurrent requests properly:
1. Ensure SQLite connection pool is configured (not just a single connection)
2. Verify `database/sql.DB` `SetMaxOpenConns()` is set appropriately
3. Test with concurrent load (use `hey` or `wrk` tool)

---

## 9. Polish & UX

### 9.1 Favicon and App Icons
**Files**: `web/public/favicon.ico`, `web/public/manifest.json`
**Detail**: Create a proper favicon and PWA icons. Design should suggest "audiobook" (headphones + book, or sound waves + book).

### 9.2 PWA Manifest
**File**: `web/public/manifest.json`
**Detail**: Make the app installable as a PWA:
```json
{
  "name": "Audiobook Organizer",
  "short_name": "AudiobookOrg",
  "start_url": "/",
  "display": "standalone",
  "background_color": "#ffffff",
  "theme_color": "#1976d2",
  "icons": [...]
}
```

### 9.3 Responsive Design Audit
**Files**: All components in `web/src/components/`
**Detail**: Verify all pages work well on:
- Desktop (1920x1080)
- Tablet (768x1024)
- Mobile (375x667)
Test with Chrome DevTools device emulation. Fix any overflow, truncation, or layout issues.

### 9.4 Keyboard Shortcuts
**File**: `web/src/hooks/useKeyboardShortcuts.ts` (NEW)
**Shortcuts**:
- `/` — focus search bar
- `g l` — go to library
- `g d` — go to dashboard
- `g s` — go to settings
- `Escape` — close modal/dialog
- `?` — show keyboard shortcuts help

### 9.5 Toast Notifications
**Files**: Check if already using MUI Snackbar
**Detail**: Ensure all operations show success/error toasts:
- Scan started/completed
- Organize started/completed
- Settings saved
- Metadata fetch complete
- Error messages for failed operations

### 9.6 Empty States
**Files**: All list/grid views
**Detail**: When a page has no data, show helpful empty states:
- Library: "No audiobooks yet. Scan a folder or import from iTunes to get started." + action buttons
- Dashboard: "Welcome! Set up your library to get started."
- Search results: "No books match your search."

### 9.7 Confirmation Dialogs
**Files**: Components that trigger destructive actions
**Detail**: Add confirmation dialogs for:
- Delete a book
- Remove from library (but keep files)
- Reset settings to defaults
- Clear all data

---

## 10. Release Pipeline

### 10.1 GoReleaser Token Permissions
**File**: `.github/workflows/release-prod.yml`
**Detail**: Verify the workflow has correct permissions for:
- Creating GitHub releases
- Uploading release artifacts
- Publishing Docker images to GHCR
- Signing with cosign (if configured)

### 10.2 Changelog Generation
**Detail**: Ensure GoReleaser generates proper changelogs from conventional commits. The `.goreleaser.yml` already has changelog filtering configured. Test by creating a release tag:
1. `git tag v0.1.0`
2. Run GoReleaser locally: `goreleaser release --snapshot --clean`
3. Verify changelog looks correct

### 10.3 Binary Smoke Test in CI
**File**: `.github/workflows/ci.yml` or new workflow
**Detail**: After building the binary in CI, add a smoke test:
1. Start the server with a temp directory
2. Wait for health check to pass
3. Hit `GET /api/v1/health` and verify 200
4. Hit `GET /` and verify HTML response
5. Kill server

### 10.4 Docker Image Publishing
**File**: `.github/workflows/prerelease.yml`
**Detail**: Verify Docker image publishes to GitHub Container Registry on release:
1. Image tagged with version and `latest`
2. Multi-arch (amd64, arm64) builds work
3. Image size is reasonable (< 100MB)

### 10.5 Installation Script
**File**: `install.sh` (NEW)
**Detail**: One-line install script for users:
```bash
curl -fsSL https://raw.githubusercontent.com/jdfalk/audiobook-organizer/main/install.sh | bash
```
Script should:
1. Detect OS and architecture
2. Download latest release binary from GitHub
3. Place in `/usr/local/bin/`
4. Print quick-start instructions

---

## Implementation Priority Order

### Phase 1: Security & Auth (must do before any public release)
1. Section 1.1 — Rotate API key, scrub git history
2. Section 1.2 — Audit for other secrets
3. Section 2.1 — Basic HTTP authentication
4. Section 4.6 — Rate limiting
5. Section 4.7 — Request size limits
6. Section 4.8 — CORS configuration

### Phase 2: Stability & Build Fixes (quick wins)
7. Section 5.1 — Fix Dockerfile.test Go version
8. Section 5.2 — Fix Dockerfile Node version
9. Section 5.3 — Align coverage thresholds
10. Section 4.2 — Graceful HTTPS degradation
11. Section 4.1 — Config validation
12. Section 6.1 — Fix flaky test

### Phase 3: Frontend Polish
13. Section 3.1 — Fix Login page
14. Section 3.3 — Dark mode toggle
15. Section 3.5 — Search integration
16. Section 3.4 — QuotaTab real data
17. Section 3.7 — Error boundaries
18. Section 3.8 — Loading states
19. Section 9.6 — Empty states
20. Section 9.5 — Toast notifications

### Phase 4: Backend Hardening
21. Section 4.3 — Organizer partial file cleanup
22. Section 4.4 — Scanner symlink loop protection
23. Section 4.5 — Stuck operation detection
24. Section 8.1 — SQLite WAL mode

### Phase 5: Deployment & Distribution
25. Section 5.4 — docker-compose.yml
26. Section 5.5 — .env.example
27. Section 5.6 — macOS launchd plist
28. Section 5.7 — Verify Docker build end-to-end
29. Section 10.3 — Binary smoke test in CI
30. Section 10.5 — Installation script

### Phase 6: Testing & Documentation
31. Section 6.2 — Manual QA checklist
32. Section 6.3 — Full workflow integration test
33. Section 6.4 — Coverage to 85%+
34. Section 6.5 — Frontend test coverage
35. Section 6.6 — E2E auth flow test
36. Section 7.1 — README overhaul
37. Section 7.3 — Environment variables docs
38. Section 7.4 — Architecture docs

### Phase 7: Performance & Nice-to-Have
39. Section 8.2 — Query optimization
40. Section 8.4 — Full-text search (FTS5)
41. Section 3.2 — Works page (decide: implement or remove)
42. Section 9.1 — Favicon and app icons
43. Section 9.2 — PWA manifest
44. Section 9.3 — Responsive design audit
45. Section 9.4 — Keyboard shortcuts
46. Section 7.2 — OpenAPI spec completion

### Phase 8: Final Polish
47. Section 3.6 — Remove unused API functions
48. Section 8.3 — In-memory cache
49. Section 8.5 — Concurrent request handling
50. Section 9.7 — Confirmation dialogs
51. Section 10.1 — GoReleaser token permissions
52. Section 10.2 — Changelog generation
53. Section 10.4 — Docker image publishing

---

## Estimated Effort

| Phase | Items | Estimated Effort |
|-------|-------|-----------------|
| 1. Security & Auth | 6 items | Large (auth is the biggest single feature remaining) |
| 2. Stability & Build | 6 items | Small (mostly config changes) |
| 3. Frontend Polish | 8 items | Medium |
| 4. Backend Hardening | 4 items | Medium |
| 5. Deployment | 6 items | Medium |
| 6. Testing & Docs | 8 items | Large |
| 7. Performance | 7 items | Medium-Large |
| 8. Final Polish | 7 items | Small-Medium |

**Total remaining items**: 52
**Critical path**: Security (Phase 1) -> Build fixes (Phase 2) -> Auth UI (Phase 3) -> Deploy (Phase 5) -> QA (Phase 6)

---

## Key Files Reference

| Purpose | File |
|---------|------|
| Main server | `internal/server/server.go` |
| Router setup | `internal/server/server.go` (routes registered in `Start()`) |
| Database store interface | `internal/database/store.go` |
| SQLite implementation | `internal/database/sqlite_store.go` |
| Migrations | `internal/database/migrations.go` |
| Config struct | `internal/config/config.go` |
| CLI commands | `cmd/root.go` |
| Frontend API client | `web/src/services/api.ts` |
| Frontend routing | `web/src/App.tsx` |
| Frontend state | `web/src/store/` (Zustand stores) |
| Build system | `Makefile` |
| Docker | `Dockerfile`, `Dockerfile.test` |
| CI | `.github/workflows/ci.yml` |
| Release | `.github/workflows/release-prod.yml`, `.goreleaser.yml` |
| Embed directive | `web_embed.go` |
