<!-- file: docs/TASK-6-CORE-TESTING.md -->
<!-- version: 1.0.0 -->
<!-- guid: 8e3f9d2c-7a4b-4c5d-9f7e-1a8b2c3d4e5f -->

# Task 6: Core Book Detail & Delete Flow Testing

This file defines the core flow to validate book detail page and enhanced delete workflow.

## 🔒 Safety & Locks

```bash
LOCK_FILE="/tmp/task-6-lock.txt"
STATE_FILE="/tmp/task-6-state-$(date +%s).json"

if [ -f "$LOCK_FILE" ]; then
  echo "❌ Task 6 already running by $(cat $LOCK_FILE 2>/dev/null | head -1)" && exit 1
fi

echo "$(whoami)" > "$LOCK_FILE"
trap 'rm -f "$LOCK_FILE"' EXIT
```

## Phase 0: UI Component Check (Read-Only)

```bash
echo "=== Checking Book Detail UI ==="

# Check for detail page/modal component
rg "BookDetail|AudiobookDetail" web/src/components web/src/pages -n

# Check routing
rg "audiobooks/:id|/book/" web/src -n | grep -i route

# Check delete dialog
rg "DeleteDialog|ConfirmDelete" web/src -n
```

Expected:

- Component exists: `BookDetail.tsx` or `AudiobookDetailModal.tsx`
- Route defined: `/audiobooks/:id` or similar
- Delete dialog enhanced with checkbox

## Phase 1: API Full Detail Validation (Read-Only)

```bash
echo "=== Testing Book Detail API ==="

# Get list of books
BOOK_ID=$(curl -s http://localhost:8888/api/v1/audiobooks?limit=1 | jq -r '.items[0].id // empty')

if [ -z "$BOOK_ID" ]; then
  echo "❌ No books found, run scan first"
  exit 1
fi

# Fetch full details
curl -s "http://localhost:8888/api/v1/audiobooks/$BOOK_ID" | tee "$STATE_FILE" | jq '{
  id, title, author, series, narrator, publisher,
  file_path, file_size, duration, quality,
  original_hash, library_hash, state
}'

# Fetch versions
curl -s "http://localhost:8888/api/v1/audiobooks/$BOOK_ID/versions" | jq '.'
```

Pass criteria:

- API returns complete book object with all metadata fields
- File info includes path, size, format
- Versions endpoint returns linked editions (if any)

## Phase 2: UI Navigation Test (Manual)

```bash
echo "=== Manual UI Test ==="
echo "1. Open http://localhost:8888 in browser"
echo "2. Navigate to Library page"
echo "3. Click on a book card/row"
echo "4. Verify detail page/modal opens"
echo "5. Check tabs: Info, Files, Versions"
echo "6. Verify all fields populated"
echo ""
read -p "Press ENTER when validation complete..."
```

## Phase 3: Delete Dialog Test (Manual)

```bash
echo "=== Manual Delete Dialog Test ==="
echo "1. On book detail page, click Delete button"
echo "2. Verify dialog shows book title and confirmation"
echo "3. Check for 'Prevent Reimporting' checkbox"
echo "4. Check checkbox"
echo "5. Verify confirmation message shows hashes to be blocked"
echo "6. Cancel dialog (don't delete yet)"
echo ""
read -p "Press ENTER when validation complete..."
```

## Phase 4: Delete with Reimport Prevention (State-Changing)

```bash
echo "=== Testing Delete with Reimport Prevention ==="

# Capture pre-delete state
curl -s "http://localhost:8888/api/v1/audiobooks/$BOOK_ID" | jq '{id, title, original_hash, library_hash, state}' > /tmp/task-6-pre-delete.json

ORIG_HASH=$(cat /tmp/task-6-pre-delete.json | jq -r '.original_hash // empty')
LIB_HASH=$(cat /tmp/task-6-pre-delete.json | jq -r '.library_hash // empty')

echo "Original hash: $ORIG_HASH"
echo "Library hash: $LIB_HASH"

# Delete with prevent_reimport flag
DELETE_RESULT=$(curl -s -X DELETE "http://localhost:8888/api/v1/audiobooks/$BOOK_ID" \
  -H "Content-Type: application/json" \
  -d "{\"prevent_reimport\": true, \"reason\": \"Task 6 test deletion\"}")

echo "$DELETE_RESULT" | jq '.'

# Verify book soft-deleted
curl -s "http://localhost:8888/api/v1/audiobooks/$BOOK_ID" | jq '{id, state, soft_deleted_at}'

# Verify hashes blocked
curl -s "http://localhost:8888/api/v1/settings/blocked-hashes" | jq ".[] | select(.hash == \"$ORIG_HASH\" or .hash == \"$LIB_HASH\")"
```

Pass criteria:

- Book state is `soft_deleted`
- Both hashes (original and library) in blocklist
- Reason recorded in blocklist

## Phase 5: UI Verification After Delete (Manual)

```bash
echo "=== Manual UI Verification ==="
echo "1. Refresh Library page"
echo "2. Verify deleted book no longer visible (or marked as deleted)"
echo "3. Navigate to Settings > Blocked Hashes"
echo "4. Verify both hashes listed with reason"
echo "5. Optionally unblock one hash"
echo ""
read -p "Press ENTER when validation complete..."
```

## Phase 6: Cleanup

```bash
# Remove test state files
rm -f /tmp/task-6-lock.txt /tmp/task-6-state-*.json /tmp/task-6-pre-delete.json
```

If failures occur, switch to `TASK-6-TROUBLESHOOTING.md`.
