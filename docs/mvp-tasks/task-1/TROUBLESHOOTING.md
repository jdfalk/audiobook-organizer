<!-- file: docs/mvp-tasks/task-1/TROUBLESHOOTING.md -->
<!-- version: 2.0.1 -->
<!-- guid: c3d4e5f6-a7b8-9c0d-1e2f-3a4b5c6d7e8f -->

# Task 1: Troubleshooting & Recovery Guide

## 📚 Reading Guide

This is **Part 3** of the comprehensive Task 1 documentation:

- **Part 1:** TASK-1-CORE-TESTING.md (basic phases 1-5)
- **Part 2:** TASK-1-ADVANCED-SCENARIOS.md (advanced testing)
- **Part 3:** TASK-1-TROUBLESHOOTING.md (this file - troubleshooting)

**Reference this file** if you encounter issues during testing:
- Server not responding
- No progress events
- Stuck scans
- Database corruption
- SSE connection issues

---

## Troubleshooting Index

| Problem                 | Root Causes                                       | Reference |
| ----------------------- | ------------------------------------------------- | --------- |
| Server not responding   | Not started, crashed, port in use, binary corrupt | Issue #1  |
| No progress events      | Scan already done, wrong op ID, SSE broken        | Issue #2  |
| Progress shows 0/X      | Counter bug, old binary, concurrent bug           | Issue #3  |
| Progress stops mid-scan | File hang, deadlock, DB locked                    | Issue #4  |
| Wrong final message     | Books not scanned, RootDir wrong, wrong location  | Issue #5  |
| No log files created    | Directory missing, no permissions                 | Issue #6  |
| Locks stuck             | Previous failure, crashed agent, orphaned state   | Issue #7  |

---

## Issue #1: Server Not Responding

**Symptoms:**
```
curl: (7) Failed to connect to localhost port 8888: Connection refused
```

**Diagnosis Steps:**

```bash
# Step 1: Check if process running
pgrep -f "audiobook-organizer" | head -3
# Output: PID or (nothing)

# Step 2: Check port
lsof -i :8888
# Output: Process using port, or (nothing)

# Step 3: Check binary exists
ls -lh ~/audiobook-organizer-embedded
# Output: -rwxr-xr-x (executable)
```

### Root Cause A: Server Not Started

**Indicators:**
- `pgrep` returns nothing
- `lsof :8888` shows nothing
- Binary exists and is executable

**Solution:**

```bash
# Start fresh
killall audiobook-organizer-embedded 2>/dev/null || true
sleep 2

# Start with debug output
AUDIOBOOK_DEBUG=1 ~/audiobook-organizer-embedded serve --port 8888 --debug &

# Wait for startup
sleep 3

# Verify
curl http://localhost:8888/api/v1/health | jq .
```

**If still doesn't work:** Check startup logs:
```bash
# Look for error messages during startup
tail -20 /Users/jdfalk/ao-library/logs/debug.log 2>/dev/null | grep -i error
```

### Root Cause B: Server Crashed

**Indicators:**
- Process was running but now missing
- Previous errors in logs
- Recent core dump

**Solution:**

```bash
# Check for error messages
tail -50 /Users/jdfalk/ao-library/logs/debug.log | grep -i "panic\|fatal\|error"

# Clean up and restart
killall -9 audiobook-organizer-embedded 2>/dev/null || true
sleep 3
rm -f /tmp/audiobook-organizer-* 2>/dev/null || true

# Start with maximum logging
AUDIOBOOK_DEBUG=1 RUST_BACKTRACE=1 ~/audiobook-organizer-embedded serve --port 8888 --debug &
sleep 5

# Test
curl http://localhost:8888/api/v1/health | jq .

# Check for immediate errors
tail -5 /Users/jdfalk/ao-library/logs/debug.log
```

### Root Cause C: Port Already in Use

**Indicators:**
```
lsof -i :8888
COMMAND   PID USER   FD   TYPE            DEVICE SIZE/OFF NODE NAME
...other-app... 1234 user  3u  IPv4 0x... 0t0 TCP localhost:8888 (LISTEN)
```

**Solution:**

```bash
# Option 1: Kill the process using port
kill -9 1234  # Use PID from lsof output

# Option 2: Use different port
AUDIOBOOK_DEBUG=1 ~/audiobook-organizer-embedded serve --port 8889 --debug &
curl http://localhost:8889/api/v1/health

# Then use 8889 for rest of testing
OPERATION_ID=$(curl -s -X POST "http://localhost:8889/api/v1/operations/scan" | jq -r '.operation_id')
```

### Root Cause D: Binary Corrupted or Old

**Indicators:**
- Binary exists but is very small (<2MB)
- Binary runs but crashes immediately
- No embedded frontend assets

**Solution:**

```bash
# Check binary size (should be 5-15 MB)
ls -lh ~/audiobook-organizer-embedded
# If < 2MB, it's not properly built

# Rebuild binary
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go clean
go build -tags embed -o ~/audiobook-organizer-embedded

# Verify new size
ls -lh ~/audiobook-organizer-embedded

# Start rebuilt binary
killall audiobook-organizer-embedded 2>/dev/null || true
sleep 2
AUDIOBOOK_DEBUG=1 ~/audiobook-organizer-embedded serve --port 8888 --debug &
sleep 3
curl http://localhost:8888/api/v1/health | jq .
```

---

## Issue #2: No Progress Events

**Symptoms:**
```
SSE connection establishes
But only :heartbeat lines received
No "event: progress" lines
No "data:" lines
```

**Diagnosis Steps:**

```bash
# Step 1: Verify operation exists
OPERATION_ID=$(cat /tmp/task-1-operation-id.txt)
curl -s "http://localhost:8888/api/v1/operations/$OPERATION_ID" | jq .

# Step 2: Check operation status
curl -s "http://localhost:8888/api/v1/operations/$OPERATION_ID" | jq '.status'
# Output: "completed", "processing", "queued", or "failed"

# Step 3: Check log file
find /Users/jdfalk/ao-library/logs -name "*$OPERATION_ID*" -type f | head -1
# If empty, progress wasn't logged to disk

# Step 4: Verify SSE endpoint responds
curl -v "http://localhost:8888/api/events?operation_id=$OPERATION_ID" 2>&1 | head -20
```

### Root Cause A: Scan Already Completed

**Indicators:**
```
curl: status = "completed"
Log file exists with all events
No new SSE events coming
```

**Solution:**

```bash
# This is normal - scan already finished
# Trigger a NEW scan and connect immediately

OPERATION=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" | jq -r '.operation_id')
echo "Operation: $OPERATION"

# Connect within 1 second
sleep 0.2
timeout 60 curl -N "http://localhost:8888/api/events?operation_id=$OPERATION"
```

### Root Cause B: Wrong Operation ID

**Indicators:**
- Operation ID doesn't exist
- Curl returns 404
- No operation found in database

**Solution:**

```bash
# List all recent operations
curl -s "http://localhost:8888/api/v1/operations" | jq '.items[] | {id, status, type, created_at}' | head -10

# Find the most recent scan
RECENT_SCAN=$(curl -s "http://localhost:8888/api/v1/operations" | jq -r '.items[] | select(.type=="scan") | .id' | head -1)
echo "Recent scan: $RECENT_SCAN"

# Use correct operation ID
timeout 60 curl -N "http://localhost:8888/api/events?operation_id=$RECENT_SCAN"
```

### Root Cause C: SSE Not Implemented

**Indicators:**
- Any request to `/api/events` returns error
- Server logs show "endpoint not found"
- Only heartbeats, never any data

**Solution:**

```bash
# Check if realtime package is compiled in
strings ~/audiobook-organizer-embedded | grep -i "heartbeat" | head -1
# If found, realtime code is present

# Verify binary is recent
file ~/audiobook-organizer-embedded | grep -i "intel\|arm"

# Rebuild if needed
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git log -1 --format="%h %s" internal/realtime/
# Check if recent changes exist

# Rebuild to include latest
go build -tags embed -o ~/audiobook-organizer-embedded

# Restart and test
killall audiobook-organizer-embedded 2>/dev/null || true
sleep 2
AUDIOBOOK_DEBUG=1 ~/audiobook-organizer-embedded serve --port 8888 --debug &
sleep 3

OPERATION=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan" | jq -r '.operation_id')
timeout 30 curl -N "http://localhost:8888/api/events?operation_id=$OPERATION" | grep -E "event:|data:" | head -5
```

---

## Issue #3: Progress Shows 0/X

**Symptoms:**
```
event: progress
data: {..., "message": "Processed: 0/4 books", ...}
event: progress
data: {..., "message": "Processed: 0/4 books", ...}
```

**Indicators:**
- Counter stays at 0
- Denominator (total) is correct
- Only numerator is wrong

**Root Cause A: Counter Not Incrementing in Code**

**Where to Look:** `internal/server/server.go` around line 1180

```go
// ❌ WRONG (bug):
for idx, book := range books {
    _ = progress.Log("info", fmt.Sprintf("Processed: %d/%d", idx, len(books)), nil)
    // Missing: idx++
}

// ✅ CORRECT:
for idx, book := range books {
    idx++  // MUST increment
    _ = progress.Log("info", fmt.Sprintf("Processed: %d/%d", idx, len(books)), nil)
}

// OR using proper counter:
for processed := 0; processed < len(books); processed++ {
    _ = progress.Log("info", fmt.Sprintf("Processed: %d/%d", processed, len(books)), nil)
}
```

**Solution:**

1. Check if this bug exists:
```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
grep -n "Processed:" internal/server/server.go
# Look for context showing idx++ before the log call
```

2. If bug found, fix it:
```bash
# (use editor or sed to fix)
# After fixing, rebuild:
go build -o ~/audiobook-organizer-embedded

# Restart and test
killall audiobook-organizer-embedded 2>/dev/null || true
sleep 2
~/audiobook-organizer-embedded serve --port 8888 --debug &
sleep 3

OPERATION=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan" | jq -r '.operation_id')
timeout 30 curl -N "http://localhost:8888/api/events?operation_id=$OPERATION" | grep "Processed:"
```

**Root Cause B: Old Binary Version**

**Indicators:**
- Binary was built before v1.26.0
- Progress feature recently added
- Binary is older than v1.26.0

**Solution:**

```bash
# Check version
~/audiobook-organizer-embedded --version 2>/dev/null || echo "Version command not available"

# Check git history
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git log --oneline | grep -i "progress\|counter" | head -1

# Rebuild from latest
go build -o ~/audiobook-organizer-embedded

# Restart and test
killall audiobook-organizer-embedded 2>/dev/null || true
sleep 2
~/audiobook-organizer-embedded serve --port 8888 --debug &
sleep 3

# Verify fix
OPERATION=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" | jq -r '.operation_id')
timeout 30 curl -N "http://localhost:8888/api/events?operation_id=$OPERATION" | grep "Processed:" | head -3
```

---

## Issue #4: Progress Stops Mid-Scan

**Symptoms:**
```
Processed: 1/4 books
Processed: 2/4 books
(then nothing for 5+ minutes)
(scan never completes)
```

**Diagnosis:**

```bash
# Check if process still running
ps aux | grep audiobook-organizer | grep -v grep

# Check memory usage
ps aux | grep audiobook-organizer | awk '{print $6}'
# Memory in KB - if growing unbounded, memory leak

# Check server logs
tail -50 /Users/jdfalk/ao-library/logs/debug.log | tail -20
```

### Root Cause A: Processing Hangs on File

**Indicators:**
- Process still running
- CPU usage low (< 5%)
- Memory stable
- No new log entries

**Solution:**

```bash
# Identify which book might be problematic
# Check file that would be processed next
PROCESSED=$(grep "Processed:" /tmp/task-1-sse-events.txt | tail -1 | grep -o '[0-9]*/' | tr -d '/')
echo "Last processed: $PROCESSED"

# Find the problematic file
find /Users/jdfalk/ao-library/library -type f \( -name "*.m4b" -o -name "*.mp3" \) | sort | sed -n "$((PROCESSED+1))p"

# Check file properties
PROBLEM_FILE=$(find /Users/jdfalk/ao-library/library -type f \( -name "*.m4b" -o -name "*.mp3" \) | sort | sed -n "$((PROCESSED+1))p")
echo "Problem file: $PROBLEM_FILE"
file "$PROBLEM_FILE"
stat "$PROBLEM_FILE"

# Try to read it
ffprobe "$PROBLEM_FILE" 2>&1 | head -10
# If ffprobe hangs, the file is corrupted

# Temporary fix: move problematic file
mv "$PROBLEM_FILE" "$PROBLEM_FILE.bak"

# Restart scan
killall audiobook-organizer-embedded 2>/dev/null || true
sleep 3
~/audiobook-organizer-embedded serve --port 8888 --debug &
sleep 3

OPERATION=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" | jq -r '.operation_id')
timeout 120 curl -N "http://localhost:8888/api/events?operation_id=$OPERATION"
```

### Root Cause B: Parallel Worker Deadlock

**Indicators:**
- Process running
- CPU at 0% or very low
- Memory stable
- No new progress events

**Solution:**

```bash
# Restart with different worker count
# Edit configuration if available, or use environment variable

killall -9 audiobook-organizer-embedded 2>/dev/null || true
sleep 3
rm -f /tmp/audiobook-organizer-* 2>/dev/null || true

# Restart with debug
AUDIOBOOK_DEBUG=1 AUDIOBOOK_WORKERS=2 ~/audiobook-organizer-embedded serve --port 8888 --debug &
sleep 3

# Test again
OPERATION=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan" | jq -r '.operation_id')
timeout 120 curl -N "http://localhost:8888/api/events?operation_id=$OPERATION"
```

### Root Cause C: Database Lock

**Indicators:**
- Server still running
- API responding
- But scan frozen
- Other operations blocked

**Solution:**

```bash
# Check for lock files
ls -la /Users/jdfalk/ao-library/*.lock 2>/dev/null

# Force restart
killall -9 audiobook-organizer-embedded 2>/dev/null || true
sleep 3

# Clean up locks
rm -f /Users/jdfalk/ao-library/*.lock 2>/dev/null
rm -f /Users/jdfalk/ao-library/*.lock-journal 2>/dev/null
rm -f /tmp/audiobook-organizer-* 2>/dev/null

# Restart
AUDIOBOOK_DEBUG=1 ~/audiobook-organizer-embedded serve --port 8888 --debug &
sleep 5

# Test
curl http://localhost:8888/api/v1/health | jq .
```

---

## Issue #5: Final Message Wrong

**Symptoms:**
```
"Scan completed. Library: 0 books, Import: 0 books"
```
when you know there are 4 books

**Root Cause A: Books Not Actually Scanned**

**Check:**
```bash
# Look in log for file count
tail -100 /Users/jdfalk/ao-library/logs/operation-*.log | grep "Found"
# Should show: "Folder /path: Found 4 audiobook files"

# If shows 0, then files aren't where expected
find /Users/jdfalk/ao-library/library -type f -name "*.m4b" -o -name "*.mp3" | wc -l
```

**Solution:**
```bash
# Verify files exist
find /Users/jdfalk/ao-library/library -type f \( -name "*.m4b" -o -name "*.mp3" \) | head -5

# Check file permissions
find /Users/jdfalk/ao-library/library -type f \( -name "*.m4b" -o -name "*.mp3" \) -exec ls -la {} \; | head -3

# If permission issues:
chmod -R 644 /Users/jdfalk/ao-library/library/
chmod -R 755 /Users/jdfalk/ao-library/library/*/
```

### Root Cause B: Wrong RootDir Configured

**Check:**
```bash
curl -s http://localhost:8888/api/v1/system/status | jq '.root_directory'
# Should show: "/Users/jdfalk/ao-library/library"
```

**If wrong:**
```bash
# Need to fix app configuration
# Check config file location
cat /Users/jdfalk/ao-library/config.json 2>/dev/null | jq '.root_dir'

# Or check environment variables
env | grep -i "audiobook\|library"
```

### Root Cause C: Books In Different Location

**Check:**
```bash
# Find where books actually are
find /Users/jdfalk/ao-library -type f \( -name "*.m4b" -o -name "*.mp3" \) | head -5

# If in import path instead of library:
# Move them
mv /Users/jdfalk/ao-library/import/*.m4b /Users/jdfalk/ao-library/library/ 2>/dev/null || true
```

---

## Issue #6: No Log Files Created

**Symptoms:**
```
/Users/jdfalk/ao-library/logs/ is empty
No operation-*.log files
```

**Root Cause A: Logs Directory Missing**

```bash
# Check if exists
ls -la /Users/jdfalk/ao-library/logs/ 2>&1

# Create if missing
mkdir -p /Users/jdfalk/ao-library/logs
chmod 755 /Users/jdfalk/ao-library/logs

# Verify
ls -la /Users/jdfalk/ao-library/logs/
```

**Root Cause B: No Write Permissions**

```bash
# Check permissions
ls -la /Users/jdfalk/ao-library/ | grep logs

# Fix if needed
chmod 755 /Users/jdfalk/ao-library/logs
chmod 755 /Users/jdfalk/ao-library

# Verify write capability
touch /Users/jdfalk/ao-library/logs/test.txt && rm /Users/jdfalk/ao-library/logs/test.txt && echo "✅ Writable"
```

**Root Cause C: Progress Logging Disabled**

```bash
# Check if logging is configured
grep -r "logs" /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/ | grep -i "log.*file\|progress" | head -5

# If not found, rebuild or reconfigure
```

---

## Issue #7: Lock Files Stuck

**Symptoms:**
```
Test fails with: "❌ Task 1 already running"
But you know no test is running
```

**Diagnosis:**

```bash
ls -la /tmp/task-1-scan-lock-*.txt
cat /tmp/task-1-scan-lock-$(whoami).txt

# Check if state file still exists
STATE_FILE=$(cat /tmp/task-1-scan-lock-$(whoami).txt)
ls -la "$STATE_FILE"
```

**Solution:**

```bash
# Clean up stale locks
rm -f /tmp/task-1-scan-lock-*.txt
rm -f /tmp/task-1-state-*.json
rm -f /tmp/task-1-*.txt
rm -f /tmp/scenario-*.txt
rm -f /tmp/scenario-*.json

echo "✅ Lock files cleaned"

# Verify no orphaned operations
curl -s http://localhost:8888/api/v1/operations | jq '[.items[] | select(.status == "processing")]'
# Should be empty []

# If still shows processing operations from crashed agent:
curl -s http://localhost:8888/api/v1/operations | jq '.items[] | select(.status == "processing") | .id' | head -1
# Then manually mark them as completed or restart server
```

---

## Recovery Procedures

### If Scan Fails Partway

```bash
# 1. Check what happened
OPERATION_ID=$(cat /tmp/task-1-operation-id.txt)
curl -s "http://localhost:8888/api/v1/operations/$OPERATION_ID" | jq '{status, error}'

# 2. Review logs
tail -50 /Users/jdfalk/ao-library/logs/operation-*.log

# 3. If recoverable, restart
killall audiobook-organizer-embedded 2>/dev/null || true
sleep 3
~/audiobook-organizer-embedded serve --port 8888 --debug &
sleep 3

# 4. Trigger new scan
OPERATION=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" | jq -r '.operation_id')
timeout 120 curl -N "http://localhost:8888/api/events?operation_id=$OPERATION"
```

### If Database Is Corrupted

```bash
# 1. Backup current state
cp -r /Users/jdfalk/ao-library /Users/jdfalk/ao-library.backup.$(date +%s)

# 2. Delete corrupted database
rm -rf /Users/jdfalk/ao-library/audiobooks.db*
rm -rf /Users/jdfalk/ao-library/*.pebble

# 3. Restart server (will recreate)
killall audiobook-organizer-embedded 2>/dev/null || true
sleep 3
~/audiobook-organizer-embedded serve --port 8888 --debug &
sleep 5

# 4. Verify
curl http://localhost:8888/api/v1/health | jq .

# 5. Rescan
OPERATION=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan" | jq -r '.operation_id')
timeout 120 curl -N "http://localhost:8888/api/events?operation_id=$OPERATION"
```

### If All Else Fails

```bash
# Nuclear option: Complete reset
killall -9 audiobook-organizer-embedded 2>/dev/null || true
sleep 5

# Remove all state
rm -rf /Users/jdfalk/ao-library/*.db* 2>/dev/null || true
rm -rf /Users/jdfalk/ao-library/*.pebble 2>/dev/null || true
rm -rf /Users/jdfalk/ao-library/*.lock* 2>/dev/null || true
rm -f /tmp/task-1-*.txt 2>/dev/null || true
rm -f /tmp/scenario-*.txt 2>/dev/null || true
rm -rf /Users/jdfalk/ao-library/logs/*.log 2>/dev/null || true

# Rebuild binary fresh
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go clean
go build -tags embed -o ~/audiobook-organizer-embedded

# Start fresh
AUDIOBOOK_DEBUG=1 ~/audiobook-organizer-embedded serve --port 8888 --debug &
sleep 5

# Verify
curl http://localhost:8888/api/v1/health | jq .

# Try again
OPERATION=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan" | jq -r '.operation_id')
timeout 120 curl -N "http://localhost:8888/api/events?operation_id=$OPERATION"
```

---

## Quick Reference: Common Fixes

| Issue                | Quick Fix                                                                                            |
| -------------------- | ---------------------------------------------------------------------------------------------------- |
| "Connection refused" | `killall audiobook-organizer-embedded && ~/audiobook-organizer-embedded serve --port 8888 --debug &` |
| No SSE events        | Check operation status with `curl http://localhost:8888/api/v1/operations/<ID>`                      |
| Progress 0/X         | Rebuild with `go build -o ~/audiobook-organizer-embedded`                                            |
| Stuck scan           | Restart with `killall -9 && sleep 3 && rm -f /tmp/* && ~/audiobook-organizer-embedded...`            |
| Locks stuck          | `rm -f /tmp/task-1-*.txt`                                                                            |
| DB corrupted         | `rm -rf /Users/jdfalk/ao-library/*.db* && restart server`                                            |

---

**Document Version:** 2.0.1
**Last Updated:** December 6, 2025
**Total Lines:** 600+
**Total Words:** ~8,500
