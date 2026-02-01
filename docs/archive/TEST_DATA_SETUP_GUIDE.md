<!-- file: docs/TEST_DATA_SETUP_GUIDE.md -->
<!-- version: 1.0.0 -->
<!-- guid: c3d4e5f6-a7b8-9012-cdef-123456789012 -->
<!-- last-edited: 2026-01-19 -->

# Test Data Setup Guide

## Overview

This guide provides instructions for creating and managing test audiobook files
for manual and automated testing of the audiobook-organizer application.

**Purpose**: Ensure consistent, reproducible test data across different testing
environments

**Target Audience**: QA Engineers, Developers, CI/CD Engineers

---

## Quick Start

### Minimal Test Setup (5 minutes)

```bash
# Create test directory
mkdir -p ~/test-audiobooks/import
cd ~/test-audiobooks

# Download sample audiobook file (if available)
# OR create minimal test file (see below)

# Generate test hash for blocked hash testing
shasum -a 256 import/sample-book.m4b > hashes.txt

# Start application pointing to test directory
cd /path/to/audiobook-organizer
./audiobook-organizer serve --port 8888 --dir ~/test-audiobooks/import
```

---

## Test File Requirements

### File Formats

The application supports multiple audiobook formats. For comprehensive testing,
prepare files in:

| Format | Extension | Priority | Notes                         |
| ------ | --------- | -------- | ----------------------------- |
| M4B    | .m4b      | P0       | Primary format, best metadata |
| MP3    | .mp3      | P1       | Common format, basic metadata |
| M4A    | .m4a      | P2       | Apple format                  |
| FLAC   | .flac     | P2       | Lossless audio                |
| OGG    | .ogg      | P3       | Open format                   |

**Minimum for P0 Testing**: 3 M4B files with varying metadata

---

## Creating Test Audiobook Files

### Option 1: Use Real Audiobooks (Recommended)

**Best for**: Authentic metadata testing, realistic file sizes

```bash
# Copy existing audiobooks to test directory
cp /path/to/your/audiobook/collection/*.m4b ~/test-audiobooks/import/

# Rename for test organization
cd ~/test-audiobooks/import
mv "Book 1.m4b" "test-book-001.m4b"
mv "Book 2.m4b" "test-book-002.m4b"
mv "Book 3.m4b" "test-book-003.m4b"
```

**Metadata Requirements**:

- At least one book with rich metadata (title, author, narrator, series,
  publisher)
- At least one book with minimal metadata (title only)
- At least one book with special characters in title

---

### Option 2: Generate Synthetic Test Files

**Best for**: Controlled test scenarios, CI/CD automation

#### Prerequisites

```bash
# Install ffmpeg (if not already installed)
# macOS
brew install ffmpeg

# Linux (Ubuntu/Debian)
sudo apt-get install ffmpeg

# Verify installation
ffmpeg -version
```

#### Create Minimal Test Audiobook

```bash
# Generate 10-second silent audio file with metadata
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Test Book 001: Basic Metadata" \
  -metadata author="Test Author" \
  -metadata album="Test Book 001: Basic Metadata" \
  -metadata artist="Test Narrator" \
  -metadata genre="Fiction" \
  -metadata date="2024" \
  -c:a aac -b:a 64k \
  ~/test-audiobooks/import/test-book-001.m4b

# Generate book with rich metadata
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Test Book 002: Rich Metadata" \
  -metadata author="John R. Doe" \
  -metadata album="Test Series - Book 2" \
  -metadata artist="Jane A. Smith" \
  -metadata genre="Science Fiction" \
  -metadata date="2023" \
  -metadata publisher="Test Publishing" \
  -metadata description="A test audiobook with comprehensive metadata for QA validation" \
  -metadata comment="Volume 2 of Test Series" \
  -c:a aac -b:a 128k \
  ~/test-audiobooks/import/test-book-002.m4b

# Generate book with minimal metadata
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Test Book 003: Minimal" \
  -c:a aac -b:a 64k \
  ~/test-audiobooks/import/test-book-003.m4b
```

#### Create Test Books with Special Characters

```bash
# Book with unicode characters in title
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Test Book 004: 日本語タイトル (Japanese Title)" \
  -metadata author="Test Author™" \
  -c:a aac -b:a 64k \
  ~/test-audiobooks/import/test-book-004-unicode.m4b

# Book with special characters
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Test Book 005: Title with & Symbols @ #1" \
  -metadata author="O'Brien-Smith, J." \
  -c:a aac -b:a 64k \
  ~/test-audiobooks/import/test-book-005-special.m4b
```

---

### Option 3: Download Public Domain Audiobooks

**Best for**: Real content without copyright issues

**Sources**:

- [LibriVox](https://librivox.org/) - Public domain audiobooks
- [Archive.org Audio Books](https://archive.org/details/audio_bookspoetry) -
  Public domain collection

```bash
# Example: Download from LibriVox (requires curl/wget)
cd ~/test-audiobooks/import

# Download sample (replace with actual URL)
curl -L -o "librivox-sample.m4b" "https://librivox.org/path/to/audiobook.m4b"

# Verify file
file librivox-sample.m4b
ffprobe librivox-sample.m4b 2>&1 | head -20
```

---

## Test Scenarios and Required Data

### 1. Metadata Provenance Testing (PR #79)

**Required Files**: 3 books with metadata in different sources

#### Book A: File Metadata Primary

```bash
# Create book with rich file metadata
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Provenance Test A: File Source" \
  -metadata author="File Author" \
  -metadata artist="File Narrator" \
  -metadata date="2024" \
  -c:a aac -b:a 64k \
  ~/test-audiobooks/import/provenance-test-a.m4b
```

#### Book B: Stored Metadata (Manual Database Entry)

```bash
# Create book with minimal file metadata
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Provenance Test B" \
  -c:a aac -b:a 64k \
  ~/test-audiobooks/import/provenance-test-b.m4b

# After import, manually update metadata via UI to create "stored" values
# Or use API:
# curl -X PATCH http://localhost:8888/api/v1/audiobooks/<id> \
#   -H "Content-Type: application/json" \
#   -d '{"title": "Provenance Test B: Stored", "author": "Stored Author"}'
```

#### Book C: Override Testing

```bash
# Create book with metadata that will be overridden
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Original Title" \
  -metadata author="Original Author" \
  -c:a aac -b:a 64k \
  ~/test-audiobooks/import/provenance-test-c.m4b

# During testing, apply overrides via UI to test override source
```

---

### 2. Blocked Hashes Testing (PR #69)

**Required**: Pre-computed SHA256 hashes

```bash
# Generate test files and compute hashes
cd ~/test-audiobooks/import

# Create test file for blocking
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Blocked Hash Test" \
  -c:a aac -b:a 64k \
  blocked-hash-test.m4b

# Compute SHA256 hash
shasum -a 256 blocked-hash-test.m4b | tee blocked-hash.txt

# Example output: a1b2c3d4e5f6... blocked-hash-test.m4b

# Save hash value for manual test use
```

**Test Hash Repository** (for validation testing):

```bash
# Create a file with known test hashes
cat > ~/test-audiobooks/test-hashes.txt << 'EOF'
# Valid SHA256 hashes for testing
a1b2c3d4e5f6789012345678901234567890123456789012345678901234abcd
1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef
fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321

# Invalid hashes (for validation testing)
abc123  # Too short
xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx  # Non-hex
EOF
```

---

### 3. State Transition Testing (PR #70)

**Required**: Multiple books in different states

#### Import State Books

```bash
# Create books for import testing
for i in {1..3}; do
  ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
    -metadata title="State Test Import $i" \
    -metadata author="Test Author" \
    -c:a aac -b:a 64k \
    ~/test-audiobooks/import/state-import-$i.m4b
done
```

#### Organized State Books

```bash
# Import books, then organize them to create "organized" state
# This requires running the application and using organize operation
# See manual test plan for organize workflow
```

#### Soft-Deleted State Books

```bash
# Create books specifically for delete testing
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Delete Test Book 1" \
  -c:a aac -b:a 64k \
  ~/test-audiobooks/import/delete-test-1.m4b

ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Delete Test Book 2" \
  -c:a aac -b:a 64k \
  ~/test-audiobooks/import/delete-test-2.m4b
```

---

## Test Data Organization

### Directory Structure

```
~/test-audiobooks/
├── import/                    # Files for import scanning
│   ├── test-book-001.m4b
│   ├── test-book-002.m4b
│   ├── provenance-test-a.m4b
│   ├── blocked-hash-test.m4b
│   └── state-import-*.m4b
├── organized/                 # Books moved after organize operation
│   └── [organized books here]
├── backup/                    # Backup copies of test files
│   └── [original copies]
├── test-hashes.txt           # Reference hashes for testing
└── metadata-reference.json   # Expected metadata values
```

### Create Directory Structure

```bash
mkdir -p ~/test-audiobooks/{import,organized,backup}
```

---

## Metadata Reference Document

Create a reference file to track expected metadata for each test file:

```bash
cat > ~/test-audiobooks/metadata-reference.json << 'EOF'
{
  "test-book-001.m4b": {
    "file_metadata": {
      "title": "Test Book 001: Basic Metadata",
      "author": "Test Author",
      "narrator": "Test Narrator",
      "genre": "Fiction",
      "year": 2024
    },
    "expected_hash": "compute with shasum -a 256"
  },
  "test-book-002.m4b": {
    "file_metadata": {
      "title": "Test Book 002: Rich Metadata",
      "author": "John R. Doe",
      "narrator": "Jane A. Smith",
      "series": "Test Series",
      "volume": 2,
      "publisher": "Test Publishing",
      "genre": "Science Fiction",
      "year": 2023
    },
    "expected_hash": "compute with shasum -a 256"
  },
  "test-book-003.m4b": {
    "file_metadata": {
      "title": "Test Book 003: Minimal"
    },
    "expected_hash": "compute with shasum -a 256"
  }
}
EOF
```

---

## Compute All Hashes

```bash
# Generate hash reference file
cd ~/test-audiobooks/import
for file in *.m4b; do
  echo "Computing hash for $file..."
  shasum -a 256 "$file" | tee -a ../all-hashes.txt
done

echo "All hashes saved to ~/test-audiobooks/all-hashes.txt"
```

---

## Verify Test Files

### Validation Script

```bash
#!/bin/bash
# File: ~/test-audiobooks/verify-test-files.sh

echo "Verifying test audiobook files..."
cd ~/test-audiobooks/import

for file in *.m4b; do
  echo "Checking $file..."

  # Check file exists and is readable
  if [ ! -r "$file" ]; then
    echo "  ❌ FAIL: File not readable"
    continue
  fi

  # Check file size (should be > 0)
  size=$(stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null)
  if [ "$size" -eq 0 ]; then
    echo "  ❌ FAIL: File is empty"
    continue
  fi

  # Check file format with ffprobe
  if ! ffprobe "$file" 2>&1 | grep -q "Audio"; then
    echo "  ❌ FAIL: Not a valid audio file"
    continue
  fi

  # Extract and display metadata
  title=$(ffprobe "$file" 2>&1 | grep "title" | head -1 | cut -d: -f2- | xargs)
  author=$(ffprobe "$file" 2>&1 | grep "author\|artist" | head -1 | cut -d: -f2- | xargs)

  echo "  ✅ PASS: $file"
  echo "     Title: $title"
  echo "     Author: $author"
  echo "     Size: $size bytes"
done

echo "Verification complete."
```

### Run Verification

```bash
chmod +x ~/test-audiobooks/verify-test-files.sh
~/test-audiobooks/verify-test-files.sh
```

---

## Database Seeding (Advanced)

For performance testing with large datasets, use database seeding:

### Generate Large Test Dataset

```bash
# Create 100 test books (minimal files for speed)
cd ~/test-audiobooks/import

for i in {1..100}; do
  ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 1 \
    -metadata title="Perf Test Book $(printf '%03d' $i)" \
    -metadata author="Author $(( ($i % 10) + 1 ))" \
    -c:a aac -b:a 32k \
    perf-test-$(printf '%03d' $i).m4b \
    -loglevel quiet -y
done

echo "Generated 100 test books for performance testing"
```

### Import All Books

```bash
# Start application
./audiobook-organizer serve --port 8888 --dir ~/test-audiobooks/import

# Trigger scan (from another terminal)
curl -X POST http://localhost:8888/api/v1/operations/scan

# Monitor progress
curl http://localhost:8888/api/v1/system/status | jq '.book_count'
```

---

## CI/CD Integration

### GitHub Actions Example

```yaml
# .github/workflows/e2e-tests.yml
name: E2E Tests with Test Data

jobs:
  e2e-tests:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v3

      - name: Install ffmpeg
        run: sudo apt-get install -y ffmpeg

      - name: Generate test audiobooks
        run: |
          mkdir -p test-data/import
          cd test-data/import

          # Generate minimal test files
          for i in {1..3}; do
            ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 1 \
              -metadata title="CI Test Book $i" \
              -c:a aac -b:a 32k \
              test-book-$i.m4b -loglevel quiet -y
          done

      - name: Run application with test data
        run: |
          ./audiobook-organizer serve --port 8888 --dir test-data/import &
          sleep 5  # Wait for startup

      - name: Run E2E tests
        run: |
          cd web
          npm run test:e2e
```

---

## Test Data Cleanup

### After Test Session

```bash
# Remove all test audiobooks
rm -rf ~/test-audiobooks/import/*.m4b

# Keep reference files
# Don't delete: test-hashes.txt, metadata-reference.json

# Reset database (if needed)
rm -f audiobooks.pebble

# Regenerate test files
~/test-audiobooks/generate-test-files.sh  # If you created a script
```

### Complete Reset

```bash
# WARNING: Deletes all test data
rm -rf ~/test-audiobooks
```

---

## Troubleshooting

### Issue: ffmpeg not found

**Solution**:

```bash
# macOS
brew install ffmpeg

# Linux
sudo apt-get install ffmpeg

# Windows
# Download from https://ffmpeg.org/download.html
```

### Issue: File format not recognized

**Solution**:

```bash
# Verify file format
file audiobook.m4b
ffprobe audiobook.m4b

# Convert to supported format
ffmpeg -i audiobook.ogg -c:a aac audiobook.m4b
```

### Issue: Metadata not extracted correctly

**Solution**:

```bash
# Check metadata tags
ffprobe audiobook.m4b 2>&1 | grep -E "title|author|artist"

# Re-add metadata
ffmpeg -i input.m4b -metadata title="New Title" \
  -c copy output.m4b
```

---

## Quick Reference Scripts

### Generate Complete Test Suite

```bash
#!/bin/bash
# File: generate-test-suite.sh

set -e

TEST_DIR="$HOME/test-audiobooks"
mkdir -p "$TEST_DIR/import"
cd "$TEST_DIR/import"

echo "Generating audiobook test suite..."

# P0: Basic metadata
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Test Book 001: Basic" \
  -metadata author="Test Author" \
  -c:a aac -b:a 64k test-book-001.m4b -loglevel quiet -y

# P0: Rich metadata
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Test Book 002: Rich Metadata" \
  -metadata author="John R. Doe" \
  -metadata artist="Jane A. Smith" \
  -metadata album="Test Series - Book 2" \
  -metadata genre="Science Fiction" \
  -metadata date="2023" \
  -metadata publisher="Test Publishing" \
  -c:a aac -b:a 128k test-book-002.m4b -loglevel quiet -y

# P0: Minimal metadata
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Test Book 003: Minimal" \
  -c:a aac -b:a 64k test-book-003.m4b -loglevel quiet -y

# Provenance tests
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Provenance Test A" \
  -metadata author="File Author" \
  -c:a aac -b:a 64k provenance-test-a.m4b -loglevel quiet -y

# Blocked hash test
ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
  -metadata title="Blocked Hash Test" \
  -c:a aac -b:a 64k blocked-hash-test.m4b -loglevel quiet -y

# State transition tests
for i in {1..3}; do
  ffmpeg -f lavfi -i anullsrc=r=44100:cl=mono -t 10 \
    -metadata title="State Test $i" \
    -c:a aac -b:a 64k state-test-$i.m4b -loglevel quiet -y
done

echo "✅ Test suite generated in $TEST_DIR/import"
echo "Files created: $(ls -1 *.m4b | wc -l)"

# Generate hash reference
echo "Computing hashes..."
shasum -a 256 *.m4b > ../test-hashes.txt
echo "✅ Hashes saved to $TEST_DIR/test-hashes.txt"
```

### Make executable and run

```bash
chmod +x generate-test-suite.sh
./generate-test-suite.sh
```

---

## Version History

- **1.0.0** (2025-12-28): Initial test data setup guide created
  - Test file generation with ffmpeg
  - Hash computation guide
  - CI/CD integration examples
  - Complete test suite generation script

---

**Related Documents**:

- [Manual Test Plan](./MANUAL_TEST_PLAN.md)
- [P0 Test Checklist](./MANUAL_TEST_CHECKLIST_P0.md)
- [E2E Test Coverage](../web/tests/e2e/TEST_COVERAGE_SUMMARY.md)
