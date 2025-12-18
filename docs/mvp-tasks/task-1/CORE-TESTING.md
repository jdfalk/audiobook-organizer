<!-- file: docs/mvp-tasks/task-1/CORE-TESTING.md -->
<!-- version: 2.0.1 -->
<!-- guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d -->

# Task 1: Core Scan Progress Testing

## 🎯 Overall Goal

Verify that the scan progress reporting system (implemented in v1.26.0) works
correctly by:

1. Triggering a full scan with `force_update=true`
2. Observing real-time progress events via SSE
3. Validating that progress shows actual file counts (not 0/0)
4. Confirming completion message separates library vs import counts
5. Verifying progress is logged in `/Users/jdfalk/ao-library/logs/`

**Success Criteria:**

- Progress events show incrementing file counts during scan
- Final message clearly states "Library: X books, Import: Y books"
- No errors in server logs during scan
- Frontend receives and displays progress updates
- Complete SSE stream recorded with all events
- Database consistency verified post-scan

---

## 📋 Multi-Part Task Structure

This comprehensive task is split across three documents for clarity:

1. **CORE-TESTING.md** (this file) - Core testing phases and basic validation
2. **ADVANCED-SCENARIOS.md** - Advanced scenarios, code details, performance
   testing
3. **TROUBLESHOOTING.md** - Comprehensive troubleshooting guide and recovery
   procedures

**Read in this order for complete understanding:**

1. Start here to understand basic testing flow
2. Move to Advanced Scenarios for complex setups
3. Reference Troubleshooting only when issues occur

---

## ⚠️ CRITICAL: Idempotency & Multi-Agent Safety

**IMPORTANT:** Multiple AIs may work on this task. Follow these rules to prevent
conflicts:

### Lock File Protocol

**Always check lock file first:**

```bash
LOCK_FILE="/tmp/task-1-scan-lock-$(whoami).txt"
if [ -f "$LOCK_FILE" ] && [ -f "$(cat $LOCK_FILE)" ]; then
    echo "❌ Task 1 already running by $(cat $LOCK_FILE 2>/dev/null | head -1)"
    exit 1
fi
```

**Create lock before proceeding:**

```bash
LOCK_FILE="/tmp/task-1-scan-lock-$(whoami).txt"
STATE_FILE="/tmp/task-1-state-$(date +%s%N).json"
echo "$STATE_FILE" > "$LOCK_FILE"
echo "✅ Lock created: $LOCK_FILE"
```

**Save baseline state (READ-ONLY, non-destructive):**

```bash
STATE_FILE=$(cat "$LOCK_FILE")

# Capture complete system state before any changes
cat > "$STATE_FILE" << 'EOF'
{
  "baseline": {
    "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "book_count_before": $(curl -s http://localhost:8888/api/v1/audiobooks | jq '.count'),
    "system_status": $(curl -s http://localhost:8888/api/v1/system/status | jq '.'),
    "operations_running": $(curl -s http://localhost:8888/api/v1/operations | jq '[.items[] | select(.status == "processing")]')
  }
}
EOF

echo "✅ Baseline state saved to $STATE_FILE"
```

**At end of task, clean up lock:**

```bash
LOCK_FILE="/tmp/task-1-scan-lock-$(whoami).txt"
if [ -f "$LOCK_FILE" ]; then
    STATE_FILE=$(cat "$LOCK_FILE")
    rm -f "$STATE_FILE" "$LOCK_FILE"
    echo "✅ Lock and state files cleaned"
fi
```

---

## Phase 1: Pre-Scan Verification

**Goal:** Ensure the system is in a known state before triggering scan.

**Duration:** ~1 minute **Destructive:** No (read-only verification) **Can be
run by multiple agents:** Yes (all read-only)

### Step 1.1: Check Server Running

```bash
echo "=== PHASE 1: PRE-SCAN VERIFICATION ==="
echo ""

# Verify server is responding
HEALTH=$(curl -s http://localhost:8888/api/v1/health)
if [ $? -ne 0 ]; then
    echo "❌ Server not responding on localhost:8888"
    echo "   Start server with: ~/audiobook-organizer-embedded serve --port 8888 --debug"
    exit 1
fi

echo "✅ Server responding"
echo "   Health: $HEALTH" | jq .
```

**Expected Output:**

```json
{
  "status": "ok",
  "timestamp": "2025-12-06T14:30:00Z"
}
```

### Step 1.2: Verify Current Book Count

```bash
echo ""
echo "Checking current database state..."

CURRENT_COUNT=$(curl -s http://localhost:8888/api/v1/audiobooks | jq '.count')
echo "✅ Current book count: $CURRENT_COUNT"

# This should be consistent across multiple calls
VERIFY_COUNT=$(curl -s http://localhost:8888/api/v1/audiobooks | jq '.count')
if [ "$CURRENT_COUNT" != "$VERIFY_COUNT" ]; then
    echo "⚠️  Warning: Book count changed between queries ($CURRENT_COUNT → $VERIFY_COUNT)"
else
    echo "✅ Book count stable: $CURRENT_COUNT"
fi
```

**Expected:**

- Same count both times (indicates no concurrent operations)
- Should match number of books in `/Users/jdfalk/ao-library/library/`

### Step 1.3: Verify Import Paths Configured

```bash
echo ""
echo "Checking import path configuration..."

IMPORT_PATHS=$(curl -s http://localhost:8888/api/v1/system/status | jq -c '.import_paths // []')
echo "Import paths configured: $IMPORT_PATHS"

if [ "$(echo $IMPORT_PATHS | jq 'length')" -gt 0 ]; then
    echo "✅ Import paths exist (count: $(echo $IMPORT_PATHS | jq 'length'))"
else
    echo "ℹ️  No import paths configured - scan will only check library path"
fi
```

**Expected:**

- Either shows import paths array or empty array
- Should be same configuration as before

### Step 1.4: Check No Operations Running

```bash
echo ""
echo "Checking for running operations..."

RUNNING=$(curl -s http://localhost:8888/api/v1/operations | jq '[.items[] | select(.status == "processing")]')
if [ "$(echo $RUNNING | jq 'length')" -gt 0 ]; then
    echo "❌ Other operations already running:"
    echo $RUNNING | jq .
    exit 1
else
    echo "✅ No operations running - safe to proceed"
fi
```

**Expected:**

- Empty array `[]`
- If operations exist, they must complete first

### Step 1.5: Verify Logs Directory

```bash
echo ""
echo "Checking logs directory..."

if [ ! -d "/Users/jdfalk/ao-library/logs" ]; then
    mkdir -p /Users/jdfalk/ao-library/logs
    echo "✅ Created logs directory"
else
    echo "✅ Logs directory exists"
fi

# Verify write permissions
if [ -w "/Users/jdfalk/ao-library/logs" ]; then
    echo "✅ Logs directory is writable"
else
    echo "❌ Cannot write to logs directory - fix permissions:"
    echo "   chmod 755 /Users/jdfalk/ao-library/logs"
    exit 1
fi
```

**Expected:**

- Directory exists and is writable
- No permission errors

---

## Phase 2: Trigger Full Scan

**Goal:** Start a scan operation with proper progress tracking enabled.

**Duration:** Immediate (returns operation ID) **Destructive:** No (modifies
database but can be rolled back) **Can be run by multiple agents:** No (one at a
time)

### Step 2.1: Trigger Scan with Force Update

```bash
echo ""
echo "=== PHASE 2: TRIGGER FULL SCAN ==="
echo ""

# Trigger scan with force_update flag
# This ensures all books are reprocessed even if they exist
SCAN_REQUEST=$(curl -s -X POST \
  "http://localhost:8888/api/v1/operations/scan?force_update=true" \
  -H "Content-Type: application/json" \
  -w "\n")

echo "Scan request response:"
echo "$SCAN_REQUEST" | jq .

# Extract operation ID
OPERATION_ID=$(echo "$SCAN_REQUEST" | jq -r '.operation_id // empty')

if [ -z "$OPERATION_ID" ]; then
    echo "❌ Failed to get operation ID from response"
    echo "   Response was: $SCAN_REQUEST"
    exit 1
fi

echo ""
echo "✅ Scan triggered successfully"
echo "   Operation ID: $OPERATION_ID"

# Save for later use
echo "$OPERATION_ID" > /tmp/task-1-operation-id.txt
```

**Expected Response:**

```json
{
  "operation_id": "01ABC123XYZ...",
  "type": "scan",
  "created_at": "2025-12-06T14:30:45.123Z",
  "status": "queued"
}
```

**Important Notes:**

- The `force_update=true` flag means all books will be reprocessed
- Operation status starts as "queued"
- Should transition to "processing" within 1 second

### Step 2.2: Verify Operation Created

```bash
echo ""
echo "Verifying operation was created in database..."

# Give server 1 second to record operation
sleep 1

# Fetch the operation to verify it exists
OP_DETAILS=$(curl -s "http://localhost:8888/api/v1/operations/$OPERATION_ID")
echo "Operation details:"
echo "$OP_DETAILS" | jq .

OP_STATUS=$(echo "$OP_DETAILS" | jq -r '.status')
echo ""
echo "✅ Operation status: $OP_STATUS"

if [ "$OP_STATUS" != "queued" ] && [ "$OP_STATUS" != "processing" ]; then
    echo "⚠️  Unexpected status: $OP_STATUS"
fi
```

**Expected:**

- Operation exists in database
- Status is either "queued" or "processing"
- Timestamps are recent

---

## Phase 3: Monitor Progress via SSE

**Goal:** Connect to SSE endpoint and observe real-time progress events.

**Duration:** 5-30 seconds (depends on library size) **Destructive:** No
(read-only) **Can be run by multiple agents:** Yes (all reading same stream)

### Step 3.1: Connect to SSE Stream

```bash
echo ""
echo "=== PHASE 3: MONITOR PROGRESS VIA SSE ==="
echo ""

OPERATION_ID=$(cat /tmp/task-1-operation-id.txt)

echo "Connecting to SSE stream for operation: $OPERATION_ID"
echo "Press Ctrl+C to stop monitoring (operation will continue)"
echo ""
echo "Receiving events:"
echo "─────────────────────────────────────────────────────"

# Connect to SSE and collect all events
# Use timeout to limit capture duration
timeout 120 curl -N -H "Accept: text/event-stream" \
  "http://localhost:8888/api/events?operation_id=$OPERATION_ID" \
  2>/dev/null | tee /tmp/task-1-sse-events.txt

echo ""
echo "─────────────────────────────────────────────────────"
echo "✅ SSE stream capture complete"
```

**What to Expect:**

The event stream will show progression through several stages:

**Stage 1: Initial startup (events 1-4)**

```
event: progress
data: {"type":"progress","level":"info","message":"Starting scan of folder: /Users/jdfalk/ao-library/library","timestamp":"2025-12-06T14:30:45.123Z"}

event: progress
data: {"type":"progress","level":"info","message":"Full rescan: including library path /Users/jdfalk/ao-library/library","timestamp":"2025-12-06T14:30:45.124Z"}
```

**Validation for Stage 1:**

- ✅ First message indicates folder being scanned
- ✅ Message mentions "Full rescan" (because force_update=true)
- ✅ Timestamps are current

**Stage 2: Pre-scan file counting (events 5-7)**

```
event: progress
data: {"type":"progress","level":"info","message":"Scanning 2 total folders (1 import paths)","timestamp":"2025-12-06T14:30:45.125Z"}

event: progress
data: {"type":"progress","level":"info","message":"Folder /Users/jdfalk/ao-library/library: Found 4 audiobook files","timestamp":"2025-12-06T14:30:46.500Z"}

event: progress
data: {"type":"progress","level":"info","message":"Total audiobook files across all folders: 4","timestamp":"2025-12-06T14:30:46.501Z"}
```

**Validation for Stage 2:**

- ✅ File count should NOT be 0 (must match actual files)
- ✅ File count should be consistent across both messages
- ✅ Count matches number of m4b/mp3 files in
  `/Users/jdfalk/ao-library/library/`

**Stage 3: Processing initiation (event 8)**

```
event: progress
data: {"type":"progress","level":"info","message":"Processing books in folder /Users/jdfalk/ao-library/library","timestamp":"2025-12-06T14:30:46.502Z"}
```

**Validation:**

- ✅ Message indicates processing is starting
- ✅ Mentions correct folder path

**Stage 4: Per-book progress (events 9-12)**

```
event: progress
data: {"type":"progress","level":"info","message":"Processed: 1/4 books","timestamp":"2025-12-06T14:30:47.123Z"}

event: progress
data: {"type":"progress","level":"info","message":"Processed: 2/4 books","timestamp":"2025-12-06T14:30:48.456Z"}

event: progress
data: {"type":"progress","level":"info","message":"Processed: 3/4 books","timestamp":"2025-12-06T14:30:49.789Z"}

event: progress
data: {"type":"progress","level":"info","message":"Processed: 4/4 books","timestamp":"2025-12-06T14:30:51.012Z"}
```

**Validation for Stage 4 - CRITICAL:**

- ✅ Progress shows incrementing counters (1/4, then 2/4, then 3/4, then 4/4)
- ✅ NEVER shows 0/4 or 0/X (common bug)
- ✅ Denominator (total) matches Stage 2 file count
- ✅ Messages appear in sequence without gaps
- ✅ Time between events is reasonable (1-2 seconds per book)

**Stage 5: Completion (event 13)**

```
event: progress
data: {"type":"progress","level":"info","message":"Scan completed. Library: 4 books, Import: 0 books","timestamp":"2025-12-06T14:30:51.500Z"}

:heartbeat
```

**Validation for Stage 5 - CRITICAL:**

- ✅ Message clearly separates library vs import counts
- ✅ Library count matches pre-scan file count
- ✅ Import count makes sense (0 if no import paths, or actual count)
- ✅ Total (4+0=4) makes sense
- ✅ No "N/A" or "unknown" values
- ✅ Heartbeat continues after completion

### Step 3.2: Validate SSE Stream Quality

```bash
echo ""
echo "Validating SSE stream quality..."
echo ""

# Count events received
EVENT_COUNT=$(grep -c "^event:" /tmp/task-1-sse-events.txt)
echo "Total events received: $EVENT_COUNT"

if [ "$EVENT_COUNT" -lt 10 ]; then
    echo "⚠️  Warning: Expected at least 10 events, got $EVENT_COUNT"
fi

# Check for progress events with incrementing counts
echo ""
echo "Progress message sequence:"
grep "Processed:" /tmp/task-1-sse-events.txt | sed 's/.*"message":"//' | sed 's/".*//'

# Check for 0/X pattern (BUG indicator)
if grep -q '"Processed: 0/' /tmp/task-1-sse-events.txt; then
    echo "❌ BUG: Found 'Processed: 0/X' - counter not incrementing"
    exit 1
fi

# Verify completion message
if grep -q "Scan completed" /tmp/task-1-sse-events.txt; then
    echo "✅ Completion message found"
else
    echo "⚠️  No completion message found"
fi
```

**Expected Output:**

```
Total events received: 14
Progress message sequence:
Processed: 1/4 books
Processed: 2/4 books
Processed: 3/4 books
Processed: 4/4 books

✅ Completion message found
```

---

## Phase 4: Verify Log Files

**Goal:** Confirm progress logging to disk succeeded.

**Duration:** ~10 seconds **Destructive:** No (read-only) **Can be run by
multiple agents:** Yes

### Step 4.1: Locate Log File

```bash
echo ""
echo "=== PHASE 4: VERIFY LOG FILES ==="
echo ""

# Get operation ID
OPERATION_ID=$(cat /tmp/task-1-operation-id.txt)

# Find log file
LOG_FILE=$(find /Users/jdfalk/ao-library/logs -name "*$OPERATION_ID*" -type f 2>/dev/null | head -1)

if [ -z "$LOG_FILE" ]; then
    echo "❌ Log file not found for operation $OPERATION_ID"
    echo "   Checked: /Users/jdfalk/ao-library/logs/*"
    echo "   Available logs:"
    ls -la /Users/jdfalk/ao-library/logs/ 2>/dev/null || echo "   (logs directory empty)"
    exit 1
fi

echo "✅ Log file found: $LOG_FILE"
echo "   Size: $(ls -lh $LOG_FILE | awk '{print $5}')"
```

**Expected:**

- Log file exists with operation ID in name
- File is readable
- File has reasonable size (> 500 bytes)

### Step 4.2: Verify Log Contents

```bash
echo ""
echo "First 20 lines of log:"
head -20 "$LOG_FILE"

echo ""
echo "Last 10 lines of log:"
tail -10 "$LOG_FILE"

# Verify log contains expected messages
echo ""
echo "Checking for required log entries..."

if grep -q "Starting scan" "$LOG_FILE"; then
    echo "✅ Contains 'Starting scan'"
else
    echo "❌ Missing 'Starting scan' message"
fi

if grep -q "Processed:" "$LOG_FILE"; then
    echo "✅ Contains progress messages"
else
    echo "❌ Missing progress messages"
fi

if grep -q "Scan completed" "$LOG_FILE"; then
    echo "✅ Contains completion message"
else
    echo "❌ Missing completion message"
fi

if grep -iq "error\|fatal" "$LOG_FILE"; then
    echo "⚠️  Log contains errors:"
    grep -i "error\|fatal" "$LOG_FILE"
else
    echo "✅ No errors in log"
fi
```

**Expected:**

- Log contains all 4 required message types
- Log shows no errors or fatal conditions
- Log is well-formatted with timestamps

### Step 4.3: Compare SSE vs Log

```bash
echo ""
echo "Comparing SSE stream with log file..."

# Count events in each source
SSE_EVENT_COUNT=$(grep -c "^event:" /tmp/task-1-sse-events.txt 2>/dev/null || echo 0)
LOG_LINE_COUNT=$(wc -l < "$LOG_FILE")

echo "SSE stream events: $SSE_EVENT_COUNT"
echo "Log file lines: $LOG_LINE_COUNT"

if [ "$SSE_EVENT_COUNT" -gt 0 ]; then
    if [ "$LOG_LINE_COUNT" -ge "$SSE_EVENT_COUNT" ]; then
        echo "✅ Log contains at least as many lines as SSE events"
    else
        echo "⚠️  Log lines ($LOG_LINE_COUNT) < SSE events ($SSE_EVENT_COUNT)"
    fi
fi

# Save log file reference
echo "$LOG_FILE" > /tmp/task-1-log-file.txt
```

---

## Phase 5: Validate Database State

**Goal:** Confirm scan completed successfully and data is consistent.

**Duration:** ~20 seconds **Destructive:** No (read-only) **Can be run by
multiple agents:** Yes

### Step 5.1: Check Operation Status

```bash
echo ""
echo "=== PHASE 5: VALIDATE DATABASE STATE ==="
echo ""

OPERATION_ID=$(cat /tmp/task-1-operation-id.txt)

# Check operation status
OP_STATUS=$(curl -s "http://localhost:8888/api/v1/operations/$OPERATION_ID" | jq -r '.status')
echo "Operation status: $OP_STATUS"

if [ "$OP_STATUS" = "completed" ]; then
    echo "✅ Operation completed successfully"
elif [ "$OP_STATUS" = "failed" ]; then
    echo "❌ Operation failed"
    curl -s "http://localhost:8888/api/v1/operations/$OPERATION_ID" | jq .
    exit 1
else
    echo "ℹ️  Operation still in progress or unknown state: $OP_STATUS"
fi
```

**Expected:**

- Status is "completed"
- If still "processing", wait a bit and check again

### Step 5.2: Check Final Book Count

```bash
echo ""
echo "Getting final database state..."

FINAL_COUNT=$(curl -s http://localhost:8888/api/v1/audiobooks | jq '.count')
echo "Final book count: $FINAL_COUNT books"

# Compare to baseline
STATE_FILE=$(cat /tmp/task-1-scan-lock-$(whoami).txt 2>/dev/null)
if [ -f "$STATE_FILE" ]; then
    BASELINE_COUNT=$(jq -r '.baseline.book_count_before' "$STATE_FILE" 2>/dev/null)
    if [ -n "$BASELINE_COUNT" ] && [ "$BASELINE_COUNT" != "null" ]; then
        echo "Baseline count: $BASELINE_COUNT books"

        if [ "$FINAL_COUNT" -eq "$BASELINE_COUNT" ]; then
            echo "✅ Count stable (expected for force_update=true)"
        else
            echo "ℹ️  Count changed: $BASELINE_COUNT → $FINAL_COUNT"
        fi
    fi
fi
```

**Expected:**

- Final count should be stable
- Should match actual files in directory

### Step 5.3: Verify Library vs Import Breakdown

```bash
echo ""
echo "Checking library vs import breakdown..."

BREAKDOWN=$(curl -s http://localhost:8888/api/v1/system/status | jq '{library_book_count, import_book_count, total_book_count: (.library_book_count + .import_book_count)}')
echo "Breakdown:"
echo "$BREAKDOWN" | jq .

LIBRARY_COUNT=$(echo "$BREAKDOWN" | jq '.library_book_count')
IMPORT_COUNT=$(echo "$BREAKDOWN" | jq '.import_book_count')
TOTAL=$(echo "$BREAKDOWN" | jq '.total_book_count')

echo ""
if [ "$TOTAL" -eq "$FINAL_COUNT" ]; then
    echo "✅ Totals are consistent ($TOTAL = $FINAL_COUNT)"
else
    echo "⚠️  Total mismatch: system says $TOTAL, audiobooks endpoint says $FINAL_COUNT"
fi

if [ "$LIBRARY_COUNT" -gt 0 ]; then
    echo "✅ Library has $LIBRARY_COUNT books"
else
    echo "⚠️  Library count is 0"
fi

echo "✅ Import count: $IMPORT_COUNT books"
```

**Expected:**

- Library count > 0
- Total = library + import
- Consistent with final count

---

## Summary & Next Steps

**If all phases pass:**

1. ✅ Scan progress reporting is working correctly
2. ✅ Real-time SSE streaming is functioning
3. ✅ Log files are being created properly
4. ✅ Database state is consistent

**Next steps:**

- Move to **ADVANCED-SCENARIOS.md** for complex testing
- Reference **TROUBLESHOOTING.md** if issues occur

**Cleanup (run at end):**

```bash
# Clean up temporary state files
LOCK_FILE="/tmp/task-1-scan-lock-$(whoami).txt"
if [ -f "$LOCK_FILE" ]; then
    STATE_FILE=$(cat "$LOCK_FILE")
    rm -f "$STATE_FILE" "$LOCK_FILE" /tmp/task-1-operation-id.txt /tmp/task-1-sse-events.txt /tmp/task-1-log-file.txt
    echo "✅ Cleanup complete"
fi
```

---

**Document Version:** 2.0.1 **Last Updated:** December 6, 2025 **Total Lines:**
600+ **Total Words:** ~8,500
