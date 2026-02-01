<!-- file: .github/instructions/go.instructions.md -->
<!-- version: 2.0.0 -->
<!-- guid: 4a5b6c7d-8e9f-1a2b-3c4d-5e6f7a8b9c0d -->
<!-- last-edited: 2026-01-31 -->

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
---
applyTo: "**/*.go"
description: |
  Go language-specific coding, documentation, and testing rules. Extends general-coding.instructions.md. Follow Google Go Style Guide for additional guidance.
---
<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->

# Go Coding Instructions

- Follow [general coding instructions](general-coding.instructions.md).
- Follow the [Google Go Style Guide](https://google.github.io/styleguide/go/index.html).
- All Go files must begin with the required file header.

## Version

- **Go 1.25** on main branch. Use `go 1.25` in `go.mod`.
- No Windows support. Build targets: linux/macOS amd64+arm64.

## Naming

- **Packages:** short, lowercase, single words. No underscores. (`user`, `httputil`)
- **Interfaces:** single-method interfaces end in `-er` (`Reader`, `Writer`). No `I` prefix.
- **Functions/Methods:** PascalCase exported, camelCase unexported. Start with verb.
- **Variables:** short for short scope, descriptive for longer scope. camelCase.
- **Constants:** PascalCase exported, camelCase unexported. Group related constants in `const` blocks.

## Code Organization

- Use `goimports` for import formatting.
- Import groups (blank line between): stdlib → third-party → local.
- Keep functions short and focused. Blank lines between logical sections.

## Formatting

- Tabs for indentation. Opening brace on same line. `gofmt` is non-negotiable.

## Comments

- Every package needs a package comment.
- All exported functions: comment starts with function name.
- Comment exported variables with purpose and constraints.
- Inline comments explain *why*, not *what*.

## Error Handling

- Lowercase error messages, no trailing punctuation.
- Wrap with `fmt.Errorf("context: %w", err)` — add context as errors bubble up.
- Use `errors.Is` / `errors.As` for checking.
- Check errors immediately. Never ignore with `_` unless truly appropriate.

```go
func processUser(id string) error {
    user, err := getUserFromDB(id)
    if err != nil {
        return fmt.Errorf("failed to get user %s: %w", id, err)
    }
    return validateUser(user)
}
```

## Best Practices

- Short variable declarations (`:=`) when possible; `var` for zero values.
- `make()` for slices/maps with known capacity.
- Accept interfaces, return concrete types.
- Keep interfaces small and focused.
- Channels for goroutine communication; sync primitives for shared state.
- Test files: `*_test.go`, functions start with `Test`.
- Table-driven tests for multiple scenarios.

## Go 1.24+ Features

### Benchmarks: `b.Loop()`

Use `for b.Loop()` instead of `for i := 0; i < b.N; i++` in sequential benchmarks.
`RunParallel` still uses `pb.Next()`.

```go
func BenchmarkProcess(b *testing.B) {
    data := setup()
    b.ResetTimer()
    for b.Loop() {
        process(data)
    }
}
```

## Go 1.25 Features

### Integer Range: `for i := range n`

Replaces `for i := 0; i < n; i++` for simple 0-to-n loops. Same performance, less boilerplate.
Still use traditional loops for non-zero start, custom step, or countdown.

```go
for i := range count {
    items[i] = create(i)
}
```

### Filesystem Isolation: `os.Root()`

Creates a sandboxed filesystem view. Blocks path traversal automatically.

```go
root, err := os.Root("/safe/dir")
if err != nil { return err }
defer root.Close()
file, err := root.Open("data.txt") // confined to /safe/dir
```

Use for: untrusted plugin execution, multi-tenant isolation, secure file serving.
Combine with input validation — `os.Root()` prevents path escape but not logic errors.

### Generics

- Constrain type parameters appropriately (`comparable`, `constraints.Ordered`).
- Don't over-genericize simple functions — prefer concrete types for hot paths.
- Go 1.25 has better type inference; explicit type args often unnecessary.

## Concurrency

```go
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

- Always consider how goroutines exit.
- Close channels when done sending. Use `defer close(ch)`.

## Testing

Table-driven tests are the standard pattern:

```go
func TestCalculateTotal(t *testing.T) {
    tests := []struct {
        name     string
        price    float64
        taxRate  float64
        expected float64
        wantErr  bool
    }{
        {"positive", 100, 0.1, 110, false},
        {"negative tax", 100, -0.1, 0, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := CalculateTotal(tt.price, tt.taxRate)
            if (err != nil) != tt.wantErr {
                t.Errorf("unexpected error: %v", err)
                return
            }
            if !tt.wantErr && got != tt.expected {
                t.Errorf("got %f, want %f", got, tt.expected)
            }
        })
    }
}
```

## Required File Header

```go
// file: path/to/file.go
// version: 1.0.0
// guid: 123e4567-e89b-12d3-a456-426614174000
```
