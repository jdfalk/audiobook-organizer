<!-- file: AGENTS.md -->
<!-- version: 2.2.2 -->
<!-- guid: 2e7c1a4b-5d3f-4b8c-9e1f-7a6b2c3d4e5f -->

# AGENTS.md

> **NOTE:** This is a pointer file. All detailed Copilot, agent, and workflow instructions are in
> the [.github/](.github/) directory.

## 🚨 CRITICAL: Documentation Update Protocol

This repository uses a direct-edit documentation workflow. The legacy doc-update scripts and
workflows are retired.

- Edit documentation directly in the target files.
- Always keep the required header (file path, version, guid) and bump the version on any change.
- Do not use create-doc-update.sh, doc_update_manager.py, or .github/doc-updates/.
- Prefer VS Code tasks for git operations (Git Add All, Git Commit, Git Push) when available.
  - These tasks use the `copilot-agent-util` Rust utility for enhanced logging, error handling, and
    safety.
  - Download: <https://github.com/jdfalk/copilot-agent-util-rust/releases/latest>

## ⚠️ CRITICAL: File Version Updates

**When modifying any file with a version header, ALWAYS update the version number:**

- **Patch version** (x.y.Z): Bug fixes, typos, minor formatting changes
- **Minor version** (x.Y.z): New features, significant content additions, template changes
- **Major version** (X.y.z): Breaking changes, structural overhauls, format changes

**Examples:**

- Fix typo: `1.2.3` → `1.2.4`
- Add new section: `1.2.3` → `1.3.0`
- Change template structure: `1.2.3` → `2.0.0`

**This applies to ALL files with version headers including documentation, templates, and
configuration files.**

## Key Copilot/Agent Documents

- [Copilot Instructions](.github/copilot-instructions.md)
- [Commit Message Standards](.github/commit-messages.md)
- [Pull Request Description Guidelines](.github/pull-request-descriptions.md)
- [Code Review Guidelines](.github/review-selection.md)
- [Test Generation Guidelines](.github/test-generation.md)
- [Security Guidelines](.github/security-guidelines.md)
- [Repository Setup Guide](.github/repository-setup.md)
- [Workflow Usage](.github/workflow-usage.md)
- [All Code Style Guides](.github/)

> For any agent, Copilot, or workflow task, **always refer to the above files.**
