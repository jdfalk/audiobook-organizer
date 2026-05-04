<!-- file: docs/superpowers/bot-tasks/2026-04-29-deluge-1-db-migration.md -->
<!-- version: 1.0.0 -->
<!-- guid: 3a7f2c1d-b8e5-4091-a6f3-9c2d0e4b7a85 -->
<!-- last-edited: 2026-04-29 -->

# BOT TASK: DELUGE-1 — Add Deluge Columns to book_files

**TODO ID:** DELUGE-1
**Audience:** burndown bot
**Branch:** `feat/deluge-1-db-columns`
**PR title:** `feat(deluge): add deluge_hash/original_path/imported_at columns to book_files`

---

## What This Task Does

Adds three new columns to the `book_files` SQLite table:

- `deluge_hash TEXT` — the Deluge torrent info-hash for this file's torrent
- `deluge_original_path TEXT` — the original file path before it was copied into
  the library (i.e., where it lived in the Deluge save directory)
- `imported_from_deluge_at TIMESTAMP` — when the copy-into-library happened

These columns are added via the **`ensureExtendedBookFileColumns`** pattern — NOT
via a `.sql` migration file. Read the "What NOT to do" section carefully.

---

## What NOT to Do

- **Do NOT create any file in `internal/database/migrations/`** (or any `.sql`
  file anywhere). This project uses the `ensureExtended*` pattern for optional
  columns on existing tables.
- **Do NOT touch PebbleDB** (`internal/database/pebble_store.go`). PebbleDB is
  schema-free JSON. Column changes only apply to SQLite.
- **Do NOT modify `ensureExtendedBookColumns`** (which is for the `books` table).
  You are adding a **separate** function for `book_files`.
- **Do NOT rename or remove any existing field** from `BookFile`. Only append new
  fields.
- **Do NOT guess import paths.** Use exactly the imports already in the file.

---

## Files to Change

Only these two files:

1. `internal/database/store.go`
2. `internal/database/sqlite_store.go`

Do not create any new files. Do not modify any other files.

---

## Step 1 — Edit `internal/database/store.go`

### Find the `BookFile` struct

Open `internal/database/store.go`. Search for the line:

```
type BookFile struct {
```

It is at approximately line 573. The struct ends with:

```go
	OrganizeMethod        string    `json:"organize_method,omitempty"` // "reflink", "hardlink", "copy", "symlink"
	Missing            bool      `json:"missing"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
```

### Add three fields BEFORE the closing brace `}`

Add the following three lines immediately before `}` (the closing brace of `BookFile`):

```go
	// Deluge integration fields (spec: deluge-protected-paths-design).
	// DelugeHash is the torrent info-hash (40-char hex string).
	// DelugeOriginalPath is the file path before copy-into-library.
	// ImportedFromDelugeAt is when the copy completed.
	DelugeHash             string     `json:"deluge_hash,omitempty"`
	DelugeOriginalPath     string     `json:"deluge_original_path,omitempty"`
	ImportedFromDelugeAt   *time.Time `json:"imported_from_deluge_at,omitempty"`
```

**Column types explanation:**

- `deluge_hash TEXT` — hex string like `"a1b2c3d4..."`, nullable (empty = not from Deluge)
- `deluge_original_path TEXT` — absolute file path string, nullable
- `imported_from_deluge_at TIMESTAMP` — SQLite stores timestamps as TEXT in ISO-8601.
  In Go this is `*time.Time` (pointer) so it can be nil when the book was not imported
  from Deluge. Use `sql.NullTime` when scanning.

**The result should look like this (full end of struct):**

```go
	OrganizeMethod        string    `json:"organize_method,omitempty"` // "reflink", "hardlink", "copy", "symlink"
	Missing            bool      `json:"missing"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	// Deluge integration fields (spec: deluge-protected-paths-design).
	// DelugeHash is the torrent info-hash (40-char hex string).
	// DelugeOriginalPath is the file path before copy-into-library.
	// ImportedFromDelugeAt is when the copy completed.
	DelugeHash             string     `json:"deluge_hash,omitempty"`
	DelugeOriginalPath     string     `json:"deluge_original_path,omitempty"`
	ImportedFromDelugeAt   *time.Time `json:"imported_from_deluge_at,omitempty"`
}
```

---

## Step 2 — Edit `internal/database/sqlite_store.go`

You will make **three** changes to this file:

### Change A — Add `ensureExtendedBookFileColumns` function

Find the line:

```go
// Close closes the database connection
func (s *SQLiteStore) Close() error {
```

It is at approximately line 797. Insert the following function **immediately before** that line (before the comment `// Close closes the database connection`):

```go
// ensureExtendedBookFileColumns adds newly introduced optional columns to the
// book_files table for existing databases created before these columns existed.
// SQLite lacks IF NOT EXISTS for ADD COLUMN, so we inspect PRAGMA table_info
// and conditionally ALTER TABLE.
func (s *SQLiteStore) ensureExtendedBookFileColumns() error {
	columns := map[string]string{
		"deluge_hash":           "TEXT",
		"deluge_original_path":  "TEXT",
		"imported_from_deluge_at": "TIMESTAMP",
	}

	rows, err := s.db.Query("PRAGMA table_info(book_files)")
	if err != nil {
		return fmt.Errorf("failed to inspect book_files schema: %w", err)
	}
	defer rows.Close()

	existing := make(map[string]struct{})
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("failed to scan book_files table_info: %w", err)
		}
		existing[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating book_files table_info: %w", err)
	}

	for name, colType := range columns {
		if _, ok := existing[name]; ok {
			continue
		}
		alter := fmt.Sprintf("ALTER TABLE book_files ADD COLUMN %s %s", name, colType)
		if _, err := s.db.Exec(alter); err != nil {
			return fmt.Errorf("failed adding column %s to book_files: %w", name, err)
		}
	}
	return nil
}
```

### Change B — Call `ensureExtendedBookFileColumns` from the init/open function

Find the line in the `open` or `initSchema` function that reads:

```go
	// Non-destructive migration for existing databases: add missing columns
	return s.ensureExtendedBookColumns()
```

It is at approximately line 637-638. Change it to:

```go
	// Non-destructive migration for existing databases: add missing columns
	if err := s.ensureExtendedBookColumns(); err != nil {
		return err
	}
	return s.ensureExtendedBookFileColumns()
```

**IMPORTANT:** The existing call returns directly. You must split the return so
both functions are called. The exact replacement is shown above.

### Change C — Update `bookFileCols` constant and `bookFileScan` function

#### C1 — Update `bookFileCols`

Find the constant (approximately line 5320):

```go
const bookFileCols = `id, book_id, file_path, original_filename, itunes_path, itunes_persistent_id,
	track_number, track_count, disc_number, disc_count, title, format, codec, duration,
	file_size, bitrate_kbps, sample_rate_hz, channels, bit_depth, file_hash, original_file_hash,
	acoustid_seg0, acoustid_seg1, acoustid_seg2, acoustid_seg3, acoustid_seg4, acoustid_seg5, acoustid_seg6,
	missing, created_at, updated_at`
```

Replace it with:

```go
const bookFileCols = `id, book_id, file_path, original_filename, itunes_path, itunes_persistent_id,
	track_number, track_count, disc_number, disc_count, title, format, codec, duration,
	file_size, bitrate_kbps, sample_rate_hz, channels, bit_depth, file_hash, original_file_hash,
	acoustid_seg0, acoustid_seg1, acoustid_seg2, acoustid_seg3, acoustid_seg4, acoustid_seg5, acoustid_seg6,
	missing, created_at, updated_at,
	deluge_hash, deluge_original_path, imported_from_deluge_at`
```

#### C2 — Update `bookFileScan` to scan the new columns

Find the variable declarations at the top of `bookFileScan` (approximately line 5332):

```go
	var originalFilename, itunesPath, itunesPID sql.NullString
```

After the existing `var` declarations, add:

```go
	var delugeHash, delugeOriginalPath sql.NullString
	var importedFromDelugeAt sql.NullTime
```

Find the `row.Scan(...)` call inside `bookFileScan`. It currently ends with:

```go
		&missing, &f.CreatedAt, &f.UpdatedAt,
	)
```

Change that ending to:

```go
		&missing, &f.CreatedAt, &f.UpdatedAt,
		&delugeHash, &delugeOriginalPath, &importedFromDelugeAt,
	)
```

After the `if err != nil { return f, err }` block, and after the existing `if acoustidSeg6.Valid { ... }` and `f.Missing = missing != 0` assignments, add:

```go
	if delugeHash.Valid {
		f.DelugeHash = delugeHash.String
	}
	if delugeOriginalPath.Valid {
		f.DelugeOriginalPath = delugeOriginalPath.String
	}
	if importedFromDelugeAt.Valid {
		t := importedFromDelugeAt.Time
		f.ImportedFromDelugeAt = &t
	}
```

Place these lines immediately before the `return f, nil` statement.

---

## Step 3 — Verify the Build

Run these two commands in order. Both must pass with zero errors.

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go build ./...
```

```bash
go test ./internal/database/...
```

If `go build` fails with "undefined: sql.NullTime", the Go version is below 1.15.
`sql.NullTime` was added in Go 1.15. Check `go.mod` — the module declares `go 1.24.0`
so this is not expected to fail. If it does fail for another reason, read the error
message carefully and fix only the line it points to.

If `go test` fails, read the test output. The most likely failure is that
`bookFileScan` now tries to scan 3 more columns than `CreateBookFile` inserts. If
that happens, you also need to update `CreateBookFile` and `UpdateBookFile` to
include the new columns — but check the test output first before touching those
functions.

---

## Step 4 — Bump the Version Header

The version header is at the top of each file you changed. Bump the patch version:

- `internal/database/store.go` — increment the `// version:` line by 0.0.1
- `internal/database/sqlite_store.go` — increment the `// version:` line by 0.0.1

---

## Step 5 — Commit and Open PR

```bash
git checkout -b feat/deluge-1-db-columns
git add internal/database/store.go internal/database/sqlite_store.go
git commit -m "feat(deluge): add deluge_hash/original_path/imported_at columns to book_files"
git push -u origin feat/deluge-1-db-columns
gh pr create \
  --title "feat(deluge): add deluge_hash/original_path/imported_at columns to book_files" \
  --body "Adds three nullable columns to book_files via ensureExtendedBookFileColumns (PRAGMA table_info + ALTER TABLE). No migration file needed. Part of the Deluge protected-paths integration (spec: 2026-04-29-deluge-protected-paths-design.md)."
```

---

## Checklist

- [ ] `BookFile` struct has three new fields in `store.go`
- [ ] `ensureExtendedBookFileColumns` function added to `sqlite_store.go`
- [ ] `ensureExtendedBookFileColumns` called from init/open (after `ensureExtendedBookColumns`)
- [ ] `bookFileCols` constant updated to include the three new column names
- [ ] `bookFileScan` scans the three new columns via `sql.NullString` / `sql.NullTime`
- [ ] `go build ./...` passes
- [ ] `go test ./internal/database/...` passes
- [ ] Version headers bumped in both files
- [ ] PR opened with correct branch name and title
