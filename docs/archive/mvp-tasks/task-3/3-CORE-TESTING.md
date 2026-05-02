<!-- file: docs/TASK-3-CORE-TESTING.md -->
<!-- version: 1.0.0 -->
<!-- guid: d5c9f6c4-5d0f-4f4c-8f61-30a9b99cfcd0 -->
<!-- last-edited: 2026-01-19 -->

# Task 3: Core Size Validation & Fix Flow

This file defines the core, repeatable flow to detect and fix negative or
incorrect import/library size reporting.

## 🔒 Safety & Locks

```bash
LOCK_FILE="/tmp/task-3-lock.txt"
STATE_FILE="/tmp/task-3-state-$(date +%s).json"
MEASURE_FILE="/tmp/task-3-measure-$(date +%s).txt"

if [ -f "$LOCK_FILE" ]; then
  echo "❌ Task 3 already running by $(cat $LOCK_FILE 2>/dev/null | head -1)" && exit 1
fi

echo "$(whoami)" > "$LOCK_FILE"
trap 'rm -f "$LOCK_FILE"' EXIT
```

## Phase 0: Baseline Capture (Read-Only)

```bash
# Capture current system status
curl -s http://localhost:8888/api/v1/system/status | tee "$STATE_FILE" | jq '{library_size_bytes, import_size_bytes, total_size_bytes, root_directory, import_paths}'

# Capture config for paths
curl -s http://localhost:8888/api/v1/config | jq '{root_dir, import_paths}'
```

Expected:

- `total_size_bytes = library_size_bytes + import_size_bytes`
- All values non-negative

## Phase 1: On-Disk Measurement (Read-Only)

```bash
ROOT_DIR=$(curl -s http://localhost:8888/api/v1/config | jq -r '.root_dir // empty')
IMPORT_DIRS=$(curl -s http://localhost:8888/api/v1/config | jq -r '.import_paths[]?')

# Measure library folder (safe: du read-only)
if [ -n "$ROOT_DIR" ]; then
  echo "Library ($ROOT_DIR)" | tee "$MEASURE_FILE"
  du -sk "$ROOT_DIR" 2>/dev/null | awk '{print $1*1024 " bytes"}' | tee -a "$MEASURE_FILE"
fi

# Measure import folders
for d in $IMPORT_DIRS; do
  echo "Import ($d)" | tee -a "$MEASURE_FILE"
  du -sk "$d" 2>/dev/null | awk '{print $1*1024 " bytes"}' | tee -a "$MEASURE_FILE"
done
```

Compare `du` totals to API values in `$STATE_FILE`.

## Phase 2: Backend Code Verification (Read-Only)

```bash
# Find size aggregation logic
rg "size_bytes" internal/server internal/database internal | head -30

# Inspect system status handler
rg "total_size_bytes" internal/server/server.go -n
```

Confirm:

- Library size adds only files under `root_dir`
- Import size adds files outside `root_dir` (import paths)
- Totals use `int64` and avoid overflow/underflow

## Phase 3: Recompute Sizes via Scan (State-Changing)

```bash
SCAN_OUT=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true")
OP_ID=$(echo "$SCAN_OUT" | jq -r '.operation_id // empty')

echo "Started scan: $OP_ID"

# Poll until done (timeout ~90s)
for i in {1..90}; do
  STATUS=$(curl -s "http://localhost:8888/api/v1/operations/$OP_ID" | jq -r '.status // empty')
  [ "$STATUS" = "completed" ] && break
  [ "$STATUS" = "failed" ] && break
  sleep 1
  [ $((i % 10)) -eq 0 ] && echo "... still running ($i s)"
done

# Capture post-scan status
curl -s http://localhost:8888/api/v1/system/status | jq '{library_size_bytes, import_size_bytes, total_size_bytes}'
```

Pass criteria:

- No negative values
- `total_size_bytes` equals sum
- Values move in expected direction after scan (no large jumps to
  negative/overflow)

## Phase 4: Restart & Persist

```bash
# Restart embedded binary (adjust path if different)
killall audiobook-organizer-embedded 2>/dev/null || true
sleep 2
~/audiobook-organizer-embedded serve --port 8888 --debug > /tmp/task-3-serve.log 2>&1 &
sleep 3

# Recheck after restart
curl -s http://localhost:8888/api/v1/system/status | jq '{library_size_bytes, import_size_bytes, total_size_bytes}'
```

Pass criteria: values unchanged (or changed only by real FS changes), never
negative.

## Phase 5: Cleanup

```bash
rm -f /tmp/task-3-lock.txt /tmp/task-3-state-*.json /tmp/task-3-measure-*.txt
```

If failures occur, switch to `TASK-3-TROUBLESHOOTING.md`.
