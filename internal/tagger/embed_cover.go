// file: internal/tagger/embed_cover.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package tagger

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrToolNotFound is returned when the required external tool is not installed.
var ErrToolNotFound = fmt.Errorf("required external tool not found")

// findTool checks if a command-line tool exists on the system PATH.
func findTool(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	return path, nil
}

// EmbedCoverArt embeds a cover image into an audio file's metadata tags.
// It detects the audio format from the file extension and uses the appropriate
// external tool (ffmpeg for MP3/M4A/M4B/AAC/OGG, metaflac for FLAC).
//
// The original file is replaced atomically: tags are written to a temp file,
// then the temp file is renamed over the original.
func EmbedCoverArt(audioPath string, coverPath string) error {
	if audioPath == "" {
		return fmt.Errorf("empty audio path")
	}
	if coverPath == "" {
		return fmt.Errorf("empty cover path")
	}

	// Verify both files exist
	if _, err := os.Stat(audioPath); err != nil {
		return fmt.Errorf("audio file not found: %w", err)
	}
	if _, err := os.Stat(coverPath); err != nil {
		return fmt.Errorf("cover file not found: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(audioPath))

	switch ext {
	case ".mp3":
		return embedWithFFmpeg(audioPath, coverPath, "mp3")
	case ".m4b", ".m4a", ".aac":
		return embedWithFFmpeg(audioPath, coverPath, "mp4")
	case ".ogg":
		return embedWithFFmpeg(audioPath, coverPath, "ogg")
	case ".flac":
		return embedWithMetaflac(audioPath, coverPath)
	default:
		return fmt.Errorf("unsupported audio format for cover embedding: %s", ext)
	}
}

// embedWithFFmpeg uses ffmpeg to embed cover art into MP3, M4A/M4B, or OGG files.
// For MP3, this writes an ID3v2 APIC frame.
// For M4A/M4B, this writes an mp4 covr atom.
func embedWithFFmpeg(audioPath, coverPath, format string) error {
	ffmpegPath, err := findTool("ffmpeg")
	if err != nil {
		return err
	}

	tmpFile := audioPath + ".tmp" + filepath.Ext(audioPath)
	defer os.Remove(tmpFile) // clean up on failure

	// Build ffmpeg command:
	//   ffmpeg -i audio -i cover -map 0 -map 1 -c copy -disposition:v:0 attached_pic output
	// For mp4 containers we need -c:v mjpeg if the cover is JPEG (ffmpeg handles this).
	args := []string{
		"-y",                       // overwrite output
		"-i", audioPath,            // input audio
		"-i", coverPath,            // input cover image
		"-map", "0",                // map all streams from audio
		"-map", "1",                // map cover image
		"-c", "copy",               // copy all codecs (no re-encoding)
		"-disposition:v:0", "attached_pic", // mark as attached picture
	}

	// For MP4 containers, set the video codec to copy (cover is just attached)
	if format == "mp4" {
		// MP4 containers need the metadata copy flag
		args = append(args, "-movflags", "+faststart")
	}

	args = append(args, tmpFile)

	cmd := exec.Command(ffmpegPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %w\noutput: %s", err, string(output))
	}

	// Atomic replace: rename temp over original
	if err := os.Rename(tmpFile, audioPath); err != nil {
		return fmt.Errorf("failed to replace original file: %w", err)
	}

	log.Printf("[DEBUG] tagger: embedded cover art from %s into %s", coverPath, audioPath)
	return nil
}

// embedWithMetaflac uses metaflac to embed cover art into FLAC files.
func embedWithMetaflac(audioPath, coverPath string) error {
	metaflacPath, err := findTool("metaflac")
	if err != nil {
		return err
	}

	// First remove any existing pictures, then import the new one
	removeCmd := exec.Command(metaflacPath, "--remove", "--block-type=PICTURE", audioPath)
	if output, err := removeCmd.CombinedOutput(); err != nil {
		log.Printf("[WARN] tagger: metaflac --remove PICTURE failed (may be ok): %s", string(output))
		// Not fatal -- file may not have had a picture
	}

	importCmd := exec.Command(metaflacPath, "--import-picture-from="+coverPath, audioPath)
	output, err := importCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("metaflac --import-picture failed: %w\noutput: %s", err, string(output))
	}

	log.Printf("[DEBUG] tagger: embedded cover art from %s into %s (FLAC)", coverPath, audioPath)
	return nil
}
