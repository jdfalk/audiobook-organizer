<!-- file: docs/DEMO_QUICKSTART.md -->
<!-- version: 1.0.0 -->
<!-- guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f9a -->
<!-- last-edited: 2026-02-05 -->

# End-to-End Demo Quick Start Guide

## 🎯 Objective

Demonstrate the complete audiobook organizer workflow:
1. **Import** audio files
2. **Fetch** metadata from Open Library
3. **Organize** files into folder structure
4. **Edit** metadata manually with overrides
5. **Verify** changes persist

---

## ⚡ Quick Start (5 minutes)

### Step 1: Start the API Server

```bash
# Terminal 1: Build and start the API server
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer

# Build (first time only or after code changes)
make build

# Start the server (runs on http://localhost:8080)
./audiobook-organizer serve
```

Expected output:
```
2026-02-05 12:34:56 [INFO] Starting audiobook-organizer API server
2026-02-05 12:34:56 [INFO] Listening on http://localhost:8080
```

### Step 2: Run the End-to-End Test Script

```bash
# Terminal 2: Run the automated test script
bash scripts/e2e_test.sh

# Or run with verbose output
bash scripts/e2e_test.sh --verbose
```

This will:
- ✅ Check server health
- ✅ Test import workflow
- ✅ Test metadata fetching
- ✅ Test file organization
- ✅ Test metadata editing
- ✅ Verify all data persists

### Step 3: View the Results

The script will show:
- Color-coded test results (green = pass, red = fail)
- Response times for each API call
- Data verification checks
- Summary of successful workflows

---

## 📋 Full Manual Demo (10-15 minutes)

If you want to walk through step-by-step manually:

### Terminal 1: Start Server
```bash
make build && ./audiobook-organizer serve
```

### Terminal 2: Run API Calls

```bash
# Source the example API functions
source scripts/api_examples.sh

# Set API endpoint (default: http://localhost:8080)
API_ENDPOINT="http://localhost:8080"

# Test 1: Health Check
echo "=== Testing Server Health ==="
curl -s $API_ENDPOINT/api/health | jq '.'

# Test 2: List Import Paths
echo "=== Existing Import Paths ==="
curl -s $API_ENDPOINT/api/v1/import-paths | jq '.items'

# Test 3: Add Import Path
echo "=== Adding Import Path ==="
IMPORT_PATH=$(curl -s -X POST \
  -H "Content-Type: application/json" \
  -d '{"path": "/tmp/audiobooks", "name": "Demo Library"}' \
  $API_ENDPOINT/api/v1/import-paths | jq -r '.id')
echo "Import Path ID: $IMPORT_PATH"

# Test 4: Browse Filesystem
echo "=== Browsing /tmp/audiobooks ==="
curl -s "$API_ENDPOINT/api/v1/filesystem/browse?path=/tmp/audiobooks" | jq '.items'

# Test 5: Import a File
echo "=== Importing Audio File ==="
BOOK_ID=$(curl -s -X POST \
  -H "Content-Type: application/json" \
  -d '{"file_path": "/tmp/audiobooks/test_book.m4b", "organize": false}' \
  $API_ENDPOINT/api/v1/import/file | jq -r '.id')
echo "Created Book ID: $BOOK_ID"

# Test 6: Verify Book Was Created
echo "=== Listing All Books ==="
curl -s "$API_ENDPOINT/api/v1/audiobooks?limit=10" | jq '.items'

# Test 7: Fetch Metadata
echo "=== Fetching Metadata ==="
curl -s -X POST \
  -H "Content-Type: application/json" \
  -d '{"missing_only": false}' \
  $API_ENDPOINT/api/v1/metadata/bulk-fetch | jq '.results'

# Test 8: Verify Metadata Was Fetched
echo "=== Book with Metadata ==="
curl -s "$API_ENDPOINT/api/v1/audiobooks/$BOOK_ID" | jq '.title, .author, .description'

# Test 9: Organize Files
echo "=== Organizing Files ==="
curl -s -X POST \
  $API_ENDPOINT/api/v1/audiobooks/$BOOK_ID/organize | jq '.status'

# Test 10: Update Book Metadata
echo "=== Updating Book Metadata ==="
curl -s -X PUT \
  -H "Content-Type: application/json" \
  -d '{"title": "Custom Title", "narrator": "Professional Narrator"}' \
  $API_ENDPOINT/api/v1/audiobooks/$BOOK_ID | jq '.title, .narrator'

# Test 11: Verify Update Persisted
echo "=== Verification ==="
curl -s $API_ENDPOINT/api/v1/audiobooks/$BOOK_ID | jq '.title, .narrator'
```

---

## 🧪 Run Automated Test Suite

Use the comprehensive test script for full validation:

```bash
# Run all tests with color output
bash scripts/e2e_test.sh

# Run specific test suite
bash scripts/e2e_test.sh --test-health
bash scripts/e2e_test.sh --test-import
bash scripts/e2e_test.sh --test-metadata
bash scripts/e2e_test.sh --test-organize
bash scripts/e2e_test.sh --test-edit
bash scripts/e2e_test.sh --test-complete

# Show help
bash scripts/e2e_test.sh --help
```

---

## 📖 Full Documentation References

For more detailed information, see:

- **END_TO_END_DEMO.md** (1000+ lines)
  - Complete workflow documentation
  - 20+ curl examples with expected responses
  - Pagination parameter documentation
  - Troubleshooting guide

- **IMPLEMENTATION_SUMMARY.md** (469 lines)
  - Technical implementation details
  - All API endpoints documented
  - Test results and metrics

- **scripts/api_examples.sh** (350+ lines)
  - Practical API examples
  - Copy-paste ready curl commands
  - Interactive help menu

---

## 🎬 Recording a Demo Video

To record a demonstration video:

### Setup

```bash
# 1. Start the API server
make build && ./audiobook-organizer serve

# 2. In another terminal, prepare test files
mkdir -p /tmp/demo-audiobooks
cp sample-audiobooks/*.m4b /tmp/demo-audiobooks/ # or use test files

# 3. Start screen recording
# macOS: Use QuickTime Player or ScreenFlow
# Linux: Use SimpleScreenRecorder or OBS
# Windows: Use OBS or built-in Screen Recording
```

### Demo Script

**Part 1: Import Files (2-3 minutes)**
- Show file browser with /tmp/demo-audiobooks
- Add import path
- Import 3-4 sample audiobooks
- Show books appearing in the list with "imported" status

**Part 2: Fetch Metadata (2-3 minutes)**
- Show empty metadata fields
- Click "Bulk Fetch Metadata"
- Show metadata populating (title, author, description, cover)
- Show metadata from Open Library

**Part 3: Organize Files (1-2 minutes)**
- Show original files in /tmp/demo-audiobooks
- Click "Organize" on a book
- Show files moving to structured folder: Author/Series/Book
- Verify file organization on disk

**Part 4: Edit Metadata (2-3 minutes)**
- Show edit dialog for a book
- Edit narrator field
- Edit publisher field
- Show changes being saved
- Verify changes persist on next view

**Part 5: Summary (1 minute)**
- Show all books with complete metadata
- Show organized file structure
- Highlight key features:
  - Automatic metadata fetching
  - Manual override capability
  - Persistent storage
  - File organization

---

## ✅ Success Criteria

After running the demo, verify:

- ✅ Server starts without errors
- ✅ All health checks pass
- ✅ Files can be imported
- ✅ Metadata is fetched automatically
- ✅ Files are organized to disk
- ✅ Metadata can be edited manually
- ✅ All changes persist across API calls
- ✅ UI shows all updated information

---

## 🐛 Troubleshooting

### Server Won't Start
```bash
# Check if port 8080 is in use
lsof -i :8080

# Kill the process if needed
kill -9 <PID>

# Try a different port
./audiobook-organizer serve --port 9000
```

### Tests Failing
```bash
# Run verbose test output
bash scripts/e2e_test.sh --verbose

# Check server logs
# Look at API server terminal for error messages

# Try manual curl command from api_examples.sh
source scripts/api_examples.sh
curl -s http://localhost:8080/api/health | jq '.'
```

### Metadata Not Fetching
```bash
# Verify Open Library is accessible
curl -s "https://openlibrary.org/search.json?q=the+hobbit" | jq '.numFound'

# Check server logs for API errors
# Verify audio files have valid format

# Try manual fetch with specific book
curl -s -X POST \
  -H "Content-Type: application/json" \
  -d '{"missing_only": false}' \
  http://localhost:8080/api/v1/metadata/bulk-fetch | jq '.'
```

### Files Not Organizing
```bash
# Check file permissions
ls -la /tmp/demo-audiobooks/

# Verify target organize path exists
# Check server logs for organization errors

# Verify book has required metadata (author, title)
curl -s http://localhost:8080/api/v1/audiobooks | jq '.items[0]'
```

---

## 📊 Expected Demo Timeline

| Phase | Task | Duration | Files/Data |
|-------|------|----------|------------|
| 1 | Import Files | 2-3 min | 3-4 audiobooks |
| 2 | Fetch Metadata | 2-3 min | All imported books |
| 3 | Organize Files | 1-2 min | 1-2 books (show result) |
| 4 | Edit Metadata | 2-3 min | 2-3 book edits |
| 5 | Verification | 1 min | Summary view |
| **Total** | **Full Workflow** | **8-12 min** | **Complete flow** |

---

## 🚀 Next Steps

After running the demo:

1. **Review Results**
   - Check test output for any failures
   - Review organized file structure
   - Verify metadata accuracy

2. **Optimize Performance**
   - Run performance tests
   - Measure API response times
   - Optimize slow queries if needed

3. **Add Sample Data**
   - Import larger audiobook collection
   - Test with different file formats
   - Verify scalability

4. **Video Production**
   - Edit demo video if recorded
   - Add voiceover explaining workflow
   - Highlight key features and benefits

---

## 📞 Support

For issues or questions:
- Check the full END_TO_END_DEMO.md documentation
- Review IMPLEMENTATION_SUMMARY.md for technical details
- Run tests with --verbose flag for detailed output
- Check server logs in the API server terminal

---

**Demo Status: ✅ Ready to Record**

All systems are functional and tested. The demo can be recorded immediately.
