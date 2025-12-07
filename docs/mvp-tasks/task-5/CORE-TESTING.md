<!-- file: docs/TASK-5-CORE-TESTING.md -->
<!-- version: 1.0.0 -->
<!-- guid: 4d8c9e2f-7b3a-4c5d-8f6e-1a9b2c3d4e5f -->

# Task 5: Core Hash Tracking & State Testing

This file defines the core flow to validate dual-hash tracking and state lifecycle management.

## 🔒 Safety & Locks

```bash
LOCK_FILE="/tmp/task-5-lock.txt"
STATE_FILE="/tmp/task-5-state-$(date +%s).json"

if [ -f "$LOCK_FILE" ]; then
  echo "❌ Task 5 already running by $(cat $LOCK_FILE 2>/dev/null | head -1)" && exit 1
fi

echo "$(whoami)" > "$LOCK_FILE"
trap 'rm -f "$LOCK_FILE"' EXIT
```

## Phase 0: Schema Verification (Read-Only)

```bash
echo "=== Checking Schema ==="

# Check for new fields in books table
rg "original_hash|library_hash|state|soft_deleted_at|quantity" internal/database/schema.go -n

# Check for do_not_import table
rg "do_not_import|DoNotImport" internal/database -n
```

Expected schema additions:

```go
type Book struct {
    // ... existing fields
    OriginalHash    string     `json:"original_hash"`     // Hash of file at import time
    LibraryHash     string     `json:"library_hash"`      // Hash after organization
    State           string     `json:"state"`             // wanted/imported/organized/soft_deleted
    Quantity        int        `json:"quantity"`          // Reference counter
    SoftDeletedAt   *time.Time `json:"soft_deleted_at"`   // Timestamp for purge job
}

type DoNotImport struct {
    Hash      string    `json:"hash"`
    Reason    string    `json:"reason"`
    CreatedAt time.Time `json:"created_at"`
}
```

## Phase 1: Import with Hash Capture (State-Changing)

```bash
echo "=== Testing Import Hash Capture ==="

# Import a test file
TEST_FILE="/tmp/test-import-book.m4b"
if [ ! -f "$TEST_FILE" ]; then
  echo "Create test file first: cp /path/to/real/book.m4b $TEST_FILE"
  exit 1
fi

# Compute expected hash
EXPECTED_HASH=$(shasum -a 256 "$TEST_FILE" | awk '{print $1}')
echo "Expected hash: $EXPECTED_HASH"

# Import via API
IMPORT_RESULT=$(curl -s -X POST "http://localhost:8888/api/v1/import/file" \
  -H "Content-Type: application/json" \
  -d "{\"file_path\": \"$TEST_FILE\", \"organize\": false}")

BOOK_ID=$(echo "$IMPORT_RESULT" | jq -r '.id // empty')
echo "Imported book: $BOOK_ID"

# Verify original_hash set
curl -s "http://localhost:8888/api/v1/audiobooks/$BOOK_ID" | tee "$STATE_FILE" | jq '{id, state, original_hash, library_hash}'
```

Pass criteria:

- `original_hash` matches file SHA256
- `state` is `imported`
- `library_hash` is null (not yet organized)

## Phase 2: Organize with Library Hash (State-Changing)

```bash
echo "=== Testing Organize Hash Update ==="

# Trigger organize for this book
ORG_RESULT=$(curl -s -X POST "http://localhost:8888/api/v1/operations/organize" \
  -H "Content-Type: application/json" \
  -d "{\"book_ids\": [\"$BOOK_ID\"]}")

OP_ID=$(echo "$ORG_RESULT" | jq -r '.operation_id // empty')
echo "Organize started: $OP_ID"

# Wait for completion
for i in {1..60}; do
  STATUS=$(curl -s "http://localhost:8888/api/v1/operations/$OP_ID" | jq -r '.status // empty')
  [ "$STATUS" = "completed" ] && break
  [ "$STATUS" = "failed" ] && echo "❌ Organize failed" && break
  sleep 1
done

# Check updated hashes
curl -s "http://localhost:8888/api/v1/audiobooks/$BOOK_ID" | jq '{id, state, original_hash, library_hash, file_path}'
```

Pass criteria:

- `library_hash` now populated (hash of organized file)
- `state` is `organized`
- `file_path` updated to library location
- `original_hash` unchanged

## Phase 3: Delete with Reimport Prevention (State-Changing)

```bash
echo "=== Testing Delete with Blocklist ==="

# Delete book with prevent_reimport flag
DELETE_RESULT=$(curl -s -X DELETE "http://localhost:8888/api/v1/audiobooks/$BOOK_ID" \
  -H "Content-Type: application/json" \
  -d "{\"prevent_reimport\": true, \"reason\": \"Test deletion\"}")

echo "$DELETE_RESULT" | jq '.'

# Verify book soft-deleted
curl -s "http://localhost:8888/api/v1/audiobooks/$BOOK_ID" | jq '{id, state, soft_deleted_at}'

# Verify hash added to do_not_import
curl -s "http://localhost:8888/api/v1/settings/blocked-hashes" | jq '.'
```

Pass criteria:

- Book `state` is `soft_deleted`
- `soft_deleted_at` timestamp set
- Both `original_hash` and `library_hash` in `do_not_import` table
- Book still retrievable but marked deleted

## Phase 4: Reimport Prevention Test (Read-Only)

```bash
echo "=== Testing Reimport Prevention ==="

# Try to import same file again
REIMPORT_RESULT=$(curl -s -X POST "http://localhost:8888/api/v1/import/file" \
  -H "Content-Type: application/json" \
  -d "{\"file_path\": \"$TEST_FILE\", \"organize\": false}")

echo "$REIMPORT_RESULT" | jq '.'
```

Pass criteria:

- Import rejected with message like "File hash is blocked from import"
- No new book created

## Phase 5: Unblock Hash (State-Changing)

```bash
echo "=== Testing Hash Unblock ==="

# Remove hash from blocklist
UNBLOCK_RESULT=$(curl -s -X DELETE "http://localhost:8888/api/v1/settings/blocked-hashes/$EXPECTED_HASH")

echo "$UNBLOCK_RESULT" | jq '.'

# Verify hash removed
curl -s "http://localhost:8888/api/v1/settings/blocked-hashes" | jq '.'

# Try import again (should succeed now)
REIMPORT2=$(curl -s -X POST "http://localhost:8888/api/v1/import/file" \
  -H "Content-Type: application/json" \
  -d "{\"file_path\": \"$TEST_FILE\", \"organize\": false}")

echo "$REIMPORT2" | jq '{id, state, original_hash}'
```

## Phase 6: Cleanup

```bash
# Remove test file
rm -f /tmp/test-import-book.m4b

# Remove any imported books
# (Manual cleanup via UI or API as needed)

rm -f /tmp/task-5-lock.txt /tmp/task-5-state-*.json
```

If failures occur, switch to `TASK-5-TROUBLESHOOTING.md`.
