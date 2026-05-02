<!-- file: docs/mvp-tasks/task-2/README.md -->
<!-- version: 2.0.1 -->
<!-- guid: 1c2d3e4f-5a6b-7c8d-9e0f-1a2b3c4d5e6f -->
<!-- last-edited: 2026-01-19 -->

# Task 2: Separate Dashboard Counts - Complete Documentation

## 📖 Overview

This task ensures **library** and **import** audiobook counts are calculated,
persisted, and displayed separately across the backend API and the
Dashboard/Library UI.

**Deliverables:**

- Backend returns distinct `library_book_count`, `import_book_count`, and
  `total_book_count`.
- Dashboard and Library views show separate counts with correct labeling.
- Scans keep counts accurate across library/import paths.
- Tests and troubleshooting steps validated.

## 📂 Document Set

| Document                              | Purpose                                            |
| ------------------------------------- | -------------------------------------------------- |
| `TASK-2-CORE-TESTING.md`              | Core validation flow, phases, safety/locks         |
| `TASK-2-ADVANCED-SCENARIOS.md`        | Edge cases, performance, code deep dive            |
| `TASK-2-TROUBLESHOOTING.md`           | Issues, root causes, and fixes                     |
| `TASK-2-README.md` (this file)        | Overview, navigation, quick commands               |
| `TASK-2-SEPARATE-DASHBOARD-COUNTS.md` | Legacy doc kept for history; points to split files |

**Reading order:** README → Core → Advanced → Troubleshooting (as needed).

## 🎯 Success Criteria

- API: `/api/v1/system/status` exposes `library_book_count`,
  `import_book_count`, `total_book_count` with total = library + import.
- UI: Dashboard and Library pages show "Library: X" and "Import: Y"
  consistently.
- Scan operations update counts accurately for both root and import paths.
- No negative counts, no double counting, no missing counts.

## 🚀 Quick Start

1. Run core phases in `TASK-2-CORE-TESTING.md` (pre-checks → backend → frontend
   → validation → cleanup).
2. If counts mismatch or UI stale, jump to `TASK-2-TROUBLESHOOTING.md`.
3. For edge cases (large libraries, mixed paths, offline UI), see
   `TASK-2-ADVANCED-SCENARIOS.md`.

## 🔐 Multi-AI Safety

Follow the same lock/state protocol used in Task 1. Create per-user lock files
under `/tmp/task-2-*` and clean them after tests. Core doc includes ready-to-run
snippets.

## 🎛️ Key Commands

```bash
# Check system status counts
curl -s http://localhost:8888/api/v1/system/status | jq '{library_book_count, import_book_count, total_book_count}'

# Check Dashboard UI code references
rg "library_book_count|import_book_count" web/src/pages/Dashboard.tsx

# Trigger scan to refresh counts
curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" | jq '.'

# Verify log for count reporting
ls -1 /Users/jdfalk/ao-library/logs | tail -3
```

## 🧭 Navigation

- Need the main flow? → `TASK-2-CORE-TESTING.md`
- Handling edge cases? → `TASK-2-ADVANCED-SCENARIOS.md`
- Something broken? → `TASK-2-TROUBLESHOOTING.md`
- Legacy context? → `TASK-2-SEPARATE-DASHBOARD-COUNTS.md`

## 🧩 Current State (from TODO)

- Priority: Medium (MVP-blocking for Dashboard accuracy)
- Backend fields expected since v1.26.0
- UI may still need adjustments or verification

## ✅ Next Actions

1. Execute Core Phases (file checks, API verification, UI validation).
2. Address discrepancies (missing fields, UI not rendering counts).
3. Validate with scan + refresh to confirm persistence.
4. Log results in operation log and mark TODO when complete.

---
