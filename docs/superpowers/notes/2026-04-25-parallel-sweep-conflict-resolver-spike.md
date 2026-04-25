<!-- file: docs/superpowers/notes/2026-04-25-parallel-sweep-conflict-resolver-spike.md -->
<!-- version: 1.0.0 -->
<!-- guid: 0a1b2c3d-4e5f-6a7b-8c9d-0e1f2a3b4c5d -->
<!-- last-edited: 2026-04-25 -->

# Spike — does the Sonnet conflict resolver actually resolve a non-trivial conflict?

**Date:** 2026-04-25
**Step:** /parallel-sweep build, step 6
**Plan reference:** [`docs/superpowers/plans/2026-04-24-parallel-sweep-slash-command.md`](../plans/2026-04-24-parallel-sweep-slash-command.md) §7 (sibling rebase + conflict resolution), §13 Q4 (Sonnet trivial / Opus fallback)

## Question

Step 6 ships `scripts/conflict_resolver.py` and `references/conflict-resolver-prompt.md`. Unit tests prove the helpers correctly inspect, count, and post-process conflicts — but the only way to know whether the Sonnet model actually does the resolution well, given this prompt, on a real conflict, is to dispatch one and check the output.

## Method

1. **Built a deliberate conflict.** Created `/tmp/parallel-sweep-conflict-spike` as a fresh git repo with a tiny Go-like `calc.go` containing `Add(a, b int) int { return a + b }`.
   - **main** branch: renamed `Add` → `Sum` and updated the doc comment.
   - **feat/overflow-check** branch (off the same base): kept the `Add` name but added `ErrOverflow`, changed the signature to `func Add(a, b int) (int, error)`, and inserted overflow-check logic.
   - `git rebase main` from `feat/overflow-check` produced a 1-marker conflict in `calc.go`.

2. **Verified the conflict assessment.** `conflict_resolver.assess_conflict` reported `files=['calc.go']`, `marker_count=1`, `is_trivial=True`. (Well within `TRIVIAL_FILE_THRESHOLD=3`, `TRIVIAL_MARKER_THRESHOLD=30`.)

3. **Built the resolver prompt** via `conflict_resolver.build_resolver_prompt` from the template at `references/conflict-resolver-prompt.md`. Hit and fixed one bug along the way: my initial template extractor used `text.find("\n```")` for the closing fence, which truncated the prompt at the first nested ``` block (the report-format example). Switched to `text.rfind("\n```")` to find the outermost closing fence. Test `BuildResolverPromptTests.test_substitutes_all_placeholders` covers this regression.

4. **Dispatched a `general-purpose` sub-agent** with the full filled-in prompt + a one-paragraph oracle hint at the end describing what the right merged intent should be ("keep main's rename AND keep the branch's overflow check"). The hint was included to make this a *capability* test (does the model follow the prompt cleanly when it knows what success looks like?) rather than an *intent inference* test (which is harder to evaluate from one example).

5. **Observed the agent's behavior**, then ran `apply_resolver_success` on the worktree.

## Result

**Resolver succeeded.** The agent's report:

```
RESOLVED_FILES:
- /private/tmp/parallel-sweep-conflict-spike/calc.go: kept main's Add→Sum rename
  and applied branch's overflow check; signature is now func Sum(a, b int) (int, error)
  with ErrOverflow

UNRESOLVED_FILES:
- none

EXIT_REASON: success
```

The merged file:

```go
package calc

import "errors"

// ErrOverflow indicates a Sum overflow.
var ErrOverflow = errors.New("integer overflow")

// Sum returns the total of two integers, or ErrOverflow.
func Sum(a, b int) (int, error) {
    if (b > 0 && a > 1<<62) || (b < 0 && a < -(1<<62)) {
        return 0, ErrOverflow
    }
    return a + b, nil
}

// Subtract returns the difference.
func Subtract(a, b int) int {
    return a - b
}
```

This is the right resolution: function name from main (`Sum`), signature + overflow logic + sentinel from the branch, and the doc comment was updated to use the new name (the agent caught that subtlety — the original branch said "Add overflow", the merged version says "Sum overflow").

`apply_resolver_success` then ran cleanly:
```
ok=True
detail=rebase continued cleanly
```

Final git state: `feat/overflow-check` branch has 3 commits (init, rename Add to Sum, add overflow check to Add) — the rebase completed and the original commit message is preserved.

Cost: ~31k tokens, ~15s wall, 3 tool uses by the resolver.

## Interpretation

The trivial-conflict path works end-to-end: assessment → prompt build → dispatch → resolver edits → apply success → rebase continues. The prompt's instructions were followed precisely (text-only edits, no git, only the listed file).

**One caveat to keep in mind:** the spike included a small oracle hint about the right merged intent. In production the coordinator won't know the right answer in advance — the resolver will have to infer intent from the two sides alone. The spike doesn't prove the resolver can do that reliably in harder cases. Step 7 (Opus fallback) covers the cases where Sonnet gets it wrong, and the prompt's "if unsure, EXIT 1" rule biases toward escalation rather than confident-but-wrong merges.

**Template extractor fix:** the bug uncovered (`find` → `rfind` for closing fence) is a load-bearing fix — without it, every resolver prompt was being truncated mid-section. Test added to lock in the behavior.

## Decisions

- Trivial threshold (≤30 markers, ≤3 files) is **kept as is** for now. Re-tune if real sweeps show a different empirical band.
- Resolver prompt is **kept as is** — no revisions warranted from this one spike. Future revisions tracked in the prompt's "Notes for future revisions" section.
- The `apply_resolver_success` content-marker check (vs. index-state check) is **load-bearing** — without it, a resolver that didn't actually edit the file would still pass `git add -u` and continue the rebase with markers still in the file. Test `test_refuses_when_markers_remain` covers this.

## Implications for the rest of the build

- Step 7 (Opus file-copy fallback) needs to handle the case where `apply_resolver_success` returns `False` (resolver claimed success but markers remain) — it should treat that as `UNCERTAIN` and dispatch the fallback, same as if the resolver had explicitly returned `EXIT_REASON: uncertain`.
- The coordinator needs to invoke `abort_rebase(worktree)` before dispatching the fallback, because the Opus path operates on a clean tree (it cherry-picks fresh files), not on a mid-rebase state.

## Cost

- 1 sub-agent dispatch, ~31k tokens, ~15s wall.
- 1 prompt-extractor bug found and fixed during the spike (would have shipped silently otherwise).
- Worth it.
