<!-- file: docs/mvp-tasks/INDEX.md -->
<!-- version: 1.0.0 -->
<!-- guid: 9a8b7c6d-5e4f-3a2b-1c0d-9e8f7a6b5c4d -->
<!-- last-edited: 2026-01-19 -->

# MVP Tasks Index

Master index for all 7 MVP-blocking tasks with organized documentation.

## 📚 Quick Navigation

| Task | Name                                                                      | Purpose                             | Status        |
| ---- | ------------------------------------------------------------------------- | ----------------------------------- | ------------- |
| 1    | [Scan Progress Reporting](#task-1-scan-progress-reporting)                | Real-time scan progress tracking    | ✅ Documented |
| 2    | [Separate Dashboard Counts](#task-2-separate-dashboard-counts)            | Library vs Import count separation  | ✅ Documented |
| 3    | [Import Size Reporting](#task-3-import-size-reporting)                    | Fix negative/incorrect size metrics | ✅ Documented |
| 4    | [Duplicate Detection](#task-4-duplicate-detection)                        | Validate SHA256-based deduplication | ✅ Documented |
| 5    | [Hash Tracking & State Lifecycle](#task-5-hash-tracking--state-lifecycle) | Dual-hash tracking + state machine  | ✅ Documented |
| 6    | [Book Detail Page & Delete Flow](#task-6-book-detail-page--delete-flow)   | UI for viewing/managing books       | ✅ Documented |
| 7    | [E2E Test Suite](#task-7-e2e-test-suite)                                  | Containerized Selenium/pytest tests | ✅ Documented |

---

## Task 1: Scan Progress Reporting

**Goal:** Real-time progress updates during library scans.

**Key Files:**

- [README.md](task-1/README.md) — Overview and quick start
- [CORE-TESTING.md](task-1/CORE-TESTING.md) — Core test phases (API, WebSocket,
  state)
- [SCAN-PROGRESS-TESTING.md](task-1/SCAN-PROGRESS-TESTING.md) — Dedicated
  progress validation
- [ADVANCED-SCENARIOS.md](task-1/ADVANCED-SCENARIOS.md) — Edge cases and
  performance
- [TROUBLESHOOTING.md](task-1/TROUBLESHOOTING.md) — Common issues and fixes

**Success Criteria:**

- ✅ `/api/v1/operations/{id}` returns real-time progress
- ✅ WebSocket sends updates every ~500ms
- ✅ Progress persists after restart
- ✅ Accurate for various library sizes

**Dependencies:** None (standalone)

---

## Task 2: Separate Dashboard Counts

**Goal:** Show distinct Library vs Import book counts.

**Key Files:**

- [README.md](task-2/README.md) — Overview and quick start
- [CORE-TESTING.md](task-2/CORE-TESTING.md) — Core verification phases
- [SEPARATE-DASHBOARD-COUNTS.md](task-2/SEPARATE-DASHBOARD-COUNTS.md) — Legacy
  comprehensive guide
- [ADVANCED-SCENARIOS.md](task-2/ADVANCED-SCENARIOS.md) — Mixed counts and edge
  cases
- [TROUBLESHOOTING.md](task-2/TROUBLESHOOTING.md) — Count mismatch issues

**Success Criteria:**

- ✅ `/api/v1/system/status` returns `library_book_count` and
  `import_book_count`
- ✅ Dashboard displays both counts separately
- ✅ Library page respects the separation
- ✅ Counts remain accurate after scans

**Dependencies:** Task 1 (scan must complete)

---

## Task 3: Import Size Reporting

**Goal:** Fix negative/incorrect import_size_bytes values.

**Key Files:**

- [README.md](task-3/README.md) — Overview and quick start
- [CORE-TESTING.md](task-3/CORE-TESTING.md) — Core size verification tests
- [ADVANCED-SCENARIOS.md](task-3/ADVANCED-SCENARIOS.md) — Symlinks, sparse
  files, performance
- [TROUBLESHOOTING.md](task-3/TROUBLESHOOTING.md) — Size mismatch debugging

**Success Criteria:**

- ✅ `import_size_bytes` never negative
- ✅ API reports match `du -sh` measurements
- ✅ Int64 overflow handled correctly
- ✅ Symlinks counted accurately

**Dependencies:** Task 2 (count separation)

---

## Task 4: Duplicate Detection

**Goal:** Validate SHA256-based duplicate detection.

**Key Files:**

- [README.md](task-4/README.md) — Overview and quick start
- [CORE-TESTING.md](task-4/CORE-TESTING.md) — Hash verification and query
  testing
- [ADVANCED-SCENARIOS.md](task-4/ADVANCED-SCENARIOS.md) — False
  positives/negatives, performance
- [TROUBLESHOOTING.md](task-4/TROUBLESHOOTING.md) — Hash mismatch issues

**Success Criteria:**

- ✅ SHA256 hashes computed correctly
- ✅ Duplicate queries return exact matches
- ✅ False positive/negative detection validated
- ✅ Performance acceptable for large libraries

**Dependencies:** Task 3 (size reporting)

---

## Task 5: Hash Tracking & State Lifecycle

**Goal:** Implement dual-hash tracking and state machine.

**Key Files:**

- [README.md](task-5/README.md) — Overview and quick start
- [CORE-TESTING.md](task-5/CORE-TESTING.md) — Hash schema and state transitions
- [ADVANCED-SCENARIOS.md](task-5/ADVANCED-SCENARIOS.md) — Reimport prevention,
  purge jobs
- [TROUBLESHOOTING.md](task-5/TROUBLESHOOTING.md) — State machine issues

**Success Criteria:**

- ✅ `original_hash` and `library_hash` tracked separately
- ✅ State machine: wanted → imported → organized → soft_deleted
- ✅ `do_not_import` table prevents reimport
- ✅ Blocked hash list viewable in Settings

**Dependencies:** Task 4 (hash infrastructure)

---

## Task 6: Book Detail Page & Delete Flow

**Goal:** Create dedicated book detail view with enhanced delete dialog.

**Key Files:**

- [README.md](task-6/README.md) — Overview and quick start
- [CORE-TESTING.md](task-6/CORE-TESTING.md) — Page rendering and delete flow
- [ADVANCED-SCENARIOS.md](task-6/ADVANCED-SCENARIOS.md) — Multi-format books,
  error states
- [TROUBLESHOOTING.md](task-6/TROUBLESHOOTING.md) — Navigation and UI issues

**Success Criteria:**

- ✅ Book detail page shows Info, Files, Versions tabs
- ✅ Delete dialog includes reimport prevention checkbox
- ✅ Blocklist confirmation prevents accidents
- ✅ Navigate between books smoothly

**Dependencies:** Task 5 (state tracking)

---

## Task 7: E2E Test Suite

**Goal:** Containerized Selenium/pytest test suite for all MVP workflows.

**Key Files:**

- [README.md](task-7/README.md) — Overview and quick start
- [CORE-TESTING.md](task-7/CORE-TESTING.md) — Test execution phases
- [ADVANCED-SCENARIOS.md](task-7/ADVANCED-SCENARIOS.md) — Performance,
  cross-browser, flaky tests
- [TROUBLESHOOTING.md](task-7/TROUBLESHOOTING.md) — Docker, networking,
  selectors

**Success Criteria:**

- ✅ All tests pass with green status
- ✅ Coverage includes all MVP tasks
- ✅ Runs in Docker container
- ✅ CI integration works
- ✅ Screenshot capture on failure

**Dependencies:** Tasks 1-6 (validates all prior tasks)

---

## 🚀 Getting Started

### For Running Task Tests

1. **Pick a task** from the table above
2. **Open the README.md** for that task (e.g., `task-1/README.md`)
3. **Follow the Quick Start** section
4. **Run CORE-TESTING.md phases** to validate
5. **Use TROUBLESHOOTING.md** if issues occur

### Example: Start with Task 1

```bash
# Navigate to task documentation
cd docs/mvp-tasks/task-1/

# Read overview
cat README.md

# Follow core testing phases
cat CORE-TESTING.md
```

### Task Execution Order

**Recommended execution sequence:**

1. **Task 1** — Ensure scan progress works
2. **Task 2** — Verify count separation
3. **Task 3** — Check size reporting accuracy
4. **Task 4** — Validate duplicate detection
5. **Task 5** — Test hash tracking and state machine
6. **Task 6** — Test book detail UI
7. **Task 7** — Run full E2E test suite

---

## 📋 Documentation Structure

Each task folder contains:

| File                    | Purpose                                                      |
| ----------------------- | ------------------------------------------------------------ |
| `README.md`             | Overview, goals, quick start (read first)                    |
| `CORE-TESTING.md`       | Essential test phases with safety locks                      |
| `ADVANCED-SCENARIOS.md` | Edge cases, performance, code deep dives                     |
| `TROUBLESHOOTING.md`    | Issues, root causes, remediation steps                       |
| (Task-specific files)   | Legacy or detailed guides (e.g., `SCAN-PROGRESS-TESTING.md`) |

---

## 🔗 Dependency Graph

```
Task 1 (Scan Progress)
    ↓
Task 2 (Separate Counts)
    ↓
Task 3 (Size Reporting)
    ↓
Task 4 (Duplicate Detection)
    ↓
Task 5 (Hash Tracking)
    ↓
Task 6 (Book Detail Page)
    ↓
Task 7 (E2E Tests) ← Validates all 1-6
```

---

## ✅ MVP Completion Checklist

- [ ] Task 1 — Scan Progress: Core + Advanced tests pass
- [ ] Task 2 — Separate Counts: Verified on Dashboard and API
- [ ] Task 3 — Size Reporting: No negative values, matches `du`
- [ ] Task 4 — Duplicate Detection: SHA256 hashes verified
- [ ] Task 5 — Hash Tracking: State machine working
- [ ] Task 6 — Book Detail Page: UI functional and tested
- [ ] Task 7 — E2E Tests: All test scenarios pass in Docker

**MVP ready when:** All tasks pass core tests AND E2E suite runs green.

---

## 📞 Quick Reference

| Need          | Action                                 |
| ------------- | -------------------------------------- |
| Task overview | Read `task-N/README.md`                |
| Run tests     | Follow `task-N/CORE-TESTING.md` phases |
| Dig deeper    | Review `task-N/ADVANCED-SCENARIOS.md`  |
| Debug issue   | Check `task-N/TROUBLESHOOTING.md`      |
| See all tasks | You're reading it!                     |

---

## 🔄 Last Updated

<!-- Updated when structure/content changes -->

- Document created: December 7, 2025
- All 7 tasks organized and documented
- Master index ready for navigation
