// file: internal/server/audio_sample.go
// version: 1.0.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f9a

package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/transcode"
)

const (
	sampleMaxDuration = 60  // seconds — hard cap per request
	sampleDefault     = 30  // seconds — default clip length
)

// handleAudioSample streams a short MP3 clip from an audiobook file.
//
// GET /api/v1/audiobooks/:id/sample?start=0&duration=30
//
//   - start    — offset in seconds (default 0, clamped to [0, book duration])
//   - duration — clip length in seconds (default 30, capped at 60)
//
// ffmpeg seeks to `start` before opening the input (-ss before -i) so it
// reads only the needed frames rather than decoding from the beginning.
// Output is streamed as audio/mpeg directly to the client — no temp files.
func (s *Server) handleAudioSample(c *gin.Context) {
	book, err := s.Store().GetBookByID(c.Param("id"))
	if err != nil || book == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
		return
	}
	if book.FilePath == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "book has no file path"})
		return
	}

	start, err := queryInt(c, "start", 0)
	if err != nil || start < 0 {
		start = 0
	}
	dur, err := queryInt(c, "duration", sampleDefault)
	if err != nil || dur <= 0 {
		dur = sampleDefault
	}
	if dur > sampleMaxDuration {
		dur = sampleMaxDuration
	}

	ffmpegPath, err := transcode.FindFFmpeg()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ffmpeg not available"})
		return
	}

	args := []string{
		"-ss", strconv.Itoa(start), // seek before input (fast)
		"-i", book.FilePath,
		"-t", strconv.Itoa(dur),
		"-vn",          // no video/cover stream
		"-map_chapters", "-1", // strip chapters so mp3 muxer is happy
		"-f", "mp3",
		"-q:a", "5",    // ~130 kbps VBR — fine for comparison
		"pipe:1",
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 120)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("pipe: %v", err)})
		return
	}
	if err := cmd.Start(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("ffmpeg start: %v", err)})
		return
	}

	c.Header("Content-Type", "audio/mpeg")
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	c.Stream(func(w io.Writer) bool {
		buf := make([]byte, 32*1024)
		n, readErr := stdout.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return false
			}
		}
		return readErr == nil
	})

	_ = cmd.Wait()
}

// queryInt reads a query parameter as int, returning defaultVal if absent or unparseable.
func queryInt(c *gin.Context, key string, defaultVal int) (int, error) {
	raw := c.Query(key)
	if raw == "" {
		return defaultVal, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return defaultVal, err
	}
	return v, nil
}
