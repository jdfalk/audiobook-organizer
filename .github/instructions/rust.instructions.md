<!-- file: .github/instructions/rust.instructions.md -->
<!-- version: 2.0.0 -->
<!-- guid: b2c3d4e5-f6a7-8901-2345-678901bcdef0 -->
<!-- last-edited: 2026-01-31 -->

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
---
applyTo: "**/*.rs"
description: |
  Rust coding rules. Extends general-coding.instructions.md. This repo has minimal Rust (safe-ai-util utility only).
---
<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->

# Rust Coding Instructions

This repo has minimal Rust code. Follow standard Rust conventions:

- **`rustfmt`** for formatting. **`clippy`** for linting. Non-negotiable.
- 4 spaces. 100 char line limit. LF line endings.
- **Naming:** Types/traits/enums `PascalCase`. Functions/variables `snake_case`. Constants `SCREAMING_SNAKE_CASE`.
- **Imports:** std → external crates → local. Alphabetized within groups.
- **Errors:** Use `Result<T, E>`. Custom error types with `thiserror`. Propagate with `?`.
- **Safety:** Avoid `unsafe`. Document invariants if used. `#[deny(unsafe_code)]` at crate level when possible.
- Document all public items with `///` doc comments.

## Required File Header

```rust
// file: path/to/file.rs
// version: 1.0.0
// guid: 123e4567-e89b-12d3-a456-426614174000
```
