<!-- file: .github/instructions/shell.instructions.md -->
<!-- version: 1.3.1 -->
<!-- guid: 5b4a3c2d-1e0f-9a8b-7c6d-5e4f3a2b1c0d -->
<!-- DO NOT EDIT: This file is managed centrally in ghcommon repository -->
<!-- To update: Create an issue/PR in jdfalk/ghcommon -->

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
---
applyTo: "**/*.{sh,bash}"
description: |
  Coding, documentation, and workflow rules for shell scripts, following Google Shell style guide and general project rules. Reference this for all shell scripts, documentation, and formatting in this repository. All unique content from the Google Shell Style Guide is merged here.
---
<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->

# Shell Script Coding Instructions

- Follow the [general coding instructions](general-coding.instructions.md).
- Follow the
  [Google Shell Style Guide](https://google.github.io/styleguide/shellguide.html)
  for additional best practices.
- All shell scripts must begin with the required file header (see general
  instructions for details and Shell example).

## Core Principles

- Readability: Scripts should be clear and easy to understand
- Robustness: Handle errors gracefully and fail safely
- Portability: Write scripts that work across different environments
- Maintainability: Code should be easy to modify and debug
- Security: Avoid common security pitfalls

## Script Structure

- Use bash for advanced features, sh for portability
- Use `.sh` extension for shell scripts
- Make scripts executable: `chmod +x script.sh`
- Start with shebang line, then header
- Include description and usage information
- Use `set -euo pipefail` for robust error handling
- Use lowercase for local variables, UPPERCASE for constants
- Quote variables to prevent word splitting
- Use descriptive names
- Document function purpose, parameters, and return values

## Required File Header

All shell scripts must begin with a standard header as described in the
[general coding instructions](general-coding.instructions.md). Example for
Shell:

```bash
#!/bin/bash
# file: path/to/script.sh
# version: 1.0.0
# guid: 123e4567-e89b-12d3-a456-426614174000
```

## HEREDOC (Here Document) Usage - AVOID IF POSSIBLE

**CRITICAL**: If you use heredocs, the ending delimiter MUST be EXACTLY the same as the opening delimiter.

**Best Practice: DON'T USE HEREDOC** - Use simpler alternatives instead:

```bash
# ❌ AVOID (HEREDOC - easy to mess up)
cat > file.txt << 'EOF'
content here
EOF

# ✅ BETTER - Use echo with redirection
echo "content here" > file.txt

# ✅ BETTER - Use printf
printf "line1\nline2\n" > file.txt

# ✅ BETTER - Use tee
echo "content" | tee file.txt
```

### If You MUST Use HEREDOC

The ending delimiter MUST match the opening delimiter exactly, on its own line, no indentation (unless using `<<-`):

```bash
# ✅ CORRECT
cat > file.txt << 'EOF'
content here
EOF

# ❌ WRONG - Delimiter mismatch
cat > file.txt << 'EOF'
content here
END  # WRONG - must be EOF
```

**Rule**: Avoid HEREDOC when possible. Use echo, printf, or tee instead. Only use HEREDOC for truly multi-line content that requires it.
