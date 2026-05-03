<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p2-01-decision-matrix-engine.md -->
<!-- version: 1.0.0 -->
<!-- guid: 93f10e8c-f268-4b55-b731-919aacce3c93 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Decision matrix: identity_score + per-pair match_score

**Pipeline phase:** P2
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P0-05` — must be merged before this task starts
- `task:P1-01` — must be merged before this task starts
- `task:P1-02` — must be merged before this task starts
- `task:P1-03` — must be merged before this task starts
- `task:P1-04` — must be merged before this task starts
- `task:P1-05` — must be merged before this task starts
- `task:P1-06` — must be merged before this task starts
- `task:P1-07` — must be merged before this task starts
- `task:P1-08` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P0-05" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P0-05"; exit 0; }
count=$(gh pr list --label "task:P1-01" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P1-01"; exit 0; }
count=$(gh pr list --label "task:P1-02" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P1-02"; exit 0; }
count=$(gh pr list --label "task:P1-03" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P1-03"; exit 0; }
count=$(gh pr list --label "task:P1-04" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P1-04"; exit 0; }
count=$(gh pr list --label "task:P1-05" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P1-05"; exit 0; }
count=$(gh pr list --label "task:P1-06" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P1-06"; exit 0; }
count=$(gh pr list --label "task:P1-07" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P1-07"; exit 0; }
count=$(gh pr list --label "task:P1-08" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P1-08"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p2-01-decision-matrix-engine
```

## Label

```bash
gh label create "task:PIPE-P2-01" --color "1d76db" --description "Bot task: Decision matrix: identity_score + per-pair match_score" 2>/dev/null || true
```

## What This Does

Implements the pure-functional decision matrix described in spec §4. Inputs are
a slice of `signals.Signal`; outputs are `IdentityResult` and a list of
`MatchPair` (book_a, book_b, kind, score).

## Files to Create / Edit

1. **Create** `internal/dedup/matrix/matrix.go`
2. **Create** `internal/dedup/matrix/matrix_test.go`

## Implementation outline

```go
package matrix

import "github.com/jdfalk/audiobook-organizer/internal/dedup/signals"

type IdentityResult struct {
    BookID         string
    IdentityScore  float64
    Contributions  []Contribution // for UI explanations
}
type Contribution struct {
    Kind       signals.Kind
    Score      float64
    Confidence float64
    Weight     float64
    Product    float64
}
type MatchPair struct {
    BookA, BookB string
    Kind         signals.Kind
    Score        float64
}

// ComputeIdentity returns the identity score for one book.
func ComputeIdentity(sigs []signals.Signal) IdentityResult { ... }

// ComputeMatchPairs returns the inferred duplicate pairs from a flat signal list.
func ComputeMatchPairs(sigs []signals.Signal) []MatchPair { ... }
```

Use a soft clamp `min(1.0, max(0.0, x))`. The match score uses **max** (not sum)
across signal kinds — see spec §4.2.

## Tests must cover

- SHA exact alone → identity 1.0
- Tag match alone → identity 0.225 (0.30·1.0·0.75)
- Whisper negative + tag match → identity below tag-match-only (negative weight applied)
- match_score uses max, not sum (two weak signals don't beat one strong)
- Bounds: every output is in [0,1]

## Definition of Done

- [ ] Pure function (no DB / IO)
- [ ] Property test: identity_score ∈ [0,1] for any random signal soup
- [ ] Soft-clamp behavior verified
- [ ] No dependency on `internal/server` or `internal/maintenance`


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): decision matrix: identity_score + per-pair match_score (PIPE-P2-01)" \
  --body "Implements PIPE-P2-01 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P2-01"
```
