<!-- file: AGENTS.md -->
<!-- version: 4.0.0 -->
<!-- guid: 2e7c1a4b-5d3f-4b8c-9e1f-7a6b2c3d4e5f -->
<!-- last-edited: 2026-01-31 -->

# AGENTS.md — Instruction File Index

All coding standards and AI agent instructions for the audiobook organizer.

## Core Instructions

| File | Applies to | Purpose |
|------|-----------|---------|
| [copilot-instructions.md](.github/copilot-instructions.md) | All | Repo architecture, workflow policy |
| [general-coding.instructions.md](.github/instructions/general-coding.instructions.md) | `**` | Universal rules: headers, git, testing |
| [commit-messages.instructions.md](.github/instructions/commit-messages.instructions.md) | `**` | Conventional commit format |
| [pull-request-descriptions.instructions.md](.github/instructions/pull-request-descriptions.instructions.md) | `**` | PR description template |
| [test-generation.instructions.md](.github/instructions/test-generation.instructions.md) | test files | Testing standards |
| [security.instructions.md](.github/instructions/security.instructions.md) | `**` | Security best practices |

## Language Instructions

| File | Applies to |
|------|-----------|
| [go.instructions.md](.github/instructions/go.instructions.md) | `**/*.go` |
| [typescript.instructions.md](.github/instructions/typescript.instructions.md) | `**/*.{ts,tsx}` |
| [javascript.instructions.md](.github/instructions/javascript.instructions.md) | `**/*.{js,jsx}` |
| [python.instructions.md](.github/instructions/python.instructions.md) | `**/*.py` |
| [shell.instructions.md](.github/instructions/shell.instructions.md) | `**/*.{sh,bash}` |
| [rust.instructions.md](.github/instructions/rust.instructions.md) | `**/*.rs` |
| [protobuf.instructions.md](.github/instructions/protobuf.instructions.md) | `**/*.proto` |
| [markdown.instructions.md](.github/instructions/markdown.instructions.md) | `**/*.md` |
| [json.instructions.md](.github/instructions/json.instructions.md) | `**/*.json` |
| [html-css.instructions.md](.github/instructions/html-css.instructions.md) | `**/*.{html,css}` |
| [github-actions.instructions.md](.github/instructions/github-actions.instructions.md) | `.github/workflows/*.yml` |

## Prompts

All in `.github/prompts/`. Key ones:

- `code-review.prompt.md` — Code review guidance
- `pull-request.prompt.md` — PR creation
- `commit-message.prompt.md` — Commit message generation
- `security-review.prompt.md` — Security review
- `bug-report.prompt.md` / `feature-request.prompt.md` — Issue templates
- `merge-conflict-resolution.agent.md` — Merge conflict resolution
- `ai-rebase-system.prompt.md` — Rebase workflow

## Version Policy

When modifying any file with a version header, **always update the version:**

- **Patch** (x.y.Z): typos, minor fixes
- **Minor** (x.Y.z): new content, additions
- **Major** (X.y.z): structural changes

## Git Operations

1. MCP GitHub tools (preferred)
2. Native git (fallback)
