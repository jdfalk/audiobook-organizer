<!-- file: docs/security/audit-2026-05-03/raw/summary.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7a8b9c0d-1e2f-3a4b-5c6d-7e8f9a0b1c2d -->
<!-- last-edited: 2026-05-03 -->

# Security Audit Summary — 2026-05-03

## Overview

This document summarizes the security alert inventory for `jdfalk/audiobook-organizer` as of May 3, 2026.

**Data Sources:**
- Code scanning alerts: `gh api repos/jdfalk/audiobook-organizer/code-scanning/alerts --paginate`
- Dependabot alerts: `gh api repos/jdfalk/audiobook-organizer/dependabot/alerts --paginate`
- Secret scanning alerts: `gh api repos/jdfalk/audiobook-organizer/secret-scanning/alerts --paginate`

## Total Counts

| Category | Total | Open | Dismissed | Fixed |
|----------|-------|------|-----------|-------|
| **Code Scanning** | 602 | 235 | 17 | 350 |
| **Dependabot** | 20 | 1 | 0 | 19 |
| **Secret Scanning** | 0 | 0 | 0 | 0 |
| **TOTAL** | 622 | 236 | 17 | 369 |

## Code Scanning Breakdown

### By State and Severity

| State | Total | Error | Warning | Note |
|-------|-------|-------|---------|------|
| Open | 235 | 231 | 4 | 0 |
| Dismissed | 17 | 17 | 0 | 0 |
| Fixed | 350 | 293 | 27 | 30 |

### Open Alerts by Rule (Top 10)

| Count | Severity | Rule ID |
|-------|----------|---------|
| 217 | error | go/path-injection |
| 6 | error | go/clear-text-logging |
| 4 | error | go/request-forgery |
| 2 | error | go/uncontrolled-allocation-size |
| 1 | error | go/zipslip |
| 1 | error | js/disabling-certificate-validation |
| 1 | warning | go/allocation-size-overflow |
| 1 | warning | go/disabled-certificate-check |
| 1 | warning | go/weak-sensitive-data-hashing |
| 1 | warning | js/incomplete-sanitization |

**Dominant Issue:** Path injection (`go/path-injection`) accounts for 217/235 (92.3%) of open alerts, spanning file operation handlers, iTunes integrations, scanner services, and server endpoints.

## Dependabot Breakdown

### By State and Severity

| State | Total | Critical | High | Medium | Low |
|-------|-------|----------|------|--------|-----|
| Open | 1 | 0 | 0 | 1 | 0 |
| Fixed | 19 | 0 | 7 | 12 | 0 |

### Open Alert

| # | Severity | Package | Ecosystem | Vulnerable Range | Fixed Version | CVE/GHSA |
|---|----------|---------|-----------|------------------|---------------|----------|
| 27 | medium | follow-redirects | npm | <= 1.15.11 | 1.16.0 | GHSA-r4q5-vmmm-2653 |

**Issue:** `follow-redirects` leaks Custom Authentication Headers to Cross-Domain Redirect Targets.

## Secret Scanning

**No alerts returned.** Either:
- Secret scanning is disabled for this repository, or
- No secrets have been detected

## Key Observations

1. **Path Injection Dominance:** 217 open path injection alerts indicate systemic issues with untrusted user input flowing into filesystem operations without proper validation/sanitization.

2. **Govulncheck Blocker:** The repository builds with `GOEXPERIMENT=jsonv2`, which currently causes govulncheck to skip or error. The vuln scanner sees the environment as incompatible. This is documented in `.github/workflows/vulnerability-scan.yml` (lines 30-33).

3. **Historical Progress:** 369 fixed alerts (59% of total) demonstrate ongoing security hardening efforts.

4. **Single Dependabot Alert:** Only one open npm vulnerability in a transitive dev dependency (`follow-redirects`).

5. **No Secret Leaks:** Clean slate for secret scanning.

## Recommended Next Steps

1. **Unblock govulncheck:** Investigate if the jsonv2 experiment is causing false negatives in Go vulnerability scanning.
2. **Address path injection systematically:** The 217 alerts likely require a centralized input validation/sanitization strategy rather than 217 individual fixes.
3. **Bump follow-redirects:** Trivial npm update to close Dependabot #27.
4. **Review dismissed alerts:** 17 code scanning alerts were dismissed — validate rationale is still sound.

---

*Generated: 2026-05-03*  
*Alert data retrieved via `gh api` on 2026-05-03*
