<!-- file: docs/superpowers/bot-tasks/2026-04-29-activity-act3-isbn-batch-noun.md -->
<!-- version: 1.0.0 -->
<!-- guid: 2f8b4c6e-1a3d-5079-e7f1-9d5b0c2a4e83 -->
<!-- last-edited: 2026-04-29 -->

# BOT TASK: ACT-3 — Fix isbn-enrich Batch Noun Label

**TODO ID:** ACT-3
**Audience:** burndown bot
**Branch:** `fix/activity-act3-isbn-batch-noun`
**PR title:** `fix(activity): add isbn-enrich batch noun label`

---

## What This Task Does

This is a **one-line change**. The `batchNoun` function in
`internal/activity/batcher.go` has a `case "isbn-enrich":` that currently returns
`"ISBN enrichments"`. The correct return value should be `"books enriched with ISBN"`
to match the UI label used by the ISBN enrichment operation.

---

## What NOT to Do

- **Do NOT modify any other code** in `batcher.go` or anywhere else.
- **Do NOT delete any existing case** in the switch statement.
- **Do NOT add new cases** other than the one isbn-enrich fix.
- **Do NOT modify the case key string** — it must remain exactly `"isbn-enrich"`
  (with a hyphen, not an underscore).
- This is a one-line change. If you find yourself editing more than one line, stop
  and re-read this file.

---

## Step 1 — Run Grep to Find the Exact Location

Run this command and read the output carefully:

```bash
grep -n "batchNoun\|isbn-enrich" \
  /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer/internal/activity/batcher.go
```

Expected output:
```
174:	noun := batchNoun(key.Type)
200:// batchNoun returns a human-readable plural noun for a batch type.
201:func batchNoun(batchType string) string {
211:	case "isbn-enrich":
212:		return "ISBN enrichments"
```

The line you need to change is line 212 (the `return` inside `case "isbn-enrich":`).
The exact line number may differ slightly — use the grep output to find it.

---

## Step 2 — Make the Change

Open `internal/activity/batcher.go`.

Find this exact text (two lines):

```go
	case "isbn-enrich":
		return "ISBN enrichments"
```

Replace it with:

```go
	case "isbn-enrich":
		return "books enriched with ISBN"
```

That is the entire change. One line edited. Nothing else.

---

## Step 3 — Confirm the Full Switch Statement Looks Correct

After your edit, the `batchNoun` function should look exactly like this:

```go
// batchNoun returns a human-readable plural noun for a batch type.
func batchNoun(batchType string) string {
	switch batchType {
	case "embedded-tag-load":
		return "embedded tag loads"
	case "tag-scan":
		return "tag scans"
	case "metadata-apply":
		return "metadata applies"
	case "path-repair":
		return "path repairs"
	case "isbn-enrich":
		return "books enriched with ISBN"
	case "temp-file-cleanup":
		return "orphaned temp files removed"
	case "missing-file-repair":
		return "missing files repaired"
	case "purge-deleted":
		return "purge errors"
	default:
		return batchType + " operations"
	}
}
```

If it does not look like this, you made an error. Undo and redo.

---

## Step 4 — Bump the Version Header

Open `internal/activity/batcher.go`. The first three lines look like:

```
// file: internal/activity/batcher.go
// version: 1.1.0
// guid: 7f3c1a2e-8b4d-4e9f-a5c6-2d0e3b7f9a1c
```

Increment the patch version: change `1.1.0` → `1.1.1`.

---

## Step 5 — Verify Tests Pass

```bash
cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer
go test ./...
```

All tests must pass. This change cannot break any test. If a test fails, it is
unrelated to your change — read the failure carefully before touching anything else.

---

## Step 6 — Commit and Open PR

```bash
git checkout -b fix/activity-act3-isbn-batch-noun
git add internal/activity/batcher.go
git commit -m "fix(activity): add isbn-enrich batch noun label"
git push -u origin fix/activity-act3-isbn-batch-noun
gh pr create \
  --title "fix(activity): add isbn-enrich batch noun label" \
  --body "One-line fix: changes batchNoun(\"isbn-enrich\") to return \"books enriched with ISBN\" instead of \"ISBN enrichments\", matching the UI label for the ISBN enrichment operation."
```

---

## Checklist

- [ ] Grep command run and line number confirmed before editing
- [ ] Exactly one line changed in `internal/activity/batcher.go`
- [ ] `case "isbn-enrich": return "books enriched with ISBN"` is the only change
- [ ] Version header in `batcher.go` bumped to `1.1.1`
- [ ] `go test ./...` passes
- [ ] PR opened with correct branch and title
