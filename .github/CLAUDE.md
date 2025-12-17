<!-- file: .github/CLAUDE.md -->
<!-- version: 2.2.0 -->
<!-- guid: 3c4d5e6f-7a8b-9c0d-1e2f-3a4b5c6d7e8f -->

# CLAUDE.md

> **NOTE:** This file is a pointer. All Claude/AI agent and workflow
> instructions are now centralized in the `.github/instructions/` and
> `.github/prompts/` directories.

## 🚨 CRITICAL: Documentation Update Protocol

This repository no longer uses doc-update scripts. Follow these rules instead:

- Edit documentation directly in the target files.
- Keep the required header (file path, version, guid) and bump the version on any change.
- Do not use create-doc-update.sh or related scripts; they are retired.
- For git operations: 1) MCP GitHub tools (preferred), 2) safe-ai-util (fallback), 3) native git (last resort). Use VS Code tasks for non-git operations only.
- Follow the guidance in `.github/instructions/general-coding.instructions.md`.

## Canonical Source for Agent Instructions

- General and language-specific rules: `.github/instructions/` (all code style,
  documentation, and workflow rules are here)
- Prompts: `.github/prompts/`
- System documentation: `.github/copilot-instructions.md`

For all agent, Claude, or workflow tasks, **refer to the above files**. Do not
duplicate or override these rules elsewhere.
