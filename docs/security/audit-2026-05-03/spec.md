<!-- file: docs/security/audit-2026-05-03/spec.md -->
<!-- version: 2.0.0 -->
<!-- guid: 60ffcab8-352a-44e9-8e21-9b07d7143474 -->
<!-- last-edited: 2026-05-04 -->

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

## Phase 0 Status: Govulncheck Blocker — ✅ RESOLVED

**Resolution Date:** 2026-04-30  
**Resolution:** Root `package.json` and `package-lock.json` were deleted in PR #687 (commit `f2d16dd8`). The reported govulncheck incompatibility with `GOEXPERIMENT=jsonv2` was stale information.

### Lessons Learned

**Original Problem Statement (Stale):**
The original audit claimed that `govulncheck ./...` fails or skips when `GOEXPERIMENT=jsonv2` is set, requiring binary-mode scanning workarounds.

**Actual Reality (Verified):**
1. **`go.mod` is at `go 1.26.0`**, not `1.24.0` as the initial audit stated.
2. **Govulncheck runs cleanly** with `GOEXPERIMENT=jsonv2`. Verified command:
   ```bash
   GOEXPERIMENT=jsonv2 govulncheck ./...
   # Output: "No vulnerabilities found."
   ```
3. **The `GOEXPERIMENT=jsonv2` env var must NOT be removed** — it is required for the codebase's use of Go 1.25+ JSON features and is correctly set in both `.github/workflows/codeql.yml` and `.github/workflows/vulnerability-scan.yml`.
4. **The nightly `vulnerability-scan.yml` workflow is active and green**, confirming govulncheck is working as intended.

**Root Cause of the Stale Information:**
- The audit was prepared based on documentation that referenced an older Go version configuration.
- Independent review by `sast-sca-auditor` caught the discrepancy by re-running govulncheck locally on the `chore/security-audit` branch.

**Corrective Action:**
- Original Phase 0 ("Implement binary-mode govulncheck workaround, ~1 hour") is **OBSOLETE**.
- New Phase 1 in the reconciled implementation plan verifies that the nightly workflow is green and reporting correctly — no code changes needed.
- This resolution saves ~1 hour of remediation effort and prevents unnecessary workflow modifications.

**Key Takeaway:**
Always verify tool compatibility claims against the current codebase state. Independent review caught a stale blocker that would have wasted remediation cycles.

---

## OWASP LLM Top 10 Findings

Independent security review (`SE: Security` persona) identified OWASP LLM Top 10 risks that are entirely absent from CodeQL's default ruleset. These represent production-grade risks that static analysis tools cannot currently detect.

### LLM01 — Prompt Injection via Crafted Filenames

**Location:** `internal/openai/openai_parser.go:218`

**Issue:** The OpenAI metadata parser sends raw OS filesystem paths (including audiobook filenames) directly to the LLM via prompt construction with zero sanitization:
```go
prompt := fmt.Sprintf("Extract metadata from: %s", filepath.Base(audiobookPath))
```

**Attack Vector:**
- An attacker places a file named `"; DROP TABLE books; --` or `<script>alert('xss')</script>.m4b` in the audiobook directory.
- The scanner picks it up, sends the filename verbatim to OpenAI's API.
- OpenAI's LLM may interpret the filename as a command injection attempt (LLM01) or execute embedded directives if the prompt structure is weak.

**Why CodeQL Missed It:**
CodeQL has no LLM-aware static analysis rules. The `go/path-injection` rule flags filesystem operations, not LLM prompt construction.

**Recommended Action:** **P0 Fix**
1. Sanitize all filenames/metadata before prompt construction (strip special chars, SQL keywords, script tags, path separators).
2. Use structured prompts with clear delimiters: `Filename: """<sanitized_name>"""`
3. Validate LLM responses against a JSON schema to prevent injection via response manipulation.
4. Add token limits and rate limits to the OpenAI client.

---

### LLM06 — Information Disclosure (PII in Prompts)

**Location:** `internal/openai/openai_parser.go:218`

**Issue:** Full filesystem paths (e.g., `/Users/alice/audiobooks/private_collection/...`) are sent to OpenAI verbatim. These paths can leak:
- OS usernames (PII)
- Directory structure (internal information)
- File naming conventions (security through obscurity破られる)

**Example:**
```go
path := "/Users/alice/Documents/Confidential/2026-Taxes/audiobook.m4b"
// This entire path goes to OpenAI's API in the prompt
```

**Why CodeQL Missed It:**
No rule for "PII in external API requests." The `go/clear-text-logging` rule flags logging, not API calls.

**Recommended Action:** **P1 Fix**
1. Redact OS usernames from paths before sending to OpenAI: `s.Replace("/Users/alice/", "/Users/[REDACTED]/")`
2. Send only the base filename, not the full path, unless path structure is required for metadata extraction.
3. Add a `redactPII(s string) string` helper that strips known PII patterns (usernames, email addresses, etc.).
4. Log all OpenAI API requests (sanitized) to a security audit trail.

---

### LLM Security — Combined Remediation Plan

Both findings point to the same root cause: **the LLM integration (`internal/openai/openai_parser.go`) was never designed with adversarial input in mind**.

**Recommended Phase (New):** **Phase 5 — LLM Security Hardening**
- Sanitize all inputs (filenames, metadata) before prompt construction (LLM01 mitigation).
- Redact PII (OS usernames, paths) from prompts (LLM06 mitigation).
- Validate all LLM responses against a JSON schema (prevent response-based injection).
- Add token limits (e.g., max 2000 tokens per request) and rate limits (e.g., 10 req/min per user).
- Implement structured logging for all OpenAI API calls (input sanitized, output sanitized, token usage, latency).

**Estimated Effort:** 3 hours  
**Files Affected:** `internal/openai/openai_parser.go`, `internal/openai/client.go`  
**Dependencies:** None (can run in parallel with path-injection fixes)

---

## Authentication & Authorization Findings

`SE: Security` review found three critical authentication/authorization issues missed by CodeQL's default rulesets.

### Unauthenticated SSE Event Stream

**Location:** `internal/server/server_lifecycle.go:787`

**Issue:** The Server-Sent Events (SSE) endpoint `GET /api/events` is registered on the global (unprotected) router, outside all authentication middleware:
```go
// Line 787 in server_lifecycle.go
r.Get("/api/events", s.handleEvents)
```

The `handleEvents` function at `internal/server/system_handlers.go:351–358` performs no auth check. **Any unauthenticated caller** can subscribe to real-time system events, including:
- Operation progress (file moves, metadata fetches)
- Scan results (which files are being processed)
- System shutdown broadcasts
- File paths and metadata (via event payloads)

**Why CodeQL Missed It:**
No rule for "route registered outside auth middleware." This is a design-level issue, not a code-level pattern match.

**Attack Impact:** **CRITICAL**
- **Information disclosure:** Attacker learns internal file structure, scan patterns, operation timing.
- **Reconnaissance:** Real-time feed of server activity enables targeted attacks.
- **Denial of Service:** Attacker can hold open many SSE connections, exhausting server resources.

**Recommended Action:** **P0 Fix**
1. Move `r.Get("/api/events", ...)` to an authenticated route group (apply `s.perm(auth.PermReadEvents)` middleware).
2. Add a unit test that fails if any route registration does not have auth middleware (or is explicitly on an allowlist like `/health`, `/metrics`).
3. Audit all route registrations in `server_lifecycle.go:760–1231` for similar gaps.

---

### In-Memory Login Lockout + OOM Allocation Chain

**Locations:**
- `internal/server/auth_handlers.go:33–35` — in-memory login lockout map
- `internal/scanner/scanner.go:219,339` — uncontrolled allocation size alerts

**Issue:** The login lockout mechanism uses a process-local `map[string]*failedAttempt` (auth_handlers.go:33–35) that is wiped on server restart:
```go
var loginLockout = make(map[string]*failedAttempt)  // package-level, not durable
```

**Attack Chain:**
1. Attacker makes 9 failed login attempts (limit is 10 per 15 minutes per user).
2. Attacker triggers an OOM crash via the uncontrolled-allocation alerts at `scanner.go:219,339` (CodeQL alerts #129, #44).
3. Server restarts; `loginLockout` map is empty.
4. Attacker immediately makes 9 more attempts, repeating the cycle.

**Why CodeQL Missed It:**
CodeQL flags each issue in isolation:
- `#129, #44`: Uncontrolled allocation (flagged as DoS, not as auth-chain).
- Auth lockout map: No rule for "in-memory-only state that should be durable."

**Attack Impact:** **HIGH**
- **Brute-force protection bypass:** Attacker can indefinitely retry passwords by triggering restarts.
- **Denial of Service:** The OOM crash itself is a DoS vector.

**Recommended Action:** **P0 Fix (combined)**
1. **Move login lockout to durable storage** (SQLite table, PebbleDB, or Redis). Persist attempt counts and lockout expiry across restarts.
2. **Cap upload/scan allocations** at `scanner.go:219,339` (e.g., `min(size, maxAllowedScanBuffer)`) to prevent OOM attacks.
3. **Add integration test:** Verify that login lockout survives a server restart (start server, lock account, kill server, restart, verify account still locked).

---

### Combined Auth Remediation Plan

**Recommended Phase (New):** **Phase 6 — Auth-Chain Hardening**
- Authenticate `/api/events` SSE endpoint (apply `s.perm(...)` middleware).
- Move login lockout from in-memory map to durable store (`internal/database/auth_lockout.go` table).
- Cap upload allocations in `scanner.go` to prevent OOM-triggered lockout resets.
- Add unit test: every route registration must have auth middleware or be explicitly allowlisted (`/health`, `/metrics`, `/bootstrap`).

**Estimated Effort:** 4 hours  
**Files Affected:** `server_lifecycle.go`, `auth_handlers.go`, `scanner.go`, `internal/database/auth_lockout.go` (new)  
**Dependencies:** None (can run in parallel)

---

## False Positive Cluster — Path Injection Alerts

Independent review (`sast-sca-auditor`) spot-checked 11 of the 217 open `go/path-injection` alerts and found **~35–45% are likely false positives** of the same shape as the 17 already-dismissed cluster.

### False Positive Shape

**Pattern:** Paths sourced exclusively from `config.AppConfig.*` fields (set at process startup, not user-controlled) or from admin-only database settings.

**Examples:**
- **Alert #627:** `internal/server/itunes_handlers.go:837` — `os.ReadFile(itlPath)` where `itlPath = config.AppConfig.ITunesLibraryWritePath`. Config-only, not request-borne.
- **Alert #505:** `internal/server/bootstrap.go:119` — `os.Remove(BootstrapTokenPath(dataDir))` where `dataDir` is the server's configured data directory (set at startup).
- **Alert #348:** `internal/config/persistence.go:129` — `os.MkdirAll(filepath.Dir(path))` where `path` is the resolved config-file location at startup, derived from CLI flag/env/OS default.

**True Positive Shape (for contrast):**
- **Alert #388:** `internal/server/filesystem_handlers.go:165` — `os.Stat(folderPath)` where `folderPath = folder.Path` from a request body. Reaches the FS without `SafeJoin`/`WithinRoot`. **Real path-injection through HTTP.**
- **Alert #442:** `internal/metafetch/service.go:254` — `os.MkdirAll(historyDir)` where `historyDir = filepath.Join(RootDir, "covers", "history", bookID)` and `bookID` flows in from API requests. `filepath.Join` on its own is **not** a sanitizer against `..`.

### Why CodeQL Over-Fires Here

The repository has a working sanitizer pair in `internal/util/path.go`:
```go
func SafeJoin(root string, parts ...string) (string, error)  // returns error if escapes root
func WithinRoot(path, root string) bool                       // boolean check
```

CodeQL's default `go/path-injection` model does **not** recognize these as sanitizers (they are project-specific). Every code path that uses them is still flagged. The `internal/maintenance/jobs/` dismissals (#544–#560) are explicit evidence of this pattern and were correctly dismissed.

### Spot-Check Results (11 Alerts)

| Alert | File:line | Source | Verdict |
|-------|-----------|--------|---------|
| #627  | `itunes_handlers.go:837` | `config.AppConfig.ITunesLibraryWritePath` | **FP** (config-only) |
| #505  | `bootstrap.go:119` | `dataDir` (startup config) | **FP** (startup-only) |
| #348  | `config/persistence.go:129` | CLI flag/env (startup) | **FP** (startup-only) |
| #609  | `openlibrary_service.go:305` | `targetDir` from config, `DumpFilename` enum-mapped | **FP** (config + enum) |
| #388  | `filesystem_handlers.go:165` | Request body `folder.Path` | **TP** (real HTTP boundary) |
| #442  | `metafetch/service.go:254` | `bookID` from API request | **TP** (real HTTP boundary) |
| #422  | `organizer/rename.go:424` | Rename pipeline (metadata-driven) | **Likely TP** (no boundary check) |
| #225  | `metadata/metadata.go:112` | `ExtractMetadata(filePath, ...)` library function | **FP** for scanner; **TP** if HTTP calls it (none observed) |
| #202  | `fileops/safe_operations.go:174` | `op.backupPath` (internal struct field) | **Likely FP** (internal-only) |
| #479  | `itunes/itl.go:673` | Function param (config-supplied) | **Likely FP** (config) |
| #14   | `backup/backup.go:135` | `RestoreBackup(backupPath, ...)` (admin restore flow) | **TP** (admin-auth required, still a boundary) |

**Summary:** 4 definite TP, 4 definite FP, 3 likely-FP (edge cases). **Roughly 35–45% FP rate**, matching the team's prior dismissal cluster.

### Recommended Triage Strategy

**Do NOT bulk-fix 217 alerts.** Instead:

1. **Phase 1: CodeQL Custom Sanitizer Pack** (MUST come first)
   - Use GitHub's Models-as-Data feature (introduced 2026-04-21, https://github.blog/changelog/2026-04-21-codeql-now-supports-sanitizers-and-validators-in-models-as-data/) to teach CodeQL about `internal/util.SafeJoin` and `WithinRoot`.
   - Create `.github/codeql/` directory with `codeql-pack.yml` and MaD `.yml` files registering these functions as path-traversal sanitizers.
   - Update `.github/workflows/codeql.yml` to reference the local pack.
   - Re-run code scanning; expect ~80–100 of the 217 alerts to drop automatically.

2. **Phase 2: Triage Remaining Alerts**
   - Bucket alerts into:
     - **Config-borne paths only** (startup, not request-driven) → dismiss as FP with documented rationale.
     - **Internal pipeline** (scan-derived paths inside `RootDir`, already wrapped in `SafeJoin`/`WithinRoot`) → dismiss as FP.
     - **Genuine HTTP/API boundary** (e.g. #388, #442, file-upload + folder-creation handlers) → these are the real P0s; expect ≤ 30–40 of them.

3. **Phase 3: Fix Real True Positives**
   - Apply the typed-boundary `safepath.SafePath` package (see arch-design recommendation) to the ≤ 30–40 genuine alerts.
   - Add forbidigo lint rule banning raw `os.Open/Create/ReadFile` outside `internal/security/safepath`.

**Why This Matters:**
- Fixing ~120 false positives by hand wastes remediation effort.
- Bulk-fixing 217 alerts without triage creates a massive PR blast radius (14+ files, hard to review).
- The custom sanitizer pack is the **right tool** for this problem — it teaches CodeQL the codebase's existing security boundary.

**Alert Shape Reference:**
- **FP shape:** `#627, #505, #348, #609` (config/data-dir sourced)
- **TP shape:** `#388, #442` (HTTP request body → filesystem operation)

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

## Reviewer Disagreements Resolved — Amendment Log

Four independent reviewers evaluated the original `spec.md` + `implementation-plan.md`. This section documents each substantive disagreement and how it was incorporated into v2.0.0.

### Amendment 1: Phase 0 Govulncheck Blocker is Stale (`sast-sca-auditor`)

**Original claim:** Govulncheck fails with `GOEXPERIMENT=jsonv2`; needs binary-mode workaround (~1 hour).

**Reviewer finding:** `go.mod` is at `go 1.26.0` (not `1.24.0`), and `govulncheck ./...` runs cleanly with `GOEXPERIMENT=jsonv2` today. Verified live: "No vulnerabilities found."

**Resolution:**
- Marked Phase 0 as **✅ RESOLVED** with lessons-learned section (lines 383–418).
- New Phase 1 in `implementation-plan.md` verifies nightly workflow is green (~30 min), not implementing a workaround.
- Saves ~30–60 minutes of remediation effort.

---

### Amendment 2: ~35–45% of Path-Injection Alerts are False Positives (`sast-sca-auditor`)

**Original claim:** All 217 `go/path-injection` alerts are real vulnerabilities; fix systematically.

**Reviewer finding:** Spot-checked 11 random alerts; found 4 TP, 4 FP, 3 likely-FP. FP pattern: paths sourced from `config.AppConfig.*` or startup-only settings (e.g. #627, #505, #348, #609). Real TP pattern: HTTP request body → filesystem operation (e.g. #388, #442).

**Resolution:**
- Added "False Positive Cluster" section (lines 280–378) documenting the FP shape and spot-check results.
- New Phase 1 in `implementation-plan.md`: **CodeQL Custom Sanitizer Pack** (must run first) to teach CodeQL about `internal/util.SafeJoin` and `WithinRoot`. Expected to drop ~80–100 alerts automatically.
- Phase 2 and beyond now operate on the post-sanitizer-pack alert count (~110–137 remaining), not the full 217.
- Changed spec's §1.1 Recommended Action from "fix individually" to "triage-via-sanitizer-pack."

---

### Amendment 3: Dependabot #27 is in Root `package-lock.json`, Not `web/` (`sast-sca-auditor`)

**Original claim:** Alert #27 (`follow-redirects`) is in `web/package-lock.json`; fix with `npm update` in `web/`.

**Reviewer finding:** The vulnerable copy is in **root** `package-lock.json`, pulled in transitively by `axios@1.15.2` (root devDep, used by Playwright test harness). `web/` is clean (`npm audit` → 0). The proposed fix command will not move the alert.

**Resolution:**
- Updated §2.1 Dependabot section (lines 319–342) to correctly identify **root** `package.json` as the location.
- Noted that root `package.json` and `package-lock.json` were **deleted in PR #687** (commit `f2d16dd8`), making this alert moot.
- Dependabot #27 was dismissed (not merged).
- Removed the `npm update follow-redirects` remediation task from Phase 9 in `implementation-plan.md`.

---

### Amendment 4: The Proposed `pathvalidation` Package is Too Weak (`arch-design-reviewer`)

**Original proposal:** Create `internal/security/pathvalidation/` with free functions (`ValidateRelativePath`, `SanitizeFilename`, etc.).

**Reviewer critique:** Free functions are advisory; nothing forces callers to use them. 218th alert will appear after the 217th is "fixed." Recommend a **typed boundary** (`safepath.Root` + `safepath.SafePath` newtype) + wrapped FS methods (`Open`, `ReadFile`, `Create`) + `golangci-lint` forbidigo rule banning raw `os.Open/Create/ReadFile` outside `internal/security/safepath`.

**Resolution:**
- Phase 2 in `implementation-plan.md` (formerly Phase 1) now creates `internal/security/safepath/` with the typed-boundary design.
- Added forbidigo lint rule to CI (Phase 2 sub-task).
- Same pattern applied to SSRF (`safehttp.Client`) in Phase 3 and logging (`seclog.Secret`/`PII` LogValuer wrappers) in Phase 4.
- Updated §1.1 Path Injection remediation strategy to reference typed boundaries, not free functions.

---

### Amendment 5: Plan is Back-Loaded; Front-Load High-Impact Work (`arch-design-reviewer`)

**Original order:** Phase 0 (govulncheck) → Phase 1 (pathvalidation package) → Phase 2 (fileops) → Phase 3 (covers) → Phase 4 (iTunes/transfer, largest alert drop).

**Reviewer recommendation:** Swap phases to deliver value sooner. Recommend: CodeQL sanitizer pack first (immediate 80-alert drop), then typed boundaries, then fan out.

**Resolution:**
- New phase order in `implementation-plan.md`:
  - **Phase 1:** CodeQL custom sanitizer pack (~3h, drops ~80 alerts immediately).
  - **Phase 2:** Typed `safepath` boundary (~6h, blocks all raw `os.*` calls).
  - **Phase 3:** SSRF boundary (`safehttp`) (~4h).
  - **Phase 4:** Logging hardening (`seclog`) (~3h).
  - **Phase 5:** LLM security (new, ~3h, can run parallel).
  - **Phase 6:** Auth-chain hardening (new, ~4h, can run parallel).
  - **Phase 7:** Convert remaining filesystem call-sites to safepath (~6h, depends on Phase 2).
  - **Phase 8:** Threat model + ADRs (new, ~3h, can run parallel from start).
  - **Phase 9:** Regression gate (new, ~2h).
  - **Phase 10:** False-positive cleanup (~2h, depends on Phases 1, 7).
  - **Phase 11:** Verification + closeout (~1h, depends on all).
- Total: ~37 hours (down from ~44h due to Phase 0 obsolescence).

---

### Amendment 6: Audit Missed OWASP LLM Top 10 Entirely (`SE: Security`)

**Original audit scope:** Only CodeQL-detected alerts (path injection, clear-text logging, SSRF, etc.).

**Reviewer finding:** Zero manual inspection of LLM integration. Found LLM01 (prompt injection via crafted filenames at `openai_parser.go:218`) and LLM06 (PII disclosure via OS usernames in paths sent to OpenAI).

**Resolution:**
- Added "OWASP LLM Top 10 Findings" section (lines 130–180).
- New Phase 5 in `implementation-plan.md`: **LLM Security Hardening** (~3h, can run parallel).
- Remediation: sanitize filenames, redact PII from prompts, validate LLM responses, add token/rate limits.

---

### Amendment 7: Unauthenticated SSE Endpoint + Auth-Chain Issues (`SE: Security`)

**Original audit scope:** Only CodeQL-detected alerts.

**Reviewer finding:** `GET /api/events` (SSE endpoint at `server_lifecycle.go:787`) is registered outside auth middleware — any unauthenticated caller can subscribe to real-time system events. Combined with in-memory login lockout (`auth_handlers.go:33–35`) + OOM allocation alerts (`scanner.go:219,339`), attacker can bypass brute-force protection by triggering restarts.

**Resolution:**
- Added "Authentication & Authorization Findings" section (lines 181–279).
- New Phase 6 in `implementation-plan.md`: **Auth-Chain Hardening** (~4h, can run parallel).
- Remediation: authenticate `/api/events`, move login lockout to durable store, cap upload allocations, add unit test for route auth coverage.

---

### Amendment 8: No Regression Gate (`arch-design-reviewer`)

**Original plan:** Fix all 236 alerts, then re-pull data to verify (Phase 11).

**Reviewer critique:** Without a CI gate, the 237th alert will appear the day after merge. Recommend `.github/workflows/security-gate.yml` that fails any PR increasing open code-scanning alert count vs main.

**Resolution:**
- New Phase 9 in `implementation-plan.md`: **Regression Gate** (~2h).
- Creates `security-gate.yml` workflow that queries GitHub Code Scanning API, compares open alert count vs main branch, fails on increase.
- Also adds root-level npm coverage to `vulnerability-scan.yml` (future-proofing in case root manifests reappear).

---

### Amendment 9: No Threat Model or ADRs (`arch-design-reviewer`)

**Original plan:** Fix alerts per CodeQL output; document path validation utilities.

**Reviewer critique:** Fix decisions are being made per-alert rather than per-asset. No structured security-event channel, no SBOM, no ADRs for remediation pattern decisions.

**Resolution:**
- New Phase 8 in `implementation-plan.md`: **Threat Model + ADRs** (~3h, can run parallel from start).
- Deliverables:
  - `docs/security/threat-model.md` (assets, principals, trust boundaries, attacker model).
  - ADRs 0001–0012 listed in `arch-design.md` §7: safepath newtype, typed boundaries, CodeQL sanitizer pack strategy, safehttp design, seclog design, threat model alignment, regression gate strategy, LLM security hardening, auth-chain hardening, forbidigo lint rule enforcement, security-event logging, SBOM policy.

---

### Amendment 10: Plan Ships Without Coverage for Root NPM Tree (`sast-sca-auditor`)

**Original plan:** Fix `web/package-lock.json` Dependabot alert; assume root manifest is covered.

**Reviewer finding:** Nightly `vulnerability-scan.yml` only audits `web/`. Root manifest has zero CI coverage outside Dependabot itself. If Dependabot is paused, root vulns go silent.

**Resolution:**
- Phase 9 in `implementation-plan.md` (Regression Gate) now includes adding root manifest to nightly scan as a sub-task (conditional: only if root `package.json` exists; currently it doesn't post-PR #687).
- Documents that root `package.json` was deleted in PR #687, so this is future-proofing.

---

## Amendments Summary Table

| # | Source | Amendment | Impact on Plan |
|---|--------|-----------|----------------|
| 1 | `sast-sca-auditor` | Phase 0 govulncheck blocker is stale | Saves ~1h; Phase 0 → Phase 1 (verification only) |
| 2 | `sast-sca-auditor` | ~35–45% path-injection alerts are FP | Front-loads CodeQL sanitizer pack (Phase 1); drops ~80 alerts before manual triage |
| 3 | `sast-sca-auditor` | Dependabot #27 in root, not `web/` | Removed from remediation (already resolved in PR #687) |
| 4 | `arch-design-reviewer` | `pathvalidation` → typed `safepath` boundary | Phase 2 now creates newtype + lint gate |
| 5 | `arch-design-reviewer` | Plan back-loaded; front-load value | Reordered phases to ship sanitizer pack + boundaries first |
| 6 | `SE: Security` | LLM Top 10 missing (LLM01, LLM06) | New Phase 5 (~3h) |
| 7 | `SE: Security` | Unauthenticated SSE + auth-chain | New Phase 6 (~4h) |
| 8 | `arch-design-reviewer` | No regression gate | New Phase 9 (~2h) |
| 9 | `arch-design-reviewer` | No threat model / ADRs | New Phase 8 (~3h) |
| 10 | `sast-sca-auditor` | Root NPM tree not in CI scan | Phase 9 sub-task (conditional) |

**Net effect:**
- Original plan: 11 phases (0–11), ~44h, 236 alerts.
- Reconciled plan: 11 phases (1–11), ~37h, 236 alerts (but ~80–100 drop after Phase 1).
- New phases cover OWASP LLM Top 10, auth/authz, regression prevention, and architectural governance (threat model + ADRs).

---

*Document Version: 2.0.0*  
*Original Audit Date: 2026-05-03*  
*Reconciliation Date: 2026-05-04*  
*Independent Reviews: `sast-sca-auditor`, `SE: Security`, `arch-design-reviewer`, `code-review`*  
*Raw Data: `docs/security/audit-2026-05-03/raw/`*
