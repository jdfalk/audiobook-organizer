<!-- file: .github/instructions/javascript.instructions.md -->
<!-- version: 1.2.1 -->
<!-- guid: 8e7d6c5b-4a3c-2d1e-0f9a-8b7c6d5e4f3a -->
<!-- DO NOT EDIT: This file is managed centrally in ghcommon repository -->
<!-- To update: Create an issue/PR in jdfalk/ghcommon -->

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
---
applyTo: "**/*.{js,jsx}"
description: |
  JavaScript language-specific coding, documentation, and testing rules for Copilot/AI agents and VS Code Copilot customization. These rules extend the general instructions in `general-coding.instructions.md` and merge all unique content from the Google
JavaScript Style Guide.

---
<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->

# JavaScript Coding Instructions

- Follow the [general coding instructions](general-coding.instructions.md).
- Follow the
  [Google JavaScript Style Guide](https://google.github.io/styleguide/jsguide.html)
  for additional best practices.
- All JavaScript files must begin with the required file header (see general
  instructions for details and JavaScript example).

## File Structure

- Use `camelCase` for file names
- Each file should contain exactly one ES module
- Prefer ES6 modules (`import`/`export`) over other module systems

## Formatting

- Use 2 spaces for indentation
- Line length maximum of 80 characters
- Use semicolons to terminate statements
- Use single quotes for string literals
- Add trailing commas for multi-line object/array literals
- Use parentheses only where required for clarity or priority

## Naming Conventions

- `functionNamesLikeThis`, `variableNamesLikeThis`, `ClassNamesLikeThis`,
  `EnumNamesLikeThis`, `methodNamesLikeThis`, `CONSTANT_VALUES_LIKE_THIS`,
  `private_values_with_underscore`, `package-names-like-this`

## Comments

- Use JSDoc for documentation
- Comment all non-obvious code sections

## Language Features

- Use `const` and `let` instead of `var`
- Use arrow functions for anonymous functions, especially callbacks
- Use template literals instead of string concatenation
- Use object/array destructuring where it improves readability
- Use default parameters instead of conditional statements
- Use spread operator and rest parameters where appropriate

## Best Practices

- Avoid using the global scope
- Avoid deeply nested code blocks
- Use early returns to reduce nesting
- Limit line length to improve readability
- Separate logic and display concerns

## Error Handling

- Always handle Promise rejections
- Use try/catch blocks appropriately
- Provide useful error messages

## Testing

- Write unit tests for all code
- Use descriptive test names
- Follow AAA pattern (Arrange, Act, Assert)

## Required File Header

All JavaScript files must begin with a standard header as described in the
[general coding instructions](general-coding.instructions.md). Example for
JavaScript:

```js
// file: path/to/file.js
// version: 1.0.0
// guid: 123e4567-e89b-12d3-a456-426614174000
```

## Google JavaScript Style Guide (Complete)

Follow the complete Google JavaScript Style Guide below for all JavaScript code:

### Google JavaScript Style Guide (Complete)

This style guide provides comprehensive conventions for writing clean, readable, and maintainable JavaScript code.

#### Source File Basics

**File Name:** File names must be all lowercase and may include underscores (`_`) or dashes (`-`), but no additional punctuation. Filenames' extension must be `.js`.

**File Encoding:** Source files are encoded in UTF-8.

**Special Characters:**

- Use ASCII horizontal space character (0x20) as the only whitespace character
- Tab characters are not used for indentation
- Use special escape sequences (`\'`, `\"`, `\\`, `\b`, `\f`, `\n`, `\r`, `\t`, `\v`) rather than numeric escapes
- For non-ASCII characters, use actual Unicode character or equivalent hex/Unicode escape based on readability

#### Source File Structure

All files follow this order:

1. License or copyright information (if present)
2. `@fileoverview` JSDoc (if present)
3. `goog.module` statement or ES `import` statements
4. `goog.require` and `goog.requireType` statements
5. The file's implementation

**ES Module Guidelines:**

- Use `import` statement for ES module files
- File extension `.js` is required in import paths
- Module import names use `lowerCamelCase` derived from imported file name
- Prefer named exports over default exports
- Exported variables must not be mutated outside module initialization

#### Formatting

**Braces:**

- Braces required for all control structures
- Follow K&R style (Egyptian brackets)
- No line break before opening brace
- Line break after opening brace and before closing brace

**Indentation:**

- Use +2 spaces for block indentation
- Array and object literals may be formatted as block-like constructs
- Function expressions indented +2 from preceding indentation

**Statements:**

- One statement per line
- Semicolons are required

**Column Limit:**

- 80 characters maximum
- Exceptions: `goog.module`, `goog.require`, ES module statements, long URLs, shell commands, file paths

**Line Wrapping:**

- Break at higher syntactic level when possible
- Operators: break comes after the symbol
- Continuation lines indented at least +4 spaces
- Method/constructor names stay with opening parenthesis

**Whitespace:**

- Single blank line between consecutive methods
- Space after reserved words before parenthesis (except `function` and `super`)
- Space before opening curly brace
- Space on both sides of binary/ternary operators
- Space after comma or semicolon
- No trailing whitespace

#### Language Features

**Variable Declarations:**

- Use `const` by default, `let` when reassignment needed
- Never use `var`
- One variable per declaration
- Declare close to first use
- JSDoc type annotations when needed

**Arrays:**

- Use trailing commas with line breaks
- Use array literals `[]` instead of `new Array()`
- Use spread operator instead of `Array.prototype` methods
- Destructuring allowed with default values

**Objects:**

- Use trailing commas with line breaks
- Use object literals `{}` instead of `new Object()`
- Don't mix quoted and unquoted keys
- Method shorthand allowed: `{method() {...}}`
- Shorthand properties allowed: `{foo, bar}`
- Computed property names allowed for dict-style

**Classes:**

- Define all fields in constructor
- Annotate fields with `@const`, `@private`, `@protected` as appropriate
- Computed properties only for symbols
- Prefer module-local functions over private static methods
- Use `@override` annotation for overridden methods

**Functions:**

- Arrow functions preferred for nested functions and callbacks
- Use `function` keyword for top-level functions and methods
- Default parameters allowed with `=` operator
- Rest parameters with `...` prefix
- Generators: attach `*` to `function` keyword

**Strings:**

- Use single quotes for ordinary strings
- Template literals for complex concatenation or multi-line strings
- No line continuations with backslash

**Control Structures:**

- Use `for-of` loops when possible
- Empty catch blocks must have explanatory comment
- Switch statements: comment fall-through, include default case

**Equality:**

- Use `===` and `!==`
- Exception: `== null` to catch both null and undefined

#### Naming Conventions

- **Package names:** `lowerCamelCase`
- **Class names:** `UpperCamelCase`
- **Method names:** `lowerCamelCase`, verbs or verb phrases
- **Enum names:** `UpperCamelCase`, items in `CONSTANT_CASE`
- **Constants:** `CONSTANT_CASE` for module-level constants
- **Fields:** `lowerCamelCase`, optional trailing underscore for private
- **Parameters:** `lowerCamelCase`
- **Local variables:** `lowerCamelCase`

#### JSDoc Requirements

**General Form:**

```javascript
/**
 * Multiple lines of JSDoc text are written here,
 * wrapped normally.
 * @param {number} arg A number to do something to.
 */
function doSomething(arg) { ... }
```

**Required Documentation:**

- All classes, interfaces, and records
- All public methods and functions
- All enum and typedef declarations
- Parameter and return types (except same-signature overrides)

**Type Annotations:**

- Use `!` for non-null, `?` for nullable reference types
- Primitives are non-nullable by default
- Always specify template parameters
- Function type expressions: always specify return type

#### Disallowed Features

- `with` keyword
- `eval` or `Function(...string)` constructor
- Non-standard features or proprietary extensions
- Wrapper objects for primitives (`new Boolean`, `new Number`, etc.)
- Modifying builtin objects
- Omitting parentheses when invoking constructors

#### Best Practices

- Use descriptive names, avoid abbreviations
- Prefer consistency when style guide doesn't specify
- Use compiler warnings and address them appropriately
- Mark deprecated code with `@deprecated` annotation
- Extract methods or variables instead of line-wrapping when possible
- Use horizontal alignment sparingly
- Prefer clear code over clever code
