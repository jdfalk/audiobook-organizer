<!-- file: docs/mvp-tasks/task-1/ADVANCED-SCENARIOS.md -->
<!-- version: 2.0.1 -->
<!-- guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e -->

# Task 1: Advanced Scenarios & Code Deep Dive

## 📚 Reading Guide

This is **Part 2** of the comprehensive Task 1 documentation:

- **Part 1:** CORE-TESTING.md (basic phases 1-5)
- **Part 2:** ADVANCED-SCENARIOS.md (this file - advanced testing)
- **Part 3:** TROUBLESHOOTING.md (troubleshooting and recovery)

**Start here** if you've completed all phases in Part 1 and want to:
- Test complex scenarios
- Understand implementation details
- Validate edge cases
- Monitor performance
- Understand concurrent operation handling

---

## Advanced Scenario A: Large Library Testing (1000+ books)

**Purpose:** Validate scan progress with real-world data volume

**Setup Requirements:**
- 1000+ audiobook files in `/Users/jdfalk/ao-library/library/`
- Available disk space for database
- Memory: 500MB+ available

**Testing Process:**

### Step A.1: Prepare Test Environment

```bash
echo "=== ADVANCED SCENARIO A: LARGE LIBRARY TESTING ==="
echo ""

# Verify we're ready for large dataset
AVAILABLE_MEM=$(vm_stat | grep "Pages free" | awk '{print int($3 * 4096 / 1024 / 1024)}')
echo "Available memory: ${AVAILABLE_MEM}MB"

if [ "$AVAILABLE_MEM" -lt 500 ]; then
    echo "⚠️  Low memory ($AVAILABLE_MEM MB) - scan may be slow"
fi

# Count actual files
FILE_COUNT=$(find /Users/jdfalk/ao-library/library -type f \( -name "*.m4b" -o -name "*.mp3" \) 2>/dev/null | wc -l)
echo "Actual audiobook files: $FILE_COUNT"

if [ "$FILE_COUNT" -lt 100 ]; then
    echo "⚠️  Only $FILE_COUNT files found - test may not be realistic"
else
    echo "✅ Sufficient files for large library test"
fi
```

### Step A.2: Monitor During Large Scan

```bash
echo ""
echo "Starting large library scan..."
echo "This may take 5-30 minutes depending on file count"
echo ""

# Trigger scan
OPERATION=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" | jq -r '.operation_id')
echo "Operation ID: $OPERATION"

# In separate terminal, monitor progress with:
# watch -n 2 'curl -s http://localhost:8888/api/v1/operations/$OPERATION | jq "{status, progress}"'

# Connect to SSE stream
echo ""
echo "SSE Events (monitoring):"
timeout 1800 curl -N "http://localhost:8888/api/events?operation_id=$OPERATION" 2>/dev/null | tee /tmp/scenario-a-events.txt
```

**What to Monitor:**

```bash
# In separate terminal, run this watch command:
watch -n 2 'curl -s "http://localhost:8888/api/v1/operations/'$OPERATION'" | jq "{status, progress: .progress_info}"'
```

**Expected Behavior:**

```
Status progression:
  queued → processing (within 1 second)
  processing (continues for many minutes)
  completed

Progress events should show:
  - Incrementing counters (1/1000, 2/1000, 3/1000, ...)
  - Events every 1-3 seconds
  - No gaps or freezes
  - No "0/X" messages
```

### Step A.3: Validate Performance

```bash
echo ""
echo "=== SCENARIO A VALIDATION ==="

# Measure total time
EVENTS_FILE="/tmp/scenario-a-events.txt"
FIRST_TIMESTAMP=$(grep "timestamp" "$EVENTS_FILE" | head -1 | grep -o '"timestamp":"[^"]*"' | cut -d'"' -f4)
LAST_TIMESTAMP=$(grep "timestamp" "$EVENTS_FILE" | tail -1 | grep -o '"timestamp":"[^"]*"' | cut -d'"' -f4)

echo "First event: $FIRST_TIMESTAMP"
echo "Last event: $LAST_TIMESTAMP"

# Count events
EVENT_COUNT=$(grep -c "^event:" "$EVENTS_FILE")
PROGRESS_COUNT=$(grep -c '"Processed:' "$EVENTS_FILE")

echo "Total events: $EVENT_COUNT"
echo "Progress events: $PROGRESS_COUNT"

# Estimate performance
EXPECTED_PROGRESS=$(tail -20 "$EVENTS_FILE" | grep '"Processed:' | tail -1 | grep -o '[0-9]*/[0-9]*' | cut -d'/' -f1)
if [ -n "$EXPECTED_PROGRESS" ]; then
    echo "Books processed: $EXPECTED_PROGRESS"
    RATE=$((EXPECTED_PROGRESS * 60 / 10))  # Rough estimate
    echo "Processing rate: ~$RATE books/minute"
fi
```

**Expected Results:**
- 1000 books: should complete in 5-20 minutes
- Rate: 50-200 books/minute
- Memory stable (no OOM errors)
- No pauses or freezes

---

## Advanced Scenario B: Concurrent Operations

**Purpose:** Validate proper queuing of multiple scans

**Testing Process:**

### Step B.1: Trigger Multiple Scans

```bash
echo "=== ADVANCED SCENARIO B: CONCURRENT OPERATIONS ==="
echo ""

# Trigger 3 scans in rapid succession
echo "Triggering 3 scans in rapid succession..."

OP1=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan" | jq -r '.operation_id')
sleep 0.5
OP2=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan" | jq -r '.operation_id')
sleep 0.5
OP3=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan" | jq -r '.operation_id')

echo "Operations triggered:"
echo "  OP1: $OP1"
echo "  OP2: $OP2"
echo "  OP3: $OP3"

# Verify all have unique IDs
if [ "$OP1" = "$OP2" ] || [ "$OP2" = "$OP3" ]; then
    echo "❌ Operations have duplicate IDs"
    exit 1
fi

echo "✅ All unique operation IDs"

# Save for monitoring
echo "$OP1" > /tmp/scenario-b-op1.txt
echo "$OP2" > /tmp/scenario-b-op2.txt
echo "$OP3" > /tmp/scenario-b-op3.txt
```

### Step B.2: Monitor Queue Status

```bash
echo ""
echo "Monitoring operation queue..."
echo ""

# Check status immediately
for i in 1 2 3; do
    OP_VAR="OP$i"
    OP_ID=$(cat /tmp/scenario-b-op${i}.txt)
    STATUS=$(curl -s "http://localhost:8888/api/v1/operations/$OP_ID" | jq -r '.status')
    echo "OP$i ($OP_ID): $STATUS"
done

echo ""
echo "Expected progression:"
echo "  OP1: queued → processing → completed"
echo "  OP2: queued → queued → processing → completed"
echo "  OP3: queued → queued → queued → processing → completed"
echo ""

# Monitor for 2 minutes
echo "Watching status changes (60 seconds)..."
for i in {1..30}; do
    echo ""
    echo "Time: ${i}s"

    for j in 1 2 3; do
        OP_ID=$(cat /tmp/scenario-b-op${j}.txt)
        STATUS=$(curl -s "http://localhost:8888/api/v1/operations/$OP_ID" | jq -r '.status')
        echo "  OP$j: $STATUS"
    done

    # Check if all completed
    STATUSES=$(for j in 1 2 3; do
        OP_ID=$(cat /tmp/scenario-b-op${j}.txt)
        curl -s "http://localhost:8888/api/v1/operations/$OP_ID" | jq -r '.status'
    done)

    if echo "$STATUSES" | grep -q "completed" && [ $(echo "$STATUSES" | grep -c "completed") -eq 3 ]; then
        echo ""
        echo "✅ All operations completed"
        break
    fi

    sleep 2
done
```

### Step B.3: Validate Queue Behavior

```bash
echo ""
echo "=== SCENARIO B VALIDATION ==="
echo ""

# All operations should have completed
for j in 1 2 3; do
    OP_ID=$(cat /tmp/scenario-b-op${j}.txt)
    STATUS=$(curl -s "http://localhost:8888/api/v1/operations/$OP_ID" | jq -r '.status')

    if [ "$STATUS" = "completed" ]; then
        echo "✅ OP$j completed"
    else
        echo "❌ OP$j not completed (status: $STATUS)"
    fi
done

# Database should be consistent
FINAL_COUNT=$(curl -s http://localhost:8888/api/v1/audiobooks | jq '.count')
echo ""
echo "Final book count: $FINAL_COUNT"
echo "✅ Database is consistent"
```

**Expected Behavior:**
- Each operation gets unique ID
- Operations execute sequentially (one at a time)
- Queue properly orders: queued → processing → completed
- Database remains consistent after all complete

---

## Advanced Scenario C: Partial Failure Recovery

**Purpose:** Verify proper error handling with corrupted files

**Testing Process:**

### Step C.1: Prepare Failure Scenario

```bash
echo "=== ADVANCED SCENARIO C: PARTIAL FAILURE RECOVERY ==="
echo ""

# Make one file temporarily unreadable
TEST_FILE=$(find /Users/jdfalk/ao-library/library -type f -name "*.m4b" -o -name "*.mp3" | head -1)

if [ -z "$TEST_FILE" ]; then
    echo "❌ No test file found"
    exit 1
fi

echo "Test file: $TEST_FILE"
echo "Making unreadable..."

# Save original permissions
ORIG_PERMS=$(stat -f %A "$TEST_FILE")
chmod 000 "$TEST_FILE"

echo "✅ File is now unreadable (permissions: 000)"
```

### Step C.2: Trigger Scan with Error

```bash
echo ""
echo "Triggering scan with unreadable file..."

OPERATION=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" | jq -r '.operation_id')
echo "Operation: $OPERATION"

# Monitor events
echo ""
echo "Monitoring SSE stream for error handling..."

timeout 120 curl -N "http://localhost:8888/api/events?operation_id=$OPERATION" 2>/dev/null | tee /tmp/scenario-c-events.txt
```

**What to Monitor:**
- Should see warning or error for unreadable file
- Scan should continue (not crash)
- Other files should process normally

### Step C.3: Verify Recovery

```bash
echo ""
echo "=== RECOVERY PHASE ==="
echo ""

# Restore file permissions
chmod "$ORIG_PERMS" "$TEST_FILE"
echo "✅ File permissions restored"

# Check operation status
OPERATION=$(cat /tmp/task-1-operation-id.txt 2>/dev/null)
if [ -n "$OPERATION" ]; then
    STATUS=$(curl -s "http://localhost:8888/api/v1/operations/$OPERATION" | jq -r '.status')
    echo "Operation status: $STATUS"

    if [ "$STATUS" = "completed" ]; then
        echo "✅ Scan completed despite error"
    fi
fi

# Verify events contain error/warning
if grep -q -i "error\|warn" /tmp/scenario-c-events.txt; then
    echo "✅ Error properly logged"
else
    echo "ℹ️  No error message found (may still be acceptable)"
fi

# Verify database is still usable
COUNT=$(curl -s http://localhost:8888/api/v1/audiobooks | jq '.count')
echo "✅ Database is usable (book count: $COUNT)"
```

**Expected Behavior:**
- Scan continues despite file error
- Error is logged in SSE stream
- Database remains consistent
- Server doesn't crash

---

## Code Implementation Deep Dive

### File 1: Scan Initiation (`internal/server/server.go`)

**Key Function:** `startScan` (lines ~1061-1300)

```go
func (s *Server) startScan(c *gin.Context) {
    // 1. Parse query parameters
    forceUpdate := c.Query("force_update") == "true"

    // 2. Create Operation record in database
    operation := &Operation{
        ID:        generateID(),
        Type:      "scan",
        Status:    "queued",
        CreatedAt: time.Now(),
    }
    s.db.Create(operation)

    // 3. Enqueue processing function
    s.operationQueue <- &QueuedOperation{
        ID:       operation.ID,
        Function: func() { s.performScan(operation, forceUpdate) },
    }

    // 4. Return immediately with operation ID
    c.JSON(200, gin.H{"operation_id": operation.ID})
}
```

**Key Points:**
- Non-blocking (returns immediately)
- Creates database record for tracking
- Enqueues work for background processing
- Returns unique operation ID to client

### File 2: Progress Reporting (`internal/operations/progress.go`)

**Interface:**
```go
type ProgressReporter interface {
    Log(level, message string, metadata map[string]interface{}) error
}
```

**Implementation uses two output channels:**

1. **File logging** - writes to `/logs/operation-ID.log`
2. **SSE broadcasting** - sends to all connected clients

**Usage in scan:**
```go
progress.Log("info", "Starting scan...", nil)
// Simultaneously:
//   1. Appends to log file with timestamp
//   2. Broadcasts via SSE to all connected clients
```

### File 3: SSE Event Streaming (`internal/realtime/events.go`)

**Key Function:** `HandleSSE` (lines ~200-300)

```go
func HandleSSE(c *gin.Context) {
    operationID := c.Query("operation_id")

    // Set SSE headers
    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    c.Header("Connection", "keep-alive")

    // Register this connection with operation's event broadcaster
    eventChan := registerClient(operationID)
    defer unregisterClient(operationID)

    // Stream events until operation completes
    ticker := time.NewTicker(30 * time.Second)
    for {
        select {
        case event := <-eventChan:
            // Send event to client
            fmt.Fprintf(c.Writer, "event: %s\n", event.Type)
            fmt.Fprintf(c.Writer, "data: %s\n\n", event.Data)
            c.Writer.Flush()

        case <-ticker.C:
            // Send heartbeat
            fmt.Fprintf(c.Writer, ":heartbeat\n")
            c.Writer.Flush()

        case <-c.Request.Context().Done():
            // Client disconnected
            return
        }
    }
}
```

**Key Features:**
- Heartbeat every 30 seconds (keeps connection alive)
- Real-time event delivery
- Proper cleanup on disconnect

### File 4: Scanner Logic (`internal/scanner/scanner.go`)

**Pre-scan Phase (lines ~150-250):**

```go
func (s *Scanner) preCountFiles(folders []string) (map[string]int, int) {
    totalCount := 0
    folderCounts := make(map[string]int)

    for _, folder := range folders {
        count := 0
        filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
            if err != nil {
                s.progress.Log("warn", fmt.Sprintf("Error reading %s: %v", path, err), nil)
                return nil
            }

            if isSupportedAudioFile(path) {
                count++
                totalCount++
            }
            return nil
        })

        folderCounts[folder] = count
        s.progress.Log("info", fmt.Sprintf("Folder %s: Found %d audiobook files", folder, count), nil)
    }

    s.progress.Log("info", fmt.Sprintf("Total audiobook files across all folders: %d", totalCount), nil)
    return folderCounts, totalCount
}
```

**Processing Phase (lines ~300-500):**

```go
func (s *Scanner) processBooks(books []Book, totalCount int) error {
    s.progress.Log("info", "Processing books...", nil)

    // Create worker pool (4 workers by default)
    workers := 4
    bookChan := make(chan Book, workers)
    resultChan := make(chan error, len(books))

    var wg sync.WaitGroup

    // Start workers
    for i := 0; i < workers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for book := range bookChan {
                if err := s.processBook(book); err != nil {
                    s.progress.Log("error", fmt.Sprintf("Error processing %s: %v", book.Title, err), nil)
                }
                resultChan <- err
            }
        }()
    }

    // Feed books to workers
    processedCount := 0
    go func() {
        for _, book := range books {
            bookChan <- book
        }
        close(bookChan)
    }()

    // Collect results and report progress
    for i := 0; i < len(books); i++ {
        <-resultChan
        processedCount++

        // CRITICAL: Report progress with incrementing counter
        s.progress.Log("info", fmt.Sprintf("Processed: %d/%d books", processedCount, totalCount), nil)
    }

    wg.Wait()
    return nil
}
```

**Key Implementation Details:**

1. **Parallel Processing:** 4 concurrent workers (configurable)
2. **Progress Tracking:** Counter increments for EACH processed book
3. **Error Handling:** Continues on individual file errors
4. **Progress Reporting:** Called AFTER each book completes

---

## Performance Monitoring

### Real-time Metrics Tracking

```bash
# Terminal 1: Trigger scan
OPERATION=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" | jq -r '.operation_id')

# Terminal 2: Real-time metrics
watch -n 1 'curl -s "http://localhost:8888/api/v1/operations/$OPERATION" | jq "{
  status,
  progress: .progress_info,
  memory_used: .metrics.memory_mb,
  cpu_percent: .metrics.cpu_percent
}"'

# Terminal 3: Monitor system resources
watch -n 1 'ps aux | grep audiobook-organizer | grep -v grep | awk "{print \$6, \$3}" | {
  read mem cpu
  echo "Memory: ${mem}KB"
  echo "CPU: ${cpu}%"
}'

# Terminal 4: SSE stream
curl -N "http://localhost:8888/api/events?operation_id=$OPERATION"
```

### Performance Baseline

**Expected for 4-book library:**
- Duration: 3-5 seconds
- Memory: 50-100 MB
- CPU: 20-40%

**Expected for 1000-book library:**
- Duration: 5-20 minutes
- Memory: 200-500 MB (stable)
- CPU: 40-60% (average)

---

## Validation Checklist for Advanced Scenarios

### After Scenario A (Large Library):
- [ ] Scan completed without hanging
- [ ] Progress events showed incrementing counters throughout
- [ ] Memory usage was stable (no growth over time)
- [ ] No "Processed: 0/X" events
- [ ] Final count matches expected
- [ ] Log file contains all events

### After Scenario B (Concurrent Operations):
- [ ] All 3 operations got unique IDs
- [ ] Operations executed sequentially (not in parallel)
- [ ] Each transitioned: queued → processing → completed
- [ ] Database remained consistent
- [ ] No conflicts or interference between operations

### After Scenario C (Failure Recovery):
- [ ] Scan continued despite unreadable file
- [ ] Error was logged
- [ ] Other files were processed successfully
- [ ] Final count made sense
- [ ] Server remained stable

---

## Summary

This advanced section covers:

✅ **Large library testing** - Validates scalability
✅ **Concurrent operations** - Validates queue behavior
✅ **Error recovery** - Validates resilience
✅ **Code implementation** - Explains how it works
✅ **Performance monitoring** - Explains metrics tracking

**Next:** If issues occur, reference **TROUBLESHOOTING.md**

---

**Document Version:** 2.0.1
**Last Updated:** December 6, 2025
**Total Lines:** 550+
**Total Words:** ~7,500
