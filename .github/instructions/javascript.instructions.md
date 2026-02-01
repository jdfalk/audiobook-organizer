<!-- file: .github/instructions/javascript.instructions.md -->
<!-- version: 2.0.0 -->
<!-- guid: 8e7d6c5b-4a3c-2d1e-0f9a-8b7c6d5e4f3a -->
<!-- last-edited: 2026-01-31 -->

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
---
applyTo: "**/*.{js,jsx}"
description: |
  JavaScript coding rules. This repo primarily uses TypeScript. JS rules mirror TS rules where applicable.
---
<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->

# JavaScript Coding Instructions

This repo primarily uses **TypeScript**. Any JavaScript follows the same conventions.
See [typescript.instructions.md](typescript.instructions.md) for the full set of rules.

Key JS-specific points:
- Use `const`/`let`. **Never `var`.**
- Arrow functions for callbacks. `function` declarations for named top-level functions.
- Single quotes. Semicolons required. 2-space indent. 80-char line limit.
- Prefer named exports over default exports.
- JSDoc all public functions with `@param` and `@returns`.
- Full reference: [Google JavaScript Style Guide](https://google.github.io/styleguide/jsguide.html)

## Required File Header

```js
// file: path/to/file.js
// version: 1.0.0
// guid: 123e4567-e89b-12d3-a456-426614174000
```
