<!-- file: REFACTORING_README.md -->
<!-- version: 1.0.0 -->
<!-- guid: d1c2b3a4-e5f6-7a8b-9c0d-1e2f3a4b5c6d -->

# LibraryFolder → ImportPath Refactoring Package

**Welcome!** This directory contains complete documentation for refactoring the
audiobook-organizer codebase to rename `LibraryFolder` to `ImportPath`
throughout.

---

## 📚 Document Index

### 🚀 **START HERE**: [REFACTORING_SUMMARY.md](REFACTORING_SUMMARY.md)

Quick overview, quick start guide, and key concepts. Read this first!

### 📋 **MAIN GUIDE**: [REFACTORING_CHECKLIST.md](REFACTORING_CHECKLIST.md)

**This is your primary working document.** Complete item-by-item checklist with
every file, line number, and change required. This has ~200 specific items to
check off.

**Parts**:

1. Database Layer (Go) - Types, interfaces, PebbleDB, SQLite
2. Server/API Layer (Go) - Routes, handlers, tests
3. Models & Metrics (Go) - Audiobook model, Prometheus
4. Frontend (TypeScript/React) - API, components, pages
5. Documentation - Comments and README
6. Testing - Verification and integration tests
7. Commit & PR - Final delivery

### 📖 **CONTEXT**: [HANDOFF.md](HANDOFF.md)

High-level project context, glossary, architecture overview, and current state.
Useful for understanding the "why" behind this refactoring.

### 📐 **STANDARDS**: [CODING_STANDARDS.md](CODING_STANDARDS.md)

Complete Go and TypeScript coding standards for this project. Includes:

- Full Google Go Style Guide
- Full Google TypeScript Style Guide
- Project-specific conventions

Reference this for naming, formatting, and style questions.

---

## 🎯 Quick Start (30 seconds)

```bash
# 1. Create branch
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git checkout -b refactor/library-folder-to-import-path

# 2. Open main checklist
open REFACTORING_CHECKLIST.md

# 3. Work through parts 1-7, checking off items

# 4. Test frequently
go test ./... -v

# 5. Rebase often (daily)
git checkout main && git pull origin main
git checkout refactor/library-folder-to-import-path
git rebase main
```

---

## 🎓 What You Need to Know

### The Problem

The codebase uses `LibraryFolder` to refer to "import paths" (monitored
directories for new audiobooks), but this is confusing because:

- "Library" actually refers to the main organized collection (`root_dir`)
- "Library folders" sounds like folders IN the library
- But it actually means folders OUTSIDE the library that are monitored

### The Solution

Rename everything from `LibraryFolder` to `ImportPath` for clarity:

- Database type: `LibraryFolder` → `ImportPath`
- API endpoints: `/library/folders` → `/import-paths`
- Database tables: `library_folders` → `import_paths`
- Frontend: All references updated

### Scope

- **~150 occurrences** across Go backend and TypeScript frontend
- **~50 files** to modify
- **All tests** to update
- **6-10 hours** estimated time

---

## 📖 Reading Order

1. **REFACTORING_SUMMARY.md** (5 min) - Get oriented
2. **HANDOFF.md** (10 min) - Understand context and glossary
3. **REFACTORING_CHECKLIST.md** (main work) - Execute the refactoring
4. **CODING_STANDARDS.md** (reference) - Check style as needed

---

## ✅ Checklist Overview

**Part 1: Database Layer** (~50 items)

- Core type definitions
- Store interface methods
- PebbleDB implementation
- SQLite implementation
- Migrations and tests

**Part 2: Server/API Layer** (~40 items)

- API routes (`/library/folders` → `/import-paths`)
- Handler functions
- Server tests
- CLI commands

**Part 3: Models & Metrics** (~5 items)

- Audiobook model fields
- Prometheus metrics

**Part 4: Frontend** (~50 items)

- TypeScript interfaces
- API client functions
- React components (rename `LibraryFolderCard` → `ImportPathCard`)
- All page components

**Part 5: Documentation** (~5 items)

- Code comments
- README updates

**Part 6: Testing** (comprehensive)

- Unit tests (Go)
- Frontend tests
- Integration tests
- Manual verification

**Part 7: Delivery** (final)

- Final rebase
- Commit message with BREAKING CHANGE
- Pull request with detailed description

---

## 🧪 Testing Strategy

### After Part 1 (Database)

```bash
go test ./internal/database/... -v
```

### After Part 2 (Server)

```bash
go test ./internal/server/... -v
go build -o ~/audiobook-organizer-embedded
killall audiobook-organizer-embedded
cd /Users/jdfalk/ao-library
~/audiobook-organizer-embedded &
curl -s http://localhost:8888/api/v1/import-paths | jq .
```

### After Part 3 (Models)

```bash
go test ./internal/models/... -v
```

### After Part 4 (Frontend)

```bash
cd web
npm run build
npm test
cd ..
go build -tags embed -o ~/audiobook-organizer-embedded
# Test in browser at http://localhost:8888
```

### Final Verification

- All endpoints work
- UI displays correct terminology
- Can add/remove import paths
- Dashboard shows import path count
- No `LibraryFolder` references remain

---

## 🔄 Rebasing Strategy

**Critical**: Rebase frequently to avoid merge conflicts!

**When to Rebase**:

- After completing each major part (1-7)
- At least once per day during active work
- Before final PR submission

**How to Rebase**:

```bash
# Save work
git add .
git commit -m "WIP: Part N complete"

# Update main
git checkout main
git pull origin main

# Rebase your branch
git checkout refactor/library-folder-to-import-path
git rebase main

# If conflicts occur
# ... resolve conflicts ...
git add .
git rebase --continue

# Force push (your branch only!)
git push --force-with-lease origin refactor/library-folder-to-import-path
```

---

## 📦 Deliverables

When complete, you'll have:

1. All code renamed (database, API, frontend)
2. All tests passing
3. API working at `/api/v1/import-paths`
4. UI showing "Import Paths" terminology
5. Documentation updated
6. Clean git history (rebased against main)
7. Commit with proper BREAKING CHANGE notation
8. Detailed pull request

---

## 🆘 Troubleshooting

### Build Errors

**Problem**: `undefined: LibraryFolder`

**Solution**: You missed renaming a reference. Search codebase:

```bash
grep -r "LibraryFolder" --include="*.go"
```

### API 404 Errors

**Problem**: Frontend calls `/library/folders`, gets 404

**Solution**: Update frontend API client (`web/src/services/api.ts`)

### Test Failures

**Problem**: Tests reference old names

**Solution**: Update test files (check Part 1, 2, 3 of checklist)

### Type Errors in TypeScript

**Problem**: `Property 'folder' does not exist`

**Solution**: Update frontend to use `importPath` not `folder`

---

## 📋 Pre-Flight Checklist

Before starting:

- [ ] I've read REFACTORING_SUMMARY.md
- [ ] I understand the glossary (Library vs Import Path)
- [ ] I have REFACTORING_CHECKLIST.md open
- [ ] I've created my feature branch
- [ ] I have access to the codebase
- [ ] I can build and run the project
- [ ] I've set up my testing environment

---

## 🎯 Success Metrics

You've succeeded when:

1. ✅ Zero occurrences of `LibraryFolder` remain
2. ✅ Zero occurrences of `library_folders` (except in migration descriptions)
3. ✅ Zero occurrences of `/library/folders` URLs
4. ✅ All tests pass (Go + TypeScript)
5. ✅ API endpoint `/api/v1/import-paths` works
6. ✅ UI displays "Import Paths" everywhere
7. ✅ Manual testing successful
8. ✅ PR created and passing CI

---

## 📞 Support

If you need help:

1. **Re-read HANDOFF.md** - make sure you understand the concepts
2. **Check CODING_STANDARDS.md** - for style/naming questions
3. **Review REFACTORING_CHECKLIST.md** - ensure you didn't miss items
4. **Run tests** - they'll tell you what's broken
5. **Search codebase** - look for remaining old references
6. **Commit often** - so you can revert if needed

---

## 🎓 Learning Resources

### Go Style

- [Effective Go](https://golang.org/doc/effective_go.html)
- [Google Go Style Guide](https://google.github.io/styleguide/go/)
- Project's `CODING_STANDARDS.md`

### TypeScript/React Style

- [TypeScript Handbook](https://www.typescriptlang.org/docs/handbook/intro.html)
- [React TypeScript Cheatsheet](https://react-typescript-cheatsheet.netlify.app/)
- Project's `CODING_STANDARDS.md`

### Testing

- [Go Testing Package](https://golang.org/pkg/testing/)
- [Table-Driven Tests in Go](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
- [React Testing Library](https://testing-library.com/docs/react-testing-library/intro/)

---

## 🚀 Ready to Start?

1. Read **REFACTORING_SUMMARY.md** (5 min)
2. Skim **HANDOFF.md** for context (10 min)
3. Open **REFACTORING_CHECKLIST.md** (your main guide)
4. Create your branch
5. Start checking off items!

**Remember**: Test frequently, rebase often, commit regularly.

Good luck! 🎉
