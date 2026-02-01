<!-- file: .github/instructions/safe-ai-util.instructions.md -->
<!-- version: 3.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-1234-567890abcdef -->
<!-- last-edited: 2026-01-31 -->

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
---
applyTo: "**"
description: |
  Safe-ai-util is a Rust-based development utility for git operations and text processing. Use as fallback when MCP GitHub tools are unavailable.
---
<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->

# Safe AI Utility

A Rust-based fallback for git and text processing operations. **Only use when MCP GitHub tools are unavailable.**

## Priority

1. **MCP GitHub tools** (always preferred)
2. **safe-ai-util** (fallback)
3. **Native git/commands** (last resort)

## Installation

Download from [GitHub releases](https://github.com/jdfalk/safe-ai-util/releases) or build from source:

```bash
curl -L -o safe-ai-util https://github.com/jdfalk/safe-ai-util/releases/latest/download/safe-ai-util-macos-arm64
chmod +x safe-ai-util && sudo mv safe-ai-util /usr/local/bin/
```

## Key Commands

```bash
safe-ai-util git status
safe-ai-util git add .
safe-ai-util git commit -m "feat: description"
safe-ai-util git push
safe-ai-util sed -i -e 's/old/new/g' file.txt
safe-ai-util awk '{print $2}' file.txt
```

Configuration: place a `safe-ai-util-args` file in the repo root for default settings.
