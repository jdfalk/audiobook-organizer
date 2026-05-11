// file: internal/fingerprint/backfill_utils.go
// version: 1.0.0
// guid: e5f6a7b8-c9d0-e1f2-a3b4-c5d6e7f8a9b0

package fingerprint

import (
	"os"
	"path/filepath"
	"strings"
)

// AudioExtensions are audio file extensions recognized by the fingerprint system.
var AudioExtensions = map[string]bool{
	".mp3":  true,
	".m4b":  true,
	".m4a":  true,
	".aac":  true,
	".flac": true,
	".opus": true,
	".ogg":  true,
	".wma":  true,
	".wav":  true,
}

// IsAudioFile checks if a file path is recognized as an audio file.
func IsAudioFile(filePath string) bool {
	if filePath == "" {
		return false
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	_, ok := AudioExtensions[ext]
	return ok
}

// FileExists checks if a file exists on disk.
func FileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}
