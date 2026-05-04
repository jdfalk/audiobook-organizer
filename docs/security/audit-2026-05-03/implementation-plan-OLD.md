<!-- file: docs/security/audit-2026-05-03/implementation-plan.md -->
<!-- version: 1.0.0 -->
<!-- guid: 4619be08-72ad-4a9d-b15c-7d6d88e228a9 -->
<!-- last-edited: 2026-05-03 -->

# Security Alert Remediation — Implementation Plan

## Overview

This document provides a phased, actionable plan to remediate all 236 open security alerts identified in the 2026-05-03 audit.

**Guiding Principles:**
- **Systematic over ad-hoc:** Build reusable security utilities, don't patch 217 files individually.
- **Test-driven:** Write tests before fixing (TDD).
- **Incremental:** One coherent PR per phase; verify CI passes before proceeding.
- **Documented:** Update code comments, security docs, and dismissed alert comments.

**Reference Documents:**
- **Audit Spec:** `docs/security/audit-2026-05-03/spec.md`
- **Raw Data:** `docs/security/audit-2026-05-03/raw/`

---

## Phase 0: Enable Govulncheck (Unblock Vulnerability Scanning)

**Goal:** Get govulncheck working with `GOEXPERIMENT=jsonv2` to detect Go vulnerabilities.

**Status:** BLOCKER — Must complete before other phases to ensure new code doesn't introduce Go CVEs.

### Task 0.1: Fix govulncheck to scan jsonv2 builds

**Problem:** Current `govulncheck ./...` fails/skips when `GOEXPERIMENT=jsonv2` is set.

**Solution:** Switch to binary-mode scanning.

**Affected Files:**
- `.github/workflows/vulnerability-scan.yml`

**Changes:**
```yaml
# Replace lines 30-33:
- name: Build with jsonv2
  run: go build -o ./bin/audiobook-organizer ./cmd/audiobook-organizer
  env:
    GOEXPERIMENT: jsonv2

- name: Run govulncheck on binary
  run: govulncheck -mode=binary ./bin/audiobook-organizer
```

**Commands:**
```bash
# Test locally
GOEXPERIMENT=jsonv2 go build -o ./bin/audiobook-organizer ./cmd/audiobook-organizer
govulncheck -mode=binary ./bin/audiobook-organizer

# Should exit 0 if no vulnerabilities, or report findings
```

**Dependencies:** None

**Verification:**
1. Trigger `.github/workflows/vulnerability-scan.yml` manually via `gh workflow run vulnerability-scan.yml`
2. Check run logs for govulncheck output
3. Confirm no errors like "incompatible environment" or "skipping analysis"

**Rollback:** Revert workflow change; govulncheck will remain broken (acceptable for rollback, not for long-term).

**PR Title:** `fix(ci): enable govulncheck for GOEXPERIMENT=jsonv2 builds`

**Estimated Effort:** 1 hour

---

## Phase 1: Path Injection — Foundation (Build Validation Utilities)

**Goal:** Create centralized, tested path validation utilities to eliminate 217 path injection alerts systematically.

**Status:** P0 — Blocks Phases 2-5.

### Task 1.1: Create `internal/security/pathvalidation` package

**New Files:**
- `internal/security/pathvalidation/validate.go`
- `internal/security/pathvalidation/validate_test.go`
- `internal/security/pathvalidation/doc.go`

**Core Functions:**
```go
// ValidateRelativePath ensures resolved path stays within baseDir
func ValidateRelativePath(input string, baseDir string) (string, error)

// SanitizeFilename strips path separators, null bytes, control chars
func SanitizeFilename(input string) (string, error)

// ValidateAgainstWhitelist checks path is within allowed directories
func ValidateAgainstWhitelist(path string, allowedDirs []string) error

// SecureJoin safely joins base and relative path (prevents traversal)
func SecureJoin(base, relative string) (string, error)
```

**Implementation Notes:**
- Use `filepath.Clean()`, `filepath.Abs()`, `filepath.Rel()` correctly
- Check for `..` after cleaning
- Reject absolute paths in user input
- Consider `github.com/cyphar/filepath-securejoin` as dependency (MIT license)

**Test Coverage:**
- Traversal attempts: `../../etc/passwd`, `/etc/passwd`, `foo/../../bar`
- Null bytes: `foo\x00bar`
- Path separators in filenames: `foo/bar`, `foo\bar` (Windows)
- Symlink attacks (if applicable)
- Valid cases: `foo/bar.txt`, `./file.mp3`

**Commands:**
```bash
cd internal/security/pathvalidation
go test -v -cover
# Target: 100% coverage for security-critical code
```

**Dependencies:** None (foundational)

**PR Title:** `feat(security): add pathvalidation package for safe filesystem operations`

**Estimated Effort:** 4 hours (implementation + tests)

---

## Phase 2: Path Injection — Apply Validation (File Operations Core)

**Goal:** Fix path injection in `internal/fileops/` (9 alerts) — the most critical file operation layer.

**Status:** P0 — High risk; used by many other services.

### Task 2.1: Refactor `internal/fileops/service.go`

**Alerts Covered:**
- #625, #624, #623, #622, #621, #620 (`internal/fileops/service.go` lines 190, 176, 166, 140, 119, 103)

**Changes:**
1. Import `internal/security/pathvalidation`
2. At function entry points (before filesystem ops), call `pathvalidation.ValidateRelativePath()` or `SecureJoin()`
3. Add error handling for validation failures (return `ErrInvalidPath`)

**Example (before/after):**
```go
// BEFORE
func (s *Service) ReadFile(path string) ([]byte, error) {
    return os.ReadFile(path) // ALERT: Uncontrolled path
}

// AFTER
func (s *Service) ReadFile(path string) ([]byte, error) {
    validated, err := pathvalidation.ValidateRelativePath(path, s.baseDir)
    if err != nil {
        return nil, fmt.Errorf("invalid path: %w", err)
    }
    return os.ReadFile(validated)
}
```

**Affected Files:**
- `internal/fileops/service.go`
- `internal/fileops/hash.go` (alert #539)
- `internal/fileops/write_tags_safe.go` (alerts #538, #537, #536)
- `internal/fileops/safe_operations.go` (alerts #543, #542)

**Test Updates:**
- Add negative tests for traversal attempts
- Verify existing tests still pass

**Commands:**
```bash
make test
# Ensure internal/fileops tests pass
go test -v -cover ./internal/fileops/...
```

**Dependencies:** Task 1.1 (pathvalidation package)

**PR Title:** `fix(security): prevent path injection in fileops layer (9 alerts)`

**Estimated Effort:** 6 hours

---

## Phase 3: Path Injection — Server Handlers (Covers)

**Goal:** Fix path injection in cover image handlers (9 alerts).

**Alerts Covered:**
- #602, #601, #600, #599, #598 (`internal/server/covers.go`)
- #597, #596, #595, #594 (`internal/server/cover_history.go`)

**Changes:**
1. Validate cover IDs/paths from HTTP requests before passing to fileops
2. Use `pathvalidation.SanitizeFilename()` for cover filenames
3. Ensure cover directory paths are within configured base directory

**Affected Files:**
- `internal/server/covers.go`
- `internal/server/cover_history.go`

**Test Updates:**
- Add handler tests with traversal payloads
- Verify 400 Bad Request on invalid paths

**Commands:**
```bash
go test -v ./internal/server/... -run TestCovers
```

**Dependencies:** Task 2.1 (fileops validation)

**PR Title:** `fix(security): prevent path injection in cover image handlers (9 alerts)`

**Estimated Effort:** 3 hours

---

## Phase 4: Path Injection — iTunes/Transfer/Server Core

**Goal:** Fix path injection in iTunes handlers, transfer service, and server core (20+ alerts).

**Alerts Covered:**
- iTunes handlers: #627, #626, #607, #606, #605, #604, #603 (`itunes_handlers.go`)
- Transfer service: #591, #590, #589, #588 (`itunes/service/transfer.go`)
- Server core: #615 (`server/server.go`)
- Audiobooks: #619, #593, #592 (`audiobooks_handlers.go`, `audiobooks/service.go`)

**Changes:**
1. Validate file paths from iTunes imports before filesystem ops
2. Validate transfer source/destination paths
3. Add validation to audiobook handlers

**Affected Files:**
- `internal/server/itunes_handlers.go`
- `internal/itunes/service/transfer.go`
- `internal/server/server.go`
- `internal/server/audiobooks_handlers.go`
- `internal/audiobooks/service.go`

**Dependencies:** Task 2.1

**PR Title:** `fix(security): prevent path injection in iTunes/transfer/audiobook handlers (20 alerts)`

**Estimated Effort:** 6 hours

---

## Phase 5: Path Injection — Scanner/Reconcile/OpenLibrary

**Goal:** Fix path injection in scanner, reconciliation, and OpenLibrary services (15+ alerts).

**Alerts Covered:**
- Scanner: #617, #616, #614 (`scanner/service.go`)
- Reconcile: #613, #612, #611 (`reconcile/reconcile.go`)
- OpenLibrary: #610, #609, #608 (`openlibrary_service.go`)
- Importer: #618 (`importer/service.go`)

**Changes:**
1. Validate scanned paths before database insertion
2. Validate reconciliation paths from database before filesystem ops
3. Validate OpenLibrary cover download paths

**Affected Files:**
- `internal/scanner/service.go`
- `internal/reconcile/reconcile.go`
- `internal/server/openlibrary_service.go`
- `internal/importer/service.go`

**Dependencies:** Task 2.1

**PR Title:** `fix(security): prevent path injection in scanner/reconcile/openlibrary (15 alerts)`

**Estimated Effort:** 5 hours

---

## Phase 6: Path Injection — Backup/Deluge/Remaining

**Goal:** Fix remaining path injection alerts in backup, Deluge import, and other services (10+ alerts).

**Alerts Covered:**
- Backup: #541 (`backup/backup.go`)
- Deluge: #535, #534 (`deluge_import_unix.go`)
- Safe operations: (already in Phase 2, but verify)

**Changes:**
1. Validate archive extraction paths (backup)
2. Validate Deluge import paths
3. Sweep for any remaining path alerts

**Affected Files:**
- `internal/backup/backup.go`
- `internal/server/deluge_import_unix.go`

**Dependencies:** Task 2.1

**PR Title:** `fix(security): prevent path injection in backup/deluge/misc (remaining alerts)`

**Estimated Effort:** 3 hours

---

## Phase 7: Non-Path-Injection Errors (14 alerts)

**Goal:** Fix clear-text logging, request forgery, allocation issues, zipslip, etc.

### Task 7.1: Fix clear-text logging (6 alerts)

**Alerts:** #530, #529, #528, #527, #526 (`maintenance_fixups.go`), #47 (`cmd/root.go`)

**Changes:**
1. Identify sensitive fields being logged (passwords, tokens, API keys)
2. Redact before logging: `log.Infof("User: %s, Token: [REDACTED]", user)`
3. Use structured logging with field filters if available

**Affected Files:**
- `internal/server/maintenance_fixups.go`
- `cmd/root.go`

**Commands:**
```bash
grep -r "log.*password\|log.*token\|log.*key" internal/ cmd/
# Manual review for false positives
```

**PR Title:** `fix(security): redact sensitive data in logs (6 alerts)`

**Estimated Effort:** 2 hours

---

### Task 7.2: Fix request forgery (4 alerts)

**Alerts:** #587 (`covers.go`), #467 (`deluge/client.go`), #458 (`webhook/plugin.go`), #232 (`metadata/cover.go`)

**Changes:**
1. Validate URLs against whitelist of allowed domains
2. Block private IP ranges (use `net.ParseIP()` + RFC 1918 checks)
3. Add URL validation utility in `internal/security/urlvalidation/`

**Affected Files:**
- `internal/server/covers.go`
- `internal/deluge/client.go`
- `internal/plugins/webhook/plugin.go`
- `internal/metadata/cover.go`

**New Files:**
- `internal/security/urlvalidation/validate.go`
- `internal/security/urlvalidation/validate_test.go`

**PR Title:** `fix(security): prevent SSRF via URL validation (4 alerts)`

**Estimated Effort:** 4 hours

---

### Task 7.3: Fix uncontrolled allocation (2 alerts)

**Alerts:** #129, #44 (`scanner/scanner.go`)

**Changes:**
1. Cap allocation sizes: `size := min(requested, maxAllowed)`
2. Validate input ranges before allocation

**Affected Files:**
- `internal/scanner/scanner.go`

**PR Title:** `fix(security): cap allocation sizes to prevent OOM (2 alerts)`

**Estimated Effort:** 1 hour

---

### Task 7.4: Fix zipslip (1 alert)

**Alert:** #13 (`backup/backup.go:153`)

**Changes:**
1. Validate archive entry paths using `pathvalidation.SecureJoin()`
2. Reject entries with absolute paths or `..`

**Affected Files:**
- `internal/backup/backup.go`

**PR Title:** `fix(security): prevent zipslip in backup extraction (1 alert)`

**Estimated Effort:** 1 hour

---

### Task 7.5: Fix weak hashing (1 alert)

**Alert:** #132 (`database/settings.go`)

**Changes:**
1. Identify what's being hashed (passwords? config checksums?)
2. If passwords: migrate to bcrypt/argon2
3. If non-password data: upgrade to SHA-256

**Affected Files:**
- `internal/database/settings.go`

**PR Title:** `fix(security): upgrade to strong hashing algorithm (1 alert)`

**Estimated Effort:** 2 hours

---

## Phase 8: Warnings (4 alerts)

**Goal:** Fix warning-level alerts (lower priority).

### Task 8.1: Fix disabled certificate check (1 alert)

**Alert:** #379 (`mtls/provisioning.go`)

**Action:** Investigate if this is test/dev code. If prod, enable validation.

**Estimated Effort:** 1 hour

---

### Task 8.2: Fix allocation overflow (1 alert)

**Alert:** #468 (`itunes/itl.go`)

**Action:** Add overflow checks before size computation.

**Estimated Effort:** 1 hour

---

### Task 8.3: Fix JS certificate bypass (1 alert)

**Alert:** #160 (`scripts/record_demo.js`)

**Action:** Document as dev-only script, or remove bypass.

**Estimated Effort:** 0.5 hours

---

### Task 8.4: Fix JS incomplete sanitization (1 alert)

**Alert:** #50 (`web/src/pages/Settings.tsx`)

**Action:** Review JSX escaping; add DOMPurify if `dangerouslySetInnerHTML` is used.

**Estimated Effort:** 1 hour

---

## Phase 9: Dependabot (1 alert)

**Goal:** Update `follow-redirects` to fix header leak.

### Task 9.1: Bump follow-redirects

**Alert:** #27 (`follow-redirects <= 1.15.11`)

**Commands:**
```bash
cd web
npm update follow-redirects
npm audit fix
npm test
cd ..
make test-all
```

**Verification:**
1. Check `web/package-lock.json` for `follow-redirects@1.16.0` or higher
2. Run `npm audit` — should show 0 vulnerabilities
3. Test frontend: `cd web && npm run build`

**PR Title:** `fix(deps): bump follow-redirects to 1.16.0 (GHSA-r4q5-vmmm-2653)`

**Estimated Effort:** 0.5 hours

---

## Phase 10: Documentation & Dismissed Alerts

**Goal:** Update docs and add comments to dismissed alerts.

### Task 10.1: Document path validation utilities

**New/Updated Files:**
- `docs/security/path-validation-policy.md` (new)
- `internal/security/pathvalidation/README.md` (new)

**Content:**
- When to use each validation function
- Examples of valid/invalid inputs
- Link to OWASP path traversal guide

**Estimated Effort:** 1 hour

---

### Task 10.2: Add dismissal comments to "no comment" alerts

**Alerts:** #560-547 (excluding those with existing comments)

**Commands:**
```bash
# For each alert
gh api -X PATCH repos/jdfalk/audiobook-organizer/code-scanning/alerts/{NUMBER} \
  -f dismissed_comment="Maintenance job: paths originate from database (trusted), not user input. Validated as false positive per 2026-05-03 audit."
```

**Script:**
```bash
#!/bin/bash
for alert in 560 559 558 557 556 555 554 553 552 551 550 548 547; do
  gh api -X PATCH repos/jdfalk/audiobook-organizer/code-scanning/alerts/$alert \
    -f dismissed_comment="Maintenance job: paths originate from database (trusted), not user input. Validated as false positive per 2026-05-03 audit."
done
```

**Note:** This modifies dismissed alerts (not opening/closing), which the user said is acceptable for adding comments.

**PR Title:** N/A (API changes, not code changes)

**Estimated Effort:** 0.5 hours

---

## Phase 11: Final Verification

**Goal:** Confirm all alerts are resolved.

### Task 11.1: Re-pull alert data

**Commands:**
```bash
gh api repos/jdfalk/audiobook-organizer/code-scanning/alerts --paginate | jq '[.[] | select(.state == "open")] | length'
gh api repos/jdfalk/audiobook-organizer/dependabot/alerts --paginate | jq '[.[] | select(.state == "open")] | length'
gh api repos/jdfalk/audiobook-organizer/secret-scanning/alerts --paginate | jq 'length'
```

**Expected Output:**
- Code scanning: `0` (or only accepted-risk alerts)
- Dependabot: `0`
- Secret scanning: `0`

**Verification:**
1. All PRs merged
2. `make ci` passes on `main`
3. No new alerts introduced

**Estimated Effort:** 1 hour

---

## Summary Table

| Phase | Tasks | Alerts Fixed | PRs | Effort | Dependencies |
|-------|-------|--------------|-----|--------|--------------|
| 0 | 1 | Unblock govulncheck | 1 | 1h | None |
| 1 | 1 | Foundation (0 alerts, enables others) | 1 | 4h | Phase 0 |
| 2 | 1 | 9 (fileops) | 1 | 6h | Phase 1 |
| 3 | 1 | 9 (covers) | 1 | 3h | Phase 2 |
| 4 | 1 | 20 (iTunes/transfer) | 1 | 6h | Phase 2 |
| 5 | 1 | 15 (scanner/reconcile) | 1 | 5h | Phase 2 |
| 6 | 1 | 10 (backup/deluge) | 1 | 3h | Phase 2 |
| 7 | 5 | 14 (non-path errors) | 5 | 10h | Phase 1 (for SSRF validation) |
| 8 | 4 | 4 (warnings) | 4 | 3.5h | None |
| 9 | 1 | 1 (Dependabot) | 1 | 0.5h | None |
| 10 | 2 | 0 (documentation) | 1 | 1.5h | None |
| 11 | 1 | 0 (verification) | 0 | 1h | All above |
| **TOTAL** | **16** | **236** | **17** | **44h** | — |

**Timeline Estimate:** 6-8 weeks (1 engineer, part-time) or 2-3 weeks (1 engineer, full-time).

---

## Rollback Strategy

For each phase:
1. **Revert PR** via `gh pr reopen <number> && git revert <commit>`
2. **Monitor for alert re-open** (GitHub may auto-reopen dismissed alerts on revert)
3. **Document rollback reason** in PR comment

**Critical Rollback Scenarios:**
- Phase 1 breaks builds → Revert immediately, fix locally, re-PR
- Phase 2+ causes test failures → Revert that phase only; Phase 1 is safe to keep

---

## Post-Remediation Checklist

- [ ] All 236 open alerts are fixed or dismissed with documented rationale
- [ ] Govulncheck runs successfully on jsonv2 builds
- [ ] All PRs merged to `main`
- [ ] `make ci` passes (tests + coverage ≥80%)
- [ ] Documentation updated (path validation policy, security README)
- [ ] Dismissed alerts have comments
- [ ] Post-audit verification confirms 0 open alerts

---

*Document Version: 1.0.0*  
*Created: 2026-05-03*  
*Estimated Completion: Q2 2026*
