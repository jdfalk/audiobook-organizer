<!-- file: docs/TASK-6-README.md -->
<!-- version: 1.0.0 -->
<!-- guid: 7d2e9f1c-8a4b-4c5d-9f6e-1a7b2c3d4e5f -->
<!-- last-edited: 2026-01-19 -->

# Task 6: Book Detail Page & Enhanced Delete Flow - Complete Documentation

## 📖 Overview

This task implements a dedicated book detail view showing comprehensive
information (files, metadata, versions) and enhances the delete workflow with
reimport prevention controls. Core requirement: provide users with full context
before deletion and offer granular control over future import behavior.

**Deliverables:**

- Book detail page/modal showing all metadata fields, file info, version group,
  and history.
- File list showing audio files, cover art, and related resources with sizes.
- Version tab showing all editions/formats linked to this book.
- Enhanced delete dialog with "Prevent Reimporting of this file" checkbox.
- Delete flow integrates with hash tracking (Task 5) to add hashes to blocklist.
- Confirmation shows which hashes will be blocked (original + library).

## 📂 Document Set

| Document                       | Purpose                                         |
| ------------------------------ | ----------------------------------------------- |
| `TASK-6-CORE-TESTING.md`       | Core validation flow, phases, safety/locks      |
| `TASK-6-ADVANCED-SCENARIOS.md` | Edge cases (multi-file books, version handling) |
| `TASK-6-TROUBLESHOOTING.md`    | Issues, root causes, and fixes                  |
| `TASK-6-README.md` (this file) | Overview, navigation, quick commands            |

**Reading order:** README → Core → Advanced → Troubleshooting.

## 🎯 Success Criteria

- Clicking a book in Library view opens detail page/modal.
- Detail view shows: title, author, series, narrator, publisher, year,
  description, duration, quality.
- File section lists all files with paths, sizes, formats.
- Versions section shows linked editions with quality comparison.
- Delete button opens enhanced dialog with reimport prevention checkbox.
- When checked, confirmation shows hashes to be blocked (original + library).
- After delete, book enters soft_deleted state and hashes added to blocklist.

## 🚀 Quick Start

```bash
# Check if detail page/modal exists
rg "BookDetail|AudiobookDetail" web/src -n | head -10

# Check delete dialog enhancement
rg "prevent.*reimport|Prevent.*Reimport" web/src -n

# Check API for full book details
curl -s http://localhost:8888/api/v1/audiobooks/BOOK_ID | jq '.'

# Check versions API
curl -s http://localhost:8888/api/v1/audiobooks/BOOK_ID/versions | jq '.'
```

## 🔐 Multi-AI Safety

- Use lock/state files under `/tmp/task-6-*`.
- Test delete flow with non-production books; verify soft-delete before purge.
- Capture book details and blocklist state before/after delete.

## 🧭 Navigation

- Need the main flow? → `TASK-6-CORE-TESTING.md`
- Handling edge cases? → `TASK-6-ADVANCED-SCENARIOS.md`
- Something broken? → `TASK-6-TROUBLESHOOTING.md`

## 🧩 Current State (from TODO)

- Priority: High (New Requirement, MVP-blocking for safe delete workflows)
- Status: Not Started
- Depends on: Task 5 (hash tracking infrastructure)

## ✅ Next Actions

1. Create BookDetail component (page or modal) with tabs: Info, Files, Versions.
2. Enhance delete dialog to include reimport prevention checkbox.
3. Update delete handler to accept `prevent_reimport` flag and reason.
4. Wire up confirmation showing which hashes will be blocked.
5. Add navigation from Library grid/list to detail view.
6. Run Core Phases to validate end-to-end flow.

---
