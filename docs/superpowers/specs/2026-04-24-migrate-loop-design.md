---
title: migrate-loop — autonomous TDD migration harness
date: 2026-04-24
status: draft
target_repo: jdfalk/migrate-loop (new standalone repo)
prior_examples:
  - docs/superpowers/specs/2026-04-24-aijobs-batch-migration-design.md
  - docs/superpowers/plans/2026-04-24-aijobs-batch-migration.md
  - itunes Importer extraction (PR series 1/3, 2/3, 3/3)
---

# migrate-loop — autonomous TDD migration harness

## Goal

A standalone Go CLI, invoked as `migrate-loop --spec <migration.md> --budget 50`, that drives a Test-Driven-Development migration loop autonomously. It writes a comprehensive failing test suite from a spec, then iterates an LLM coder until the suite is green or a budget is exhausted, then closes coverage gaps, then opens a PR — escalating to the human only when tests themselves seem wrong, the budget is exhausted, or the agent is fundamentally stuck.

Modeled on the audiobook-organizer **aijobs batch-API migration** and the **iTunes Importer extraction**, both of which followed a hand-driven test-first → loop-until-green → polish rhythm. This harness codifies that rhythm.

## Locked decisions (from brainstorming)

| Decision | Choice |
|---|---|
| Home | Standalone repo `jdfalk/migrate-loop` (will be reused on other repos) |
| Agent substrate | `claude -p` headless, `--allowed-tools` scoped per phase |
| Budget unit | Iterations of CODE loop, primary; `--iter-timeout 10m` secondary |
| Stagnation guard | 3 consecutive flat iterations → REDIRECT (one-shot) → escalate on next stagnation |
| Escalation surface | `ESCALATION.md` written to worktree root + exit code 2 |
| Test freeze | Pre-commit hook in target worktree rejects `*_test.go` writes unless `ALLOW_TEST_EDITS=1` |
| Coverage budget | Separate `--coverage-budget` flag, default `ceil(0.3 * --budget)` |
| Spec format | YAML frontmatter (machine-readable hooks) + free-form markdown body (prose reasoning) |
| Progress signal | `go test -json` parsed; progress = failing-count↓ OR failing-set rotated |
| Worktree | Harness creates a fresh worktree on `migrate/<slug>` branched from `origin/main` |
| Internal architecture | Phase-as-package, state machine in `cmd/migrate-loop/main.go` |

## State machine

```
INIT
  ├─ parse --spec frontmatter, validate
  ├─ git fetch origin && create worktree on branch migrate/<slug>
  ├─ install pre-commit hook (rejects *_test.go writes unless ALLOW_TEST_EDITS=1)
  ├─ install lock (.migrate-loop.lock, flock(2))
  └─ write STATE.md (phase=PLAN, iteration=0)

PLAN  (one claude -p invocation; ALLOW_TEST_EDITS=1; EXPECTED_COMMIT_PREFIX=test(plan))
  ├─ planner reads spec body + prior_examples, writes failing tests
  ├─ harness re-runs go test -json to confirm RED (else escalate: tests_vacuous)
  └─ commit "test(plan): <slug> failing test suite" → STATE.phase=CODE

LOOP (up to --budget iterations of CODE)
  CODE  (one claude -p per iteration; EXPECTED_COMMIT_PREFIX=wip(coder-N))
    ├─ coder reads STATE.md, edits src, runs go test -race + go vet
    ├─ pre-commit hook rejects *_test.go changes (no ALLOW_TEST_EDITS)
    ├─ harness re-runs go test -json (canonical truth)
    ├─ progress check: count↓ OR set rotated?
    │    yes → reset stagnation_streak, continue
    │    no  → stagnation_streak++; if ==3 → REDIRECT once, then escalate at ==4
    └─ all green → STATE.phase=COVER, exit loop

  REDIRECT  (single elevated claude -p; one-shot per CODE block)
    └─ "you've stagnated, here's what's been tried" → stagnation_streak=0, continue

COVER (up to --coverage-budget iterations)
  ├─ harness runs go test -coverprofile, identifies touched-but-untested symbols
  ├─ coverage subagent (planner-style: ALLOW_TEST_EDITS=1) writes new tests
  ├─ commit "test(coverage): close gap on <symbol>"
  └─ if new tests red → re-enter LOOP (shares remaining --budget, not --coverage-budget)

PR
  ├─ git push -u origin migrate/<slug>
  ├─ gh pr create with body summarizing iterations, oscillations, cost, coverage delta
  └─ exit 0

ESCALATE (from any phase)
  ├─ write ESCALATION.md (reason, last test output, last 3 diffs, agent diagnosis)
  ├─ commit "chore(migrate-loop): escalate <reason>"
  └─ exit 2 (worktree intact for human resolution)
```

## Architecture (Approach A — phase-as-package)

```
migrate-loop/
  cmd/migrate-loop/main.go          flag parsing + state-machine switch
  internal/spec/                    frontmatter parser + validator
  internal/worktree/                git operations + hook installation
  internal/runner/                  test runner abstraction (Go in v1)
  internal/agent/                   claude -p invocation, prompt rendering
  internal/phases/                  plan.go, code.go, redirect.go, cover.go, pr.go
  internal/state/                   STATE.md round-trip + progress tracking
  internal/escalate/                ESCALATION.md writer
  prompts/                          embedded *.tmpl files (//go:embed)
  testdata/specs/                   fixture migration.md files
  testdata/fixtures/                end-to-end fixture target repos
  testdata/priors/                  fake prior examples for prompt tests
```

### Key interfaces

```go
// internal/agent — boundary between control flow and Claude
type Agent interface {
    Run(ctx context.Context, req Request) (Response, error)
}
type Request struct {
    Phase           Phase  // PLAN | CODE | REDIRECT | COVER
    Cwd             string
    AllowedTools    []string
    DisallowedTools []string
    Env             map[string]string  // ALLOW_TEST_EDITS, EXPECTED_COMMIT_PREFIX
    Prompt          string
    Timeout         time.Duration
}
type Response struct {
    ExitCode  int
    Stdout    string
    Stderr    string
    Duration  time.Duration
    SessionID string  // parsed from claude -p --output-format json
    Cost      float64
}

// internal/runner — seam for "Go today, pytest tomorrow"
type Runner interface {
    Run(ctx context.Context, cwd string) (Result, error)
    CoverProfile(ctx context.Context, cwd string) (CoverageReport, error)
}
type Result struct {
    Failing []TestID
    Passing []TestID
    Errors  []string  // build failures, panics
    Raw     []byte    // full -json for STATE.md
}

// internal/state — single source of truth
type State struct {
    Slug                  string
    Phase                 Phase
    Iteration             int
    StagnationStreak      int
    OscillationLog        []OscillationEvent
    LastFailing           []TestID
    LastDiffSummary       string
    BudgetUsed            int
    CoverageBudgetUsed    int
    HumanInterventionCount int
    EscalationReason      string
}
```

State marshals to `STATE.md` (YAML frontmatter + human-readable body). Re-running the harness on the same worktree resumes from `STATE.phase`.

### Phase boundary contract

Each phase is a pure function over `*State` + `Deps`:

```go
type Deps struct {
    Agent    agent.Agent
    Runner   runner.Runner
    Worktree worktree.Worktree
    Clock    func() time.Time  // injectable for tests
}
func Plan(ctx context.Context, st *state.State, d Deps) error
func Code(ctx context.Context, st *state.State, d Deps) (advance bool, err error)
// ...
```

Phases own *what to do*. `agent` owns *how to talk to Claude*. This means `FakeAgent` in tests can drive the entire state machine deterministically with no API spend.

## Spec format

```markdown
---
title: aijobs batch-API migration
slug: aijobs-batch-migration
target_packages:
  - internal/ai/aijobs
  - internal/ai
test_runner: "go test -race -json ./..."
prior_examples:
  - docs/superpowers/specs/2026-04-24-aijobs-batch-migration-design.md
  - PR#XXX  # planner can resolve via gh pr view
success_criteria:
  - all tests in target_packages pass
  - coverage on internal/ai/aijobs >= 75%
---

# Free-form markdown body
[narrative reasoning, edge cases, gotchas — same shape as existing specs]
```

The frontmatter is the machine-readable contract; the body is the prose the planner reads to understand intent.

## Data flow — happy path summary

```
chore(migrate-loop): init aijobs-batch-migration
test(plan): aijobs-batch-migration failing test suite
chore(migrate-loop): plan complete, 14 tests red
wip(coder-1): wire RowResult to Content string
wip(coder-2): handle .data unwrap in dispatcher
... (5 more wip commits) ...
chore(migrate-loop): all green at iteration 7/50
test(coverage): close gap on RowResult.Unmarshal
chore(migrate-loop): cover phase done, +3 tests, +6.2% coverage
```

The commit graph **is** the audit trail.

## Error handling

Three failure classes, three responses:

### Class 1 — Infrastructure errors (exit 1, no ESCALATION.md)
Harness's own bugs, missing tools, git/network failures. Examples: `claude` not on PATH, malformed frontmatter, unparseable `go test -json`. Each is a typed error; `main` switches on type for exit code.

### Class 2 — Migration escalations (exit 2 + ESCALATION.md)

| Reason | Trigger |
|---|---|
| `tests_vacuous` | PLAN finishes but `go test -json` shows 0 FAIL |
| `budget_exhausted` | LOOP hit `--budget` without reaching green |
| `stagnation_after_redirect` | Stagnation streak hit 4 (one REDIRECT used) |
| `tests_seem_wrong` | Coder wrote to `FROZEN_TESTS.md` (the sanctioned objection channel) |
| `iteration_timeout` | Single `claude -p` invocation exceeded `--iter-timeout` 3 times |

`ESCALATION.md` is committed (`chore(migrate-loop): escalate <reason>`) so the worktree is in a clean, resumable state.

### Class 3 — Resumable interruptions (no exit, no file)
- Re-invoking on a worktree where `STATE.phase != INIT` resumes from `STATE.phase`.
- `Ctrl-C` writes "interrupted at iteration N" to `STATE.md`, exits 130.
- Single transient `claude -p` failure: retry once with 30s backoff. Second failure → escalate `iteration_timeout`.

### The escape hatch — `FROZEN_TESTS.md`

Empty file in the worktree, referenced by the pre-commit hook's reject message. If the coder thinks a test is wrong, the only sanctioned way to communicate is to write to `FROZEN_TESTS.md`. The harness reads it after each CODE iteration; non-empty content triggers `tests_seem_wrong` escalation. Turns a deadlock into an escalation.

### Resumption semantics

- After `tests_seem_wrong`: human edits tests, commits as `test(human): ...`, runs `migrate-loop --resume`. Stagnation counters cleared, `human_intervention_count++`.
- After `budget_exhausted`: `migrate-loop --resume --budget 25` adds iterations. Stagnation counters preserved (so a still-stuck loop re-escalates fast, which is correct).
- After `tests_vacuous`: human fixes the spec, re-invocation re-runs PLAN as `test(plan-v2): ...`.

### Concurrency
- `.migrate-loop.lock` (`flock(2)`) in worktree root, held for run duration.
- Stale lock (PID dead) requires explicit `--force-unlock` (no auto-recovery — that masks bugs).

## Testing strategy

Three layers:

### Layer 1 — Unit tests with `FakeAgent` (fast, free, majority of coverage)

`FakeAgent` returns canned `Response`s in order; `FakeAgent.Editor` is a closure that simulates the real agent's side effects (writing files, committing). Tests verify: after `Plan()`, expected test files exist, expected commit prefix appears, `STATE.phase == CODE`.

Per-package coverage:
- `spec/`: malformed frontmatter, missing keys, prior_examples resolution.
- `worktree/`: branch already exists, dirty parent, hook installation, hook-content correctness.
- `runner/`: `go test -json` edge cases (build failures, panics, t.Skip, slashes in subtest names, multiple FAIL events).
- `state/`: round-trip, oscillation detection, stagnation math.
- `phases/`: each phase × 3-5 scripted scenarios (happy, agent-fails-to-commit, hook-rejects, timeout).

### Layer 2 — Integration tests with FakeAgent + real git/runner

End-to-end on fixture target repos with scripted agent responses:

```
testdata/fixtures/
  trivial-add/                 tiny target with simple feature
  oscillation-recovery/        triggers oscillation, validates REDIRECT
  budget-exhaustion/           coder never reaches green
  tests-vacuous/               planner writes 0-FAIL tests → escalate
  resume-after-escalation/     multi-step: run, escalate, edit, --resume
```

Each asserts on: exit code, golden commit-graph file, `ESCALATION.md` content, final `STATE.md` fields.

### Layer 3 — Real-API smoke tests (slow, costly, gated)

Build tag `live_api`, runs `claude -p` against `testdata/fixtures/trivial-add` with `--budget 5`. Cost-bounded ($0.50–$2/run). Run by `make test-live` and a nightly Action. **Existing primarily as a prompt-drift canary**, not a quality test.

### Explicitly NOT tested
- LLM output quality. Layer 3 covers smoke; unit-layer string-matching on prompt outputs is brittle theater.
- `claude -p` network behavior. Trust the binary; non-zero exit = `iteration_timeout`.

### Coverage gate
80% required for non-prompt non-cmd packages. `cmd/` and `prompts/` excluded — `cmd/` is glue, `prompts/` is `//go:embed`'d Markdown.

## Open questions for the implementation plan

1. Exact `claude -p` flag set and `--allowed-tools` strings for each phase (verify against current Claude Code version).
2. `STATE.md` YAML schema versioning (probably `schema_version: 1` from day one for forward compat).
3. PR body template (Go template? Markdown with placeholders?).
4. Whether to commit `STATE.md` with each transition or write it un-versioned (decision: commit it, so resume is fully git-driven).
5. Bootstrap order for the new repo: this spec lives in `audiobook-organizer/docs/superpowers/specs/` for review; once approved, copy to `jdfalk/migrate-loop` repo as `docs/design.md` at repo init.

## Out of scope

- Non-Go test runners in v1 (`Runner` interface admits them; `pytest` impl is a follow-up).
- GUI / TUI for the loop. CLI + STATE.md is enough.
- Multi-repo orchestration (running migrations across several repos in one invocation).
- Resuming across machines (lock is local only).
- Cost-based budget (out by explicit decision; iterations chosen).
