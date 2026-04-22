// file: internal/server/malformed_m4b_transcode.go
// version: 1.1.0
// guid: f1a2b3c4-d5e6-7f8a-9b0c-1d2e3f4a5b6c

package server

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	taglib "go.senan.xyz/taglib"
)

const malformedTranscodeKey = "malformed_m4b_transcode_v1_done"

// transcodeSkipKey returns a settings key that marks a specific file as
// permanently unfixable by transcode, so restarts don't re-attempt it.
func transcodeSkipKey(path string) string {
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("transcode_skip_%x", h[:8])
}

// transcodeMalformedM4BFiles walks the library and re-encodes any M4B/M4A
// file that taglib cannot parse even after the remux pass. Full AAC transcode
// at 64 kbps rebuilds the file from scratch, which fixes corruption that a
// stream copy cannot repair. Runs once at startup.
func (s *Server) transcodeMalformedM4BFiles() {
	store := s.Store()
	if store == nil {
		return
	}

	if setting, err := store.GetSetting(malformedTranscodeKey); err == nil && setting != nil && setting.Value == "true" {
		log.Printf("[INFO] Malformed M4B transcode already completed, skipping")
		return
	}

	root := config.AppConfig.RootDir
	if root == "" {
		log.Printf("[WARN] transcodeMalformedM4BFiles: RootDir not configured, skipping")
		return
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Printf("[WARN] transcodeMalformedM4BFiles: ffmpeg not found, skipping")
		return
	}

	log.Printf("[INFO] Starting malformed M4B transcode scan under %s …", root)
	transcoded, clean, failed, skipped := 0, 0, 0, 0

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".m4b" && ext != ".m4a" {
			return nil
		}

		if strings.Contains(filepath.Base(path), ".tmp.") {
			return nil
		}

		if _, err := taglib.ReadTags(path); err == nil {
			clean++
			return nil
		}

		// Skip files that have already been confirmed permanently unfixable.
		if skip, err := store.GetSetting(transcodeSkipKey(path)); err == nil && skip != nil && skip.Value == "true" {
			log.Printf("[INFO] malformed M4B transcode: skipping known-unfixable %s", path)
			skipped++
			failed++
			return nil
		}

		// taglib failed — attempt full AAC transcode.
		if err := transcodeFile(path); err != nil {
			log.Printf("[WARN] malformed M4B transcode failed for %s: %v", path, err)
			_ = store.SetSetting(transcodeSkipKey(path), "true", "bool", false)
			failed++
			return nil
		}

		// Verify the output is now readable.
		if _, err := taglib.ReadTags(path); err != nil {
			log.Printf("[WARN] malformed M4B transcode produced unreadable file for %s: %v", path, err)
			_ = store.SetSetting(transcodeSkipKey(path), "true", "bool", false)
			failed++
			return nil
		}

		log.Printf("[INFO] malformed M4B transcoded: %s", path)
		transcoded++
		return nil
	})

	log.Printf("[INFO] Malformed M4B transcode: %d transcoded, %d already readable, %d failed (%d permanently skipped)", transcoded, clean, failed, skipped)
	_ = store.SetSetting(malformedTranscodeKey, "true", "bool", false)
}

// transcodeFile re-encodes an M4B/M4A file to 64 kbps AAC in-place.
// Writes to a temp file first, then atomically renames over the original.
func transcodeFile(path string) error {
	tmp := path + ".remux.tmp"
	defer os.Remove(tmp)

	cmd := exec.Command("ffmpeg",
		"-nostdin", "-loglevel", "error", "-y",
		"-i", path,
		"-vn",
		"-c:a", "aac", "-b:a", "64k",
		"-map_metadata", "0",
		"-map_chapters", "0",
		"-f", "mp4",
		tmp,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg: %w — %s", err, strings.TrimSpace(string(out)))
	}

	return os.Rename(tmp, path)
}
