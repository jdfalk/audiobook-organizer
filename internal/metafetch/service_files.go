// file: internal/metafetch/service_files.go
// version: 1.0.0
// guid: 969b284a-5657-442b-beba-275e325e000b
// last-edited: 2026-05-01

package metafetch

import (
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/fileops"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

func AudioFilesInDir(dir string) []string {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	var files []string
	for _, ext := range audioExtensions {
		matches, err := filepath.Glob(filepath.Join(dir, ext))
		if err == nil {
			files = append(files, matches...)
		}
	}
	return files
}
// backupFileBeforeWrite creates a timestamped .bak copy of a file before
// writing tags — IF the WriteBackupBeforeTagWrite config flag is enabled.
//
// Default is OFF. Historically this function ran unconditionally on every
// tag write and used os.Link (hardlink) for "no disk space cost". Two
// problems with that:
//
//  1. Tens of thousands of stale backup files accumulated across the
//     library (43K+ files, multi-TB apparent size in production) because
//     nothing ever cleaned them up.
//  2. Hardlinks don't actually preserve pre-write content when the
//     writer modifies the inode in place (which TagLib does for some
//     formats). The "backup" could be a hardlink to the same now-modified
//     data, providing false safety.
//
// The flag is opt-in. Users who turn it on should also run the
// cleanup-backups maintenance endpoint periodically to keep the library
// from growing unbounded.
//
// Failures are logged but non-fatal — the write-back proceeds regardless.
func backupFileBeforeWrite(filePath string) {
	if !config.AppConfig.WriteBackupBeforeTagWrite {
		return
	}
	if filePath == "" {
		return
	}
	if _, err := os.Stat(filePath); err != nil {
		return
	}
	backupPath := filePath + ".bak-" + time.Now().Format("20060102-150405")
	if err := os.Link(filePath, backupPath); err != nil {
		// Hardlink failed — fall back to copy
		if err := fileops.SafeCopy(filePath, backupPath, fileops.OperationConfig{}); err != nil {
			log.Printf("[WARN] backup before tag write failed: %s: %v", filePath, err)
			return
		}
	}
	log.Printf("[DEBUG] backup before tag write: %s", backupPath)
}
// ApplyMetadataFileIO runs the slow file operations after metadata is applied:
// cover embed, tag write-back, file rename. Cover download is done inline
// in ApplyMetadataCandidate so the response includes the updated cover URL.
// Designed to run in a background goroutine.
func (mfs *Service) ApplyMetadataFileIO(id string) {
	book, err := mfs.db.GetBookByID(id)
	if err != nil || book == nil {
		return
	}

	// Embed cover art into audio files (slow: ffmpeg)
	if config.AppConfig.RootDir != "" {
		mfs.embedCoverInBookFiles(book, metadata.CoverPathForBook(config.AppConfig.RootDir, id))
	}

	// Run file rename + tag write pipeline
	if config.AppConfig.AutoRenameOnApply || config.AppConfig.AutoWriteTagsOnApply {
		if err := mfs.runApplyPipeline(id, book); err != nil {
			log.Printf("[WARN] apply pipeline failed for %s: %v", id, err)
		}
	}
}
// computeITunesPath converts a local file path to an iTunes file:// URL
// using the configured path mappings (m.To = Linux prefix, m.From = Windows prefix).
// Returns an empty string if no mapping matches.
func ComputeITunesPath(localPath string) string {
	for _, m := range config.AppConfig.ITunesPathMappings {
		if m.To != "" && m.From != "" && strings.HasPrefix(localPath, m.To) {
			remainder := localPath[len(m.To):]
			windowsPath := m.From + remainder
			encoded := url.PathEscape(windowsPath)
			encoded = strings.ReplaceAll(encoded, "%2F", "/")
			encoded = strings.ReplaceAll(encoded, "%3A", ":")
			return "file://localhost/" + encoded
		}
	}
	return ""
}
// removeEmptyDirs removes empty directories walking up from dir until reaching stopAt.
func removeEmptyDirs(dir, stopAt string) {
	for dir != stopAt && dir != "/" && dir != "." {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		if err := os.Remove(dir); err != nil {
			break
		}
		log.Printf("[INFO] removed empty directory: %s", dir)
		dir = filepath.Dir(dir)
	}
}
var audioExtensions = []string{"*.m4b", "*.m4a", "*.mp3", "*.flac", "*.ogg", "*.opus", "*.wma", "*.aac"}
