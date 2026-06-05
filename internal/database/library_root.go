// file: internal/database/library_root.go
// version: 1.0.0
// guid: 01234567-89ab-cdef-0123-456789abcdef
// last-edited: 2026-06-04

package database

import (
	"os"
	"path/filepath"
	"strings"
)

// buildLibraryRootCandidates returns a deduplicated slice of normalized paths
// used to attribute book_files and books to a library root. Candidates include
// the configured organized root plus every import path path.
func buildLibraryRootCandidates(rootDir string, importPaths []ImportPath) []string {
	seen := make(map[string]struct{})
	var roots []string
	add := func(path string) {
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		if clean == "." {
			return
		}
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		roots = append(roots, clean)
	}
	add(rootDir)
	for _, ip := range importPaths {
		add(ip.Path)
	}
	return roots
}

// libraryRootForPath returns the best-matching candidate for filePath or a
// sensible fallback when no candidate matches. Cleaned paths are compared using
// prefix matching so longer import path prefixes win when multiples overlap.
func libraryRootForPath(filePath string, candidates []string) string {
	cleaned := filepath.Clean(filePath)
	if cleaned == "" || cleaned == "." {
		return ""
	}

	best := ""
	bestLen := -1
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		candidate = filepath.Clean(candidate)
		if candidate == "." {
			continue
		}
		candidate = strings.TrimSuffix(candidate, string(os.PathSeparator))
		if candidate == "" {
			continue
		}
		if strings.HasPrefix(cleaned, candidate) {
			if len(candidate) > bestLen {
				best = candidate
				bestLen = len(candidate)
			}
		}
	}
	if best != "" {
		return best
	}
	return fallbackLibraryRoot(cleaned)
}

func fallbackLibraryRoot(cleanedPath string) string {
	if cleanedPath == "" || cleanedPath == "." {
		return ""
	}
	vol := filepath.VolumeName(cleanedPath)
	remainder := strings.TrimPrefix(cleanedPath, vol)
	remainder = strings.TrimPrefix(remainder, string(os.PathSeparator))
	parts := strings.Split(remainder, string(os.PathSeparator))
	if len(parts) == 0 || parts[0] == "" {
		if vol != "" {
			return filepath.Clean(vol + string(os.PathSeparator))
		}
		return cleanedPath
	}
	if vol != "" {
		return filepath.Clean(filepath.Join(vol+string(os.PathSeparator), parts[0]))
	}
	if strings.HasPrefix(cleanedPath, string(os.PathSeparator)) {
		return filepath.Clean(filepath.Join(string(os.PathSeparator), parts[0]))
	}
	return parts[0]
}
