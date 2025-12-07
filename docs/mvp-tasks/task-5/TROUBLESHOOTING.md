<!-- file: docs/TASK-5-TROUBLESHOOTING.md -->
<!-- version: 1.0.0 -->
<!-- guid: 6f1d8c2e-9a4b-4c5d-8f7e-1a9b2c3d4e5f -->

# Task 5: Troubleshooting - Hash Tracking & State Lifecycle

Use this guide when hash tracking fails, states are incorrect, or reimport prevention doesn't work.

## Quick Index

| Problem                   | Likely Causes                    | Fix                             | Reference |
| ------------------------- | -------------------------------- | ------------------------------- | --------- |
| Hashes not captured       | Migration missing, scanner skips | Run migration, update scanner   | Issue 1   |
| Reimports not blocked     | do_not_import check missing      | Add check in import handler     | Issue 2   |
| State not updating        | Organize handler not patched     | Update state on organize/delete | Issue 3   |
| Blocked hashes UI missing | Settings tab not implemented     | Create UI component             | Issue 4   |

---

## Issue 1: Hashes Not Captured During Import

**Symptoms:** `original_hash` is null after import.

**Steps:**

```bash
# Check if migration ran
rg "original_hash|library_hash" internal/database/migrations -n

# Check scanner logic
rg "original_hash|OriginalHash" internal/scanner -n

# Query DB directly
curl -s http://localhost:8888/api/v1/audiobooks?limit=5 | jq '.items[] | {title, original_hash, library_hash}'
```

**Fix:**

- Create migration to add columns if missing.
- Update scanner to compute SHA256 and set `OriginalHash` field during import.
- Re-import test file to verify hash captured.

## Issue 2: Reimports Not Blocked

**Symptoms:** Deleted file reimported even though hash in blocklist.

**Steps:**

```bash
# Check if hash in do_not_import
curl -s http://localhost:8888/api/v1/settings/blocked-hashes | jq '.'

# Check import handler
rg "do_not_import|DoNotImport" internal/server -n
```

**Fix:**

- Add check in import handler:

```go
func importFile(c *gin.Context) {
    // ... compute hash
    blocked, err := database.GlobalStore.IsHashBlocked(hash)
    if err != nil || blocked {
        c.JSON(http.StatusConflict, gin.H{"error": "File hash is blocked from import"})
        return
    }
    // ... continue import
}
```

- Implement `IsHashBlocked()` in store.

## Issue 3: State Not Updating

**Symptoms:** Book shows `imported` after organize, or `organized` after delete.

**Steps:**

```bash
# Check book state
curl -s http://localhost:8888/api/v1/audiobooks/BOOK_ID | jq '{state, soft_deleted_at}'

# Check organize handler
rg "state.*organized|State.*Organized" internal/server internal/operations -n
```

**Fix:**

- Update organize handler to set `State = "organized"` after successful operation.
- Update delete handler to set `State = "soft_deleted"` and `SoftDeletedAt = time.Now()`.
- Re-run operation to verify state change.

## Issue 4: Blocked Hashes UI Missing

**Symptoms:** No way to view/unblock hashes in UI.

**Steps:**

```bash
# Check for Settings tab
rg "blocked.*hash|BlockedHash|do.*not.*import" web/src -n
```

**Fix:**

- Create Settings page tab: "Blocked Hashes".
- Fetch list: `GET /api/v1/settings/blocked-hashes`.
- Display table with hash (first 12 chars), reason, timestamp, unblock button.
- Unblock: `DELETE /api/v1/settings/blocked-hashes/:hash`.

## Issue 5: Library Hash Doesn't Update After Organize

**Symptoms:** `library_hash` remains null after organize completes.

**Steps:**

```bash
# Check file after organize
NEW_PATH=$(curl -s http://localhost:8888/api/v1/audiobooks/BOOK_ID | jq -r '.file_path')
shasum -a 256 "$NEW_PATH"

# Compare to library_hash field
curl -s http://localhost:8888/api/v1/audiobooks/BOOK_ID | jq '{file_path, library_hash}'
```

**Fix:**

- Update organize operation to compute hash of newly organized file.
- Set `LibraryHash` after file move completes.
- Ensure hash computation happens even if file unchanged (copy vs move).

## Cleanup

```bash
rm -f /tmp/task-5-lock.txt /tmp/task-5-state-*.json /tmp/test-import-*.m4b
```

If unresolved, capture server logs showing import/organize/delete flows and escalate to code review.
