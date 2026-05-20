// file: internal/remux/transcode.go
// version: 1.0.1
// guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e

package remux

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	taglib "go.senan.xyz/taglib"
)

const TranscodeKey = "malformed_m4b_transcode_v1_done"

// Transcoder provides malformed M4B transcode operations.
type Transcoder struct {
	store Store
}

// NewTranscoder creates a new Transcoder instance.
func NewTranscoder(store Store) *Transcoder {
	return &Transcoder{store: store}
}

// TranscodeSkipKey returns a settings key that marks a specific file as
// permanently unfixable by transcode, so restarts don't re-attempt it.
func TranscodeSkipKey(path string) string {
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("transcode_skip_%x", h[:8])
}

// TranscodeMalformedFiles walks the library and re-encodes any M4B/M4A
// file that taglib cannot parse even after the remux pass. Full AAC transcode
// at 64 kbps rebuilds the file from scratch, which fixes corruption that a
// stream copy cannot repair. Runs once at startup.
func (t *Transcoder) TranscodeMalformedFiles() {
	if t.store == nil {
		return
	}

	if setting, err := t.store.GetSetting(TranscodeKey); err == nil && setting != nil && setting.Value == "true" {
		slog.Info("Malformed M4B transcode already completed, skipping")
		return
	}

	root := config.AppConfig.RootDir
	if root == "" {
		slog.Warn("TranscodeMalformedFiles: RootDir not configured, skipping")
		return
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		slog.Warn("TranscodeMalformedFiles: ffmpeg not found, skipping")
		return
	}

	// Pre-mark files confirmed permanently unfixable by full transcode.
	// These produce valid ffmpeg output but taglib still cannot parse them.
	permanentlyUnfixable := []string{
		"/mnt/bigdata/books/audiobook-organizer/David Petrie/Necrotic Apocalypse (7 book series)/Necrotic Apocalypse (7 book series)/Necrotic Apocalypse (7 book series) - David Petrie - read by narrator.m4b",
		"/mnt/bigdata/books/audiobook-organizer/Eric Ugland/One More Last Time_ A LitRPG/GameLit Novel (The Good Guys/One More Last Time_ A LitRPG/GameLit Novel (The Good Guys, Book 1)/One More Last Time_ A LitRPG/GameLit Novel (The Good Guys, Book 1) - Eric Ugland - read by narrator.m4b",
	}
	for _, p := range permanentlyUnfixable {
		k := TranscodeSkipKey(p)
		if skip, _ := t.store.GetSetting(k); skip == nil {
			_ = t.store.SetSetting(k, "true", "bool", false)
			slog.Info("malformed M4B transcode: pre-marked permanently unfixable:", "value0", "p", p)
		}
	}

	slog.Info("Starting malformed M4B transcode scan under  …", "value0", "root", root)
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
		if skip, err := t.store.GetSetting(TranscodeSkipKey(path)); err == nil && skip != nil && skip.Value == "true" {
			slog.Info("malformed M4B transcode: skipping known-unfixable", "value0", "path", path)
			skipped++
			failed++
			return nil
		}

		// taglib failed — attempt full AAC transcode.
		if err := TranscodeFile(path); err != nil {
			slog.Warn("malformed M4B transcode failed for :", "value0", "path", "path", path, "err", err)
			_ = t.store.SetSetting(TranscodeSkipKey(path), "true", "bool", false)
			failed++
			return nil
		}

		// Verify the output is now readable.
		if _, err := taglib.ReadTags(path); err != nil {
			slog.Warn("malformed M4B transcode produced unreadable file for :", "value0", "path", "path", path, "err", err)
			_ = t.store.SetSetting(TranscodeSkipKey(path), "true", "bool", false)
			failed++
			return nil
		}

		slog.Info("malformed M4B transcoded:", "value0", "path", path)
		transcoded++
		return nil
	})

	slog.Info("Malformed M4B transcode:  transcoded,  already readable,  failed ( permanently skipped)", "value0", "transcoded", "transcoded", transcoded, "value2", "clean", "clean", clean, "failed", failed, "skipped", skipped)
	_ = t.store.SetSetting(TranscodeKey, "true", "bool", false)
}

// TranscodeFile re-encodes an M4B/M4A file to 64 kbps AAC in-place.
// Writes to a temp file first, then atomically renames over the original.
func TranscodeFile(path string) error {
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
