<!-- file: docs/TASK-6-TROUBLESHOOTING.md -->
<!-- version: 1.0.0 -->
<!-- guid: 1a5e9d2c-8f4b-4c5d-9f7e-1a8b2c3d4e5f -->

# Task 6: Troubleshooting - Book Detail Page & Delete Flow

Use this guide when detail page doesn't load, delete fails, or reimport prevention doesn't work.

## Quick Index

| Problem                         | Likely Causes                      | Fix                         | Reference |
| ------------------------------- | ---------------------------------- | --------------------------- | --------- |
| Detail page not found           | Component/route missing            | Create component, add route | Issue 1   |
| Detail API returns incomplete   | Handler not returning all fields   | Update API response         | Issue 2   |
| Delete checkbox missing         | Dialog not enhanced                | Add checkbox to dialog      | Issue 3   |
| Hashes not blocked after delete | Handler not inserting to blocklist | Update delete handler       | Issue 4   |

---

## Issue 1: Detail Page Not Found (404)

**Symptoms:** Clicking book shows 404 or no navigation.

**Steps:**

```bash
# Check routing
rg "audiobooks/:id|/book/" web/src -n | grep -i route

# Check component
find web/src -name "*Detail*" -o -name "*AudiobookDetail*"
```

**Fix:**

- Create component: `web/src/components/BookDetail.tsx` or `pages/BookDetail.tsx`.
- Add route in `App.tsx`:

```tsx
<Route path="/audiobooks/:id" element={<BookDetail />} />
```

- Add navigation from Library: `onClick={() => navigate(`/audiobooks/${book.id}`)}`.

## Issue 2: Detail API Returns Incomplete Data

**Symptoms:** Detail page shows missing fields or errors.

**Steps:**

```bash
# Test API directly
curl -s http://localhost:8888/api/v1/audiobooks/BOOK_ID | jq 'keys'

# Check handler
rg "func.*getAudiobook|func.*GetBookByID" internal/server -n
```

**Fix:**

- Ensure handler returns complete `Book` struct:

```go
func getAudiobook(c *gin.Context) {
    id := c.Param("id")
    book, err := database.GlobalStore.GetBookByID(id)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Book not found"})
        return
    }
    c.JSON(http.StatusOK, book) // Returns all fields
}
```

## Issue 3: Delete Checkbox Not Visible

**Symptoms:** Delete dialog doesn't show reimport prevention checkbox.

**Steps:**

```bash
# Check dialog component
rg "DeleteDialog|ConfirmDelete" web/src -n

# Check for checkbox
rg "prevent.*reimport|Prevent.*Reimport" web/src/components -n
```

**Fix:**

- Update DeleteDialog component:

```tsx
<FormControlLabel
  control={<Checkbox checked={preventReimport} onChange={(e) => setPreventReimport(e.target.checked)} />}
  label="Prevent this file from being imported again"
/>
```

- Pass `preventReimport` flag to delete API call.

## Issue 4: Hashes Not Blocked After Delete

**Symptoms:** Delete succeeds but hashes not in blocklist.

**Steps:**

```bash
# Check delete API
curl -s -X DELETE http://localhost:8888/api/v1/audiobooks/BOOK_ID \
  -H "Content-Type: application/json" \
  -d '{"prevent_reimport": true, "reason": "test"}'

# Check blocklist
curl -s http://localhost:8888/api/v1/settings/blocked-hashes | jq '.'
```

**Fix:**

- Update delete handler to insert hashes:

```go
func deleteAudiobook(c *gin.Context) {
    // ... get book, check prevent_reimport flag
    if req.PreventReimport {
        if book.OriginalHash != "" {
            database.GlobalStore.AddBlockedHash(book.OriginalHash, req.Reason)
        }
        if book.LibraryHash != "" && book.LibraryHash != book.OriginalHash {
            database.GlobalStore.AddBlockedHash(book.LibraryHash, req.Reason)
        }
    }
    // ... update book state, soft delete
}
```

## Issue 5: Files Tab Not Showing Files

**Symptoms:** Files tab empty or shows only primary file.

**Steps:**

```bash
# Check if files API exists
curl -s http://localhost:8888/api/v1/audiobooks/BOOK_ID/files | jq '.'

# Check component
rg "files.*tab|Files.*Tab" web/src/components -n
```

**Fix:**

- If multi-file books exist, implement files API.
- For single-file books, display primary file info from main book object.
- List related files: cover art, metadata, additional audio files.

## Cleanup

```bash
rm -f /tmp/task-6-lock.txt /tmp/task-6-state-*.json /tmp/task-6-pre-delete.json
```

If unresolved, capture browser console errors and server logs for detail page load and delete operations.
