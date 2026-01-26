<!-- file: AGENTS.md -->
<!-- version: 3.0.0 -->
<!-- guid: 2e7c1a4b-5d3f-4b8c-9e1f-7a6b2c3d4e5f -->
<!-- last-edited: 2026-01-25 -->

# AGENTS.md

> **NOTE:** This is a pointer file. All detailed Copilot, agent, and workflow
> instructions are in the [.github/](.github/) directory.

## 🎯 Quick Reference

**Main Documentation:**

- [Copilot Instructions](.github/copilot-instructions.md) - Primary AI agent configuration
- [Instructions Directory](.github/instructions/) - All coding standards and language-specific rules
- [Prompts Directory](.github/prompts/) - Specialized prompts for specific tasks

## 📋 Instruction Files

### Core Workflow Instructions

- [commit-messages.instructions.md](.github/instructions/commit-messages.instructions.md) -
  Conventional commit standards
- [pull-request-descriptions.instructions.md](.github/instructions/pull-request-descriptions.instructions.md) -
  PR description templates
- [test-generation.instructions.md](.github/instructions/test-generation.instructions.md) -
  Test writing guidelines
- [security.instructions.md](.github/instructions/security.instructions.md) -
  Security best practices
- [safe-ai-util.instructions.md](.github/instructions/safe-ai-util.instructions.md) -
  Git operation safety

### Language-Specific Instructions

- [general-coding.instructions.md](.github/instructions/general-coding.instructions.md) -
  Universal coding standards
- [go.instructions.md](.github/instructions/go.instructions.md) - Go language rules
- [python.instructions.md](.github/instructions/python.instructions.md) - Python language rules
- [typescript.instructions.md](.github/instructions/typescript.instructions.md) - TypeScript/React rules
- [javascript.instructions.md](.github/instructions/javascript.instructions.md) - JavaScript rules
- [rust.instructions.md](.github/instructions/rust.instructions.md) - Rust language rules
- [shell.instructions.md](.github/instructions/shell.instructions.md) - Shell scripting rules
- [protobuf.instructions.md](.github/instructions/protobuf.instructions.md) - Protobuf standards
- [markdown.instructions.md](.github/instructions/markdown.instructions.md) - Markdown formatting
- [json.instructions.md](.github/instructions/json.instructions.md) - JSON formatting
- [html-css.instructions.md](.github/instructions/html-css.instructions.md) - HTML/CSS rules
- [github-actions.instructions.md](.github/instructions/github-actions.instructions.md) -
  CI/CD workflow rules

### Specialized Prompts

- [code-review.prompt.md](.github/prompts/code-review.prompt.md) - Code review guidance
- [documentation.prompt.md](.github/prompts/documentation.prompt.md) - Documentation generation
- [bug-report.prompt.md](.github/prompts/bug-report.prompt.md) - Bug report templates
- [feature-request.prompt.md](.github/prompts/feature-request.prompt.md) - Feature request templates
- [merge-conflict-resolution.agent.md](.github/prompts/merge-conflict-resolution.agent.md) -
  Merge conflict help
- [test-generation.prompt.md](.github/prompts/test-generation.prompt.md) - Test generation prompts

## 🚨 CRITICAL: File Version Updates

**When modifying any file with a version header, ALWAYS update the version number:**

- **Patch version** (x.y.Z): Bug fixes, typos, minor formatting changes
- **Minor version** (x.Y.z): New features, significant content additions
- **Major version** (X.y.z): Breaking changes, structural overhauls

**This applies to ALL files with version headers including documentation, templates, and configuration files.**

## 🔧 Git Operations

**Preferred order for git operations:**

1. MCP GitHub tools (preferred)
2. safe-ai-util (fallback)
3. Native git commands (last resort)

**Use VS Code tasks for non-git operations only (build, lint, test).**

> For any agent, Copilot, or workflow task, **always refer to the files listed above.**
