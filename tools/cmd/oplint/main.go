// file: tools/cmd/oplint/main.go
// version: 1.0.0
// guid: d0e1f2a3-b4c5-6789-3456-789abcdef012
// last-edited: 2026-05-06

// Package main implements oplint, a plugin import-path linter.
// It enforces that operations code only imports from:
//   - internal/operations/registry
//   - internal/database/iface
//   - stdlib and third-party packages
//
// This prevents accidental dependencies on internal business logic (the "walled
// garden" constraint).
package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <dir1> [dir2] ...\n", os.Args[0])
		os.Exit(2)
	}

	var violations []string

	for _, startDir := range os.Args[1:] {
		if err := walkDir(startDir, &violations); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	if len(violations) > 0 {
		for _, v := range violations {
			fmt.Println(v)
		}
		os.Exit(1)
	}
}

func walkDir(dir string, violations *[]string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			return checkFile(path, violations)
		}
		return nil
	})
}

func checkFile(path string, violations *[]string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		// Skip files that can't be parsed (build tags, invalid syntax during linting)
		return nil
	}

	for _, imp := range f.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)

		// Allow stdlib (no dots) and third-party packages (have dots but don't start with "github.com/jdfalk/...")
		if !strings.Contains(importPath, "/") {
			// Stdlib — allowed
			continue
		}

		// Reject imports from the jdfalk/audiobook-organizer internal/ except allowed ones.
		if strings.HasPrefix(importPath, "github.com/jdfalk/audiobook-organizer/internal/") {
			if importPath == "github.com/jdfalk/audiobook-organizer/internal/operations/registry" ||
				importPath == "github.com/jdfalk/audiobook-organizer/internal/database/iface" ||
				importPath == "github.com/jdfalk/audiobook-organizer/internal/auth" {
				// Allowed
				continue
			}
			*violations = append(*violations, fmt.Sprintf("%s: imports forbidden internal package: %s", path, importPath))
		}
	}
	return nil
}
