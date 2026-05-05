<!-- file: docs/superpowers/bot-tasks/2026-05-04-uos-07-canary-embed.md -->
<!-- version: 1.0.0 -->
<!-- guid: 57b8c9d0-e1f2-3a4b-5c6d-7e8f9a0b1c2d -->
<!-- last-edited: 2026-05-04 -->

# UOS-07 — Migrate `embed-scan` as canary

**Companion human spec:** §11 phase A.

## Branch

```
feat/uos-07-canary-embed
```

## Goal

Move the existing `triggerEmbedScan` handler logic into a real
OperationDef registered through the SDK. This is the first op to
actually use UOS end-to-end. Old `triggerEmbedScan` becomes a
redirect to the new path.

## Files to add

1. `internal/plugins/dedup/plugin.go` — bare-minimum plugin shell:
   ```go
   type Plugin struct { /* embed shared state from existing dedup.Engine */ }
   func New(...) *Plugin { ... }
   func (p *Plugin) Name() string { return "dedup" }
   func (p *Plugin) DisplayName() string { return "Deduplication" }
   func (p *Plugin) Description() string { return "..." }
   func (p *Plugin) Version() string { return "1.0.0" }
   func (p *Plugin) RequiresCoreSchema() string { return ">=1" }
   func (p *Plugin) Migrations() []sdk.Migration { return nil } // no plugin-owned tables yet
   func (p *Plugin) Register(reg sdk.Registry) error {
       return reg.RegisterOp(p.embedScanDef())
   }
   func (p *Plugin) OnEnable(ctx context.Context) error { return nil }
   func (p *Plugin) OnDisable(ctx context.Context, mode sdk.DisableMode) error { return nil }
   ```
   Other dedup ops (acoustid-scan, llm-review, etc.) stay on the OLD
   path; they migrate in UOS-09.

2. `internal/plugins/dedup/embed_scan.go` — `embedScanDef()` returns:
   - `ID`: `"dedup.embed-scan"`
   - `Plugin`: `"dedup"`
   - `DisplayName`: `"Embed all books"`
   - `Description`: `"Re-embeds every primary book that lacks a
     fresh embedding."`
   - `Run`: extracted from existing
     `internal/server/dedup_handlers.go:triggerEmbedScan` opFunc
   - `DefaultPriority`: `PriorityLow`
   - `Cancellable`: `true`
   - `Isolate`: **false** (in-process; the OpenAI call is fast and
     doesn't justify subprocess for the canary)
   - `Timeout`: `120 * time.Minute`
   - `ResumePolicy`: `ResumeRequeue` (idempotent — re-embedding is
     safe)
   - `ConcurrencyKey`: `"dedup.embed-scan"` (serializes its own
     concurrent invocations)
   - Plugin's `MaxConcurrent()` returns `2` (allows embed-scan +
     one other dedup op simultaneously)
   - `Capabilities`: `[CapLibraryRead, CapLibraryWrite,
     CapNetworkOpenAI]`
   - `Permissions`: existing `auth.PermScanTrigger`

3. `internal/plugins/plugins.go` (NEW) — central import file:
   ```go
   package plugins
   import _ "github.com/jdfalk/audiobook-organizer/internal/plugins/dedup"
   // others added in later PRs
   ```

4. `internal/plugins/dedup/embed_scan_test.go` — happy path: register,
   enqueue, run completes, status=completed.

## Files to edit

1. `internal/server/dedup_handlers.go`:
   - `triggerEmbedScan`: replace body with a redirect. New body:
     - Parse user permissions.
     - Call `s.opRegistry.EnqueueOp(ctx, "dedup.embed-scan", nil)`
       through the SDK's narrowed registry interface.
     - Return the resulting OperationV2 row in the response (same
       envelope as before).
   - Old route stays: `POST /api/v1/dedup/embed`. New route also
     exposed: `POST /api/v1/operations/v2 { def_id: "dedup.embed-scan" }`
     (already wired in UOS-06).

2. `internal/server/server.go` — register the plugin set:
   - After registry init, call `dedup.New(...).Register(opRegistry)`.

## Hard rules

- The Run function MUST be functionally identical to the existing
  `triggerEmbedScan` opFunc. Only the wiring changes.
- Do NOT touch other dedup ops (acoustid, llm, scan) in this PR.
- Do NOT delete the old route — it now delegates to the registry.
- The plugin's `OnEnable` is a no-op; enable-state plumbing is
  handled in UOS-08 (watchdog) / UOS-12 (maintenance plugin
  defaults).

## Acceptance criteria

- [ ] `go test ./internal/plugins/dedup/...` passes.
- [ ] `make ci` passes.
- [ ] Manual on staging: click "Re-embed All" on the dedup page; op
      appears in bell + Activity within 1s; completes; status flips
      to `completed`; logs are tagged with `op_id`, `plugin=dedup`,
      `def_id=dedup.embed-scan`.
- [ ] Manual: trigger embed-scan, then `make deploy` while it's
      running. After restart: op status reflects `interrupted_*`;
      the next user-triggered embed-scan runs cleanly. Because
      ResumePolicy=ResumeRequeue, the old run is dropped and a new
      one is fresh.

## PR title

```
feat(uos): canary — migrate dedup.embed-scan to UOS
```
