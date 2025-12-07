<!-- file: docs/TASK-3-README.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7f1a9b7c-4d3a-4f50-8b82-2e9a6b2a1d70 -->

# Task 3: Fix Import Size Reporting - Complete Documentation

## 📖 Overview

This task fixes negative or incorrect import size reporting. The goal is to ensure API and UI show accurate, non-negative `library_size_bytes`, `import_size_bytes`, and `total_size_bytes`, with totals matching file reality even after scans and restarts.

**Deliverables:**

- API returns correct, non-negative size fields: `library_size_bytes`, `import_size_bytes`, `total_size_bytes` (total = library + import).
- UI surfaces library vs import sizes separately without negative or stale values.
- Scans and restarts keep sizes accurate; no integer overflow or sign errors on large libraries.
- Tests and troubleshooting steps validated.

## 📂 Document Set

| Document                       | Purpose                                        |
| ------------------------------ | ---------------------------------------------- |
| `TASK-3-CORE-TESTING.md`       | Core validation flow, phases, safety/locks     |
| `TASK-3-ADVANCED-SCENARIOS.md` | Edge cases (overflow, symlinks, partial disks) |
| `TASK-3-TROUBLESHOOTING.md`    | Issues, root causes, and fixes                 |
| `TASK-3-README.md` (this file) | Overview, navigation, quick commands           |

**Reading order:** README → Core → Advanced → Troubleshooting.

## 🎯 Success Criteria

- `/api/v1/system/status` exposes correct, non-negative size fields with `total_size_bytes = library_size_bytes + import_size_bytes`.
- Sizes match on-disk reality for both library and import paths.
- UI shows distinct Library vs Import sizes with no negative or wildly large (overflow) values.
- Scans, restarts, and rescans keep numbers stable.

## 🚀 Quick Start

```bash
# Check system status sizes
curl -s http://localhost:8888/api/v1/system/status | jq '{library_size_bytes, import_size_bytes, total_size_bytes}'

# Check config for root/import paths
curl -s http://localhost:8888/api/v1/config | jq '{root_dir, import_paths}'

# Trigger scan to refresh sizes (state-changing)
curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" | jq '.'

# Grep backend size aggregation
rg "size_bytes" internal/server internal | head -20
```

## 🔐 Multi-AI Safety

- Use lock/state files under `/tmp/task-3-*` (examples in Core doc) to avoid concurrent runs.
- Never delete or move library/import folders during validation.
- Capture pre/post API payloads before making changes.

## 🧭 Navigation

- Need the main flow? → `TASK-3-CORE-TESTING.md`
- Handling edge cases? → `TASK-3-ADVANCED-SCENARIOS.md`
- Something broken? → `TASK-3-TROUBLESHOOTING.md`

## 🧩 Current State (from TODO)

- Priority: Medium (MVP-blocking for accurate storage reporting)
- Symptom: `import_size_bytes` can go negative after scans
- Related: Task 2 counts; ensure totals remain aligned

## ✅ Next Actions

1. Run Core Phases (baseline capture, config check, size recomputation, scan).
2. If sizes mismatch or negative, follow Troubleshooting to pinpoint code/DB causes.
3. Validate after restart to ensure persistence.
4. Log results and mark TODO when complete.

---
