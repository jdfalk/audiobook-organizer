<!-- file: docs/mvp-tasks/task-3/EXECUTION-LOG.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-4a5b-8c9d-0e1f2a3b4c5d -->
<!-- last-edited: 2026-01-19 -->

# Task 3: Execution Log - Import Size Reporting Fixes

**Date:** December 14, 2025 **Agent:** Task-3 Independent Agent **Status:** In
Progress

## 📋 Executive Summary

This document tracks the execution of all phases in Task-3 to fix negative or
incorrect import size reporting.

**Primary Deliverable:** Fix `/api/v1/system/status` to return correct,
non-negative `library_size_bytes`, `import_size_bytes`, and `total_size_bytes`
values that match on-disk reality.

---

## 🎯 Phase Timeline

| Phase                                | Status     | Start Time | End Time | Notes                                  |
| ------------------------------------ | ---------- | ---------- | -------- | -------------------------------------- |
| Phase 0: Baseline Capture            | ⏳ Pending | -          | -        | Read-only system status & config check |
| Phase 1: On-Disk Measurement         | ⏳ Pending | -          | -        | Use `du` to verify sizes               |
| Phase 2: Backend Code Verification   | ⏳ Pending | -          | -        | Inspect size aggregation logic         |
| Phase 3: Size Recomputation via Scan | ⏳ Pending | -          | -        | Trigger scan and verify totals         |
| Phase 4: Restart & Persistence       | ⏳ Pending | -          | -        | Kill/restart server, verify no loss    |
| Phase 5: Cleanup                     | ⏳ Pending | -          | -        | Remove lock/state files                |

---

## 📊 Findings (Will be populated as phases execute)

### Phase 0: Baseline Capture

- **Status:** ⏳ Pending
- **Expected:** `total_size_bytes = library_size_bytes + import_size_bytes`, all
  non-negative
- **API Response:** TBD

### Phase 1: On-Disk Measurement

- **Status:** ⏳ Pending
- **Library Path:** TBD
- **Import Paths:** TBD
- **On-Disk Totals:** TBD
- **Mismatch vs API:** TBD

### Phase 2: Backend Code Verification

- **Status:** ⏳ Pending
- **Key Findings:** TBD
- **Potential Issues Identified:** TBD

### Phase 3: Size Recomputation

- **Status:** ⏳ Pending
- **Pre-Scan Values:** TBD
- **Post-Scan Values:** TBD
- **Scan Duration:** TBD
- **Pass Criteria:** TBD

### Phase 4: Restart & Persistence

- **Status:** ⏳ Pending
- **Pre-Restart Values:** TBD
- **Post-Restart Values:** TBD
- **Drift Check:** TBD

---

## 🔧 Backend Code Inspection (Phase 2 Deep Dive)

### Files to Inspect

- `internal/server/server.go` - System status handler
- `internal/database/*.go` - Size calculation logic
- `internal/scanner/*.go` - File scanning and size accumulation

### Key Functions to Check

- [ ] System status handler (`total_size_bytes` calculation)
- [ ] Library size aggregation (only files under `root_dir`)
- [ ] Import size aggregation (files in import paths)
- [ ] Data type verification (int64 usage, no int32 casts)
- [ ] Path normalization before classification

---

## 🚨 Issues Found

(Will be populated as issues are discovered)

| ID  | Issue | Symptom | Fix Required | Status |
| --- | ----- | ------- | ------------ | ------ |
| -   | -     | -       | -            | -      |

---

## ✅ Verification Checklist

- [ ] Phase 0 complete: baseline sizes captured
- [ ] Phase 1 complete: on-disk measurement done
- [ ] Phase 2 complete: no code issues found or documented
- [ ] Phase 3 complete: scan recomputes correctly
- [ ] Phase 4 complete: restart preserves sizes
- [ ] No negative values observed in any phase
- [ ] `total = library + import` holds true
- [ ] UI can be tested with correct API data

---

## 📝 Next Steps

1. Execute Phase 0 (baseline capture)
2. Compare with Phase 1 (on-disk)
3. Investigate Phase 2 (code review)
4. Execute Phase 3 (scan)
5. Execute Phase 4 (restart)
6. Finalize report

---
