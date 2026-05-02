# Suspicious File Detection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect single-file "books" below a configurable size threshold, save them with `library_state='suspicious'`, and skip all expensive processing (hashing, tag extraction, AI).

**Architecture:** A fast `os.Stat` guard in `ProcessBooksParallel` fires right after the incremental-skip check. The scanner `Book` struct gains a `LibraryState` field so `saveBookToDatabase` can honour it instead of hardcoding `'imported'`. No new DB columns or migrations needed.

**Tech Stack:** Go 1.24, SQLite (via existing store), React/TypeScript (MUI FilterSidebar)

---

## File Map

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `MinBookSizeBytes int64` field, viper wiring, default, validation |
| `internal/scanner/scanner.go` | Add `LibraryState string` to `Book` struct; thread through `saveBookToDatabase`; add guard in `ProcessBooksParallel` |
| `internal/config/config_unit_test.go` | Add `TestMinBookSizeBytesDefault` |
| `internal/scanner/scanner_test.go` | Add `TestSuspiciousFileSkipped` |
| `web/src/components/audiobooks/FilterSidebar.tsx` | Add `<MenuItem value="suspicious">` |
| `web/src/components/audiobooks/SearchBar.tsx` | Add `library_state:suspicious` example |

---

## Task 1: Config — MinBookSizeBytes field

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_unit_test.go`

- [ ] **Step 1: Write the failing test**

Open `internal/config/config_unit_test.go`. Add at the bottom, before the final `}`:

```go
func TestMinBookSizeBytesDefault(t *testing.T) {
	c := &Config{MinBookSizeBytes: 0}
	_ = c.Validate()
	assert.Equal(t, int64(10*1024*1024), c.MinBookSizeBytes,
		"zero value should be coerced to 10 MB default")
}

func TestMinBookSizeBytesDisable(t *testing.T) {
	c := &Config{MinBookSizeBytes: -1}
	_ = c.Validate()
	assert.Equal(t, int64(-1), c.MinBookSizeBytes,
		"-1 sentinel should not be coerced")
}

func TestMinBookSizeBytesCustom(t *testing.T) {
	c := &Config{MinBookSizeBytes: 5 * 1024 * 1024}
	_ = c.Validate()
	assert.Equal(t, int64(5*1024*1024), c.MinBookSizeBytes,
		"explicit value should be preserved")
}
```

- [ ] **Step 2: Run to confirm fail**

```bash
cd /path/to/repo && go test ./internal/config/... -run TestMinBookSizeBytes -v
```
Expected: `FAIL — undefined: Config.MinBookSizeBytes`

- [ ] **Step 3: Add the struct field**

In `internal/config/config.go`, find the Performance section (around line 134):
```go
	// Performance
	ConcurrentScans int `json:"concurrent_scans"`
	// Background operation timeout in minutes (0 disables timeout)
	OperationTimeoutMinutes int `json:"operation_timeout_minutes"`
```
Insert after `OperationTimeoutMinutes`:
```go
	// MinBookSizeBytes: single-file books below this size are flagged as suspicious and
	// skipped for heavy processing. Set to -1 to disable. Defaults to 10485760 (10 MB).
	MinBookSizeBytes int64 `json:"min_book_size_bytes"`
```

- [ ] **Step 4: Wire viper**

In the same file, find the Performance viper block (around line 533):
```go
		ConcurrentScans:         viper.GetInt("concurrent_scans"),
		OperationTimeoutMinutes: viper.GetInt("operation_timeout_minutes"),
```
Add after `OperationTimeoutMinutes` line:
```go
		MinBookSizeBytes:        viper.GetInt64("min_book_size_bytes"),
```

- [ ] **Step 5: Add default**

Find `DefaultConfig()` (returns `&Config{...}`). Locate the Performance defaults section (around line 914):
```go
		// Performance
		ConcurrentScans:         max(runtime.NumCPU(), 4),
		OperationTimeoutMinutes: 30,
```
Add after `OperationTimeoutMinutes`:
```go
		MinBookSizeBytes:        10 * 1024 * 1024,
```

- [ ] **Step 6: Add validation / default coercion**

Find the `Validate()` method, at the `if c.ConcurrentScans < 0` block (around line 812). Add after it:
```go
	if c.MinBookSizeBytes == 0 {
		c.MinBookSizeBytes = 10 * 1024 * 1024
	}
```

- [ ] **Step 7: Bump file version header**

The file starts with `// version: X.Y.Z`. Increment the patch version (e.g. `1.0.0` → `1.0.1`).

- [ ] **Step 8: Run tests to confirm pass**

```bash
go test ./internal/config/... -run TestMinBookSizeBytes -v
```
Expected: all three tests PASS.

- [ ] **Step 9: Run full config suite to check for regressions**

```bash
go test ./internal/config/... -v 2>&1 | tail -20
```
Expected: PASS (no failures).

- [ ] **Step 10: Commit**

```bash
git add internal/config/config.go internal/config/config_unit_test.go
git commit -m "feat(config): add MinBookSizeBytes threshold for suspicious file detection"
```

---

## Task 2: Scanner Book.LibraryState + saveBookToDatabase passthrough

**Files:**
- Modify: `internal/scanner/scanner.go`
- Test: `internal/scanner/scanner_test.go`

- [ ] **Step 1: Write the failing test**

Open `internal/scanner/scanner_test.go`. Add after `TestProcessBooks`:

```go
func TestBookLibraryStateField(t *testing.T) {
	// Verifies the LibraryState field exists on Book and is threaded to saveBook.
	var capturedState string
	oldSaver := saveBook
	t.Cleanup(func() { saveBook = oldSaver })
	saveBook = func(b *Book) error {
		capturedState = b.LibraryState
		return nil
	}

	books := withTempBooks(t, []string{"test.mp3"})
	books[0].LibraryState = "suspicious"

	oldExts := config.AppConfig.SupportedExtensions
	t.Cleanup(func() { config.AppConfig.SupportedExtensions = oldExts })
	config.AppConfig.SupportedExtensions = []string{".mp3"}

	// Drive through saveBook directly (unit-level: just check the var is called).
	err := saveBook(&books[0])
	if err != nil {
		t.Fatalf("saveBook returned error: %v", err)
	}
	if capturedState != "suspicious" {
		t.Errorf("expected LibraryState=suspicious, got %q", capturedState)
	}
}
```

- [ ] **Step 2: Run to confirm fail**

```bash
go test ./internal/scanner/... -run TestBookLibraryStateField -v
```
Expected: `FAIL — b.LibraryState undefined`

- [ ] **Step 3: Add LibraryState to the Book struct**

In `internal/scanner/scanner.go`, find the `Book` struct (line 108):
```go
type Book struct {
	FilePath        string
	Title           string
	Author          string
	Series          string
	Position        int
	Format          string
	Duration        int
	Narrator        string
	Language        string
	Publisher       string
	BookOrganizerID string // Embedded AUDIOBOOK_ORGANIZER_ID for re-linking
	ASIN            string
	OpenLibraryID   string
	HardcoverID     string
	SegmentFiles    []string // For multi-file books grouped by album in mixed directories
	GoogleBooksID   string
	FileHash        string // Pre-computed hash from ProcessFile (avoids double-read)
}
```
Replace with:
```go
type Book struct {
	FilePath        string
	Title           string
	Author          string
	Series          string
	Position        int
	Format          string
	Duration        int
	Narrator        string
	Language        string
	Publisher       string
	BookOrganizerID string // Embedded AUDIOBOOK_ORGANIZER_ID for re-linking
	ASIN            string
	OpenLibraryID   string
	HardcoverID     string
	SegmentFiles    []string // For multi-file books grouped by album in mixed directories
	GoogleBooksID   string
	FileHash        string // Pre-computed hash from ProcessFile (avoids double-read)
	LibraryState    string // If set, overrides the default "imported" state in saveBookToDatabase
}
```

- [ ] **Step 4: Thread LibraryState through saveBookToDatabase**

In `saveBookToDatabase` (line ~1424), find:
```go
		dbBook := &database.Book{
			Title:             book.Title,
```
Replace with:
```go
		ls := "imported"
		if book.LibraryState != "" {
			ls = book.LibraryState
		}
		dbBook := &database.Book{
			Title:             book.Title,
```

Then find:
```go
			LibraryState:      stringPtr("imported"),
```
Replace with:
```go
			LibraryState:      stringPtr(ls),
```

- [ ] **Step 5: Bump file version header**

Increment the patch version in the `// version:` header at the top of `scanner.go`.

- [ ] **Step 6: Run test to confirm pass**

```bash
go test ./internal/scanner/... -run TestBookLibraryStateField -v
```
Expected: PASS.

- [ ] **Step 7: Run full scanner suite**

```bash
go test ./internal/scanner/... -v 2>&1 | tail -20
```
Expected: no new failures.

- [ ] **Step 8: Commit**

```bash
git add internal/scanner/scanner.go internal/scanner/scanner_test.go
git commit -m "feat(scanner): add Book.LibraryState and thread through saveBookToDatabase"
```

---

## Task 3: Suspicious-file guard in ProcessBooksParallel

**Files:**
- Modify: `internal/scanner/scanner.go`
- Test: `internal/scanner/scanner_test.go`

- [ ] **Step 1: Write the failing test**

Open `internal/scanner/scanner_test.go`. Add after `TestBookLibraryStateField`:

```go
func TestSuspiciousFileSkipped(t *testing.T) {
	oldThreshold := config.AppConfig.MinBookSizeBytes
	oldExts := config.AppConfig.SupportedExtensions
	t.Cleanup(func() {
		config.AppConfig.MinBookSizeBytes = oldThreshold
		config.AppConfig.SupportedExtensions = oldExts
	})
	config.AppConfig.MinBookSizeBytes = 1024 * 1024 // 1 MB threshold
	config.AppConfig.SupportedExtensions = []string{".mp3"}

	dir := t.TempDir()
	smallFile := filepath.Join(dir, "tiny.mp3")
	// Write 100 bytes — well under the 1 MB threshold.
	if err := os.WriteFile(smallFile, make([]byte, 100), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	books := []Book{{FilePath: smallFile, Format: ".mp3"}}

	var savedBook *Book
	oldSaver := saveBook
	t.Cleanup(func() { saveBook = oldSaver })
	saveBook = func(b *Book) error {
		saved := *b
		savedBook = &saved
		return nil
	}

	err := ProcessBooksParallel(context.Background(), books, 1, nil, nil)
	if err != nil {
		t.Fatalf("ProcessBooksParallel: %v", err)
	}
	if savedBook == nil {
		t.Fatal("expected saveBook to be called for suspicious file")
	}
	if savedBook.LibraryState != "suspicious" {
		t.Errorf("expected LibraryState=suspicious, got %q", savedBook.LibraryState)
	}
}

func TestSuspiciousFileSkipDisabledWhenNegativeOne(t *testing.T) {
	oldThreshold := config.AppConfig.MinBookSizeBytes
	oldExts := config.AppConfig.SupportedExtensions
	t.Cleanup(func() {
		config.AppConfig.MinBookSizeBytes = oldThreshold
		config.AppConfig.SupportedExtensions = oldExts
	})
	config.AppConfig.MinBookSizeBytes = -1 // disabled
	config.AppConfig.SupportedExtensions = []string{".mp3"}

	dir := t.TempDir()
	smallFile := filepath.Join(dir, "tiny.mp3")
	if err := os.WriteFile(smallFile, make([]byte, 100), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	books := []Book{{FilePath: smallFile, Format: ".mp3"}}

	var savedState string
	oldSaver := saveBook
	t.Cleanup(func() { saveBook = oldSaver })
	saveBook = func(b *Book) error {
		savedState = b.LibraryState
		return nil
	}

	err := ProcessBooksParallel(context.Background(), books, 1, nil, nil)
	if err != nil {
		t.Fatalf("ProcessBooksParallel: %v", err)
	}
	if savedState == "suspicious" {
		t.Error("threshold=-1 should disable suspicious detection, but book was flagged")
	}
}
```

- [ ] **Step 2: Run to confirm fail**

```bash
go test ./internal/scanner/... -run "TestSuspiciousFile" -v
```
Expected: both tests FAIL (guard does not exist yet, so `LibraryState` stays empty).

- [ ] **Step 3: Add the guard to ProcessBooksParallel**

In `scanner.go`, find this exact block (around line 369):
```go
			fallbackUsed := false
			filePath := books[idx].FilePath

			// Handle directory-based books (multi-file books grouped by album tag)
			if info, statErr := os.Stat(filePath); statErr == nil && info.IsDir() {
```
Replace with:
```go
			fallbackUsed := false
			filePath := books[idx].FilePath

			// Suspicious-file guard: single files below MinBookSizeBytes skip heavy processing.
			if threshold := config.AppConfig.MinBookSizeBytes; threshold > 0 {
				if fi, statErr := os.Stat(filePath); statErr == nil && !fi.IsDir() && fi.Size() < threshold {
					extractInfoFromPath(&books[idx])
					books[idx].LibraryState = "suspicious"
					if saveErr := saveBook(&books[idx]); saveErr != nil {
						scanLog.Warn("failed to save suspicious book %s: %v", filePath, saveErr)
					}
					scanLog.Warn("suspicious file (%d bytes, threshold %d): %s", fi.Size(), threshold, filePath)
					func() {
						defer func() { recover() }()
						if store := database.GetGlobalStore(); store != nil {
							if dbBook, dbErr := store.GetBookByFilePath(filePath); dbErr == nil && dbBook != nil {
								_ = store.UpdateScanCache(dbBook.ID, fi.ModTime().Unix(), fi.Size())
							}
						}
					}()
					return
				}
			}

			// Handle directory-based books (multi-file books grouped by album tag)
			if info, statErr := os.Stat(filePath); statErr == nil && info.IsDir() {
```

- [ ] **Step 4: Bump version header**

Increment patch version in `// version:` at top of `scanner.go`.

- [ ] **Step 5: Run tests to confirm pass**

```bash
go test ./internal/scanner/... -run "TestSuspiciousFile" -v
```
Expected: both PASS.

- [ ] **Step 6: Run full scanner suite**

```bash
go test ./internal/scanner/... -v 2>&1 | tail -30
```
Expected: no new failures.

- [ ] **Step 7: Run full backend test suite**

```bash
make test 2>&1 | tail -20
```
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/scanner/scanner.go internal/scanner/scanner_test.go
git commit -m "feat(scanner): flag single-file books below MinBookSizeBytes as suspicious"
```

---

## Task 4: Frontend — add 'suspicious' to FilterSidebar and SearchBar

**Files:**
- Modify: `web/src/components/audiobooks/FilterSidebar.tsx`
- Modify: `web/src/components/audiobooks/SearchBar.tsx`

No automated test for this task — verify visually after running the dev server.

- [ ] **Step 1: Add MenuItem to FilterSidebar**

Open `web/src/components/audiobooks/FilterSidebar.tsx`. Find:
```tsx
              <MenuItem value="deleted">Deleted</MenuItem>
            </Select>
```
Replace with:
```tsx
              <MenuItem value="deleted">Deleted</MenuItem>
              <MenuItem value="suspicious">Suspicious</MenuItem>
            </Select>
```

- [ ] **Step 2: Add search example to SearchBar**

Open `web/src/components/audiobooks/SearchBar.tsx`. Find:
```tsx
  { example: 'library_state:imported', desc: 'Imported but not organized' },
```
Add after it:
```tsx
  { example: 'library_state:suspicious', desc: 'Suspicious / incomplete files' },
```

- [ ] **Step 3: Bump version headers**

Increment patch version in the `// version:` header of both modified `.tsx` files.

- [ ] **Step 4: Build frontend to confirm no type errors**

```bash
make build-api   # backend only — fast check
cd web && npm run build 2>&1 | tail -20
```
Expected: no TypeScript errors, build succeeds.

- [ ] **Step 5: Smoke test in browser**

```bash
make run-api
```
Open the app, open the filter sidebar, confirm "Suspicious" appears in the Library State dropdown.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/audiobooks/FilterSidebar.tsx \
        web/src/components/audiobooks/SearchBar.tsx
git commit -m "feat(ui): add suspicious library state to filter sidebar and search examples"
```

---

## Self-Review Checklist

- [x] Config: zero-value coercion (→ 10 MB) and -1 disable path both tested
- [x] Guard calls `saveBook` (the mockable var), not `saveBookToDatabase` directly — testable
- [x] Guard fires only for single files (`!fi.IsDir()`) — multi-file directory books unaffected
- [x] `LibraryState` threads from `Book` struct into `saveBookToDatabase` dbBook
- [x] Scan cache updated after suspicious save so incremental-skip catches it on next run
- [x] recover() guard around scan cache update (matches existing pattern at line 594)
- [x] Frontend: existing `library_state` filter infrastructure handles `suspicious` with no new endpoints
- [x] No migrations needed — `library_state` is already free-form TEXT
