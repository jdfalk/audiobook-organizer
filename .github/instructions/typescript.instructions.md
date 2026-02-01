<!-- file: .github/instructions/typescript.instructions.md -->
<!-- version: 2.0.0 -->
<!-- guid: ts123456-e89b-12d3-a456-426614174000 -->
<!-- last-edited: 2026-01-31 -->

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
---
applyTo: "**/*.{ts,tsx}"
description: |
  TypeScript/React coding rules. Extends general-coding.instructions.md. Full reference: Google TypeScript Style Guide (https://google.github.io/styleguide/tsguide.html).
---
<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->

# TypeScript Coding Instructions

- Follow [general coding instructions](general-coding.instructions.md).
- Full style reference: [Google TypeScript Style Guide](https://google.github.io/styleguide/tsguide.html).
- All TypeScript files must begin with the required file header.

## Naming

- **Variables/functions/methods/properties:** camelCase
- **Classes/interfaces/types/enums:** PascalCase
- **Module-level constants:** SCREAMING_SNAKE_CASE
- No `I` prefix on interfaces. No trailing/leading underscores.
- Treat acronyms as words: `loadHttpUrl` not `loadHTTPURL`.

## Imports & Exports

- Use **named exports** only. No default exports.
- Prefer named imports for frequently used symbols; namespace imports (`* as foo`) for large APIs.
- Use relative imports (`./foo`) within the same project.

## Types

- Annotate function parameters and return types explicitly.
- Use `interface` for object shapes (not type aliases for objects).
- Use `T[]` syntax for simple array types; `Array<T>` for complex element types.
- Prefer `unknown` over `any`. Suppress lint + document if `any` is truly needed.
- Use `?` optional fields over `| undefined` in type definitions.
- Rely on type inference for trivially inferred types (literals, `new` expressions).

## Variables & Declarations

- `const` by default. `let` only when reassigned. **Never `var`.**
- One variable per declaration.
- Use array literals `[]`, not `new Array()`.
- Use object literals `{}`, not `new Object()`.

## Functions

- Prefer **function declarations** for named functions.
- Use **arrow functions** for callbacks and nested functions.
- No function expressions (use arrows instead).
- Arrow concise bodies when the return value is used.

## Classes

- No `#private` fields — use TypeScript `private` keyword.
- Mark never-reassigned properties `readonly`.
- Use parameter properties: `constructor(private readonly svc: Service) {}`
- No empty constructors (ES2015 provides defaults).
- No `public` modifier except on non-readonly constructor parameter properties.

## Control Flow

- Always use `===` / `!==`. Exception: `== null` to catch both null and undefined.
- No type assertions (`as`) or non-null assertions (`!`) — write runtime checks instead.
- Use `as` syntax (not angle brackets) when assertions are unavoidable.

## Comments

- `/** JSDoc */` for user-facing documentation.
- `//` for implementation comments.
- No JSDoc type annotations (TypeScript handles types). No `@implements`, `@private` etc. on typed code.
- Document all exported symbols. Method descriptions start with a verb phrase (third person).

## Formatting

- 2 spaces indentation. Max 80 chars per line.
- Semicolons required (no ASI reliance).
- Single quotes. Template literals for interpolation.
- Trailing commas in multiline structures.

## Disallowed

- `const enum` (use plain `enum`)
- `eval` / `Function()` constructor
- Wrapper objects (`new String()`, `new Boolean()`, `new Number()`)
- Modifying builtins
- `debugger` statements in production
- Non-standard ECMAScript features

## Testing

- Descriptive test names. AAA pattern (Arrange, Act, Assert).
- Table-driven tests for multiple scenarios.

## Required File Header

```typescript
// file: path/to/file.ts
// version: 1.0.0
// guid: 123e4567-e89b-12d3-a456-426614174000
```
