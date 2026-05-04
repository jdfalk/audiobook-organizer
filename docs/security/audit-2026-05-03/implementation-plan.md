<!-- file: docs/security/audit-2026-05-03/implementation-plan.md -->
<!-- version: 2.0.0 -->
<!-- guid: 4619be08-72ad-4a9d-b15c-7d6d88e228a9 -->
<!-- last-edited: 2026-05-04 -->

# Security Alert Remediation — Implementation Plan (v2.0)

## Overview

This document provides a phased, actionable plan to remediate all 236 open security alerts identified in the 2026-05-03 audit, incorporating amendments from four independent reviews (`sast-sca-auditor`, `SE: Security`, `arch-design-reviewer`, `code-review`).

**Key Changes from v1.0:**
- Phase 0 (govulncheck blocker) marked RESOLVED; replaced with workflow verification (~30 min, not 1 hour).
- Front-loaded CodeQL custom sanitizer pack (Phase 1) to drop ~80 alerts before manual triage.
- Replaced free-function `pathvalidation` package with typed `safepath.SafePath` boundary + lint gate (Phase 2).
- Added LLM security hardening (Phase 5), auth-chain hardening (Phase 6), threat model + ADRs (Phase 8), and regression gate (Phase 9).
- Reordered phases to ship high-impact work first.
- **Total estimate: ~37 hours** (down from ~44h).

**Guiding Principles:**
- **Type-system enforcement over human reviewers:** Use newtypes + lint rules to make invalid states unrepresentable.
- **Triage before fixing:** CodeQL sanitizer pack collapses FP cluster; only fix real alerts.
- **Parallel-safe phases:** Phases 5, 6, 8 can run concurrently with path-injection work.
- **One coherent PR per phase:** Each phase ships independently; CI must pass before next phase.
- **Dispatch-ready prompts:** Every phase includes a complete sub-agent prompt block (copy-paste into Task tool).

**Reference Documents:**
- **Audit Spec v2:** `docs/security/audit-2026-05-03/spec.md`
- **Raw Data:** `docs/security/audit-2026-05-03/raw/`
- **Reviews:** `docs/security/audit-2026-05-03/reviews/SYNTHESIS.md`

---

## Phase Summary Table

| Phase | Goal | Effort | Alert Drop | Dependencies | Dispatch |
|-------|------|--------|------------|--------------|----------|
| **1** | CodeQL custom sanitizer pack | 3h | ~80–100 | None | ✅ READY |
| **2** | Typed `safepath` boundary + lint gate | 6h | 0 (enables fixes) | Phase 1 | ⏳ AFTER 1 |
| **3** | SSRF boundary (`safehttp`) | 4h | 4 | Phase 2 | ⏳ AFTER 2 |
| **4** | Logging hardening (`seclog`) | 3h | 6 | Phase 2 | ⏳ AFTER 2 |
| **5** | LLM security | 3h | 0 (new category) | None | ✅ READY |
| **6** | Auth-chain hardening | 4h | 2 + coverage | None | ✅ READY |
| **7** | Convert filesystem call-sites to safepath | 6h | ~110–130 | Phase 2 | ⏳ AFTER 2 |
| **8** | Threat model + ADRs | 3h | 0 (governance) | None | ✅ READY |
| **9** | Regression gate | 2h | 0 (CI gate) | Any fix merged | ⏳ AFTER 1 |
| **10** | False-positive cleanup | 2h | ~20–40 | Phases 1, 7 | ⏳ AFTER 1, 7 |
| **11** | Verification + closeout | 1h | Confirm 0 | All | ⏳ AFTER ALL |

**Parallel dispatch strategy:**
- **Day 1:** Kick off Phases 1, 5, 6, 8 in parallel (all READY, no dependencies).
- **Day 2:** After Phase 1 merges, kick off Phase 2.
- **Day 3:** After Phase 2 merges, kick off Phases 3, 4, 7 in parallel.
- **Day 4:** After at least one fix phase merges, kick off Phase 9 (regression gate).
- **Day 5:** After Phases 1 and 7 merge, kick off Phase 10 (FP cleanup).
- **Day 6:** After all phases merge, kick off Phase 11 (verification).

---

## Phase 1: CodeQL Custom Sanitizer Pack

**Goal:** Teach CodeQL about existing `internal/util.SafeJoin` and `WithinRoot` sanitizers to collapse the false-positive cluster (~80–100 of the 217 path-injection alerts).

**Status:** ✅ READY (no dependencies)

**Rationale:** The repository already has working sanitizers in `internal/util/path.go`, but CodeQL doesn't recognize them (project-specific). Every code path that uses them is flagged. The 17 dismissed alerts in `internal/maintenance/jobs/` (#544–#560) are explicit evidence of this. GitHub's Models-as-Data feature (introduced 2026-04-21, https://github.blog/changelog/2026-04-21-codeql-now-supports-sanitizers-and-validators-in-models-as-data/) allows custom CodeQL packs to register project-specific sanitizers.

**Affected Files:**
- `.github/codeql/codeql-pack.yml` (new)
- `.github/codeql/models/go-sanitizers.yml` (new)
- `.github/workflows/codeql.yml` (update to reference local pack)

**Tasks:**

### Task 1.1: Create CodeQL Pack Structure

Create `.github/codeql/codeql-pack.yml`:
```yaml
---
name: jdfalk/audiobook-organizer-sanitizers
version: 1.0.0
library: true
dependencies:
  codeql/go-all: "*"
```

### Task 1.2: Create Models-as-Data Sanitizer Registry

Create `.github/codeql/models/go-sanitizers.yml`:
```yaml
---
extensions:
  - addsTo:
      pack: codeql/go-all
      extensible: sanitizer
    data:
      # internal/util.SafeJoin is a path-traversal sanitizer
      - ["github.com/jdfalk/audiobook-organizer/internal/util", "SafeJoin", "", "ReturnValue", "path-injection", "manual"]
      # internal/util.WithinRoot is a boolean path validator
      - ["github.com/jdfalk/audiobook-organizer/internal/util", "WithinRoot", "", "Argument[0]", "path-injection", "manual"]
```

**Explanation of the MaD format:**
- `["package", "function", "input", "output", "kind", "provenance"]`
- `SafeJoin` returns a sanitized path (output = `ReturnValue`).
- `WithinRoot` validates its first argument (input = `Argument[0]`).
- `kind: "path-injection"` tells CodeQL this sanitizes path-traversal taint.
- `provenance: "manual"` indicates manual (not auto-generated) model.

### Task 1.3: Update CodeQL Workflow

Edit `.github/workflows/codeql.yml` to reference the local pack. Find the `Initialize CodeQL` step and add:
```yaml
- name: Initialize CodeQL
  uses: github/codeql-action/init@v3
  with:
    languages: go, javascript
    packs: codeql/go-queries, ./.github/codeql  # Add local pack
```

### Task 1.4: Re-Run Code Scanning

After PR merge, CodeQL will automatically re-scan on the next push to main. Manually trigger if needed:
```bash
gh workflow run codeql.yml --ref main
```

Wait for scan to complete (~10 minutes), then check alert count:
```bash
gh api repos/jdfalk/audiobook-organizer/code-scanning/alerts \
  --jq '[.[] | select(.state == "open" and .rule.id == "go/path-injection")] | length'
```

**Expected result:** ~110–137 open path-injection alerts (down from 217).

**Verification:**
1. ✅ `.github/codeql/` directory created with `codeql-pack.yml` and `models/go-sanitizers.yml`.
2. ✅ `.github/workflows/codeql.yml` references local pack.
3. ✅ CodeQL scan completes without errors.
4. ✅ Open `go/path-injection` alert count drops by ~80–100.

**Rollback:**
Revert the `.github/codeql/` directory and the `codeql.yml` change. Alert count will return to 217.

**PR Title:** `chore(security): add CodeQL custom sanitizer pack for path validators`

**Estimated Effort:** 3 hours

**Dispatch Readiness:** ✅ READY

```prompt
# Phase 1: CodeQL Custom Sanitizer Pack

You are working in worktree `/Users/jdfalk/.worktrees/sec-audit-phase1` on branch `chore/security-codeql-sanitizer-pack`.

## Goal
Teach CodeQL about existing `internal/util.SafeJoin` and `WithinRoot` sanitizers using GitHub's Models-as-Data feature. Expected to drop ~80–100 of the 217 open path-injection alerts.

## Context
- The repo already has working sanitizers in `internal/util/path.go`.
- CodeQL doesn't recognize them (project-specific), so every code path using them is still flagged.
- The 17 dismissed alerts in `internal/maintenance/jobs/` (#544–#560) are evidence of this FP pattern.
- GitHub's Models-as-Data feature (launched 2026-04-21) allows custom CodeQL packs to register project-specific sanitizers.
- Reference: https://github.blog/changelog/2026-04-21-codeql-now-supports-sanitizers-and-validators-in-models-as-data/
- Docs: https://docs.github.com/en/code-security/tutorials/customize-code-scanning/customizing-analysis-with-codeql-packs#codeqlpack-yml-properties

## Hard Rules
- Do NOT modify any Go source code outside `.github/codeql/`.
- Do NOT dismiss any alerts.
- Use the exact Models-as-Data YAML format from the task description.
- The local pack must be added to `codeql.yml` via the `packs:` input, not `config-file:`.

## Deliverables
1. Create `.github/codeql/codeql-pack.yml` with pack metadata (name: `jdfalk/audiobook-organizer-sanitizers`, version: `1.0.0`, library: true, deps: `codeql/go-all: "*"`).
2. Create `.github/codeql/models/go-sanitizers.yml` with MaD entries for `internal/util.SafeJoin` (ReturnValue sanitizer) and `internal/util.WithinRoot` (Argument[0] validator), both `kind: "path-injection"`, `provenance: "manual"`.
3. Update `.github/workflows/codeql.yml` to add `./.github/codeql` to the `packs:` input in the `Initialize CodeQL` step.
4. Commit with conventional commit: `chore(security): add CodeQL custom sanitizer pack for path validators`
5. Push and open PR. Body should explain the FP cluster, the MaD feature, and the expected alert drop (~80–100).

## Acceptance Criteria
- `.github/codeql/` directory created with correct structure.
- `codeql.yml` references local pack.
- PR opened (do not merge).
- PR body explains the expected alert drop and links to the GitHub Models-as-Data changelog.

## Verification Commands
After PR merges, manually trigger CodeQL scan:
```bash
gh workflow run codeql.yml --ref main
```

Check alert count after scan completes (~10 min):
```bash
gh api repos/jdfalk/audiobook-organizer/code-scanning/alerts \
  --jq '[.[] | select(.state == "open" and .rule.id == "go/path-injection")] | length'
```

Expected: ~110–137 (down from 217).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Phase 2: Typed Safepath Boundary + Lint Gate

**Goal:** Replace free-function `pathvalidation` helpers with a typed `safepath.SafePath` newtype + wrapped FS methods + forbidigo lint rule, so the type system (not reviewers) enforces path validation.

**Status:** ⏳ DEPENDS-ON Phase 1

**Rationale:** 217 path-injection alerts are not a bug pattern — they are a **missing type**. A function-based validator (`ValidateRelativePath(s, base)`) doesn't change the type of the argument flowing through the program; it's just an optional checkpoint that future contributors will forget. CodeQL will (and should) re-flag the same call sites the moment someone refactors and drops the validator call. A typed boundary (`safepath.Root` + `SafePath` newtype) makes invalid states unrepresentable.

**Affected Files:**
- `internal/security/safepath/` (new package)
- `internal/fileops/service.go` (first call-site conversion, proof-of-concept)
- `.golangci.yml` (add forbidigo rule)
- `.github/codeql/models/go-sanitizers.yml` (add MaD entries for new constructors)

**Tasks:**

### Task 2.1: Create `safepath` Package

Create `internal/security/safepath/safepath.go`:
```go
package safepath

import (
"errors"
"os"
"path/filepath"
)

// Root is a validated, absolute base directory.
type Root struct {
abs string
}

// SafePath is a path proven to resolve inside some Root.
// Zero value is invalid. Only constructible via Root methods.
type SafePath struct {
root *Root
abs  string // cleaned, absolute, guaranteed within root.abs
}

// NewRoot creates a validated Root. Returns error if dir doesn't exist.
func NewRoot(dir string) (*Root, error) {
abs, err := filepath.Abs(dir)
if err != nil {
return nil, err
}
abs, err = filepath.EvalSymlinks(abs)
if err != nil {
return nil, err
}
if _, err := os.Stat(abs); err != nil {
return nil, err
}
return &Root{abs: abs}, nil
}

// Resolve validates userInput resolves within this Root.
// Returns error if path escapes root or contains traversal sequences.
func (r *Root) Resolve(userInput string) (SafePath, error) {
cleaned := filepath.Clean(userInput)
joined := filepath.Join(r.abs, cleaned)
resolved, err := filepath.EvalSymlinks(joined)
if err != nil {
if os.IsNotExist(err) {
// Path doesn't exist yet; just check containment
resolved = joined
} else {
return SafePath{}, err
}
}
if !withinRoot(resolved, r.abs) {
return SafePath{}, errors.New("path escapes root")
}
return SafePath{root: r, abs: resolved}, nil
}

// OSPath returns the absolute path for syscall use. Use sparingly.
func (p SafePath) OSPath() string {
return p.abs
}

// String returns the path for logging (marked as safe).
func (p SafePath) String() string {
return p.abs
}

func withinRoot(path, root string) bool {
rel, err := filepath.Rel(root, path)
if err != nil {
return false
}
return !filepath.IsAbs(rel) && !hasTraversal(rel)
}

func hasTraversal(path string) bool {
parts := filepath.SplitList(path)
for _, part := range parts {
if part == ".." {
return true
}
}
return false
}
```

Create `internal/security/safepath/fs.go`:
```go
package safepath

import (
"io/fs"
"os"
)

// Open opens the file for reading.
func (p SafePath) Open() (*os.File, error) {
return os.Open(p.abs)
}

// ReadFile reads the entire file.
func (p SafePath) ReadFile() ([]byte, error) {
return os.ReadFile(p.abs)
}

// Create creates or truncates the file.
func (p SafePath) Create() (*os.File, error) {
return os.Create(p.abs)
}

// Stat returns file info.
func (p SafePath) Stat() (fs.FileInfo, error) {
return os.Stat(p.abs)
}

// Remove deletes the file.
func (p SafePath) Remove() error {
return os.Remove(p.abs)
}

// MkdirAll creates directory and parents.
func (p SafePath) MkdirAll(perm os.FileMode) error {
return os.MkdirAll(p.abs, perm)
}
```

Create `internal/security/safepath/safepath_test.go` with tests for:
- `NewRoot` with valid/invalid dirs.
- `Resolve` with traversal attempts (`../../etc/passwd`).
- `Resolve` with symlinks escaping root.
- FS methods (`Open`, `ReadFile`, etc.).

### Task 2.2: Add Forbidigo Lint Rule

Edit `.golangci.yml` (or create if missing):
```yaml
linters:
  enable:
    - forbidigo

linters-settings:
  forbidigo:
    forbid:
      - p: '^os\.(Open|Create|ReadFile|WriteFile|Remove(All)?|Mkdir(All)?|Rename|Stat|Lstat|Chmod|Chown)$'
        msg: 'Use safepath.SafePath methods; raw os path API is forbidden outside internal/security/safepath'
        exclude_godoc_examples: false
      - p: '^filepath\.Join$'
        msg: 'Use safepath.Root.Resolve to compose user input with a root'
      - p: '^ioutil\.'
        msg: 'Package ioutil is deprecated; use os or io'
```

Add exception for `internal/security/safepath/` itself:
```yaml
issues:
  exclude-rules:
    - path: internal/security/safepath/
      linters:
        - forbidigo
```

### Task 2.3: Convert First Call-Site (Proof-of-Concept)

Convert `internal/fileops/hash.go` (smallest file, #539):
```go
// Before:
func HashFile(path string) (string, error) {
data, err := os.ReadFile(path)
// ...
}

// After:
func HashFile(sp safepath.SafePath) (string, error) {
data, err := sp.ReadFile()
// ...
}
```

Update callers of `HashFile` to pass a `SafePath` (may require threading a `Root` through the call stack).

### Task 2.4: Add MaD Entries for Safepath Constructors

Update `.github/codeql/models/go-sanitizers.yml`:
```yaml
extensions:
  - addsTo:
      pack: codeql/go-all
      extensible: sanitizer
    data:
      # Existing entries for internal/util
      - ["github.com/jdfalk/audiobook-organizer/internal/util", "SafeJoin", "", "ReturnValue", "path-injection", "manual"]
      - ["github.com/jdfalk/audiobook-organizer/internal/util", "WithinRoot", "", "Argument[0]", "path-injection", "manual"]
      
      # New entries for internal/security/safepath
      - ["github.com/jdfalk/audiobook-organizer/internal/security/safepath", "Root", "Resolve", "ReturnValue", "path-injection", "manual"]
      - ["github.com/jdfalk/audiobook-organizer/internal/security/safepath", "SafePath", "OSPath", "ReturnValue", "path-injection", "manual"]
```

**Verification:**
1. ✅ `internal/security/safepath/` package created with `Root`, `SafePath` types and FS methods.
2. ✅ Tests pass (`go test ./internal/security/safepath/...`).
3. ✅ Forbidigo lint rule added and passes for `internal/security/safepath/` (exempted).
4. ✅ At least one call-site (`internal/fileops/hash.go`) converted to `SafePath`.
5. ✅ `make ci` passes.

**Rollback:**
Revert the `internal/security/safepath/` package and `.golangci.yml` changes. Alert count unchanged (Phase 1 already dropped ~80).

**PR Title:** `feat(security): add typed safepath boundary + forbidigo lint gate`

**Estimated Effort:** 6 hours

**Dispatch Readiness:** ⏳ DEPENDS-ON Phase 1

```prompt
# Phase 2: Typed Safepath Boundary + Lint Gate

You are working in worktree `/Users/jdfalk/.worktrees/sec-audit-phase2` on branch `feat/security-safepath-boundary`.

## Goal
Create `internal/security/safepath` package with `Root` + `SafePath` newtype, wrapped FS methods, and forbidigo lint rule banning raw `os.*` calls outside the package.

## Context
- Phase 1 (CodeQL sanitizer pack) has merged; ~80 alerts dropped.
- The remaining ~110–137 alerts require fixing call-sites, but free-function validators are too weak.
- A typed boundary makes invalid states unrepresentable: `SafePath` can only be constructed via `Root.Resolve`, which enforces containment.
- Forbidigo lint rule prevents future code from bypassing the boundary.

## Hard Rules
- Do NOT modify existing call-sites beyond the proof-of-concept (`internal/fileops/hash.go`).
- The `safepath` package itself must be exempted from the forbidigo rule (it needs raw `os.*` to implement the boundary).
- All tests must pass (`go test ./internal/security/safepath/...`).
- `make ci` must pass.

## Deliverables
1. Create `internal/security/safepath/safepath.go` with `Root`, `SafePath` types, `NewRoot`, `Resolve`, `OSPath`, `String` methods.
2. Create `internal/security/safepath/fs.go` with wrapped FS methods (`Open`, `ReadFile`, `Create`, `Stat`, `Remove`, `MkdirAll`).
3. Create `internal/security/safepath/safepath_test.go` with tests for traversal attempts, symlink escapes, and FS methods.
4. Edit `.golangci.yml` to add forbidigo rule banning raw `os.Open/Create/ReadFile/WriteFile/Remove/Mkdir/Rename/Stat` and `filepath.Join`.
5. Add exception for `internal/security/safepath/` in `.golangci.yml` `issues.exclude-rules`.
6. Convert `internal/fileops/hash.go` to use `SafePath` (proof-of-concept).
7. Update `.github/codeql/models/go-sanitizers.yml` to add MaD entries for `Root.Resolve` and `SafePath.OSPath`.
8. Commit with conventional commit: `feat(security): add typed safepath boundary + forbidigo lint gate`
9. Push and open PR. Body should explain the newtype design, the lint gate, and the proof-of-concept conversion.

## Acceptance Criteria
- `internal/security/safepath/` package exists with tests passing.
- Forbidigo lint rule added and exempts `internal/security/safepath/`.
- At least one call-site (`internal/fileops/hash.go`) converted.
- `make ci` passes.
- PR opened (do not merge).

## Verification Commands
```bash
go test ./internal/security/safepath/... -v
golangci-lint run ./internal/security/safepath/  # Should pass (exempted)
golangci-lint run ./internal/fileops/hash.go     # Should fail if raw os.* used
make ci
```

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Phase 3: SSRF Boundary (`safehttp`)

**Goal:** Create `internal/security/safehttp` package with typed `Client` wrapping `http.Client`, enforcing allowlist + private-IP blocking for all outbound HTTP requests.

**Status:** ⏳ DEPENDS-ON Phase 2 (for pattern consistency)

**Rationale:** 4 SSRF alerts (#587, #467, #458, #232) show user-controlled URLs flowing into `http.Get` and `http.Client.Do`. Allowlisting domains + blocking private IPs prevents attackers from probing internal network resources (cloud metadata endpoints, internal services).

**Affected Files:**
- `internal/security/safehttp/` (new package)
- `internal/metafetch/service.go` (convert cover fetches)
- `internal/server/covers.go` (convert cover proxy)
- `internal/openai/client.go` (convert OpenAI client)

**Estimated Effort:** 4 hours

**Dispatch Readiness:** ⏳ AFTER Phase 2

```prompt
# Phase 3: SSRF Boundary (safehttp)

Working in worktree `/Users/jdfalk/.worktrees/sec-audit-phase3` on branch `feat/security-ssrf-boundary`.

## Goal
Create `internal/security/safehttp` with `Client` wrapping `http.Client`, enforcing allowlist + private-IP blocking.

## Deliverables
1. Create `internal/security/safehttp/client.go` with `Client` type, `NewClient(allowlist []string) *Client`, `Do(ctx context.Context, req *http.Request) (*http.Response, error)` method.
2. `Do` must: (a) validate request URL is in allowlist, (b) resolve hostname, (c) reject private IPs (RFC1918, loopback, link-local), (d) enforce timeout from context, (e) return error if any check fails.
3. Create `internal/security/safehttp/client_test.go` with tests for allowlist enforcement, private-IP blocking, and timeout.
4. Convert `internal/metafetch/service.go`, `internal/server/covers.go`, `internal/openai/client.go` to use `safehttp.Client`.
5. Add forbidigo rule banning `http.DefaultClient`, `http.Get`, `http.Post` outside `internal/security/safehttp`.
6. Commit: `feat(security): add SSRF boundary with safehttp.Client`
7. Open PR.

## Acceptance
- `safehttp` package with tests passing.
- At least 2 call-sites converted.
- `make ci` passes.
- CodeQL alerts #587, #467, #458, #232 expected to drop (verify post-merge).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Phase 4: Logging Hardening (`seclog`)

**Goal:** Create `internal/security/seclog` package with `Secret`/`PII` `slog.LogValuer` wrappers that redact in non-debug mode, resolving 6 clear-text logging alerts.

**Status:** ⏳ DEPENDS-ON Phase 2 (for pattern consistency)

**Affected Files:**
- `internal/security/seclog/` (new package)
- `internal/server/maintenance_fixups.go` (6 alerts: #530-#526)
- `cmd/root.go` (#47)

**Estimated Effort:** 3 hours

**Dispatch Readiness:** ⏳ AFTER Phase 2

```prompt
# Phase 4: Logging Hardening (seclog)

Working in worktree `/Users/jdfalk/.worktrees/sec-audit-phase4` on branch `feat/security-logging-hardening`.

## Goal
Create `internal/security/seclog` with `Secret(s string)`, `PII(s string)` `slog.LogValuer` types that redact in production.

## Deliverables
1. Create `internal/security/seclog/values.go` with `Secret`, `PII` types implementing `slog.LogValuer`. Both render as `[REDACTED]` unless `DEBUG=true` env var set.
2. Create `internal/security/seclog/handler.go` with slog handler that scrubs known field names (`password`, `token`, `authorization`) defensively.
3. Create `internal/security/seclog/values_test.go` with tests for redaction behavior.
4. Convert `internal/server/maintenance_fixups.go:170,179,188,197,206` and `cmd/root.go:261` to wrap sensitive values in `seclog.Secret()` or `seclog.PII()`.
5. Add unit test: log a `Secret` value, assert output contains `[REDACTED]`.
6. Commit: `feat(security): add seclog redaction wrappers for sensitive logging`
7. Open PR.

## Acceptance
- `seclog` package with tests passing.
- All 7 clear-text logging alerts' call-sites wrapped.
- `make ci` passes.
- CodeQL alerts #530-#526, #47 expected to drop (verify post-merge).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Phase 5: LLM Security

**Goal:** Sanitize inputs and validate outputs for the OpenAI metadata parser, mitigating OWASP LLM01 (prompt injection) and LLM06 (PII disclosure).

**Status:** ✅ READY (no dependencies, can run parallel)

**Affected Files:**
- `internal/openai/openai_parser.go`
- `internal/openai/client.go`

**Estimated Effort:** 3 hours

**Dispatch Readiness:** ✅ READY

```prompt
# Phase 5: LLM Security

Working in worktree `/Users/jdfalk/.worktrees/sec-audit-phase5` on branch `feat/security-llm-hardening`.

## Goal
Sanitize filenames/metadata before OpenAI prompts, redact PII, validate LLM responses, add token/rate limits.

## Context
- `internal/openai/openai_parser.go:218` sends raw filesystem paths (including usernames, e.g. `/Users/alice/audiobooks/...`) to OpenAI verbatim.
- Crafted filenames (e.g. `"; DROP TABLE books; --.m4b`) can trigger LLM01 prompt injection.
- OS usernames in paths leak PII (LLM06).

## Deliverables
1. Add `sanitizeFilename(s string) string` helper: strip special chars, SQL keywords, script tags, path separators, limit length to 255 chars.
2. Add `redactPII(path string) string` helper: replace OS username patterns (`/Users/[^/]+/` → `/Users/[REDACTED]/`, `/home/[^/]+/` → `/home/[REDACTED]/`).
3. Update `openai_parser.go:218` to call both helpers before prompt construction.
4. Add JSON schema validation for LLM responses (expected fields: `title`, `author`, `narrator`; reject if schema mismatch or response >5000 chars).
5. Add token limit (max 2000 tokens per request) and rate limit (10 req/min per user) to `internal/openai/client.go`.
6. Add structured logging for all OpenAI API calls (input sanitized, output sanitized, token usage, latency) with `event=llm_api_call`.
7. Create `internal/openai/sanitize_test.go` with tests for traversal sequences, script tags, PII redaction.
8. Commit: `feat(security): LLM hardening - sanitize inputs, redact PII, validate responses`
9. Open PR.

## Acceptance
- Filename sanitization and PII redaction helpers added.
- All OpenAI prompt construction call-sites updated.
- JSON schema validation added for LLM responses.
- Token/rate limits added.
- Tests pass.
- `make ci` passes.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Phase 6: Auth-Chain Hardening

**Goal:** Authenticate `/api/events` SSE endpoint, move login lockout to durable storage, cap upload allocations, add route auth coverage test.

**Status:** ✅ READY (no dependencies, can run parallel)

**Affected Files:**
- `internal/server/server_lifecycle.go` (authenticate SSE endpoint)
- `internal/server/auth_handlers.go` (move lockout to DB)
- `internal/database/auth_lockout.go` (new table)
- `internal/scanner/scanner.go` (cap allocations)
- `internal/server/server_lifecycle_test.go` (new route auth test)

**Estimated Effort:** 4 hours

**Dispatch Readiness:** ✅ READY

```prompt
# Phase 6: Auth-Chain Hardening

Working in worktree `/Users/jdfalk/.worktrees/sec-audit-phase6` on branch `feat/security-auth-chain-hardening`.

## Goal
Fix unauthenticated SSE endpoint, move login lockout to durable storage, cap upload allocations, add route auth coverage test.

## Context
- `GET /api/events` at `server_lifecycle.go:787` is registered outside auth middleware — any unauthenticated caller can subscribe to real-time system events.
- Login lockout map at `auth_handlers.go:33–35` is process-local (wiped on restart). Attacker can trigger OOM via uncontrolled allocations at `scanner.go:219,339` to reset lockouts.

## Deliverables
1. Move `r.Get("/api/events", s.handleEvents)` (line 787 in `server_lifecycle.go`) to an authenticated route group with `s.perm(auth.PermReadEvents)` middleware.
2. Create `internal/database/auth_lockout.go` with SQL table `auth_lockout` (username, attempt_count, locked_until timestamp). Migrate `auth_handlers.go:33–35` from in-memory map to this table.
3. Cap upload allocations in `scanner.go:219,339`: replace `make([]byte, size)` with `make([]byte, min(size, MaxScanBufferBytes))` where `MaxScanBufferBytes = 100MB` constant.
4. Add `internal/server/server_lifecycle_test.go::TestRouteAuthCoverage`: parse all route registrations in `server_lifecycle.go`, assert every route either has auth middleware or is on allowlist (`/health`, `/metrics`, `/bootstrap`). Test should fail if a new unauthenticated route is added.
5. Commit: `feat(security): authenticate SSE endpoint, harden auth lockout, cap scanner allocations`
6. Open PR.

## Acceptance
- `/api/events` requires auth.
- Login lockout survives server restart (integration test: lock account, restart server, verify still locked).
- Scanner allocations capped.
- Route auth coverage test passes.
- `make ci` passes.
- CodeQL alerts #129, #44 expected to drop (verify post-merge).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Phase 7: Convert Remaining Filesystem Call-Sites to Safepath

**Goal:** Apply `safepath.SafePath` to all remaining filesystem call-sites identified by the ~110–137 post-Phase-1 path-injection alerts.

**Status:** ⏳ DEPENDS-ON Phase 2

**Affected Files:**
- `internal/scanner/` (multiple files)
- `internal/server/itunes_handlers.go`
- `internal/server/covers.go`
- `internal/reconcile/`
- `internal/backup/`
- ~10–15 other files (determined by remaining alert list post-Phase-1)

**Estimated Effort:** 6 hours

**Dispatch Readiness:** ⏳ AFTER Phase 2

```prompt
# Phase 7: Convert Remaining Filesystem Call-Sites to Safepath

Working in worktree `/Users/jdfalk/.worktrees/sec-audit-phase7` on branch `fix/security-path-injection-remaining`.

## Goal
Convert all remaining filesystem call-sites (post-Phase-1 alert list) to use `safepath.SafePath`.

## Context
- Phase 1 (CodeQL sanitizer pack) dropped ~80 alerts; ~110–137 remain.
- Phase 2 created `safepath` package and converted 1 proof-of-concept call-site.
- This phase converts the remaining genuine true positives (HTTP boundaries, scanner flows, etc.).

## Deliverables
1. Pull fresh alert list post-Phase-1:
   ```bash
   gh api repos/jdfalk/audiobook-organizer/code-scanning/alerts \
     --jq '[.[] | select(.state == "open" and .rule.id == "go/path-injection")] | .[].number' > alerts.txt
   ```
2. For each remaining alert:
   - Trace the tainted path back to its source.
   - If source is HTTP request body, query param, or user upload: convert to `safepath.Root.Resolve(userInput)`.
   - If source is internal pipeline (scanner, reconcile): thread a `safepath.Root` through the call stack.
   - If source is config-only (startup, not request-driven): document as FP for Phase 10 dismissal.
3. Commit after each file or logical cluster (e.g., "fix: convert scanner paths to safepath", "fix: convert iTunes handler paths to safepath").
4. Final commit: `fix(security): convert remaining filesystem call-sites to safepath`
5. Open PR.

## Acceptance
- All remaining open `go/path-injection` alerts either:
  - Converted to `safepath` (alert drops), OR
  - Documented as FP with rationale (for Phase 10 dismissal).
- `make ci` passes.
- Expected alert drop: ~70–90 (remaining ~20–40 are documented FPs for Phase 10).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Phase 8: Threat Model + ADRs

**Goal:** Write `docs/security/threat-model.md` and ADRs 0001–0012 documenting remediation pattern decisions.

**Status:** ✅ READY (no dependencies, can run parallel from start)

**Affected Files:**
- `docs/security/threat-model.md` (new)
- `docs/adr/ADR-0000-record-architecture-decisions.md` (new, meta-ADR)
- `docs/adr/ADR-0001-safepath-newtype.md` through `ADR-0012-sbom-policy.md` (new)

**Estimated Effort:** 3 hours

**Dispatch Readiness:** ✅ READY

```prompt
# Phase 8: Threat Model + ADRs

Working in worktree `/Users/jdfalk/.worktrees/sec-audit-phase8` on branch `docs/security-threat-model-adrs`.

## Goal
Document threat model and architectural decisions (ADRs) for remediation patterns.

## Context
- Original audit made fix decisions per-alert rather than per-asset. No structured security-event channel, no SBOM, no ADRs.
- Threat model identifies assets, principals, trust boundaries, attacker model.
- ADRs document "why" for each remediation pattern (safepath newtype, safehttp, seclog, etc.).

## Deliverables
1. Create `docs/security/threat-model.md`:
   - **Assets:** audiobook library metadata, cover images, user credentials, admin API keys, OpenAI API keys, mTLS certs.
   - **Principals:** admin users, read-only users, unauthenticated callers, internal services (scanner, metadata fetcher), external services (OpenAI, Open Library).
   - **Trust boundaries:** HTTP API surface, filesystem (audiobook root dirs), external HTTP fetches, LLM API calls, database.
   - **Attacker model:** external attacker with network access (no physical access), malicious audiobook files (crafted filenames, zip bombs), compromised external service (SSRF targets).
   - **Threat scenarios:** path traversal, SSRF to cloud metadata, LLM prompt injection, auth bypass, DoS via OOM.

2. Create `docs/adr/ADR-0000-record-architecture-decisions.md` (meta-ADR following Michael Nygard's template).

3. Create ADRs 0001–0012:
   - **ADR-0001:** Safepath newtype (why typed boundary over free functions).
   - **ADR-0002:** Forbidigo lint gate (why enforce at compile time).
   - **ADR-0003:** CodeQL custom sanitizer pack (why MaD over manual dismissals).
   - **ADR-0004:** Safehttp boundary (why allowlist + private-IP block).
   - **ADR-0005:** Seclog redaction (why LogValuer over manual redaction).
   - **ADR-0006:** Threat model alignment (why asset-centric over alert-centric).
   - **ADR-0007:** Regression gate (why CI alert-count check).
   - **ADR-0008:** LLM security hardening (why sanitize + validate).
   - **ADR-0009:** Auth-chain hardening (why durable lockout).
   - **ADR-0010:** Forbidigo enforcement (why ban raw os.*, http.Get).
   - **ADR-0011:** Security-event logging (why structured event channel).
   - **ADR-0012:** SBOM policy (why track dependencies).

4. Commit: `docs(security): add threat model and remediation ADRs`
5. Open PR.

## Acceptance
- `docs/security/threat-model.md` exists with all required sections.
- 13 ADRs (0000-0012) exist in `docs/adr/`.
- All ADRs follow Michael Nygard's template (Context, Decision, Consequences).
- PR opened (do not merge yet; can merge anytime, non-blocking).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Phase 9: Regression Gate

**Goal:** Add `.github/workflows/security-gate.yml` that fails any PR increasing open code-scanning alert count vs main, preventing regressions.

**Status:** ⏳ DEPENDS-ON at least one fix phase merged (Phase 1 or later)

**Affected Files:**
- `.github/workflows/security-gate.yml` (new)
- `.github/workflows/vulnerability-scan.yml` (add root npm coverage)

**Estimated Effort:** 2 hours

**Dispatch Readiness:** ⏳ AFTER Phase 1 (or any fix phase)

```prompt
# Phase 9: Regression Gate

Working in worktree `/Users/jdfalk/.worktrees/sec-audit-phase9` on branch `ci/security-regression-gate`.

## Goal
Create CI workflow that blocks PRs increasing open code-scanning alert count, preventing security regressions.

## Deliverables
1. Create `.github/workflows/security-gate.yml`:
   ```yaml
   name: Security Regression Gate
   on: [pull_request]
   jobs:
     check-alert-count:
       runs-on: ubuntu-latest
       steps:
         - uses: actions/checkout@v4
         - name: Get main branch alert count
           id: main
           run: |
             COUNT=$(gh api repos/${{ github.repository }}/code-scanning/alerts \
               --jq '[.[] | select(.state == "open")] | length')
             echo "count=$COUNT" >> $GITHUB_OUTPUT
         - name: Get PR branch alert count
           id: pr
           run: |
             COUNT=$(gh api repos/${{ github.repository }}/code-scanning/alerts?ref=${{ github.head_ref }} \
               --jq '[.[] | select(.state == "open")] | length')
             echo "count=$COUNT" >> $GITHUB_OUTPUT
         - name: Compare counts
           run: |
             if [ "${{ steps.pr.outputs.count }}" -gt "${{ steps.main.outputs.count }}" ]; then
               echo "❌ FAIL: PR increases open alert count (${{ steps.main.outputs.count }} → ${{ steps.pr.outputs.count }})"
               exit 1
             else
               echo "✅ PASS: Alert count did not increase (${{ steps.main.outputs.count }} → ${{ steps.pr.outputs.count }})"
             fi
   ```

2. Update `.github/workflows/vulnerability-scan.yml` to add root npm coverage (conditional, only if root `package.json` exists):
   ```yaml
   - name: Audit root npm dependencies (if present)
     if: hashFiles('package-lock.json') != ''
     run: npm audit --audit-level=moderate
     working-directory: .
   ```

3. Test the workflow locally (simulate):
   ```bash
   # Get current alert count
   gh api repos/jdfalk/audiobook-organizer/code-scanning/alerts \
     --jq '[.[] | select(.state == "open")] | length'
   ```

4. Commit: `ci(security): add regression gate blocking alert count increases`
5. Open PR. Body should explain the gate logic and note that root npm coverage is conditional (currently no root `package.json`).

## Acceptance
- `security-gate.yml` workflow created.
- `vulnerability-scan.yml` updated with conditional root npm audit.
- PR opened (do not merge yet; merge after at least one fix phase to have baseline).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Phase 10: False-Positive Cleanup

**Goal:** For each remaining open alert that's a documented FP (config-sourced, startup-only), dismiss via GitHub API with `dismissed_reason=false_positive` and specific comment.

**Status:** ⏳ DEPENDS-ON Phases 1, 7

**Affected Files:**
- None (API-only dismissals)

**Estimated Effort:** 2 hours

**Dispatch Readiness:** ⏳ AFTER Phases 1, 7

```prompt
# Phase 10: False-Positive Cleanup

Working in worktree `/Users/jdfalk/.worktrees/sec-plan-reconcile` (no branch change, API-only task).

## Goal
Dismiss all remaining open code-scanning alerts that are documented false positives (config-sourced, startup-only paths).

## Context
- Phase 1 (CodeQL sanitizer pack) dropped ~80 alerts.
- Phase 7 (convert filesystem call-sites) fixed ~70–90 real TPs.
- Remaining ~20–40 alerts are FP shape: config-only, startup-only, or internal pipeline (already wrapped in SafeJoin).

## Deliverables
1. Pull fresh alert list post-Phases 1 and 7:
   ```bash
   gh api repos/jdfalk/audiobook-organizer/code-scanning/alerts \
     --jq '[.[] | select(.state == "open" and .rule.id == "go/path-injection")] | .[] | {number, file: .most_recent_instance.location.path, line: .most_recent_instance.location.start_line}' > remaining_alerts.json
   ```

2. For each remaining alert, trace source:
   - If source is `config.AppConfig.*` or `dataDir` (startup): dismiss with `dismissed_reason=false_positive`, comment: "Path sourced from config at startup, not user-controlled."
   - If source is internal pipeline (scanner, reconcile) already wrapped in `SafeJoin`: dismiss with `dismissed_reason=false_positive`, comment: "Path validated via internal/util.SafeJoin before use."
   - If source is genuinely user-controlled (HTTP boundary): DO NOT dismiss; file tracking issue for Phase 7 follow-up.

3. Dismiss via GitHub API:
   ```bash
   for ALERT_NUM in $(cat remaining_fp_alerts.txt); do
     gh api -X PATCH repos/jdfalk/audiobook-organizer/code-scanning/alerts/$ALERT_NUM \
       -f state=dismissed \
       -f dismissed_reason=false_positive \
       -f dismissed_comment="Path sourced from config at startup, not user-controlled. Verified non-exploitable by security review."
   done
   ```

4. Document dismissals in `docs/security/audit-2026-05-03/dismissed-alerts.md` (append to existing file):
   - List each dismissed alert number, file:line, source pattern, rationale.

5. Commit: `chore(security): dismiss false-positive path-injection alerts with rationale`
6. Open PR (or commit directly to main if preferred).

## Acceptance
- All remaining FP alerts dismissed via API with documented rationale.
- `dismissed-alerts.md` updated.
- Target end state: 0 open `go/path-injection` alerts (all either fixed in Phases 1-7 or dismissed here).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Phase 11: Verification + Closeout

**Goal:** Re-pull all three alert APIs (code scanning, Dependabot, secret scanning), compare against 2026-05-03 baseline, write closeout report.

**Status:** ⏳ DEPENDS-ON all phases (1-10)

**Affected Files:**
- `docs/security/audit-2026-05-XX/raw/` (new dated directory)
- `docs/security/audit-2026-05-03/closeout.md` (new)

**Estimated Effort:** 1 hour

**Dispatch Readiness:** ⏳ AFTER ALL

```prompt
# Phase 11: Verification + Closeout

Working in worktree `/Users/jdfalk/.worktrees/sec-plan-reconcile` (no branch change, reporting task).

## Goal
Verify all 236 open alerts are resolved (fixed or consciously dismissed), write closeout report.

## Deliverables
1. Re-pull alert data:
   ```bash
   mkdir -p docs/security/audit-2026-05-XX/raw  # Use current date
   
   gh api repos/jdfalk/audiobook-organizer/code-scanning/alerts \
     --paginate --jq '.' > docs/security/audit-2026-05-XX/raw/code-scanning-alerts.json
   
   gh api repos/jdfalk/audiobook-organizer/dependabot/alerts \
     --paginate --jq '.' > docs/security/audit-2026-05-XX/raw/dependabot-alerts.json
   
   gh api repos/jdfalk/audiobook-organizer/secret-scanning/alerts \
     --paginate --jq '.' > docs/security/audit-2026-05-XX/raw/secret-scanning-alerts.json
   ```

2. Generate summary:
   ```bash
   echo "# Alert Summary $(date +%Y-%m-%d)" > docs/security/audit-2026-05-XX/raw/summary.md
   echo "" >> docs/security/audit-2026-05-XX/raw/summary.md
   echo "## Code Scanning" >> docs/security/audit-2026-05-XX/raw/summary.md
   jq '[.[] | select(.state == "open")] | length' docs/security/audit-2026-05-XX/raw/code-scanning-alerts.json >> docs/security/audit-2026-05-XX/raw/summary.md
   echo "" >> docs/security/audit-2026-05-XX/raw/summary.md
   echo "## Dependabot" >> docs/security/audit-2026-05-XX/raw/summary.md
   jq '[.[] | select(.state == "open")] | length' docs/security/audit-2026-05-XX/raw/dependabot-alerts.json >> docs/security/audit-2026-05-XX/raw/summary.md
   ```

3. Write `docs/security/audit-2026-05-03/closeout.md`:
   - **Original State (2026-05-03):** 236 open alerts (235 code scanning, 1 Dependabot).
   - **Final State (2026-05-XX):** 0 open alerts (or list remaining with rationale).
   - **Phases Completed:** Table showing Phase 1-11, effort actual vs estimate, PRs merged, alert delta per phase.
   - **Key Achievements:** CodeQL sanitizer pack (80-alert drop), typed safepath boundary, SSRF boundary, LLM hardening, auth-chain hardening, regression gate, threat model + ADRs.
   - **Lessons Learned:** Always verify tool compatibility claims (Phase 0 was stale), triage before fixing (FP cluster), type system enforcement (lint gates).
   - **Next Steps:** Monitor security-gate.yml for regressions, schedule quarterly security reviews, consider OWASP ASVS L1 compliance audit.

4. Commit: `docs(security): closeout report for 2026-05-03 audit remediation`
5. Open PR (or commit directly).

## Acceptance
- New dated raw data directory created.
- Closeout report written with before/after comparison.
- All acceptance criteria from `spec.md` confirmed met.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Rollback Strategy

Each phase ships as an independent PR. If a phase introduces regressions:

1. **Identify the problematic PR** via git bisect or CI logs.
2. **Revert the PR**:
   ```bash
   gh pr view <PR_NUMBER> --json mergeCommit --jq '.mergeCommit.oid' | xargs git revert
   git push
   ```
3. **File a tracking issue** documenting the regression and why the phase failed.
4. **Re-plan the phase** with fixes, then re-execute.

**Critical rollback scenarios:**
- Phase 2 (safepath): If forbidigo lint gate blocks too much existing code, temporarily disable the rule and file issue for incremental rollout.
- Phase 7 (filesystem conversion): If a conversion breaks production, revert the specific commit and mark that call-site for manual review.
- Phase 9 (regression gate): If gate produces false positives (e.g., transient CodeQL scan issues), add `allow_failure: true` temporarily while investigating.

---

## Post-Remediation Checklist

After all 11 phases merge:

- [ ] ✅ All 236 open alerts addressed (fixed or consciously dismissed).
- [ ] ✅ Govulncheck runs successfully (Phase 0 resolution verified).
- [ ] ✅ CodeQL custom sanitizer pack registered and active (Phase 1).
- [ ] ✅ Typed safepath boundary + lint gate deployed (Phase 2).
- [ ] ✅ SSRF boundary (safehttp) deployed (Phase 3).
- [ ] ✅ Logging hardening (seclog) deployed (Phase 4).
- [ ] ✅ LLM security hardening deployed (Phase 5).
- [ ] ✅ Auth-chain hardening deployed (Phase 6).
- [ ] ✅ All remaining filesystem call-sites converted or dismissed (Phase 7).
- [ ] ✅ Threat model + ADRs documented (Phase 8).
- [ ] ✅ Regression gate active in CI (Phase 9).
- [ ] ✅ False positives dismissed with rationale (Phase 10).
- [ ] ✅ Closeout report written (Phase 11).
- [ ] ✅ `make ci` passes on main.
- [ ] ✅ Post-remediation audit confirms 0 open alerts (or only accepted-risk alerts).

---

*Document Version: 2.0.0*  
*Original Plan Date: 2026-05-03*  
*Reconciliation Date: 2026-05-04*  
*Independent Reviews: `sast-sca-auditor`, `SE: Security`, `arch-design-reviewer`, `code-review`*  
*Total Estimated Effort: ~37 hours (down from ~44h in v1.0)*
