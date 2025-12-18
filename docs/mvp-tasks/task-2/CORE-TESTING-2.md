<!-- file: docs/mvp-tasks/task-2/CORE-TESTING.md -->
<!-- version: 2.0.1 -->
<!-- guid: 2e3f4a5b-6c7d-8e9f-0a1b-2c3d4e5f6a7b -->

# Task 2: Core Testing - Separate Dashboard Counts

## 🎯 Goals

- Verify backend exposes `library_book_count`, `import_book_count`,
  `total_book_count`.
- Verify counts are accurate (total = library + import) after a scan.
- Verify Dashboard and Library pages display separate counts correctly.
- Keep process idempotent and safe for multi-agent execution.

## ⚠️ Multi-AI Safety (Lock Protocol)

Use per-user lock/state files to avoid interference.

```bash
LOCK_FILE="/tmp/task-2-lock-$(whoami).txt"
STATE_FILE="/tmp/task-2-state-$(date +%s%N).json"

# Acquire lock
if [ -f "$LOCK_FILE" ] && [ -s "$(cat $LOCK_FILE)" ]; then
  echo "❌ Task 2 already running (lock: $(cat $LOCK_FILE))" && exit 1
fi
echo "$STATE_FILE" > "$LOCK_FILE"

# Baseline snapshot (read-only)
cat > "$STATE_FILE" <<'EOF'
{
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "system_status": $(curl -s http://localhost:8888/api/v1/system/status | jq '.'),
  "book_count": $(curl -s http://localhost:8888/api/v1/audiobooks | jq '.count')
}
EOF

echo "✅ Baseline captured at $STATE_FILE"
```

## Phase 1: Pre-Checks (Read-Only)

```bash
echo "=== PHASE 1: PRE-CHECKS ==="

# Server health
curl -s http://localhost:8888/api/v1/health | jq .

# Current system status
curl -s http://localhost:8888/api/v1/system/status | jq '{library_book_count, import_book_count, total_book_count}'

# Current book count endpoint
curl -s http://localhost:8888/api/v1/audiobooks | jq '.count'
```

**Expectations:**

- Health returns status ok.
- System status has the three count fields.
- `total_book_count` equals the count endpoint (or is consistent with
  library+import).

## Phase 2: Backend Verification (Read-Only)

```bash
echo "=== PHASE 2: BACKEND VERIFICATION ==="

# Check implementation references
rg "library_book_count|import_book_count" internal/server/server.go

# Inspect status handler (quick peek)
sed -n '1550,1650p' internal/server/server.go | head -n 50
```

**What to confirm:**

- Aggregation of counts splits root dir vs import paths.
- Final JSON sets library/import/total and keeps them consistent.

## Phase 3: Frontend Verification (Read-Only)

```bash
echo "=== PHASE 3: FRONTEND VERIFICATION ==="

# Dashboard
rg "library_book_count|import_book_count" web/src/pages/Dashboard.tsx

# Library page
rg "library_book_count|import_book_count" web/src/pages/Library.tsx
```

**What to confirm:**

- Dashboard renders both counts with clear labels.
- Library page mirrors the separation (cards, stats, or headers).

## Phase 4: Functional Test (Scan + Validate)

Trigger a scan to refresh counts, then validate API and UI data. (Destructive
only in the sense of reprocessing counts; safe for data.)

```bash
echo "=== PHASE 4: SCAN AND VALIDATE ==="

# Trigger scan (force to update counts)
SCAN_OP=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" | jq -r '.operation_id')
echo "Operation: $SCAN_OP"

# Wait briefly for status update
sleep 2

# Check operation status
curl -s "http://localhost:8888/api/v1/operations/$SCAN_OP" | jq '{status, progress_info}'

# Re-check counts
curl -s http://localhost:8888/api/v1/system/status | jq '{library_book_count, import_book_count, total_book_count}'
```

**Validation rules:**

- `total_book_count = library_book_count + import_book_count`.
- Counts are non-negative.
- Counts align with known dataset (4 books in current library unless changed).

## Phase 5: UI Spot Check (Manual)

1. Open Dashboard: ensure two numbers: Library X, Import Y.
2. Open Library page: ensure both numbers shown or accessible in stats header.
3. If UI uses cache, refresh page after scan or hard reload.

## Phase 6: Cleanup

```bash
LOCK_FILE="/tmp/task-2-lock-$(whoami).txt"
if [ -f "$LOCK_FILE" ]; then
  STATE_FILE=$(cat "$LOCK_FILE")
  rm -f "$STATE_FILE" "$LOCK_FILE"
  echo "✅ Cleaned task 2 lock/state"
fi
```

## ✅ Completion Criteria

- Backend surfaces separate counts correctly.
- UI renders separate counts on Dashboard and Library.
- Scan updates counts consistently.
- No lint errors or API errors observed.

---

**Next:** If any discrepancy appears, go to `TASK-2-TROUBLESHOOTING.md`. For
edge cases or performance concerns, see `TASK-2-ADVANCED-SCENARIOS.md`.
