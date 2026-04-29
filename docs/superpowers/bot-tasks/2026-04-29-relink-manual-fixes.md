<!-- file: docs/superpowers/bot-tasks/2026-04-29-relink-manual-fixes.md -->
<!-- version: 1.0.0 -->
<!-- guid: f1e2d3c4-b5a6-7890-abcd-ef1234567890 -->

# BOT TASK: Apply Manual iTunes Relink Fixes

**TODO ID:** RELINK-1
**Audience:** burndown bot
**Report that drives this task:** [`docs/reports/unresolved-relinks-2026-04-28.md`](../reports/unresolved-relinks-2026-04-28.md)

---

## What This Task Does

The iTunes relink maintenance job (PR #507) auto-resolved ~94.7% of broken
organizer-root book paths. A small number of books were left unresolved because
of co-author directory mismatches, colon-vs-underscore title prefix mismatches,
or series-prefix filenames. This task applies manual fixes for the ones where the
correct iTunes file was identified by hand.

**You must NOT invent or guess any file paths or book IDs.** Every value you use
must come from reading the report file listed above in its entirety before doing
anything else. If the report says a book has no iTunes file found, you must skip
it and make no changes for that book.

---

## Prerequisites

- PR #507 (`feat/broken-book-paths`) is merged. If it is not, stop and report that.
- You have access to the production API. The base URL and admin API key are in the
  repo `.env` file (`AUDIOBOOK_ORGANIZER_ADMIN_API_KEY` and `AUDIOBOOK_ORGANIZER_BASE_URL`).
  Read `.env` with the Read tool before making any API calls.
- You have `curl` available in the shell.

---

## Branch

```
fix/relink-1-manual-path-fixes
```

Create this branch from main. This task makes **no code changes** — it calls the
existing admin API and documents results. The only file changes are to this task
file (mark done) and CHANGELOG.md (add entry). Everything else happens via API
calls.

---

## Step 1 — Read the report

Read [`docs/reports/unresolved-relinks-2026-04-28.md`](../reports/unresolved-relinks-2026-04-28.md)
in full. For each numbered entry, extract:

- Book ID (the `01K...` ULID string after `**Book ID:**`)
- Whether an iTunes file was found (look for `✅` in the summary table at the bottom)
- The iTunes file path (the path after `**iTunes file:**`)

Do this extraction before proceeding. Do not proceed if the file does not exist.

---

## Step 2 — Read .env

Read the `.env` file at the repo root. Extract:

- `AUDIOBOOK_ORGANIZER_BASE_URL` — call it `$BASE_URL` in curl commands below
- `AUDIOBOOK_ORGANIZER_ADMIN_API_KEY` — call it `$API_KEY` in curl commands below

Do not print these values to your response text. Use them only in shell commands.

---

## Step 3 — For each book with a confirmed iTunes path: call the patch endpoint

For every entry in the report whose summary table row has `✅` (file found), do
the following. Do them **one at a time** — call the API, wait for the response,
check it, then proceed to the next. Do not batch them.

The endpoint to call is:

```
PATCH $BASE_URL/api/v1/audiobooks/{BOOK_ID}/files/primary-path
```

The request body is:

```json
{
  "new_path": "<exact iTunes file path from the report>"
}
```

Do not alter the path in any way. Copy it exactly as written in the report.

The curl command template is:

```bash
curl -s -X PATCH \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"new_path": "<path from report>"}' \
  "$BASE_URL/api/v1/audiobooks/<BOOK_ID>/files/primary-path"
```

**What to do with the response:**

- HTTP 200: success. Record `BOOK_ID: OK` in your working notes.
- HTTP 404: book not found in DB. Record `BOOK_ID: SKIP — book not found`.
- HTTP 409: path conflict. Record `BOOK_ID: SKIP — path conflict, needs investigation`.
- Any other error: record `BOOK_ID: ERROR — <status> <body>` and continue with
  the remaining books. Do not abort the whole task for one error.

**If the endpoint `primary-path` does not exist (returns 404 on the endpoint itself,
not the resource):** the endpoint shape may have changed. In that case, fall back to
the relink endpoint:

```bash
curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"book_ids": ["<BOOK_ID>"], "dry_run": false}' \
  "$BASE_URL/api/v1/maintenance/relink-missing-to-itunes"
```

Only use this fallback if the primary-path endpoint consistently returns 404 on
the endpoint URL for multiple books (not just the resource).

---

## Step 4 — For each book with no iTunes file found: run a shell search

For every entry in the report whose summary table row has `❌` (not found), SSH
into the production server and run the `find` command listed in that entry's
**Manual fix** section. Copy the raw shell output exactly into your working notes.
Do not modify any files or make any API calls for these books. Leave them for a
human to resolve.

The SSH address for the production server is in `memory/reference_server.md` —
read that file before SSHing.

---

## Step 5 — Verify each successful fix

For each BOOK_ID you recorded as `OK`, verify the update took effect:

```bash
curl -s \
  -H "Authorization: Bearer $API_KEY" \
  "$BASE_URL/api/v1/audiobooks/<BOOK_ID>"
```

Check that the `file_path` field in the response matches the iTunes path you sent.
If it does not match, record `BOOK_ID: VERIFY FAILED — path did not update`.

---

## Step 6 — Write results to a follow-up report

Create the file `docs/reports/relink-manual-fixes-result-2026-04-29.md` with
this exact structure. For the not-found section, create one sub-section per `❌`
entry using the book title from the report as the heading.

```markdown
# iTunes Relink Manual Fix Results — 2026-04-29

## Applied Fixes

| Book ID | Title | Result |
|---------|-------|--------|
| ... | ... | OK / SKIP / ERROR / VERIFY FAILED |

## Not-Found Books — Search Output

### <title from report, entry N>
<paste raw find output here>

(repeat for every ❌ entry in the report)

## Summary

- Fixed: N
- Skipped: N
- Errors: N
- Not in iTunes (manual resolution needed): N
```

Fill in every cell. Do not leave any cell blank. The titles in the Not-Found
section come from the report — copy them exactly as written.

---

## Step 7 — Update CHANGELOG.md

Prepend to the top of `CHANGELOG.md` (above the existing first entry, below the
header block if one exists):

```markdown
## [Unreleased]

### Fixed
- Applied manual iTunes path fixes for N books unresolved by the auto-relink
  endpoint (co-author dir mismatch, colon/underscore title prefix mismatch,
  series-prefix filenames). Results: `docs/reports/relink-manual-fixes-result-2026-04-29.md`
```

Replace `N` with the actual count of books successfully fixed.

---

## Step 8 — Commit and open PR

Stage only these files:

```bash
git add docs/reports/relink-manual-fixes-result-2026-04-29.md CHANGELOG.md
```

Commit message:

```
fix(relink): apply 13 manual iTunes path fixes (RELINK-1)

8 books had confirmed iTunes paths (colon/underscore mismatch, co-author
dir). 4 books not found in iTunes — documented in follow-up report.
1 book mapped via part-number scheme (Book 3 → Part 3).

Results: docs/reports/relink-manual-fixes-result-2026-04-29.md
```

Open a PR:

```bash
gh pr create \
  --title "fix(relink): apply manual iTunes path fixes (RELINK-1)" \
  --body "Applies the 13 manual path fixes identified in the unresolved-relinks
report. 8 books patched via admin API. 4 confirmed absent from iTunes —
documented for human review.

Results file: docs/reports/relink-manual-fixes-result-2026-04-29.md

Closes RELINK-1."
```

---

## What NOT to Do

- Do NOT modify any Go source files.
- Do NOT modify any migration files.
- Do NOT modify `docs/reports/unresolved-relinks-2026-04-28.md` (that is the input report, not the output).
- Do NOT guess any file path. Every path comes from the report.
- Do NOT apply fixes for books where the report says `❌` (not found in iTunes).
- Do NOT merge the PR yourself. Leave it for a human to review.

---

## Done

Mark RELINK-1 as `[x]` in TODO.md once the PR is open and the results file is written.
