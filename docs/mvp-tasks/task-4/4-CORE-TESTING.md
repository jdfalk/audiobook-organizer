<!-- file: docs/mvp-tasks/task-4/4-CORE-TESTING.md -->
<!-- version: 1.0.1 -->
<!-- guid: 2b5e8c1f-9d4a-4e6b-8f7c-1a9b2c3d4e5f -->

# Task 4: Core Duplicate Detection Testing

This file defines the core flow to validate duplicate detection via SHA256 hashing.

## 🔒 Safety & Locks

```bash
LOCK_FILE="/tmp/task-4-lock.txt"
STATE_FILE="/tmp/task-4-state-$(date +%s).json"

if [ -f "$LOCK_FILE" ]; then
  echo "❌ Task 4 already running by $(cat $LOCK_FILE 2>/dev/null | head -1)" && exit 1
fi

echo "$(whoami)" > "$LOCK_FILE"
trap 'rm -f "$LOCK_FILE"' EXIT
```

## Phase 0: Implementation Check (Read-Only)

```bash
# Check if duplicate detection is implemented
echo "=== Checking Implementation ==="

# Look for duplicate endpoint
rg "duplicates" internal/server -n | grep -i "route\|handler\|endpoint"

# Look for hash computation
rg "SHA256|content_hash|file_hash" internal/scanner internal/database -n

# Check database schema for hash field
rg "content_hash|file_hash" internal/database/schema.go -n
```

Expected:

- API endpoint exists (e.g., `GET /api/v1/audiobooks/duplicates`)
- Scanner computes SHA256 hash during file processing
- Database stores hash in `books` table

## Phase 1: Baseline Scan with Hash Computation (State-Changing)

```bash
echo "=== Triggering Scan to Compute Hashes ==="

SCAN_OUT=$(curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true")
OP_ID=$(echo "$SCAN_OUT" | jq -r '.operation_id // empty')

echo "Scan started: $OP_ID"

# Poll until complete (90s timeout)
for i in {1..90}; do
  STATUS=$(curl -s "http://localhost:8888/api/v1/operations/$OP_ID" | jq -r '.status // empty')
  [ "$STATUS" = "completed" ] && break
  [ "$STATUS" = "failed" ] && echo "❌ Scan failed" && break
  sleep 1
  [ $((i % 10)) -eq 0 ] && echo "... scanning ($i s)"
done
```

## Phase 2: Query Duplicate API (Read-Only)

```bash
echo "=== Querying Duplicate Endpoint ==="

# Capture duplicate groups
curl -s http://localhost:8888/api/v1/audiobooks/duplicates | tee "$STATE_FILE" | jq '.'

# Count duplicate groups
GROUPS=$(cat "$STATE_FILE" | jq 'length // 0')
echo "Found $GROUPS duplicate groups"

# Show summary
cat "$STATE_FILE" | jq 'map({group: .[0].content_hash[0:12], count: length, titles: map(.title)})'
```

Expected:

- Returns array of arrays (each inner array is a duplicate group)
- Each group has 2+ books with identical `content_hash` or `file_hash`
- Books differ in `file_path` or `id` but have same content

## Phase 3: Create Test Duplicates (Optional, State-Changing)

```bash
# Only if no duplicates found, create test case
if [ "$GROUPS" -eq 0 ]; then
  echo "=== Creating Test Duplicate ==="

  # Find a book file
  SAMPLE_FILE=$(curl -s http://localhost:8888/api/v1/audiobooks?limit=1 | jq -r '.items[0].file_path // empty')

  if [ -n "$SAMPLE_FILE" ] && [ -f "$SAMPLE_FILE" ]; then
    # Copy to temp location (DO NOT use library paths)
    TEST_COPY="/tmp/test-duplicate-$(basename "$SAMPLE_FILE")"
    cp "$SAMPLE_FILE" "$TEST_COPY"
    echo "Created test duplicate at: $TEST_COPY"

    # Import test file
    curl -s -X POST "http://localhost:8888/api/v1/import/file" \
      -H "Content-Type: application/json" \
      -d "{\"file_path\": \"$TEST_COPY\", \"organize\": false}" | jq '.'

    # Re-scan to compute hash
    curl -s -X POST "http://localhost:8888/api/v1/operations/scan?force_update=true" | jq -r '.operation_id'
    sleep 5

    # Re-query duplicates
    curl -s http://localhost:8888/api/v1/audiobooks/duplicates | jq '.'
  fi
fi
```

## Phase 4: Verify Groups (Read-Only)

```bash
echo "=== Verifying Duplicate Groups ==="

# For each group, compute actual file hashes to confirm
cat "$STATE_FILE" | jq -r '.[][] | .file_path' | while read file; do
  if [ -f "$file" ]; then
    ACTUAL_HASH=$(shasum -a 256 "$file" | awk '{print $1}')
    echo "$file -> $ACTUAL_HASH"
  else
    echo "$file -> MISSING"
  fi
done | sort -k3
```

Pass criteria:

- Files in same group have identical SHA256 hashes
- No distinct files incorrectly grouped
- No duplicate pairs missing from results

## Phase 5: UI Verification (Manual)

```bash
# Check if UI displays duplicates
rg "duplicate" web/src -n | grep -i "component\|page\|view"

# Dashboard should show duplicate count
# Library view should allow filtering/viewing duplicates
```

## Phase 6: Cleanup

```bash
# Remove test files if created
rm -f /tmp/test-duplicate-*.m4b /tmp/test-duplicate-*.mp3

# Remove lock
rm -f /tmp/task-4-lock.txt /tmp/task-4-state-*.json
```

If failures occur, switch to `4-TROUBLESHOOTING.md`.
