<!-- file: scripts/TEST-ORGANIZE-README.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d -->

# Audiobook Organizer Testing Guide

This guide explains how to use the `test-organize-import.py` script to test the
audiobook organizer's scanning, metadata extraction, and file organization logic
against large file lists.

## Purpose

The test script validates that the audiobook-organizer can:

- **Scan large file lists** efficiently
- **Extract metadata** from file paths and names (title, author, series,
  position)
- **Identify series** using pattern matching and heuristics
- **Generate organized paths** showing how files would be renamed/organized
- **Report issues** with problematic files

## Requirements

- Python 3.13.0 or higher
- The `file-list-books` file (or any text file with one file path per line)

## Basic Usage

### Process the full file list

```bash
cd ~/repos/audiobook-organizer
./scripts/test-organize-import.py /Users/jdfalk/repos/scratch/file-list-books
```

### Test with a limited number of files

```bash
./scripts/test-organize-import.py /Users/jdfalk/repos/scratch/file-list-books --limit 1000
```

### Customize output and samples

```bash
./scripts/test-organize-import.py /Users/jdfalk/repos/scratch/file-list-books \
  --output my-report.json \
  --sample 20
```

## Command Line Options

| Option           | Description                           | Default                     |
| ---------------- | ------------------------------------- | --------------------------- |
| `file_list`      | Path to file list (one path per line) | Required                    |
| `--output`, `-o` | Output JSON file for detailed report  | `organize-test-report.json` |
| `--limit`, `-l`  | Limit number of files to process      | All files                   |
| `--sample`, `-s` | Number of sample books to display     | 10                          |

## Output

### Console Output

The script prints:

1. **Progress updates** (every 1000 files)
2. **Summary statistics**:
   - Total files processed
   - Books with series identified
   - Confidence levels (high/medium/low)
   - Books with issues
3. **File format breakdown**
4. **Extraction method statistics**
5. **Top 10 authors and series**
6. **Common issues**
7. **Sample book details** (configurable)

### JSON Report

A detailed JSON report is saved with:

```json
{
  "summary": {
    "total_files_processed": 150000,
    "books_with_series": 120000,
    "books_with_position": 95000,
    "high_confidence": 85000,
    "medium_confidence": 45000,
    "low_confidence": 20000,
    "books_with_issues": 35000
  },
  "statistics": {
    "by_extension": { ".m4b": 50000, ".mp3": 100000 },
    "by_extraction_method": { "pattern:numbered_series": 60000, ... },
    "top_authors": { "Author Name": 250 },
    "top_series": { "Series Name": 12 }
  },
  "books": [
    {
      "original_path": "/mnt/bigdata/books/...",
      "title": "Book Title",
      "author": "Author Name",
      "series": "Series Name",
      "position": 1,
      "confidence": "high",
      "extraction_method": "pattern:numbered_series",
      "proposed_path": "Author Name/Series Name/01 - Book Title.m4b",
      "issues": []
    }
  ],
  "issues_summary": {
    "Missing or unclear title": 5000,
    "Missing or unclear author": 3000
  }
}
```

## Understanding Results

### Confidence Levels

- **High**: Title, author, series, and position all identified
- **Medium**: At least title and author identified
- **Low**: Missing critical metadata

### Extraction Methods

The script reports which pattern matching method successfully identified series:

- `pattern:dash_separator` - "Series - Title" format
- `pattern:numbered_series` - "Series 1: Title" format
- `pattern:book_numbered` - "Series Book 1: Title" format
- `pattern:hash_numbered` - "Series #1: Title" format
- `pattern:volume_numbered` - "Series Vol. 1: Title" format
- `directory:keyword` - Series identified from directory name with keywords
- `directory:fuzzy` - Series identified from directory name (fuzzy match)
- `title:colon` - Series identified from title (colon separator)
- `title:dash` - Series identified from title (dash separator)
- `none` - No series detected

### Proposed Path Structure

The script generates organized paths following this pattern:

```text
Author/Series/Position - Title.ext
Author/Title.ext  (if no series)
```

Examples:

- `Timothy Zahn/Star Wars Thrawn/01 - Thrawn.m4b`
- `Brandon Sanderson/Mistborn/02 - The Well of Ascension.m4b`
- `Unknown_Author/Single Book.mp3` (if metadata unclear)

## Common Issues

The script identifies these common problems:

1. **Missing or unclear title** - Cannot extract a clear title from
   path/filename
2. **Missing or unclear author** - Cannot identify author from directory
   structure
3. **Low confidence in metadata extraction** - Overall poor metadata quality
4. **No series pattern detected** - Book might be standalone or pattern not
   recognized

## Example Session

```bash
$ ./scripts/test-organize-import.py /Users/jdfalk/repos/scratch/file-list-books --limit 5000

Reading file list from: /Users/jdfalk/repos/scratch/file-list-books
Processing 5,000 files...
  Processed 1,000 files...
  Processed 2,000 files...
  Processed 3,000 files...
  Processed 4,000 files...
Scanned 4,523 valid audiobook files

================================================================================
AUDIOBOOK ORGANIZER TEST REPORT
================================================================================

SUMMARY:
  Total files processed: 4,523
  Books with series identified: 3,891
  Books with series position: 2,456

CONFIDENCE LEVELS:
  High confidence: 3,234
  Medium confidence: 987
  Low confidence: 302

  Books with issues: 654

FILE FORMATS:
  .m4b: 2,341
  .mp3: 2,182

EXTRACTION METHODS:
  pattern:numbered_series: 1,567
  pattern:dash_separator: 891
  directory:keyword: 543
  none: 632
  ...

TOP 10 AUTHORS:
  Brandon Sanderson: 47 books
  Timothy Zahn: 38 books
  ...

SAMPLE OF 10 BOOKS:
--------------------------------------------------------------------------------

Original: /mnt/bigdata/books/bt/incomplete/Star Wars - Thrawn - Timothy Zahn - Audiobook 2017/Chapter 18.mp3
  Title:
  Author: Star Wars - Thrawn - Timothy Zahn - Audiobook 2017
  Series:
  Method: none
  Confidence: low
  Proposed: Star Wars - Thrawn - Timothy Zahn - Audiobook 2017/Unknown_Title.mp3
  Issues: Missing or unclear title, Low confidence in metadata extraction, No series pattern detected

...

Detailed report written to: organize-test-report.json
```

## Interpreting Results for Development

Use this test to:

1. **Validate pattern matching** - Check if series patterns are correctly
   identified
2. **Identify edge cases** - Find problematic file naming conventions
3. **Measure accuracy** - Track confidence levels and issue rates
4. **Optimize algorithms** - Use results to improve extraction heuristics
5. **Test at scale** - Ensure the organizer handles large libraries efficiently

## Next Steps

After running the test:

1. Review the JSON report for detailed per-book results
2. Examine books with low confidence or issues
3. Update pattern matching in `internal/matcher/matcher.go` if needed
4. Test against actual Go implementation to verify parity
5. Use results to improve the audiobook-organizer logic

## Integration with Go Implementation

This Python script mirrors the logic from:

- `internal/scanner/scanner.go` - File scanning
- `internal/matcher/matcher.go` - Series pattern matching
- Proposed file organization logic

Compare results with actual Go implementation to ensure consistency.
