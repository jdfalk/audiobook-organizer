// file: internal/server/malformed_m4b_remux.go
// version: 1.1.0
// guid: d3e4f5a6-b7c8-9d0e-1f2a-3b4c5d6e7f8a

package server

import (
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

const malformedRemuxKey = "malformed_m4b_remux_v2_done"

// remuxMalformedM4BFiles walks the library once and re-muxes any M4B/M4A
// file that taglib cannot parse (malformed atom structure). Re-muxing with
// ffmpeg -c copy rewrites the atom layout without re-encoding audio, making
// the file readable by taglib, AtomicParsley, and Apple Devices. The output
// is verified before replacing the original. Runs once at startup.
func (s *Server) remuxMalformedM4BFiles() {
	store := s.Store()
	if store == nil {
		return
	}

	if setting, err := store.GetSetting(malformedRemuxKey); err == nil && setting != nil && setting.Value == "true" {
		log.Printf("[INFO] Malformed M4B remux already completed, skipping")
		return
	}

	root := config.AppConfig.RootDir
	if root == "" {
		log.Printf("[WARN] remuxMalformedM4BFiles: RootDir not configured, skipping")
		return
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		log.Printf("[WARN] remuxMalformedM4BFiles: ffmpeg not found, skipping")
		return
	}

	log.Printf("[INFO] Starting malformed M4B remux scan under %s …", root)
	remuxed, clean, failed := 0, 0, 0

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".m4b" && ext != ".m4a" {
			return nil
		}

		// Skip orphaned temp files — those are handled by cleanupOrphanedTempFiles.
		if strings.Contains(filepath.Base(path), ".tmp.") {
			return nil
		}

		if _, err := taglib.ReadTags(path); err == nil {
			clean++
			return nil
		}

		// taglib failed — attempt to remux with ffmpeg.
		if err := remuxFile(path); err != nil {
			log.Printf("[WARN] malformed M4B remux failed for %s: %v", path, err)
			failed++
			return nil
		}

		// Verify the output is now readable.
		if _, err := taglib.ReadTags(path); err != nil {
			log.Printf("[WARN] malformed M4B remux produced unreadable file for %s: %v", path, err)
			failed++
			return nil
		}

		log.Printf("[INFO] malformed M4B remuxed: %s", path)
		remuxed++
		return nil
	})

	log.Printf("[INFO] Malformed M4B remux: %d remuxed, %d already readable, %d failed", remuxed, clean, failed)
	_ = store.SetSetting(malformedRemuxKey, "true", "bool", false)
}

// remuxFile re-muxes an M4B/M4A file in-place using ffmpeg -c copy.
// Writes to a temp file first, then atomically renames over the original.
func remuxFile(path string) error {
	tmp := path + ".remux.tmp"
	defer os.Remove(tmp)

	cmd := exec.Command("ffmpeg",
		"-nostdin", "-loglevel", "error", "-y",
		"-i", path,
		"-map", "0",
		"-c", "copy",
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
