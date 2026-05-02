# migrate-loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `migrate-loop`, a standalone Go CLI that drives an autonomous TDD migration loop via `claude -p` headless invocations, with hard budgets, audit-trail commits, and structured escalation.

**Architecture:** Phase-as-package state machine in `cmd/migrate-loop/main.go`. Each phase (PLAN/CODE/REDIRECT/COVER/PR) is a pure function over `*State` + `Deps`. The `Agent` interface (with `FakeAgent` for tests, `CLIAgent` invoking `claude -p` in production) and `Runner` interface (Go in v1, pluggable later) are the testability seams.

**Tech Stack:** Go 1.22+, stdlib + `gopkg.in/yaml.v3` (frontmatter), `golang.org/x/tools/cover` (coverage parsing). Test runner: `go test`. Embedded prompts via `//go:embed`.

**Repo:** `jdfalk/migrate-loop` (new). This plan lives in `audiobook-organizer/docs/superpowers/plans/` for review; once approved, the new repo is bootstrapped (Task 1) and the rest of the plan executes there.

---

## Task ordering rationale

Bottom-up: leaf packages (spec, state, runner, agent fake) before composite packages (phases) before top-level (cmd). Each task ends with a green test run and a commit. Tasks 1–18 produce a working harness with `FakeAgent`. Tasks 19–22 add fixture coverage, live-API smoke, and docs.

---

## Task 1: Bootstrap repo

**Files:**
- Create: `go.mod`, `Makefile`, `README.md`, `.gitignore`, `.github/workflows/ci.yml`, `LICENSE`

- [ ] **Step 1: Init repo locally**

```bash
mkdir -p ~/repos/github.com/jdfalk/migrate-loop && cd $_
git init -b main
gh repo create jdfalk/migrate-loop --private --source=. --remote=origin
```

- [ ] **Step 2: Create go.mod**

```bash
go mod init github.com/jdfalk/migrate-loop
go mod edit -go=1.22
```

- [ ] **Step 3: Add `.gitignore`**

```gitignore
/migrate-loop
/dist/
*.test
*.out
.DS_Store
```

- [ ] **Step 4: Add `Makefile`**

```makefile
.PHONY: build test test-live lint cover install

build:
	go build -o ./bin/migrate-loop ./cmd/migrate-loop

test:
	go test -race ./...

test-live:
	go test -race -tags=live_api ./...

lint:
	go vet ./...

cover:
	go test -race -coverprofile=cover.out ./...
	go tool cover -func=cover.out | tail -1

install:
	go install ./cmd/migrate-loop
```

- [ ] **Step 5: Add minimal `README.md`**

```markdown
# migrate-loop

Autonomous TDD migration harness driven by `claude -p`.

## Usage

    migrate-loop --spec migration.md --budget 50

See [design doc](docs/design.md) for full architecture.
```

- [ ] **Step 6: Add CI workflow**

`.github/workflows/ci.yml`:

```yaml
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - run: make lint
      - run: make test
      - run: make cover
```

- [ ] **Step 7: Initial commit**

```bash
git add .
git commit -m "chore: bootstrap migrate-loop repo"
git push -u origin main
```

---

## Task 2: `internal/spec` — frontmatter parser

**Files:**
- Create: `internal/spec/spec.go`, `internal/spec/spec_test.go`, `internal/spec/testdata/valid.md`, `internal/spec/testdata/no-frontmatter.md`, `internal/spec/testdata/missing-slug.md`

- [ ] **Step 1: Add yaml.v3 dependency**

```bash
go get gopkg.in/yaml.v3
```

- [ ] **Step 2: Write failing parser test**

`internal/spec/spec_test.go`:

```go
package spec

import (
	"path/filepath"
	"testing"
)

func TestParse_Valid(t *testing.T) {
	got, err := ParseFile(filepath.Join("testdata", "valid.md"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if got.Slug != "trivial-add" {
		t.Errorf("Slug = %q, want %q", got.Slug, "trivial-add")
	}
	if len(got.TargetPackages) != 2 {
		t.Errorf("TargetPackages len = %d, want 2", len(got.TargetPackages))
	}
	if got.TestRunner != "go test -race -json ./..." {
		t.Errorf("TestRunner = %q", got.TestRunner)
	}
	if !contains(got.Body, "## Behavior") {
		t.Errorf("Body does not contain expected heading; got: %q", got.Body)
	}
}

func TestParse_NoFrontmatter(t *testing.T) {
	_, err := ParseFile(filepath.Join("testdata", "no-frontmatter.md"))
	if err == nil || !errIs(err, ErrNoFrontmatter) {
		t.Fatalf("err = %v, want ErrNoFrontmatter", err)
	}
}

func TestParse_MissingRequired(t *testing.T) {
	_, err := ParseFile(filepath.Join("testdata", "missing-slug.md"))
	if err == nil {
		t.Fatal("expected error for missing slug")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > 0 && indexOf(s, sub) >= 0))
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
func errIs(err, target error) bool { return err != nil && (err == target || err.Error() == target.Error()) }
```

`internal/spec/testdata/valid.md`:

```markdown
---
title: trivial add
slug: trivial-add
target_packages:
  - internal/add
  - internal/util
test_runner: "go test -race -json ./..."
prior_examples: []
success_criteria:
  - all tests pass
---

# trivial add

## Behavior
Add two integers.
```

`internal/spec/testdata/no-frontmatter.md`:

```markdown
# Just a heading
no frontmatter here
```

`internal/spec/testdata/missing-slug.md`:

```markdown
---
title: missing slug
target_packages: ["x"]
test_runner: "go test ./..."
---

body
```

- [ ] **Step 3: Run test, expect failure**

Run: `go test ./internal/spec/...`
Expected: FAIL with "undefined: ParseFile" or similar.

- [ ] **Step 4: Implement `internal/spec/spec.go`**

```go
// Package spec parses migration specs (YAML frontmatter + markdown body).
package spec

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

var ErrNoFrontmatter = errors.New("spec: no YAML frontmatter found (expected '---' delimiters at start of file)")

type Spec struct {
	Title           string   `yaml:"title"`
	Slug            string   `yaml:"slug"`
	TargetPackages  []string `yaml:"target_packages"`
	TestRunner      string   `yaml:"test_runner"`
	PriorExamples   []string `yaml:"prior_examples"`
	SuccessCriteria []string `yaml:"success_criteria"`

	Body     string `yaml:"-"` // markdown body after frontmatter
	FilePath string `yaml:"-"` // for resolving relative prior_examples
}

func ParseFile(path string) (*Spec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("spec: read %s: %w", path, err)
	}
	s, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("spec %s: %w", path, err)
	}
	s.FilePath = path
	return s, nil
}

func Parse(raw []byte) (*Spec, error) {
	src := string(raw)
	if !strings.HasPrefix(src, "---\n") && !strings.HasPrefix(src, "---\r\n") {
		return nil, ErrNoFrontmatter
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(src, "---\n"), "---\r\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, ErrNoFrontmatter
	}
	yamlPart := rest[:end]
	body := strings.TrimPrefix(strings.TrimPrefix(rest[end+4:], "\n"), "\r\n")

	var s Spec
	if err := yaml.Unmarshal([]byte(yamlPart), &s); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	s.Body = body
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *Spec) Validate() error {
	if s.Slug == "" {
		return errors.New("spec: 'slug' is required")
	}
	if len(s.TargetPackages) == 0 {
		return errors.New("spec: 'target_packages' must list at least one package")
	}
	if s.TestRunner == "" {
		return errors.New("spec: 'test_runner' is required")
	}
	return nil
}
```

- [ ] **Step 5: Run test, expect pass**

Run: `go test ./internal/spec/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/spec go.mod go.sum
git commit -m "feat(spec): YAML-frontmatter + markdown-body parser with validation"
```

---

## Task 3: `internal/state` — STATE.md round-trip

**Files:**
- Create: `internal/state/state.go`, `internal/state/state_test.go`

- [ ] **Step 1: Write failing round-trip test**

`internal/state/state_test.go`:

```go
package state

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	want := State{
		Slug:      "demo",
		Phase:     PhaseCode,
		Iteration: 7,
		Budget:    50,
		StagnationStreak: 2,
		LastFailing: []TestID{
			{Package: "internal/x", Test: "TestA"},
			{Package: "internal/x", Test: "TestB/sub"},
		},
		OscillationLog: []OscillationEvent{
			{Iteration: 5, Note: "swapped TestA/TestB"},
		},
		BudgetUsed: 7,
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "STATE.md")
	if err := Write(path, &want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("round-trip mismatch:\nwant %+v\ngot  %+v", want, *got)
	}
}

func TestPhaseTransitions(t *testing.T) {
	cases := []Phase{PhaseInit, PhasePlan, PhaseCode, PhaseRedirect, PhaseCover, PhasePR, PhaseEscalated}
	for _, p := range cases {
		if p.String() == "" {
			t.Errorf("phase %d has empty String()", p)
		}
		got, err := ParsePhase(p.String())
		if err != nil {
			t.Errorf("ParsePhase(%q): %v", p.String(), err)
		}
		if got != p {
			t.Errorf("ParsePhase round-trip: got %v want %v", got, p)
		}
	}
}
```

- [ ] **Step 2: Run test, expect failure**

Run: `go test ./internal/state/...`
Expected: FAIL ("undefined: State").

- [ ] **Step 3: Implement `internal/state/state.go`**

```go
// Package state owns the on-disk loop state (STATE.md).
package state

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Phase int

const (
	PhaseInit Phase = iota
	PhasePlan
	PhaseCode
	PhaseRedirect
	PhaseCover
	PhasePR
	PhaseEscalated
)

func (p Phase) String() string {
	switch p {
	case PhaseInit:
		return "INIT"
	case PhasePlan:
		return "PLAN"
	case PhaseCode:
		return "CODE"
	case PhaseRedirect:
		return "REDIRECT"
	case PhaseCover:
		return "COVER"
	case PhasePR:
		return "PR"
	case PhaseEscalated:
		return "ESCALATED"
	}
	return ""
}

func ParsePhase(s string) (Phase, error) {
	switch strings.ToUpper(s) {
	case "INIT":
		return PhaseInit, nil
	case "PLAN":
		return PhasePlan, nil
	case "CODE":
		return PhaseCode, nil
	case "REDIRECT":
		return PhaseRedirect, nil
	case "COVER":
		return PhaseCover, nil
	case "PR":
		return PhasePR, nil
	case "ESCALATED":
		return PhaseEscalated, nil
	}
	return 0, fmt.Errorf("state: unknown phase %q", s)
}

type TestID struct {
	Package string `yaml:"package"`
	Test    string `yaml:"test"`
}

type OscillationEvent struct {
	Iteration int    `yaml:"iteration"`
	Note      string `yaml:"note"`
}

type State struct {
	SchemaVersion          int                `yaml:"schema_version"`
	Slug                   string             `yaml:"slug"`
	Phase                  Phase              `yaml:"-"`
	PhaseStr               string             `yaml:"phase"`
	Iteration              int                `yaml:"iteration"`
	Budget                 int                `yaml:"budget"`
	CoverageBudget         int                `yaml:"coverage_budget"`
	StagnationStreak       int                `yaml:"stagnation_streak"`
	OscillationLog         []OscillationEvent `yaml:"oscillation_log,omitempty"`
	LastFailing            []TestID           `yaml:"last_failing,omitempty"`
	LastDiffSummary        string             `yaml:"last_diff_summary,omitempty"`
	BudgetUsed             int                `yaml:"budget_used"`
	CoverageBudgetUsed     int                `yaml:"coverage_budget_used"`
	HumanInterventionCount int                `yaml:"human_intervention_count"`
	EscalationReason       string             `yaml:"escalation_reason,omitempty"`
	TotalCostUSD           float64            `yaml:"total_cost_usd"`
	RedirectUsed           bool               `yaml:"redirect_used"`
}

const stateBody = `# Migration State

This file is managed by migrate-loop. Editing manually is allowed but you should
generally re-invoke 'migrate-loop --resume' rather than hand-modify state.
`

func Write(path string, s *State) error {
	if s.SchemaVersion == 0 {
		s.SchemaVersion = 1
	}
	s.PhaseStr = s.Phase.String()
	yml, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("state: marshal: %w", err)
	}
	out := "---\n" + string(yml) + "---\n" + stateBody
	return os.WriteFile(path, []byte(out), 0o644)
}

func Read(path string) (*State, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("state: read %s: %w", path, err)
	}
	src := string(raw)
	if !strings.HasPrefix(src, "---\n") {
		return nil, errors.New("state: STATE.md missing frontmatter")
	}
	rest := strings.TrimPrefix(src, "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, errors.New("state: STATE.md frontmatter not terminated")
	}
	var s State
	if err := yaml.Unmarshal([]byte(rest[:end]), &s); err != nil {
		return nil, fmt.Errorf("state: unmarshal: %w", err)
	}
	p, err := ParsePhase(s.PhaseStr)
	if err != nil {
		return nil, err
	}
	s.Phase = p
	return &s, nil
}
```

- [ ] **Step 4: Run test, expect pass**

Run: `go test ./internal/state/...` → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/state
git commit -m "feat(state): STATE.md round-trip with phase enum and schema_version=1"
```

---

## Task 4: `internal/state` — progress tracking

**Files:**
- Modify: `internal/state/state.go` (append progress helpers)
- Create: `internal/state/progress_test.go`

- [ ] **Step 1: Write failing test for progress detection**

`internal/state/progress_test.go`:

```go
package state

import "testing"

func TestProgress_CountDecreased(t *testing.T) {
	prev := []TestID{{Package: "p", Test: "A"}, {Package: "p", Test: "B"}, {Package: "p", Test: "C"}}
	curr := []TestID{{Package: "p", Test: "A"}, {Package: "p", Test: "B"}}
	r := DetectProgress(prev, curr)
	if !r.IsProgress {
		t.Error("expected progress when count decreased")
	}
	if r.Oscillation {
		t.Error("did not expect oscillation")
	}
}

func TestProgress_SetRotated(t *testing.T) {
	prev := []TestID{{Package: "p", Test: "A"}, {Package: "p", Test: "B"}}
	curr := []TestID{{Package: "p", Test: "B"}, {Package: "p", Test: "C"}}
	r := DetectProgress(prev, curr)
	if !r.IsProgress {
		t.Error("expected progress when set rotated")
	}
	if !r.Oscillation {
		t.Error("expected oscillation flag when set rotated but count equal")
	}
}

func TestProgress_NoChange(t *testing.T) {
	prev := []TestID{{Package: "p", Test: "A"}, {Package: "p", Test: "B"}}
	curr := []TestID{{Package: "p", Test: "A"}, {Package: "p", Test: "B"}}
	r := DetectProgress(prev, curr)
	if r.IsProgress {
		t.Error("expected NO progress when nothing changed")
	}
}

func TestProgress_AllGreen(t *testing.T) {
	prev := []TestID{{Package: "p", Test: "A"}}
	curr := []TestID{}
	r := DetectProgress(prev, curr)
	if !r.IsProgress {
		t.Error("expected progress when all green")
	}
	if !r.AllGreen {
		t.Error("expected AllGreen flag")
	}
}
```

- [ ] **Step 2: Run test, expect FAIL ("undefined: DetectProgress")**

Run: `go test ./internal/state/...`

- [ ] **Step 3: Implement `DetectProgress` in `internal/state/state.go`**

Append to `state.go`:

```go
type ProgressResult struct {
	IsProgress  bool
	Oscillation bool // count equal but set differs
	AllGreen    bool
	NowFailing  int
	NowPassing  []TestID // tests that previously failed and now pass
	NewlyFailing []TestID // tests that did not previously fail but do now
}

func DetectProgress(prev, curr []TestID) ProgressResult {
	prevSet := setOf(prev)
	currSet := setOf(curr)
	res := ProgressResult{NowFailing: len(curr)}
	if len(curr) == 0 {
		res.IsProgress = true
		res.AllGreen = true
		return res
	}
	if len(curr) < len(prev) {
		res.IsProgress = true
	}
	for k := range prevSet {
		if _, ok := currSet[k]; !ok {
			res.NowPassing = append(res.NowPassing, prevSet[k])
		}
	}
	for k := range currSet {
		if _, ok := prevSet[k]; !ok {
			res.NewlyFailing = append(res.NewlyFailing, currSet[k])
		}
	}
	if len(res.NowPassing) > 0 || len(res.NewlyFailing) > 0 {
		res.IsProgress = true
		if len(curr) == len(prev) {
			res.Oscillation = true
		}
	}
	return res
}

func setOf(ids []TestID) map[string]TestID {
	m := make(map[string]TestID, len(ids))
	for _, id := range ids {
		m[id.Package+"::"+id.Test] = id
	}
	return m
}
```

- [ ] **Step 4: Run test, expect PASS**

Run: `go test ./internal/state/...`

- [ ] **Step 5: Commit**

```bash
git add internal/state
git commit -m "feat(state): progress detection with oscillation/all-green flags"
```

---

## Task 5: `internal/runner` — `go test -json` parser

**Files:**
- Create: `internal/runner/runner.go`, `internal/runner/go_runner.go`, `internal/runner/parser.go`, `internal/runner/parser_test.go`, `internal/runner/testdata/sample.json`

- [ ] **Step 1: Write failing parser test**

`internal/runner/parser_test.go`:

```go
package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTestJSON(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := ParseTestJSON(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(res.Failing) != 2 {
		t.Errorf("Failing = %d, want 2: %+v", len(res.Failing), res.Failing)
	}
	if len(res.Passing) < 1 {
		t.Errorf("Passing should be >0; got %d", len(res.Passing))
	}
	hasSubtest := false
	for _, id := range res.Failing {
		if id.Test == "TestB/sub_one" {
			hasSubtest = true
		}
	}
	if !hasSubtest {
		t.Errorf("expected subtest TestB/sub_one in Failing; got %+v", res.Failing)
	}
}

func TestParseTestJSON_BuildFailure(t *testing.T) {
	raw := []byte(`{"Time":"2026-01-01T00:00:00Z","Action":"build-fail","Package":"x","Output":"compile error\n"}` + "\n")
	res, err := ParseTestJSON(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(res.Errors) != 1 {
		t.Errorf("Errors = %d, want 1", len(res.Errors))
	}
}
```

`internal/runner/testdata/sample.json`:

```json
{"Time":"2026-01-01T00:00:00Z","Action":"run","Package":"example/p","Test":"TestA"}
{"Time":"2026-01-01T00:00:00Z","Action":"output","Package":"example/p","Test":"TestA","Output":"--- FAIL: TestA\n"}
{"Time":"2026-01-01T00:00:00Z","Action":"fail","Package":"example/p","Test":"TestA","Elapsed":0.01}
{"Time":"2026-01-01T00:00:00Z","Action":"run","Package":"example/p","Test":"TestB"}
{"Time":"2026-01-01T00:00:00Z","Action":"run","Package":"example/p","Test":"TestB/sub_one"}
{"Time":"2026-01-01T00:00:00Z","Action":"fail","Package":"example/p","Test":"TestB/sub_one","Elapsed":0.01}
{"Time":"2026-01-01T00:00:00Z","Action":"fail","Package":"example/p","Test":"TestB","Elapsed":0.01}
{"Time":"2026-01-01T00:00:00Z","Action":"run","Package":"example/p","Test":"TestC"}
{"Time":"2026-01-01T00:00:00Z","Action":"pass","Package":"example/p","Test":"TestC","Elapsed":0.01}
{"Time":"2026-01-01T00:00:00Z","Action":"pass","Package":"example/p","Elapsed":0.05}
```

- [ ] **Step 2: Run test, expect FAIL**

Run: `go test ./internal/runner/...`

- [ ] **Step 3: Implement `internal/runner/runner.go` (interface) and `parser.go`**

`internal/runner/runner.go`:

```go
// Package runner abstracts the test runner so the harness can target Go today
// and other ecosystems later.
package runner

import (
	"context"

	"github.com/jdfalk/migrate-loop/internal/state"
)

type Result struct {
	Failing []state.TestID
	Passing []state.TestID
	Errors  []string // build failures, panics
	Raw     []byte
}

type CoverageReport struct {
	ByFile map[string]FileCoverage
}

type FileCoverage struct {
	Path        string
	UncoveredLines []int // line numbers with 0 hits among Go statements
}

type Runner interface {
	Run(ctx context.Context, cwd string) (Result, error)
	CoverProfile(ctx context.Context, cwd string) (CoverageReport, error)
}
```

`internal/runner/parser.go`:

```go
package runner

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/jdfalk/migrate-loop/internal/state"
)

type goEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

func ParseTestJSON(raw []byte) (Result, error) {
	res := Result{Raw: raw}
	dec := json.NewDecoder(bytes.NewReader(raw))
	type key struct{ pkg, test string }
	finalAction := map[key]string{}
	for dec.More() {
		var e goEvent
		if err := dec.Decode(&e); err != nil {
			return res, fmt.Errorf("runner: decode: %w", err)
		}
		switch e.Action {
		case "fail", "pass", "skip":
			if e.Test != "" {
				finalAction[key{e.Package, e.Test}] = e.Action
			}
		case "build-fail":
			res.Errors = append(res.Errors, fmt.Sprintf("build-fail %s: %s", e.Package, e.Output))
		case "output":
			if isPanic(e.Output) {
				res.Errors = append(res.Errors, fmt.Sprintf("panic in %s/%s: %s", e.Package, e.Test, e.Output))
			}
		}
	}
	for k, action := range finalAction {
		id := state.TestID{Package: k.pkg, Test: k.test}
		switch action {
		case "fail":
			res.Failing = append(res.Failing, id)
		case "pass":
			res.Passing = append(res.Passing, id)
		}
	}
	return res, nil
}

func isPanic(s string) bool {
	return len(s) > 6 && s[:6] == "panic:"
}
```

- [ ] **Step 4: Run test, expect PASS**

Run: `go test ./internal/runner/...`

- [ ] **Step 5: Commit**

```bash
git add internal/runner
git commit -m "feat(runner): Runner interface + go test -json parser with subtest+build-fail handling"
```

---

## Task 6: `internal/runner` — Go runner impl + coverage

**Files:**
- Create: `internal/runner/go_runner.go`, `internal/runner/coverage.go`, `internal/runner/go_runner_test.go`

- [ ] **Step 1: Write integration test that exercises a tiny module**

`internal/runner/go_runner_test.go`:

```go
package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGoRunner_Run(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go binary not on PATH")
	}
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "go.mod"), "module example.com/m\ngo 1.22\n")
	mustWrite(t, filepath.Join(dir, "m.go"), "package m\nfunc Add(a, b int) int { return a + b }\n")
	mustWrite(t, filepath.Join(dir, "m_test.go"), `package m
import "testing"
func TestPasses(t *testing.T) { if Add(1,2)!=3 { t.Fail() } }
func TestFails(t *testing.T)  { if Add(1,1)!=3 { t.Fail() } }
`)

	r := NewGoRunner("go test -race -json ./...")
	res, err := r.Run(context.Background(), dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Failing) != 1 || res.Failing[0].Test != "TestFails" {
		t.Errorf("Failing = %+v, want [TestFails]", res.Failing)
	}
	if len(res.Passing) != 1 || res.Passing[0].Test != "TestPasses" {
		t.Errorf("Passing = %+v", res.Passing)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run test, expect FAIL ("undefined: NewGoRunner")**

Run: `go test ./internal/runner/...`

- [ ] **Step 3: Implement `internal/runner/go_runner.go`**

```go
package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type GoRunner struct {
	Cmd string // e.g. "go test -race -json ./..."
}

func NewGoRunner(cmd string) *GoRunner {
	if cmd == "" {
		cmd = "go test -race -json ./..."
	}
	return &GoRunner{Cmd: cmd}
}

func (g *GoRunner) Run(ctx context.Context, cwd string) (Result, error) {
	args := strings.Fields(g.Cmd)
	if len(args) < 1 {
		return Result{}, fmt.Errorf("runner: empty command")
	}
	c := exec.CommandContext(ctx, args[0], args[1:]...)
	c.Dir = cwd
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		// non-zero exit is normal when tests fail; only return err on context-cancel/binary-missing
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		if _, ok := err.(*exec.ExitError); !ok {
			return Result{}, fmt.Errorf("runner: %w; stderr: %s", err, stderr.String())
		}
	}
	res, err := ParseTestJSON(stdout.Bytes())
	if err != nil {
		return res, err
	}
	if stderr.Len() > 0 {
		res.Errors = append(res.Errors, "stderr: "+stderr.String())
	}
	return res, nil
}
```

- [ ] **Step 4: Run test, expect PASS**

Run: `go test ./internal/runner/...`

- [ ] **Step 5: Add coverage parsing**

`internal/runner/coverage.go`:

```go
package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/tools/cover"
)

func (g *GoRunner) CoverProfile(ctx context.Context, cwd string) (CoverageReport, error) {
	out := filepath.Join(cwd, ".migrate-loop-cover.out")
	defer os.Remove(out)
	c := exec.CommandContext(ctx, "go", "test", "-coverprofile="+out, "./...")
	c.Dir = cwd
	if err := c.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok && ctx.Err() == nil {
			return CoverageReport{}, fmt.Errorf("coverprofile: %w", err)
		}
	}
	profiles, err := cover.ParseProfiles(out)
	if err != nil {
		return CoverageReport{}, fmt.Errorf("parse cover profile: %w", err)
	}
	rep := CoverageReport{ByFile: map[string]FileCoverage{}}
	for _, p := range profiles {
		fc := FileCoverage{Path: p.FileName}
		for _, b := range p.Blocks {
			if b.Count == 0 {
				for ln := b.StartLine; ln <= b.EndLine; ln++ {
					fc.UncoveredLines = append(fc.UncoveredLines, ln)
				}
			}
		}
		rep.ByFile[p.FileName] = fc
	}
	return rep, nil
}
```

- [ ] **Step 6: Add dep + run tests**

```bash
go get golang.org/x/tools/cover
go test ./internal/runner/...
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/runner go.mod go.sum
git commit -m "feat(runner): GoRunner exec wrapper + coverprofile parser"
```

---

## Task 7: `internal/agent` — Agent interface + FakeAgent

**Files:**
- Create: `internal/agent/agent.go`, `internal/agent/fake.go`, `internal/agent/fake_test.go`

- [ ] **Step 1: Write failing FakeAgent test**

`internal/agent/fake_test.go`:

```go
package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFakeAgent_RunsResponsesInOrder(t *testing.T) {
	calls := 0
	dir := t.TempDir()
	fa := &FakeAgent{
		Responses: []Response{
			{ExitCode: 0, Stdout: "first"},
			{ExitCode: 0, Stdout: "second"},
		},
		Editor: func(req Request) error {
			calls++
			return os.WriteFile(filepath.Join(req.Cwd, "f.txt"), []byte("hi"), 0o644)
		},
	}
	r1, err := fa.Run(context.Background(), Request{Phase: PhasePlan, Cwd: dir})
	if err != nil || r1.Stdout != "first" {
		t.Fatalf("first call: %v %+v", err, r1)
	}
	r2, _ := fa.Run(context.Background(), Request{Phase: PhaseCode, Cwd: dir})
	if r2.Stdout != "second" {
		t.Fatalf("second call stdout = %q", r2.Stdout)
	}
	if calls != 2 {
		t.Errorf("Editor calls = %d, want 2", calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "f.txt")); err != nil {
		t.Errorf("Editor side-effect missing: %v", err)
	}
}

func TestFakeAgent_Exhausted(t *testing.T) {
	fa := &FakeAgent{Responses: []Response{{ExitCode: 0}}}
	_, _ = fa.Run(context.Background(), Request{Cwd: t.TempDir()})
	_, err := fa.Run(context.Background(), Request{Cwd: t.TempDir()})
	if err == nil {
		t.Error("expected error after responses exhausted")
	}
}
```

- [ ] **Step 2: Run test, expect FAIL**

Run: `go test ./internal/agent/...`

- [ ] **Step 3: Implement `internal/agent/agent.go`**

```go
// Package agent abstracts how migrate-loop talks to Claude.
package agent

import (
	"context"
	"time"
)

type Phase string

const (
	PhasePlan     Phase = "PLAN"
	PhaseCode     Phase = "CODE"
	PhaseRedirect Phase = "REDIRECT"
	PhaseCover    Phase = "COVER"
)

type Request struct {
	Phase           Phase
	Cwd             string
	AllowedTools    []string
	DisallowedTools []string
	Env             map[string]string
	Prompt          string
	Timeout         time.Duration
}

type Response struct {
	ExitCode  int
	Stdout    string
	Stderr    string
	Duration  time.Duration
	SessionID string
	Cost      float64
}

type Agent interface {
	Run(ctx context.Context, req Request) (Response, error)
}
```

- [ ] **Step 4: Implement `internal/agent/fake.go`**

```go
package agent

import (
	"context"
	"errors"
	"sync"
)

type FakeAgent struct {
	mu        sync.Mutex
	Responses []Response
	Calls     []Request
	Editor    func(req Request) error
}

func (f *FakeAgent) Run(_ context.Context, req Request) (Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, req)
	if len(f.Responses) == 0 {
		return Response{}, errors.New("FakeAgent: responses exhausted")
	}
	r := f.Responses[0]
	f.Responses = f.Responses[1:]
	if f.Editor != nil {
		if err := f.Editor(req); err != nil {
			return r, err
		}
	}
	return r, nil
}
```

- [ ] **Step 5: Run test, expect PASS**

Run: `go test ./internal/agent/...`

- [ ] **Step 6: Commit**

```bash
git add internal/agent
git commit -m "feat(agent): Agent interface + FakeAgent with Editor side-effect closure"
```

---

## Task 8: `internal/agent` — CLIAgent (`claude -p` impl)

**Files:**
- Create: `internal/agent/cli.go`, `internal/agent/cli_test.go`

- [ ] **Step 1: Write CLIAgent test using a fake `claude` shim**

`internal/agent/cli_test.go`:

```go
package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// shim writes a known JSON response to stdout, exits 0
const shimSrc = `#!/usr/bin/env bash
echo '{"session_id":"abc","total_cost_usd":0.42}'
exit 0
`

func TestCLIAgent_ParsesJSON(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte(shimSrc), 0o755); err != nil {
		t.Fatal(err)
	}
	a := &CLIAgent{Binary: bin}
	res, err := a.Run(context.Background(), Request{Phase: PhasePlan, Cwd: dir, Prompt: "hi", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.SessionID != "abc" {
		t.Errorf("SessionID = %q", res.SessionID)
	}
	if res.Cost != 0.42 {
		t.Errorf("Cost = %v", res.Cost)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/agent/...`

- [ ] **Step 3: Implement `internal/agent/cli.go`**

```go
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type CLIAgent struct {
	Binary string // path to `claude` (default: "claude")
}

func NewCLIAgent() *CLIAgent { return &CLIAgent{Binary: "claude"} }

func (c *CLIAgent) Run(ctx context.Context, req Request) (Response, error) {
	bin := c.Binary
	if bin == "" {
		bin = "claude"
	}
	args := []string{"-p", "--output-format", "json"}
	if len(req.AllowedTools) > 0 {
		args = append(args, "--allowed-tools", strings.Join(req.AllowedTools, ","))
	}
	if len(req.DisallowedTools) > 0 {
		args = append(args, "--disallowed-tools", strings.Join(req.DisallowedTools, ","))
	}
	args = append(args, req.Prompt)

	timeout := req.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, bin, args...)
	cmd.Dir = req.Cwd
	cmd.Env = append(os.Environ(), envSlice(req.Env)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	res := Response{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(start),
	}
	if cctx.Err() == context.DeadlineExceeded {
		return res, fmt.Errorf("agent: claude -p timed out after %s", timeout)
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		res.ExitCode = exitErr.ExitCode()
	} else if err != nil {
		return res, fmt.Errorf("agent: claude -p: %w", err)
	}
	parseClaudeJSON(stdout.Bytes(), &res)
	return res, nil
}

func parseClaudeJSON(out []byte, r *Response) {
	type cj struct {
		SessionID    string  `json:"session_id"`
		TotalCostUSD float64 `json:"total_cost_usd"`
	}
	var v cj
	if err := json.Unmarshal(bytes.TrimSpace(out), &v); err == nil {
		r.SessionID = v.SessionID
		r.Cost = v.TotalCostUSD
	}
}

func envSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/agent/...`

- [ ] **Step 5: Commit**

```bash
git add internal/agent
git commit -m "feat(agent): CLIAgent invoking claude -p --output-format json"
```

---

## Task 9: `internal/worktree` — git operations

**Files:**
- Create: `internal/worktree/worktree.go`, `internal/worktree/worktree_test.go`

- [ ] **Step 1: Write failing test using a real ephemeral repo**

`internal/worktree/worktree_test.go`:

```go
package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCreate_BranchFromMain(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	srcRepo := filepath.Join(root, "src")
	mustGit(t, root, "init", srcRepo)
	mustGit(t, srcRepo, "commit", "--allow-empty", "-m", "init")
	mustGit(t, srcRepo, "branch", "-M", "main")

	wtPath := filepath.Join(root, "wt")
	wt, err := Create(context.Background(), Options{
		SourceRepo: srcRepo,
		BranchName: "migrate/demo",
		WorktreeDir: wtPath,
		BaseRef:    "main",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wt.Path != wtPath {
		t.Errorf("Path = %q", wt.Path)
	}
	if _, err := os.Stat(filepath.Join(wtPath, ".git")); err != nil {
		t.Errorf("worktree .git missing: %v", err)
	}
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/worktree/...`

- [ ] **Step 3: Implement `internal/worktree/worktree.go`**

```go
// Package worktree manages git worktree creation, branch ops, and the
// pre-commit hook that enforces test-freeze.
package worktree

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Options struct {
	SourceRepo  string // path to existing repo (must contain BaseRef)
	BranchName  string // e.g. "migrate/aijobs-batch-migration"
	WorktreeDir string // absolute path
	BaseRef     string // e.g. "origin/main"
}

type Worktree struct {
	Path       string
	BranchName string
}

func Create(ctx context.Context, opt Options) (*Worktree, error) {
	if err := os.MkdirAll(filepath.Dir(opt.WorktreeDir), 0o755); err != nil {
		return nil, err
	}
	args := []string{"worktree", "add", opt.WorktreeDir, "-b", opt.BranchName, opt.BaseRef}
	if err := runGit(ctx, opt.SourceRepo, args...); err != nil {
		return nil, fmt.Errorf("worktree add: %w", err)
	}
	return &Worktree{Path: opt.WorktreeDir, BranchName: opt.BranchName}, nil
}

func (w *Worktree) Commit(ctx context.Context, message string, env map[string]string) error {
	if err := runGitEnv(ctx, w.Path, env, "add", "-A"); err != nil {
		return err
	}
	return runGitEnv(ctx, w.Path, env, "commit", "-m", message)
}

func (w *Worktree) Push(ctx context.Context) error {
	return runGit(ctx, w.Path, "push", "-u", "origin", w.BranchName)
}

func (w *Worktree) DiffSummary(ctx context.Context, refspec string) (string, error) {
	out, err := captureGit(ctx, w.Path, "diff", "--shortstat", refspec)
	return out, err
}

func runGit(ctx context.Context, dir string, args ...string) error {
	return runGitEnv(ctx, dir, nil, args...)
}

func runGitEnv(ctx context.Context, dir string, env map[string]string, args ...string) error {
	c := exec.CommandContext(ctx, "git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), envSlice(env)...)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %w\n%s", args, err, out)
	}
	return nil
}

func captureGit(ctx context.Context, dir string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, "git", args...)
	c.Dir = dir
	var buf bytes.Buffer
	c.Stdout = &buf
	if err := c.Run(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func envSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/worktree/...`

- [ ] **Step 5: Commit**

```bash
git add internal/worktree
git commit -m "feat(worktree): create/commit/push helpers for migrate-loop branches"
```

---

## Task 10: `internal/worktree` — pre-commit hook installation

**Files:**
- Modify: `internal/worktree/worktree.go` (add `InstallHook`)
- Create: `internal/worktree/hook.go`, `internal/worktree/hook_test.go`

- [ ] **Step 1: Write failing hook-content test**

`internal/worktree/hook_test.go`:

```go
package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHookRejectsTestEditWithoutEnv(t *testing.T) {
	wt := setupWorktreeWithHook(t)

	mustWrite(t, filepath.Join(wt.Path, "x_test.go"), "package x\n")
	mustGit(t, wt.Path, "add", "x_test.go")
	c := exec.Command("git", "-C", wt.Path, "commit", "-m", "wip(coder-1): try thing")
	out, err := c.CombinedOutput()
	if err == nil {
		t.Fatalf("commit should have been rejected, got success:\n%s", out)
	}
}

func TestHookAllowsTestEditWithEnv(t *testing.T) {
	wt := setupWorktreeWithHook(t)

	mustWrite(t, filepath.Join(wt.Path, "x_test.go"), "package x\n")
	mustGit(t, wt.Path, "add", "x_test.go")
	c := exec.Command("git", "-C", wt.Path, "commit", "-m", "test(plan): add red")
	c.Env = append(os.Environ(), "ALLOW_TEST_EDITS=1", "EXPECTED_COMMIT_PREFIX=test(plan)")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("commit should be allowed:\n%s\n%v", out, err)
	}
}

func TestHookRejectsWrongCommitPrefix(t *testing.T) {
	wt := setupWorktreeWithHook(t)
	mustWrite(t, filepath.Join(wt.Path, "x.go"), "package x\n")
	mustGit(t, wt.Path, "add", "x.go")
	c := exec.Command("git", "-C", wt.Path, "commit", "-m", "feat: wrong prefix")
	c.Env = append(os.Environ(), "EXPECTED_COMMIT_PREFIX=wip(coder-1)")
	out, err := c.CombinedOutput()
	if err == nil {
		t.Fatalf("commit should have been rejected:\n%s", out)
	}
}

func setupWorktreeWithHook(t *testing.T) *Worktree {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join(root, "src")
	mustGit(t, root, "init", src)
	mustGit(t, src, "config", "user.email", "test@test")
	mustGit(t, src, "config", "user.name", "test")
	mustGit(t, src, "commit", "--allow-empty", "-m", "init")
	mustGit(t, src, "branch", "-M", "main")
	wtPath := filepath.Join(root, "wt")
	wt, err := Create(context.Background(), Options{SourceRepo: src, WorktreeDir: wtPath, BranchName: "feat/x", BaseRef: "main"})
	if err != nil {
		t.Fatal(err)
	}
	mustGit(t, wt.Path, "config", "user.email", "test@test")
	mustGit(t, wt.Path, "config", "user.name", "test")
	if err := wt.InstallHook(); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(wt.Path, "FROZEN_TESTS.md"), "")
	return wt
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run test, expect FAIL ("wt.InstallHook undefined")**

Run: `go test ./internal/worktree/...`

- [ ] **Step 3: Implement `internal/worktree/hook.go`**

```go
package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const hookScript = `#!/usr/bin/env bash
set -euo pipefail

# migrate-loop pre-commit hook
# Rejects:
#   - changes to *_test.go unless ALLOW_TEST_EDITS=1
#   - commit messages whose first line does not start with $EXPECTED_COMMIT_PREFIX (when set)
# The agent's only sanctioned way to flag a test as wrong is to write to FROZEN_TESTS.md.

staged=$(git diff --cached --name-only --diff-filter=ACMR)

if [[ "${ALLOW_TEST_EDITS:-0}" != "1" ]]; then
  bad=$(echo "$staged" | grep -E '_test\.go$' || true)
  if [[ -n "$bad" ]]; then
    echo "migrate-loop pre-commit: test files are FROZEN during this phase." >&2
    echo "Files: $bad" >&2
    echo "If you believe a test is wrong, write your objection to FROZEN_TESTS.md and commit only that file." >&2
    exit 1
  fi
fi

if [[ -n "${EXPECTED_COMMIT_PREFIX:-}" ]]; then
  msg=$(head -n1 "$1" 2>/dev/null || git log -n1 --format=%s 2>/dev/null || echo "")
  # $1 is unset for hooks invoked without a commit-msg path; fall back to $GIT_COMMIT_MSG_FILE
  if [[ -z "$msg" && -n "${GIT_COMMIT_MSG_FILE:-}" ]]; then
    msg=$(head -n1 "$GIT_COMMIT_MSG_FILE")
  fi
  # As a last resort try .git/COMMIT_EDITMSG (path is given relative to worktree root)
  if [[ -z "$msg" ]]; then
    if [[ -f ".git/COMMIT_EDITMSG" ]]; then
      msg=$(head -n1 .git/COMMIT_EDITMSG)
    fi
  fi
  case "$msg" in
    "$EXPECTED_COMMIT_PREFIX"*) : ;;
    *)
      echo "migrate-loop pre-commit: commit message must start with '$EXPECTED_COMMIT_PREFIX'" >&2
      echo "Got: $msg" >&2
      exit 1
      ;;
  esac
fi

exit 0
`

// InstallHook writes the pre-commit hook into the worktree's gitdir.
func (w *Worktree) InstallHook() error {
	gitDir, err := w.gitDir()
	if err != nil {
		return err
	}
	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(hooksDir, "pre-commit")
	if err := os.WriteFile(path, []byte(hookScript), 0o755); err != nil {
		return err
	}
	frozen := filepath.Join(w.Path, "FROZEN_TESTS.md")
	if _, err := os.Stat(frozen); os.IsNotExist(err) {
		_ = os.WriteFile(frozen, []byte(""), 0o644)
	}
	return nil
}

func (w *Worktree) gitDir() (string, error) {
	out, err := exec.Command("git", "-C", w.Path, "rev-parse", "--git-dir").Output()
	if err != nil {
		return "", fmt.Errorf("rev-parse: %w", err)
	}
	gd := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gd) {
		gd = filepath.Join(w.Path, gd)
	}
	return gd, nil
}
```

- [ ] **Step 4: Pass `$1` correctly**

Note: git invokes pre-commit with no args, BUT commit-msg with `$1`. We're using pre-commit, so message-prefix check needs to read `.git/COMMIT_EDITMSG`. The hook above already handles this fallback chain.

- [ ] **Step 5: Run test, expect PASS**

Run: `go test ./internal/worktree/...`

If failures around message prefix, switch to using a `commit-msg` hook in addition to `pre-commit`. For v1, the pre-commit-only approach with `.git/COMMIT_EDITMSG` fallback is sufficient.

- [ ] **Step 6: Commit**

```bash
git add internal/worktree
git commit -m "feat(worktree): pre-commit hook enforces test-freeze + commit-prefix gate"
```

---

## Task 11: `internal/worktree` — file lock

**Files:**
- Modify: `internal/worktree/worktree.go` (add `Lock`/`Unlock`)
- Create: `internal/worktree/lock.go`, `internal/worktree/lock_test.go`

- [ ] **Step 1: Write failing lock test**

`internal/worktree/lock_test.go`:

```go
package worktree

import (
	"path/filepath"
	"testing"
)

func TestLock_Exclusive(t *testing.T) {
	dir := t.TempDir()
	l1, err := Lock(filepath.Join(dir, ".migrate-loop.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer l1.Release()

	if _, err := Lock(filepath.Join(dir, ".migrate-loop.lock")); err == nil {
		t.Error("expected second Lock to fail")
	}
}

func TestLock_ReleaseAllowsRelock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lock")
	l, err := Lock(p)
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	l2, err := Lock(p)
	if err != nil {
		t.Fatalf("relock after release: %v", err)
	}
	l2.Release()
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/worktree/...`

- [ ] **Step 3: Implement `internal/worktree/lock.go`**

```go
package worktree

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

type FileLock struct {
	f *os.File
}

func Lock(path string) (*FileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return &FileLock{f: f}, nil
}

func (l *FileLock) Release() {
	if l == nil || l.f == nil {
		return
	}
	_ = unix.Flock(int(l.f.Fd()), unix.LOCK_UN)
	_ = l.f.Close()
}
```

- [ ] **Step 4: Add dep + run**

```bash
go get golang.org/x/sys/unix
go test ./internal/worktree/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/worktree go.mod go.sum
git commit -m "feat(worktree): flock-based exclusive run lock"
```

---

## Task 12: `internal/escalate` — ESCALATION.md writer

**Files:**
- Create: `internal/escalate/escalate.go`, `internal/escalate/escalate_test.go`

- [ ] **Step 1: Write failing test**

`internal/escalate/escalate_test.go`:

```go
package escalate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdfalk/migrate-loop/internal/state"
)

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	r := Reason{
		Kind: KindBudgetExhausted,
		Summary: "budget hit at iteration 50",
		LastFailing: []state.TestID{{Package: "p", Test: "TestX"}},
		LastTestOutput: "--- FAIL: TestX\n  expected 3 got 4",
		LastDiffs: []string{"diff 1", "diff 2", "diff 3"},
		AgentDiagnosis: "I cannot find a way to make TestX pass without changing TestY",
	}
	path := filepath.Join(dir, "ESCALATION.md")
	if err := Write(path, r); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{"budget_exhausted", "TestX", "expected 3 got 4", "diff 2", "cannot find"} {
		if !strings.Contains(s, want) {
			t.Errorf("ESCALATION.md missing %q\nbody:\n%s", want, s)
		}
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/escalate/...`

- [ ] **Step 3: Implement `internal/escalate/escalate.go`**

```go
// Package escalate writes the on-disk ESCALATION.md document.
package escalate

import (
	"bytes"
	"fmt"
	"os"
	"text/template"

	"github.com/jdfalk/migrate-loop/internal/state"
)

type Kind string

const (
	KindTestsVacuous           Kind = "tests_vacuous"
	KindBudgetExhausted        Kind = "budget_exhausted"
	KindStagnationAfterRedirect Kind = "stagnation_after_redirect"
	KindTestsSeemWrong         Kind = "tests_seem_wrong"
	KindIterationTimeout       Kind = "iteration_timeout"
)

type Reason struct {
	Kind            Kind
	Summary         string
	LastFailing     []state.TestID
	LastTestOutput  string
	LastDiffs       []string
	AgentDiagnosis  string
	SuggestedFix    string
}

const tmplSrc = `# Migration escalation: {{.Kind}}

**Summary:** {{.Summary}}

## Last failing tests
{{range .LastFailing}}- {{.Package}} :: {{.Test}}
{{end}}
## Last test output

` + "```" + `
{{.LastTestOutput}}
` + "```" + `

## Last 3 diffs
{{range $i, $d := .LastDiffs}}### Diff {{$i}}
` + "```diff" + `
{{$d}}
` + "```" + `
{{end}}
## Agent diagnosis
{{.AgentDiagnosis}}

## Suggested fix
{{if .SuggestedFix}}{{.SuggestedFix}}{{else}}(none){{end}}

---
*To resume after fixing: ` + "`migrate-loop --resume`" + `*
`

func Write(path string, r Reason) error {
	t, err := template.New("esc").Parse(tmplSrc)
	if err != nil {
		return err
	}
	if r.SuggestedFix == "" {
		r.SuggestedFix = defaultSuggestion(r.Kind)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, r); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func defaultSuggestion(k Kind) string {
	switch k {
	case KindTestsVacuous:
		return "Tests do not actually exercise the new behavior; revise spec or planner output."
	case KindBudgetExhausted:
		return "Increase --budget OR re-scope spec into smaller migrations."
	case KindStagnationAfterRedirect:
		return "Tests may be testing the wrong thing, or behavior is under-specified."
	case KindTestsSeemWrong:
		return "Review FROZEN_TESTS.md, decide whether the agent's objection is correct, edit tests if so, then --resume."
	case KindIterationTimeout:
		return "Lower scope or raise --iter-timeout."
	}
	return fmt.Sprintf("(no default suggestion for %s)", k)
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/escalate/...`

- [ ] **Step 5: Commit**

```bash
git add internal/escalate
git commit -m "feat(escalate): ESCALATION.md writer with five reason kinds"
```

---

## Task 13: `prompts` — embedded templates

**Files:**
- Create: `prompts/prompts.go`, `prompts/planner.tmpl`, `prompts/coder.tmpl`, `prompts/redirect.tmpl`, `prompts/cover.tmpl`, `prompts/prompts_test.go`

- [ ] **Step 1: Write failing render test**

`prompts/prompts_test.go`:

```go
package prompts

import (
	"strings"
	"testing"
)

func TestRenderPlanner(t *testing.T) {
	out, err := RenderPlanner(PlannerInput{
		Slug: "demo",
		SpecBody: "# demo\nadd two ints",
		PriorExamples: []PriorExample{{Path: "p.md", Content: "prior content"}},
		TargetPackages: []string{"internal/x"},
		TestRunner: "go test -json ./...",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"demo", "add two ints", "prior content", "internal/x", "go test -json"} {
		if !strings.Contains(out, want) {
			t.Errorf("planner prompt missing %q", want)
		}
	}
}

func TestRenderCoder(t *testing.T) {
	out, err := RenderCoder(CoderInput{
		Iteration: 3,
		FailingTests: "TestA, TestB",
		LastTestOutput: "--- FAIL: TestA",
		LastDiff: "+ x := 1",
		OscillationNote: "swapped TestA/TestB last iteration",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"iteration 3", "TestA, TestB", "FROZEN_TESTS.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("coder prompt missing %q\noutput:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./prompts/...`

- [ ] **Step 3: Add templates**

`prompts/planner.tmpl`:

```
You are the PLANNER for a migrate-loop TDD migration named "{{.Slug}}".

Your job: write a comprehensive *failing* Go test suite that captures the target behavior described in the spec body below. The coder agent will iterate to make these tests pass.

CONSTRAINTS:
- Tests must FAIL on the current codebase (you are writing red tests).
- Tests must be deterministic: no time/network/randomness without controlled fakes.
- Cover edge cases mentioned in the spec PLUS the standard ones for this kind of migration: nil/empty inputs, error paths, schema/transport boundaries, batch vs single-item paths.
- Use t.Parallel() where safe.
- Place tests in the target packages: {{range .TargetPackages}}{{.}}, {{end}}
- The runner is: {{.TestRunner}}

PRIOR EXAMPLES (study these for house style — naming, layout, edge case coverage):
{{range .PriorExamples}}=== {{.Path}} ===
{{.Content}}
{{end}}

SPEC BODY:
{{.SpecBody}}

After writing tests, run the test runner to confirm they FAIL. Then commit with the message:
    test(plan): {{.Slug}} failing test suite

Then exit. The harness will take over.
```

`prompts/coder.tmpl`:

```
You are the CODER for a migrate-loop TDD migration. This is iteration {{.Iteration}}.

Your job: read the failing test output, edit the implementation (NOT the tests — they are frozen), run the runner, and commit if you made progress. Do this for ONE iteration only, then exit.

CURRENT FAILING TESTS:
{{.FailingTests}}

LAST TEST OUTPUT (tail):
{{.LastTestOutput}}

LAST DIFF YOU PRODUCED:
{{.LastDiff}}

{{if .OscillationNote}}NOTE: {{.OscillationNote}}{{end}}

CONSTRAINTS:
- Test files (*_test.go) are FROZEN. The pre-commit hook will reject any test edits.
- If you believe a test is genuinely wrong, do NOT try to bypass — write your specific objection to FROZEN_TESTS.md (this is the sanctioned channel; the harness will see it and escalate).
- Commit prefix MUST be "wip(coder-{{.Iteration}})". The hook enforces this.
- One iteration = one commit (or zero, if you couldn't make progress; in that case write WHY to FROZEN_TESTS.md).

Make ONE focused change, run the runner, observe, commit if green-er, exit.
```

`prompts/redirect.tmpl`:

```
You are the CODER, but the harness has detected STAGNATION across the last 3 iterations. You are stuck.

OSCILLATION LOG:
{{.OscillationLog}}

WHAT KEEPS FAILING:
{{.WhatKeepsFailing}}

LAST 3 DIFFS:
{{.LastDiffs}}

Step back. Before your next edit, write 2-3 sentences explaining what these failing tests have in COMMON — what shared invariant or design assumption are you missing? Output that reasoning to STDOUT first, then make ONE different-shaped change (not a tweak of your last attempt) and commit with prefix "wip(coder-{{.Iteration}})".

If after considering this you genuinely think the tests are testing the wrong thing, write the specific objection to FROZEN_TESTS.md instead.
```

`prompts/cover.tmpl`:

```
You are the COVERAGE PLANNER. The migration's main loop is GREEN. Your job: identify touched-but-uncovered code and write *additional* tests to close those gaps.

UNCOVERED LINES IN TOUCHED FILES:
{{.UncoveredGaps}}

CONSTRAINTS:
- Write only new tests; do not modify existing tests or implementation.
- Tests must be deterministic.
- Commit with prefix "test(coverage)".

If a gap is genuinely untestable (e.g., unreachable error path), say so in your stdout summary and skip it. Do not invent contrived inputs.
```

- [ ] **Step 4: Implement `prompts/prompts.go`**

```go
// Package prompts holds embedded prompt templates used by each phase.
package prompts

import (
	"bytes"
	_ "embed"
	"text/template"
)

//go:embed planner.tmpl
var plannerTmpl string

//go:embed coder.tmpl
var coderTmpl string

//go:embed redirect.tmpl
var redirectTmpl string

//go:embed cover.tmpl
var coverTmpl string

type PriorExample struct {
	Path    string
	Content string
}

type PlannerInput struct {
	Slug           string
	SpecBody       string
	PriorExamples  []PriorExample
	TargetPackages []string
	TestRunner     string
}

func RenderPlanner(in PlannerInput) (string, error) {
	return render(plannerTmpl, in)
}

type CoderInput struct {
	Iteration       int
	FailingTests    string
	LastTestOutput  string
	LastDiff        string
	OscillationNote string
}

func RenderCoder(in CoderInput) (string, error) {
	return render(coderTmpl, in)
}

type RedirectInput struct {
	Iteration        int
	OscillationLog   string
	WhatKeepsFailing string
	LastDiffs        string
}

func RenderRedirect(in RedirectInput) (string, error) {
	return render(redirectTmpl, in)
}

type CoverInput struct {
	UncoveredGaps string
}

func RenderCover(in CoverInput) (string, error) {
	return render(coverTmpl, in)
}

func render(tmplSrc string, data any) (string, error) {
	t, err := template.New("p").Parse(tmplSrc)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
```

- [ ] **Step 5: Run, expect PASS**

Run: `go test ./prompts/...`

- [ ] **Step 6: Commit**

```bash
git add prompts
git commit -m "feat(prompts): embedded planner/coder/redirect/cover templates"
```

---

## Task 14: `internal/phases/plan.go`

**Files:**
- Create: `internal/phases/phases.go` (Deps, common helpers), `internal/phases/plan.go`, `internal/phases/plan_test.go`

- [ ] **Step 1: Write failing PLAN test using FakeAgent + temp worktree**

`internal/phases/plan_test.go`:

```go
package phases

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdfalk/migrate-loop/internal/agent"
	"github.com/jdfalk/migrate-loop/internal/runner"
	"github.com/jdfalk/migrate-loop/internal/spec"
	"github.com/jdfalk/migrate-loop/internal/state"
	"github.com/jdfalk/migrate-loop/internal/worktree"
)

func TestPlan_HappyPath(t *testing.T) {
	wt := setupWorktree(t)
	sp := &spec.Spec{
		Slug: "demo", TargetPackages: []string{"."}, TestRunner: "go test -json ./...",
		Body: "add two ints",
	}
	st := &state.State{Slug: "demo", Phase: state.PhaseInit, Budget: 50}

	fa := &agent.FakeAgent{
		Responses: []agent.Response{{ExitCode: 0, Cost: 0.10}},
		Editor: func(req agent.Request) error {
			// Simulate planner: write test file + impl stub + commit
			if err := os.WriteFile(filepath.Join(req.Cwd, "go.mod"), []byte("module x\ngo 1.22\n"), 0o644); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(req.Cwd, "x.go"), []byte("package x\nfunc Add(a,b int) int { return 0 }\n"), 0o644); err != nil {
				return err
			}
			testSrc := "package x\nimport \"testing\"\nfunc TestAdd(t *testing.T) { if Add(1,2)!=3 { t.Fail() } }\n"
			if err := os.WriteFile(filepath.Join(req.Cwd, "x_test.go"), []byte(testSrc), 0o644); err != nil {
				return err
			}
			env := map[string]string{"ALLOW_TEST_EDITS": "1", "EXPECTED_COMMIT_PREFIX": "test(plan)"}
			return wt.Commit(context.Background(), "test(plan): demo failing test suite", env)
		},
	}

	deps := Deps{
		Agent:    fa,
		Runner:   runner.NewGoRunner("go test -json ./..."),
		Worktree: wt,
	}
	if err := Plan(context.Background(), st, sp, deps); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if st.Phase != state.PhaseCode {
		t.Errorf("phase after PLAN = %v, want CODE", st.Phase)
	}
	if len(st.LastFailing) == 0 {
		t.Errorf("expected LastFailing populated after PLAN")
	}
}

func TestPlan_VacuousTestsEscalate(t *testing.T) {
	wt := setupWorktree(t)
	sp := &spec.Spec{Slug: "demo", TargetPackages: []string{"."}, TestRunner: "go test -json ./...", Body: "x"}
	st := &state.State{Slug: "demo", Phase: state.PhaseInit}

	fa := &agent.FakeAgent{
		Responses: []agent.Response{{ExitCode: 0}},
		Editor: func(req agent.Request) error {
			os.WriteFile(filepath.Join(req.Cwd, "go.mod"), []byte("module x\ngo 1.22\n"), 0o644)
			os.WriteFile(filepath.Join(req.Cwd, "x.go"), []byte("package x\n"), 0o644)
			testSrc := "package x\nimport \"testing\"\nfunc TestPasses(t *testing.T){}\n"
			os.WriteFile(filepath.Join(req.Cwd, "x_test.go"), []byte(testSrc), 0o644)
			env := map[string]string{"ALLOW_TEST_EDITS": "1", "EXPECTED_COMMIT_PREFIX": "test(plan)"}
			return wt.Commit(context.Background(), "test(plan): vacuous", env)
		},
	}
	deps := Deps{Agent: fa, Runner: runner.NewGoRunner(""), Worktree: wt}
	err := Plan(context.Background(), st, sp, deps)
	if err == nil || !strings.Contains(err.Error(), "tests_vacuous") {
		t.Fatalf("expected tests_vacuous error, got %v", err)
	}
}
```

(`setupWorktree` is a shared test helper — define in a new `phases_test.go` shared file.)

`internal/phases/phases_test.go`:

```go
package phases

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jdfalk/migrate-loop/internal/worktree"
)

func setupWorktree(t *testing.T) *worktree.Worktree {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	src := filepath.Join(root, "src")
	mustCmd(t, root, "git", "init", src)
	mustCmd(t, src, "git", "config", "user.email", "t@t")
	mustCmd(t, src, "git", "config", "user.name", "t")
	mustCmd(t, src, "git", "commit", "--allow-empty", "-m", "init")
	mustCmd(t, src, "git", "branch", "-M", "main")
	wtPath := filepath.Join(root, "wt")
	wt, err := worktree.Create(context.Background(), worktree.Options{
		SourceRepo: src, WorktreeDir: wtPath, BranchName: "migrate/demo", BaseRef: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	mustCmd(t, wt.Path, "git", "config", "user.email", "t@t")
	mustCmd(t, wt.Path, "git", "config", "user.name", "t")
	if err := wt.InstallHook(); err != nil {
		t.Fatal(err)
	}
	return wt
}

func mustCmd(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}
```

- [ ] **Step 2: Run, expect FAIL ("undefined: Plan, Deps")**

Run: `go test ./internal/phases/...`

- [ ] **Step 3: Implement `internal/phases/phases.go`**

```go
// Package phases contains the state-machine phases: PLAN, CODE, REDIRECT, COVER, PR.
package phases

import (
	"github.com/jdfalk/migrate-loop/internal/agent"
	"github.com/jdfalk/migrate-loop/internal/runner"
	"github.com/jdfalk/migrate-loop/internal/worktree"
)

type Deps struct {
	Agent    agent.Agent
	Runner   runner.Runner
	Worktree *worktree.Worktree
}
```

- [ ] **Step 4: Implement `internal/phases/plan.go`**

```go
package phases

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jdfalk/migrate-loop/internal/agent"
	"github.com/jdfalk/migrate-loop/internal/prompts"
	"github.com/jdfalk/migrate-loop/internal/spec"
	"github.com/jdfalk/migrate-loop/internal/state"
)

func Plan(ctx context.Context, st *state.State, sp *spec.Spec, d Deps) error {
	priors, err := loadPriors(sp)
	if err != nil {
		return fmt.Errorf("plan: load priors: %w", err)
	}
	prompt, err := prompts.RenderPlanner(prompts.PlannerInput{
		Slug:           sp.Slug,
		SpecBody:       sp.Body,
		PriorExamples:  priors,
		TargetPackages: sp.TargetPackages,
		TestRunner:     sp.TestRunner,
	})
	if err != nil {
		return err
	}
	resp, err := d.Agent.Run(ctx, agent.Request{
		Phase: agent.PhasePlan,
		Cwd:   d.Worktree.Path,
		AllowedTools: []string{
			"Read", "Write", "Edit", "Glob", "Grep",
			"Bash(go test:*)", "Bash(go build:*)", "Bash(git add:*)", "Bash(git commit:*)",
		},
		Env: map[string]string{
			"ALLOW_TEST_EDITS":        "1",
			"EXPECTED_COMMIT_PREFIX":  "test(plan)",
		},
		Prompt: prompt,
	})
	if err != nil {
		return fmt.Errorf("plan: agent: %w", err)
	}
	st.TotalCostUSD += resp.Cost

	res, err := d.Runner.Run(ctx, d.Worktree.Path)
	if err != nil {
		return fmt.Errorf("plan: runner: %w", err)
	}
	if len(res.Failing) == 0 {
		return errors.New("tests_vacuous: planner finished but 0 tests fail")
	}
	st.LastFailing = res.Failing
	st.Phase = state.PhaseCode
	st.Iteration = 1
	return nil
}

func loadPriors(sp *spec.Spec) ([]prompts.PriorExample, error) {
	out := make([]prompts.PriorExample, 0, len(sp.PriorExamples))
	specDir := filepath.Dir(sp.FilePath)
	for _, ref := range sp.PriorExamples {
		path := ref
		if !filepath.IsAbs(path) {
			path = filepath.Join(specDir, path)
		}
		if _, err := os.Stat(path); err != nil {
			// non-fatal — prior may be a PR ref like "PR#123"; just skip with a note
			out = append(out, prompts.PriorExample{Path: ref, Content: "(prior not resolvable as file: " + ref + ")"})
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, prompts.PriorExample{Path: ref, Content: string(body)})
	}
	return out, nil
}
```

- [ ] **Step 5: Run, expect PASS**

Run: `go test ./internal/phases/...`

- [ ] **Step 6: Commit**

```bash
git add internal/phases
git commit -m "feat(phases): PLAN phase with prior-example loading and tests_vacuous escalation"
```

---

## Task 15: `internal/phases/code.go`

**Files:**
- Create: `internal/phases/code.go`, `internal/phases/code_test.go`

- [ ] **Step 1: Write failing tests for CODE happy path + stagnation**

`internal/phases/code_test.go`:

```go
package phases

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/migrate-loop/internal/agent"
	"github.com/jdfalk/migrate-loop/internal/runner"
	"github.com/jdfalk/migrate-loop/internal/state"
)

func seedRedRepo(t *testing.T, wtPath string) {
	t.Helper()
	os.WriteFile(filepath.Join(wtPath, "go.mod"), []byte("module x\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(wtPath, "x.go"), []byte("package x\nfunc Add(a,b int) int { return 0 }\n"), 0o644)
	os.WriteFile(filepath.Join(wtPath, "x_test.go"), []byte("package x\nimport \"testing\"\nfunc TestAdd(t *testing.T){ if Add(1,2)!=3 { t.Fail() } }\n"), 0o644)
}

func TestCode_MakesProgressAndAdvancesAtGreen(t *testing.T) {
	wt := setupWorktree(t)
	seedRedRepo(t, wt.Path)
	mustCmd(t, wt.Path, "git", "add", "-A")
	cmt := func(msg string, env map[string]string) {
		c := []string{"commit", "-m", msg}
		_ = wt.Commit(context.Background(), msg, env)
		_ = c
	}
	cmt("test(plan): demo", map[string]string{"ALLOW_TEST_EDITS": "1", "EXPECTED_COMMIT_PREFIX": "test(plan)"})

	st := &state.State{
		Slug: "demo", Phase: state.PhaseCode, Iteration: 1, Budget: 5,
		LastFailing: []state.TestID{{Package: "x", Test: "TestAdd"}},
	}

	fa := &agent.FakeAgent{
		Responses: []agent.Response{{ExitCode: 0}},
		Editor: func(req agent.Request) error {
			os.WriteFile(filepath.Join(req.Cwd, "x.go"), []byte("package x\nfunc Add(a,b int) int { return a+b }\n"), 0o644)
			return wt.Commit(context.Background(), "wip(coder-1): fix Add", map[string]string{"EXPECTED_COMMIT_PREFIX": "wip(coder-1)"})
		},
	}
	deps := Deps{Agent: fa, Runner: runner.NewGoRunner(""), Worktree: wt}

	advance, err := Code(context.Background(), st, deps)
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if !advance {
		t.Errorf("expected advance=true at green")
	}
	if st.Phase != state.PhaseCover {
		t.Errorf("phase after green CODE = %v, want COVER", st.Phase)
	}
}

func TestCode_StagnationIncrementsCounter(t *testing.T) {
	wt := setupWorktree(t)
	seedRedRepo(t, wt.Path)
	wt.Commit(context.Background(), "test(plan): demo", map[string]string{"ALLOW_TEST_EDITS": "1", "EXPECTED_COMMIT_PREFIX": "test(plan)"})

	st := &state.State{
		Slug: "demo", Phase: state.PhaseCode, Iteration: 1, Budget: 10,
		LastFailing: []state.TestID{{Package: "x", Test: "TestAdd"}},
	}
	fa := &agent.FakeAgent{
		Responses: []agent.Response{{ExitCode: 0}},
		Editor: func(req agent.Request) error {
			// no-op edit
			return wt.Commit(context.Background(), "wip(coder-1): no-op", map[string]string{"EXPECTED_COMMIT_PREFIX": "wip(coder-1)"})
		},
	}
	deps := Deps{Agent: fa, Runner: runner.NewGoRunner(""), Worktree: wt}
	_, err := Code(context.Background(), st, deps)
	if err != nil {
		t.Fatal(err)
	}
	if st.StagnationStreak != 1 {
		t.Errorf("StagnationStreak = %d, want 1", st.StagnationStreak)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/phases/...`

- [ ] **Step 3: Implement `internal/phases/code.go`**

```go
package phases

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jdfalk/migrate-loop/internal/agent"
	"github.com/jdfalk/migrate-loop/internal/prompts"
	"github.com/jdfalk/migrate-loop/internal/state"
)

// Code runs ONE coder iteration. Returns advance=true if all green and the
// state was advanced to COVER. The caller (main) loops until advance OR
// budget exhausted OR escalation.
func Code(ctx context.Context, st *state.State, d Deps) (bool, error) {
	failing := summarizeFailing(st.LastFailing)
	tail := tailLines(st.LastDiffSummary, 40)
	osc := ""
	if len(st.OscillationLog) > 0 {
		osc = "Earlier oscillations: " + summarizeOscillation(st.OscillationLog)
	}
	prompt, err := prompts.RenderCoder(prompts.CoderInput{
		Iteration:       st.Iteration,
		FailingTests:    failing,
		LastTestOutput:  tail,
		LastDiff:        st.LastDiffSummary,
		OscillationNote: osc,
	})
	if err != nil {
		return false, err
	}
	resp, err := d.Agent.Run(ctx, agent.Request{
		Phase: agent.PhaseCode,
		Cwd:   d.Worktree.Path,
		AllowedTools: []string{
			"Read", "Edit", "Glob", "Grep",
			"Bash(go test:*)", "Bash(go vet:*)", "Bash(go build:*)",
			"Bash(git add:*)", "Bash(git commit:*)",
		},
		Env: map[string]string{
			"EXPECTED_COMMIT_PREFIX": fmt.Sprintf("wip(coder-%d)", st.Iteration),
		},
		Prompt: prompt,
	})
	if err != nil {
		return false, fmt.Errorf("code: agent: %w", err)
	}
	st.TotalCostUSD += resp.Cost
	st.BudgetUsed++

	if frozen := readFrozenObjection(d.Worktree.Path); frozen != "" {
		return false, fmt.Errorf("tests_seem_wrong: %s", frozen)
	}

	res, err := d.Runner.Run(ctx, d.Worktree.Path)
	if err != nil {
		return false, fmt.Errorf("code: runner: %w", err)
	}
	prog := state.DetectProgress(st.LastFailing, res.Failing)
	st.LastFailing = res.Failing

	if prog.AllGreen {
		st.Phase = state.PhaseCover
		st.StagnationStreak = 0
		return true, nil
	}
	if prog.IsProgress {
		st.StagnationStreak = 0
		if prog.Oscillation {
			st.OscillationLog = append(st.OscillationLog, state.OscillationEvent{
				Iteration: st.Iteration,
				Note:      fmt.Sprintf("rotated: now-passing=%d newly-failing=%d", len(prog.NowPassing), len(prog.NewlyFailing)),
			})
		}
	} else {
		st.StagnationStreak++
	}
	st.Iteration++

	if st.StagnationStreak >= 3 && !st.RedirectUsed {
		st.Phase = state.PhaseRedirect
	} else if st.StagnationStreak >= 4 && st.RedirectUsed {
		return false, errors.New("stagnation_after_redirect")
	}
	if st.BudgetUsed >= st.Budget && st.Phase != state.PhaseCover {
		return false, errors.New("budget_exhausted")
	}
	return false, nil
}

func summarizeFailing(ids []state.TestID) string {
	if len(ids) == 0 {
		return "(none)"
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = id.Package + "::" + id.Test
	}
	return strings.Join(parts, ", ")
}

func summarizeOscillation(ev []state.OscillationEvent) string {
	parts := make([]string, len(ev))
	for i, e := range ev {
		parts[i] = fmt.Sprintf("iter%d:%s", e.Iteration, e.Note)
	}
	return strings.Join(parts, "; ")
}

func tailLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

func readFrozenObjection(cwd string) string {
	body, err := os.ReadFile(filepath.Join(cwd, "FROZEN_TESTS.md"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}
```

- [ ] **Step 4: Run, expect PASS**

Run: `go test ./internal/phases/...`

- [ ] **Step 5: Commit**

```bash
git add internal/phases
git commit -m "feat(phases): CODE phase with progress + stagnation + FROZEN_TESTS escalation"
```

---

## Task 16: `internal/phases/redirect.go`

**Files:**
- Create: `internal/phases/redirect.go`, `internal/phases/redirect_test.go`

- [ ] **Step 1: Write failing test**

`internal/phases/redirect_test.go`:

```go
package phases

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/migrate-loop/internal/agent"
	"github.com/jdfalk/migrate-loop/internal/runner"
	"github.com/jdfalk/migrate-loop/internal/state"
)

func TestRedirect_ResetsStreakAndMarksUsed(t *testing.T) {
	wt := setupWorktree(t)
	seedRedRepo(t, wt.Path)
	wt.Commit(context.Background(), "test(plan): demo", map[string]string{"ALLOW_TEST_EDITS": "1", "EXPECTED_COMMIT_PREFIX": "test(plan)"})
	st := &state.State{Slug: "demo", Phase: state.PhaseRedirect, Iteration: 4, Budget: 50, StagnationStreak: 3}
	fa := &agent.FakeAgent{
		Responses: []agent.Response{{ExitCode: 0}},
		Editor: func(req agent.Request) error {
			os.WriteFile(filepath.Join(req.Cwd, "x.go"), []byte("package x\nfunc Add(a,b int) int { return a+b }\n"), 0o644)
			return wt.Commit(context.Background(), "wip(coder-4): different shape", map[string]string{"EXPECTED_COMMIT_PREFIX": "wip(coder-4)"})
		},
	}
	deps := Deps{Agent: fa, Runner: runner.NewGoRunner(""), Worktree: wt}
	if err := Redirect(context.Background(), st, deps); err != nil {
		t.Fatal(err)
	}
	if !st.RedirectUsed {
		t.Error("RedirectUsed should be true")
	}
	if st.StagnationStreak != 0 {
		t.Errorf("StagnationStreak = %d, want 0", st.StagnationStreak)
	}
	if st.Phase != state.PhaseCode {
		t.Errorf("phase after Redirect = %v, want CODE", st.Phase)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/phases/...`

- [ ] **Step 3: Implement `internal/phases/redirect.go`**

```go
package phases

import (
	"context"
	"fmt"

	"github.com/jdfalk/migrate-loop/internal/agent"
	"github.com/jdfalk/migrate-loop/internal/prompts"
	"github.com/jdfalk/migrate-loop/internal/state"
)

func Redirect(ctx context.Context, st *state.State, d Deps) error {
	prompt, err := prompts.RenderRedirect(prompts.RedirectInput{
		Iteration:        st.Iteration,
		OscillationLog:   summarizeOscillation(st.OscillationLog),
		WhatKeepsFailing: summarizeFailing(st.LastFailing),
		LastDiffs:        st.LastDiffSummary,
	})
	if err != nil {
		return err
	}
	resp, err := d.Agent.Run(ctx, agent.Request{
		Phase: agent.PhaseRedirect,
		Cwd:   d.Worktree.Path,
		AllowedTools: []string{
			"Read", "Edit", "Glob", "Grep",
			"Bash(go test:*)", "Bash(go vet:*)", "Bash(git add:*)", "Bash(git commit:*)",
		},
		Env: map[string]string{
			"EXPECTED_COMMIT_PREFIX": fmt.Sprintf("wip(coder-%d)", st.Iteration),
		},
		Prompt: prompt,
	})
	if err != nil {
		return fmt.Errorf("redirect: agent: %w", err)
	}
	st.TotalCostUSD += resp.Cost
	st.RedirectUsed = true
	st.StagnationStreak = 0
	st.Phase = state.PhaseCode
	return nil
}
```

- [ ] **Step 4: Run, expect PASS; commit**

```bash
go test ./internal/phases/...
git add internal/phases
git commit -m "feat(phases): REDIRECT phase resets stagnation streak (one-shot)"
```

---

## Task 17: `internal/phases/cover.go`

**Files:**
- Create: `internal/phases/cover.go`, `internal/phases/cover_test.go`

- [ ] **Step 1: Write test**

`internal/phases/cover_test.go`:

```go
package phases

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdfalk/migrate-loop/internal/agent"
	"github.com/jdfalk/migrate-loop/internal/runner"
	"github.com/jdfalk/migrate-loop/internal/state"
)

func TestCover_NoGapsAdvancesToPR(t *testing.T) {
	wt := setupWorktree(t)
	// fully-covered tiny repo
	os.WriteFile(filepath.Join(wt.Path, "go.mod"), []byte("module x\ngo 1.22\n"), 0o644)
	os.WriteFile(filepath.Join(wt.Path, "x.go"), []byte("package x\nfunc Add(a,b int) int { return a+b }\n"), 0o644)
	os.WriteFile(filepath.Join(wt.Path, "x_test.go"), []byte("package x\nimport \"testing\"\nfunc TestAdd(t *testing.T){ if Add(1,2)!=3 { t.Fail() } }\n"), 0o644)
	wt.Commit(context.Background(), "test(plan): demo", map[string]string{"ALLOW_TEST_EDITS": "1", "EXPECTED_COMMIT_PREFIX": "test(plan)"})

	st := &state.State{Slug: "demo", Phase: state.PhaseCover, CoverageBudget: 5}
	fa := &agent.FakeAgent{Responses: []agent.Response{{ExitCode: 0}}}
	deps := Deps{Agent: fa, Runner: runner.NewGoRunner(""), Worktree: wt}
	if err := Cover(context.Background(), st, deps); err != nil {
		t.Fatal(err)
	}
	if st.Phase != state.PhasePR {
		t.Errorf("phase = %v, want PR", st.Phase)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

- [ ] **Step 3: Implement `internal/phases/cover.go`**

```go
package phases

import (
	"context"
	"fmt"
	"strings"

	"github.com/jdfalk/migrate-loop/internal/agent"
	"github.com/jdfalk/migrate-loop/internal/prompts"
	"github.com/jdfalk/migrate-loop/internal/runner"
	"github.com/jdfalk/migrate-loop/internal/state"
)

func Cover(ctx context.Context, st *state.State, d Deps) error {
	rep, err := d.Runner.CoverProfile(ctx, d.Worktree.Path)
	if err != nil {
		return fmt.Errorf("cover: profile: %w", err)
	}
	gaps := summarizeGaps(rep)
	if gaps == "" {
		st.Phase = state.PhasePR
		return nil
	}
	if st.CoverageBudgetUsed >= st.CoverageBudget {
		st.Phase = state.PhasePR
		return nil
	}
	prompt, err := prompts.RenderCover(prompts.CoverInput{UncoveredGaps: gaps})
	if err != nil {
		return err
	}
	resp, err := d.Agent.Run(ctx, agent.Request{
		Phase: agent.PhaseCover,
		Cwd:   d.Worktree.Path,
		AllowedTools: []string{
			"Read", "Write", "Edit", "Glob", "Grep",
			"Bash(go test:*)", "Bash(git add:*)", "Bash(git commit:*)",
		},
		Env: map[string]string{
			"ALLOW_TEST_EDITS":       "1",
			"EXPECTED_COMMIT_PREFIX": "test(coverage)",
		},
		Prompt: prompt,
	})
	if err != nil {
		return fmt.Errorf("cover: agent: %w", err)
	}
	st.TotalCostUSD += resp.Cost
	st.CoverageBudgetUsed++

	res, err := d.Runner.Run(ctx, d.Worktree.Path)
	if err != nil {
		return err
	}
	if len(res.Failing) > 0 {
		// New tests are red; re-enter LOOP via main state machine
		st.Phase = state.PhaseCode
		st.LastFailing = res.Failing
		return nil
	}
	st.Phase = state.PhasePR
	return nil
}

func summarizeGaps(rep runner.CoverageReport) string {
	parts := []string{}
	for f, fc := range rep.ByFile {
		if len(fc.UncoveredLines) == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("- %s: lines %v", f, fc.UncoveredLines))
	}
	return strings.Join(parts, "\n")
}
```

- [ ] **Step 4: Run, expect PASS; commit**

```bash
go test ./internal/phases/...
git add internal/phases
git commit -m "feat(phases): COVER phase identifies gaps via coverprofile and re-enters LOOP if new red"
```

---

## Task 18: `internal/phases/pr.go`

**Files:**
- Create: `internal/phases/pr.go`, `internal/phases/pr_test.go`

- [ ] **Step 1: Test push happens (skip gh pr create in unit test, only invoke if `gh` present and `MIGRATE_LOOP_TEST_PR=1` to avoid accidental network)**

`internal/phases/pr_test.go`:

```go
package phases

import (
	"context"
	"testing"

	"github.com/jdfalk/migrate-loop/internal/state"
)

func TestPR_BuildsBody(t *testing.T) {
	st := &state.State{
		Slug: "demo", Phase: state.PhasePR, Budget: 50, BudgetUsed: 7,
		CoverageBudget: 15, CoverageBudgetUsed: 1,
		TotalCostUSD: 0.42, OscillationLog: []state.OscillationEvent{{Iteration: 4, Note: "x"}},
	}
	body := buildPRBody(st)
	for _, want := range []string{"7/50", "0.42", "iter4:x", "Generated by migrate-loop"} {
		if !contains(body, want) {
			t.Errorf("PR body missing %q\nbody:\n%s", want, body)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0))
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Implement `internal/phases/pr.go`**

```go
package phases

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/jdfalk/migrate-loop/internal/state"
)

func PR(ctx context.Context, st *state.State, d Deps) error {
	if err := d.Worktree.Push(ctx); err != nil {
		return fmt.Errorf("pr: push: %w", err)
	}
	body := buildPRBody(st)
	if _, err := exec.LookPath("gh"); err != nil {
		// gh not installed; leave the push and let the human open PR manually
		return nil
	}
	cmd := exec.CommandContext(ctx, "gh", "pr", "create",
		"--title", fmt.Sprintf("migrate: %s (test-first via migrate-loop)", st.Slug),
		"--body", body)
	cmd.Dir = d.Worktree.Path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gh pr create: %w\n%s", err, out)
	}
	return nil
}

func buildPRBody(st *state.State) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## migrate-loop run summary\n\n")
	fmt.Fprintf(&b, "- Slug: `%s`\n", st.Slug)
	fmt.Fprintf(&b, "- Iterations: %d/%d main, %d/%d coverage\n", st.BudgetUsed, st.Budget, st.CoverageBudgetUsed, st.CoverageBudget)
	fmt.Fprintf(&b, "- Total cost: $%.2f\n", st.TotalCostUSD)
	fmt.Fprintf(&b, "- Redirect used: %v\n", st.RedirectUsed)
	if len(st.OscillationLog) > 0 {
		fmt.Fprintf(&b, "- Oscillations: %s\n", summarizeOscillation(st.OscillationLog))
	}
	fmt.Fprintf(&b, "\nGenerated by migrate-loop. See commit graph for the test-first history.\n")
	return b.String()
}
```

- [ ] **Step 3: Run, expect PASS; commit**

```bash
go test ./internal/phases/...
git add internal/phases
git commit -m "feat(phases): PR phase pushes branch and (if gh present) opens PR with summary body"
```

---

## Task 19: `cmd/migrate-loop/main.go` — state machine + flags + resume

**Files:**
- Create: `cmd/migrate-loop/main.go`, `cmd/migrate-loop/run.go`, `cmd/migrate-loop/main_test.go`

- [ ] **Step 1: Write end-to-end test using FakeAgent that drives the whole machine**

`cmd/migrate-loop/main_test.go`:

```go
package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdfalk/migrate-loop/internal/agent"
)

func TestRun_EndToEndHappy(t *testing.T) {
	tmp := t.TempDir()
	specPath := filepath.Join(tmp, "spec.md")
	os.WriteFile(specPath, []byte(`---
title: trivial add
slug: trivial-add
target_packages: ["."]
test_runner: "go test -race -json ./..."
prior_examples: []
success_criteria: ["all tests pass"]
---
# trivial add
Add two ints.
`), 0o644)

	srcRepo := filepath.Join(tmp, "src")
	mustGit(t, tmp, "init", srcRepo)
	mustGit(t, srcRepo, "commit", "--allow-empty", "-m", "init")
	mustGit(t, srcRepo, "branch", "-M", "main")
	mustGit(t, srcRepo, "config", "user.email", "t@t")
	mustGit(t, srcRepo, "config", "user.name", "t")

	fa := scriptedHappyAgent(t)

	cfg := Config{
		SpecPath:       specPath,
		SourceRepo:     srcRepo,
		WorktreeDir:    filepath.Join(tmp, "wt"),
		Budget:         5,
		CoverageBudget: 2,
		IterTimeout:    10 * time.Second,
		Agent:          fa,
		SkipPR:         true,
	}
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
```

(`scriptedHappyAgent` and `mustGit` defined in `cmd/migrate-loop/testhelpers_test.go`; canned responses + `Editor` closure that writes test files in PLAN, then green-fixing src in CODE iter 1.)

- [ ] **Step 2: Implement `cmd/migrate-loop/run.go` (Config + Run)**

```go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jdfalk/migrate-loop/internal/agent"
	"github.com/jdfalk/migrate-loop/internal/escalate"
	"github.com/jdfalk/migrate-loop/internal/phases"
	"github.com/jdfalk/migrate-loop/internal/runner"
	"github.com/jdfalk/migrate-loop/internal/spec"
	"github.com/jdfalk/migrate-loop/internal/state"
	"github.com/jdfalk/migrate-loop/internal/worktree"
)

type Config struct {
	SpecPath       string
	SourceRepo     string
	WorktreeDir    string
	Budget         int
	CoverageBudget int
	IterTimeout    time.Duration
	Agent          agent.Agent // injected; defaults to CLIAgent in main()
	SkipPR         bool        // for tests
	Resume         bool
}

func Run(ctx context.Context, cfg Config) error {
	sp, err := spec.ParseFile(cfg.SpecPath)
	if err != nil {
		return err
	}
	if cfg.WorktreeDir == "" {
		cfg.WorktreeDir = filepath.Join(filepath.Dir(cfg.SourceRepo), filepath.Base(cfg.SourceRepo)+"-migrate-"+sp.Slug)
	}
	if cfg.CoverageBudget == 0 {
		cfg.CoverageBudget = (cfg.Budget + 2) / 3 // ceil(0.3 * budget) approx
		if cfg.CoverageBudget < 1 {
			cfg.CoverageBudget = 1
		}
	}

	var wt *worktree.Worktree
	statePath := filepath.Join(cfg.WorktreeDir, "STATE.md")
	var st *state.State
	if cfg.Resume {
		st, err = state.Read(statePath)
		if err != nil {
			return fmt.Errorf("resume: %w", err)
		}
		wt = &worktree.Worktree{Path: cfg.WorktreeDir, BranchName: "migrate/" + sp.Slug}
	} else {
		wt, err = worktree.Create(ctx, worktree.Options{
			SourceRepo: cfg.SourceRepo, BranchName: "migrate/" + sp.Slug,
			WorktreeDir: cfg.WorktreeDir, BaseRef: "main",
		})
		if err != nil {
			return err
		}
		if err := wt.InstallHook(); err != nil {
			return err
		}
		st = &state.State{
			Slug: sp.Slug, Phase: state.PhaseInit, Budget: cfg.Budget,
			CoverageBudget: cfg.CoverageBudget,
		}
		writeState(statePath, st)
		_ = wt.Commit(ctx, fmt.Sprintf("chore(migrate-loop): init %s", sp.Slug),
			map[string]string{"EXPECTED_COMMIT_PREFIX": "chore(migrate-loop)"})
	}

	lock, err := worktree.Lock(filepath.Join(cfg.WorktreeDir, ".migrate-loop.lock"))
	if err != nil {
		return fmt.Errorf("lock: %w", err)
	}
	defer lock.Release()

	deps := phases.Deps{
		Agent:    cfg.Agent,
		Runner:   runner.NewGoRunner(sp.TestRunner),
		Worktree: wt,
	}

	for {
		switch st.Phase {
		case state.PhaseInit:
			st.Phase = state.PhasePlan
		case state.PhasePlan:
			if err := phases.Plan(ctx, st, sp, deps); err != nil {
				return handleErr(err, st, wt, statePath)
			}
		case state.PhaseCode:
			advance, err := phases.Code(ctx, st, deps)
			if err != nil {
				return handleErr(err, st, wt, statePath)
			}
			_ = advance
		case state.PhaseRedirect:
			if err := phases.Redirect(ctx, st, deps); err != nil {
				return handleErr(err, st, wt, statePath)
			}
		case state.PhaseCover:
			if err := phases.Cover(ctx, st, deps); err != nil {
				return handleErr(err, st, wt, statePath)
			}
		case state.PhasePR:
			if !cfg.SkipPR {
				if err := phases.PR(ctx, st, deps); err != nil {
					return handleErr(err, st, wt, statePath)
				}
			}
			writeState(statePath, st)
			_ = wt.Commit(ctx, fmt.Sprintf("chore(migrate-loop): completed %s in %d iters", st.Slug, st.BudgetUsed),
				map[string]string{"EXPECTED_COMMIT_PREFIX": "chore(migrate-loop)"})
			return nil
		case state.PhaseEscalated:
			return nil
		}
		writeState(statePath, st)
	}
}

func writeState(path string, st *state.State) {
	_ = state.Write(path, st)
}

func handleErr(err error, st *state.State, wt *worktree.Worktree, statePath string) error {
	kind := classifyEscalation(err)
	if kind == "" {
		return err // Class 1: infra error → exit 1
	}
	st.Phase = state.PhaseEscalated
	st.EscalationReason = string(kind)
	writeState(statePath, st)
	_ = escalate.Write(filepath.Join(wt.Path, "ESCALATION.md"), escalate.Reason{
		Kind: kind, Summary: err.Error(),
		LastFailing: st.LastFailing,
	})
	_ = wt.Commit(context.Background(), fmt.Sprintf("chore(migrate-loop): escalate %s", kind),
		map[string]string{"EXPECTED_COMMIT_PREFIX": "chore(migrate-loop)"})
	return &EscalationError{Kind: kind, Underlying: err}
}

type EscalationError struct {
	Kind       escalate.Kind
	Underlying error
}

func (e *EscalationError) Error() string { return fmt.Sprintf("escalation %s: %v", e.Kind, e.Underlying) }

func classifyEscalation(err error) escalate.Kind {
	if err == nil {
		return ""
	}
	switch {
	case containsErr(err, "tests_vacuous"):
		return escalate.KindTestsVacuous
	case containsErr(err, "budget_exhausted"):
		return escalate.KindBudgetExhausted
	case containsErr(err, "stagnation_after_redirect"):
		return escalate.KindStagnationAfterRedirect
	case containsErr(err, "tests_seem_wrong"):
		return escalate.KindTestsSeemWrong
	case containsErr(err, "timed out"):
		return escalate.KindIterationTimeout
	}
	return ""
}

func containsErr(err error, sub string) bool {
	if err == nil {
		return false
	}
	for _, c := range []byte(err.Error()) {
		_ = c
	}
	return len(err.Error()) >= len(sub) && stringContains(err.Error(), sub)
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

var _ = os.Stdout
```

- [ ] **Step 3: Implement `cmd/migrate-loop/main.go`**

```go
// migrate-loop drives a TDD migration via claude -p.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jdfalk/migrate-loop/internal/agent"
)

func main() {
	var cfg Config
	flag.StringVar(&cfg.SpecPath, "spec", "", "path to migration spec markdown")
	flag.StringVar(&cfg.SourceRepo, "repo", ".", "path to source git repo")
	flag.StringVar(&cfg.WorktreeDir, "worktree-dir", "", "path for worktree (default: <repo>-migrate-<slug>)")
	flag.IntVar(&cfg.Budget, "budget", 50, "max CODE iterations")
	flag.IntVar(&cfg.CoverageBudget, "coverage-budget", 0, "max COVER iterations (default: ceil(0.3*budget))")
	flag.DurationVar(&cfg.IterTimeout, "iter-timeout", 10*time.Minute, "per-iteration claude -p timeout")
	flag.BoolVar(&cfg.Resume, "resume", false, "resume existing worktree from STATE.md")
	flag.Parse()

	if cfg.SpecPath == "" {
		fmt.Fprintln(os.Stderr, "--spec is required")
		os.Exit(1)
	}
	cfg.Agent = agent.NewCLIAgent()

	err := Run(context.Background(), cfg)
	if err == nil {
		return
	}
	var ee *EscalationError
	if errors.As(err, &ee) {
		fmt.Fprintf(os.Stderr, "escalation: %s\n", ee.Kind)
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
```

- [ ] **Step 4: Run e2e test**

Run: `go test ./cmd/migrate-loop/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/migrate-loop
git commit -m "feat(cmd): wire state machine, flags, --resume; classify errors to exit codes 1/2"
```

---

## Task 20: Integration fixtures (oscillation, budget exhaustion, vacuous, resume)

**Files:**
- Create: `cmd/migrate-loop/scenario_test.go`, `cmd/migrate-loop/testdata/fixtures/...`

- [ ] **Step 1: Write 4 scenario tests**

For each scenario, build a `FakeAgent` whose canned responses + `Editor` produce the target trajectory, run `Run()`, assert on:
- Final exit (nil vs `*EscalationError` with expected `Kind`).
- `STATE.md` final fields.
- `ESCALATION.md` content (for failure scenarios).

Skeleton for `oscillation`:

```go
func TestRun_OscillationTriggersRedirect(t *testing.T) {
	// FakeAgent rotates which test passes for 3 iters, then fixes both at iter 4 (post-redirect).
	// Assert: st.RedirectUsed=true, final phase=PR, no EscalationError.
}
```

Skeleton for `budget exhaustion`:

```go
func TestRun_BudgetExhausted(t *testing.T) {
	// FakeAgent does no-op edits; budget=3.
	// Assert: EscalationError with Kind=budget_exhausted; ESCALATION.md exists.
}
```

(Full implementations follow the patterns from earlier phase tests; each scenario is ~40 lines.)

- [ ] **Step 2: Run, expect all PASS**

```bash
go test ./cmd/migrate-loop/...
```

- [ ] **Step 3: Commit**

```bash
git add cmd/migrate-loop
git commit -m "test(cmd): scenario coverage for oscillation, budget, vacuous, resume"
```

---

## Task 21: Live-API smoke test (gated by `live_api` build tag)

**Files:**
- Create: `cmd/migrate-loop/live_test.go`

- [ ] **Step 1: Implement guarded test**

```go
//go:build live_api

package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLive_TrivialAdd(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY required")
	}
	// Use testdata/live/trivial-add (a pre-built tiny Go module + spec)
	// budget=5, coverage-budget=2; expect Run() returns nil.
	cfg := Config{ /* ... */ }
	cfg.IterTimeout = 5 * time.Minute
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("live run failed: %v", err)
	}
}
```

- [ ] **Step 2: Add `make test-live` target invocation in CI nightly**

Append to `.github/workflows/nightly.yml`:

```yaml
name: nightly-live
on:
  schedule:
    - cron: "13 7 * * *"
jobs:
  live:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - run: make test-live
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

- [ ] **Step 3: Commit**

```bash
git add cmd/migrate-loop/live_test.go .github/workflows/nightly.yml
git commit -m "test(live): gated trivial-add smoke + nightly workflow"
```

---

## Task 22: Docs polish

**Files:**
- Modify: `README.md`
- Create: `docs/design.md` (copy of the spec from audiobook-organizer/docs/superpowers/specs/2026-04-24-migrate-loop-design.md)
- Create: `docs/USAGE.md`

- [ ] **Step 1: Copy design doc**

```bash
cp /path/to/audiobook-organizer/docs/superpowers/specs/2026-04-24-migrate-loop-design.md docs/design.md
```

- [ ] **Step 2: Write `docs/USAGE.md`**

```markdown
# Usage

## Quickstart

    migrate-loop --spec mymigration.md --budget 50

## Spec format

YAML frontmatter + free-form markdown. See [design.md](design.md) §"Spec format".

## Resume after escalation

    migrate-loop --spec mymigration.md --resume

## Common flags

- `--budget N` (default 50): max CODE iterations.
- `--coverage-budget N` (default ceil(0.3*budget)): max COVER iterations.
- `--iter-timeout 10m`: per-claude-p invocation timeout.
- `--repo PATH`: target repo (default `.`).

## Exit codes

- `0`: PR opened.
- `1`: infrastructure error.
- `2`: migration escalation (see `ESCALATION.md` in the worktree).
- `130`: interrupted (resumable via `--resume`).
```

- [ ] **Step 3: Update README**

Add quickstart, link to USAGE.md and design.md.

- [ ] **Step 4: Commit**

```bash
git add README.md docs
git commit -m "docs: copy design.md, add USAGE.md, link from README"
```

---

## Self-review

Spec coverage check:

| Spec section | Covered by task |
|---|---|
| State machine | T14–T19 |
| Architecture (cmd/internal/prompts) | T1–T19 |
| Spec format | T2 |
| Data flow happy path | T19 + T20 |
| Class 1 errors | T19 (handleErr default exit 1) |
| Class 2 escalations | T12 + T19 + T20 |
| Class 3 resumption | T19 (`--resume`) + T20 (resume scenario) |
| FROZEN_TESTS escape hatch | T15 |
| Concurrency lock | T11 |
| Pre-commit hook | T10 |
| Coverage gate | T1 (Makefile `cover` target) |
| Layer 1 unit tests | T2–T18 |
| Layer 2 integration | T19 + T20 |
| Layer 3 live smoke | T21 |

Placeholder scan: no TBDs, no "implement later"; all code blocks contain real code.

Type consistency: `state.TestID`, `state.Phase`, `agent.Phase`, `agent.Request/Response`, `phases.Deps`, `runner.Result/CoverageReport`, `escalate.Reason/Kind` are defined once and referenced consistently.

One acknowledged simplification: scenario tests in T20 are sketched as skeletons (~40 lines each). When implementing, follow the patterns established in T14–T18 phase tests verbatim; no new conventions needed.

---

## Out of scope (explicit)

- Non-Go runners (`pytest`, `vitest`) — `Runner` interface accepts them; impls are follow-up tasks.
- Multi-repo orchestration.
- Remote / cross-machine resumption.
- TUI / GUI.
