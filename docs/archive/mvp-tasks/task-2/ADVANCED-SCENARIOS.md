<!-- file: docs/mvp-tasks/task-2/ADVANCED-SCENARIOS.md -->
<!-- version: 2.0.1 -->
<!-- guid: 3f4a5b6c-7d8e-9f0a-1b2c-3d4e5f6a7b8c -->
<!-- last-edited: 2026-01-19 -->

# Task 2: Advanced Scenarios & Code Deep Dive

## 📚 Reading Guide

- Part 1: `TASK-2-CORE-TESTING.md` (complete first)
- Part 2: `TASK-2-ADVANCED-SCENARIOS.md` (this file)
- Part 3: `TASK-2-TROUBLESHOOTING.md` (if issues arise)

## Scenario A: Large Library / Many Import Paths

**Purpose:** Confirm counts stay correct with many folders and large item sets.

### Steps (Scenario A)

```bash
echo "=== SCENARIO A: LARGE LIBRARY ==="

# Count audio files across library + import paths
find /Users/jdfalk/ao-library -type f \( -name "*.m4b" -o -name "*.mp3" \) | wc -l

# Trigger force scan
SCAN_OP=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" | jq -r '.operation_id')

# Monitor counts during scan
watch -n 3 'curl -s http://localhost:8888/api/v1/system/status | jq '{"lib":.library_book_count,"imp":.import_book_count,"tot":.total_book_count}'
```

**Expected:**

- Counts remain non-negative and consistent.
- Total matches library + import.

## Scenario B: Mixed Paths (Library + Import) With Overlaps

**Purpose:** Ensure files inside root dir count as library, outside as import.

### Steps (Scenario B)

```bash
echo "=== SCENARIO B: MIXED PATHS ==="

# List configured paths
curl -s http://localhost:8888/api/v1/system/status | jq '{root_directory, import_paths}'

# Verify classification logic in code
rg "strings.HasPrefix" internal/server/server.go
```

**Expected:**

- RootDir prefix defines library membership.
- Import paths excluded from library count.

## Scenario C: UI Caching / Stale Data

**Purpose:** Ensure UI updates after scans and does not show stale counts.

### Steps (Scenario C)

```bash
echo "=== SCENARIO C: UI CACHE ==="

# Trigger scan
SCAN_OP=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan" | jq -r '.operation_id')

# After completion, force UI reload
# In browser: hard refresh (Cmd+Shift+R) on Dashboard & Library
```

**Expected:**

- UI reflects latest counts post-scan.
- No stuck values in Dashboard cards.

## Scenario D: API Contract Verification

**Purpose:** Validate response schema is stable.

### Steps

```bash
echo "=== SCENARIO D: API CONTRACT ==="

curl -s http://localhost:8888/api/v1/system/status | jq '{library_book_count, import_book_count, total_book_count, root_directory, import_paths}'

# Optional: simple schema check
curl -s http://localhost:8888/api/v1/system/status | jq 'with_entries(select(["library_book_count","import_book_count","total_book_count","root_directory","import_paths"] | index(.key)))'
```

**Expected:**

- Fields exist and are typed as numbers (for counts).
- No breaking changes or renamed fields.

## Code Deep Dive (Pointers)

- **Backend aggregation:** `internal/server/server.go` (status handler). Check
  how root directory vs import paths are counted.
- **Scanner results:** Count calculation likely near scan completion; search for
  `library_book_count` usages.
- **Frontend Dashboard:** `web/src/pages/Dashboard.tsx` for rendering of
  library/import counts.
- **Frontend Library page:** `web/src/pages/Library.tsx` for any per-path
  display.

## Validation Checklist (Advanced)

- [ ] Large data set completes with consistent counts.
- [ ] Mixed path classification correct (root → library, others → import).
- [ ] UI updates after scan (no stale cached values).
- [ ] API schema stable and typed correctly.

---

**Next:** If any failures appear, jump to `TASK-2-TROUBLESHOOTING.md`.
Otherwise, record results and close TODO item.
