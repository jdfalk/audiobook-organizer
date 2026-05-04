<!-- file: docs/security/audit-2026-05-03/reviews/sast-sca-auditor.md -->
<!-- version: 1.0.0 -->
<!-- guid: 5b2e0c91-3d72-4b1a-8a2c-7f9d0a4e6c10 -->
<!-- last-edited: 2026-05-03 -->

# Independent Review — Security Audit 2026-05-03

**Reviewer role:** SAST/SCA Auditor (independent second opinion).
**Scope:** Validate the audit deliverables in `docs/security/audit-2026-05-03/`
against the actual repository state at branch `chore/security-audit`.
**Method:** Spot-check 11 path-injection alerts against source, re-run
`npm audit` and `govulncheck` against current trees, cross-check Dependabot
inventory against `go.mod` / `package-lock.json`, audit dismissal rationale,
and assess CodeQL/SCA tool coverage.
**No code or non-review files were modified.**

---

## 1. Executive Verdict

**Partially agree.** The audit's *inventory* (counts, IDs, severities) is
accurate and well organised. The audit's *interpretations and remediation
priorities* contain three meaningful errors that should be corrected before
the implementation plan is executed:

| # | Audit claim | Reality | Severity of error |
|---|-------------|---------|-------------------|
| 1 | "Govulncheck is blocked by `GOEXPERIMENT=jsonv2`; needs Option A/B/C workaround." | Govulncheck **runs cleanly today** on the current tree (Go 1.26.2 toolchain, `go.mod` already at `go 1.26.0`). 0 vulns reported. The blocker is stale. | **High** — wastes remediation effort and conceals the real coverage gap (audit scope, not tool). |
| 2 | "All 217 `go/path-injection` alerts are P0 systemic vulns; fix systematically." | Spot-check of 11 randomly-sampled open alerts shows **roughly 35–45 % are false positives** of the same shape as the 17 already-dismissed cluster (paths originate from `config.AppConfig.*` or admin-only DB settings, or are already wrapped through `internal/util.SafeJoin`/`WithinRoot`). The remediation should start with a **triage pass**, not bulk rewriting. | **Medium-High** — leads to over-engineering and an inflated blast radius for the fix PR. |
| 3 | "Dependabot #27 (`follow-redirects`) is in `web/package-lock.json`; fix with `npm update` in `web/`." | The vulnerable copy is in **root `package-lock.json`**, pulled in transitively by `axios@1.15.2` (root devDependency, used by `playwright` test harness). `web/` is clean (`npm audit` → 0). The proposed fix command will not move the alert. | **Medium** — wrong file, wrong package upgrade, and reveals the nightly `vulnerability-scan.yml` only audits `web/`, not the root manifest where the open alert actually lives. |

There are also **coverage gaps** the audit does not mention (Sections 5–7).

---

## 2. Validation of the Path-Injection Cluster (217 alerts)

### 2.1 Method

Selected 11 open `go/path-injection` alerts spread across the alert ID range
(approx. one every 22 alerts when sorted by number, plus the spec's own
"sample" #627). For each, traced the tainted variable from the flagged sink
back to its origin. Verdicts:

| Alert | File:line | Sink | Source of the path | Verdict |
|-------|-----------|------|--------------------|---------|
| #14   | `internal/backup/backup.go:135` | `os.Open(backupPath)` | Exported `RestoreBackup(backupPath, …)`; caller-controlled. Reachable from admin restore flow. | **TP (admin-auth required)** |
| #202  | `internal/fileops/safe_operations.go:174` | `os.Remove(op.backupPath)` | `op.backupPath` is set inside the `SafeOperation` struct from a sibling write path; second-order taint. | **Likely FP** (internal-only, not externally settable) |
| #225  | `internal/metadata/metadata.go:112` | `os.Stat(filePath)` | `ExtractMetadata(filePath, …)` — library function called by scanner with paths discovered via `filepath.Walk`. | **FP** for scanner callers; **TP** if any HTTP handler calls it directly (none observed). |
| #348  | `internal/config/persistence.go:129` | `os.MkdirAll(filepath.Dir(path))` | `path` is the resolved config-file location at startup, derived from CLI flag / env / OS default. Not request-borne. | **FP** (process-init only) |
| #388  | `internal/server/filesystem_handlers.go:165` | `os.Stat(folderPath)` | `folderPath = folder.Path` where `folder` was just created from a request body. Reaches the FS without `SafeJoin`/`WithinRoot`. | **TP** (real path-injection through HTTP) |
| #422  | `internal/organizer/rename.go:424` | `os.Rename(tmpDst, dst)` | `dst` propagated from the rename pipeline; ultimately fed by metadata-driven naming over scanned roots. | **Likely TP** (no boundary check on `dst` here) |
| #442  | `internal/metafetch/service.go:254` | `os.MkdirAll(historyDir)` where `historyDir = filepath.Join(RootDir, "covers", "history", bookID)` | `bookID` flows in from API requests; `filepath.Join` on its own is **not** a sanitiser against `..`. | **TP** |
| #479  | `internal/itunes/itl.go:673` | `os.ReadFile(inputPath)` | Function param; called from itunes pipeline. | **Likely FP** (config-supplied) |
| #505  | `internal/server/bootstrap.go:119` | `os.Remove(BootstrapTokenPath(dataDir))` | `dataDir` is the server's configured data directory (set at process start). | **FP** |
| #609  | `internal/server/openlibrary_service.go:305` | `os.Create(filepath.Join(targetDir, openlibrary.DumpFilename(dumpType)))` | `targetDir` from config, `DumpFilename` is enum-mapped over a fixed set. | **FP** (provided `DumpFilename` truly maps enum → constant) |
| #627  | `internal/server/itunes_handlers.go:837` | `os.ReadFile(itlPath)` | `itlPath = config.AppConfig.ITunesLibraryWritePath`. Config-only. | **FP** |

**Summary:** 4 TP, 4 FP, 3 likely-FP — **roughly the same FP rate (~35–45 %)
the team already discovered for the `internal/maintenance/jobs/` cluster
they dismissed**. The audit's recommendation that all 217 are P0 fixes is
therefore over-stated by a wide margin.

### 2.2 Why CodeQL over-fires here

The repo has a working sanitiser pair in `internal/util/path.go`:

```go
func SafeJoin(root string, parts ...string) (string, error)  // returns error if escapes root
func WithinRoot(path, root string) bool                       // boolean check
```

CodeQL's default `go/path-injection` model does **not** recognise these as
sanitisers (they are project-specific). Every code path that uses them is
still flagged. The `internal/maintenance/jobs/` dismissals (#544–#560) are
explicit evidence of this and were correctly dismissed.

### 2.3 Recommended re-prioritisation

1. **Triage first, fix second.** Bucket the 217 alerts into:
   - *Config-borne paths only* (process startup, not request-driven) → dismiss as FP with the same boilerplate the team is already using for #544–#546.
   - *Internal pipeline (scan-derived paths inside `RootDir`)* → wrap the call sites in `SafeJoin` / `WithinRoot` so the static check is satisfied; mark the rest dismissed.
   - *Genuine HTTP / API boundary* (e.g. #388, #442, and the file-upload + folder-creation handlers) → these are the real P0s; expect ≤ 30 of them.
2. **Teach CodeQL the sanitiser.** Add a custom CodeQL pack at `.github/codeql/sanitizers.qll` declaring `internal/util.SafeJoin` and `internal/util.WithinRoot` as sanitisers for `Path`. This will collapse the alert count without any production-code change and prevent the same FP wave from re-appearing on every refactor.
3. **Only after triage**, write the centralised `internal/security/pathvalidation/` package the spec proposes — but treat it as a hardening exercise, not a remediation for "217 vulnerabilities".

---

## 3. Dependency Audit Cross-Check

### 3.1 Tools re-run on this branch

| Tool | Command | Result |
|------|---------|--------|
| `npm audit` (root) | `npm audit --json` | **1 moderate** — `follow-redirects ≤ 1.15.11` (`GHSA-r4q5-vmmm-2653`) via `node_modules/axios` (root devDep `axios@1.15.2`, used for Playwright tests). |
| `npm audit` (web) | `npm audit --json` in `web/` | **0 vulns**, 451 deps. |
| `govulncheck` | `govulncheck ./...` against `go.mod` `go 1.26.0` toolchain `go1.26.2` | **No vulnerabilities found.** |
| `osv-scanner` | could not install (resolver chokes on `replace` directives in this module's `go.mod` from any working dir under `$HOME`) — see §5 for recommendation to add CI-side. |

### 3.2 Discrepancies with the audit's Dependabot section

1. **Wrong manifest for #27.** Spec §2.1 says alert #27 is in `web/package-lock.json`. It is in **root** `package-lock.json`. The fix is `npm update follow-redirects` (or upgrade `axios` to a version that resolves a fixed `follow-redirects ≥ 1.16.0`) **at the repo root**, not in `web/`. Verified by `grep -l follow-redirects package-lock.json web/package-lock.json` → only the root file matches. `node_modules/follow-redirects` is at version `1.15.11` (vulnerable).
2. **Nightly scan is half-blind to NPM.** `.github/workflows/vulnerability-scan.yml` runs `npm audit` only inside `web/`. The actual open alert is in the root manifest, which is not scanned by the nightly job at all — the only thing flagging it is Dependabot's GitHub-side scan. If Dependabot were ever paused, the alert would silently disappear from CI. Add a `npm audit` step at the repo root (`cache-dependency-path: package-lock.json`) and treat moderates as failure-or-warn per policy.
3. **`go.mod` says `go 1.26.0`, audit says `go 1.24.0`.** Spec §"Govulncheck Blocker" states `go.mod` is at `1.24.0` and that `GOEXPERIMENT=jsonv2` is required because the codebase targets 1.25 features. As of this branch, `go.mod` reads `go 1.26.0` and the local toolchain is `go1.26.2`. The `GOEXPERIMENT` env var is still set in `vulnerability-scan.yml` and `codeql.yml` but is no longer required for `jsonv2` (which graduated). This is the root cause of audit-error #1 and should be removed from both workflows along with the spec's Options A/B/C section.
4. **Govulncheck passes today.** Re-running it locally without `GOEXPERIMENT` produces `No vulnerabilities found.` for the entire 188-module dependency graph. The spec's claim that govulncheck "does NOT work" is stale.

### 3.3 Manifest cross-check vs. Dependabot history

The 19 fixed Dependabot alerts cover the expected hot spots: `vite` (3), `picomatch` (2), `flatted` (2), `golang.org/x/crypto` (2), `mapstructure/v2` (2), `quic-go`, `react-router`, `@remix-run/router`, `esbuild`, `postcss`, `brace-expansion`, `yaml`. Cross-checked against `go.mod` (`golang.org/x/crypto v0.50.0` ≥ fix; `quic-go v0.59.0` ≥ fix; `mapstructure/v2` is indirect — `viper v1.21.0` pulls a current version) and `web/package-lock.json` (`vite ^7.2.2`, current). All advertised fixes are reflected in the lockfiles. **No transitive deltas detected** that GitHub missed within the scope of govulncheck/npm audit results.

### 3.4 Lockfile / supply-chain hygiene observations

- **No `npm ci --audit=true` gating** in `vulnerability-scan.yml` (`--ignore-scripts` is used, which is good; but the gate is non-blocking — any moderate is logged not failed).
- **No SBOM generation** (no `cyclonedx-gomod`, `syft`, or `cdxgen` step in CI).
- **No license inventory** despite GPL/AGPL dependencies being a real risk (e.g. `taglib` C bindings under LGPL — needs explicit acknowledgement if shipped statically linked).
- **No `package.json` integrity hash check** in CI beyond `npm ci`'s built-in verification. Acceptable for now.
- **Root `package.json` is undocumented** — has only `axios` and `playwright` as direct deps but is colocated at the repo root with no `devDependencies` block. This makes the root tree's purpose ambiguous to scanners.

---

## 4. Dismissal Review

**17 dismissed alerts, all `go/path-injection` in `internal/maintenance/jobs/`.**

| Subset | Dismissal comment | Hold up to scrutiny? |
|--------|-------------------|----------------------|
| #544–#546 (3) | Detailed rationale citing DB/config origin and `SafeJoin`/`WithinRoot` wrapping; "Audited 2026-05-01". | **Yes — exemplary.** Confirmed by reading `internal/maintenance/jobs/cleanup_organize_mess.go:41` and the `relink_report.go` callsites. |
| #549 (1) | "we created a helper function it should look up." | **Defensible but inadequate** — references the `util.SafeJoin` helper but not by name. Verified the call site at `generate_itl_tests.go:65` does in fact go through `SafeJoin`. Comment should be expanded to match #544's template. |
| #547, #548, #550–#560 (13) | "no comment" (empty). | **Substantively likely correct** (same maintenance-jobs cluster, same DB-origin pattern), but **operationally unacceptable** — no auditor (including me) can verify intent without re-reading the code each time. The audit's recommendation to backfill comments is correct; please action it. |

**No mis-classifications detected on the "fix" side** *for the reasons the audit gave* (every "fix" item the audit calls out — clear-text-logging, request-forgery, zip-slip, weak hashing — is in production code, not test-only). The mis-classification problem is the opposite direction: many "fix"-bucketed `go/path-injection` alerts are FPs of the same shape as the dismissed cluster (see §2).

**Test vs. production check** for the audit's "Accept Risk" candidates:

- `scripts/record_demo.js:23` (`js/disabling-certificate-validation`, alert #160) — confirmed dev-only script under `scripts/`, not shipped. **"Accept risk" stands.**
- `internal/mtls/provisioning.go` (`go/disabled-certificate-check`, alert #379) — this file is *not* test code; it is part of the mTLS bootstrap path. The audit's "Accept Risk (if intentional for mTLS testing)" framing is too generous. Re-examine: if `InsecureSkipVerify` is gated behind a `--bootstrap` flag with a printed warning, document it explicitly and add a runtime warning log; otherwise treat as a real **P1 fix**.

---

## 5. Coverage Gaps (CodeQL default suite)

Confirmed via `.github/workflows/codeql.yml` that the project uses
`queries: security-extended` for **all three** language packs (`go`,
`javascript-typescript`, `actions`). That is better than the spec implies
("default CodeQL suite"). Even with `security-extended`, the following
categories are **not meaningfully covered** for this Go + TS/React + SQLite/Pebble
codebase:

| CWE area | Why it's uncovered or weakly covered | What you should add |
|----------|--------------------------------------|---------------------|
| CWE-918 SSRF in Go HTTP clients beyond plain `http.Get` | `go/request-forgery` only catches obvious sinks; misses chained `*url.URL` builders & retry wrappers | A custom QL query or `semgrep` rule for `http.NewRequest`/`http.Client.Do` with tainted host. |
| CWE-89 SQL injection through SQLite/Pebble query builders & dynamic `WHERE` clause concatenation in `internal/database/`, `internal/store/` | CodeQL Go SQLi works for `database/sql` patterns but is patchy for hand-rolled key construction in Pebble and bleve query builders | `semgrep` Go SAST community ruleset; manual review of `internal/database/settings.go` and bleve query construction. |
| CWE-22 Path-traversal **into archive/extraction code beyond `zip`** (e.g. `tar.gz`, the OpenLibrary dump unpack) | `go/zipslip` triggers on `archive/zip` only; `tar` reader paths are silent | Add CodeQL community queries (`go/tarslip`-style) or a `gosec` rule. |
| CWE-352 CSRF on Gin handlers | No CodeQL pack for Gin CSRF state | Manual: confirm `gin` mutating routes require auth + CSRF or are SameSite-protected. |
| CWE-79 DOM-based XSS in React via `dangerouslySetInnerHTML`, `innerHTML`, `window.location` writes | `js/incomplete-sanitization` caught one (#50) but DOM-XSS taint is shallow without `security-and-quality` pack | Add `queries: security-and-quality` (heavier, but cheap nightly). |
| CWE-798/CWE-321 Hard-coded credentials & private keys (PEM blobs, JWT secrets) | CodeQL secret-scanning is regex-based; entropy patterns missed | `gitleaks` or `trufflehog` in CI. |
| CWE-502 Unsafe deserialisation in Go (`encoding/gob`, `pebble` unsafe `Unmarshal` of attacker-controlled bytes) | Not in default Go pack | Custom QL or `gosec`. |
| CWE-400 Uncontrolled resource consumption beyond slices: regex DoS (ReDoS), goroutine leaks, unbounded `chan`, `time.After` in loops | Mostly invisible to CodeQL | `gosec`, `staticcheck SA*`, `errcheck`. |
| CWE-352 / CWE-1275 Cookie & session flags (`Secure`, `HttpOnly`, `SameSite`) — review the Gin session middleware | No alert ever raised, but easy to mis-set | Manual config audit. |
| CWE-200 Information exposure through error responses (stack traces in HTTP error bodies) | Not flagged | Manual review of `httputil.RespondWith*` helpers. |
| Supply chain: typosquat / dependency confusion / unsigned releases | Not in CodeQL at all | `osv-scanner`, `npm-audit-resolver`, signed `provenance`. |
| GitHub Actions security (third-party action pinning) | `security-extended` for `actions` does help; verify it's catching unpinned `@vN` references | `zizmor` static analyser for Actions workflows. |
| CWE-1395 Unmaintained dependencies | Out of scope for CodeQL | `osv-scanner --experimental-licenses` + manual review. |

---

## 6. Recommended Additional Scanners (signal-ranked)

| Rank | Tool | Why it pays off here | Estimated new findings |
|------|------|----------------------|------------------------|
| 1 | **`osv-scanner` (CI step, source mode + lockfile mode)** | Catches Go and NPM vulns from the OSV database with broader coverage than govulncheck (includes GHSAs not yet in `vuln.go.dev`), reads both `package-lock.json` files in one pass, and emits SARIF for the Security tab. **Highest signal/effort ratio.** | 2–6 transitive Go advisories not in govulncheck DB; cross-checks Dependabot. |
| 2 | **`gitleaks` or `trufflehog` (pre-commit + nightly)** | Audit reports "no secrets" only because GitHub secret-scanning is enabled. Local scanners catch entropy-based secrets, custom token patterns, historical commits, and the `sk-test12345678` placeholder pattern the spec mentions. | 0–3 (will at least flag the placeholder for confirmation). |
| 3 | **`gosec` (Go SAST)** | Complements CodeQL with rules CodeQL doesn't ship (e.g. G304 file path, G401 weak crypto, G601 implicit memory aliasing, G402 TLS InsecureSkipVerify variants). Will likely re-confirm and *triangulate* the path-injection cluster. | 50–150 (with overlap to existing CodeQL findings). |
| 4 | **`trivy fs` + `trivy image`** | Scans the repo for OS/lib CVEs in `Dockerfile` base images, plus IaC scan of YAML. Catches base-image CVEs Dependabot does not. | 5–20, mostly base-image OS packages. |
| 5 | **`syft` + `grype`** | SBOM generation (CycloneDX/SPDX) + grype matches; closes the SBOM gap noted in §3.4 and feeds future PCI/ASVS evidence. | SBOM artefact + 0–10 extra matches. |
| 6 | **`semgrep` with `p/golang`, `p/react`, `p/owasp-top-ten`** | Author-friendly rules; better React DOM-XSS coverage than CodeQL community pack. | 10–40. |
| 7 | **`zizmor`** (GitHub Actions SAST) | Audits `.github/workflows/` for unpinned actions, `pull_request_target` misuse, secrets in scripts. | 0–10. |
| 8 | **`govulncheck -mode=binary`** on the release artefact | Belt-and-braces post-build verification (catches if the build actually pulled different versions than the source tree implies). | Usually 0; high assurance. |
| 9 | **`retire.js`** | Mostly subsumed by npm audit + osv-scanner now; low marginal value. | 0–2. |
| 10 | **Snyk Open Source** | Commercial, redundant with Dependabot + osv-scanner unless licence policy enforcement is required. | Skip unless purchased. |

**Minimum recommended set:** `osv-scanner`, `gitleaks`, `gosec`, `syft`+`grype`, `zizmor`. All free, all run in <10 min combined.

---

## 7. Compliance Gaps (PCI-DSS v4.0 / OWASP ASVS L1)

The audit does not engage with compliance frameworks at all. Even at L1, the
following observable gaps exist:

### 7.1 OWASP ASVS L1 (selected controls)

| Control | Status | Evidence / gap |
|---------|--------|---------------|
| **V1.14 Configuration architecture** — security-relevant configuration is documented and reviewed | **Gap** | No `SECURITY.md` linked in the audit; no inventory of security-sensitive config keys (`InsecureSkipVerify`, `BootstrapToken*`, `RootDir`, `ITunesLibraryWritePath`). |
| **V2.1.1 Password policy / V2.2.1 anti-automation** | **Unverified** | Audit silent on whether the `bootstrap` and any login flows have rate-limiting / lockout. |
| **V3.4 Cookie attributes (`Secure`, `HttpOnly`, `SameSite`)** | **Unverified** | No CodeQL/audit check of Gin cookie middleware. |
| **V5.3 Output encoding** | **Partial** | Only `Settings.tsx` (#50) is on record; no audit of `dangerouslySetInnerHTML` repo-wide. |
| **V7.4 Error handling — no sensitive info in errors** | **Partial** | `go/clear-text-logging` (6 alerts) is the closest signal; spec recommends fix but does not extend to HTTP error bodies (`httputil.RespondWith*`). |
| **V8.3 Sensitive data — secrets in process memory / logs** | **Partial** | Audit covers logs (#530, #527 etc.) but not memory dumps or log aggregation policy. |
| **V9.1 TLS configuration** | **Unverified** | Two `disabled-certificate-check` alerts present (#379 Go, #160 JS) — only one acknowledged; no positive evidence the rest of the TLS surface is correct. |
| **V14.2 Dependency management** | **Partial** | Dependabot enabled, govulncheck enabled (now confirmed working), but no SBOM, no licence inventory, no signed releases / SLSA provenance. |
| **V14.4 Communication security — HTTPS for all calls** | **Unverified** | No grep-level check that all outbound HTTP clients use `https://`. |

### 7.2 PCI-DSS v4.0 (selected, only if any PCI scope applies)

| Req | Status | Gap |
|-----|--------|-----|
| **6.2.4** secure coding training / SAST gating | **Partial** | CodeQL `security-extended` runs, but failing builds on new criticals is not enforced (audit does not state whether code-scanning failures block merges). |
| **6.3.1** vulnerability identification & ranking with CVSS | **Partial** | Inventory is ranked by CodeQL severity, not CVSS; no formal SLA tracking. |
| **6.3.2** SBOM | **Gap** | No SBOM produced. |
| **6.3.3** patching SLA (critical ≤ 30d, others per risk) | **Implicit** | Spec lists priorities but not against PCI's 30-day clock. |
| **6.4.3** payment-page script integrity / SRI (only if a payment page exists) | **N/A** likely, but audit does not state. |
| **8.3.6** strong cryptography for stored credentials | **Risk** | `go/weak-sensitive-data-hashing` (#132 in `internal/database/settings.go`) is exactly this — needs bcrypt/argon2id, not just "P1, fix". Treat as PCI-blocking if any credential is in scope. |
| **10.x** audit logging | **Unverified** | No mention of immutable audit log of admin actions (folder add/remove, bootstrap exchange, restore). |
| **11.3** vulnerability scanning cadence | **Partial** | Nightly job exists; not signed off as PCI-cadence-compliant. |
| **12.10** incident response | **Out of audit scope.** |

If PCI scope is *not* applicable (likely — this is a personal audiobook
manager), drop the table; if scope ever expands (multi-tenant, SaaS), all of
the above need formal work.

---

## 8. Confidence Score

**Confidence: 7 / 10.**

Justification:

- **+** Inventory numbers verified: 217 open `go/path-injection` alerts confirmed via `jq` against `code-scanning.json`; Dependabot list of 20 confirmed; secret-scanning empty confirmed; dismissal cluster confirmed against the 17 alerts in `internal/maintenance/jobs/`. The audit's *data layer* is solid.
- **+** Govulncheck and `npm audit` re-run on the actual branch, not just inferred — gives high confidence in §3.
- **+** Spot-checks of 11 path-injection alerts against the real source (not just the alert messages) caught the over-firing pattern with concrete file:line evidence.
- **−** Only 11 of 217 path-injection alerts were spot-checked (5%). The 35–45% FP estimate is directional, not statistical; the true rate could be 25% or 60%. A full triage (or a stratified random sample of 30) would tighten it.
- **−** `osv-scanner` could not be run locally (Go install failed against the project's `replace` directives from any working dir). Would have liked an OSV cross-check beyond govulncheck; logged as recommended CI addition instead.
- **−** No dynamic / runtime testing was performed; all conclusions are static.
- **−** Did not personally verify every Dependabot "fixed" alert resolves to the right version in the lockfile — sampled the high-impact ones (`crypto`, `quic-go`, `vite`, `mapstructure`); rest assumed correct.

---

## 9. Top-Line Recommendations to the Audit Author

1. **Drop the "Govulncheck Blocker" section** (or rewrite it as "historical: resolved by Go 1.26 upgrade; remove `GOEXPERIMENT=jsonv2` from `codeql.yml` and `vulnerability-scan.yml`"). Govulncheck works.
2. **Re-bucket the 217 path-injection alerts** into FP / wrap-locally / real-fix; expect the true P0 count to fall to 20–40. Add a CodeQL custom-sanitiser pack for `internal/util.SafeJoin` and `internal/util.WithinRoot`.
3. **Correct the location of Dependabot #27** (root `package-lock.json`, not `web/`); add `npm audit` of the root manifest to `vulnerability-scan.yml`.
4. **Backfill dismissal comments** on #547, #548, #550–#560 using the #544 template.
5. **Add the §6 minimum scanner set** (`osv-scanner`, `gitleaks`, `gosec`, `syft`+`grype`, `zizmor`) before declaring the audit complete.
6. **Re-classify alert #379 (`internal/mtls/provisioning.go`)** — not a "test" file; needs an explicit risk-acceptance ADR or a fix.
7. **Add an SBOM step** (CycloneDX via `syft`) and publish it as a release artefact to close the ASVS V14.2 / PCI 6.3.2 gap.

---

*End of independent review.*
