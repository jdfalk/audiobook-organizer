# Phase 2 Handler Refactoring - Quick Start Guide

## Current Status: 50% Complete ✅⏳

**Completed:** Service creation (Phase 2A)
**Remaining:** Handler integration (Phase 2B)

---

## What Was Done (Phase 2A) ✅

**4 service classes created and fully tested:**

| Service | File | Purpose | Tests |
|---------|------|---------|-------|
| AudiobookUpdateService | `internal/server/audiobook_update_service.go` | JSON parsing & field updates | 8 ✅ |
| ImportPathService | `internal/server/import_path_service.go` | Path validation & creation | 4 ✅ |
| ConfigUpdateService | `internal/server/config_update_service.go` | Config field mapping | 5 ✅ |
| SystemService | `internal/server/system_service.go` | Status & log filtering | 4 ✅ |

**Total:** 21 tests passing | 399 lines of testable code | 0 compiler errors

---

## What's Next (Phase 2B) ⏳

**Integrate services into 5 handlers (2-4 hours of work):**

1. **updateAudiobook** - 141 → 15 lines (uses AudiobookUpdateService)
2. **updateConfig** - 167 → 22 lines (uses ConfigUpdateService)
3. **getSystemStatus** - 94 → 10 lines (uses SystemService)
4. **getSystemLogs** - 111 → 25 lines (uses SystemService)
5. **addImportPath** - simplify path creation (uses ImportPathService)

**Pattern for each:** Parse request → Call service → Format response

---

## Quick Start: Refactor First Handler

```bash
# Start here - refactor updateAudiobook handler

# 1. Read the detailed instructions
cat docs/plans/2026-02-03-phase2-status-and-next-steps.md

# 2. Look at Task 2B.1 section for exact code changes

# 3. Make changes to internal/server/server.go
#    - Add service field to Server struct (line ~468)
#    - Initialize in NewServer() (line ~490)
#    - Replace updateAudiobook handler (lines 1097-1237)

# 4. Test
make test  # Should pass

# 5. Commit
git add internal/server/server.go
git commit -m "refactor(updateAudiobook): thin HTTP adapter using service layer"
```

---

## File References

| Document | Purpose |
|----------|---------|
| `docs/plans/2026-02-03-phase2-status-and-next-steps.md` | **START HERE** - Complete guide with exact code examples |
| `docs/plans/2026-02-03-phase2-handler-refactoring.md` | Phase 2A service creation plan (for reference) |
| `internal/server/audiobook_update_service.go` | Example of completed service (well-structured) |
| `internal/server/audiobook_update_service_test.go` | Example of unit tests for service |

---

## Key Points

✅ **Services are ready** - All created, tested, and committed
✅ **Tests pass** - 21 new tests, 100% pass rate
✅ **No blockers** - Can start handler integration immediately
✅ **Code examples provided** - Copy-paste ready code in detailed guide
✅ **Testing workflow documented** - Know exactly what to verify

---

## Verification Checklist

Before starting handler integration:

```bash
# Verify current state
git status  # Should be clean

# Run all tests
make test  # Should see "✅ Backend tests passed"

# Verify build
make build-api  # Should complete successfully

# Check service files exist
ls -lh internal/server/*_service.go  # Should see 4 files + 4 test files
```

---

## Expected Time

- **Phase 2B Task 1** (updateAudiobook): 15-20 minutes
- **Phase 2B Tasks 2-4** (other handlers): 10-15 minutes each
- **Testing & verification**: 10 minutes
- **Total Phase 2B**: 1-2 hours for all 5 handler refactorings

---

## Next AI Instance Instructions

1. Read `docs/plans/2026-02-03-phase2-status-and-next-steps.md` (complete guide)
2. Start with Task 2B.1 (updateAudiobook handler refactoring)
3. Follow exact code examples provided
4. Commit each handler refactoring atomically
5. When all 5 handlers done, run full test suite
6. Update this document with completion date

**Estimated completion:** Same session or next session

---

## Git Commits So Far

```
e724470 docs: add Phase 2 status and handler integration next steps
25627a8 feat: create remaining Phase 2 services (ImportPath, ConfigUpdate, System)
84830e7 fix(audiobook_update_service): update to use any instead of interface{}
4d73733 fix(server): resolve compiler errors in Phase 1 services
ce9811c feat(scan_service): extract multi-folder scan logic into testable service
ec77581 refactor(server): integrate services and thin out HTTP handler adapters
...
```

---

## Questions?

Check `docs/plans/2026-02-03-phase2-status-and-next-steps.md` for:
- Exact method signatures
- Database interface details
- Handler line numbers
- Code examples for each refactoring
- Testing verification steps
- Quality checklist

All information needed for successful handoff is documented there.
