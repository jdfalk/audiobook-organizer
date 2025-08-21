<!-- file: .github/instructions/rust.instructions.md -->
<!-- version: 1.0.0 -->
<!-- guid: b2c3d4e5-f6a7-8901-2345-678901bcdef0 -->

---
applyTo: "**/*.rs"
description: |
  Rust coding style and best practices for all Rust source files. Follows official Rust conventions and community standards for safe, idiomatic, and performant Rust code.
---

# Rust Code Style Guide

This guide follows the official Rust style conventions and community best practices for all Rust source files in this repository.

## Formatting

- Use `rustfmt` to automatically format code
- Use 4 spaces for indentation (no tabs)
- Maximum line length of 100 characters
- Use Unix-style line endings (LF)
- End files with a newline character

## Naming Conventions

- Types, traits, and enums: `PascalCase` (e.g., `HashMap`, `ToString`)
- Modules, functions, methods, variables: `snake_case` (e.g., `process_data`, `file_path`)
- Constants: `SCREAMING_SNAKE_CASE` (e.g., `MAX_ITERATIONS`)
- Static variables: `SCREAMING_SNAKE_CASE`
- Macro names: `snake_case!` by convention, but some use `PascalCase!` for constructor-like macros
- Lifetimes: short, lowercase names like `'a`, `'ctx`, `'src`
- Crates: `kebab-case` for package names, `snake_case` for imports

```rust
// Good naming examples
struct DatabaseConnection;
trait DataSerialization {}
const MAX_CONNECTIONS: u32 = 100;
static DEFAULT_SETTINGS: &str = "default";
fn connect_to_database(url: &str) -> Result<DatabaseConnection, ConnectionError> {
    // Implementation
}
```

## Imports and Modules

- Use nested paths for imports from the same crate/module
- Order imports: std first, then external crates, then local imports
- Separate import groups with a blank line
- Alphabetize within each import group
- Prefer using absolute paths (starting with crate name) rather than relative paths

```rust
// Good import ordering
use std::collections::HashMap;
use std::fs::{self, File};
use std::path::{Path, PathBuf};

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

use crate::error::CustomError;
use crate::models::User;
```

## Comments and Documentation

- Use `///` for outer documentation comments (generates docs)
- Use `//` for inner implementation comments
- Use `//!` for module-level documentation
- Follow [rustdoc conventions](https://doc.rust-lang.org/rustdoc/how-to-write-documentation.html)
- Document all public items (functions, methods, structs, enums, traits)
- Include examples in documentation where helpful

```rust
/// Represents a user in the system.
///
/// # Examples
///
/// ```
/// let user = User::new("username", "password");
/// assert_eq!(user.username(), "username");
/// ```
pub struct User {
    username: String,
    password_hash: String,
}
```

## Error Handling

- Use `Result<T, E>` for operations that can fail
- Prefer returning errors over panicking
- Use `?` operator for error propagation
- Create custom error types for your libraries
- Implement `std::error::Error` trait for custom error types
- Use `thiserror` or similar crates for error handling utilities

```rust
fn read_config(path: &Path) -> Result<Config, ConfigError> {
    let content = fs::read_to_string(path)
        .map_err(|e| ConfigError::Io(e))?;

    let config = parse_config(&content)
        .map_err(|e| ConfigError::Parse(e))?;

    Ok(config)
}
```

## Control Structures

- Omit parentheses around conditions in `if`, `while`, etc.
- Put opening braces on the same line as the declaration
- Use `match` instead of `if let` when checking multiple conditions
- Use early returns to avoid nesting and improve readability
- Prefer `if let` and `while let` for single-pattern matching

```rust
// Control flow examples
if some_condition {
    do_something();
} else {
    do_alternative();
}

match value {
    Some(x) if x > 0 => process_positive(x),
    Some(x) => process_non_positive(x),
    None => handle_none(),
}
```

## Functions and Methods

- Keep functions short and focused on a single task
- Use default arguments via the `Option` type and the `Default` trait
- Return types that implement `Iterator` rather than concrete collections
- Use builder pattern for complex object construction
- Prefer methods to functions when operating on a specific data type

## Memory Management and Ownership

- Follow the ownership rules religiously
- Use references when ownership is not needed
- Prefer slices (`&[T]`) over references to vectors (`&Vec<T>`)
- Use `Cow<T>` for situations requiring both owned and borrowed data
- Use `AsRef` and `AsMut` traits for flexible argument types

## Generics and Traits

- Use trait bounds to constrain generic types
- Implement standard traits when appropriate (e.g., `Display`, `Debug`, `Clone`)
- Use trait objects when runtime polymorphism is required
- Prefer associated types over generic parameters for traits with a single implementation
- Use the `where` clause for complex trait bounds

```rust
// Complex trait bounds with where clause
fn process_data<T, U>(data: T, processor: U) -> Result<Vec<String>, ProcessError>
where
    T: AsRef<[u8]> + Send + 'static,
    U: Processor + Clone,
{
    // Implementation
}
```

## Required File Header

All Rust source files MUST begin with the standard file header:

```rust
// file: path/to/file.rs
// version: 1.0.0
// guid: 123e4567-e89b-12d3-a456-426614174000
```

This header must come immediately after any shebang line but before all other content including imports and module declarations.

## Tools

- Use `cargo fmt` for automatic formatting
- Use `cargo clippy` for additional linting
- Use `cargo doc` to generate documentation
- Use `cargo test` for running tests
- Use `cargo bench` for benchmarking

## Best Practices

- Follow the API guidelines: <https://rust-lang.github.io/api-guidelines/>
- Create meaningful custom types instead of using primitive types directly
- Leverage the type system to make illegal states unrepresentable
- Use `#[derive]` for common traits when possible
- Write tests for public API functionality
- Use the `#[non_exhaustive]` attribute for enums that may grow in the future
- Prefer immutable variables (`let` without `mut`)

## Safety and Security

- Avoid `unsafe` code unless absolutely necessary and well-documented
- When using `unsafe`, clearly document the safety invariants
- Prefer standard library types over raw pointers
- Use `#[deny(unsafe_code)]` at the crate level if no unsafe code is needed
- Validate all external inputs and boundary conditions
- Use appropriate integer types to prevent overflow
