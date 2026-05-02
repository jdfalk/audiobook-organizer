// file: internal/util/normalize.go
// version: 1.0.0
// guid: a3f7e2d1-9b4c-4e8a-b6f0-2c5d7a1e3f9b
// last-edited: 2026-05-02

// Package util provides shared string and path normalization helpers.
package util

import (
	"path/filepath"
	"strings"
	"unicode"
)

// NormalizePath cleans a filepath and lowercases it for consistent comparison.
func NormalizePath(p string) string {
	return strings.ToLower(filepath.Clean(p))
}

// NormalizeTitle trims whitespace and lowercases a title for comparison.
func NormalizeTitle(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// NormalizeAuthor trims whitespace and lowercases an author name for comparison.
func NormalizeAuthor(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// NormalizeString trims whitespace and lowercases any generic string.
func NormalizeString(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// CollapseSpaces replaces runs of whitespace with a single space and trims.
func CollapseSpaces(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteRune(' ')
			}
			prevSpace = true
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return b.String()
}
