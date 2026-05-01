<!-- file: docs/superpowers/specs/2026-04-30-settings-component-split.md -->
<!-- version: 1.0.0 -->
<!-- guid: 5e6f7a8b-9c0d-1e2f-3a4b-5c6d7e8f9a0b -->
<!-- last-edited: 2026-04-30 -->

# Settings.tsx Component Split

**Status:** Draft — awaiting implementation
**Scope:** `web/src/pages/Settings.tsx`, `web/src/components/settings/`
**Related specs:** [`2026-04-30-library-component-split.md`](./2026-04-30-library-component-split.md), [`2026-04-30-frontend-cleanup.md`](./2026-04-30-frontend-cleanup.md)

---

## Problem

**F-2 — Settings.tsx is 4,166 lines:**
`web/src/pages/Settings.tsx` contains 61 `useState` declarations. Similar to
Library.tsx, this makes the file untestable, fragile, and a merge-conflict magnet.
Unlike Library.tsx (which has display/interaction concerns), Settings.tsx has a
natural split: each settings tab is already a distinct UI section with its own
state and API calls.

---

## Core Rule / Goal

> **Each settings tab must become its own component in
> `web/src/components/settings/`. Settings.tsx becomes a thin tab-router that
> renders the active tab component.**

---

## Sub-Component Breakdown

| Task | Component | Tab content |
|------|-----------|-------------|
| FE-4 | `GeneralSettingsTab` | General/application-wide settings |
| FE-5 | `AudioPathsTab` | Import paths, library root, path configuration |
| FE-6 | `MetadataSettingsTab` | Metadata sources, OpenAI API key, fetch settings |

---

## Approach

For each tab extraction:

1. Identify which `useState` variables are used only within that tab's JSX subtree.
   These become internal state of the extracted component.
2. Identify which state is shared across tabs (e.g., a global "save pending" flag).
   These remain in Settings.tsx and are passed as props.
3. Create `web/src/components/settings/<TabComponent>.tsx`.
4. Move the state and JSX into the new component. Accept shared state as props with
   proper TypeScript types.
5. In Settings.tsx, replace the tab content block with `<TabComponent ... />`.
6. Run `npx --prefix web tsc --noEmit` to verify zero type errors.

---

## What Does NOT Change

- Tab navigation logic in Settings.tsx.
- The visual appearance of any settings tab.
- API calls invoked by each tab — they follow the relevant state into the sub-component.

---

## Acceptance Criteria

- [ ] `web/src/components/settings/GeneralSettingsTab.tsx` exists.
- [ ] `web/src/components/settings/AudioPathsTab.tsx` exists.
- [ ] `web/src/components/settings/MetadataSettingsTab.tsx` exists.
- [ ] `npx --prefix web tsc --noEmit` passes after each extraction.
- [ ] Settings.tsx line count is materially reduced after all three extractions.
- [ ] All settings tabs render and save correctly (smoke test or existing E2E pass).

---

## Related Bot-Tasks

- [`2026-04-30-fe-4-settings-general.md`](../bot-tasks/2026-04-30-fe-4-settings-general.md) — FE-4
- [`2026-04-30-fe-5-settings-paths.md`](../bot-tasks/2026-04-30-fe-5-settings-paths.md) — FE-5
- [`2026-04-30-fe-6-settings-metadata.md`](../bot-tasks/2026-04-30-fe-6-settings-metadata.md) — FE-6
