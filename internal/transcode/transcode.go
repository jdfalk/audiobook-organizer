// file: internal/transcode/transcode.go
// version: 1.2.0
// guid: f8a1b2c3-d4e5-6789-abcd-ef0123456789

package transcode

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log"
	"os"
	"bufio"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
)

// TranscodeOpts configures a transcode operation.
type TranscodeOpts struct {
	BookID       string
	OutputFormat string // "m4b" default
	Bitrate      int    // kbps, default 128
	KeepOriginal bool   // default true
}

// FindFFmpeg locates ffmpeg on the system PATH.
func FindFFmpeg() (string, error) {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg not found on PATH: %w", err)
	}
	return path, nil
}

// FindFFprobe locates ffprobe on the system PATH.
func FindFFprobe() (string, error) {
	path, err := exec.LookPath("ffprobe")
	if err != nil {
		return "", fmt.Errorf("ffprobe not found on PATH: %w", err)
	}
	return path, nil
}

// CollectInputFiles gathers audio files for a book, sorted by track number.
// If the book has segments, those are used; otherwise the book's file_path is used.
func CollectInputFiles(book *database.Book, segments []database.BookSegment) ([]string, error) {
	if len(segments) > 0 {
		// Sort by track number, then by file path as tiebreaker
		sort.Slice(segments, func(i, j int) bool {
			ti, tj := 0, 0
			if segments[i].TrackNumber != nil {
				ti = *segments[i].TrackNumber
			}
			if segments[j].TrackNumber != nil {
				tj = *segments[j].TrackNumber
			}
			if ti != tj {
				return ti < tj
			}
			return segments[i].FilePath < segments[j].FilePath
		})
		var files []string
		for _, seg := range segments {
			if !seg.Active {
				continue
			}
			if _, err := os.Stat(seg.FilePath); err != nil {
				return nil, fmt.Errorf("segment file not found: %s: %w", seg.FilePath, err)
			}
			files = append(files, seg.FilePath)
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("no active segments found for book %s", book.ID)
		}
		return files, nil
	}

	if book.FilePath == "" {
		return nil, fmt.Errorf("book %s has no file_path", book.ID)
	}
	if _, err := os.Stat(book.FilePath); err != nil {
		return nil, fmt.Errorf("book file not found: %s: %w", book.FilePath, err)
	}
	return []string{book.FilePath}, nil
}

// BuildConcatFile writes an ffmpeg concat demuxer file listing the input files.
// Returns the path to the temp file (caller must clean up).
func BuildConcatFile(inputFiles []string) (string, error) {
	f, err := os.CreateTemp("", "audiobook-concat-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create concat file: %w", err)
	}
	for _, path := range inputFiles {
		// Escape single quotes for ffmpeg concat format
		escaped := strings.ReplaceAll(path, "'", "'\\''")
		if _, err := fmt.Fprintf(f, "file '%s'\n", escaped); err != nil {
			f.Close()
			os.Remove(f.Name())
			return "", fmt.Errorf("failed to write concat file: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// probeDuration returns the duration of an audio file in seconds using ffprobe.
func probeDuration(ffprobePath, filePath string) (float64, error) {
	cmd := exec.Command(ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		filePath,
	)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed for %s: %w", filePath, err)
	}
	var result struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return 0, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}
	dur, err := strconv.ParseFloat(result.Format.Duration, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration %q: %w", result.Format.Duration, err)
	}
	return dur, nil
}

// BuildChapterMetadata probes each input file and generates an FFMetadata chapter file.
// Returns the path to the temp metadata file (caller must clean up).
func BuildChapterMetadata(inputFiles []string) (string, error) {
	ffprobePath, err := FindFFprobe()
	if err != nil {
		return "", err
	}
	return BuildChapterMetadataWithProber(inputFiles, func(path string) (float64, error) {
		return probeDuration(ffprobePath, path)
	})
}

// BuildChapterMetadataWithProber generates chapter metadata using a custom duration prober.
// This is useful for testing.
func BuildChapterMetadataWithProber(inputFiles []string, prober func(string) (float64, error)) (string, error) {
	f, err := os.CreateTemp("", "audiobook-chapters-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create chapter file: %w", err)
	}

	if _, err := fmt.Fprintln(f, ";FFMETADATA1"); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}

	var offsetMs int64
	for i, path := range inputFiles {
		dur, err := prober(path)
		if err != nil {
			f.Close()
			os.Remove(f.Name())
			return "", err
		}
		durationMs := int64(dur * 1000)
		title := fmt.Sprintf("Chapter %d", i+1)

		if _, err := fmt.Fprintf(f, "\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=%d\nEND=%d\ntitle=%s\n",
			offsetMs, offsetMs+durationMs, title); err != nil {
			f.Close()
			os.Remove(f.Name())
			return "", err
		}
		offsetMs += durationMs
	}

	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// Transcode converts audio files for a book into a single M4B.
func Transcode(ctx context.Context, opts TranscodeOpts, store database.Store, progress operations.ProgressReporter) (string, error) {
	ffmpegPath, err := FindFFmpeg()
	if err != nil {
		return "", err
	}

	if opts.OutputFormat == "" {
		opts.OutputFormat = "m4b"
	}
	if opts.Bitrate <= 0 {
		opts.Bitrate = 128
	}

	progress.UpdateProgress(0, 5, "Loading book data")

	book, err := store.GetBookByID(opts.BookID)
	if err != nil {
		return "", fmt.Errorf("failed to get book: %w", err)
	}

	// Get segments for multi-file books
	bookNumericID := int(crc32.ChecksumIEEE([]byte(book.ID)))
	segments, segErr := store.ListBookSegments(bookNumericID)
	if segErr != nil {
		progress.Log("warn", fmt.Sprintf("Could not fetch segments: %v, falling back to book file path", segErr), nil)
		segments = nil
	} else {
		progress.Log("info", fmt.Sprintf("Found %d segments for book %s", len(segments), book.ID), nil)
	}

	inputFiles, err := CollectInputFiles(book, segments)
	if err != nil {
		progress.Log("error", fmt.Sprintf("Failed to collect input files: %v", err), nil)
		return "", fmt.Errorf("failed to collect input files: %w", err)
	}

	multiFile := len(inputFiles) > 1
	progress.Log("info", fmt.Sprintf("Transcoding %d input file(s) to M4B", len(inputFiles)), nil)
	for i, f := range inputFiles {
		progress.Log("info", fmt.Sprintf("  Input %d: %s", i+1, f), nil)
	}

	// Determine output path: same directory, .m4b extension
	baseDir := filepath.Dir(inputFiles[0])
	baseName := strings.TrimSuffix(filepath.Base(book.FilePath), filepath.Ext(book.FilePath))
	if baseName == "" {
		baseName = book.Title
	}
	outputPath := filepath.Join(baseDir, baseName+".m4b")
	tmpOutput := outputPath + ".tmp.m4b"

	progress.UpdateProgress(1, 5, "Preparing transcode")

	var args []string

	// Compute total duration (microseconds) for progress reporting
	var totalDurationUs int64
	for _, seg := range segments {
		totalDurationUs += int64(seg.DurationSec) * 1_000_000
	}
	if totalDurationUs == 0 && book.Duration != nil && *book.Duration > 0 {
		totalDurationUs = int64(*book.Duration) * 1_000_000
	}

	if multiFile {
		// Build concat file
		concatFile, err := BuildConcatFile(inputFiles)
		if err != nil {
			return "", err
		}
		defer os.Remove(concatFile)

		args = []string{
			"-y",
			"-f", "concat",
			"-safe", "0",
			"-i", concatFile,
			"-c:a", "aac",
			"-b:a", fmt.Sprintf("%dk", opts.Bitrate),
			"-movflags", "+faststart",
			"-progress", "pipe:1",
			tmpOutput,
		}
	} else {
		// Single file transcode
		args = []string{
			"-y",
			"-i", inputFiles[0],
			"-c:a", "aac",
			"-b:a", fmt.Sprintf("%dk", opts.Bitrate),
			"-movflags", "+faststart",
			"-progress", "pipe:1",
			tmpOutput,
		}
	}

	progress.UpdateProgress(2, 5, "Transcoding audio")
	progress.Log("info", fmt.Sprintf("Running ffmpeg: %s %s", ffmpegPath, strings.Join(args, " ")), nil)

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Capture stderr in background
	var stderrBuf strings.Builder
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			stderrBuf.WriteString(scanner.Text())
			stderrBuf.WriteString("\n")
		}
	}()

	// Parse stdout progress lines
	scanner := bufio.NewScanner(stdoutPipe)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "out_time_ms=") {
			if us, err := strconv.ParseInt(strings.TrimPrefix(line, "out_time_ms="), 10, 64); err == nil && totalDurationUs > 0 {
				pct := int(us * 100 / totalDurationUs)
				if pct > 100 {
					pct = 100
				}
				progress.UpdateProgress(2, 5, fmt.Sprintf("Transcoding audio (%d%%)", pct))
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		errMsg := fmt.Sprintf("ffmpeg transcode failed: %v", err)
		outputStr := stderrBuf.String()
		progress.Log("error", errMsg, &outputStr)
		return "", fmt.Errorf("ffmpeg transcode failed: %w\noutput: %s", err, outputStr)
	}
	progress.Log("info", "FFmpeg transcode completed successfully", nil)

	// Mux chapter metadata if multi-file
	if multiFile {
		progress.UpdateProgress(3, 5, "Adding chapter markers")

		chapterFile, err := BuildChapterMetadata(inputFiles)
		if err != nil {
			log.Printf("[WARN] transcode: failed to build chapter metadata, skipping: %v", err)
		} else {
			defer os.Remove(chapterFile)

			chapteredOutput := outputPath + ".ch.m4b"
			chapterArgs := []string{
				"-y",
				"-i", tmpOutput,
				"-i", chapterFile,
				"-map_metadata", "1",
				"-map_chapters", "1",
				"-c", "copy",
				chapteredOutput,
			}
			chapterCmd := exec.CommandContext(ctx, ffmpegPath, chapterArgs...)
			chOut, err := chapterCmd.CombinedOutput()
			if err != nil {
				log.Printf("[WARN] transcode: chapter muxing failed, using unchaptered output: %v\noutput: %s", err, string(chOut))
			} else {
				os.Remove(tmpOutput)
				tmpOutput = chapteredOutput
			}
		}
	}

	progress.UpdateProgress(4, 5, "Finalizing")

	// Embed cover art if available
	if book.CoverURL != nil && *book.CoverURL != "" {
		coverPath := *book.CoverURL
		if _, err := os.Stat(coverPath); err == nil {
			coverArgs := []string{
				"-y",
				"-i", tmpOutput,
				"-i", coverPath,
				"-map", "0", "-map", "1",
				"-c", "copy",
				"-disposition:v:0", "attached_pic",
				"-movflags", "+faststart",
				tmpOutput + ".cover.m4b",
			}
			coverCmd := exec.CommandContext(ctx, ffmpegPath, coverArgs...)
			if coverOut, err := coverCmd.CombinedOutput(); err != nil {
				log.Printf("[WARN] transcode: cover art embedding failed: %v\noutput: %s", err, string(coverOut))
			} else {
				os.Remove(tmpOutput)
				tmpOutput = tmpOutput + ".cover.m4b"
			}
		}
	}

	// Atomic rename to final path
	if err := os.Rename(tmpOutput, outputPath); err != nil {
		return "", fmt.Errorf("failed to finalize output file: %w", err)
	}

	progress.UpdateProgress(5, 5, "Complete")
	progress.Log("info", fmt.Sprintf("Transcode complete: %s → %s", book.FilePath, outputPath), nil)

	log.Printf("[INFO] transcode: completed %s → %s", book.FilePath, outputPath)
	return outputPath, nil
}
