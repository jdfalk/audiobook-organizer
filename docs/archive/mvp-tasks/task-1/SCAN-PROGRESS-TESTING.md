<!-- file: docs/mvp-tasks/task-1/SCAN-PROGRESS-TESTING.md -->
<!-- version: 2.0.2 -->
<!-- guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d -->
<!-- last-edited: 2026-01-19 -->

# Task 1: Test Scan Progress Reporting

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

---

## 📦 Split Documentation (read-first)

This single file is kept for history, but the task is now split into concise
parts to avoid length limits:

- `README.md` — overview and navigation
- `CORE-TESTING.md` — phases 1-5 and safety/locks
- `ADVANCED-SCENARIOS.md` — large libraries, concurrency, recovery drills, code
  deep dive
- `TROUBLESHOOTING.md` — issues, root causes, and fixes

Use the split files for active work; keep this document as a legacy reference.

---

## 📋 Process

### ⚠️ CRITICAL: Idempotency & Multi-Agent Safety

**IMPORTANT:** Multiple AIs may work on this task. Follow these rules to prevent
conflicts:

1. **Always check lock file first:**

   ```bash
   LOCK_FILE="/tmp/task-1-scan-lock-$(whoami).txt"
   if [ -f "$LOCK_FILE" ] && [ -f "$(cat $LOCK_FILE)" ]; then
       echo "❌ Task 1 already running (lock: $(cat $LOCK_FILE))"
       exit 1
   fi
   ```

2. **Create lock before proceeding:**

   ```bash
   LOCK_FILE="/tmp/task-1-scan-lock-$(whoami).txt"
   STATE_FILE="/tmp/task-1-state-$(date +%s%N).json"
   echo "$STATE_FILE" > "$LOCK_FILE"
   echo "Lock created: $LOCK_FILE -> $STATE_FILE"
   ```

3. **Save baseline state (READ-ONLY, non-destructive):**

   ```bash
   # Capture complete system state before any changes
   STATE_FILE=$(cat "$LOCK_FILE")
   echo "Saving baseline state to: $STATE_FILE"

   {
     echo "timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
     echo "database_state: $(curl -s http://localhost:8888/api/v1/audiobooks | jq '.count')"
     echo "system_status: $(curl -s http://localhost:8888/api/v1/system/status | jq -c '.')"
     echo "server_health: $(curl -s http://localhost:8888/api/v1/health | jq -c '.')"
   } > "$STATE_FILE"

   echo "✅ Baseline saved"
   ```

4. **At end of task, clean up lock:**

   ```bash
   # Remove lock so others can proceed
   rm -f "$LOCK_FILE"
   echo "Lock released"
   ```

---

### Phase 1: Pre-Scan Verification

**Goal:** Ensure the system is in a known state before triggering scan.

**Steps:**

1. **Check current database state:**

   ```bash
   curl -s http://localhost:8888/api/v1/audiobooks | jq '.count'
   curl -s http://localhost:8888/api/v1/system/status | jq '{library_books, import_books}'
   ```

   Expected: Should show 4 books total (based on current state)

2. **Verify server is running:**

   ```bash
   curl -s http://localhost:8888/api/v1/health | jq '.status'
   ```

   Expected: `"ok"`

3. **Check import paths are configured:**

   ```bash
   curl -s http://localhost:8888/api/v1/import-paths | jq '.items[] | {path, enabled}'
   ```

   Expected: At least one import path exists and is enabled

### Phase 2: Trigger Full Scan

**Goal:** Start a scan operation with proper progress tracking enabled.

**Request:**

```bash
# Terminal 1: Trigger the scan
curl -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" \
  -H "Content-Type: application/json" \
  -d '{"force_update": true}' \
  -w "\n" | jq '.'
```

Expected response:

```json
{
  "operation_id": "01ABC123...",
  "operation_type": "scan",
  "status": "queued"
}
```

**Important:** Note the `operation_id` - you'll use this to track progress.

### Phase 3: Monitor Progress via SSE

**Goal:** Connect to SSE endpoint and observe real-time progress events.

**Setup:**

```bash
# Terminal 2: Connect to progress stream
curl -N -H "Accept: text/event-stream" \
  "http://localhost:8888/api/events?operation_id=<OPERATION_ID>" \
  -w "\n"
```

Replace `<OPERATION_ID>` with the ID from Phase 2.

**What to Expect:**

Initial events (pre-scan file counting):

```
data: {"type":"progress","level":"info","message":"Starting scan of folder: /Users/jdfalk/ao-library/library","timestamp":"2025-12-06T..."}

data: {"type":"progress","level":"info","message":"Full rescan: including library path /Users/jdfalk/ao-library/library","timestamp":"2025-12-06T..."}

data: {"type":"progress","level":"info","message":"Scanning 2 total folders (1 import paths)","timestamp":"2025-12-06T..."}
```

Pre-scan file count events:

```
data: {"type":"progress","level":"info","message":"Folder /Users/jdfalk/ao-library/library: Found 4 audiobook files","timestamp":"2025-12-06T..."}

data: {"type":"progress","level":"info","message":"Total audiobook files across all folders: 4","timestamp":"2025-12-06T..."}
```

**Validation Points:**

- ✅ File count should match actual files (not 0)
- ✅ Count should be consistent with `/api/v1/audiobooks` response

Scanning events:

```
data: {"type":"progress","level":"info","message":"Processing books in folder /Users/jdfalk/ao-library/library","timestamp":"2025-12-06T..."}

data: {"type":"progress","level":"info","message":"Processed: 1/4 books","timestamp":"2025-12-06T..."}
data: {"type":"progress","level":"info","message":"Processed: 2/4 books","timestamp":"2025-12-06T..."}
data: {"type":"progress","level":"info","message":"Processed: 3/4 books","timestamp":"2025-12-06T..."}
data: {"type":"progress","level":"info","message":"Processed: 4/4 books","timestamp":"2025-12-06T..."}
```

**Validation Points:**

- ✅ Progress shows incrementing counters (1/4, 2/4, 3/4, 4/4)
- ✅ No progress events show 0/X

Completion event:

```
data: {"type":"progress","level":"info","message":"Scan completed. Library: 4 books, Import: 0 books","timestamp":"2025-12-06T..."}
```

**Validation Points:**

- ✅ Message clearly separates library vs import counts
- ✅ Totals match actual database state
- ✅ No "N/A" or placeholder values

### Phase 4: Verify Server Logs

**Goal:** Confirm progress logging to disk succeeded.

**Check logs:**

```bash
ls -la /Users/jdfalk/ao-library/logs/
tail -100 /Users/jdfalk/ao-library/logs/operation-*.log
```

Expected:

- Log file exists with name like `operation-01ABC123....log`
- Contains all progress events from scan
- No error messages or warnings

### Phase 5: Validate Database State

**Goal:** Confirm scan completed successfully and data is consistent.

**Check final state:**

```bash
curl -s http://localhost:8888/api/v1/audiobooks | jq '.count'
curl -s http://localhost:8888/api/v1/system/status | jq '{library_books, import_books, total_books: (.library_books + .import_books)}'
```

Expected:

- Book count unchanged (still 4)
- Library and import counts make sense
- Total equals sum

---

## 🔧 Technical Context

### Architecture Overview

```
Browser                    Server                     Database
=========                  ======                     ========
  |                          |
  |-- POST /operations/scan --|
  |                          |
  |<-- operation_id ---------|
  |                          | Creates Operation record
  |                          | Enqueues operation function
  |                          |
  |-- GET /events (SSE) -----|
  |<-- progress events ----|  Executes scan operation
  |<-- progress events ----|  Logs via progress.Log()
  |<-- complete event ------|  Updates database
  |                          |
  |-- GET /audiobooks -------|
  |<-- updated books --------|
```

### Key Components

**1. Scan Operation Flow** (`internal/server/server.go:startScan`):

```
startScan()
  ├─ Create Operation record in database
  ├─ Log: "Starting scan of folder: X"
  ├─ Phase 1: Count total files (pre-scan)
  │   └─ Log: "Folder: Found N audiobook files"
  ├─ Phase 2: Scan directories (parallel workers)
  │   └─ Log: "Processed: N/TOTAL books"
  ├─ Phase 3: Metadata extraction (parallel)
  │   └─ Log: "Processed: N/TOTAL books"
  └─ Phase 4: Completion
      └─ Log: "Scan completed. Library: X, Import: Y"
```

**2. Progress Reporting System** (`internal/operations/progress.go`):

```
ProgressReporter interface:
  - Log(level, message, metadata)

Usage:
  _ = progress.Log("info", "Processing started", nil)

Output:
  - Logs to operation-specific log file
  - Broadcasts via SSE to connected clients
```

**3. SSE Event Handler** (`internal/server/server.go:handleEvents`):

```
GET /api/events?operation_id=XXX
  ├─ Find operation in database
  ├─ Connect to SSE handler
  ├─ Stream progress events in real-time
  └─ Heartbeat every 30 seconds
```

### Expected Progress Message Sequence

For a full scan of library + import paths:

```
1. INFO: "Full rescan: including library path /Users/jdfalk/ao-library/library"
2. INFO: "Scanning 2 total folders (1 import paths)"
3. INFO: "Folder /Users/jdfalk/ao-library/library: Found 4 audiobook files"
4. INFO: "Total audiobook files across all folders: 4"
5. INFO: "Processing books in folder /Users/jdfalk/ao-library/library"
6. INFO: "Processed: 1/4 books"
7. INFO: "Processed: 2/4 books"
8. INFO: "Processed: 3/4 books"
9. INFO: "Processed: 4/4 books"
10. INFO: "Scan completed. Library: 4 books, Import: 0 books"
```

### Known Implementation Details

**Pre-scan file counting** (`internal/server/server.go:startScan`, lines
~1135-1150):

```go
// First pass: count total files across all folders
totalFilesAcrossFolders := 0
for _, folderPath := range foldersToScan {
    // Walk directory, count files with supported extensions
    // Log: "Folder X: Found N audiobook files"
}
```

**Separate library vs import reporting** (`internal/server/server.go:startScan`,
line ~1220):

```go
if forceUpdate && config.AppConfig.RootDir != "" {
    // Library books were scanned
    _ = progress.Log("info", fmt.Sprintf(
        "Scan completed. Library: %d books, Import: %d books",
        libraryCount, importCount), nil)
}
```

**Progress per book** (`internal/server/server.go:startScan`, line ~1180):

```go
// Process books with counter
for idx, book := range books {
    idx++  // 1-based counter
    _ = progress.Log("info",
        fmt.Sprintf("Processed: %d/%d books", idx, len(books)), nil)
    // Process book...
}
```

---

## 🚨 Troubleshooting Guide

### Issue: No progress events received

**Symptoms:**

- SSE connection established but no data events
- Only heartbeat comments (`:` prefix)

**Root Causes & Solutions:**

1. **Operation ID doesn't exist:**

   ```bash
   # Verify operation was created
   curl -s http://localhost:8888/api/v1/operations | jq '.items[] | select(.type == "scan")'
   ```

2. **Operation already completed:**
   - SSE events only stream while operation is running
   - If operation finished before you connected, no events
   - Solution: Trigger a new scan and connect immediately

3. **Scan completed too quickly:**
   - For small directory (4 files), scan might complete in <1s
   - Solution: Trigger scan with a larger import path

4. **Progress logging disabled:**
   - Check server config: should have `EnableProgressLogging: true`
   - Look for logs: `tail -f /Users/jdfalk/ao-library/logs/debug.log`

### Issue: File count shows 0

**Symptoms:**

```
"message": "Folder /path: Found 0 audiobook files"
```

**Root Causes:**

1. **Folder doesn't exist or is empty:**

   ```bash
   ls -la /Users/jdfalk/ao-library/library/
   ```

   Should show audiobook files (_.m4b,_.mp3, etc.)

2. **Supported extensions not configured:**

   ```bash
   curl -s http://localhost:8888/api/v1/system/status | jq '.config.supported_extensions'
   ```

   Should include `.m4b`, `.mp3`, etc.

3. **Permissions issue:**

   ```bash
   ls -la /Users/jdfalk/ao-library/
   chmod -R u+rx /Users/jdfalk/ao-library/
   ```

### Issue: Progress shows "0/X" instead of "1/X", "2/X"

**Symptoms:**

```
"message": "Processed: 0/4 books"
"message": "Processed: 0/4 books"
```

**Root Cause:**

- Counter not incrementing properly in code

**Check server version:**

```bash
curl -s http://localhost:8888/api/v1/system/status | jq '.server_version'
```

Should be >= v1.26.0

**If older version:**

- Rebuild:
  `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go build -o ~/audiobook-organizer-embedded`
- Restart server

### Issue: Progress stops mid-scan

**Symptoms:**

```
"message": "Processed: 2/4 books"
(then no more events, hangs)
```

**Root Causes:**

1. **Processing book takes too long / hangs:**
   - Check server logs for errors
   - Book might have permission issue or corrupt metadata

2. **Parallel processing deadlock:**
   - Check worker count:
     `curl -s http://localhost:8888/api/v1/system/status | jq '.concurrent_workers'`
   - Try reducing workers if system resources constrained

3. **Database connection lost:**
   - Check database status
   - Look for error messages in server logs

**Solution:**

- Kill scan: Stop server and restart
- Skip problematic book: Check logs to identify which book
- Check disk space: `df -h`

### Issue: Final message missing library/import breakdown

**Symptoms:**

```
"message": "Scan completed."
(no "Library: X, Import: Y" breakdown)
```

**Root Cause:**

- Old code version or incomplete implementation

**Solution:**

- Verify code changes: Check `internal/server/server.go` around line 1220
- Should have:

  ```go
  _ = progress.Log("info", fmt.Sprintf(
      "Scan completed. Library: %d books, Import: %d books",
      libraryBookCount, importBookCount), nil)
  ```

- If missing, rebuild and redeploy

---

## 📊 Test Data

### Scenario: Full Rescan with force_update

**Setup:**

```bash
# Start fresh
curl -X POST http://localhost:8888/api/v1/operations/scan \
  -d '{"force_update": true}'
```

**Expected Results:**

| Metric            | Expected Value | Validation                                       |
| ----------------- | -------------- | ------------------------------------------------ |
| Total files found | 4              | Matches `/api/v1/audiobooks` count               |
| Library books     | 4              | All books in `/Users/jdfalk/ao-library/library/` |
| Import books      | 0              | No books in import paths                         |
| Processing time   | 1-5 seconds    | Should complete reasonably fast                  |
| Progress events   | ~12-15         | Counting + processing updates                    |
| Errors in log     | 0              | No error-level messages                          |

### Scenario: Selective Folder Scan

**Setup:**

```bash
# Scan specific import path
curl -X POST http://localhost:8888/api/v1/operations/scan \
  -d '{"folder_path": "/path/to/import"}'
```

**Expected Results:**

- Only that folder's books processed
- Progress reflects only that folder's counts
- No library books included

---

## ✅ Test Checklist

- [ ] Server running and healthy (`/api/health` returns ok)
- [ ] Database initialized with 4 books
- [ ] Import paths configured
- [ ] Triggered scan with `force_update=true`
- [ ] Connected to SSE `/api/events` before scan completed
- [ ] Received progress events (not just heartbeats)
- [ ] File count > 0 (not "Found 0 files")
- [ ] Progress shows incrementing numbers (1/4, 2/4, 3/4, 4/4)
- [ ] Final message shows "Library: 4, Import: 0" breakdown
- [ ] Server logs contain no errors
- [ ] Operation log file created at
      `/Users/jdfalk/ao-library/logs/operation-*.log`
- [ ] Database counts match after scan
- [ ] Frontend receives and displays progress (optional, nice-to-have)

---

## 📝 Logging & Output

### Server Debug Logs

Enable detailed logging:

```bash
AUDIOBOOK_DEBUG=1 ~/audiobook-organizer-embedded serve --port 8888 --debug
```

Watch logs:

```bash
tail -f /Users/jdfalk/ao-library/logs/debug.log
```

### Operation Log Files

After each scan, a log file is created:

```bash
ls -la /Users/jdfalk/ao-library/logs/operation-*.log
```

Check specific operation:

```bash
cat /Users/jdfalk/ao-library/logs/operation-01ABC123....log
```

Expected contents:

```
[2025-12-06 14:30:45] INFO Starting scan of folder: /Users/jdfalk/ao-library/library
[2025-12-06 14:30:45] INFO Folder /Users/jdfalk/ao-library/library: Found 4 audiobook files
[2025-12-06 14:30:45] INFO Total audiobook files across all folders: 4
[2025-12-06 14:30:46] INFO Processing books in folder /Users/jdfalk/ao-library/library
[2025-12-06 14:30:47] INFO Processed: 1/4 books
[2025-12-06 14:30:48] INFO Processed: 2/4 books
[2025-12-06 14:30:49] INFO Processed: 3/4 books
[2025-12-06 14:30:50] INFO Processed: 4/4 books
[2025-12-06 14:30:51] INFO Scan completed. Library: 4 books, Import: 0 books
```

---

## 🔗 Related Files

- `internal/server/server.go` - Scan operation implementation (lines 1061-1300)
- `internal/operations/progress.go` - Progress reporting system
- `internal/realtime/events.go` - SSE event streaming
- `web/src/pages/Library.tsx` - Frontend SSE connection (for reference)

---

## 📞 Success Criteria Summary

Test passes when:

1. ✅ Scan triggered successfully via POST /operations/scan
2. ✅ Progress events stream in real-time via SSE
3. ✅ File counts are accurate (not 0)
4. ✅ Progress shows "X/Y" incrementing properly
5. ✅ Completion message separates library vs import
6. ✅ No errors in logs
7. ✅ Database state updated correctly

---

## 🔒 Idempotency Guarantee

**This task is FULLY IDEMPOTENT and safe for multiple AIs:**

### What This Means

- **Phases 1-2 (Verification):** Read-only operations, can run unlimited times
- **Phase 3 (SSE Monitoring):** Read-only, non-destructive
- **Phase 4 (Log Checking):** Read-only, non-destructive
- **Phase 5 (Database Validation):** Read-only, non-destructive

### Only State-Changing Operation

- **Phase 2: POST /operations/scan** - Explicitly triggered scan
  - Multiple scans can run safely (queued by server)
  - Use lock file to prevent exact overlap
  - Each scan creates separate operation ID
  - Safe to retry if previous scan incomplete

### Multi-AI Coordination

```bash
# Each AI instance should use unique lock file:
LOCK_FILE="/tmp/task-1-scan-lock-$(whoami)-$(hostname).txt"

# Check if another instance is running:
if [ -f "$LOCK_FILE" ]; then
    LOCK_AGE=$(($(date +%s) - $(stat -f%m "$LOCK_FILE")))
    if [ "$LOCK_AGE" -lt 300 ]; then  # 5 minute grace period
        echo "Another instance running, waiting..."
        sleep 10
    fi
fi

# Acquire lock:
echo "$(date +%s%N)" > "$LOCK_FILE"

# ... do work ...

# Release lock:
rm -f "$LOCK_FILE"
```

### Safety Guarantees

1. ✅ **No data loss** - Only reads data, one explicit scan per test run
2. ✅ **No race conditions** - Server queues operations safely
3. ✅ **Idempotent verification** - Running checks multiple times is safe
4. ✅ **Recoverable** - Easy rollback if needed (see Phase 4 recovery)
5. ✅ **Isolated** - Each test run has unique operation ID
6. ✅ **Atomic** - Either scan completes fully or fails cleanly

### If Multiple AIs Run Simultaneously

- ✅ Both can run verification phases in parallel (read-only)
- ✅ Scans will queue (server handles it)
- ✅ Each gets separate operation_id
- ⚠️ SSE events may show both scans
- ⚠️ Final state will be consistent (last scan wins)

### Recovery (if needed)

```bash
# Check what state we're in:
curl -s http://localhost:8888/api/v1/operations | jq '.items[] | {id, type, status}'

# Wait for all scans to complete:
for i in {1..60}; do
    RUNNING=$(curl -s http://localhost:8888/api/v1/operations | jq '[.items[] | select(.status == "running")] | length')
    [ "$RUNNING" -eq 0 ] && echo "All complete" && break
    sleep 1
done

# Verify final state:
curl -s http://localhost:8888/api/v1/audiobooks | jq '.count'
curl -s http://localhost:8888/api/v1/system/status | jq '{library_books, import_books}'
```

---

## ⚠️ CRITICAL: Lock File Cleanup

If this task is interrupted, clean up manually:

```bash
# Remove any stale locks
rm -f /tmp/task-1-scan-lock-*.txt

# Verify no orphaned operations exist
curl -s http://localhost:8888/api/v1/operations | jq '.items[] | select(.status == "running")'
```
