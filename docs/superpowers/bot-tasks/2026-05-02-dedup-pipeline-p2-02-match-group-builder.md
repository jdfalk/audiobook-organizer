<!-- file: docs/superpowers/bot-tasks/2026-05-02-dedup-pipeline-p2-02-match-group-builder.md -->
<!-- version: 1.0.0 -->
<!-- guid: 6b62fed5-7492-4417-902e-2ec3d1217de9 -->
<!-- last-edited: 2026-05-02 -->

# BOT TASK: Build/update dedup_match_groups from MatchPair output

**Pipeline phase:** P2
**Audience:** burndown bot
**Master spec:** [`docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md`](../specs/2026-05-02-audiobook-identification-dedup-pipeline.md)

## Prerequisites

- `task:P0-06` — must be merged before this task starts
- `task:P2-01` — must be merged before this task starts

```bash
count=$(gh pr list --label "task:P0-06" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P0-06"; exit 0; }
count=$(gh pr list --label "task:P2-01" --state merged --json number | jq 'length')
[ "$count" -gt 0 ] || { echo "UNMET: task:P2-01"; exit 0; }
```


## Branch

```
feat/dedup-pipeline-p2-02-match-group-builder
```

## Label

```bash
gh label create "task:PIPE-P2-02" --color "1d76db" --description "Bot task: Build/update dedup_match_groups from MatchPair output" 2>/dev/null || true
```

## What This Does

Takes the `MatchPair` output of the matrix and incrementally maintains the
`dedup_match_groups` + `dedup_match_group_members` tables. Two pairs that
share a book go into the same group (transitive closure within a single
incoming batch).

## Files to Create / Edit

1. **Create** `internal/dedup/matrix/groups.go`
2. **Create** `internal/dedup/matrix/groups_test.go`

## Implementation outline

```go
type GroupWriter interface {
    UpsertGroup(g Group) error
    AddMember(groupID, bookID, role string, pairScore float64) error
    OpenGroupForBook(bookID string) (*Group, error)
}

func ApplyPairs(w GroupWriter, pairs []MatchPair) error { ... }
```

- For each pair: find an existing open group containing either book; if
  found, attach the other; otherwise create a new group with `canonical_book`
  = whichever book has the higher identity_score (tiebreak: earliest
  `created_at`).
- `strongest_kind` and `strongest_score` updated to the max across the group.

## Tests must cover

- New pair → new group with two members
- Pair sharing a book with an existing group → existing group grows
- Two existing groups linked by a new bridging pair → groups merge
- `merged`/`dismissed` groups are NEVER reopened by new pairs (state guarded)

## Definition of Done

- [ ] Idempotent: running the same pairs twice yields the same group set
- [ ] No physical deletes — group merging marks one as `state=merged` rather
      than deleting members


## PR Instructions

```bash
gh pr create \
  --title "feat(dedup): build/update dedup_match_groups from matchpair output (PIPE-P2-02)" \
  --body "Implements PIPE-P2-02 from docs/superpowers/specs/2026-05-02-audiobook-identification-dedup-pipeline.md. See task file for details." \
  --label "task:PIPE-P2-02"
```
