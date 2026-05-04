<!-- file: docs/security/audit-2026-05-03/reviews/arch-design.md -->
<!-- version: 1.0.0 -->
<!-- guid: 2c4f9b7a-3e2e-4a9d-9c1b-7e8d6c2a9f10 -->
<!-- last-edited: 2026-05-03 -->

# Architectural Review — Security Audit 2026-05-03 Remediation Plan

**Reviewer role:** Independent system-design auditor
**Scope:** `docs/security/audit-2026-05-03/spec.md` + `implementation-plan.md` (11 phases, 16 tasks, ~44h)
**Verdict TL;DR:** The plan is *operationally* solid (well sequenced, scoped per-PR, includes rollback) but *architecturally* under-ambitious. It proposes a utility package where the codebase needs a **type-system boundary**. Without that, you will fix 217 alerts and quietly accumulate a 218th the next time someone calls `os.ReadFile(s)` directly.

---

## 1. Executive Verdict on the Plan

| Dimension | Rating | Notes |
|---|---|---|
| Plan coherence (ordering, dependencies) | **8/10** | Phase 0 → 1 → 2 → fan-out (3,4,5,6) is correct. |
| Architectural depth of fix | **4/10** | A package of helper functions is *advisory*; nothing forces callers to use it. |
| Defense-in-depth coverage | **3/10** | Plan is single-layer (input validation). No mention of egress controls, sandboxing, or runtime monitoring. |
| Process/governance (PR strategy, testing) | **6/10** | Per-phase PRs are good. Missing: fuzzing, CodeQL diff gating, regression tests as an explicit deliverable. |
| Cross-cutting concerns | **3/10** | No threat model, no secrets-management plan, no security-event logging design, no SBOM. |
| Resilience/monitoring after fix | **2/10** | Plan ends at "0 open alerts." There is no story for ongoing detection. |
| **Overall confidence in the plan as written** | **5/10** | Will close the alerts. Will *not* prevent the next 217. |

**Recommendation:** Adopt the plan **with the architectural amendments in §2 and §4 below**. Do not merge Phase 1 as currently scoped — expand it to introduce a typed `SafePath` boundary, then fan out.

---

## 2. Architectural Root Cause Analysis

### The real diagnosis

217 path-injection alerts in 14+ files is **not a bug pattern, it is a missing type**. Go's stdlib uses `string` for paths, and CodeQL is correctly observing that any `string` reaching `os.Open`, `os.Create`, `os.ReadFile`, `filepath.Join`, etc. is taint-reachable from HTTP handlers. A function-based validator (`pathvalidation.ValidateRelativePath(s, base)`) does not change the type of the argument flowing through the program — it just adds an *optional* checkpoint that future contributors will forget.

CodeQL will (and should) re-flag the same call sites the moment someone refactors and drops the validator call.

### Bandaid (what the plan proposes)

```go
func (s *Service) ReadFile(path string) ([]byte, error) {
    validated, err := pathvalidation.ValidateRelativePath(path, s.baseDir)
    if err != nil { return nil, err }
    return os.ReadFile(validated)
}
```

Problems:
1. `validated` is still a `string`. Nothing prevents `os.ReadFile(rawPath)` two lines later.
2. `s.baseDir` is implicit context — if a service is reused with a different base, validation silently widens.
3. CodeQL data-flow may still flag the call because the sanitizer isn't recognized.
4. New code that doesn't go through `Service` has no obligation to validate.

### Architectural fix (what the plan should propose)

Introduce a **typed boundary** so the type checker, not the reviewer, enforces validation:

```go
// internal/security/safepath/safepath.go
package safepath

// Root is a validated, absolute base directory. Construct via NewRoot.
type Root struct{ abs string }

// SafePath is a path that has been proven to resolve inside some Root.
// The zero value is invalid. Only constructible via Root methods.
type SafePath struct {
    root *Root
    abs  string // cleaned, absolute, guaranteed within root.abs
}

func NewRoot(dir string) (*Root, error)               // resolves symlinks, validates exists
func (r *Root) Resolve(userInput string) (SafePath, error) // securejoin under the hood
func (r *Root) ResolveExisting(input string) (SafePath, error) // + os.Lstat verification
func (p SafePath) String() string                     // for logging only (marked)
func (p SafePath) OSPath() string                     // explicit unwrap for syscall use
```

Then **wrap or replace** the filesystem APIs the codebase uses:

```go
// internal/security/safepath/fs.go
func (p SafePath) Open() (*os.File, error)            { return os.Open(p.abs) }
func (p SafePath) ReadFile() ([]byte, error)          { return os.ReadFile(p.abs) }
func (p SafePath) Create() (*os.File, error)          { return os.Create(p.abs) }
func (p SafePath) Stat() (os.FileInfo, error)         { return os.Stat(p.abs) }
func (p SafePath) Walk(fn fs.WalkDirFunc) error       { ... } // re-roots WalkDir results
```

And add a **lint gate** so the value is preserved (see §5):

```yaml
# .golangci.yml — forbid raw os.* path APIs in everything except internal/security/safepath
forbidigo:
  forbid:
    - p: '^os\.(Open|Create|ReadFile|WriteFile|Remove(All)?|Mkdir(All)?|Rename|Stat|Lstat|Chmod|Chown)$'
      msg: 'use safepath.SafePath methods; raw os path API is forbidden outside internal/security/safepath'
    - p: '^ioutil\.'
    - p: '^filepath\.Join$'
      msg: 'use safepath.Root.Resolve to compose user input with a root'
```

Combined with **a CodeQL custom sanitizer query** (`docs/security/codeql/safepath-sanitizer.ql`) that teaches CodeQL the `Root.Resolve` boundary, the alerts go to zero *and stay there*.

### Equivalent structural fixes for the other categories

| Category | Plan proposes | Architectural fix |
|---|---|---|
| Path injection (217) | `pathvalidation` package of free functions | `safepath.SafePath` newtype + wrapped FS API + linter ban on raw `os.*` |
| SSRF / request forgery (4) | `urlvalidation` package | `safehttp.Client` wrapping `http.Client` with allowlist + private-IP block + per-call `context.Context` timeout, used as the *only* exported HTTP client; ban `http.DefaultClient`, `http.Get`, `net.Dial` outside the package |
| Clear-text logging (6) | Manual redaction at call sites | `log/slog` `LogValuer` wrappers (`security.Secret(s)`, `security.PII(s)`) that always render as `[REDACTED]`; combined with a slog handler that scrubs known field names (`password`, `token`, `authorization`) defensively |
| Zip Slip (1) | `pathvalidation.SecureJoin` per entry | `safepath.Root.ExtractInto(archive io.Reader)` — extraction *cannot* be expressed without a `Root` |
| Weak hashing (1) | Swap algorithm | Move to `internal/security/crypto` with named functions (`HashPassword`, `HashConfigChecksum`) so call sites express *intent*; ban direct `crypto/md5`, `crypto/sha1` via linter |
| Disabled TLS (2: Go + JS) | Investigate; possibly re-enable | Quarantine: split mTLS provisioning into `…/mtls/provisioning_test.go` build-tag-gated test helpers. Production code path should not be able to reach `InsecureSkipVerify: true`. |
| Uncontrolled allocation (3) | Cap with `min(...)` | Introduce `internal/security/limits` constants (`MaxScanBufferBytes`, `MaxImportEntries`) sourced from config so caps are auditable in one place |

---

## 3. Plan Coherence & Ordering

### What the plan gets right

- **Phase 0 (govulncheck) is correctly identified as the unblocker**, and the binary-mode workaround is the correct interim fix for `GOEXPERIMENT=jsonv2`.
- **Phase 1 → 2 → {3,4,5,6} fan-out** matches the dependency DAG (foundation, then layer-by-layer).
- **Phase 2 targets `internal/fileops/`** which is the right *call-graph* root: most other layers (`server`, `scanner`, `reconcile`) eventually delegate filesystem access there. Fixing fileops first means upstream callers gain partial protection automatically.
- **Per-phase PRs** are correct (see §5).
- **44h estimate** is realistic for a bandaid fix; **add ~12–16h** for the typed boundary, lint gate, and CodeQL sanitizer query — well worth it.

### What the plan gets wrong

1. **Phase 1 is mis-scoped as "build the package, don't apply it."** This produces a deliverable that is unverifiable — there is no way to tell from CI that the package is correct in isolation. **Fix:** Phase 1 should also convert *one* call site (the smallest, e.g., `internal/fileops/hash.go`) end-to-end, including the lint rule and a CodeQL custom sanitizer. This proves the architecture before scaling it.

2. **Phase 1 will *not* close the 217 alerts on its own** — and the plan's own table acknowledges "0 alerts" for Phase 1. That is fine, but the *real* risk is that Phases 2–6 only close alerts whose call sites the author remembered to refactor. **Without a lint ban on raw `os.*`, the closure is not enforceable.** Add Task 1.2: "Add forbidigo rule + CI gate for raw filesystem APIs outside `internal/security/safepath`."

3. **Phase 7 bundles five unrelated fixes (logging, SSRF, allocation, zipslip, hashing) into one phase but five PRs.** That's fine for PRs, but the phase-level "Status" / "Dependencies" model breaks down. **Fix:** Promote SSRF (Task 7.2) to its own Phase (call it 7a) because it deserves the same `safehttp.Client` newtype treatment and is currently buried.

4. **No phase for the Go module/`go.mod` upgrade.** The spec acknowledges `go 1.24.0` in `go.mod` while the codebase targets 1.25 features via `GOEXPERIMENT=jsonv2`. This is itself a *supply-chain* and *toolchain* hazard (govulncheck, gosec, staticcheck all behave subtly differently). **Add a Phase 0.5:** decision record on the Go-toolchain pinning strategy, plus CI matrix for both the experimental and the stable build.

5. **Phase 11 ("re-pull alert data") is weak.** A drift-detection mechanism is needed — see §6.

6. **Resilience to phase block:** If Phase 0 cannot be solved (e.g., govulncheck in binary mode misses something material), Phases 1–10 are *not* actually blocked — only the assurance that no *new* CVE landed during the audit is. The plan should explicitly call out that Phases 1–10 can proceed in parallel with Phase 0, with a tracking issue rather than a hard gate.

### Front- vs back-loaded value

- **Cumulative alert closure:** Phase 0 = 0, Phase 1 = 0, Phase 2 = 9, Phase 3 = 18, Phase 4 = 38, Phase 5 = 53, Phase 6 = 63, Phase 7 = 77, Phase 8 = 81, Phase 9 = 82, Phase 10 = 82, Phase 11 = (verification).
- The plan is **back-loaded** — the largest single drop (20 alerts) is in Phase 4, halfway through. Consider re-sequencing Phase 4 ahead of Phase 3 so the largest single PR ships earliest after the foundation; this de-risks the schedule.

---

## 4. Cross-Cutting Gaps

### 4.1 Threat model — MISSING

The spec lists alert categories but does not describe the trust model: who are the principals (anonymous web visitor? authenticated admin? local CLI user?), what assets are protected (audiobook media, OPDS feeds, iTunes credentials, scan database), and which boundaries cross trust zones. Without this, "fix" decisions are made per-alert rather than per-asset.

**Recommendation:** Add `docs/security/threat-model.md` (STRIDE per component, data-flow diagram, asset inventory). This should *precede* Phase 1 — it will likely re-prioritize fixes (e.g., the OPDS endpoint that serves files to unauthenticated readers should be ahead of the admin-only iTunes transfer).

### 4.2 Secrets management — UNADDRESSED

- Spec notes a `sk-test12345678` placeholder in `config.yaml` and dismisses it. Fine.
- But the plan never asks: **how are real secrets (OpenAI keys, Audible/Audnex tokens, mTLS private keys, webhook secrets) loaded, rotated, and audited?**
- `internal/mtls/provisioning.go` is in the audit (cert-check disabled). Provisioning by definition handles a private key.

**Recommendation:** Phase 8 should include an inventory of secret-bearing config keys and a decision: file-based (mode 0600 + checksum), env-var-based (12-factor), or external (sops, age, vault). Capture as an ADR. At minimum: forbid logging of `*Secret`, `*Token`, `*Key` fields via the `slog` wrapper described in §2.

### 4.3 Defense in depth — SINGLE LAYER

The plan addresses **input validation** only. A complete posture for a service that accepts HTTP, reads/writes filesystem, and makes outbound HTTP needs:

| Layer | Plan? | Recommendation |
|---|---|---|
| Network ingress | No | Document reverse-proxy expectations (TLS termination, body-size limits, request timeouts) in `deploy/` |
| Auth/authz | No | Audit current auth on file-serving endpoints; ADR for session/CSRF model |
| Rate limiting | No | Per-IP token bucket on cover/import endpoints (DoS surface acknowledged in alerts #44, #129) |
| Input validation | **Yes (this plan)** | — |
| Sandboxing of FS ops | No | Consider running with read-only root + bind-mounted media dir in container (`Dockerfile` already exists); document in deploy docs |
| Egress filtering | Partial (SSRF) | `safehttp.Client` + container egress allowlist if available |
| Output encoding | Partial (alert #50) | Audit all template/JSX render paths |
| Audit logging | No | See §4.4 |
| Runtime detection | No | See §6 |

### 4.4 Security-event logging — MISSING

Validation rejections, SSRF blocks, zip-slip rejects, auth failures, and rate-limit hits are *signals*. The plan throws them into `fmt.Errorf("invalid path: %w", err)` and discards. **Add a structured security event channel** (separate slog handler, JSON-formatted to `logs/security.log` or stderr with `event=security`) with at least: timestamp, principal (if any), event_type, decision, path/url (sanitized via the `LogValuer`), source IP, request ID. This is the input to §6.

### 4.5 SBOM / supply-chain — UNADDRESSED beyond govulncheck

govulncheck only catches Go vulns. The repo also has a `web/` (npm), `Dockerfile` (base image CVEs), and a `go.mod` with `cyphar/filepath-securejoin` proposed as a new dependency. Add:
- `syft`/`cyclonedx-gomod` SBOM generation in CI.
- `trivy` scan of the built container image.
- npm `audit` already implicit via Phase 9 — make it a recurring CI gate, not a one-shot.

---

## 5. Process / PR Strategy Recommendation

### Mega-PR vs. multi-PR

**Multi-PR, as the plan proposes**, is correct. Rationale:
- Reviewability: a 217-file diff is unreviewable; per-domain PRs (covers, iTunes, scanner) map to mental modules.
- Bisectability: if a regression appears, `git bisect` between phase commits localizes it.
- Rollback granularity: Phase 6 break does not roll back Phase 2.
- CI cost: each PR re-runs CodeQL; you get incremental alert-count feedback.

**However**, hard rules:

1. **Phase 1 PR must include the lint gate and at least one converted call site.** A package without an enforced consumer is dead code.
2. **Each subsequent phase PR must be net-negative on `gosec` + CodeQL alert counts.** Make this a CI gate (compare against `main`'s alert count via `gh api code-scanning/alerts`).
3. **No squash-merge for Phases 2–6.** Preserve the per-domain commits for forensic value.
4. **Tag a release after Phase 1 + lint gate land** (e.g., `v0.X.0-security-foundation`) so any downstream consumer can pin a known-clean baseline.

### Testing strategy (the plan under-specifies this)

| Test type | Plan? | Recommendation |
|---|---|---|
| Unit tests for `safepath` | Yes | Add table-driven traversal corpus (see OWASP fuzz dictionary `path-traversal.txt`) |
| Fuzz tests | **No — gap** | `FuzzResolve(f *testing.F)` on `Root.Resolve`. Cheap, high value. |
| Integration tests | Implicit | Add e2e tests that POST traversal payloads to each affected handler and assert 400 + security log emit |
| Golden tests for path normalization | No | One golden file per OS (Unix/Windows path separators) |
| CodeQL regression | No | Add a workflow step that fails the PR if open alert count increases vs base branch |
| Mutation testing on `safepath` | Optional | `gremlins-go` on the security package; small surface, worth the rigor |
| Property-based tests | No | `gopter` invariant: for any string `s`, `Root.Resolve(s).OSPath()` either errors or has prefix `Root.abs` |

### CI gating (concrete)

Add `.github/workflows/security-gate.yml`:
- On every PR: run gosec, codeql, govulncheck, npm audit, trivy.
- Compute open-alert delta vs base branch; fail if delta > 0 in any category.
- Run forbidigo / golangci-lint with the `safepath` rule.
- Run the safepath fuzz suite for 60s on PRs touching `internal/security/**`.

---

## 6. Resilience & Monitoring Recommendations

### Remaining attack surfaces after the plan

Even with all 236 alerts closed:

1. **Logic-layer auth/authz gaps** — CodeQL doesn't see them. Where are the unauthenticated endpoints? (OPDS, covers, public health endpoints?)
2. **Database-sourced paths trusted as "not user input"** — see dismissed-alerts justification. *But*: any endpoint that lets a user influence what gets written to the database (a scan trigger, an import) creates a stored-tainted-data path. Dismissals should be re-examined under that lens.
3. **The mTLS provisioning insecure-skip-verify path** — quarantine vs. fix is glossed over.
4. **Container/runtime posture** — ports exposed, user the binary runs as, capabilities. None addressed.
5. **Time-of-check / time-of-use (TOCTOU)** on filesystem ops — `safepath.Stat` then `safepath.Open` can race against symlink swap. Use `openat2(RESOLVE_BENEATH)` on Linux (via `golang.org/x/sys/unix`) inside `safepath` for true safety.
6. **Resource exhaustion beyond allocation** — goroutine leaks, file-descriptor leaks, sqlite WAL bloat.

### Monitoring and alerting to add

| Signal | Source | Alert threshold |
|---|---|---|
| `safepath` resolve rejections | security event log | >N/min sustained → potential probe |
| `safehttp` SSRF blocks | security event log | any in production |
| zipslip rejects | security event log | any (almost always malicious) |
| govulncheck regressions | scheduled CI | any |
| CodeQL open alert count | scheduled CI on `main` | increase vs. previous run |
| Auth failures | security event log | bursty pattern → enumeration |
| 5xx rate on file-serving endpoints | metrics | spike → possible DoS via crafted paths |
| Outbound HTTP latency / failure to non-allowlisted host | `safehttp.Client` metrics | any |

Persist security events to `logs/security.log` with rotation; document export to whatever observability stack the deployment uses.

---

## 7. ADRs to Create

Place under `docs/adr/` (the plan currently has no ADRs at all — first entry should be `ADR-0000-record-architecture-decisions.md` adopting the format).

1. **ADR-0001 — Adopt typed `safepath.SafePath` newtype as the only path crossing the FS boundary.** Status: Proposed. Captures §2 above; documents tradeoff vs. helper-function package.
2. **ADR-0002 — Forbid raw `os.*` filesystem APIs outside `internal/security/safepath` via golangci-lint forbidigo.** Status: Proposed. Documents enforcement mechanism.
3. **ADR-0003 — Adopt `internal/security/safehttp` as the only outbound HTTP client.** Status: Proposed. Allowlist + private-IP block + timeouts.
4. **ADR-0004 — Structured `slog` security-event channel (`event=security`).** Status: Proposed. Specifies fields, redaction rules, sink.
5. **ADR-0005 — Govulncheck binary-mode workaround for `GOEXPERIMENT=jsonv2`; revisit on Go 1.25 GA.** Status: Accepted. Captures Phase 0.
6. **ADR-0006 — Go toolchain pinning policy** (when do we bump `go` directive in `go.mod`, when do we drop GOEXPERIMENTs).
7. **ADR-0007 — Threat model scope and update cadence.** Points to `docs/security/threat-model.md`. Quarterly review.
8. **ADR-0008 — Secrets management strategy** (file vs env vs vault; what gets logged; rotation cadence).
9. **ADR-0009 — Dependency / SBOM strategy** (govulncheck + npm audit + trivy; cadence; CVE response SLA).
10. **ADR-0010 — Quarantine policy for `InsecureSkipVerify` and similar deliberate weaknesses** (build tags, test-only, documented exceptions list).
11. **ADR-0011 — CodeQL custom sanitizer queries policy** — how we teach CodeQL about our `safepath`/`safehttp` boundaries; where queries live; how they're versioned.
12. **ADR-0012 — Security-alert regression CI gate** (PR cannot increase open alert count).

---

## 8. Concrete Files / Packages to Introduce

```
internal/security/
├── safepath/                  # ADR-0001, ADR-0002
│   ├── doc.go
│   ├── safepath.go            # Root, SafePath types
│   ├── fs.go                  # Open/ReadFile/Create/Walk/Extract methods
│   ├── linux_openat2.go       # build-tag gated TOCTOU-safe variant
│   ├── safepath_test.go
│   ├── safepath_fuzz_test.go
│   └── README.md
├── safehttp/                  # ADR-0003
│   ├── client.go              # Client wrapping http.Client + allowlist
│   ├── dialer.go              # net.Dialer w/ private-IP block
│   ├── client_test.go
│   └── README.md
├── crypto/                    # consolidate hashing
│   ├── password.go            # argon2id wrappers
│   ├── checksum.go            # sha256 wrappers w/ named intent
│   └── crypto_test.go
├── seclog/                    # ADR-0004
│   ├── handler.go             # slog handler that scrubs + tags event=security
│   ├── values.go              # Secret(), PII(), URL() LogValuer wrappers
│   └── handler_test.go
└── limits/
    └── limits.go              # exported MaxScanBufferBytes, etc.

docs/
├── adr/
│   ├── ADR-0000-record-architecture-decisions.md
│   ├── ADR-0001-safepath-newtype.md
│   ├── … (see §7)
├── security/
│   ├── threat-model.md
│   ├── path-validation-policy.md   (already in plan)
│   └── codeql/
│       └── safepath-sanitizer.ql

.github/workflows/
└── security-gate.yml          # alert-count regression gate
.golangci.yml                  # forbidigo rules (new file or amend)
```

Existing call sites in §1 of the spec all become consumers of the above; the plan's Phases 2–7 then become *mechanical refactors* (much faster per file because the type system tells you when you're done).

---

## 9. Confidence in the Plan (1–10)

- **As-written confidence: 5/10.** Will close alerts; will not prevent recurrence; leaves cross-cutting concerns untouched.
- **With §2 typed-boundary amendment + §5 lint gate + §6 monitoring: 8/10.** Closes alerts, makes recurrence statically detectable, and produces a residual-risk signal for ongoing operations.
- **With all of the above + threat model (§4.1) + ADRs (§7): 9/10.** Remaining 1 point reserved for the unknowable: TOCTOU/runtime issues that no static plan can fully cover; mitigated by §6 monitoring.

---

## 10. Top 3 Architectural Recommendations (executive)

1. **Replace the `pathvalidation` helper package with a `safepath.SafePath` newtype + lint ban on raw `os.*` outside that package.** Without enforcement, the 217 alerts will return.
2. **Add a structured security-event channel (`seclog`) and a CI alert-count regression gate.** Together these convert "fix once" into "never regress."
3. **Write a threat model and a small ADR series *before* fanning out Phases 3–6.** It will re-rank priorities (likely OPDS / unauthenticated endpoints jump above admin-only iTunes flows) and gives Phase 11 ("verify") something concrete to verify against.

---

*Document version: 1.0.0*
*Reviewer: arch-design-reviewer*
*Status: independent review; no source code modified*
