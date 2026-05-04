<!-- file: docs/security/audit-2026-05-03/spec.md -->
<!-- version: 1.0.0 -->
<!-- guid: 60ffcab8-352a-44e9-8e21-9b07d7143474 -->
<!-- last-edited: 2026-05-03 -->

# Security Alert Audit Specification — 2026-05-03

## Executive Summary

This document provides a complete inventory and actionable remediation plan for all GitHub security alerts in the `jdfalk/audiobook-organizer` repository as of May 3, 2026.

### Alert Totals

| Category | Total Alerts | Open | Dismissed | Fixed |
|----------|--------------|------|-----------|-------|
| **Code Scanning** | 602 | 235 | 17 | 350 |
| **Dependabot** | 20 | 1 | 0 | 19 |
| **Secret Scanning** | 0 | 0 | 0 | 0 |
| **TOTAL** | **622** | **236** | **17** | **369** |

### Severity Breakdown (Open Alerts Only)

| Severity | Code Scanning | Dependabot | Total |
|----------|---------------|------------|-------|
| **Critical** | 0 | 0 | 0 |
| **Error/High** | 231 | 0 | 231 |
| **Warning/Medium** | 4 | 1 | 5 |
| **Note/Low** | 0 | 0 | 0 |

### Key Findings

1. **Path Injection Dominance:** 217 of 235 open code scanning alerts (92.3%) are `go/path-injection`, indicating systemic issues with untrusted input flowing into filesystem operations.

2. **Govulncheck Blocker:** The repository uses `GOEXPERIMENT=jsonv2`, which is documented in `.github/workflows/vulnerability-scan.yml` (lines 30-33). However, this workflow does NOT use a reusable workflow from `jdfalk/ghcommon` — it runs govulncheck directly. The jsonv2 experiment may cause govulncheck to skip or error because it sees the build environment as incompatible.

3. **Single Dependabot Alert:** Only one open npm vulnerability in `follow-redirects` (transitive dev dependency).

4. **No Secret Leaks:** Secret scanning returned an empty array (either disabled or no secrets detected).

5. **Historical Progress:** 369 fixed alerts demonstrate ongoing security hardening.

---

## Detailed Alert Inventory

### 1. Code Scanning Alerts (235 Open)

#### 1.1 Path Injection (217 alerts, error severity)

**Rule:** `go/path-injection`  
**Description:** Uncontrolled data used in path expression  
**Impact:** Potential directory traversal, arbitrary file access

**Affected Areas:**
- **File operations** (`internal/fileops/`) — 9 alerts
- **iTunes handlers** (`internal/server/itunes_handlers.go`) — 9 alerts  
- **Cover image handling** (`internal/server/covers.go`, `internal/server/cover_history.go`) — 9 alerts
- **iTunes transfer service** (`internal/itunes/service/transfer.go`) — 4 alerts
- **Scanner service** (`internal/scanner/service.go`) — 4 alerts
- **Audiobook handlers** (`internal/server/audiobooks_handlers.go`, `internal/audiobooks/service.go`) — 3 alerts
- **Reconciliation** (`internal/reconcile/reconcile.go`) — 3 alerts
- **OpenLibrary service** (`internal/server/openlibrary_service.go`) — 3 alerts
- **Server core** (`internal/server/server.go`, `internal/server/openlibrary_service.go`) — 2 alerts
- **Backup** (`internal/backup/backup.go`) — 1 alert
- **Importer** (`internal/importer/service.go`) — 1 alert
- **Deluge import** (`internal/server/deluge_import_unix.go`) — 2 alerts
- **Safe operations** (`internal/fileops/safe_operations.go`, `internal/fileops/hash.go`, `internal/fileops/write_tags_safe.go`) — 5 alerts
- **Many others** scattered across the codebase

**Sample Alerts:**
- #627: `internal/server/itunes_handlers.go:837`
- #625-620: `internal/fileops/service.go` (multiple lines)
- #615: `internal/server/server.go:703`
- #602-594: `internal/server/covers.go` and `internal/server/cover_history.go` (multiple)
- #543, #542: `internal/fileops/safe_operations.go:207, 136`
- #541: `internal/backup/backup.go:367`
- #539: `internal/fileops/hash.go:16`
- #538-536: `internal/fileops/write_tags_safe.go` (multiple)
- #535, #534: `internal/server/deluge_import_unix.go:54, 29`

**Recommended Action:** **Fix systematically**

**Rationale:** These alerts represent a systemic vulnerability pattern. User-controlled input (from HTTP requests, database fields, or API responses) flows into filesystem operations without proper validation or sanitization. This enables:
- Directory traversal attacks (`../../etc/passwd`)
- Arbitrary file access outside intended directories
- Potential data exfiltration or file manipulation

**Remediation Strategy:**
1. **Create centralized path validation utilities** in `internal/security/pathvalidation/`:
   - `ValidateRelativePath(input string, baseDir string) (string, error)` — ensures resolved path stays within baseDir
   - `SanitizeFilename(input string) (string, error)` — strips path separators, null bytes, control chars
   - `ValidateAgainstWhitelist(path string, allowedDirs []string) error`

2. **Apply validation at API boundaries** before paths reach business logic

3. **Use `filepath.Clean()` and `filepath.Join()` correctly** — ensure base directory is prepended AFTER cleaning user input

4. **Add unit tests** for each validation function covering traversal attempts

5. **Consider using `securejoin`** package (github.com/cyphar/filepath-securejoin) for robust path resolution

**Priority:** **P0 (Critical)** — This is the highest-impact category by volume and risk.

---

#### 1.2 Clear-Text Logging (6 alerts, error severity)

**Rule:** `go/clear-text-logging`  
**Description:** Clear-text logging of sensitive information  
**Impact:** Credential/sensitive data exposure in logs

**Alerts:**
- #530: `internal/server/maintenance_fixups.go:206`
- #529: `internal/server/maintenance_fixups.go:197`
- #528: `internal/server/maintenance_fixups.go:188`
- #527: `internal/server/maintenance_fixups.go:179`
- #526: `internal/server/maintenance_fixups.go:170`
- #47: `cmd/root.go:261`

**Recommended Action:** **Fix**

**Rationale:** Sensitive data (passwords, tokens, API keys, PII) must not be logged in clear text. Logs are often retained, aggregated, and accessible to multiple parties (operators, log aggregation systems, monitoring tools).

**Remediation:**
1. **Redact sensitive fields** before logging (use `[REDACTED]` or hash)
2. **Use structured logging** with explicit field exclusions (e.g., logrus field filters)
3. **Audit existing log statements** for accidental credential exposure
4. **Add linter rules** to catch future violations (e.g., `staticcheck` or custom CodeQL query)

**Priority:** **P1 (High)** — Direct exposure risk if logs are compromised.

---

#### 1.3 Request Forgery (4 alerts, error severity)

**Rule:** `go/request-forgery`  
**Description:** Uncontrolled data used in network request  
**Impact:** Server-Side Request Forgery (SSRF)

**Alerts:**
- #587: `internal/server/covers.go:53`
- #467: `internal/deluge/client.go:97`
- #458: `internal/plugins/webhook/plugin.go:128`
- #232: `internal/metadata/cover.go:48`

**Recommended Action:** **Fix**

**Rationale:** User-controlled URLs in HTTP requests enable SSRF attacks, allowing attackers to:
- Probe internal network resources (cloud metadata endpoints, internal services)
- Bypass firewall/network segmentation
- Exfiltrate data via out-of-band channels

**Remediation:**
1. **Validate URLs against a whitelist** of allowed domains/schemes (e.g., only `https://covers.openlibrary.org`)
2. **Block private IP ranges** (RFC 1918, loopback, link-local)
3. **Use a proxy** with egress filtering if dynamic URL fetching is required
4. **Implement timeout and size limits** on outbound requests

**Priority:** **P1 (High)** — SSRF can lead to internal network compromise.

---

#### 1.4 Uncontrolled Allocation Size (2 alerts, error severity)

**Rule:** `go/uncontrolled-allocation-size`  
**Description:** Slice memory allocation with excessive size value  
**Impact:** Denial of Service (OOM)

**Alerts:**
- #129: `internal/scanner/scanner.go:339`
- #44: `internal/scanner/scanner.go:219`

**Recommended Action:** **Fix**

**Rationale:** Allocating slices based on untrusted input enables memory exhaustion attacks. An attacker can send a request with a large size field, causing the server to OOM and crash.

**Remediation:**
1. **Cap allocation sizes** with reasonable maximums (e.g., `min(requested, maxAllowed)`)
2. **Validate input ranges** before allocation
3. **Use buffered reading/streaming** for large data instead of upfront allocation

**Priority:** **P2 (Medium)** — DoS vector, but requires specific attack conditions.

---

#### 1.5 Zip Slip (1 alert, error severity)

**Rule:** `go/zipslip`  
**Description:** Arbitrary file access during archive extraction ("Zip Slip")  
**Impact:** Directory traversal via malicious archive

**Alert:**
- #13: `internal/backup/backup.go:153`

**Recommended Action:** **Fix**

**Rationale:** Extracting archives without validating entry paths allows attackers to write files outside the intended directory (e.g., `../../etc/cron.d/malicious`).

**Remediation:**
1. **Validate each archive entry path** using `filepath.Clean()` and checking it starts with the extraction directory
2. **Use `securejoin`** or equivalent to resolve paths safely
3. **Reject entries with absolute paths or traversal sequences**

**Priority:** **P1 (High)** — Critical if archives come from untrusted sources.

---

#### 1.6 Disabled Certificate Check (1 alert, warning severity)

**Rule:** `go/disabled-certificate-check`  
**Description:** Disabled TLS certificate check  
**Impact:** Man-in-the-middle attacks

**Alert:**
- #379: `internal/mtls/provisioning.go`

**Recommended Action:** **Accept Risk (if intentional for mTLS testing) OR Fix**

**Rationale:** Disabling certificate validation in production code exposes connections to MITM attacks. However, this is in `mtls/provisioning.go`, which may be test/dev infrastructure.

**Remediation (if production code):**
1. **Enable certificate validation** with proper CA trust
2. **If self-signed certs are required**, pin the certificate or use a custom CA pool
3. **Never disable validation in production** without explicit, documented risk acceptance

**Priority:** **P2 (Medium)** — Context-dependent; investigate if this is prod or test code.

---

#### 1.7 Weak Sensitive Data Hashing (1 alert, warning severity)

**Rule:** `go/weak-sensitive-data-hashing`  
**Description:** Use of a broken or weak cryptographic hashing algorithm on sensitive data  
**Impact:** Credential compromise if hash is reversed/collided

**Alert:**
- #132: `internal/database/settings.go`

**Recommended Action:** **Fix**

**Rationale:** Weak hashing algorithms (MD5, SHA1) for passwords or sensitive data are vulnerable to precomputation (rainbow tables) and collision attacks.

**Remediation:**
1. **Use bcrypt, argon2, or scrypt** for password hashing
2. **If hashing non-password sensitive data**, use SHA-256 or SHA-3 (NOT MD5/SHA1)
3. **Migrate existing hashes** if possible (require password reset, or re-hash on next login)

**Priority:** **P1 (High)** — Credential security is critical.

---

#### 1.8 Allocation Size Overflow (1 alert, warning severity)

**Rule:** `go/allocation-size-overflow`  
**Description:** Size computation for allocation may overflow  
**Impact:** Incorrect allocation size, potential buffer overflow or panic

**Alert:**
- #468: `internal/itunes/itl.go`

**Recommended Action:** **Fix**

**Rationale:** Integer overflow in size calculations can lead to allocating smaller buffers than expected, causing out-of-bounds writes or panics.

**Remediation:**
1. **Check for overflow** before multiplication/addition (e.g., `if size > math.MaxInt64/factor { return error }`)
2. **Use `math/bits` overflow detection** or safe arithmetic libraries
3. **Validate input sizes** are within reasonable bounds

**Priority:** **P2 (Medium)** — Can cause crashes or memory corruption.

---

#### 1.9 Disabling Certificate Validation (JS, 1 alert, error severity)

**Rule:** `js/disabling-certificate-validation`  
**Description:** Disabling certificate validation  
**Impact:** MITM attacks in JavaScript code

**Alert:**
- #160: `scripts/record_demo.js:23`

**Recommended Action:** **Accept Risk (test script) OR Fix**

**Rationale:** This is in `scripts/record_demo.js`, likely a development/demo script, not production code.

**Remediation (if needed):**
1. **Remove certificate bypass** if script runs in sensitive environments
2. **Document that the script is dev-only** and should not be deployed

**Priority:** **P3 (Low)** — Dev script; low production impact.

---

#### 1.10 Incomplete Sanitization (JS, 1 alert, warning severity)

**Rule:** `js/incomplete-sanitization`  
**Description:** Incomplete string escaping or encoding  
**Impact:** Potential XSS or injection

**Alert:**
- #50: `web/src/pages/Settings.tsx`

**Recommended Action:** **Fix**

**Rationale:** Incomplete escaping in React components can lead to XSS if user-controlled data is rendered without proper sanitization.

**Remediation:**
1. **Use React's JSX auto-escaping** (avoid `dangerouslySetInnerHTML`)
2. **If HTML rendering is required**, use a sanitization library like DOMPurify
3. **Validate and encode** user input before rendering

**Priority:** **P2 (Medium)** — XSS risk if user data flows through this component.

---

### 2. Dependabot Alerts (1 Open)

#### 2.1 follow-redirects Header Leak (1 alert, medium severity)

**Package:** `follow-redirects`  
**Ecosystem:** npm (transitive dev dependency)  
**Vulnerable Range:** `<= 1.15.11`  
**Fixed Version:** `1.16.0`  
**CVE/GHSA:** `GHSA-r4q5-vmmm-2653`  
**Description:** follow-redirects leaks Custom Authentication Headers to Cross-Domain Redirect Targets

**Alert:**
- #27: `web/package-lock.json`

**Recommended Action:** **Fix**

**Rationale:** The package leaks authentication headers across domain boundaries during HTTP redirects, potentially exposing credentials to third-party sites.

**Remediation:**
1. **Update `follow-redirects` to 1.16.0+** via `npm update` in `web/`
2. **Verify transitive dependency update** with `npm audit fix` or manual package-lock.json update
3. **Test frontend builds** to ensure no breaking changes

**Priority:** **P2 (Medium)** — Transitive dev dependency; lower risk than direct runtime dependencies.

---

### 3. Secret Scanning Alerts (0)

**Status:** No secrets detected (or secret scanning may be disabled).

**Recommended Action:** None

**Note:** The user mentioned a fake OpenAI key (`sk-test12345678`) in `config.yaml`. If secret scanning were to flag this, it should be marked as **"intentional placeholder / false positive"** and dismissed with:
- **Reason:** `false_positive`
- **Comment:** "Documented example key for testing; not a real credential."

---

## Dismissed Alerts Review

**Total Dismissed:** 17 code scanning alerts (all `go/path-injection`)

**Location:** All in `internal/maintenance/jobs/` (maintenance job scripts)

**Dismissed Reason:** `false positive`

**Dismissed Comments:**
- "Paths originate from the database or system config (RootDir/iTunesRoot), not HTTP" (3 alerts: #546, #545, #544)
- "we created a helper function it should look up." (1 alert: #549)
- "no comment" (13 alerts: #560-547, excluding those with specific comments)

**Recommended Action:** **Accept dismissals** (with caveat)

**Rationale:** The dismissals appear valid IF:
1. The paths in question genuinely originate from trusted sources (database, config files) and **never** from user input (HTTP requests, file uploads, API responses).
2. The database/config values themselves are not user-controllable (e.g., admin-only settings, not user-editable fields).

**Caveat:** The "no comment" dismissals (#560-547, excluding documented ones) lack justification. While they are in maintenance jobs (likely admin-triggered, not user-facing), the **lack of documentation is a concern**. If these are re-audited in the future, the rationale will be lost.

**Recommended Follow-Up:**
- **Add dismissal comments** to the "no comment" alerts retroactively (via `gh api -X PATCH ...`) with a brief justification (e.g., "Maintenance job: paths from DB only, not user input").
- **Document in code comments** near the flagged lines that paths are trusted (e.g., `// CodeQL: path from database (trusted)`).

---

## Govulncheck Blocker — GOEXPERIMENT=jsonv2

### Problem Statement

The repository's `go.mod` specifies `go 1.24.0`, but the codebase targets Go 1.25 features (per the `.github/instructions/go.instructions.md`). The build uses `GOEXPERIMENT=jsonv2` (a Go 1.25 experiment) to enable enhanced JSON support.

**Current Situation:**
- `.github/workflows/vulnerability-scan.yml` (lines 30-33) runs `govulncheck ./...` with `GOEXPERIMENT: jsonv2` set as an environment variable.
- **This workflow does NOT use a reusable workflow from `jdfalk/ghcommon`** — it runs govulncheck directly inline.
- The user reports that govulncheck "does NOT work" with jsonv2, seeing the package environment as incompatible and skipping/erroring.

### Root Cause

`govulncheck` analyzes Go binaries and source code for known vulnerabilities. When `GOEXPERIMENT=jsonv2` is set, the Go toolchain may:
1. Change build tags or constraints, making govulncheck think modules are for a different Go version/OS.
2. Produce binaries with altered symbols or imports, confusing the vuln scanner's pattern matching.
3. Fail to match vulnerability database entries that assume standard Go builds.

The jsonv2 experiment is not yet stable, so tooling (including govulncheck) may not fully support it.

### Impact

Without working govulncheck:
- **No automated Go vulnerability detection** for `golang.org/x/` packages, stdlib issues, or third-party Go deps.
- **Risk of undetected CVEs** in critical dependencies (crypto, net, etc.).
- **Compliance gaps** if security audits require vuln scanning.

### Proposed Solution (Does NOT Require ghcommon Changes)

Since the workflow runs govulncheck inline (not via ghcommon reusable workflow), the fix is local to this repository:

1. **Option A: Remove jsonv2 from govulncheck step**
   ```yaml
   - name: Run govulncheck
     run: govulncheck ./...
     # DO NOT set GOEXPERIMENT here
   ```
   **Pros:** Govulncheck works immediately.  
   **Cons:** Govulncheck scans code as-if built without jsonv2, potentially missing issues specific to that build mode.

2. **Option B: Run govulncheck on built binary**
   ```yaml
   - name: Build with jsonv2
     run: go build -o audiobook-organizer ./cmd/...
     env:
       GOEXPERIMENT: jsonv2

   - name: Run govulncheck on binary
     run: govulncheck -mode=binary ./audiobook-organizer
   ```
   **Pros:** Scans the actual deployed binary, including jsonv2 behavior.  
   **Cons:** Requires building first; may not catch source-level issues govulncheck usually finds.

3. **Option C: Wait for Go 1.25 release**
   Once Go 1.25 is stable and jsonv2 is no longer experimental, govulncheck should work. Update `go.mod` to `go 1.25` and remove `GOEXPERIMENT`.

**Recommended Approach:** **Option B** (scan built binary) for immediate coverage, then transition to **Option C** when Go 1.25 releases.

### User's Original Request (Re-examined)

The user initially requested adding **env injection support to a ghcommon reusable workflow**. However:
- The vulnerability-scan workflow **does not use ghcommon reusable workflows** (see grep output above).
- The reusable workflows found (`reusable-ci.yml`, `reusable-release.yml`, `reusable-ci-minimal.yml`) are for CI/build/release, not vuln scanning.

**Conclusion:** **No ghcommon changes needed for govulncheck.** The fix is entirely within `.github/workflows/vulnerability-scan.yml` in this repository.

**If ghcommon env injection is desired for other reasons** (e.g., passing `GOEXPERIMENT` to reusable CI workflows), that's a separate feature request. The relevant workflows are:
- `jdfalk/ghcommon/.github/workflows/reusable-ci.yml`
- `jdfalk/ghcommon/.github/workflows/reusable-ci-minimal.yml`
- `jdfalk/ghcommon/.github/workflows/reusable-release.yml`

**Proposed Input (if implementing in ghcommon):**
```yaml
inputs:
  build_env_vars:
    description: 'Additional environment variables for build steps (KEY=VALUE, newline-separated)'
    required: false
    type: string
    default: ''
```

**Usage in job:**
```yaml
- name: Set custom env vars
  if: inputs.build_env_vars != ''
  run: |
    echo "${{ inputs.build_env_vars }}" >> $GITHUB_ENV
```

But again: **This is NOT needed for the govulncheck fix described above.**

---

## Acceptance Criteria

The security audit is complete when:

1. ✅ **All 236 open alerts are addressed** (fixed or consciously dismissed with documented reason).

2. ✅ **Govulncheck runs successfully** on the jsonv2 build (via binary mode or post-Go-1.25-release).

3. ✅ **Path injection alerts (217) are fixed systematically** via centralized validation utilities and boundary checks.

4. ✅ **Clear-text logging (6), request forgery (4), allocation issues (3), zipslip (1), and other errors are fixed** per priority.

5. ✅ **Dependabot alert #27 (follow-redirects) is fixed** via npm update.

6. ✅ **Dismissed alerts (17) are reviewed** and comments are added to "no comment" dismissals.

7. ✅ **All fixes pass CI** (`make ci`, including tests and coverage ≥80%).

8. ✅ **Post-remediation audit** confirms zero open alerts (or only accepted-risk alerts with documented rationale).

9. ✅ **Documentation updated** to reflect new path validation utilities and security policies.

---

## Severity Priority Guide

| Priority | Severity | Example Rules | SLA |
|----------|----------|---------------|-----|
| **P0** | Critical / Error (High Volume) | go/path-injection (217 alerts) | 30 days |
| **P1** | Error / High | go/clear-text-logging, go/request-forgery, go/zipslip, go/weak-sensitive-data-hashing | 60 days |
| **P2** | Warning / Medium | go/uncontrolled-allocation-size, go/allocation-size-overflow, go/disabled-certificate-check, js/incomplete-sanitization, Dependabot medium | 90 days |
| **P3** | Low / Info | js/disabling-certificate-validation (dev script) | 180 days |

---

## Next Steps

See **`implementation-plan.md`** for the phased remediation plan with concrete tasks, commands, and dependencies.

---

*Document Version: 1.0.0*  
*Audit Date: 2026-05-03*  
*Raw Data: `docs/security/audit-2026-05-03/raw/`*
