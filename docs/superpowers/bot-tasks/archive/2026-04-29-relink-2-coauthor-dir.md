<!-- file: docs/superpowers/bot-tasks/2026-04-29-relink-2-coauthor-dir.md -->
<!-- version: 1.0.0 -->
<!-- guid: 1e9a4c7f-3d2b-4e8a-c6f0-5b1d7a3e9c2f -->
<!-- last-edited: 2026-04-29 -->

# Bot Task: RELINK-2 — Co-author Directory Matching

> **Read every word.** Do exactly what is written. Do not guess, do not
> improvise, do not add extra changes. If something is unclear, STOP and ask.

---

## Goal

Fix a bug in `findInITunes` where a book whose DB author is `"Robert Jordan"`
fails to match an iTunes directory named `"Robert Jordan, Brandon Sanderson"`.
The current code only searches for directories containing the first significant
word of the author name (e.g. `"Robert"`). This finds too many false positives
and misses co-author combos. The fix adds a second-pass fallback that uses the
author's surname to match directories that include the primary author alongside
co-authors.

---

## File You Will Touch

`internal/server/maintenance_fixups.go`

Do NOT touch any other file until step 4 (tests).

---

## Step 0 — Find the function

Before making any edits, run:

```bash
grep -n "findInITunes" internal/server/maintenance_fixups.go
```

Confirm the function is a closure assigned around line 4154. If the line numbers
have shifted, find the correct location by searching for the comment:

```
// findInITunes searches iTunesRoot for iTunes album directories (or single
```

All edits below are relative to the function body, not absolute line numbers.

---

## Step 1 — Understand the current code

The function `findInITunes` (a closure inside the relink handler) currently does
the following:

1. Computes `authorWordLower` = first space-delimited word of `authorName`
   (e.g. `"robert"` from `"Robert Jordan"`).
2. Scans top-level entries in `iTunesRoot`.
3. Skips any entry whose name does not contain `authorWordLower`.
4. For matching author dirs, searches album subdirectories by `titlePrefixLower`.

The bug: `authorWordLower = "robert"` does NOT match a directory named
`"Robert Jordan, Brandon Sanderson"` if that directory is structured differently,
AND even when it does match by first name, the current code has no fallback when
zero M4B/audio files are found via the primary search and the directory structure
uses the co-author pattern.

The fix adds a **surname fallback pass** after the primary pass. It is activated
only when the primary pass returns zero matches.

---

## Step 2 — Add the surname fallback

Find the exact block at the end of `findInITunes` that assembles and returns the
result:

```go
		result := make([]string, 0, len(dirMatches))
		for d := range dirMatches {
			result = append(result, d)
		}
		sort.Strings(result)
		return result
	}
```

Replace it with:

```go
		// Primary pass produced matches — return them directly.
		if len(dirMatches) > 0 {
			result := make([]string, 0, len(dirMatches))
			for d := range dirMatches {
				result = append(result, d)
			}
			sort.Strings(result)
			return result
		}

		// Surname fallback: when the primary (first-word) pass finds nothing,
		// try matching iTunes directories by the author's surname — the last
		// space-delimited word of the primary author name. This handles
		// co-author directories like "Robert Jordan, Brandon Sanderson".
		surname := authorName
		if idx := strings.LastIndex(authorName, " "); idx > 0 {
			surname = authorName[idx+1:]
		}
		surnameLower := strings.ToLower(surname)

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if !strings.Contains(strings.ToLower(entry.Name()), surnameLower) {
				continue
			}
			authorDir := filepath.Join(iTunesRoot, entry.Name())

			albumEntries, err := os.ReadDir(authorDir)
			if err != nil {
				continue
			}
			for _, album := range albumEntries {
				albumPath := filepath.Join(authorDir, album.Name())
				if album.IsDir() {
					if strings.Contains(strings.ToLower(album.Name()), titlePrefixLower) {
						dirMatches[albumPath] = struct{}{}
						continue
					}
					_ = filepath.WalkDir(albumPath, func(path string, d os.DirEntry, err error) error {
						if err != nil || d.IsDir() {
							return nil
						}
						if !audioExts[strings.ToLower(filepath.Ext(path))] {
							return nil
						}
						if strings.Contains(strings.ToLower(filepath.Base(path)), titlePrefixLower) {
							dirMatches[albumPath] = struct{}{}
							return filepath.SkipDir
						}
						return nil
					})
				} else {
					if !audioExts[strings.ToLower(filepath.Ext(albumPath))] {
						continue
					}
					if strings.Contains(strings.ToLower(album.Name()), titlePrefixLower) {
						dirMatches[albumPath] = struct{}{}
					}
				}
			}
		}

		result := make([]string, 0, len(dirMatches))
		for d := range dirMatches {
			result = append(result, d)
		}
		sort.Strings(result)
		return result
	}
```

### Important notes about this replacement

- The variable `entries` is already declared earlier in the function (the
  `os.ReadDir(iTunesRoot)` call). The fallback reuses it — do NOT add a second
  `os.ReadDir` call.
- The `dirMatches` map is also already declared. The fallback writes into the
  same map — this is intentional. If the primary pass found some matches, we
  returned early. If we reach the fallback, the map is empty.
- `audioExts` is declared in the outer handler function and is in scope for the
  closure — do NOT redefine it.
- Do NOT change anything ABOVE the result-assembly block you replaced.

---

## Step 3 — Verify no new imports are needed

The fallback uses only `strings`, `os`, `filepath`, and `sort` — all of which
are already imported in this file. Run:

```bash
grep -n '"strings"\|"os"\|"path/filepath"\|"sort"' internal/server/maintenance_fixups.go | head -10
```

Confirm all four are present. If any is missing, add it to the import block.

---

## Step 4 — Tests

File: `internal/server/maintenance_fixups_test.go`

If this file does not exist, create it with `package server`.

Add the following test. It builds a fake iTunes directory tree on disk using
`t.TempDir()` and calls the relink handler's internal logic through an
exported helper or by testing the HTTP handler end-to-end with a test store.

Because `findInITunes` is a closure inside the HTTP handler, you cannot call it
directly. Instead, write a small black-box test that exercises the relink
endpoint with a temporary iTunes root set via the config.

If an existing test harness for the relink handler already exists, add the case
there. Otherwise, add this unit-level reproduction:

```go
func TestFindInITunes_CoauthorDirectory(t *testing.T) {
	// Build a fake iTunes root:
	//   <root>/Robert Jordan, Brandon Sanderson/The Wheel of Time/book.m4b
	iTunesRoot := t.TempDir()
	coauthorDir := filepath.Join(iTunesRoot, "Robert Jordan, Brandon Sanderson")
	albumDir := filepath.Join(coauthorDir, "The Wheel of Time")
	if err := os.MkdirAll(albumDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "The Wheel of Time.m4b"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Invoke findInITunes via the exported helper (if one is added) or via
	// a test-only shim. See comment below.
	//
	// If no exported helper exists, add this to maintenance_fixups.go for tests:
	//
	//   func findInITunesForTest(iTunesRoot, authorName, title string) []string { ... }
	//
	// Then call it here:
	results := findInITunesForTest(iTunesRoot, "Robert Jordan", "The Wheel of Time")

	if len(results) == 0 {
		t.Fatal("expected at least one match for co-author directory, got none")
	}
	if !strings.Contains(results[0], "Robert Jordan, Brandon Sanderson") {
		t.Errorf("expected match inside co-author dir, got: %v", results)
	}
}
```

**To make the test compile,** add this exported test helper at the bottom of
`internal/server/maintenance_fixups.go` (guarded by a build tag so it does not
ship in production):

```go
// findInITunesForTest is a test-only shim that exposes the findInITunes
// closure logic as a callable function. Only compiled during tests.
//
//go:build ignore
```

Actually, the cleanest approach is to extract `findInITunes` into a
package-level function (unexported is fine, tests in the same package can call
it). Do this extraction ONLY if the test cannot otherwise be written. If the
existing test setup already has an integration harness, use that instead and
skip the extraction.

---

## Step 5 — Verify

```bash
go test ./internal/server/...
```

All existing tests must pass. The new co-author test must pass.

---

## What NOT to Do

- Do NOT replace the primary (first-word) search. The fallback only runs when
  the primary search returns zero results.
- Do NOT add a third `os.ReadDir(iTunesRoot)` call. The fallback reuses the
  `entries` slice already read at the top of the function.
- Do NOT change the `disambiguate` function or anything after `findInITunes`.
- Do NOT change the title-matching logic (the `titlePrefixLower` comparison).
  Only the author directory matching gets the fallback.
- Do NOT change the `sort.Strings(result)` behavior — results must remain
  sorted for deterministic disambiguation.

---

## PR Instructions

Branch: `fix/relink-2-coauthor-dir-matching`

```bash
git checkout -b fix/relink-2-coauthor-dir-matching
# make your changes
git add internal/server/maintenance_fixups.go \
        internal/server/maintenance_fixups_test.go
git commit -m "fix(relink): surname fallback for co-author iTunes directories

When findInITunes finds no matches by first-word prefix, fall back to
matching by the author's surname. This allows \"Robert Jordan\" to match
the iTunes directory \"Robert Jordan, Brandon Sanderson\"."
git push -u origin fix/relink-2-coauthor-dir-matching
gh pr create \
  --title "fix(relink): surname fallback for co-author iTunes directories" \
  --body "$(cat <<'EOF'
## Summary
- Adds a surname-based fallback pass to \`findInITunes\`
- Fallback only activates when the primary (first-word) search returns 0 matches
- Reuses the already-read \`entries\` slice — no extra filesystem I/O
- Covers the \"Robert Jordan\" → \"Robert Jordan, Brandon Sanderson\" case

## Test plan
- [ ] \`go test ./internal/server/...\` passes
- [ ] Co-author directory test passes
- [ ] Verified against production: relink a co-authored Wheel of Time book
EOF
)"
gh pr merge --rebase
```
