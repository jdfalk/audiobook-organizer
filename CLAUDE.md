<!-- file: CLAUDE.md -->
<!-- version: 2.2.0 -->
<!-- guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f -->

# CLAUDE.md

> **NOTE:** This file is a pointer. All Claude/AI agent and workflow instructions are now
> centralized in the `.github/instructions/` and `.github/prompts/` directories.

## 🚨 CRITICAL: Documentation Update Protocol

This repository uses a direct-edit documentation workflow. The legacy doc-update scripts and
workflows are retired.

- Edit documentation directly in the target files.
- Always keep the required header (file path, version, guid) and bump the version on any change.
- Do not use create-doc-update.sh, doc_update_manager.py, or .github/doc-updates/.
- **Use `copilot-agent-util` for git operations** - Download latest from
  [releases](https://github.com/jdfalk/copilot-agent-util-rust/releases/latest)
  - The utility provides command filtering, safety checks, and consistent logging
  - VS Code tasks automatically use the utility when available
  - Use the utility directly for git commands: `copilot-agent-util git add`,
    `copilot-agent-util git commit`, etc.

## Canonical Source for Agent Instructions

- General and language-specific rules: `.github/instructions/` (all code style, documentation, and
  workflow rules are here)
- Prompts: `.github/prompts/`
- System documentation: `.github/copilot-instructions.md`

For all agent, Claude, or workflow tasks, **refer to the above files**. Do not duplicate or override
these rules elsewhere.
