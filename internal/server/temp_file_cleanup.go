// file: internal/server/temp_file_cleanup.go
// version: 1.0.0
// guid: e4f5a6b7-c8d9-0e1f-2a3b-4c5d6e7f8a9b

package server

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// cleanupOrphanedTempFiles removes *.tmp.m4b, *.tmp.m4a, *.tmp.mp3, and
// *.remux.tmp files left behind by ffmpeg operations that were interrupted
// by a crash or server restart. Returns the number of files removed.
func cleanupOrphanedTempFiles(root string) int {
	if root == "" {
		return 0
	}

	removed := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if isOrphanedTempFile(name) {
			if rmErr := os.Remove(path); rmErr != nil {
				log.Printf("[WARN] temp file cleanup: could not remove %s: %v", path, rmErr)
			} else {
				log.Printf("[INFO] temp file cleanup: removed %s", path)
				removed++
			}
		}
		return nil
	})
	return removed
}

func isOrphanedTempFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, ".tmp.m4b") ||
		strings.Contains(lower, ".tmp.m4a") ||
		strings.Contains(lower, ".tmp.mp3") ||
		strings.Contains(lower, ".tmp.flac") ||
		strings.HasSuffix(lower, ".remux.tmp")
}
