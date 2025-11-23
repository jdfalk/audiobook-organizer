<!-- file: REFACTORING_SUMMARY.md -->
<!-- version: 1.0.0 -->
<!-- guid: e1f2a3b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b -->

# LibraryFolder → ImportPath Refactoring - Quick Reference

**Date**: November 23, 2025
**Read This First**: Quick overview before diving into detailed checklists

---

## 📋 Documents in This Package

1. **HANDOFF.md** - High-level context and current project state
2. **REFACTORING_CHECKLIST.md** (THIS IS THE MAIN GUIDE) - Complete step-by-step checklist with every file and line number
3. **CODING_STANDARDS.md** - Full Go and TypeScript coding standards
4. **REFACTORING_SUMMARY.md** (this file) - Quick reference and overview

---

## 🎯 What You Need to Do

Rename `LibraryFolder` to `ImportPath` throughout the entire codebase.

**Why?**: "Library folders" is confusing - it actually refers to import paths (monitored directories for new content), not the main library.

**Scope**: ~150+ occurrences across:

- Go backend (database, server, models, metrics)
- TypeScript frontend (API client, React components)
- Tests
- Documentation

---

## 🚀 Quick Start

### 1. Create Your Branch

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git checkout -b refactor/library-folder-to-import-path
```

### 2. Follow the Checklist

Open **REFACTORING_CHECKLIST.md** and work through it section by section:

- **Part 1**: Database Layer (Go) - Types, interfaces, implementations
- **Part 2**: Server/API Layer (Go) - Routes, handlers, tests
- **Part 3**: Models and Metrics (Go) - Audiobook model, Prometheus metrics
- **Part 4**: Frontend (TypeScript/React) - API client, components, pages
- **Part 5**: Documentation - Comments and README updates
- **Part 6**: Testing - Verify everything works
- **Part 7**: Commit and PR - Final rebase and submission

### 3. Test After Each Part

```bash
# After Parts 1-3 (Go changes)
go test ./... -v

# After Part 4 (Frontend changes)
cd web && npm run build && cd ..

# Integration test
go build -tags embed -o ~/audiobook-organizer-embedded
killall audiobook-organizer-embedded
cd /Users/jdfalk/ao-library
~/audiobook-organizer-embedded &
curl -s http://localhost:8888/api/v1/import-paths | jq .
```

### 4. Rebase Frequently

```bash
# At end of each part or at least daily
git checkout main && git pull origin main
git checkout refactor/library-folder-to-import-path
git rebase main
```

---

## 📊 Affected Files Summary

### Go Backend (~50 files)

**Database Layer**:

- `internal/database/store.go` - Interface definition and `LibraryFolder` type
- `internal/database/pebble_store.go` - PebbleDB implementation (~15 methods)
- `internal/database/sqlite_store.go` - SQLite implementation (~15 methods)
- `internal/database/database.go` - Initialization and table creation
- `internal/database/web.go` - Helper functions
- `internal/database/migrations.go` - Migration descriptions
- `internal/database/pebble_store_test.go` - Tests

**Server Layer**:

- `internal/server/server.go` - API routes and handlers (major file, ~20 occurrences)
- `internal/server/server_test.go` - API tests

**Models & Metrics**:

- `internal/models/audiobook.go` - Audiobook model field
- `internal/models/audiobook_test.go` - Model tests
- `internal/metrics/metrics.go` - Prometheus metrics

**CLI**:

- `cmd/root.go` - Command line diagnostics

### TypeScript Frontend (~40 files)

**API Client**:

- `web/src/services/api.ts` - Interface, functions, types (major file)

**Components**:

- `web/src/components/filemanager/LibraryFolderCard.tsx` → Rename to `ImportPathCard.tsx`
- `web/src/components/system/StorageTab.tsx`
- `web/src/components/system/SystemInfoTab.tsx`
- `web/src/components/wizard/WelcomeWizard.tsx`

**Pages**:

- `web/src/pages/FileManager.tsx` (major file, ~30 occurrences)
- `web/src/pages/Library.tsx`
- `web/src/pages/Settings.tsx`
- `web/src/pages/Dashboard.tsx`
- `web/src/pages/Logs.tsx`

---

## 🔑 Key Terminology Changes

| Old Term                       | New Term               | Context             |
| ------------------------------ | ---------------------- | ------------------- |
| `LibraryFolder` (Go type)      | `ImportPath`           | Database model      |
| `library_folders` (DB table)   | `import_paths`         | SQLite table name   |
| `library:<id>` (DB key)        | `import_path:<id>`     | PebbleDB key prefix |
| `/api/v1/library/folders`      | `/api/v1/import-paths` | API endpoint        |
| `listLibraryFolders`           | `listImportPaths`      | Handler function    |
| `GetAllLibraryFolders()`       | `GetAllImportPaths()`  | Store method        |
| `LibraryFolder` (TS interface) | `ImportPath`           | TypeScript type     |
| `getLibraryFolders()`          | `getImportPaths()`     | API client function |
| `<LibraryFolderCard>`          | `<ImportPathCard>`     | React component     |

---

## ⚠️ Important Notes

### DO:

- ✅ Follow the checklist item-by-item
- ✅ Test after each major section
- ✅ Rebase frequently (at least daily)
- ✅ Update ALL occurrences (database, API, frontend, tests)
- ✅ Keep variable names consistent (`importPath`, not `ip` or abbreviations)
- ✅ Update comments and documentation
- ✅ Use `gofmt` for Go code
- ✅ Follow naming conventions in CODING_STANDARDS.md

### DON'T:

- ❌ Skip testing between parts
- ❌ Rush through without checking off items
- ❌ Forget to update tests
- ❌ Mix "library folder" and "import path" terminology
- ❌ Leave commented-out old code
- ❌ Forget to rebase (will cause merge conflicts)

---

## 🧪 Testing Strategy

### Unit Tests

```bash
# Database tests
go test ./internal/database/... -v

# Server tests
go test ./internal/server/... -v

# All Go tests
go test ./... -v

# Frontend tests
cd web && npm test
```

### Integration Tests

```bash
# Build with embedded frontend
go build -tags embed -o ~/audiobook-organizer-embedded

# Start server
killall audiobook-organizer-embedded
cd /Users/jdfalk/ao-library
~/audiobook-organizer-embedded &

# Test new API endpoints
curl -s http://localhost:8888/api/v1/import-paths | jq .

# Test in browser
open http://localhost:8888
# Check Settings → Import Paths
# Check Dashboard shows correct counts
# Check FileManager page
```

---

## 📦 Deliverables

When complete, you should have:

1. ✅ All code renamed from `LibraryFolder` to `ImportPath`
2. ✅ All tests passing (Go and TypeScript)
3. ✅ API endpoints working at new URLs (`/import-paths`)
4. ✅ Frontend displaying "Import Paths" terminology
5. ✅ Documentation updated
6. ✅ Branch rebased against latest main
7. ✅ Commit with proper BREAKING CHANGE message
8. ✅ Pull request with detailed description

---

## 🆘 Getting Help

If stuck:

1. **Review the glossary** in HANDOFF.md - make sure you understand library vs import path
2. **Check REFACTORING_CHECKLIST.md** - every file and line number is listed
3. **Review CODING_STANDARDS.md** - for style questions
4. **Test frequently** - catch issues early
5. **Rebase often** - avoid merge conflicts

---

## 🎓 Understanding the Architecture

### Current Setup

**Library** (`root_dir`):

- Configured in `config.yaml`
- Example: `/Users/jdfalk/ao-library/library/`
- Where organized audiobooks are stored permanently
- Uses organized folder structure

**Import Paths** (what we're renaming):

- Stored in database (currently `library_folders` table - confusing!)
- Example: `/Users/jdfalk/Downloads/test_books`
- Monitored directories for new content
- External to the library
- New files here get imported into library

### Database Schema

**PebbleDB** (key-value):

```
import_path:1 → {"id":1,"path":"/Downloads/test_books","name":"Test Books",...}
import_path:2 → {"id":2,"path":"/tmp/imports","name":"Temp",...}
counter:import_path → 3
```

**SQLite** (relational):

```sql
CREATE TABLE import_paths (
  id INTEGER PRIMARY KEY,
  path TEXT NOT NULL,
  name TEXT NOT NULL,
  enabled BOOLEAN DEFAULT 1,
  last_scan TIMESTAMP,
  book_count INTEGER DEFAULT 0,
  total_size INTEGER DEFAULT 0
);
```

### API Flow

```
Frontend Request:
GET /api/v1/import-paths

↓

Server Handler (server.go):
listImportPaths() → c.JSON(200, importPaths)

↓

Database Layer (store.go interface):
GetAllImportPaths() → []ImportPath

↓

Implementation (pebble_store.go or sqlite_store.go):
Query database → return []ImportPath

↓

Response to Frontend:
[{"id":1,"path":"/Downloads/test_books","name":"Test Books",...}]
```

---

## 🏁 Final Checklist Before PR

- [ ] All items in REFACTORING_CHECKLIST.md marked complete
- [ ] `go test ./... -v` passes
- [ ] `cd web && npm test` passes
- [ ] Frontend builds: `cd web && npm run build`
- [ ] Server builds: `go build -tags embed -o ~/audiobook-organizer-embedded`
- [ ] Manual testing completed (API + UI)
- [ ] All `LibraryFolder` references replaced
- [ ] All `library_folder` references replaced
- [ ] All `library/folders` URL paths replaced
- [ ] Comments and documentation updated
- [ ] No linter errors
- [ ] Rebased against latest main
- [ ] Commit message follows format (with BREAKING CHANGE)
- [ ] PR description is detailed and clear

---

## 📈 Progress Tracking

Mark your progress:

- [ ] Part 1: Database Layer (Go) - ~50 items
- [ ] Part 2: Server/API Layer (Go) - ~40 items
- [ ] Part 3: Models and Metrics (Go) - ~5 items
- [ ] Part 4: Frontend (TypeScript/React) - ~50 items
- [ ] Part 5: Documentation - ~5 items
- [ ] Part 6: Testing and Verification - all tests
- [ ] Part 7: Commit and PR - final delivery

**Estimated Time**: 6-10 hours for careful, tested implementation

---

## 🎯 Success Criteria

You're done when:

1. Code compiles with no errors
2. All tests pass
3. API endpoint `/api/v1/import-paths` returns data
4. UI displays "Import Paths" not "Library Folders"
5. You can add/remove import paths via UI
6. Dashboard shows "Import Paths: N" count
7. No remaining references to `LibraryFolder` in codebase
8. PR is created and reviewed

---

**Good luck! Follow REFACTORING_CHECKLIST.md step-by-step and test frequently.** 🚀
