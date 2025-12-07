<!-- file: docs/TASK-4-TROUBLESHOOTING.md -->
<!-- version: 1.0.0 -->
<!-- guid: 1e8b9c2d-4f5a-4b6c-8e7d-1a9b2c3d4e5f -->

# Task 4: Troubleshooting - Duplicate Detection

Use this guide when duplicate detection is missing, incorrect, or surfacing false results.

## Quick Index

| Problem                             | Likely Causes                        | Fix                        | Reference |
| ----------------------------------- | ------------------------------------ | -------------------------- | --------- |
| No duplicates API endpoint          | Not implemented yet                  | Implement handler          | Issue 1   |
| Hashes not computed                 | Scanner not updated, DB missing col  | Add hash logic, migrate DB | Issue 2   |
| False positives (distinct grouped)  | Metadata-based grouping, not content | Use SHA256, not metadata   | Issue 3   |
| False negatives (duplicates missed) | Hash not computed, stale data        | Re-scan with force         | Issue 4   |

---

## Issue 1: No Duplicates Endpoint

**Symptoms:** `404 Not Found` on `/api/v1/audiobooks/duplicates`.

**Steps:**

```bash
curl -s http://localhost:8888/api/v1/audiobooks/duplicates
# Expected: JSON array of duplicate groups
# Actual: 404 or missing route
```

**Fix:**

- Implement handler in `internal/server/audiobook_handlers.go`:

```go
func getDuplicates(c *gin.Context) {
    groups, err := database.GlobalStore.GetDuplicateBooks()
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, groups)
}
```

- Add route: `router.GET("/api/v1/audiobooks/duplicates", getDuplicates)`
- Implement `GetDuplicateBooks()` in store to query grouped by `content_hash`.

## Issue 2: Hashes Not Computed

**Symptoms:** Duplicate endpoint returns empty array; DB rows have null/empty `content_hash`.

**Steps:**

```bash
# Check if hash field exists
rg "content_hash|file_hash" internal/database/schema.go

# Query DB directly (if using SQLite)
sqlite3 audiobooks.db "SELECT id, title, content_hash FROM books LIMIT 5;"
```

**Fix:**

- Add migration to add `content_hash TEXT` column if missing.
- Update scanner to compute SHA256:

```go
import "crypto/sha256"

hash := sha256.New()
file, _ := os.Open(filepath)
io.Copy(hash, file)
book.ContentHash = fmt.Sprintf("%x", hash.Sum(nil))
```

- Re-run scan with `force_update=true` to populate hashes.

## Issue 3: False Positives (Distinct Files Grouped)

**Symptoms:** Books with different content shown as duplicates.

**Steps:**

```bash
# Verify hashes manually
curl -s http://localhost:8888/api/v1/audiobooks/duplicates | jq '.[][] | {title, file_path, content_hash}'

# For each file in a "duplicate" group:
shasum -a 256 /path/to/file1.m4b
shasum -a 256 /path/to/file2.m4b
# Should match if truly duplicates
```

**Fix:**

- Ensure grouping uses `content_hash` (file SHA256), not metadata fields like title/author.
- If using fuzzy matching, disable it for duplicate detection; only exact hash matches.

## Issue 4: False Negatives (Duplicates Missed)

**Symptoms:** Known duplicate files not appearing in duplicate groups.

**Steps:**

```bash
# Manually hash known duplicates
shasum -a 256 /library/book1.m4b
shasum -a 256 /import/book1-copy.m4b

# Check if both are in DB with hashes
curl -s http://localhost:8888/api/v1/audiobooks?search=book1 | jq '.items[] | {title, file_path, content_hash}'
```

**Fix:**

- Ensure both files were scanned (check import paths configured).
- Re-run scan if files added after initial scan.
- Verify no errors during hash computation (large files timing out, permissions).

## Cleanup

```bash
rm -f /tmp/task-4-lock.txt /tmp/task-4-state-*.json /tmp/test-duplicate-*
```

If unresolved, capture server logs showing scan/hash operations and escalate to code review.
