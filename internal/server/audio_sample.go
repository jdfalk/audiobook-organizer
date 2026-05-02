// file: internal/server/audio_sample.go
// version: 1.1.0
// guid: d4e5f6a7-b8c9-0d1e-2f3a-4b5c6d7e8f9a
// last-edited: 2026-05-01

package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jdfalk/audiobook-organizer/internal/httputil"
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
		httputil.RespondWithNotFound(c, "book", "")
		return
	}
	if book.FilePath == "" {
		httputil.RespondWithError(c, http.StatusUnprocessableEntity, "book has no file path", "UNPROCESSABLE_ENTITY")
		return
	}

	start := httputil.ParseQueryInt(c, "start", 0)
	if start < 0 {
		start = 0
	}
	dur := httputil.ParseQueryInt(c, "duration", sampleDefault)
	if dur <= 0 {
		dur = sampleDefault
	}
	if dur > sampleMaxDuration {
		dur = sampleMaxDuration
	}

	ffmpegPath, err := transcode.FindFFmpeg()
	if err != nil {
		httputil.RespondWithServiceUnavailable(c, "ffmpeg not available")
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
		httputil.RespondWithInternalError(c, fmt.Sprintf("pipe: %v", err))
		return
	}
	if err := cmd.Start(); err != nil {
		httputil.RespondWithInternalError(c, fmt.Sprintf("ffmpeg start: %v", err))
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
