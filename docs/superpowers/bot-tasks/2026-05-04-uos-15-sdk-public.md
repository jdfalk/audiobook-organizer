<!-- file: docs/superpowers/bot-tasks/2026-05-04-uos-15-sdk-public.md -->
<!-- version: 1.0.0 -->
<!-- guid: d5d6e7f8-a9b0-1c2d-3e4f-5a6b7c8d9e0f -->
<!-- last-edited: 2026-05-04 -->

# UOS-15 — Promote SDK to public import path + docs

**Companion human spec:** §7, §11 phase B.

## Branch

```
feat/uos-15-sdk-public
```

## Goal

The SDK has lived under `pkg/plugin/sdk/` since UOS-04, but it's
been informal. This PR makes it a stable public contract:
1. Re-verify there are zero `internal/...` imports from `pkg/plugin/sdk/...`.
2. Add a `doc.go` with a complete tutorial-style example.
3. Add a "Writing a plugin" doc page.
4. Add a CI lint that fails the build on any new `internal/` import
   from `pkg/plugin/sdk/`.

## Files to add

1. `docs/development/writing-a-plugin.md` — tutorial covering:
   - The contract (what an OperationDef is)
   - The lifecycle (`Plugin.Register` → `Registry.RegisterOp`)
   - Picking a `ResumePolicy` (decision tree)
   - When to set `Isolate: true`
   - Capability declaration vocabulary
   - Schedules vs triggers vs ad-hoc
   - Testing your plugin (table-driven OperationDef tests; integration
     tests using the registry test harness)
   - Worked example: a 60-line plugin that emits a `book.imported`
     trigger to record import counts to a custom table
2. `pkg/plugin/sdk/doc.go` — package-level godoc with a 30-line
   minimal-plugin example.
3. `tools/cmd/sdkguard/main.go` — CI tool that asserts no
   `internal/` package is in the dep tree of any `pkg/plugin/sdk`
   subpackage. Wired to `make sdkguard` and run in `make ci`.

## Files to edit

1. `Makefile` — add `make sdkguard`; include in `make ci`.
2. `docs/index.md` (or root README) — link to the writing-a-plugin
   doc.

## Hard rules

- This is a docs/lint PR. No production code changes.
- `pkg/plugin/sdk` API is now considered STABLE. Future breaking
  changes to its types require a deprecation cycle.

## Acceptance criteria

- [ ] `make sdkguard` passes.
- [ ] `make ci` passes.
- [ ] `go doc ./pkg/plugin/sdk` shows the documented example.

## PR title

```
docs(uos): promote pkg/plugin/sdk to stable public API
```
