<!-- file: HANDOFF.md -->
<!-- version: 1.0.0 -->
<!-- guid: 6c7d8e9f-0a1b-2c3d-4e5f-6a7b8c9d0e1f -->

# Handoff Notes (Book Detail Metadata/Tags)

## Context
- Worktree: `../audiobook-organizer-wt` (branch `feature/book-detail-metadata-ui`, tracking `origin/main`). Use this worktree to avoid clobbering other agents.
- Frontend: Book Detail has Info/Files/Versions, Tags, Compare tabs; Edit Metadata button; Compare tab apply actions. Playwright tests (Chromium + WebKit) use mocked tags/provenance.
- Backend: Added `GET /api/v1/audiobooks/:id/tags` that currently returns file/stored values and media info (no provenance/override). Helper functions added for pointer values. Go tests pass.
- TODO.md/CURRENT_STATUS.md updated with backend gaps and next steps.

## What’s Left (Backend/Frontend alignment)
- Backend needs to persist per-field provenance/override/lock flags, resolve author/series names, and return fetched/override values in tags response. Update `UpdateBook` to accept override payloads/locks. Add handler tests for tags endpoint.
- Frontend Compare/Tags tabs should be aligned to final payload (fetched/override/locked). Playwright mocks cover Tags/Compare; adjust once backend shape is final.
- Ensure `web/package-lock.json` changes are intentional (npm install was run).

## How to Start
1) `cd ../audiobook-organizer-wt`
2) Implement backend changes (DB model/handlers) for tags/provenance/overrides; add Go tests for `getAudiobookTags`.
3) Align frontend data shaping to new payload (Compare tab actions/labels, override handling).
4) Run: `go test ./...`, `cd web && npm run lint`, `npm run test:e2e -- book-detail`.
5) Keep TODO.md and CURRENT_STATUS.md in sync with progress.
