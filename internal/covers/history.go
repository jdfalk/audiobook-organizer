// file: internal/covers/history.go
// version: 1.1.0
// guid: d4e5f6a7-8901-bcde-f123-4567890abcde
// last-edited: 2026-05-18
//
// Cover history management for browsing and restoring previous cover versions.
// Business logic extracted from internal/server/cover_history.go.

package covers

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/security/safepath"
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
	histDirSP, err := safepath.Join(rootDir, "covers", "history", bookID)
	if err != nil {
		return []CoverHistoryEntry{}, nil
	}

	entries, err := os.ReadDir(histDirSP.String())
	if err != nil {
		if os.IsNotExist(err) {
			return []CoverHistoryEntry{}, nil
		}
		return nil, err
	}

	var result []CoverHistoryEntry
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
		result = append(result, CoverHistoryEntry{
			Filename:  name,
			URL:       "/api/v1/covers/local/" + name,
			SizeBytes: info.Size(),
			ModTime:   info.ModTime().Format("2006-01-02T15:04:05Z"),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ModTime > result[j].ModTime
	})

	return result, nil
}

// RestoreCoverFile copies a historical cover to the current cover location.
func RestoreCoverFile(bookID, filename, rootDir string) (string, error) {
	// Reject filenames that cross directory boundaries before path construction.
	if strings.ContainsAny(filename, "/\\") || strings.Contains(filename, "..") {
		return "", os.ErrInvalid
	}
	srcSP, err := safepath.Join(rootDir, "covers", "history", bookID, filename)
	if err != nil {
		return "", os.ErrInvalid
	}
	if _, err := os.Stat(srcSP.String()); os.IsNotExist(err) {
		return "", os.ErrNotExist
	}

	ext := filepath.Ext(filename)
	dstSP, err := safepath.Join(rootDir, "covers", bookID+ext)
	if err != nil {
		return "", os.ErrInvalid
	}

	src, err := os.ReadFile(srcSP.String())
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dstSP.String(), src, 0o644); err != nil {
		return "", err
	}

	return dstSP.String(), nil
}
