<!-- file: docs/superpowers/bot-tasks/2026-04-29-duration-mismatch-chip.md -->
<!-- version: 1.0.0 -->
<!-- guid: c2a85e4f-17d3-4b8c-a9e3-6d4f93c70b25 -->
<!-- last-edited: 2026-04-29 -->

# Bot Task: RATE-5 / DUR-1 — Duration Mismatch Warning Chip in MetadataReviewDialog

## Task ID
RATE-5 / DUR-1

## Summary
Add a yellow warning chip to `MetadataReviewDialog` when a metadata candidate's runtime differs from the book's known runtime by more than 10 minutes. Also show the candidate's own duration in the two-column card view. This is a **frontend-only** change.

## DO NOT DO THESE THINGS
- **DO NOT** modify any Go/backend file.
- **DO NOT** create any new files. Only edit the one file listed below.
- **DO NOT** change the API or data fetching logic.
- **DO NOT** remove any existing chips, rows, or UI elements.

---

## Background / Context

PR #520 added `duration_sec` and `duration_delta_sec` to `MetadataCandidate`. These fields are already returned by `GET /api/v1/audiobooks/:id/metadata-candidates` and are available on the `CandidateResult` / `r.candidate` object inside `MetadataReviewDialog`.

- `r.candidate.duration_sec`: the candidate source's total duration in seconds (0 if unknown)
- `r.candidate.duration_delta_sec`: difference between candidate duration and the local book's duration, in seconds. Positive means candidate is longer, negative means shorter. Magnitude is what matters for the warning.

---

## The ONE File You Edit

```
web/src/components/audiobooks/MetadataReviewDialog.tsx
```

---

## Step 1 — Bump the version header

At the very top of the file there is a comment block like:

```tsx
// file: web/src/components/audiobooks/MetadataReviewDialog.tsx
// version: X.Y.Z
```

Change the version to the next patch version (e.g. `1.4.2` → `1.4.3`). If the version comment is missing, add it as the first two lines of the file.

---

## Step 2 — Add the duration formatting helper

Search the file for any existing time/duration formatting helpers (grep for `formatDuration\|formatSeconds\|toHoursMinutes`). If one exists that converts seconds to "Xh Ym" format, reuse it. If not, add this helper near the top of the component file, outside the component function, after the imports:

```ts
/** Converts a duration in seconds to a compact "Xh Ym" string. */
function formatDurationSec(totalSec: number): string {
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  if (h === 0) return `${m}m`;
  if (m === 0) return `${h}h`;
  return `${h}h ${m}m`;
}
```

---

## Step 3 — Add the duration mismatch chip in `renderCompactRow`

### Find the function

Search the file for `renderCompactRow`. The function signature will look something like:

```tsx
const renderCompactRow = (r: CandidateRowData, ...) => {
```

or it may be an inner function. Find it.

### Find where chips are rendered

Inside `renderCompactRow`, look for a `<Box>` or `<Stack>` that contains `<Chip` elements. There will be chips for things like the source name, match score, ISBN, etc. You need to add the duration-mismatch chip **after** all existing chips in that group.

### Add the chip

After the existing chips block, add exactly this code:

```tsx
{Math.abs(r.candidate?.duration_delta_sec ?? 0) > 600 && (
  <Chip
    label={`⚠ runtime differs by ${formatDurationSec(Math.abs(r.candidate.duration_delta_sec))}`}
    color="warning"
    size="small"
    sx={{ fontWeight: 500 }}
  />
)}
```

Explanation:
- `600` seconds = 10 minutes threshold
- `Math.abs` because the delta can be negative (candidate shorter) or positive (candidate longer)
- `color="warning"` renders as yellow in MUI
- `size="small"` keeps it compact

If `r.candidate` might be undefined, use optional chaining throughout: `r.candidate?.duration_delta_sec`.

---

## Step 4 — Show candidate duration in `renderTwoColumnCard`

### Find the function

Search the file for `renderTwoColumnCard`. The function renders the expanded/two-column view of a candidate card.

### Find where candidate metadata rows are shown

Inside `renderTwoColumnCard`, there will be a section that renders field rows — e.g., author, narrator, publisher, runtime. Look for any existing row that shows `runtime` or `duration`. It may look like:

```tsx
{candidate.runtime && <DetailRow label="Runtime" value={candidate.runtime} />}
```

### Add or modify the duration row

If there is already a runtime row, modify it to also show the computed duration. If there is no runtime row, add one after the last metadata field row:

```tsx
{(r.candidate?.duration_sec ?? 0) > 0 && (
  <DetailRow
    label="Duration"
    value={formatDurationSec(r.candidate!.duration_sec)}
  />
)}
```

(`DetailRow` may have a different name in the file — grep for `label="Runtime"\|label="Duration"\|DetailRow\|MetaRow` and use whatever component is already used for metadata rows in this view.)

If no row component exists and duration is shown inline, use the same pattern as the nearest existing field (e.g., a `<Typography>` or a `<Box>` with a label span and a value span).

---

## Step 5 — TypeScript types

Confirm that the `CandidateResult` or equivalent type (grep for `interface CandidateResult\|type CandidateResult\|duration_sec`) already has `duration_sec` and `duration_delta_sec` as optional number fields. They should have been added in PR #520.

If they are missing from the TypeScript type, add them:

```ts
duration_sec?: number;
duration_delta_sec?: number;
```

Add them inside the relevant interface, in alphabetical order among the other `d` fields, or at the end of the interface block.

---

## Step 6 — Verify

Run the TypeScript compiler:

```bash
cd web && npx tsc --noEmit
```

Must produce zero errors.

Run the frontend tests:

```bash
cd web && npm test -- --watchAll=false
```

Must produce zero failures.

Visually verify (if possible):

1. Open the MetadataReviewDialog for a book.
2. Find a candidate where the duration differs from the book by more than 10 minutes.
3. Confirm a yellow "⚠ runtime differs by Xh Ym" chip appears in the compact row.
4. Expand that candidate's two-column card.
5. Confirm a "Duration: Xh Ym" row appears.

---

## Checklist

- [ ] File version header bumped in `MetadataReviewDialog.tsx`
- [ ] `formatDurationSec` helper added (or existing helper reused)
- [ ] Duration-mismatch chip added in `renderCompactRow`
- [ ] Chip only appears when `|duration_delta_sec| > 600`
- [ ] Chip uses `color="warning"` and `size="small"`
- [ ] Candidate duration row added in `renderTwoColumnCard`
- [ ] Row only appears when `duration_sec > 0`
- [ ] `duration_sec` and `duration_delta_sec` present in TypeScript type
- [ ] `npx tsc --noEmit` passes
- [ ] `npm test` passes

---

## PR Instructions

1. Branch name: `feat/duration-mismatch-chip`
2. Commit message: `feat(ui): show duration mismatch warning chip in MetadataReviewDialog (DUR-1)`
3. PR title: `feat(ui): duration mismatch warning chip in candidate cards (DUR-1)`
4. PR body: mention that this is frontend-only, no backend changes, links to PR #520 for context.
5. Do NOT include any backend or Go file changes in this PR.
