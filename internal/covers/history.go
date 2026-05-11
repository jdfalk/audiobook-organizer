// file: internal/covers/history.go
// version: 1.0.0
// guid: d4e5f6a7-8901-bcde-f123-4567890abcde
// last-edited: 2026-05-11
//
// Cover history management for browsing and restoring previous cover versions.
// Business logic extracted from internal/server/cover_history.go.

package covers

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CoverHistoryEntry represents one saved cover version.
type CoverHistoryEntry struct {
	Filename  string `json:"filename"`
	URL       string `json:"url"`
	SizeBytes int64  `json:"size_bytes"`
	ModTime   string `json:"mod_time"`
}

// ListCoverHistory returns all cover versions for a book, sorted by modification time (newest first).
func ListCoverHistory(bookID, rootDir string) ([]CoverHistoryEntry, error) {
	histDir := filepath.Join(rootDir, "covers", "history", bookID)

	entries, err := os.ReadDir(histDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []CoverHistoryEntry{}, nil
		}
		return nil, err
	}

	var covers []CoverHistoryEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		covers = append(covers, CoverHistoryEntry{
			Filename:  name,
			URL:       "/api/v1/covers/local/" + name,
			SizeBytes: info.Size(),
			ModTime:   info.ModTime().Format("2006-01-02T15:04:05Z"),
		})
	}

	sort.Slice(covers, func(i, j int) bool {
		return covers[i].ModTime > covers[j].ModTime
	})

	return covers, nil
}

// RestoreCoverFile copies a historical cover to the current cover location.
func RestoreCoverFile(bookID, filename, rootDir string) (string, error) {
	// Prevent path traversal
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		return "", os.ErrInvalid
	}

	srcPath := filepath.Join(rootDir, "covers", "history", bookID, filename)
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return "", os.ErrNotExist
	}

	// Copy the history file to the current cover location
	dstDir := filepath.Join(rootDir, "covers")
	ext := filepath.Ext(filename)
	dstPath := filepath.Join(dstDir, bookID+ext)

	src, err := os.ReadFile(srcPath)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dstPath, src, 0o644); err != nil {
		return "", err
	}

	return dstPath, nil
}
