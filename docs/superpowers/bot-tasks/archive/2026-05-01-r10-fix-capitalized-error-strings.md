<!-- file: docs/superpowers/bot-tasks/2026-05-01-r10-fix-capitalized-error-strings.md -->
<!-- version: 1.0.0 -->
<!-- guid: 6f7a8b9c-0d1e-2f3a-4b5c-6d7e8f9a0b1c -->
<!-- last-edited: 2026-05-01 -->

# BOT TASK: R-10 — Fix capitalized error strings in metadata packages (ST1005)

**TODO ID:** R-10
**Audience:** burndown bot
**Branch:** `fix/r10-error-string-casing`
**PR title:** `fix(metadata): lowercase capitalized error strings (staticcheck ST1005)`

---

## What This Task Does

Fixes 12 staticcheck ST1005 warnings ("error strings should not be capitalized")
across 6 files in `internal/metadata/`:

| File | Lines | Count |
|------|-------|-------|
| `audible.go` | 150, 159, 180 | 3 |
| `audnexus.go` | 115, 185 | 2 |
| `googlebooks.go` | 115 | 1 |
| `hardcover.go` | 260, 275 | 2 |
| `openlibrary.go` | 171, 234, 308 | 3 |
| `wikipedia.go` | 127 | 1 |

The Go convention (per `errors` package docs and staticcheck ST1005) is that error
strings must start with a lowercase letter and not end with punctuation, so they
compose cleanly when wrapped: `fmt.Errorf("outer: %w", err)`.

---

## What NOT to Do

- **Do NOT** change any logic — only change the string literal first character to lowercase.
- **Do NOT** change error strings that start with a proper noun (e.g., `"OpenLibrary: ..."` → keep `OpenLibrary` capitalized is acceptable, but `"Failed to ..."` → `"failed to ..."` is the fix).
- **Do NOT** modify test files.

---

## Read First

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
staticcheck ./internal/metadata/... 2>&1 | grep ST1005
```

This lists the exact files and lines. Read each line in context before editing.

---

## Step-by-step

### Step 1 — Read each flagged line

For each file, read 3 lines of context:

```bash
sed -n '148,152p' internal/metadata/audible.go
sed -n '157,161p' internal/metadata/audible.go
sed -n '178,182p' internal/metadata/audible.go
sed -n '113,117p' internal/metadata/audnexus.go
sed -n '183,187p' internal/metadata/audnexus.go
sed -n '113,117p' internal/metadata/googlebooks.go
sed -n '258,262p' internal/metadata/hardcover.go
sed -n '273,277p' internal/metadata/hardcover.go
sed -n '169,173p' internal/metadata/openlibrary.go
sed -n '232,236p' internal/metadata/openlibrary.go
sed -n '306,310p' internal/metadata/openlibrary.go
sed -n '125,129p' internal/metadata/wikipedia.go
```

### Step 2 — Apply the fix

For each flagged error string, lowercase the first letter:

```go
// BEFORE (example):
return nil, fmt.Errorf("Failed to parse response: %w", err)
return nil, fmt.Errorf("No results found")
return nil, fmt.Errorf("Invalid status code: %d", code)

// AFTER:
return nil, fmt.Errorf("failed to parse response: %w", err)
return nil, fmt.Errorf("no results found")
return nil, fmt.Errorf("invalid status code: %d", code)
```

Do one file at a time and verify staticcheck after each file to avoid mistakes.

### Step 3 — Check staticcheck after each file

```bash
staticcheck ./internal/metadata/... 2>&1 | grep ST1005
```

After all edits, this should return zero lines.

### Step 4 — Build and test

```bash
go build ./internal/metadata/...
go test ./internal/metadata/... -timeout 60s 2>&1 | grep -E 'FAIL|ok|---'
```

### Step 5 — Bump version headers on all changed files

Increment patch version in each of the 6 changed files. Update last-edited date.

### Step 6 — Commit and open PR

```bash
git checkout -b fix/r10-error-string-casing
git add internal/metadata/
git commit -m "fix(metadata): lowercase capitalized error strings (ST1005)

Fixes 12 staticcheck ST1005 warnings across audible.go, audnexus.go,
googlebooks.go, hardcover.go, openlibrary.go, and wikipedia.go.
Error strings must start with a lowercase letter per Go convention.
Re-audit R-10.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin fix/r10-error-string-casing
gh pr create \
  --title "fix(metadata): lowercase capitalized error strings (ST1005)" \
  --body "Fixes 12 ST1005 staticcheck warnings across 6 metadata files. Lowercase first letter of error strings. Re-audit R-10."
```

---

## Checklist

- [ ] `staticcheck ./internal/metadata/...` shows zero ST1005 warnings
- [ ] `go build ./internal/metadata/...` clean
- [ ] `go test ./internal/metadata/...` passes
- [ ] Version headers bumped on all 6 changed files
- [ ] PR opened with correct branch and title
