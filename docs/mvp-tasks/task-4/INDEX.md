<!-- file: docs/mvp-tasks/task-4/INDEX.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7f4e3b2a-9c8d-4e5f-8a7b-1c2d3e4f5a6b -->

# Task 4: Duplicate Detection - Document Index

## 📚 Quick Navigation

| File                                               | Description                                      | When to Use                                |
| -------------------------------------------------- | ------------------------------------------------ | ------------------------------------------ |
| [README.md](README.md)                             | Overview, success criteria, quick start          | Start here for task understanding          |
| [4-CORE-TESTING.md](4-CORE-TESTING.md)             | Main testing flow with phases and safety locks   | Primary testing execution                  |
| [4-ADVANCED-SCENARIOS.md](4-ADVANCED-SCENARIOS.md) | Edge cases, symlinks, performance considerations | After core testing passes                  |
| [4-TROUBLESHOOTING.md](4-TROUBLESHOOTING.md)       | Common issues and resolution steps               | When tests fail or show unexpected results |

## 🎯 Task Overview

**Goal:** Validate hash-based duplicate detection system (SHA256) for audiobooks.

**Key Components:**
- API endpoint: `/api/v1/audiobooks/duplicates`
- Backend: SHA256 hash computation during scan
- Database: `content_hash` or `file_hash` column
- Frontend: Duplicate visualization and management UI

## 🚦 Recommended Reading Order

### For First-Time Testing
1. **README.md** - Understand task goals and success criteria
2. **4-CORE-TESTING.md** - Execute main validation flow
3. **4-ADVANCED-SCENARIOS.md** - Test edge cases if needed
4. **4-TROUBLESHOOTING.md** - Refer when issues arise

### For Debugging/Issues
1. **4-TROUBLESHOOTING.md** - Find your specific issue
2. **4-CORE-TESTING.md** - Review relevant test phase
3. **4-ADVANCED-SCENARIOS.md** - Check if edge case applies

### For Implementation Review
1. **README.md** - Success criteria and deliverables
2. **4-ADVANCED-SCENARIOS.md** - Code checklist and schema requirements
3. **4-CORE-TESTING.md** - Verification procedures

## 🔍 Quick Command Reference

```bash
# Check implementation status
rg "duplicates|SHA256|content_hash" internal -n | head -20

# Query duplicate API
curl -s http://localhost:8888/api/v1/audiobooks/duplicates | jq '.'

# Trigger hash computation scan
curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" | jq '.'

# Verify file hashes manually
shasum -a 256 /path/to/audiobook.m4b
```

## 📊 Testing Phases Summary

| Phase | Type         | Description                         | File Reference       |
| ----- | ------------ | ----------------------------------- | -------------------- |
| 0     | Read-Only    | Implementation check                | 4-CORE-TESTING.md §0 |
| 1     | State-Change | Baseline scan with hash computation | 4-CORE-TESTING.md §1 |
| 2     | Read-Only    | Query duplicate API                 | 4-CORE-TESTING.md §2 |
| 3     | Optional     | Create test duplicates if needed    | 4-CORE-TESTING.md §3 |
| 4     | Read-Only    | Verify duplicate groups             | 4-CORE-TESTING.md §4 |
| 5     | Manual       | UI verification                     | 4-CORE-TESTING.md §5 |
| 6     | Cleanup      | Remove test files and locks         | 4-CORE-TESTING.md §6 |

## 🔐 Safety Guidelines

- Use lock files (`/tmp/task-4-lock.txt`) to prevent concurrent execution
- Capture state to JSON before making changes
- Test with read-only queries first
- Create test files in `/tmp/` only, never in library paths
- Clean up test files after validation

## ✅ Success Criteria Checklist

- [ ] API endpoint `/api/v1/audiobooks/duplicates` exists and returns valid JSON
- [ ] Duplicate groups contain 2+ books with identical SHA256 content hashes
- [ ] No false positives (distinct files incorrectly grouped)
- [ ] No false negatives (known duplicates missed)
- [ ] UI displays duplicate count on dashboard
- [ ] UI provides duplicate management interface
- [ ] Performance acceptable for large libraries (multi-GB files)
- [ ] Edge cases documented and handled (symlinks, metadata changes, missing files)

## 📞 Support Resources

- **Backend Implementation:** `internal/scanner/`, `internal/database/`
- **API Handlers:** `internal/server/audiobook_handlers.go`
- **Frontend Components:** `web/src/` (search for "duplicate")
- **Database Schema:** Check for `content_hash` or `file_hash` column

---

**Last Updated:** 2025-12-14
**Task Status:** Implementation complete (v1.9.0), testing in progress
**Priority:** Low (Optional MVP feature)
