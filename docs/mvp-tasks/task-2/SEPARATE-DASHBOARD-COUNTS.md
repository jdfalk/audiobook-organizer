<!-- file: docs/mvp-tasks/task-2/SEPARATE-DASHBOARD-COUNTS.md -->
<!-- version: 2.0.1 -->
<!-- guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e -->

# Task 2: Separate Dashboard Counts (Library vs Import)

## 🎯 Overall Goal

Implement and verify separate tracking of book counts for:

- **Library Books**: Books in the configured root directory (`RootDir`)
- **Import Books**: Books detected in import paths but not yet organized
- **Display**: Show both counts separately on Dashboard and Library pages

**Success Criteria:**

- ✅ Dashboard shows two separate counts: "Library: X" and "Import: Y"
- ✅ Library page respects the separation
- ✅ `/api/v1/system/status` returns distinct `library_book_count` and `import_book_count`
- ✅ Counts are accurate and persistent after scans
- ✅ No data loss or duplication

---

## 📦 Split Documentation (read-first)

This legacy file is kept for history. Active, organized documentation is split:

- `TASK-2-README.md` — overview and navigation
- `TASK-2-CORE-TESTING.md` — core phases, safety/locks
- `TASK-2-ADVANCED-SCENARIOS.md` — edge cases, performance, code deep dive
- `TASK-2-TROUBLESHOOTING.md` — issues, root causes, fixes

Use the split files for current work.

---

## 📋 Process

### Phase 1: Idempotent State Assessment

**Goal:** Determine current implementation state WITHOUT modifying anything.

**Idempotency Check:**

```bash
# SAFE - READ ONLY - Can run multiple times
STATE_FILE="/tmp/dashboard-counts-state-$(date +%s).json"

# Check current API state
echo "=== CHECKING CURRENT STATE ==="
curl -s http://localhost:8888/api/v1/system/status > "$STATE_FILE"
echo "Saved to: $STATE_FILE"

# Display results
echo "API Response:"
cat "$STATE_FILE" | jq '.'
```

**Verify before proceeding:**

```bash
# Check what fields exist in current response
cat "$STATE_FILE" | jq 'keys'
```

Expected fields:

- ✅ `library_book_count` - Already implemented in v1.26.0
- ✅ `import_book_count` - Already implemented in v1.26.0
- ✅ `total_book_count` - Should equal sum of above

If these fields exist, go to Phase 2. If not, implementation needed first.

### Phase 2: Verify Backend Implementation

**Goal:** Confirm server code properly separates library vs import counts.

**Idempotent Verification:**

```bash
# Read-only check - safe to run anytime
echo "=== CHECKING BACKEND CODE ==="

# Verify code exists at expected location
if grep -q "libraryBookCount" /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/server/server.go; then
    echo "✅ Backend separation code found"
    grep -A 3 "libraryBookCount" internal/server/server.go | head -10
else
    echo "❌ Backend code not found - needs implementation"
    exit 1
fi
```

Expected code pattern:

```go
libraryBookCount := 0
importBookCount := 0

for _, book := range allBooks {
    if rootDir != "" && strings.HasPrefix(book.FilePath, rootDir) {
        libraryBookCount++
    } else {
        importBookCount++
    }
}
```

### Phase 3: Verify Frontend Display

**Goal:** Confirm Dashboard and Library pages show separate counts.

**Idempotent UI Verification:**

```bash
# Read-only check - no state changes
echo "=== CHECKING FRONTEND CODE ==="

# Check Dashboard component
if grep -q "library_book_count\|libraryBookCount" /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/web/src/pages/Dashboard.tsx; then
    echo "✅ Dashboard shows library count"
else
    echo "⚠️  Dashboard may not display library count separately"
fi

# Check Library component
if grep -q "import_book_count\|importBookCount" /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/web/src/pages/Library.tsx; then
    echo "✅ Library page shows import count"
else
    echo "⚠️  Library page may not display import count separately"
fi
```

### Phase 4: Test Count Accuracy

**Goal:** Verify counts are calculated correctly without changing anything.

**Idempotent Test:**

```bash
echo "=== TESTING COUNT ACCURACY ==="

# Get current counts from API
API_RESPONSE=$(curl -s http://localhost:8888/api/v1/system/status)
LIBRARY_COUNT=$(echo "$API_RESPONSE" | jq '.library_book_count')
IMPORT_COUNT=$(echo "$API_RESPONSE" | jq '.import_book_count')
TOTAL_FROM_API=$(echo "$API_RESPONSE" | jq '.total_book_count')

# Manually count books to verify
echo "Verifying library count ($LIBRARY_COUNT)..."
LIBRARY_ACTUAL=$(curl -s "http://localhost:8888/api/v1/audiobooks?limit=1000" | jq '.items[] | select(.file_path | startswith("'"${HOME}"'/ao-library/library/")) | .id' | wc -l)

echo "Verifying import count ($IMPORT_COUNT)..."
IMPORT_ACTUAL=$(curl -s "http://localhost:8888/api/v1/audiobooks?limit=1000" | jq '.items[] | select(.file_path | startswith("'"${HOME}"'/ao-library/library/") | not) | .id' | wc -l)

echo ""
echo "Count Verification Results:"
echo "  Library - Expected: $LIBRARY_COUNT, Actual: $LIBRARY_ACTUAL, Match: $([ "$LIBRARY_COUNT" -eq "$LIBRARY_ACTUAL" ] && echo '✅' || echo '❌')"
echo "  Import  - Expected: $IMPORT_COUNT, Actual: $IMPORT_ACTUAL, Match: $([ "$IMPORT_COUNT" -eq "$IMPORT_ACTUAL" ] && echo '✅' || echo '❌')"
echo "  Total   - API reports: $TOTAL_FROM_API, Expected: $((LIBRARY_ACTUAL + IMPORT_ACTUAL)), Match: $([ "$TOTAL_FROM_API" -eq "$((LIBRARY_ACTUAL + IMPORT_ACTUAL))" ] && echo '✅' || echo '❌')"
```

### Phase 5: Test After Scan

**Goal:** Verify counts remain accurate after a scan operation (idempotent test).

**Idempotent Scan Test:**

```bash
echo "=== PRE-SCAN STATE ==="

# Capture state BEFORE scan
BEFORE_STATE=$(curl -s http://localhost:8888/api/v1/system/status)
BEFORE_LIBRARY=$(echo "$BEFORE_STATE" | jq '.library_book_count')
BEFORE_IMPORT=$(echo "$BEFORE_STATE" | jq '.import_book_count')

echo "Before scan - Library: $BEFORE_LIBRARY, Import: $BEFORE_IMPORT"

echo ""
echo "=== RUNNING SCAN ==="

# Trigger scan (this modifies state)
SCAN_RESULT=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true")
OPERATION_ID=$(echo "$SCAN_RESULT" | jq -r '.operation_id // empty')

if [ -z "$OPERATION_ID" ]; then
    echo "❌ Failed to start scan"
    exit 1
fi

echo "Scan started: $OPERATION_ID"

# Wait for scan to complete (with timeout)
echo "Waiting for scan to complete..."
for i in {1..60}; do
    STATUS=$(curl -s "http://localhost:8888/api/v1/operations/$OPERATION_ID" | jq -r '.status')
    if [ "$STATUS" = "completed" ] || [ "$STATUS" = "failed" ]; then
        echo "Scan $STATUS after $i seconds"
        break
    fi
    [ $((i % 10)) -eq 0 ] && echo "  Still running... ($i seconds elapsed)"
    sleep 1
done

echo ""
echo "=== POST-SCAN STATE ==="

# Capture state AFTER scan
AFTER_STATE=$(curl -s http://localhost:8888/api/v1/system/status)
AFTER_LIBRARY=$(echo "$AFTER_STATE" | jq '.library_book_count')
AFTER_IMPORT=$(echo "$AFTER_STATE" | jq '.import_book_count')

echo "After scan - Library: $AFTER_LIBRARY, Import: $AFTER_IMPORT"

echo ""
echo "=== VERIFICATION ==="

# Verify counts make sense
echo "Library count stable: $([ "$BEFORE_LIBRARY" -eq "$AFTER_LIBRARY" ] && echo '✅' || echo '⚠️ (may have changed)')"
echo "Import count stable: $([ "$BEFORE_IMPORT" -eq "$AFTER_IMPORT" ] && echo '✅' || echo '⚠️ (may have changed)')"
echo "Total is sum: $([ "$((AFTER_LIBRARY + AFTER_IMPORT))" -eq "$(echo "$AFTER_STATE" | jq '.total_book_count')" ] && echo '✅' || echo '❌')"
```

---

## 🔧 Technical Context

### Current Implementation Status

**Already Implemented** (in v1.26.0+):

Backend (`internal/server/server.go:getSystemStatus`):

```go
// Separate counts: books in RootDir (library) vs import paths
libraryBookCount := 0
importBookCount := 0
rootDir := config.AppConfig.RootDir

for _, book := range allBooks {
    if rootDir != "" && strings.HasPrefix(book.FilePath, rootDir) {
        libraryBookCount++
    } else {
        importBookCount++
    }
}

// Return response with separate counts
c.JSON(http.StatusOK, gin.H{
    "library_book_count": libraryBookCount,
    "import_book_count": importBookCount,
    "total_book_count": libraryBookCount + importBookCount,
    // ... other fields
})
```

Frontend (`web/src/pages/Dashboard.tsx`, `web/src/pages/Library.tsx`):

- Should display both counts
- May need UI updates to show them prominently

### API Response Format

Expected `/api/v1/system/status` response:

```json
{
  "library_book_count": 4,
  "import_book_count": 0,
  "total_book_count": 4,
  "library_size_bytes": 1453506723,
  "import_size_bytes": 0,
  "total_size_bytes": 1453506723,
  "concurrent_workers": 4,
  "server_version": "1.26.0"
}
```

### Root Directory Configuration

The separation depends on proper `RootDir` configuration:

```bash
# Check configured RootDir
curl -s http://localhost:8888/api/v1/system/status | jq '.root_directory'

# Expected: "/Users/jdfalk/ao-library/library"
```

If `RootDir` is empty or misconfigured:

- All books classified as "import"
- Library count always 0
- This is expected behavior - need to fix config, not code

---

## ✅ Verification Checklist

### Backend Verification

- [ ] `/api/v1/system/status` returns `library_book_count` field
- [ ] `/api/v1/system/status` returns `import_book_count` field
- [ ] Sum of library + import equals `total_book_count`
- [ ] Library count matches books in configured `RootDir`
- [ ] Import count matches books outside `RootDir`
- [ ] Counts persist correctly after restart
- [ ] Counts update correctly after scan

### Frontend Verification

- [ ] Dashboard displays library count
- [ ] Dashboard displays import count
- [ ] Library page shows both counts
- [ ] Counts update in real-time after scan
- [ ] No console errors in browser DevTools
- [ ] Mobile view displays counts properly

### Integration Verification

- [ ] Counts accurate with 0 library books
- [ ] Counts accurate with 0 import books
- [ ] Counts accurate with mixed books
- [ ] No race conditions during concurrent operations
- [ ] Counts correct after removing books
- [ ] Counts correct after adding import paths

---

## 🚨 Idempotent Rollback / Recovery

**If anything breaks, these commands safely restore state:**

```bash
# 1. Check current counts (non-destructive)
curl -s http://localhost:8888/api/v1/system/status | jq '{library_book_count, import_book_count}'

# 2. Restart server (idempotent - no data loss)
killall audiobook-organizer-embedded 2>/dev/null || true
sleep 2
~/audiobook-organizer-embedded serve --port 8888 --debug

# 3. Verify counts restored
curl -s http://localhost:8888/api/v1/system/status | jq '{library_book_count, import_book_count}'

# 4. If counts incorrect, rebuild and restart
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
git status  # Should be clean if following idempotent steps
go build -o ~/audiobook-organizer-embedded
~/audiobook-organizer-embedded serve --port 8888 --debug
```

**Key Principle:** All state-changing operations (scans, modifications) should be explicitly triggered. If following the idempotent verification steps above, nothing is modified.

---

## 📊 Test Scenarios

### Scenario A: Pure Library Books Only

**Setup:**

- Root directory: `/Users/jdfalk/ao-library/library/`
- Import paths: (none or empty)
- Database: 4 books

**Expected Result:**

- `library_book_count`: 4
- `import_book_count`: 0
- `total_book_count`: 4

**Verification:**

```bash
curl -s http://localhost:8888/api/v1/system/status | jq '{library_book_count, import_book_count, total_book_count}'
```

### Scenario B: Import Books Only

**Setup:**

- Root directory: (empty or not set)
- Import paths: `/Users/jdfalk/import-path/`
- Database: 4 books (all in import path)

**Expected Result:**

- `library_book_count`: 0
- `import_book_count`: 4
- `total_book_count`: 4

### Scenario C: Mixed Library + Import

**Setup:**

- Root directory: `/Users/jdfalk/ao-library/library/`
- Import paths: `/Users/jdfalk/import-path/`
- Database: 6 books (4 in library, 2 in import)

**Expected Result:**

- `library_book_count`: 4
- `import_book_count`: 2
- `total_book_count`: 6

---

## 📝 Code Locations

**Backend Implementation:**

- File: `internal/server/server.go`
- Function: `getSystemStatus()`
- Lines: ~1648-1720 (approximate)

**Frontend Display:**

- File: `web/src/pages/Dashboard.tsx`
- File: `web/src/pages/Library.tsx`

**Database Schema:**

- File: `internal/database/schema.go`
- Note: No schema changes needed - counts are computed, not stored

---

## 🔗 Dependencies & Related Tasks

**Depends on:**

- ✅ Task 1: Scan Progress Reporting (must be working for testing)
- ✅ Database with books initialized
- ✅ RootDir configured correctly

**Affects:**

- UI displaying book statistics
- Dashboard counts
- Library organization workflow

**Does NOT affect:**

- Actual book data or files
- Scan operations (they work independently)
- API functionality

---

## 📞 Success Criteria Summary

Test passes when:

1. ✅ `/api/v1/system/status` returns distinct `library_book_count` and `import_book_count`
2. ✅ Library count equals actual books in `RootDir`
3. ✅ Import count equals books outside `RootDir`
4. ✅ Total equals library + import
5. ✅ Dashboard displays both counts
6. ✅ Counts persist after server restart
7. ✅ Counts update correctly after scan
8. ✅ No data loss or corruption
