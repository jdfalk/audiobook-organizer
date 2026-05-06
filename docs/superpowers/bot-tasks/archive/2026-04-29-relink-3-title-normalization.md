<!-- file: docs/superpowers/bot-tasks/2026-04-29-relink-3-title-normalization.md -->
<!-- version: 1.0.0 -->
<!-- guid: 5d2f8b3a-9e1c-4a7d-b6f2-0c4e8a1d5f3b -->
<!-- last-edited: 2026-04-29 -->

# Bot Task: RELINK-3 — Title Colon-to-Underscore Normalization

> **Read every word.** Do exactly what is written. Do not guess, do not
> improvise, do not add extra changes. If something is unclear, STOP and ask.

---

## Goal

Fix a bug in `findInITunes` where a book titled `"Mistborn: The Final Empire"`
fails to match the iTunes filename `"Mistborn_ The Final Empire.m4b"`. macOS
and iTunes replace `: ` with `_ ` in filenames because colons are illegal in
HFS+ paths. The current code compares raw title strings without normalizing
colons, so the prefix check fails. The fix normalizes both the search title and
the filename candidate before comparison.

---

## File You Will Touch

`internal/server/maintenance_fixups.go`

Do NOT touch any other file until step 4 (tests).

---

## Step 0 — Find the function

Before making any edits, run:

```bash
grep -n "findInITunes\|titlePrefixLower\|normalizeForFilename" internal/server/maintenance_fixups.go
```

Confirm:
- `findInITunes` is a closure (around line 4154).
- `titlePrefixLower` is used inside the closure (several times).
- `normalizeForFilename` does NOT yet exist.

---

## Step 1 — Add the `normalizeForFilename` helper

This helper must be a **package-level function** (not a closure), defined
OUTSIDE the HTTP handler. Find the end of the file or a suitable place near
other helper functions in `maintenance_fixups.go`.

Add the following function. Place it BEFORE the handler that contains
`findInITunes`, or at the bottom of the file — either location is acceptable:

```go
// normalizeForFilename normalizes a string for iTunes/macOS filename comparison.
// macOS HFS+ and iTunes replace ": " and ":" with "_ " and "_" respectively
// because colons are illegal in filenames. This function applies the same
// transformation so that "Mistborn: The Final Empire" matches the file
// "Mistborn_ The Final Empire.m4b".
func normalizeForFilename(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ": ", "_ ")
	s = strings.ReplaceAll(s, ":", "_")
	return strings.TrimSpace(s)
}
```

**Do not** add this inside the `findInITunes` closure. It must be a top-level
function so the test file can call it too.

---

## Step 2 — Update `findInITunes` to use `normalizeForFilename`

### 2a. Find the title prefix computation

Inside `findInITunes`, find these lines (exact text):

```go
		titlePrefix := title
		if len(titlePrefix) > 25 {
			titlePrefix = titlePrefix[:25]
		}
		titlePrefixLower := strings.ToLower(titlePrefix)
```

Replace them with:

```go
		titlePrefix := title
		if len(titlePrefix) > 25 {
			titlePrefix = titlePrefix[:25]
		}
		// Normalize for macOS/iTunes filename encoding: ":" → "_", ": " → "_ "
		titlePrefixLower := normalizeForFilename(titlePrefix)
```

### 2b. Normalize filename candidates at the comparison sites

There are FOUR places inside `findInITunes` where `titlePrefixLower` is compared
against a filename or directory name. You must wrap each left-hand side with
`normalizeForFilename(...)`.

Find and replace each occurrence exactly as shown:

**Occurrence 1** — album directory name (fast path):

Find:
```go
					if strings.Contains(strings.ToLower(album.Name()), titlePrefixLower) {
						dirMatches[albumPath] = struct{}{}
						continue
					}
```

Replace with:
```go
					if strings.Contains(normalizeForFilename(album.Name()), titlePrefixLower) {
						dirMatches[albumPath] = struct{}{}
						continue
					}
```

**Occurrence 2** — file inside album dir (WalkDir callback):

Find:
```go
						if strings.Contains(strings.ToLower(filepath.Base(path)), titlePrefixLower) {
							dirMatches[albumPath] = struct{}{}
							return filepath.SkipDir
						}
```

Replace with:
```go
						if strings.Contains(normalizeForFilename(filepath.Base(path)), titlePrefixLower) {
							dirMatches[albumPath] = struct{}{}
							return filepath.SkipDir
						}
```

**Occurrence 3** — single audio file directly under author dir:

Find:
```go
					if strings.Contains(strings.ToLower(album.Name()), titlePrefixLower) {
						dirMatches[albumPath] = struct{}{}
					}
```

(This is in the `else` branch for non-directory album entries.)

Replace with:
```go
					if strings.Contains(normalizeForFilename(album.Name()), titlePrefixLower) {
						dirMatches[albumPath] = struct{}{}
					}
```

### Important — distinguish occurrence 3 from occurrence 1

Occurrence 1 is in the `if album.IsDir()` branch and has `continue` after it.
Occurrence 3 is in the `else` branch and has NO `continue`. Make sure you edit
the right one.

### 2c. Check for occurrence 4

The `disambiguate` closure (defined AFTER `findInITunes`) also does its own
filename normalization via:

```go
			stemNorm := strings.ReplaceAll(strings.ReplaceAll(stemLower, "_", " "), ":", " ")
```

Do NOT change `disambiguate`. It already handles underscores by normalizing both
sides to spaces. Leave it exactly as is.

---

## Step 3 — Verify imports

`normalizeForFilename` uses `strings` only. `strings` is already imported.
No new imports are needed. Confirm with:

```bash
grep -n '"strings"' internal/server/maintenance_fixups.go
```

---

## Step 4 — Tests

### 4a. Test `normalizeForFilename` directly

File: `internal/server/maintenance_fixups_test.go`

Add these test cases. If the file does not exist, create it with
`package server`:

```go
func TestNormalizeForFilename(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Mistborn: The Final Empire", "mistborn_ the final empire"},
		{"Mistborn:The Final Empire", "mistborn_the final empire"},
		{"The Name of the Wind", "the name of the wind"},
		{"SHOGUN", "shogun"},
		{"  Leading Space  ", "leading space"},
		{"A: B: C", "a_ b_ c"},
	}
	for _, tc := range cases {
		got := normalizeForFilename(tc.input)
		if got != tc.want {
			t.Errorf("normalizeForFilename(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
```

### 4b. Test that `findInITunes` matches colon-title to underscore-filename

Add this test. It builds a fake iTunes directory tree with the macOS underscore
encoding and asserts that the relink logic finds it:

```go
func TestFindInITunes_ColonTitleMatchesUnderscoreFilename(t *testing.T) {
	// Build fake iTunes root:
	//   <root>/Brandon Sanderson/Mistborn_ The Final Empire.m4b
	iTunesRoot := t.TempDir()
	authorDir := filepath.Join(iTunesRoot, "Brandon Sanderson")
	if err := os.MkdirAll(authorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fname := "Mistborn_ The Final Empire.m4b"
	if err := os.WriteFile(filepath.Join(authorDir, fname), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Call findInITunes (via exported shim if needed — see RELINK-2 task for
	// the extraction pattern). The book's title has a colon; the file has an
	// underscore.
	results := findInITunesForTest(iTunesRoot, "Brandon Sanderson", "Mistborn: The Final Empire")

	if len(results) == 0 {
		t.Fatalf("expected a match for colon-title vs underscore-filename, got none")
	}
	if !strings.Contains(results[0], "Mistborn_") && !strings.Contains(results[0], "Brandon Sanderson") {
		t.Errorf("unexpected match path: %v", results)
	}
}
```

Note: if the RELINK-2 task already extracted `findInITunesForTest`, reuse it
here. If RELINK-2 has not been done yet, extract the helper as described in that
task first, then write this test.

---

## Step 5 — Verify

```bash
go test ./internal/server/...
```

All existing tests must pass. Both new tests must pass.

---

## What NOT to Do

- Do NOT change `disambiguate`. It already normalizes underscores to spaces
  internally.
- Do NOT change the 25-character prefix truncation. The truncation happens
  BEFORE normalization in the updated code (that order is correct — truncate
  the raw title, then normalize). Do NOT swap the order.
- Do NOT call `strings.ToLower` again after `normalizeForFilename` — the helper
  already lowercases the input.
- Do NOT normalize the `authorWordLower` or `surnameLower` variables with this
  function. The author directory matching uses a different pattern (the `: `
  problem is specific to titles, not author names).
- Do NOT touch the iTunes XML parsing code. This fix is filesystem path
  comparison only.
- Do NOT use `strings.ReplaceAll(s, "_", " ")` on the title side. The
  correct direction is title `": "` → `"_ "` to match the filename encoding,
  not the reverse.

---

## PR Instructions

Branch: `fix/relink-3-title-normalization`

```bash
git checkout -b fix/relink-3-title-normalization
# make your changes
git add internal/server/maintenance_fixups.go \
        internal/server/maintenance_fixups_test.go
git commit -m "fix(relink): normalize colon to underscore in title comparison

macOS replaces ': ' with '_ ' in filenames (HFS+ restriction). Add
normalizeForFilename() and apply it in findInITunes so that a book titled
'Mistborn: The Final Empire' matches 'Mistborn_ The Final Empire.m4b'."
git push -u origin fix/relink-3-title-normalization
gh pr create \
  --title "fix(relink): normalize colon to underscore in iTunes title comparison" \
  --body "$(cat <<'EOF'
## Summary
- Adds \`normalizeForFilename(s string) string\` helper (lowercases, \`: \` → \`_ \`, \`:\` → \`_\`)
- Applies normalization to both the search title prefix and every filename/dirname candidate in \`findInITunes\`
- Leaves \`disambiguate\` unchanged (it already normalizes \`_\` → \` \` on both sides)

## Test plan
- [ ] \`go test ./internal/server/...\` passes
- [ ] \`TestNormalizeForFilename\` all cases pass
- [ ] \`TestFindInITunes_ColonTitleMatchesUnderscoreFilename\` passes
- [ ] Verified on production: relink "Mistborn: The Final Empire" finds correct iTunes file
EOF
)"
gh pr merge --rebase
```
