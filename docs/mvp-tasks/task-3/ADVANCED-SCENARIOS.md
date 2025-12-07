<!-- file: docs/TASK-3-ADVANCED-SCENARIOS.md -->
<!-- version: 1.0.0 -->
<!-- guid: 6f0b4de0-6e2d-4a90-bbd3-5e2e410a8b59 -->

# Task 3: Advanced Scenarios & Code Deep Dive (Size Reporting)

Use these scenarios when the core flow passes but edge conditions still cause negative or incorrect size totals.

## 🧮 Large Library / Overflow Defense

**Risk:** `int64` overflow or mis-cast to `int` when summing large trees.

```bash
rg "size_bytes" internal -g'*.go' | rg "int32|int" -n
```

- Ensure accumulators use `int64` (or `uint64` with safe checks) before JSON marshal.
- Watch for conversions to `int` in DTOs or API responses.

## 🔗 Symlinks and Double Counting

**Risk:** Symlinks inside library/import paths inflating totals or pointing outside intended scope.

```bash
find "$ROOT_DIR" -type l -maxdepth 5 2>/dev/null
find $IMPORT_DIRS -type l -maxdepth 5 2>/dev/null
```

- Decide: follow or skip? Prefer skipping symlinks in size calc to avoid double counting.
- If following, guard against loops (track visited inodes) to prevent runaway totals.

## 🧭 Misconfigured Paths (Import Inside Library)

**Risk:** Import path nested under `root_dir` leads to double counting or negative adjustments.

- Detect nesting: ensure import paths are not children of `root_dir`.
- If nesting exists, treat imports outside `root_dir` only; warn in API/UI.

## 🗑️ Deleted or Inaccessible Files

**Risk:** Stale DB entries referencing missing files skew totals or go negative if subtraction logic exists.

```bash
# Spot missing files
psql or sqlite3 to query books table for file_path, then test -f
```

- Rebuild size totals from filesystem, not DB, when possible.
- Purge missing file entries or mark skipped with zero size.

## 🧹 Temporary / Hidden Files

**Risk:** `du` includes temp files that the scanner skips (or vice versa), causing mismatch.

- Align inclusion rules (hidden files, temp extensions) between scanner and size calc.
- Document exclusions in API response or logs.

## 📡 Concurrent Operations

**Risk:** Organize/scan running while measuring sizes.

- Use locks from Core doc; avoid measuring mid-operation.
- If simultaneous, expect minor drift; rerun after operations finish.

## 🧰 Backend Code Checklist

- Size computation function uses `int64`, zero-initialized, no subtraction of unsigned.
- Uses `filepath.Clean` and consistent path normalization before classification.
- Classification rule: `strings.HasPrefix(path, rootDir)` for library vs import.
- Ignores symlinks or guards against cycles.
- Returns JSON with explicit fields; UI reads those fields directly.

## 🪛 Frontend Checklist

- UI reads `library_size_bytes` and `import_size_bytes` and sums client-side only for display (no re-derivation from counts).
- Formatting uses 64-bit safe values; avoid JavaScript `Number` overflow by using bigint-aware formatting if sizes > 2^53 (unlikely but note).
- Refresh after scans (watch SSE/WebSocket events or refetch after scan completion).

When an edge condition is identified, document the reproduction and fix in `TASK-3-TROUBLESHOOTING.md`.
