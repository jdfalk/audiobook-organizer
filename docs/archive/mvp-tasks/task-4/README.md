<!-- file: docs/mvp-tasks/task-4/README.md -->
<!-- version: 1.0.1 -->
<!-- guid: 8c2d9e1f-7a3b-4c5d-9e7f-1a8b3c4d5e6f -->
<!-- last-edited: 2026-01-19 -->

# Task 4: Duplicate Detection Testing - Complete Documentation

## 📖 Overview

This task validates the hash-based duplicate detection system implemented in
v1.9.0. The goal is to ensure duplicate audiobooks are correctly identified via
content hashing (SHA256), reported through the API, and surfaced in the UI
without false positives.

**Deliverables:**

- API returns accurate duplicate book groups via
  `/api/v1/audiobooks/duplicates`.
- Duplicates computed using SHA256 file hashes (not metadata fuzzy matching).
- UI displays duplicate groups with clear labeling and actionable options
  (keep/delete).
- No false positives (distinct books incorrectly grouped) or false negatives
  (duplicates missed).
- Tests and troubleshooting steps validated.

## 📂 Document Set

| Document                  | Purpose                                           |
| ------------------------- | ------------------------------------------------- |
| `4-CORE-TESTING.md`       | Core validation flow, phases, safety/locks        |
| `4-ADVANCED-SCENARIOS.md` | Edge cases (partial files, symlinks, bit-perfect) |
| `4-TROUBLESHOOTING.md`    | Issues, root causes, and fixes                    |
| `README.md` (this file)   | Overview, navigation, quick commands              |

**Reading order:** README → Core → Advanced → Troubleshooting.

## 🎯 Success Criteria

- `/api/v1/audiobooks/duplicates` returns groups of books with identical SHA256
  hashes.
- Each group contains 2+ books with same content hash but may differ in
  path/metadata.
- UI shows duplicate count on Dashboard and allows navigation to duplicate
  management view.
- No crashes, no negative counts, no grouping of distinct files.

## 🚀 Quick Start

```bash
# Check if duplicate detection is implemented
rg "duplicates|SHA256|content_hash" internal -n | head -20

# Query duplicate API endpoint
curl -s http://localhost:8888/api/v1/audiobooks/duplicates | jq '.'

# Check if hashes are computed during scan
rg "hash|SHA256" internal/scanner internal/database -n | head -20

# Trigger scan to compute hashes
curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" | jq '.'
```

## 🔐 Multi-AI Safety

- Use lock/state files under `/tmp/task-4-*` to avoid concurrent runs.
- Test with read-only queries first; avoid deleting files during validation
  phase.
- Capture duplicate groups before attempting any cleanup operations.

## 🧭 Navigation

- Need the main flow? → `4-CORE-TESTING.md`
- Handling edge cases? → `4-ADVANCED-SCENARIOS.md`
- Something broken? → `4-TROUBLESHOOTING.md`

## 🧩 Current State (from TODO)

- Priority: Low (Optional MVP feature for library maintenance)
- Status: Implemented in v1.9.0 but not yet tested
- Related: Hash tracking for reimport prevention (Task 5)

## ✅ Next Actions

1. Run Core Phases (check implementation, scan to compute hashes, query API,
   verify groups).
2. If duplicates incorrect or endpoint missing, follow Troubleshooting.
3. Validate with known duplicate files (copy same file to multiple paths).
4. Log results and mark TODO when complete.

---
