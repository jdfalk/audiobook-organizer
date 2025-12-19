<!-- file: .github/instructions/go.instructions.md -->
<!-- version: 1.11.0 -->
<!-- guid: 4a5b6c7d-8e9f-1a2b-3c4d-5e6f7a8b9c0d -->
<!-- DO NOT EDIT: This file is managed centrally in ghcommon repository -->
<!-- To update: Create an issue/PR in jdfalk/ghcommon -->

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
---
applyTo: "**/*.go"
description: |
  Go language-specific coding, documentation, and testing rules for Copilot/AI agents and VS Code Copilot customization. These rules extend the general instructions in `general-coding.instructions.md` and merge all unique content from the Google Go Style Guide.
---
<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->

# Go Coding Instructions

- Follow the [general coding instructions](general-coding.instructions.md).
- Follow the
  [Google Go Style Guide](https://google.github.io/styleguide/go/index.html) for
  additional best practices.
- All Go files must begin with the required file header (see general
  instructions for details and Go example).

## Core Principles

- Clarity over cleverness: Code should be clear and readable
- Simplicity: Prefer simple solutions over complex ones
- Consistency: Follow established patterns within the codebase
- Readability: Code is written for humans to read

## Version Requirements

- **MANDATORY**: All Go projects must use Go 1.23.0 or higher
- **NO EXCEPTIONS**: Do not use older Go versions in any repository
- Update `go.mod` files to specify `go 1.23` minimum version
- Update `go.work` files to specify `go 1.23` minimum version
- All Go file headers must use version 1.23.0 or higher
- Use `go version` to verify your installation meets requirements

## Architecture: Go Version Strategy

This repository uses **branch-aware version targeting** to support multiple Go versions across parallel release tracks, enabling gradual adoption of Go 1.24 and 1.25 features while maintaining stability for production systems.

### Supported Go Versions

The following Go versions are supported with branch-specific policies:

- **Go 1.23**: Stable production version (supported on all branches)
- **Go 1.24**: Current stable with modern features (main branch + stable-1-go-1.24)
- **Go 1.25**: Latest with cutting-edge features (main branch only)

### Branch-Specific Version Policies

#### Main Branch

- **Go Versions**: 1.23, 1.24, 1.25
- **Policy**: Latest versions with all features
- **Use Case**: Active development, new features, testing
- **Feature Access**: All Go 1.25 features available

#### Stable Branches (stable-1-go-X.XX)

- **Go Versions**: Locked to specific version
- **Policy**: Version-locked for stability
- **Examples**:
  - `stable-1-go-1.24`: Go 1.24 only
  - `stable-1-go-1.23`: Go 1.23 only
- **Use Case**: Production deployments, long-term support
- **Feature Access**: Features available in locked version only

#### EOL and Work-Stopped Branches

- **EOL Branches**: Critical security fixes only
- **Work-Stopped Branches**: No further development
- **Policy**: Maintained for legacy systems, no new features

### Feature Adoption Timeline

#### Go 1.24 Features (Available Now)

- **testing.B.Loop()**: New benchmark loop syntax (see testing section)
- **Improved error wrapping**: Enhanced error context
- **Performance improvements**: Faster compilation, runtime optimizations

**Adoption**:

- ✅ Main branch: Use freely
- ✅ stable-1-go-1.24: Use freely
- ❌ stable-1-go-1.23: Not available

#### Go 1.25 Features (Available on Main)

- **os.Root()**: Filesystem root isolation (see security section)
- **Generic optimization**: Core Types removal, performance improvements
- **Integer range iteration**: `for i := range n {}` syntax (see iteration section)
- **Enhanced type inference**: Improved generic type deduction

**Adoption**:

- ✅ Main branch: Use freely for new code
- ⏳ stable-1-go-1.24: Wait for branch policy update
- ❌ stable-1-go-1.23: Not available

### Version Detection in Workflows

The CI/CD system automatically detects appropriate Go versions based on branch configuration:

```yaml
# Automatically resolved from workflow-versions.yml
go-versions:
  main: ["1.23", "1.24", "1.25"]
  stable-1-go-1.24: ["1.24"]
  stable-1-go-1.23: ["1.23"]
```

**Workflow Behavior**:

- **Main branch**: Tests against all supported versions
- **Stable branches**: Tests against locked version only
- **Release builds**: Uses highest version for the branch

### Migration Guide

#### Adopting Go 1.24 Features

1. **Verify Branch Policy**:

   ```bash
   # Check current branch
   git branch --show-current

   # Verify Go version in go.mod
   grep "^go " go.mod
   ```

2. **Update go.mod (if needed)**:

   ```go
   go 1.24
   ```

3. **Use New Features**:

   - Replace manual benchmark loops with `testing.B.Loop()`
   - Leverage improved error wrapping
   - Benefit from performance improvements automatically

#### Adopting Go 1.25 Features

1. **Confirm Main Branch**:

   ```bash
   git branch --show-current
   # Must be: main
   ```

2. **Update go.mod**:

   ```go
   go 1.25
   ```

3. **Use New Features**:

   - Use `os.Root()` for filesystem isolation in sandboxed operations
   - Replace traditional `for i := 0; i < n; i++` with `for i := range n {}`
   - Benefit from generic optimizations automatically
   - Leverage enhanced type inference

### Best Practices

#### Version Selection

- **New Projects**: Start with Go 1.24 for stability
- **Experimental Features**: Use Go 1.25 on main branch only
- **Production Code**: Use stable branches with locked versions

#### Feature Flags

Use build tags for version-specific features:

```go
//go:build go1.24

package mypackage

// Go 1.24-specific implementation using testing.B.Loop()
```

```go
//go:build go1.25

package mypackage

// Go 1.25-specific implementation using os.Root()
```

#### Compatibility

- **Write for minimum version**: Target Go 1.23 for maximum compatibility
- **Test across versions**: CI tests all supported versions on main branch
- **Document requirements**: Clearly state minimum Go version in README

### Version-Specific Sections

This document includes detailed sections for version-specific features:

- **testing.B.Loop() (Go 1.24+)**: See benchmarking section below
- **os.Root() (Go 1.25)**: See filesystem isolation section below
- **Integer Range Iteration (Go 1.25)**: See iteration patterns section below
- **Generic Optimization (Go 1.25)**: See generics section below

### Decision: When to Upgrade

#### Upgrade to Go 1.24 When

- ✅ Need improved benchmark testing with `testing.B.Loop()`
- ✅ Want better error wrapping capabilities
- ✅ Require performance improvements
- ✅ Ready to test on stable-1-go-1.24 branch

#### Upgrade to Go 1.25 When

- ✅ Need filesystem isolation with `os.Root()`
- ✅ Want cleaner integer range iteration
- ✅ Benefit from generic optimizations
- ✅ Working on main branch for active development
- ❌ **NOT for stable branches** (wait for policy update)

### Platform Support

All Go versions support the same platforms:

- **Linux**: amd64, arm64
- **macOS**: amd64 (Intel), arm64 (Apple Silicon)
- **NO WINDOWS**: Windows platform not supported

Cross-compilation configured for all targets in `.cargo/config.toml` and release workflows.

## Naming Conventions

- Use short, concise, evocative package names (lowercase, no underscores)
- Use camelCase for unexported names, PascalCase for exported names
- Use short names for short-lived variables, descriptive names for longer-lived
  variables
- Use PascalCase for exported constants, camelCase for unexported constants
- Single-method interfaces should end in "-er" (e.g., Reader, Writer)

## Code Organization

- Use `goimports` to format imports automatically
- Group imports: standard library, third-party, local
- No blank lines within groups, one blank line between groups
- Keep functions short and focused
- Use blank lines to separate logical sections
- Order: receiver, name, parameters, return values

## Formatting

- Use tabs for indentation, spaces for alignment
- Opening brace on same line as declaration, closing brace on its own line
- No strict line length limit, but aim for readability

## Comments

- Every package should have a package comment
- Public functions must have comments starting with the function name
- Comment exported variables, explain purpose and constraints

## Error Handling

- Use lowercase for error messages, no punctuation at end
- Be specific about what failed
- Create custom error types for specific error conditions
- Use `errors.Is` and `errors.As` for error checking

## Best Practices

- Use short variable declarations (`:=`) when possible
- Use `var` for zero values or when type is important
- Use `make()` for slices and maps with known capacity
- Accept interfaces, return concrete types
- Keep interfaces small and focused
- Use channels for communication between goroutines
- Use sync primitives for protecting shared state
- Test file names end with `_test.go`, test function names start with `Test`
- Use table-driven tests for multiple scenarios

## Required File Header

All Go files must begin with a standard header as described in the
[general coding instructions](general-coding.instructions.md). Example for Go:

```go
// file: path/to/file.go
// version: 1.0.0
// guid: 123e4567-e89b-12d3-a456-426614174000
```

## Google Go Style Guide (Complete)

Follow the complete Google Go Style Guide below for all Go code:

### Google Go Style Guide (Complete)

This style guide provides comprehensive conventions for writing clean, readable, and maintainable Go code.

#### Formatting

**gofmt:** All Go code must be formatted with `gofmt`. This is non-negotiable.

**Line Length:** No hard limit, but prefer shorter lines. Break long lines sensibly.

**Indentation:** Use tabs for indentation (handled automatically by gofmt).

**Spacing:** Let gofmt handle spacing. Generally:

- No space inside parentheses: `f(a, b)`
- Space around binary operators: `a + b`
- No space around unary operators: `!condition`

#### Naming Conventions

**Packages:**

- Short, concise, evocative names
- Lowercase, no underscores or mixedCaps
- Often single words

```go
// Good
package user
package httputil
package json

// Bad
package userService
package http_util
```

**Interfaces:**

- Use -er suffix for single-method interfaces
- Use MixedCaps

```go
// Good
type Reader interface {
    Read([]byte) (int, error)
}

type FileWriter interface {
    WriteFile(string, []byte) error
}

// Bad
type IReader interface {  // Don't prefix with I
    Read([]byte) (int, error)
}
```

**Functions and Methods:**

- Use MixedCaps
- Exported functions start with capital letter
- Unexported functions start with lowercase letter

```go
// Good - exported
func CalculateTotal(price, tax float64) float64 {
    return price + tax
}

// Good - unexported
func validateInput(input string) bool {
    return len(input) > 0
}
```

**Variables:**

- Use MixedCaps
- Short names for short scopes
- Longer descriptive names for longer scopes

```go
// Good - short scope
for i, v := range items {
    process(i, v)
}

// Good - longer scope
func processUserData(userData map[string]interface{}) error {
    userID, exists := userData["id"]
    if !exists {
        return errors.New("user ID not found")
    }
    // ... more processing
}

// Bad
func processUserData(d map[string]interface{}) error {  // 'd' too short for scope
    userIdentificationNumber, exists := d["id"]  // Too long for simple value
    // ...
}
```

**Constants:**

- Use MixedCaps
- Group related constants in blocks

```go
// Good
const (
    StatusOK       = 200
    StatusNotFound = 404
    StatusError    = 500
)

const DefaultTimeout = 30 * time.Second

// Bad
const STATUS_OK = 200  // Don't use underscores
```

#### Package Organization

**Package Names:**

- Choose package names that are both short and clear
- Avoid generic names like "util", "common", "misc"
- Package name should describe what it provides, not what it contains

```go
// Good
package user     // for user management
package auth     // for authentication
package httputil // for HTTP utilities

// Bad
package utils    // Too generic
package stuff    // Too vague
```

**Import Organization:**

- Group imports: standard library, third-party, local
- Use goimports to handle this automatically

```go
import (
    // Standard library
    "fmt"
    "os"
    "time"

    // Third-party
    "github.com/gorilla/mux"
    "google.golang.org/grpc"

    // Local
    "myproject/internal/auth"
    "myproject/pkg/utils"
)
```

#### Error Handling

**Error Strings:**

- Don't capitalize error messages
- Don't end with punctuation
- Be descriptive but concise

```go
// Good
return fmt.Errorf("failed to connect to database: %w", err)
return errors.New("invalid user ID")

// Bad
return errors.New("Failed to connect to database.")  // Capitalized, punctuation
return errors.New("error")  // Too vague
```

**Error Wrapping:**

- Use fmt.Errorf with %w verb to wrap errors
- Add context to errors as they bubble up

```go
func processUser(id string) error {
    user, err := getUserFromDB(id)
    if err != nil {
        return fmt.Errorf("failed to get user %s: %w", id, err)
    }

    if err := validateUser(user); err != nil {
        return fmt.Errorf("user validation failed: %w", err)
    }

    return nil
}
```

**Error Checking:**

- Check errors immediately after operations
- Don't ignore errors (use _ only when truly appropriate)

```go
// Good
file, err := os.Open(filename)
if err != nil {
    return fmt.Errorf("failed to open file: %w", err)
}
defer file.Close()

// Bad
file, _ := os.Open(filename)  // Ignoring error
// ... later in code ...
if file == nil {  // Too late to handle properly
    return errors.New("file is nil")
}
```

#### Function Design

**Function Length:** Keep functions short and focused. If a function is very long, consider breaking it up.

**Function Signature:**

- Related parameters should be grouped
- Use meaningful parameter names

```go
// Good
func CreateUser(firstName, lastName, email string, age int) *User {
    return &User{
        FirstName: firstName,
        LastName:  lastName,
        Email:     email,
        Age:       age,
    }
}

// Bad
func CreateUser(a, b, c string, d int) *User {  // Unclear parameter names
    return &User{
        FirstName: a,
        LastName:  b,
        Email:     c,
        Age:       d,
    }
}
```

**Return Values:**

- Return errors as the last value
- Use named return parameters sparingly

```go
// Good
func divide(a, b float64) (float64, error) {
    if b == 0 {
        return 0, errors.New("division by zero")
    }
    return a / b, nil
}

// Acceptable for short, clear functions
func split(path string) (dir, file string) {
    // ... implementation
    return
}
```

#### Struct Design

**Field Organization:**

- Group related fields together
- Consider field alignment for memory efficiency

```go
type User struct {
    // Identity fields
    ID       int64
    Username string
    Email    string

    // Personal information
    FirstName string
    LastName  string
    Age       int

    // Metadata
    CreatedAt time.Time
    UpdatedAt time.Time
    Active    bool
}
```

**Constructor Functions:**

- Use New prefix for constructor functions
- Return pointers for structs that will be modified

```go
func NewUser(username, email string) *User {
    return &User{
        Username:  username,
        Email:     email,
        CreatedAt: time.Now(),
        Active:    true,
    }
}
```

#### Concurrency

**Goroutines:**

- Use goroutines for independent tasks
- Always consider how goroutines will exit

```go
// Good
func processItems(items []Item) {
    var wg sync.WaitGroup

    for _, item := range items {
        wg.Add(1)
        go func(item Item) {
            defer wg.Done()
            process(item)
        }(item)
    }

    wg.Wait()
}
```

**Channels:**

- Use channels for communication between goroutines
- Close channels when done sending

```go
func producer(ch chan<- int) {
    defer close(ch)
    for i := 0; i < 10; i++ {
        ch <- i
    }
}

func consumer(ch <-chan int) {
    for value := range ch {
        fmt.Println(value)
    }
}
```

#### Comments and Documentation

**Package Comments:**

- Every package should have a package comment
- Use complete sentences

```go
// Package user provides functionality for user management,
// including authentication, authorization, and user data operations.
package user
```

**Function Comments:**

- Document all exported functions
- Start with the function name
- Explain what the function does, not how

```go
// CalculateTotal computes the total price including tax.
// It returns an error if the tax rate is negative.
func CalculateTotal(price, taxRate float64) (float64, error) {
    if taxRate < 0 {
        return 0, errors.New("tax rate cannot be negative")
    }
    return price * (1 + taxRate), nil
}
```

**Inline Comments:**

- Use for complex logic or non-obvious code
- Explain why, not what

```go
// Sort items by priority to ensure high-priority items are processed first
sort.Slice(items, func(i, j int) bool {
    return items[i].Priority > items[j].Priority
})
```

#### Testing

**Test Functions:**

- Use TestXxx naming convention
- Use t.Run for subtests

```go
func TestCalculateTotal(t *testing.T) {
    tests := []struct {
        name     string
        price    float64
        taxRate  float64
        expected float64
        hasError bool
    }{
        {
            name:     "positive values",
            price:    100.0,
            taxRate:  0.1,
            expected: 110.0,
            hasError: false,
        },
        {
            name:     "negative tax rate",
            price:    100.0,
            taxRate:  -0.1,
            expected: 0.0,
            hasError: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := CalculateTotal(tt.price, tt.taxRate)

            if tt.hasError {
                if err == nil {
                    t.Errorf("expected error, got none")
                }
                return
            }

            if err != nil {
                t.Errorf("unexpected error: %v", err)
                return
            }

            if result != tt.expected {
                t.Errorf("expected %f, got %f", tt.expected, result)
            }
        })
    }
}
```

**Benchmark Functions:**

```go
func BenchmarkCalculateTotal(b *testing.B) {
    for i := 0; i < b.N; i++ {
        CalculateTotal(100.0, 0.1)
    }
}
```

## Benchmarking with testing.B.Loop() (Go 1.24+)

**Available**: Go 1.24 and later

Go 1.24 introduces `testing.B.Loop()`, a new method for writing benchmark loops that replaces the traditional `for i := 0; i < b.N; i++` pattern. This new approach provides better compiler optimizations and cleaner benchmark code.

### Basic Usage

#### Traditional Approach (Go 1.23 and earlier)

```go
func BenchmarkProcessData(b *testing.B) {
    data := generateTestData()
    b.ResetTimer() // Reset timer after setup

    for i := 0; i < b.N; i++ {
        processData(data)
    }
}
```

#### Modern Approach (Go 1.24+)

```go
func BenchmarkProcessData(b *testing.B) {
    data := generateTestData()
    b.ResetTimer() // Reset timer after setup

    for b.Loop() {
        processData(data)
    }
}
```

### Key Benefits

1. **Cleaner Syntax**: Eliminates the need for explicit loop variable
2. **Compiler Optimizations**: Better optimization opportunities for the compiler
3. **Consistent Pattern**: Matches the style of `for range` loops
4. **No Manual Counter**: Removes potential for off-by-one errors

### When to Use testing.B.Loop()

✅ **Use testing.B.Loop() when:**

- Writing new benchmarks in Go 1.24+ codebases
- Refactoring existing benchmarks for modernization
- Working on main branch or stable-1-go-1.24 branches
- Benchmark doesn't need the iteration index

❌ **Use traditional loop when:**

- Maintaining Go 1.23 compatibility
- Need access to iteration index (rare in benchmarks)
- Working on stable-1-go-1.23 or older branches

### Complete Examples

#### Simple Function Benchmark

```go
// Traditional (Go 1.23)
func BenchmarkStringConcat(b *testing.B) {
    for i := 0; i < b.N; i++ {
        _ = "hello" + "world"
    }
}

// Modern (Go 1.24+)
func BenchmarkStringConcat(b *testing.B) {
    for b.Loop() {
        _ = "hello" + "world"
    }
}
```

#### Benchmark with Setup

```go
func BenchmarkDatabaseQuery(b *testing.B) {
    // Setup (not measured)
    db := setupTestDatabase()
    defer db.Close()

    query := "SELECT * FROM users WHERE active = ?"
    b.ResetTimer() // Start timing here

    for b.Loop() {
        rows, err := db.Query(query, true)
        if err != nil {
            b.Fatal(err)
        }
        rows.Close()
    }
}
```

#### Benchmark with Sub-Benchmarks

```go
func BenchmarkJSONOperations(b *testing.B) {
    data := map[string]interface{}{
        "name": "John Doe",
        "age":  30,
        "active": true,
    }

    b.Run("Marshal", func(b *testing.B) {
        for b.Loop() {
            _, err := json.Marshal(data)
            if err != nil {
                b.Fatal(err)
            }
        }
    })

    b.Run("Unmarshal", func(b *testing.B) {
        jsonData, _ := json.Marshal(data)
        var result map[string]interface{}

        for b.Loop() {
            err := json.Unmarshal(jsonData, &result)
            if err != nil {
                b.Fatal(err)
            }
        }
    })
}
```

#### Parallel Benchmarks

```go
func BenchmarkConcurrentProcessor(b *testing.B) {
    processor := NewProcessor()

    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() { // Note: RunParallel uses pb.Next(), not b.Loop()
            processor.Process(generateRandomData())
        }
    })
}
```

**Important**: When using `b.RunParallel()`, continue using `pb.Next()` in the parallel function. The `testing.B.Loop()` method is for sequential benchmarks only.

### Migration Strategy

#### Gradual Migration (Recommended)

1. **Update go.mod** to Go 1.24:

   ```go
   go 1.24
   ```

2. **Use build tags** for version-specific benchmarks:

   ```go
   //go:build go1.24

   package mypackage

   import "testing"

   func BenchmarkModern(b *testing.B) {
       for b.Loop() {
           // benchmark code
       }
   }
   ```

3. **Keep compatibility versions** for older Go:

   ```go
   //go:build !go1.24

   package mypackage

   import "testing"

   func BenchmarkModern(b *testing.B) {
       for i := 0; i < b.N; i++ {
           // same benchmark code
       }
   }
   ```

#### Complete Migration (Go 1.24+ Only)

Replace all traditional benchmark loops:

```bash
# Find all traditional benchmark loops
grep -r "for i := 0; i < b.N; i++" . --include="*_test.go"

# Replace with testing.B.Loop()
# (Do this carefully, reviewing each change)
```

### Best Practices

#### DO: Use for Simple Iterations

```go
func BenchmarkSimpleOperation(b *testing.B) {
    for b.Loop() {
        result := expensiveOperation()
        _ = result // Prevent compiler optimization
    }
}
```

#### DO: Reset Timer After Setup

```go
func BenchmarkWithSetup(b *testing.B) {
    data := setupTestData() // Not measured
    b.ResetTimer()          // Start measuring here

    for b.Loop() {
        process(data)
    }
}
```

#### DO: Use Sub-Benchmarks for Comparisons

```go
func BenchmarkStringBuilding(b *testing.B) {
    b.Run("Concat", func(b *testing.B) {
        for b.Loop() {
            _ = "a" + "b" + "c"
        }
    })

    b.Run("Builder", func(b *testing.B) {
        for b.Loop() {
            var builder strings.Builder
            builder.WriteString("a")
            builder.WriteString("b")
            builder.WriteString("c")
            _ = builder.String()
        }
    })
}
```

#### DON'T: Access Non-Existent Loop Variable

```go
// Bad - no loop variable with testing.B.Loop()
func BenchmarkBad(b *testing.B) {
    for b.Loop() {
        // Can't use i here - it doesn't exist
        fmt.Println(i) // Compile error
    }
}

// Good - use traditional loop if you need the index
func BenchmarkWithIndex(b *testing.B) {
    for i := 0; i < b.N; i++ {
        if i%100 == 0 {
            // Do something every 100 iterations
        }
    }
}
```

#### DON'T: Mix with RunParallel

```go
// Wrong - use pb.Next() with RunParallel
func BenchmarkParallel(b *testing.B) {
    b.RunParallel(func(pb *testing.PB) {
        for b.Loop() { // Wrong! Use pb.Next()
            process()
        }
    })
}

// Correct
func BenchmarkParallel(b *testing.B) {
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() { // Correct
            process()
        }
    })
}
```

### Performance Characteristics

The `testing.B.Loop()` method provides the same performance as traditional loops while offering:

- **Better Compiler Optimization**: Simpler loop structure allows for better optimizations
- **Reduced Overhead**: Eliminates unnecessary loop variable management
- **Consistent Behavior**: Same iteration count as `for i := 0; i < b.N; i++`

### Branch-Specific Guidelines

#### Main Branch

- ✅ Use `testing.B.Loop()` for all new benchmarks
- ✅ Refactor existing benchmarks when convenient
- ✅ Both styles acceptable during transition

#### stable-1-go-1.24

- ✅ Use `testing.B.Loop()` for all new benchmarks
- ✅ Migration encouraged but not required

#### stable-1-go-1.23

- ❌ Cannot use `testing.B.Loop()` (not available)
- ✅ Continue using traditional loop syntax

### Compatibility Check

Ensure your code targets the correct Go version:

```go
// In go.mod
go 1.24  // Required for testing.B.Loop()
```

```go
// In benchmark file (optional build tag)
//go:build go1.24

package mypackage

import "testing"

func BenchmarkModern(b *testing.B) {
    for b.Loop() {
        // Modern benchmark code
    }
}
```

## Filesystem Isolation with os.Root() (Go 1.25+)

> **Availability**: Go 1.25+ only. Not available on Go 1.23 or 1.24.

### Overview

The `os.Root()` API introduced in Go 1.25 provides secure filesystem isolation by creating a restricted view of the filesystem rooted at a specific directory. This is critical for sandboxing untrusted code, implementing secure file operations, and preventing path traversal attacks.

### Why Use os.Root()?

- **Security Sandboxing**: Prevent access to files outside a designated directory tree
- **Path Traversal Protection**: Automatically blocks `..` and symlink escapes
- **Multi-Tenant Isolation**: Safely isolate operations for different users/tenants
- **Plugin Security**: Sandbox plugin file operations to specific directories
- **Testing Isolation**: Create isolated filesystem views for tests

### Basic Usage

```go
package main

import (
    "fmt"
    "os"
)

func main() {
    // Create a filesystem root at /safe/directory
    root, err := os.Root("/safe/directory")
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to create root: %v\n", err)
        return
    }
    defer root.Close()

    // All file operations through root are isolated to /safe/directory
    file, err := root.Open("data.txt") // Actually opens /safe/directory/data.txt
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to open file: %v\n", err)
        return
    }
    defer file.Close()

    // Attempts to escape are blocked
    _, err = root.Open("../../../etc/passwd") // Returns error, cannot escape
    if err != nil {
        fmt.Println("Path traversal blocked (expected)")
    }
}
```

### Complete Example: Sandboxed File Server

```go
package main

import (
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
)

// SecureFileServer serves files only from a sandboxed directory
type SecureFileServer struct {
    root *os.Root
}

func NewSecureFileServer(baseDir string) (*SecureFileServer, error) {
    // Create isolated filesystem view
    root, err := os.Root(baseDir)
    if err != nil {
        return nil, fmt.Errorf("failed to create root: %w", err)
    }

    return &SecureFileServer{root: root}, nil
}

func (s *SecureFileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Clean the path but root isolation prevents escapes anyway
    cleanPath := filepath.Clean(r.URL.Path)

    // Open file through isolated root
    file, err := s.root.Open(cleanPath)
    if err != nil {
        http.Error(w, "File not found", http.StatusNotFound)
        return
    }
    defer file.Close()

    // Get file info for content type detection
    info, err := file.Stat()
    if err != nil {
        http.Error(w, "Internal error", http.StatusInternalServerError)
        return
    }

    // Serve the file
    http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}

func (s *SecureFileServer) Close() error {
    return s.root.Close()
}

func main() {
    server, err := NewSecureFileServer("/var/www/public")
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to create server: %v\n", err)
        os.Exit(1)
    }
    defer server.Close()

    http.Handle("/files/", server)
    http.ListenAndServe(":8080", nil)
}
```

### Example: Secure Plugin Execution

```go
package main

import (
    "fmt"
    "os"
    "path/filepath"
)

type PluginSandbox struct {
    root      *os.Root
    pluginID  string
}

func NewPluginSandbox(pluginID string) (*PluginSandbox, error) {
    // Create isolated directory for this plugin
    sandboxDir := filepath.Join("/var/plugins", pluginID)
    if err := os.MkdirAll(sandboxDir, 0755); err != nil {
        return nil, fmt.Errorf("failed to create sandbox: %w", err)
    }

    // Create filesystem root for isolation
    root, err := os.Root(sandboxDir)
    if err != nil {
        return nil, fmt.Errorf("failed to create root: %w", err)
    }

    return &PluginSandbox{
        root:     root,
        pluginID: pluginID,
    }, nil
}

// ReadConfig safely reads plugin configuration
func (s *PluginSandbox) ReadConfig(filename string) ([]byte, error) {
    file, err := s.root.Open(filename)
    if err != nil {
        return nil, fmt.Errorf("config not found: %w", err)
    }
    defer file.Close()

    return io.ReadAll(file)
}

// WriteOutput safely writes plugin output
func (s *PluginSandbox) WriteOutput(filename string, data []byte) error {
    file, err := s.root.Create(filename)
    if err != nil {
        return fmt.Errorf("failed to create output: %w", err)
    }
    defer file.Close()

    _, err = file.Write(data)
    return err
}

// ListFiles lists files in the sandbox
func (s *PluginSandbox) ListFiles() ([]string, error) {
    dir, err := s.root.Open(".")
    if err != nil {
        return nil, err
    }
    defer dir.Close()

    entries, err := dir.Readdir(-1)
    if err != nil {
        return nil, err
    }

    var files []string
    for _, entry := range entries {
        files = append(files, entry.Name())
    }
    return files, nil
}

func (s *PluginSandbox) Close() error {
    return s.root.Close()
}
```

### Example: Secure Testing Isolation

```go
package mypackage_test

import (
    "os"
    "path/filepath"
    "testing"
)

func TestWithIsolatedFS(t *testing.T) {
    // Create temporary directory for test
    tempDir := t.TempDir()

    // Create isolated filesystem view
    root, err := os.Root(tempDir)
    if err != nil {
        t.Fatalf("Failed to create root: %v", err)
    }
    defer root.Close()

    // Create test files in isolated environment
    testData := []byte("test content")
    if err := root.WriteFile("test.txt", testData, 0644); err != nil {
        t.Fatalf("Failed to write test file: %v", err)
    }

    // Test file operations in isolation
    data, err := root.ReadFile("test.txt")
    if err != nil {
        t.Fatalf("Failed to read file: %v", err)
    }

    if string(data) != string(testData) {
        t.Errorf("Data mismatch: got %q, want %q", data, testData)
    }

    // Verify path traversal is blocked
    _, err = root.ReadFile("../../../../../etc/passwd")
    if err == nil {
        t.Error("Path traversal should have been blocked")
    }
}
```

### Security Considerations

#### ✅ DO: Use for Untrusted Code

```go
// Good - isolate untrusted plugin operations
func executePlugin(pluginPath string) error {
    root, err := os.Root(filepath.Dir(pluginPath))
    if err != nil {
        return err
    }
    defer root.Close()

    // Plugin can only access files within its directory
    return runPluginInSandbox(root, filepath.Base(pluginPath))
}
```

#### ✅ DO: Validate Before Creating Root

```go
// Good - validate directory exists and is safe
func createSafeRoot(path string) (*os.Root, error) {
    // Ensure path exists
    info, err := os.Stat(path)
    if err != nil {
        return nil, fmt.Errorf("invalid path: %w", err)
    }

    // Ensure it's a directory
    if !info.IsDir() {
        return nil, fmt.Errorf("path is not a directory: %s", path)
    }

    // Create isolated root
    return os.Root(path)
}
```

#### ✅ DO: Always Close Roots

```go
// Good - clean up root when done
root, err := os.Root("/sandbox")
if err != nil {
    return err
}
defer root.Close() // Always clean up

// Use root for operations
```

#### ❌ DON'T: Trust User-Provided Paths Without Validation

```go
// Bad - no validation of user input
func openUserFile(userPath string) error {
    root, _ := os.Root(userPath) // Could be anything!
    defer root.Close()
    // ...
}

// Good - validate user input first
func openUserFile(userPath string) error {
    // Validate path is within allowed directory
    allowedBase := "/var/user-data"
    cleanPath := filepath.Clean(userPath)
    if !filepath.HasPrefix(cleanPath, allowedBase) {
        return fmt.Errorf("path outside allowed directory")
    }

    root, err := os.Root(cleanPath)
    if err != nil {
        return err
    }
    defer root.Close()
    // ...
}
```

#### ❌ DON'T: Assume Root Prevents All Attacks

```go
// Bad - root isolation doesn't prevent logic errors
func processFile(filename string) error {
    root, _ := os.Root("/data")
    defer root.Close()

    // Still vulnerable to logic errors:
    // - SQL injection if filename used in queries
    // - Command injection if filename passed to exec
    // - Resource exhaustion if file is huge

    file, _ := root.Open(filename)
    // ... process file without size limits (bad!)
}

// Good - combine root isolation with other security measures
func processFile(filename string) error {
    root, err := os.Root("/data")
    if err != nil {
        return err
    }
    defer root.Close()

    // Validate filename
    if err := validateFilename(filename); err != nil {
        return err
    }

    file, err := root.Open(filename)
    if err != nil {
        return err
    }
    defer file.Close()

    // Check file size before processing
    info, err := file.Stat()
    if err != nil {
        return err
    }
    if info.Size() > maxFileSize {
        return fmt.Errorf("file too large: %d bytes", info.Size())
    }

    // Process with limits
    return processWithLimits(file, info.Size())
}
```

### Performance Characteristics

The `os.Root()` API provides:

- **Minimal Overhead**: Root creation is lightweight (similar to opening a directory)
- **No Path Rewriting**: Operations are enforced at the kernel level where possible
- **Efficient Checking**: Path validation happens once at root creation
- **Scalable**: Multiple roots can coexist for different isolation contexts

### Branch-Specific Guidelines

#### Main Branch (Go 1.25+)

- ✅ Use `os.Root()` for all sandboxing operations
- ✅ Refactor existing sandboxing code to use `os.Root()`
- ✅ Document security benefits in code comments

#### stable-1-go-1.24 and stable-1-go-1.23

- ❌ Cannot use `os.Root()` (not available)
- ✅ Use alternative approaches:
  - Manual path validation with `filepath.Clean()` and prefix checking
  - chroot (Unix-specific, requires privileges)
  - Container-level isolation
  - Third-party libraries like `securejoin`

### Compatibility Check

```go
// In go.mod
go 1.25  // Required for os.Root()
```

```go
// With build tags for version-specific code
//go:build go1.25

package secure

import "os"

func CreateSandbox(dir string) (*os.Root, error) {
    return os.Root(dir)
}
```

```go
// Fallback for older versions
//go:build !go1.25

package secure

import (
    "fmt"
    "os"
)

type Root struct {
    basePath string
}

func (r *Root) Close() error {
    return nil
}

func CreateSandbox(dir string) (*Root, error) {
    // Fallback implementation using manual validation
    return &Root{basePath: dir}, nil
}
```

### Migration from Manual Path Validation

**Before (Go 1.24 and earlier):**

```go
func openSandboxedFile(baseDir, filename string) (*os.File, error) {
    // Manual validation required
    cleanPath := filepath.Clean(filename)
    if filepath.IsAbs(cleanPath) {
        return nil, fmt.Errorf("absolute paths not allowed")
    }

    fullPath := filepath.Join(baseDir, cleanPath)
    if !strings.HasPrefix(fullPath, baseDir) {
        return nil, fmt.Errorf("path escapes base directory")
    }

    return os.Open(fullPath)
}
```

**After (Go 1.25+):**

```go
func openSandboxedFile(baseDir, filename string) (*os.File, error) {
    root, err := os.Root(baseDir)
    if err != nil {
        return nil, err
    }
    defer root.Close()

    // Root automatically prevents escapes
    return root.Open(filename)
}
```

### Integration with Existing Security Practices

#### Combine with Capabilities

```go
import (
    "os"
    "syscall"
)

// Drop privileges and create sandbox
func createSecureSandbox(dir string) (*os.Root, error) {
    // Drop unnecessary capabilities (Unix)
    if err := dropCapabilities(); err != nil {
        return nil, err
    }

    // Create filesystem isolation
    return os.Root(dir)
}

func dropCapabilities() error {
    // Implementation depends on platform
    // Use syscall or x/sys/unix for capability management
    return nil
}
```

#### Combine with Resource Limits

```go
import (
    "os"
    "syscall"
)

// Create sandbox with resource limits
type SandboxConfig struct {
    BaseDir     string
    MaxFileSize int64
    MaxFiles    int
}

func CreateLimitedSandbox(cfg SandboxConfig) (*os.Root, error) {
    // Set resource limits
    var rlimit syscall.Rlimit
    rlimit.Cur = uint64(cfg.MaxFiles)
    rlimit.Max = uint64(cfg.MaxFiles)
    if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
        return nil, fmt.Errorf("failed to set file limit: %w", err)
    }

    // Create filesystem isolation
    return os.Root(cfg.BaseDir)
}
```

## Generic Optimization and Type Parameters (Go 1.25+)

> **Availability**: Go 1.25+ includes significant generic optimizations. Type parameters available since Go 1.18, but performance improved in 1.25.

### Overview

Go 1.25 brings major improvements to generics performance through:

- **Core Types Removal**: Simplified type parameter constraints (breaking change from Go 1.18-1.24)
- **Better Type Inference**: Reduced need for explicit type arguments
- **Performance Optimizations**: Dictionary-based implementation improvements
- **Reduced Code Bloat**: Better monomorphization strategies

### What Changed in Go 1.25

#### Core Types Constraint Simplification

**Before (Go 1.18-1.24):**

```go
// Old syntax with core types
type Integer interface {
    ~int | ~int8 | ~int16 | ~int32 | ~int64
}

type Signed interface {
    ~int | ~int8 | ~int16 | ~int32 | ~int64
}

// Complex approximation constraints
type Stringer interface {
    ~string
}
```

**After (Go 1.25+):**

```go
// Simplified constraints - no tilde operator needed for basic cases
type Integer interface {
    int | int8 | int16 | int32 | int64
}

type Signed interface {
    int | int8 | int16 | int32 | int64
}

// More natural constraint syntax
type Stringer interface {
    string
}

// Tilde still available for underlying type matching when needed
type CustomInt interface {
    ~int  // Still works: matches int and types with underlying type int
}
```

### Generic Function Optimization

#### Example: Generic Slice Utilities

**Optimized Generic Functions (Go 1.25):**

```go
package sliceutil

// Map applies a function to each element
// Go 1.25: Better type inference, reduced allocations
func Map[T, U any](slice []T, fn func(T) U) []U {
    result := make([]U, len(slice))
    for i, v := range slice {
        result[i] = fn(v)
    }
    return result
}

// Filter returns elements matching predicate
// Go 1.25: Optimized for common types (int, string, etc.)
func Filter[T any](slice []T, predicate func(T) bool) []T {
    result := make([]T, 0, len(slice))
    for _, v := range slice {
        if predicate(v) {
            result = append(result, v)
        }
    }
    return result
}

// Reduce combines elements using a function
func Reduce[T, U any](slice []T, initial U, fn func(U, T) U) U {
    result := initial
    for _, v := range slice {
        result = fn(result, v)
    }
    return result
}

// Contains checks if slice contains element
// Go 1.25: Compiler can optimize for comparable types
func Contains[T comparable](slice []T, value T) bool {
    for _, v := range slice {
        if v == value {
            return true
        }
    }
    return false
}
```

**Usage Examples:**

```go
package main

import "fmt"

func main() {
    numbers := []int{1, 2, 3, 4, 5}

    // Type inference works better in Go 1.25
    doubled := Map(numbers, func(n int) int {
        return n * 2
    })
    fmt.Println(doubled) // [2 4 6 8 10]

    // Even works without explicit types
    evens := Filter(numbers, func(n int) bool {
        return n%2 == 0
    })
    fmt.Println(evens) // [2 4]

    // Type inference across different types
    sum := Reduce(numbers, 0, func(acc, n int) int {
        return acc + n
    })
    fmt.Println(sum) // 15
}
```

### Enhanced Type Inference

**Go 1.25 improvements:**

```go
// Complex generic function
func Transform[T, U, V any](input []T, f1 func(T) U, f2 func(U) V) []V {
    temp := make([]U, len(input))
    for i, v := range input {
        temp[i] = f1(v)
    }

    result := make([]V, len(temp))
    for i, v := range temp {
        result[i] = f2(v)
    }
    return result
}

// Go 1.25: Type inference works without explicit type arguments
numbers := []int{1, 2, 3}
result := Transform(
    numbers,
    func(n int) string { return fmt.Sprint(n) },
    func(s string) bool { return len(s) > 0 },
)
// result is []bool, fully inferred

// Go 1.24 and earlier often required:
// result := Transform[int, string, bool](numbers, ...)
```

### Generic Data Structures

#### Example: Type-Safe Cache

```go
package cache

import (
    "sync"
    "time"
)

// Cache is a generic thread-safe cache with expiration
// Go 1.25: Optimized for common key/value types
type Cache[K comparable, V any] struct {
    mu      sync.RWMutex
    items   map[K]*cacheItem[V]
    ttl     time.Duration
}

type cacheItem[V any] struct {
    value      V
    expiration time.Time
}

func NewCache[K comparable, V any](ttl time.Duration) *Cache[K, V] {
    c := &Cache[K, V]{
        items: make(map[K]*cacheItem[V]),
        ttl:   ttl,
    }
    go c.cleanup()
    return c
}

func (c *Cache[K, V]) Set(key K, value V) {
    c.mu.Lock()
    defer c.mu.Unlock()

    c.items[key] = &cacheItem[V]{
        value:      value,
        expiration: time.Now().Add(c.ttl),
    }
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    item, exists := c.items[key]
    if !exists {
        var zero V
        return zero, false
    }

    if time.Now().After(item.expiration) {
        var zero V
        return zero, false
    }

    return item.value, true
}

func (c *Cache[K, V]) cleanup() {
    ticker := time.NewTicker(c.ttl)
    defer ticker.Stop()

    for range ticker.C {
        c.mu.Lock()
        now := time.Now()
        for key, item := range c.items {
            if now.After(item.expiration) {
                delete(c.items, key)
            }
        }
        c.mu.Unlock()
    }
}
```

**Usage:**

```go
// Type inference makes this clean
userCache := NewCache[string, User](5 * time.Minute)
userCache.Set("user123", User{Name: "Alice", Age: 30})

if user, found := userCache.Get("user123"); found {
    fmt.Printf("Found user: %s\n", user.Name)
}

// Works with any comparable key and any value type
intCache := NewCache[int, []string](1 * time.Hour)
intCache.Set(42, []string{"foo", "bar"})
```

### Performance Best Practices

#### ✅ DO: Use Concrete Types for Hot Paths

```go
// Good - use generics for API flexibility
func ProcessData[T any](data []T, processor func(T) T) []T {
    result := make([]T, len(data))
    for i, v := range data {
        result[i] = processor(v)
    }
    return result
}

// Better - use concrete type for performance-critical inner loop
func ProcessIntegers(data []int, processor func(int) int) []int {
    result := make([]int, len(data))
    for i, v := range data {
        result[i] = processor(v)
    }
    return result
}

// Best - provide both: generic for API, concrete for performance
func Process[T any](data []T, processor func(T) T) []T {
    return ProcessData(data, processor)
}

// Specialized version for common case
func ProcessInt(data []int, processor func(int) int) []int {
    return ProcessIntegers(data, processor)
}
```

#### ✅ DO: Constrain Type Parameters Appropriately

```go
// Good - specific constraint enables optimization
func Sum[T constraints.Integer](values []T) T {
    var sum T
    for _, v := range values {
        sum += v
    }
    return sum
}

// Good - comparable enables map usage
func Unique[T comparable](slice []T) []T {
    seen := make(map[T]bool)
    result := make([]T, 0)

    for _, v := range slice {
        if !seen[v] {
            seen[v] = true
            result = append(result, v)
        }
    }
    return result
}
```

#### ✅ DO: Preallocate Slices with Known Capacity

```go
// Good - preallocate for known size
func Map[T, U any](slice []T, fn func(T) U) []U {
    result := make([]U, len(slice)) // Exact size
    for i, v := range slice {
        result[i] = fn(v)
    }
    return result
}

// Good - preallocate with capacity hint
func Filter[T any](slice []T, predicate func(T) bool) []T {
    result := make([]T, 0, len(slice)) // Capacity hint
    for _, v := range slice {
        if predicate(v) {
            result = append(result, v)
        }
    }
    return result
}
```

#### ❌ DON'T: Over-Genericize Simple Functions

```go
// Bad - unnecessary generics for simple case
func Add[T constraints.Integer](a, b T) T {
    return a + b
}

// Good - just use int or int64
func Add(a, b int) int {
    return a + b
}

// Or if you really need multiple types, use concrete overloads
func AddInt(a, b int) int { return a + b }
func AddInt64(a, b int64) int64 { return a + b }
```

#### ❌ DON'T: Use `any` When More Specific Constraint Exists

```go
// Bad - too permissive
func Max[T any](a, b T) T {
    // Can't compare T without constraint
    // Requires reflection or panic
}

// Good - use appropriate constraint
func Max[T constraints.Ordered](a, b T) T {
    if a > b {
        return a
    }
    return b
}
```

### Common Generic Patterns

#### Option Type

```go
package option

// Option represents an optional value
type Option[T any] struct {
    value T
    valid bool
}

func Some[T any](value T) Option[T] {
    return Option[T]{value: value, valid: true}
}

func None[T any]() Option[T] {
    return Option[T]{valid: false}
}

func (o Option[T]) IsSome() bool {
    return o.valid
}

func (o Option[T]) IsNone() bool {
    return !o.valid
}

func (o Option[T]) Unwrap() T {
    if !o.valid {
        panic("called Unwrap on None")
    }
    return o.value
}

func (o Option[T]) UnwrapOr(defaultValue T) T {
    if !o.valid {
        return defaultValue
    }
    return o.value
}

func (o Option[T]) Map[U any](fn func(T) U) Option[U] {
    if !o.valid {
        return None[U]()
    }
    return Some(fn(o.value))
}
```

**Usage:**

```go
func FindUser(id string) Option[User] {
    user, found := userCache.Get(id)
    if !found {
        return None[User]()
    }
    return Some(user)
}

// Chain operations
result := FindUser("123").
    Map(func(u User) string { return u.Email }).
    UnwrapOr("no-email@example.com")
```

#### Result Type

```go
package result

// Result represents success or failure
type Result[T any] struct {
    value T
    err   error
}

func Ok[T any](value T) Result[T] {
    return Result[T]{value: value}
}

func Err[T any](err error) Result[T] {
    return Result[T]{err: err}
}

func (r Result[T]) IsOk() bool {
    return r.err == nil
}

func (r Result[T]) IsErr() bool {
    return r.err != nil
}

func (r Result[T]) Unwrap() (T, error) {
    return r.value, r.err
}

func (r Result[T]) Map[U any](fn func(T) U) Result[U] {
    if r.err != nil {
        return Err[U](r.err)
    }
    return Ok(fn(r.value))
}

func (r Result[T]) AndThen[U any](fn func(T) Result[U]) Result[U] {
    if r.err != nil {
        return Err[U](r.err)
    }
    return fn(r.value)
}
```

### Migration from Pre-1.25 Generics

**If you have existing generic code from Go 1.18-1.24:**

1. **Review Core Types Usage**: The `~` operator behavior is more refined in Go 1.25
2. **Test Performance**: Many generic operations are faster in 1.25
3. **Simplify Constraints**: Remove unnecessary `~` operators where applicable
4. **Leverage Better Inference**: Remove explicit type arguments where inference now works

**Example Migration:**

```go
// Go 1.24 code
type Number interface {
    ~int | ~int64 | ~float64
}

func Sum[T Number](values []T) T {
    var sum T
    for _, v := range values {
        sum += v
    }
    return sum
}

// Usage required explicit types
result := Sum[int]([]int{1, 2, 3})
```

```go
// Go 1.25 optimized
import "golang.org/x/exp/constraints"

func Sum[T constraints.Integer | constraints.Float](values []T) T {
    var sum T
    for _, v := range values {
        sum += v
    }
    return sum
}

// Better type inference
result := Sum([]int{1, 2, 3}) // Type inferred
```

### Branch-Specific Guidelines

#### Main Branch (Go 1.25+)

- ✅ Use optimized generics freely
- ✅ Leverage improved type inference
- ✅ Benchmark generic vs concrete for hot paths
- ✅ Use standard constraint packages from `golang.org/x/exp/constraints`

#### stable-1-go-1.24 and stable-1-go-1.23

- ✅ Generics available but less optimized
- ⚠️ More explicit type arguments may be required
- ⚠️ Consider concrete types for performance-critical code
- ⚠️ Core types constraints work differently

### Performance Characteristics

**Go 1.25 improvements:**

- **10-30% faster** generic function calls for common types
- **Reduced memory allocations** in generic data structures
- **Better inlining** of generic functions
- **Smaller binary sizes** due to improved monomorphization

**Benchmark before and after:**

```go
// Benchmark generic operations
func BenchmarkGenericMap(b *testing.B) {
    data := make([]int, 1000)
    for i := range data {
        data[i] = i
    }

    for b.Loop() {
        _ = Map(data, func(n int) int { return n * 2 })
    }
}

// Go 1.24: ~500 ns/op
// Go 1.25: ~350 ns/op (30% improvement)
```

### Compatibility Check

```go
// In go.mod
go 1.25  // Required for optimized generics
```

```go
// Use build tags for version-specific optimizations
//go:build go1.25

package mypackage

// Go 1.25-specific optimized generic implementation
```

## Integer Range Iteration (Go 1.25+)

> **Availability**: Go 1.25+ only. Not available on Go 1.23 or 1.24.

### Overview

Go 1.25 introduces a cleaner syntax for iterating over integer ranges using `for i := range n {}` instead of the traditional `for i := 0; i < n; i++` pattern. This simplifies common loop patterns and reduces boilerplate code.

### Basic Syntax

**Traditional Approach (Go 1.23, 1.24):**

```go
// Old way - manual counter initialization and condition
for i := 0; i < 10; i++ {
    fmt.Println(i) // 0 through 9
}
```

**Modern Approach (Go 1.25+):**

```go
// New way - range over integer
for i := range 10 {
    fmt.Println(i) // 0 through 9
}
```

### Key Benefits

- **Less Boilerplate**: No need to write initialization, condition, and increment
- **More Readable**: Intent is clearer - "iterate from 0 to n-1"
- **Consistent Syntax**: Follows existing `range` patterns for slices/maps
- **Less Error-Prone**: Eliminates off-by-one errors in loop conditions

### Complete Examples

#### Example 1: Simple Iteration

```go
package main

import "fmt"

func main() {
    // Print numbers 0-4
    for i := range 5 {
        fmt.Println(i)
    }
    // Output: 0, 1, 2, 3, 4
}
```

#### Example 2: Creating Slices

```go
// Traditional
func makeSlice(n int) []int {
    result := make([]int, n)
    for i := 0; i < n; i++ {
        result[i] = i * 2
    }
    return result
}

// Go 1.25
func makeSlice(n int) []int {
    result := make([]int, n)
    for i := range n {
        result[i] = i * 2
    }
    return result
}
```

#### Example 3: Initializing Arrays

```go
package main

import "fmt"

func main() {
    var grid [5][5]int

    // Fill grid with coordinates
    for i := range 5 {
        for j := range 5 {
            grid[i][j] = i*10 + j
        }
    }

    fmt.Println(grid)
}
```

#### Example 4: Batch Processing

```go
package batch

import "fmt"

// ProcessInBatches processes items in batches
func ProcessInBatches(items []string, batchSize int) {
    numBatches := (len(items) + batchSize - 1) / batchSize

    for i := range numBatches {
        start := i * batchSize
        end := start + batchSize
        if end > len(items) {
            end = len(items)
        }

        batch := items[start:end]
        fmt.Printf("Processing batch %d: %v\n", i, batch)
        processBatch(batch)
    }
}

func processBatch(batch []string) {
    // Process the batch
    for _, item := range batch {
        fmt.Printf("  - %s\n", item)
    }
}
```

#### Example 5: Generating Test Data

```go
package generator

import "fmt"

type User struct {
    ID   int
    Name string
}

// GenerateUsers creates n test users
func GenerateUsers(n int) []User {
    users := make([]User, n)
    for i := range n {
        users[i] = User{
            ID:   i + 1,
            Name: fmt.Sprintf("User%d", i+1),
        }
    }
    return users
}
```

#### Example 6: Retry Logic

```go
package retry

import (
    "fmt"
    "time"
)

// RetryOperation attempts operation up to maxRetries times
func RetryOperation(maxRetries int, operation func() error) error {
    for attempt := range maxRetries {
        err := operation()
        if err == nil {
            return nil
        }

        if attempt < maxRetries-1 {
            backoff := time.Duration(attempt+1) * time.Second
            fmt.Printf("Attempt %d failed: %v. Retrying in %v...\n",
                attempt+1, err, backoff)
            time.Sleep(backoff)
        }
    }

    return fmt.Errorf("operation failed after %d attempts", maxRetries)
}
```

### When to Use

#### ✅ Use Integer Range When

- **Simple counter loops**: Iterating a fixed number of times
- **Zero-based sequences**: When you need 0, 1, 2, ..., n-1
- **Array/slice initialization**: Filling arrays with computed values
- **Batch processing**: Dividing work into n chunks
- **Retry logic**: Fixed number of retry attempts
- **Test data generation**: Creating n test objects

#### ❌ Use Traditional Loop When

- **Non-zero start**: Loops that don't start at 0
- **Custom increment**: Loops with step != 1 (e.g., `i += 2`)
- **Complex conditions**: Conditions beyond simple `i < n`
- **Counting backwards**: Descending loops
- **Need loop variable modification**: Changing `i` inside loop body

### Comparison Examples

#### Simple Counting

```go
// Traditional - more verbose
for i := 0; i < 10; i++ {
    fmt.Println(i)
}

// Go 1.25 - cleaner
for i := range 10 {
    fmt.Println(i)
}
```

#### Custom Start (Still Use Traditional)

```go
// Traditional - necessary when not starting at 0
for i := 5; i < 10; i++ {
    fmt.Println(i)
}

// Can't use range for this - it always starts at 0
```

#### Custom Step (Still Use Traditional)

```go
// Traditional - necessary for step != 1
for i := 0; i < 10; i += 2 {
    fmt.Println(i) // 0, 2, 4, 6, 8
}

// Can't use range for this
```

#### Countdown (Still Use Traditional)

```go
// Traditional - necessary for descending
for i := 10; i > 0; i-- {
    fmt.Println(i)
}

// Can't use range for descending loops
```

### Best Practices

#### ✅ DO: Use for Simple 0-to-n Loops

```go
// Good - clear and concise
for i := range count {
    process(i)
}
```

#### ✅ DO: Use with Variables

```go
// Good - works with any integer expression
n := calculateSize()
for i := range n {
    items[i] = createItem(i)
}
```

#### ✅ DO: Use for Nested Loops

```go
// Good - clear coordinate iteration
for i := range height {
    for j := range width {
        grid[i][j] = compute(i, j)
    }
}
```

#### ❌ DON'T: Use When Traditional is Clearer

```go
// Bad - adding offset makes it less clear
for i := range 10 {
    process(i + 5) // Unclear start point
}

// Good - explicit start value
for i := 5; i < 15; i++ {
    process(i)
}
```

#### ❌ DON'T: Use with Magic Numbers

```go
// Bad - unclear what 100 means
for i := range 100 {
    process(i)
}

// Good - use named constant
const maxItems = 100
for i := range maxItems {
    process(i)
}
```

### Performance Characteristics

The integer range syntax has identical performance to traditional loops:

- **Zero Overhead**: Compiles to the same machine code
- **Same Optimization**: Compiler optimizations apply equally
- **No Allocations**: No heap allocations for the counter

```go
// Both have identical performance
func benchTraditional(n int) int {
    sum := 0
    for i := 0; i < n; i++ {
        sum += i
    }
    return sum
}

func benchRange(n int) int {
    sum := 0
    for i := range n {
        sum += i
    }
    return sum
}

// Benchmark results are identical
```

### Migration Strategy

#### Gradual Migration

Use build tags to support both versions during transition:

```go
//go:build go1.25

package mypackage

func initialize(items []int) {
    for i := range len(items) {
        items[i] = i
    }
}
```

```go
//go:build !go1.25

package mypackage

func initialize(items []int) {
    for i := 0; i < len(items); i++ {
        items[i] = i
    }
}
```

#### Complete Migration

When targeting Go 1.25+ exclusively:

1. **Find candidates**: Search for simple `for i := 0; i < n; i++` patterns
2. **Verify simplicity**: Ensure no custom start/step/condition
3. **Replace systematically**: Convert to `for i := range n`
4. **Test thoroughly**: Verify behavior unchanged

```bash
# Find potential candidates (review each manually)
grep -rn "for [a-z] := 0; [a-z] <" . --include="*.go"
```

### Common Patterns

#### Pattern 1: Array Initialization

```go
// Initialize array with index values
var data [100]int
for i := range len(data) {
    data[i] = i
}
```

#### Pattern 2: Parallel Processing

```go
package parallel

import "sync"

func ProcessParallel(n int, worker func(int)) {
    var wg sync.WaitGroup

    for i := range n {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            worker(id)
        }(i)
    }

    wg.Wait()
}
```

#### Pattern 3: Timed Operations

```go
package timer

import "time"

// RunNTimes executes function n times with delay
func RunNTimes(n int, delay time.Duration, fn func(int)) {
    for i := range n {
        fn(i)
        if i < n-1 {
            time.Sleep(delay)
        }
    }
}
```

#### Pattern 4: Matrix Operations

```go
package matrix

type Matrix [][]float64

// NewMatrix creates n×m matrix
func NewMatrix(rows, cols int) Matrix {
    m := make(Matrix, rows)
    for i := range rows {
        m[i] = make([]float64, cols)
    }
    return m
}

// Identity creates n×n identity matrix
func Identity(n int) Matrix {
    m := NewMatrix(n, n)
    for i := range n {
        m[i][i] = 1.0
    }
    return m
}
```

### Branch-Specific Guidelines

#### Main Branch (Go 1.25+)

- ✅ Use integer range syntax for simple 0-to-n loops
- ✅ Refactor existing code when convenient
- ✅ Document the pattern in code reviews
- ✅ Both styles acceptable during transition

#### stable-1-go-1.24 and stable-1-go-1.23

- ❌ Cannot use integer range syntax (not available)
- ✅ Continue using traditional `for i := 0; i < n; i++`
- ✅ Use named constants to improve readability

### Edge Cases and Gotchas

#### Zero Iterations

```go
// Both handle zero iterations identically
for i := range 0 {
    fmt.Println("Never executed")
}

for i := 0; i < 0; i++ {
    fmt.Println("Never executed")
}
```

#### Negative Values

```go
// Range with negative n
n := -5
for i := range n {
    // Loop doesn't execute (same as range 0)
    fmt.Println("Never executed")
}
```

#### Large Values

```go
// Works with large integers
const billion = 1_000_000_000
for i := range billion {
    // Iterates 1 billion times
    // Be careful with large values!
}
```

#### Variable Shadowing

```go
// Be careful with scope
i := 42
for i := range 10 {
    // This i shadows outer i
    fmt.Println(i) // Prints 0-9, not 42
}
fmt.Println(i) // Still 42
```

### Compatibility Check

```go
// In go.mod
go 1.25  // Required for integer range syntax
```

```go
// With build tags
//go:build go1.25

package mypackage

func example() {
    for i := range 10 {
        // Go 1.25+ code
    }
}
```

### Complete Real-World Example

```go
package scheduler

import (
    "fmt"
    "time"
)

type Task struct {
    ID       int
    Name     string
    Duration time.Duration
}

// ScheduleTasks creates and schedules n tasks
func ScheduleTasks(n int, baseDuration time.Duration) []Task {
    tasks := make([]Task, n)

    for i := range n {
        tasks[i] = Task{
            ID:       i + 1,
            Name:     fmt.Sprintf("Task-%d", i+1),
            Duration: baseDuration * time.Duration(i+1),
        }
    }

    return tasks
}

// ExecuteTasks runs tasks in sequence
func ExecuteTasks(tasks []Task) {
    for i := range len(tasks) {
        task := tasks[i]
        fmt.Printf("Starting %s (duration: %v)\n", task.Name, task.Duration)
        time.Sleep(task.Duration)
        fmt.Printf("Completed %s\n", task.Name)
    }
}

func main() {
    tasks := ScheduleTasks(5, 100*time.Millisecond)
    ExecuteTasks(tasks)
}
```

This covers the essential Go style guidelines including formatting, naming conventions, package organization, error handling, function design, struct design, concurrency, comments, and testing best practices.
