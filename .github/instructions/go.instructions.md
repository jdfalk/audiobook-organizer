<!-- file: .github/instructions/go.instructions.md -->
<!-- version: 1.6.0 -->
<!-- guid: 4f5a6b7c-8d9e-0f1a-2b3c-4d5e6f7a8b9c -->
<!-- DO NOT EDIT: This file is managed centrally in ghcommon repository -->
<!-- To update: Create an issue/PR in jdfalk/ghcommon -->

---
applyTo: "**/*.go"
description: |
  Go language-specific coding, documentation, and testing rules for Copilot/AI agents and VS Code Copilot customization. These rules extend the general instructions in `general-coding.instructions.md` and merge all unique content from the Google Go Style Guide.
---

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

## Version Requirements

- **MANDATORY**: All Go projects must use Go 1.23.0 or higher
- **NO EXCEPTIONS**: Do not use older Go versions in any repository
- Update `go.mod` files to specify `go 1.23` minimum version
- Update `go.work` files to specify `go 1.23` minimum version
- All Go file headers must use version 1.23.0 or higher
- Use `go version` to verify your installation meets requirements

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

This covers the essential Go style guidelines including formatting, naming conventions, package organization, error handling, function design, struct design, concurrency, comments, and testing best practices.
