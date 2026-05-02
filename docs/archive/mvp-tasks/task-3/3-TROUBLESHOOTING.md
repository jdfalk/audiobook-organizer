<!-- file: docs/TASK-3-TROUBLESHOOTING.md -->
<!-- version: 1.0.0 -->
<!-- guid: e6b4c8d7-b8f7-4f94-9cb1-1a7fb1530e07 -->
<!-- last-edited: 2026-01-19 -->

# Task 3: Troubleshooting - Import Size Reporting

Use this guide when size fields are negative, inconsistent, or stale.

## Quick Index

| Problem                             | Likely Causes                               | Fix                           | Reference |
| ----------------------------------- | ------------------------------------------- | ----------------------------- | --------- |
| Negative `import_size_bytes`        | Overflow, bad subtraction, misclassified    | Fix aggregation logic, rescan | Issue 1   |
| Total ≠ library + import            | Double counting, missing files, rounding    | Normalize paths, recompute    | Issue 2   |
| UI shows stale or single size value | Frontend not wired or no refresh            | Update UI bindings, refetch   | Issue 3   |
| Mismatch vs `du` measurement        | Scanner excludes files, symlinks, temp data | Align inclusion rules         | Issue 4   |

---

## Issue 1: Negative Import Size

**Symptoms:** `import_size_bytes < 0` or extremely large values.

**Steps:**

```bash
# Capture current values
curl -s http://localhost:8888/api/v1/system/status | jq '{lib:.library_size_bytes, imp:.import_size_bytes, tot:.total_size_bytes}'

# Find aggregation code
rg "import_size_bytes" internal -n | head -20
```

**Fix:**

- Ensure accumulators use `int64` and never subtract from totals.
- Recompute after fix: rerun scan with `force_update=true`.
- If import path overlaps root_dir, treat nested imports as library or exclude.

## Issue 2: Total Does Not Match Sum

**Symptoms:** `total_size_bytes != library_size_bytes + import_size_bytes`.

**Steps:**

```bash
curl -s http://localhost:8888/api/v1/system/status | jq '{lib:.library_size_bytes, imp:.import_size_bytes, tot:.total_size_bytes, sum:(.library_size_bytes + .import_size_bytes)}'
```

**Fix:**

- Normalize paths; avoid counting the same folder in both buckets.
- Check for missing files in DB versus disk; remove stale records or set size=0.
- Recompute sizes using scan after corrections.

## Issue 3: UI Not Showing Separate Sizes

**Symptoms:** UI shows one size or stale value.

**Steps:**

```bash
rg "library_size_bytes|import_size_bytes" web/src -n
```

**Fix:**

- Wire both fields into Dashboard/Library components with clear labels.
- Ensure UI refetches after scan completion (SSE/WebSocket or manual refresh).
- Avoid client-side recomputation from counts; use API size fields directly.

## Issue 4: `du` vs API Mismatch

**Symptoms:** On-disk measurement differs notably from API values.

**Steps:**

```bash
# Measure on disk (read-only)
ROOT_DIR=$(curl -s http://localhost:8888/api/v1/config | jq -r '.root_dir // empty')
IMPORT_DIRS=$(curl -s http://localhost:8888/api/v1/config | jq -r '.import_paths[]?')

du -sk "$ROOT_DIR" 2>/dev/null | awk '{print $1*1024 " bytes"}'
for d in $IMPORT_DIRS; do du -sk "$d" 2>/dev/null | awk '{print $1*1024 " bytes"}'; done
```

**Fix:**

- Align inclusion/exclusion rules (hidden files, temp files, symlinks).
- Handle sparse files if present (may differ from logical size).
- After adjustments, rerun scan and re-verify.

## Cleanup

```bash
rm -f /tmp/task-3-lock.txt /tmp/task-3-state-*.json /tmp/task-3-measure-*.txt
```

If unresolved, capture server logs and open a code review referencing
`TASK-3-CORE-TESTING.md` results.
