<!-- file: docs/END_TO_END_DEMO.md -->
<!-- version: 1.0.0 -->
<!-- guid: 5c6d7e8f-9a0b-1c2d-3e4f-5a6b7c8d9e0f -->
<!-- last-edited: 2026-02-04 -->

# End-to-End Workflow Demo

This guide provides step-by-step instructions for testing the complete audiobook organizer workflow, from importing files to organizing and managing metadata.

## Prerequisites

Before running the demo, ensure:

1. The application is built: `make build-api` or `make build`
2. The server is running on `localhost:8080` or you know the correct API endpoint
3. `curl` is installed for making API requests
4. Optional: A test directory with sample audiobook files

## Quick Start

Start the API server:
```bash
./audiobook-organizer serve
# Server runs on http://localhost:8080
```

In another terminal, run the demo commands below.

---

## Part 1: Import Files

### Step 1.1: Add Import Path

Configure a directory to scan for audiobooks.

**Request:**
```bash
curl -X POST http://localhost:8080/api/v1/import/paths \
  -H "Content-Type: application/json" \
  -d '{
    "path": "/home/user/audiobooks",
    "name": "Main Library"
  }'
```

**Response (201 Created):**
```json
{
  "id": "path-001",
  "path": "/home/user/audiobooks",
  "name": "Main Library",
  "created_at": "2026-02-04T12:00:00Z"
}
```

### Step 1.2: Browse Filesystem

Browse available files in the import path.

**Request:**
```bash
curl -X GET "http://localhost:8080/api/v1/filesystem/browse?path=/home/user/audiobooks" \
  -H "Accept: application/json"
```

**Response (200 OK):**
```json
{
  "path": "/home/user/audiobooks",
  "items": [
    {
      "name": "Book1.m4b",
      "path": "/home/user/audiobooks/Book1.m4b",
      "type": "file",
      "size": 1234567890,
      "is_audiobook": true
    },
    {
      "name": "Subfolder",
      "path": "/home/user/audiobooks/Subfolder",
      "type": "directory",
      "size": 0,
      "is_audiobook": false
    }
  ]
}
```

### Step 1.3: Import Single File

Import a specific audiobook file.

**Request:**
```bash
curl -X POST http://localhost:8080/api/v1/import/file \
  -H "Content-Type: application/json" \
  -d '{
    "file_path": "/home/user/audiobooks/Book1.m4b",
    "title": "Sample Book",
    "author": "Sample Author"
  }'
```

**Response (201 Created):**
```json
{
  "id": "book-001",
  "title": "Sample Book",
  "author": "Sample Author",
  "file_path": "/home/user/audiobooks/Book1.m4b",
  "format": "m4b",
  "duration": 36000,
  "is_audiobook": true
}
```

**Common Issues:**
- `404 Not Found`: File doesn't exist or path is inaccessible
- `400 Bad Request`: Invalid file format or missing required fields
- `409 Conflict`: File already imported

---

## Part 2: Fetch Metadata

### Step 2.1: List Imported Books

List all books in the library with pagination.

**Request:**
```bash
curl -X GET "http://localhost:8080/api/v1/audiobooks?limit=50&offset=0" \
  -H "Accept: application/json"
```

**Response (200 OK):**
```json
{
  "items": [
    {
      "id": "book-001",
      "title": "Sample Book",
      "author": "Sample Author",
      "series": null,
      "series_sequence": null,
      "file_path": "/home/user/audiobooks/Book1.m4b",
      "format": "m4b",
      "duration": 36000,
      "release_year": null,
      "genre": null,
      "narrators": null,
      "publisher": null,
      "language": null,
      "cover_art_path": null,
      "description": null,
      "rating": null,
      "tags": [],
      "is_marked_for_deletion": false,
      "is_audiobook": true
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0,
  "total": 1
}
```

**Pagination Parameters:**
- `limit`: Items per page (default: 50, max: 1000)
- `offset`: Pagination offset (default: 0)
- `search`: Optional search term (searches title, author, etc.)

Example with pagination:
```bash
# Get second page (25 items per page)
curl -X GET "http://localhost:8080/api/v1/audiobooks?limit=25&offset=25"

# Search for specific book
curl -X GET "http://localhost:8080/api/v1/audiobooks?search=sample"
```

### Step 2.2: Fetch Metadata for All Books

Bulk fetch metadata from external sources (Goodreads, iTunes, etc).

**Request:**
```bash
curl -X POST http://localhost:8080/api/v1/metadata/bulk-fetch \
  -H "Content-Type: application/json" \
  -d '{
    "book_ids": ["book-001"],
    "source": "goodreads",
    "force_refresh": false
  }'
```

**Response (200 OK):**
```json
{
  "total": 1,
  "succeeded": 1,
  "failed": 0,
  "results": [
    {
      "id": "book-001",
      "status": "success",
      "warnings": []
    }
  ]
}
```

**Expected Metadata:**
After successful fetch, book should have:
- `release_year`: Publication year
- `genre`: Primary genre
- `narrators`: Narrator names
- `publisher`: Publishing company
- `language`: Language code
- `description`: Book synopsis
- `rating`: Average rating (0-5 or 0-10)

### Step 2.3: Get Updated Book Details

Verify metadata was fetched.

**Request:**
```bash
curl -X GET "http://localhost:8080/api/v1/audiobooks/book-001" \
  -H "Accept: application/json"
```

**Response (200 OK):**
```json
{
  "data": {
    "id": "book-001",
    "title": "Sample Book",
    "author": "Sample Author",
    "series": "Sample Series",
    "series_sequence": 1,
    "file_path": "/home/user/audiobooks/Book1.m4b",
    "format": "m4b",
    "duration": 36000,
    "release_year": 2020,
    "genre": "Fantasy",
    "narrators": "Professional Narrator",
    "publisher": "Major Publisher",
    "language": "en",
    "cover_art_path": "/path/to/cover.jpg",
    "description": "An epic fantasy novel...",
    "rating": 4.5,
    "tags": ["fantasy", "epic"],
    "is_marked_for_deletion": false,
    "is_audiobook": true
  }
}
```

---

## Part 3: Organize Files

### Step 3.1: Organize Single Book

Reorganize a book's files according to configured rules.

**Request:**
```bash
curl -X POST http://localhost:8080/api/v1/audiobooks/book-001/organize \
  -H "Content-Type: application/json" \
  -d '{
    "dry_run": false,
    "copy_instead_of_move": false
  }'
```

**Response (200 OK):**
```json
{
  "id": "book-001",
  "status": "organized",
  "original_path": "/home/user/audiobooks/Book1.m4b",
  "organized_path": "/home/user/organized/Fantasy/Sample Author/Sample Series 1 - Sample Book.m4b",
  "files_moved": 1,
  "files_skipped": 0,
  "warnings": []
}
```

**Dry Run:**
To preview changes without making them, use `"dry_run": true`:

```bash
curl -X POST http://localhost:8080/api/v1/audiobooks/book-001/organize \
  -H "Content-Type: application/json" \
  -d '{
    "dry_run": true
  }'
```

**Common Issues:**
- `400 Bad Request`: Invalid book ID
- `404 Not Found`: Book doesn't exist
- `409 Conflict`: File already exists at target location
- `403 Forbidden`: Permission denied writing to organize directory

### Step 3.2: Organize All Books

Organize all books in the library (batch operation).

**Request:**
```bash
curl -X POST http://localhost:8080/api/v1/audiobooks/organize-all \
  -H "Content-Type: application/json" \
  -d '{
    "dry_run": false,
    "copy_instead_of_move": false,
    "skip_if_exists": true
  }'
```

**Response (200 OK):**
```json
{
  "total": 10,
  "succeeded": 9,
  "failed": 1,
  "results": [
    {
      "id": "book-001",
      "status": "success",
      "warnings": []
    },
    {
      "id": "book-002",
      "status": "success",
      "warnings": [
        "File already exists at target location"
      ]
    },
    {
      "id": "book-003",
      "status": "failed",
      "error": "Permission denied"
    }
  ]
}
```

---

## Part 4: Edit Metadata

### Step 4.1: Update Book Metadata

Update book information (title, author, series, etc).

**Request:**
```bash
curl -X PUT http://localhost:8080/api/v1/audiobooks/book-001 \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Updated Title",
    "author": "Updated Author",
    "series": "Updated Series",
    "series_sequence": 2,
    "release_year": 2021,
    "genre": "Science Fiction",
    "narrators": "Updated Narrator",
    "publisher": "New Publisher",
    "language": "en",
    "rating": 4.8,
    "description": "Updated description",
    "tags": ["scifi", "updated"]
  }'
```

**Response (200 OK):**
```json
{
  "data": {
    "id": "book-001",
    "title": "Updated Title",
    "author": "Updated Author",
    "series": "Updated Series",
    "series_sequence": 2,
    "release_year": 2021,
    "genre": "Science Fiction",
    "narrators": "Updated Narrator",
    "publisher": "New Publisher",
    "language": "en",
    "rating": 4.8,
    "description": "Updated description",
    "tags": ["scifi", "updated"]
  }
}
```

### Step 4.2: Set Metadata Overrides

Override specific metadata fields (useful when automatic fetch is incorrect).

**Request:**
```bash
curl -X POST http://localhost:8080/api/v1/audiobooks/book-001/metadata-override \
  -H "Content-Type: application/json" \
  -d '{
    "author": "Correct Author Name",
    "series": "Correct Series Name",
    "locked": true
  }'
```

**Response (200 OK):**
```json
{
  "id": "book-001",
  "overrides": {
    "author": {
      "value": "Correct Author Name",
      "locked": true
    },
    "series": {
      "value": "Correct Series Name",
      "locked": true
    }
  },
  "message": "Metadata overrides applied"
}
```

### Step 4.3: Manage Tags

Add or remove tags from a book.

**Request - Add Tags:**
```bash
curl -X POST http://localhost:8080/api/v1/audiobooks/book-001/tags \
  -H "Content-Type: application/json" \
  -d '{
    "tags": ["favorite", "completed"],
    "action": "add"
  }'
```

**Response (200 OK):**
```json
{
  "id": "book-001",
  "tags": ["scifi", "updated", "favorite", "completed"],
  "message": "Tags updated"
}
```

**Request - Remove Tags:**
```bash
curl -X POST http://localhost:8080/api/v1/audiobooks/book-001/tags \
  -H "Content-Type: application/json" \
  -d '{
    "tags": ["old-tag"],
    "action": "remove"
  }'
```

---

## Part 5: Complete Workflow Demo

Execute a complete workflow from import to organization:

### Setup
```bash
# Set variables for easier testing
API_BASE="http://localhost:8080/api/v1"
AUDIOBOOK_FILE="/home/user/audiobooks/test_book.m4b"
```

### Execute Full Workflow
```bash
# 1. Add import path
IMPORT_PATH=$(curl -s -X POST $API_BASE/import/paths \
  -H "Content-Type: application/json" \
  -d '{"path":"/home/user/audiobooks"}' | jq -r '.id')

echo "Import path created: $IMPORT_PATH"

# 2. Import file
BOOK=$(curl -s -X POST $API_BASE/import/file \
  -H "Content-Type: application/json" \
  -d "{\"file_path\":\"$AUDIOBOOK_FILE\",\"title\":\"Test Book\"}" | jq -r '.id')

echo "Book imported: $BOOK"

# 3. List books
curl -s -X GET "$API_BASE/audiobooks?limit=10" | jq '.items[] | {id, title, author}'

# 4. Fetch metadata
curl -s -X POST $API_BASE/metadata/bulk-fetch \
  -H "Content-Type: application/json" \
  -d "{\"book_ids\":[\"$BOOK\"],\"source\":\"goodreads\"}" | jq '.results'

# 5. Get updated book
curl -s -X GET $API_BASE/audiobooks/$BOOK | jq '.data | {title, author, genre, rating}'

# 6. Update metadata
curl -s -X PUT $API_BASE/audiobooks/$BOOK \
  -H "Content-Type: application/json" \
  -d '{"genre":"Fantasy","tags":["favorite"]}' | jq '.data | {title, genre, tags}'

# 7. Organize
curl -s -X POST $API_BASE/audiobooks/$BOOK/organize \
  -H "Content-Type: application/json" \
  -d '{"dry_run":false}' | jq '.{status, organized_path}'

echo "Workflow complete!"
```

---

## API Reference

### Common Response Codes

| Code | Meaning | Description |
|------|---------|-------------|
| 200 | OK | Request succeeded |
| 201 | Created | Resource created successfully |
| 204 | No Content | Request succeeded, no content to return |
| 400 | Bad Request | Invalid parameters or malformed request |
| 404 | Not Found | Resource doesn't exist |
| 409 | Conflict | Resource already exists or operation conflict |
| 500 | Server Error | Internal server error |

### Pagination

All list endpoints support pagination:

```bash
curl -X GET "http://localhost:8080/api/v1/audiobooks?limit=25&offset=0&search=query"
```

**Parameters:**
- `limit`: Number of items per page (default: 50, max: 1000)
- `offset`: Pagination offset (default: 0, must be >= 0)
- `search`: Optional search term (searches title, author, series)

**Response:**
```json
{
  "items": [...],
  "count": 25,
  "limit": 25,
  "offset": 0,
  "total": 100
}
```

### Error Responses

All errors follow this format:

```json
{
  "error": "Description of the error",
  "code": "ERROR_CODE",
  "status": 400
}
```

---

## Troubleshooting

### Common Issues

**Issue: Permission Denied**
```
"error": "permission denied"
```
**Solution:** Ensure the application has read/write permissions to import and organize directories.

**Issue: File Not Found**
```
"error": "file not found: /path/to/file"
```
**Solution:** Verify the file path exists and is accessible.

**Issue: Metadata Fetch Fails**
```
"error": "metadata fetch failed"
```
**Solution:**
- Verify API keys are configured (if required by metadata source)
- Check network connectivity
- Try with a different metadata source

**Issue: Duplicate File**
```
"error": "file already exists at target location"
```
**Solution:**
- Use `dry_run: true` to preview without changes
- Use `skip_if_exists: true` to skip already organized files
- Manually remove conflicting file before organizing

### Debug Mode

Enable debug logging:

```bash
# Set environment variable before running
export LOG_LEVEL=DEBUG
./audiobook-organizer serve
```

### Reset Library

To start fresh:

```bash
# Remove database file (WARNING: This deletes all data)
rm /path/to/audiobook-organizer/db.sqlite

# Restart application
./audiobook-organizer serve
```

---

## Integration Testing

### Automated Test Script

Create `test_e2e.sh`:

```bash
#!/bin/bash
set -e

API="http://localhost:8080/api/v1"

echo "Testing Audiobook Organizer E2E..."

# Test 1: Health check
echo -n "1. Health check... "
curl -s -f http://localhost:8080/api/health > /dev/null && echo "✓" || echo "✗"

# Test 2: List audiobooks
echo -n "2. List audiobooks... "
curl -s -f "$API/audiobooks" > /dev/null && echo "✓" || echo "✗"

# Test 3: Pagination validation
echo -n "3. Pagination validation... "
RESP=$(curl -s "$API/audiobooks?limit=1000")
LIMIT=$(echo $RESP | jq '.limit')
if [ "$LIMIT" -le 1000 ]; then echo "✓"; else echo "✗"; fi

# Test 4: Invalid pagination
echo -n "4. Invalid pagination handling... "
RESP=$(curl -s "$API/audiobooks?limit=5000&offset=-10")
LIMIT=$(echo $RESP | jq '.limit')
OFFSET=$(echo $RESP | jq '.offset')
if [ "$LIMIT" -le 1000 ] && [ "$OFFSET" -ge 0 ]; then echo "✓"; else echo "✗"; fi

echo "All tests completed!"
```

Run tests:
```bash
chmod +x test_e2e.sh
./test_e2e.sh
```

---

## Browser Testing

### Using Postman

1. Import OpenAPI spec: `docs/openapi.yaml`
2. Set base URL: `http://localhost:8080`
3. Create requests for each endpoint
4. Test with various parameters

### Using curl

See examples throughout this document.

---

## Performance Testing

### Load Testing

Test with multiple concurrent requests:

```bash
# Install: brew install ab (macOS) or apt-get install apache2-utils (Linux)

# Test with 100 concurrent requests
ab -c 100 -n 1000 http://localhost:8080/api/v1/audiobooks

# Expected: < 100ms response time for most requests
```

### Pagination Performance

```bash
# Test large pagination
time curl -s "http://localhost:8080/api/v1/audiobooks?limit=1000&offset=10000" > /dev/null

# Should complete in < 1 second
```

---

## Next Steps

After testing the workflow:

1. **Configure Import Paths**: Set up directories to automatically scan
2. **Setup Metadata Sources**: Configure API keys for Goodreads, iTunes, etc
3. **Schedule Auto-Organization**: Set up cron job to organize new files
4. **Monitor Library**: Use dashboard to track library statistics
5. **Fine-tune Settings**: Adjust organization patterns and metadata rules

For more information, see:
- `README.md`: General documentation
- `docs/OPTIMIZATION_SUMMARY.md`: Architecture and optimization details
- `CLAUDE.md`: Development and architecture guidelines
- `.github/copilot-instructions.md`: Detailed architectural documentation
