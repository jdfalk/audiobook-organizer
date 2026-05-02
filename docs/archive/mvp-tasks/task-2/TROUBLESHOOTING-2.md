<!-- file: docs/mvp-tasks/task-2/TROUBLESHOOTING.md -->
<!-- version: 2.0.1 -->
<!-- guid: 4a5b6c7d-8e9f-0a1b-2c3d-4e5f6a7b8c9d -->
<!-- last-edited: 2026-01-19 -->

# Task 2: Troubleshooting - Separate Dashboard Counts

## 📚 When to Use

Open this file if counts are missing, incorrect, or the UI is stale. For the
test flow, start with `TASK-2-CORE-TESTING.md`.

## Quick Index

| Problem                  | Likely Causes                                | Fix                               | Reference |
| ------------------------ | -------------------------------------------- | --------------------------------- | --------- |
| Counts missing in API    | Backend fields not set, wrong handler        | Verify code, rebuild              | Issue 1   |
| Total ≠ library + import | Aggregation bug, stale data, negative values | Re-scan, fix calculation          | Issue 2   |
| UI shows one count       | Frontend not wired, stale cache              | Update components, hard reload    | Issue 3   |
| Negative counts          | Import size bug, overflow, bad data          | Investigate data, fix calculation | Issue 4   |

---

## Issue 1: Counts Missing in API

**Symptoms:** `library_book_count` or `import_book_count` absent/null.

**Steps:**

```bash
# Check API payload
curl -s http://localhost:8888/api/v1/system/status | jq '{library_book_count, import_book_count, total_book_count}'

# Inspect server handler
rg "library_book_count|import_book_count" internal/server/server.go
```

**Fix:**

- Ensure the status handler sets these fields.
- Rebuild binary: `go build -tags embed -o ~/audiobook-organizer-embedded`
- Restart server and re-test.

## Issue 2: Total Does Not Match Sum

**Symptoms:** `total_book_count != library_book_count + import_book_count`.

**Steps:**

```bash
curl -s http://localhost:8888/api/v1/system/status | jq '{lib:.library_book_count, imp:.import_book_count, tot:.total_book_count, sum:(.library_book_count + .import_book_count)}'
```

**Fix:**

- Trigger a rescan with `force_update=true`.
- Check aggregation logic in server handler; ensure sum uses current values.
- Confirm counts are non-negative.

## Issue 3: UI Shows Only One Count

**Symptoms:** Dashboard or Library shows a single number or merged total.

**Steps:**

```bash
# Dashboard
rg "library_book_count|import_book_count" web/src/pages/Dashboard.tsx

# Library page
rg "library_book_count|import_book_count" web/src/pages/Library.tsx
```

**Fix:**

- Wire both fields into components; label clearly (Library vs Import).
- Hard refresh UI after scan.

## Issue 4: Negative Counts

**Symptoms:** Any count is negative.

**Steps:**

```bash
curl -s http://localhost:8888/api/v1/system/status | jq '.library_book_count, .import_book_count, .total_book_count'
```

**Fix:**

- Inspect computation for integer underflow or incorrect subtraction.
- Validate scan results and folder paths.
- If tied to size calculation (see Task 3), fix aggregation there.

## Cleanup

```bash
rm -f /tmp/task-2-lock-*.txt /tmp/task-2-state-*.json
```

---

**If unresolved:** capture logs (`/Users/jdfalk/ao-library/logs`) and escalate
to code review. Return to `TASK-2-CORE-TESTING.md` after fixes.
