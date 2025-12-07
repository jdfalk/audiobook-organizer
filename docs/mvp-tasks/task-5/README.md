<!-- file: docs/TASK-5-README.md -->
<!-- version: 1.0.0 -->
<!-- guid: 3f7d8c2e-9b4a-4d5c-8f6e-1a7b2c3d4e5f -->

# Task 5: Hash Tracking & State Lifecycle - Complete Documentation

## 📖 Overview

This task implements persistent hash tracking and book state management to prevent unwanted reimports and enable safe delete workflows. Core requirement: track both **original import hash** and **post-organization hash** for each book to detect when library copies are removed and reimport originals without data loss.

**Deliverables:**

- DB stores `original_hash` (import copy) and `library_hash` (organized copy) for each book.
- Scanner computes and persists both hashes during import and organize operations.
- Delete workflow records hashes in `do_not_import` blocklist to prevent future reimports.
- State machine tracks book lifecycle: `wanted` → `imported` → `organized` → `soft_deleted`.
- Background purge job cleans soft-deleted entries after retention period.
- Settings page provides UI to manage blocked hashes.

## 📂 Document Set

| Document                       | Purpose                                                  |
| ------------------------------ | -------------------------------------------------------- |
| `TASK-5-CORE-TESTING.md`       | Core validation flow, phases, safety/locks               |
| `TASK-5-ADVANCED-SCENARIOS.md` | Edge cases (hash drift, multi-version, orphaned imports) |
| `TASK-5-TROUBLESHOOTING.md`    | Issues, root causes, and fixes                           |
| `TASK-5-README.md` (this file) | Overview, navigation, quick commands                     |

**Reading order:** README → Core → Advanced → Troubleshooting.

## 🎯 Success Criteria

- DB schema includes `original_hash`, `library_hash`, `state`, `soft_deleted_at`, `quantity` fields.
- `do_not_import` table with hash, reason, timestamp.
- Scanner skips files matching `do_not_import` hashes.
- Delete dialog offers "Prevent Reimporting" checkbox that adds hash to blocklist.
- Settings page shows blocked hashes with unblock action.
- Background job purges soft-deleted books after N days (configurable).

## 🚀 Quick Start

```bash
# Check if state fields exist in DB
rg "original_hash|library_hash|state|soft_deleted" internal/database/schema.go

# Check for do_not_import table
rg "do_not_import|blocklist|banned_hash" internal/database -n

# Query current book states
curl -s http://localhost:8888/api/v1/audiobooks?limit=10 | jq '.items[] | {title, state, original_hash, library_hash}'

# Check if delete flow includes reimport prevention
rg "prevent.*import|do_not_import" web/src -n
```

## 🔐 Multi-AI Safety

- Use lock/state files under `/tmp/task-5-*`.
- Test hash tracking with non-production copies; avoid deleting real library files.
- Capture DB state before/after delete operations.

## 🧭 Navigation

- Need the main flow? → `TASK-5-CORE-TESTING.md`
- Handling edge cases? → `TASK-5-ADVANCED-SCENARIOS.md`
- Something broken? → `TASK-5-TROUBLESHOOTING.md`

## 🧩 Current State (from TODO)

- Priority: High (New Requirement, MVP-blocking for safe delete workflows)
- Status: Not Started
- Depends on: Task 4 (hash computation infrastructure)

## ✅ Next Actions

1. Design DB schema additions (migration for new fields and table).
2. Update scanner to compute/store both hashes at import and organize stages.
3. Implement delete flow with reimport prevention option.
4. Create Settings tab for managing blocked hashes.
5. Add background purge job for soft-deleted entries.
6. Run Core Phases to validate end-to-end flow.

---
