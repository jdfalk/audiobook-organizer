<!-- file: docs/mvp-tasks/task-4/4-ADVANCED-SCENARIOS.md -->
<!-- version: 1.0.1 -->
<!-- guid: 9f3c2d1e-8b4a-4d5c-9e7f-1a8b2c3d4e5f -->
<!-- last-edited: 2026-01-19 -->

# Task 4: Advanced Scenarios & Code Deep Dive (Duplicate Detection)

Use these scenarios when core testing passes but edge conditions need
validation.

## 🧮 Partial File Duplicates

**Risk:** Same book in different formats/quality leads to false positives.

```bash
# Test with transcoded versions
# Original: book.m4b
# Transcoded: book-128k.mp3
# These should NOT be duplicates (different content)
```

- Hash entire file content, not just metadata or first N bytes.
- Document: "Duplicates" means identical binary content, not semantic
  duplicates.

## 🔗 Symlinks and Hardlinks

**Risk:** Symlinks to same file reported as duplicates vs. distinct entries.

```bash
find /library -type l -maxdepth 5 2>/dev/null
```

- Decision: skip symlinks in duplicate detection (they point to same inode).
- Hardlinks share inode; detecting them requires inode tracking, not just hash.

## 📦 Bit-Perfect vs. Metadata Changes

**Risk:** File with updated ID3 tags has different hash but is same audio
content.

- SHA256 on entire file includes tags; updated tags = different hash.
- For audio-only duplicate detection (ignore metadata drift), need separate
  audio stream hashing (out of scope for MVP).

## 🚫 Missing or Corrupted Files

**Risk:** Stale DB entries with hashes for missing files skew duplicate count.

```bash
# Check for files in DB but not on disk
curl -s http://localhost:8888/api/v1/audiobooks?limit=1000 | jq -r '.items[] | .file_path' | while read f; do
  [ -f "$f" ] || echo "Missing: $f"
done
```

- Filter duplicate groups to exclude missing files before surfacing to UI.

## 🧹 Database Schema for Hashes

Expected schema:

```sql
ALTER TABLE books ADD COLUMN content_hash TEXT;
CREATE INDEX idx_books_content_hash ON books(content_hash);
```

- Indexed for fast duplicate queries:
  `SELECT * FROM books WHERE content_hash IN (SELECT content_hash FROM books GROUP BY content_hash HAVING COUNT(*) > 1)`.

## 🧰 Backend Code Checklist

- Scanner computes SHA256 during file read; stores in `content_hash` or
  `file_hash` column.
- Duplicate endpoint groups by hash, returns only groups with count > 1.
- API returns array of arrays (each inner array = duplicate group).
- No false grouping (different hashes lumped together).

## 🪛 Frontend Checklist

- Dashboard shows "X duplicates found" stat.
- Library page has "Duplicates" filter or dedicated view.
- Duplicate view shows groups with file paths, sizes, metadata for comparison.
- Actions: "Keep this one, delete others" workflow.

## 🔬 Performance Considerations

- Hashing large files (multi-GB audiobooks) during scan may be slow; show
  progress.
- Consider parallel hashing (goroutines) if scan becomes bottleneck.
- Cache hashes; re-hash only if file mtime changes (incremental scan
  optimization).

When an edge condition is identified, document in `4-TROUBLESHOOTING.md`.
