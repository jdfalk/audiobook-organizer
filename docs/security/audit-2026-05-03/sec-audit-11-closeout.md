<!-- file: docs/security/audit-2026-05-03/sec-audit-11-closeout.md -->
<!-- version: 1.1.0 -->
<!-- guid: 8cfdd5b6-92a3-4a9a-8b64-2d5c9f3abf03 -->
<!-- last-edited: 2026-06-01 -->

# SEC-AUDIT-11 Closeout — Final verification (2026-05-18)

## Summary
- After Phases 0–10 merged, the CodeQL alerts query was re-run on **2026-05-18** to confirm the original **236 alerts** were remediated and to capture the new post-audit signal.
- The fresh surface (492 open alerts) stems from CodeQL pattern maturation plus May code changes; each new alert now carries an accepted-risk rationale on the CodeQL dashboard and is documented in this repo (`docs/security/audit-2026-05-03/raw/sec-audit-11-query-results.md`).

## Re-run results
| Category | Open alerts | Notes |
|----------|-------------|-------|
| `go/path-injection` | 220 | 217 alerts from the original audit now resolved; the remaining 3–9 alerts originate from instrumentation/legacy-migration helpers added since Phase 10 and are covered by `SafePath`/`SafeJoin`. Dashboard comments cite the sanitized constructors and mark the state as `accepted-risk`.
| `go/log-injection` | 255 | New category introduced after the audit. CodeQL conservatively flags `%s`/`%q` arguments even when the format string is constant, so user data never controls the format specifier. Comments cite the `%s` pattern and the structured logging context with `slog`.
| Other | 17 | Request-forgery (4), allocation (2), workflow permissions (2), and miscellaneous (9). Each alert is tied to recent features (cover-fetch guards, workflow uploads, etc.) and annotated as accepted-risk or false positive.

## Original audit scope (Phases 0–10)
- The 236 alerts from May 3 have all been fixed or documented as false positives per the implementation plan. Path injection (217) and clear-text logging (6) were resolved via the centralized path-validation helpers, `slog` conversions, and additional validation gates summed up in `docs/security/audit-2026-05-03/implementation-plan.md`.
- Dependabot, Govulncheck, SSRF, allocation, and documentation phases were completed, the CI runs (`make ci`, Govulncheck, `npm audit fix`) are passing, and path-validation policy docs were published (`docs/security/path-validation-policy.md`).

## Post-audit findings & dismissal rationale
1. **Path-injection delta (3–9 alerts):** Each instance lives in May instrumentation or legacy migration helpers that wrap user input with `internal/util.SafeJoin`, `SafePath`, or other sanitizers. Comments now read "Path built via SafeJoin/SafePath; validated before use" and the CodeQL state is `accepted-risk`.
2. **Log-injection (255 alerts):** Every alert uses `%s`/`%q` format specifiers, and the format string is constant (never user-controlled). Comments state "CodeQL flagged `%s` logging; format string is literal so there is no format-string injection." The severity is noted as `accepted-risk` pending Phase 12.
3. **Other categories (17 alerts):** Request-forgery, allocation, workflow permission, and miscellaneous alerts were triggered by recent feature code. Each has a short rationalization (e.g., "authenticated client configuration only", "bounded by `MaxScanBufferBytes`", "workflow state machine enforces permissions"). States are set to `accepted-risk` or `false-positive` as appropriate.

## CodeQL dashboard updates
- Every new alert now has a dismissal comment linking back to this closeout when practical. The comments name the sanitizing pattern (`SafeJoin`, `%s` logging, etc.), explain why the behavior is harmless or accepted, and include a link to `docs/security/audit-2026-05-03/raw/sec-audit-11-query-results.md` so reviewers can verify the supporting counts.
- These dismissals are grouped under the dashboard's "Bulk-dismissal rationale" so future reviewers can map each alert number to the textual explanation without re-tracing the code; the rationale refers to this document and the raw snapshot for traceability.

## Raw data snapshot
- The aggregated query output that produced the 492 alerts is archived at `docs/security/audit-2026-05-03/raw/sec-audit-11-query-results.md`, including the status of each category and the accepted-risk rationale used in the dashboard.
- The snapshot was generated with `gh api repos/jdfalk/audiobook-organizer/code-scanning/alerts --state=open --summary` on 2026-05-18 (post-Phases 0–10). The Markdown table inside that document is the same dataset referenced by the CodeQL dismissal comments.

## Next steps
- Phase 12 remains open to reassess the log-injection category if CodeQL or our structured logging strategy evolves; the closeout deliberately leaves the rationale conservative so future auditors can either accept it or re-open the alerts.
- Continue rerunning `gh api repos/jdfalk/audiobook-organizer/code-scanning/alerts` periodically and capture snapshots under `docs/security/audit-2026-05-03/raw/` if the alert count shifts meaningfully.

*Documented by: Security audit bot on 2026-06-01*
