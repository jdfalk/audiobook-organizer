<!-- file: docs/security/audit-2026-05-03/reviews/SYNTHESIS.md -->
<!-- version: 1.0.0 -->
<!-- guid: 8b4f2d3a-9e1c-4f6b-8a7d-5e2c1b9f4a6d -->
<!-- last-edited: 2026-05-03 -->

# Security Audit — Independent Review Synthesis

Four specialized reviewers independently evaluated `spec.md` + `implementation-plan.md` + raw alert dumps. This document reconciles their findings: where they agree, where they disagree, and what the original audit missed.

## Reviewer roster

| Reviewer | Lens | File | Confidence |
|---|---|---|---|
| `sast-sca-auditor` | SAST/SCA classifications, dependency cross-check | `sast-sca-auditor.md` | 7/10 |
| `SE: Security` | OWASP Top 10 + LLM Top 10 + Zero Trust | `se-security.md` | (verdict: NOT production-ready) |
| `arch-design-reviewer` | Plan coherence, root-cause analysis, ADRs | `arch-design.md` | 5/10 as written, 8–9/10 with amendments |
| `code-review` | Documentation accuracy, conventions, reproducibility | `code-review.md` | 9/10 (Approve) |

## Where all four agree

1. **The audit's data is accurate.** `code-review` spot-checked 10 alert citations against the raw JSON — 10/10 match. `sast-sca-auditor` re-pulled the data live and confirmed counts. No reviewer disputes the headline numbers (236 open / 17 dismissed / 369 fixed).
2. **The 217-alert path-injection cluster is the single biggest issue.** All four flag it as the dominant remediation work item.
3. **A bandaid fix won't hold.** Both `arch-design` and `sast-sca-auditor` say sprinkling `filepath.Clean` calls across handlers will leave the 218th alert one refactor away.

## Where they disagree with the original audit

### High-impact disagreements

1. **The govulncheck blocker is stale (sast-sca-auditor).**
   - Spec says: "`GOEXPERIMENT=jsonv2` breaks govulncheck; need binary-mode workaround."
   - Reality (verified live): `go.mod` is at `1.26.0`, `govulncheck ./...` runs cleanly and reports `No vulnerabilities found`. The `GOEXPERIMENT=jsonv2` line in `codeql.yml` and `vulnerability-scan.yml` is no longer needed.
   - **Action change:** Phase 0 collapses from "implement binary-mode scan" to "delete the obsolete env var and re-enable the workflow." ~30 minutes, not 1 hour.

2. **~35–45% of the path-injection alerts are likely false positives (sast-sca-auditor).**
   - Spot-check of 11 random open alerts found ~4–5 are the same shape as the 17 already-dismissed cluster: paths sourced from `config.AppConfig.*` or the data dir (e.g. #627 `itunes_handlers.go:837`, #505 `bootstrap.go:119`, #348 `config/persistence.go:129`).
   - Real true positives sit at HTTP boundaries (e.g. #388 `filesystem_handlers.go:165`, #442 `metafetch/service.go:254`).
   - **Action change:** Before bulk-fixing, build a CodeQL custom-sanitizer pack registering `internal/util.SafeJoin` / `WithinRoot`. This will cut the open count dramatically and let triage focus on real boundaries.

3. **Dependabot #27 (`follow-redirects`) is in the wrong manifest (sast-sca-auditor).**
   - Spec says `web/package-lock.json`. Actually in **root** `package-lock.json` via `axios@1.15.2` (root devDep). `web/` is clean.
   - The nightly `vulnerability-scan.yml` only audits `web/`, so root has zero CI coverage outside Dependabot itself.
   - **Action change:** Update root `package.json`, not `web/`. Add root manifest to nightly scan.

### Architectural disagreements

4. **The proposed `pathvalidation` package is too weak (arch-design).**
   - Spec proposes a package of free functions (`SafeJoin`, `WithinRoot`, etc.). Reviewer says: ship a typed-boundary `safepath.Root` + `safepath.SafePath` newtype with wrapped FS methods (`Open`, `ReadFile`, `ExtractInto`), enforced by a `golangci-lint` forbidigo rule banning raw `os.Open/Create/ReadFile` and `filepath.Join` outside `internal/security/safepath`.
   - Same pattern needed for SSRF (`safehttp.Client`) and logging (`seclog.Secret`/`PII` `LogValuer`s).
   - The type system, not human reviewers, must enforce the boundary.

5. **The plan is back-loaded (arch-design).**
   - Largest single drop is Phase 4. Recommend swapping Phases 3 and 4 to ship more value sooner.

6. **Plan ships fixes without a regression gate (arch-design).**
   - Recommend a `.github/workflows/security-gate.yml` that fails any PR increasing open code-scanning alerts. Converts "fix once" into "never regress."

### Documentation issues (code-review)

7. **Phase count discrepancy** — implementation plan says "11 phases" but the document contains 12 headers (Phase 0–11). Likely counting only remediation phases (1–11). Cosmetic.

## Audit blind spots — categories the original audit missed entirely

### From `SE: Security`

| Blind spot | Citation | Why CodeQL missed it |
|---|---|---|
| **Unauthenticated SSE event stream** at `GET /api/events` | `server_lifecycle.go:787` | CodeQL has no rule for "route registered outside auth middleware." Leaks real-time operation, file-move, and system-shutdown events to anonymous callers. |
| **OWASP LLM Top 10 entirely absent** | `internal/openai/openai_parser.go:218` sends raw OS filesystem paths (with usernames/PII) to OpenAI with zero sanitization | No LLM-aware static analysis in default CodeQL pack. Enables LLM06 (info disclosure) + LLM01 (prompt injection via crafted filenames). |
| **In-memory login lockout** | `auth_handlers.go:33–35` (process-local map) | Combined with uncontrolled-allocation alerts at `scanner.go:219,339`, attacker can OOM the server to reset lockouts. Audit lists each alert in isolation, never chains them. |

### From `arch-design`

| Blind spot | Why it matters |
|---|---|
| **No threat model document** | Fix decisions are being made per-alert rather than per-asset. Likely mis-prioritizes (unauthenticated OPDS/cover endpoints probably outrank admin-only iTunes transfer flows). |
| **No structured security-event channel** | After remediation, there's no signal: rejection rates, SSRF blocks, zip-slip rejects all go silent. Recommend `internal/security/seclog` + `event=security` slog handler. |
| **No ADRs for the remediation pattern decisions** | 12 ADRs needed (listed in `arch-design.md` §7) before Phases 3–6 fan out. |

### From `sast-sca-auditor`

| Blind spot | Why it matters |
|---|---|
| **Root `package-lock.json` not in CI scan** | nightly `vulnerability-scan.yml` only audits `web/`. |
| **No `osv-scanner` or `trivy fs` in the scanning stack** | Would catch transitive vulns Dependabot might miss. |
| **Compliance posture** | Original audit didn't address PCI-DSS / OWASP ASVS L1 even at a "we are/aren't in scope" level. |

## Production readiness — consolidated verdict

**Not production-ready** (per `SE: Security`). Blocking issues:
- 217 open path-injection alerts (systemic file-traversal surface)
- Unauthenticated SSE event leak (`server_lifecycle.go:787`)
- LLM prompt-injection / PII disclosure (`openai_parser.go:218`)
- In-memory lockout vulnerable to OOM-reset chain

The phased remediation plan is sound but needs the amendments below before execution.

## Recommended amendments to the original audit

In priority order (apply before kicking off Phase 1):

1. **Update Phase 0** — drop govulncheck binary-mode workaround; just remove the obsolete `GOEXPERIMENT=jsonv2` env var. (~30 min instead of 1 hr.)
2. **Insert new Phase 0.5** — build a CodeQL custom-sanitizer pack registering `internal/util.SafeJoin` / `WithinRoot` and re-run code scanning. Will likely reclassify ~80 of the 217 path-injection alerts as fixed/dismissed-false-positive.
3. **Rewrite Phase 1** — replace the free-function `pathvalidation` package with a typed-boundary `internal/security/safepath` (`Root` + `SafePath` newtype with wrapped FS methods) + `golangci-lint` forbidigo rule. Same shape for `safehttp` (SSRF) and `seclog` (PII logging).
4. **Add Phase 1.5: Threat model + ADRs.** Write `docs/security/threat-model.md` and ADRs 0001–0012 from `arch-design.md` §7.
5. **Add new phase: LLM security.** Cover `internal/openai/openai_parser.go` — sanitize filename/metadata before prompts, validate responses, redact PII.
6. **Add new phase: SSE/auth coverage.** Audit every route registration to confirm middleware is applied; add a unit test that fails on unauthenticated route registration.
7. **Add new phase: auth-chain hardening.** Move login lockout to durable storage, cap upload allocations, address the OOM-reset chain.
8. **Add new phase: regression gates.** `.github/workflows/security-gate.yml` (fail PRs that increase open alert count) + add root `package.json` to `vulnerability-scan.yml`.
9. **Fix Dependabot #27 in root** `package-lock.json`, not `web/`.
10. **Swap original Phases 3 and 4** to front-load value delivery.

## Confidence score grid

| Reviewer | Score | What they couldn't verify |
|---|---|---|
| sast-sca-auditor | 7/10 | Only 5% sampling on path-injection cluster; `osv-scanner` couldn't install against `replace`-heavy `go.mod`; no dynamic testing |
| SE: Security | (not numeric, "NO" verdict) | LLM tests rely on code reading, not runtime exercise |
| arch-design-reviewer | 5/10 plan as-written, 8–9/10 amended | n/a — design recommendations not yet executed |
| code-review | 9/10 | Did not execute govulncheck locally; only spot-checked 10 of 602 alerts |

## Next-step decision

Two reviewers (sast-sca-auditor, arch-design-reviewer) recommend **deferring bulk path-injection fixes** until after the CodeQL sanitizer pack and the typed-boundary refactor land — otherwise we'll fix ~120 false positives by hand. Treat amendments 1–4 above as the new Phase 0 work.
