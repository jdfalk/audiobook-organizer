# Multi-Pass AI Author Dedup Pipeline — Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:writing-plans to create the implementation plan.

**Goal:** Replace the current groups/full/combined AI review modes with a single automated multi-pass pipeline that runs two models in parallel, enriches uncertain results with book data, cross-validates via logic tree, and presents agreed results for user approval.

**Date:** 2026-03-08

---

## 1. Pipeline Architecture

The AI Author Dedup is a single automated pipeline with 4 phases:

```
Phase 1: Parallel Scan
├── 1a: Groups scan (gpt-5-mini) — heuristic groups from Jaro-Winkler ≥0.90
└── 1b: Full scan (o4-mini) — all authors chunked (~500/chunk), finds misclassified/junk/compound entries

Phase 2: Enrich (starts as soon as EITHER scan completes)
├── Take medium/low confidence results from whichever finished
├── Fetch book titles for involved authors from DB
└── Resubmit enriched prompt to same model (batch or realtime)

Phase 3: Cross-validate (starts when BOTH scans + enrichments complete)
├── Logic tree compares groups results vs full results
├── Agreement = high confidence
├── Disagreement = flagged for manual review
└── Found-by-one-only = medium confidence

Phase 4: Present results
├── Agreed suggestions at top
├── Disagreements and one-sided findings below
└── User reviews and applies
```

**Key behaviors:**
- **Async throughout** — each phase triggers the next automatically
- **No waiting** — if groups finishes first, enrichment starts immediately while full is still running
- **Batch or real-time** — user toggle. Batch = 50% cheaper, hours. Real-time = seconds.
- **Everything persisted** — each scan saved to dedicated DB with all raw I/O

**Why two scans with different roles:**
- **Groups scan** finds name-variant duplicates (pairs pre-clustered by Jaro-Winkler across entire dataset)
- **Full scan** finds structural issues within each entry (publishers-as-authors, narrators-as-authors, compound entries, junk, initials formatting) — no cross-chunk comparison needed

## 2. Data Model

### Scan Record
```
Scan
├── id (auto-increment)
├── status: pending → scanning → enriching → cross_validating → complete → failed
├── mode: "batch" | "realtime"
├── created_at, completed_at
├── models: {groups: "gpt-5-mini", full: "o4-mini"}
├── author_count (snapshot at scan time)
```

### Phase Records
```
Phase (groups_scan | full_scan | groups_enrich | full_enrich | cross_validate)
├── scan_id
├── phase_type
├── status: pending → submitted → processing → complete → failed
├── batch_id (OpenAI batch ID, null for realtime)
├── input_data (raw prompt JSON)
├── output_data (raw response JSON)
├── model
├── started_at, completed_at
├── suggestions[] (parsed results)
```

### Scan Results (post cross-validation)
```
ScanResult
├── scan_id
├── suggestion JSON (action, canonical_name, confidence, roles...)
├── agreement: "agreed" | "groups_only" | "full_only" | "disagreed"
├── applied: bool
├── applied_at
```

### Storage

Separate PebbleDB alongside the main database:
```
<library_path>/
├── audiobooks.db          (main PebbleDB)
├── openlibrary.db         (OpenLibrary cache)
└── ai_scans.db            (scan history + raw I/O)
```

Key schema:
```
scan:<id>                           → Scan JSON
scan_phase:<scan_id>:<phase_type>   → Phase JSON
scan_result:<scan_id>:<index>       → ScanResult JSON
counter:scan                        → next scan ID
```

Operations log gets a one-liner entry linking to the scan ID (no data duplication).

## 3. Author Tombstones

When authors are merged, the deleted variant IDs become **tombstones** — permanent redirects to the canonical author.

```
author_tombstone:<old_id> → canonical_id
```

**Behavior:**
- On merge: instead of just deleting variant, also write a tombstone pointing to the canonical
- `GetAuthorByID(oldID)` follows the redirect transparently
- Old scan results stay valid — IDs in suggestions still resolve
- No stale/dismiss logic needed for partially-applied results

**Chain resolution:**
- A daily/periodic maintenance task finds tombstone chains (A→B→C) and collapses them (A→C, B→C)
- On write, always point to the final canonical (not intermediate)
- Registered as a scheduler task alongside existing periodic jobs

## 4. Background Processing

Uses the existing scheduler (`internal/server/scheduler.go`):

1. **On scan start**: Creates Scan record, submits Phase 1a + 1b in parallel, writes operation log "AI scan #N started"
2. **Batch polling**: New scheduler task checks OpenAI batch status every 5 minutes for in-progress scans. Downloads results on completion, triggers next phase.
3. **Phase transitions**: Each phase completion checks what can start next:
   - Groups scan done → start groups enrichment
   - Full scan done → start full enrichment
   - Both enrichments done → run cross-validation (local logic, instant)
   - Cross-validation done → mark scan complete, write operation log
4. **Real-time mode**: Same pipeline, phases run via direct API calls in background goroutines. UI polls for status.

New file: `internal/server/ai_scan_pipeline.go`

## 5. Cross-Validation Logic Tree

Local logic, instant, free. No AI call needed.

```
For each suggestion from groups scan:
  Match against full scan by overlapping author IDs (fallback: canonical name)

  If match found:
    Same action + same canonical       → "agreed" (inherit higher confidence)
    Same action, different canonical   → "agreed" (use groups' canonical, note diff)
    Different actions                  → "disagreed" (present both, user decides)

  If no match:
    → "groups_only" (keep original confidence)

For each unmatched full scan suggestion:
    → "full_only" (keep original confidence)
```

Confidence rules:
- Agreed suggestions inherit the higher confidence of the two sources
- Enrichment upgrades (medium→high from book evidence) carry through
- No downgrading — if one model is confident, that stands

## 6. Frontend UI

```
AI Tab
├── Authors
│   ├── Header bar
│   │   ├── "Run Scan" button
│   │   ├── Batch / Realtime toggle
│   │   └── "Scan History" button → opens sidebar
│   │
│   ├── Active scan status (when running)
│   │   └── Phase progress: ✓ Groups  ✓ Full  → Enriching...  ○ Cross-validate
│   │
│   ├── Results (when complete)
│   │   ├── Filter tabs: Agreed | Groups Only | Full Only | Disagreed
│   │   ├── Confidence filter (high / medium / low)
│   │   ├── Action filter (merge / split / rename / alias / reclassify)
│   │   ├── Suggestion cards with role decomposition (existing component)
│   │   └── Multi-select + "Apply Selected" button
│   │
│   └── Scan History sidebar
│       ├── List: date, author count, suggestion count, status
│       ├── Click to load scan results into main view
│       └── "Compare" checkbox — select two scans to diff
│           └── Shows: new (in B not A), resolved (in A not B), unchanged
│
└── Books (placeholder)
    └── "Coming soon"
```

Changes from current UI:
- **Remove**: groups / full / combined sub-tabs and mode selector
- **Add**: scan status progress, scan history sidebar, batch toggle
- **Keep**: suggestion cards, filters, apply flow (already work well)

## 7. API Endpoints

```
POST   /api/v1/ai/scans                    Start new scan (body: {mode: "batch"|"realtime"})
GET    /api/v1/ai/scans                    List all scans
GET    /api/v1/ai/scans/:id                Get scan with phases and status
GET    /api/v1/ai/scans/:id/results        Get cross-validated results
POST   /api/v1/ai/scans/:id/apply          Apply selected suggestions (body: {result_ids: [...]})
DELETE /api/v1/ai/scans/:id                Delete scan and its data

GET    /api/v1/ai/scans/compare?a=1&b=2    Compare two scans
```

Existing endpoints for author merge/rename/split/reclassify/alias remain unchanged.

## 8. Cost Comparison

| Approach | Models | Batch Discount | Estimated Cost (5000 authors) |
|----------|--------|---------------|-------------------------------|
| Current (single gpt-5-mini call) | 1 model, 1 pass | None | ~$0.50 |
| New pipeline, realtime | 2 models, 2-3 passes | None | ~$2.00 |
| New pipeline, batch | 2 models, 2-3 passes | 50% off | ~$1.00 |

The batch pipeline costs roughly 2× the current single-call approach but delivers much higher accuracy through cross-validation and enrichment.
