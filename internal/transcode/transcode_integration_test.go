// file: internal/transcode/transcode_integration_test.go
// version: 1.0.0
// guid: c9789340-1670-48d9-88e7-383f2c998be1

//go:build integration

package transcode_test

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/operations"
	"github.com/jdfalk/audiobook-organizer/internal/testutil"
	"github.com/jdfalk/audiobook-organizer/internal/transcode"
	ulid "github.com/oklog/ulid/v2"
)

// testMP3Files lists the Librivox Odyssey MP3 fixtures used for integration tests.
var testMP3Files = []string{
	"odyssey_01_homer_butler_64kb.mp3",
	"odyssey_02_homer_butler_64kb.mp3",
	"odyssey_03_homer_butler_64kb.mp3",
}

// testFixtureDir is the repo-relative path to the real MP3 test files.
const testFixtureDir = "testdata/audio/librivox/odyssey_butler_librivox"

// mockProgressReporter implements operations.ProgressReporter for testing.
type mockProgressReporter struct {
	progressUpdates []progressUpdate
	logEntries      []logEntry
	canceled        bool
}

type progressUpdate struct {
	current int
	total   int
	message string
}

type logEntry struct {
	level   string
	message string
	detail  *string
}

func (m *mockProgressReporter) UpdateProgress(current, total int, message string) error {
	m.progressUpdates = append(m.progressUpdates, progressUpdate{current, total, message})
	return nil
}

func (m *mockProgressReporter) Log(level, message string, detail *string) error {
	m.logEntries = append(m.logEntries, logEntry{level, message, detail})
	return nil
}

func (m *mockProgressReporter) IsCanceled() bool {
	return m.canceled
}

// copyMP3sToTemp copies the 3 test MP3 files into a temporary directory.
// Returns the temp directory path and a slice of copied file paths.
func copyMP3sToTemp(t *testing.T, repoRoot string) (string, []string) {
	t.Helper()
	tmpDir := t.TempDir()
	bookDir := filepath.Join(tmpDir, "odyssey")
	require.NoError(t, os.MkdirAll(bookDir, 0755))

	var copied []string
	for _, name := range testMP3Files {
		src := filepath.Join(repoRoot, testFixtureDir, name)
		dst := filepath.Join(bookDir, name)
		testutil.CopyFile(t, src, dst)
		copied = append(copied, dst)
	}
	return bookDir, copied
}

// intPtr returns a pointer to an int value.
func intPtr(v int) *int { return &v }

// boolPtr returns a pointer to a bool value.
func boolPtr(v bool) *bool { return &v }

// ffprobeChapterCount uses ffprobe to count chapters in an audio file.
func ffprobeChapterCount(t *testing.T, filePath string) int {
	t.Helper()
	ffprobePath, err := exec.LookPath("ffprobe")
	require.NoError(t, err, "ffprobe must be available")

	cmd := exec.Command(ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_chapters",
		filePath,
	)
	output, err := cmd.Output()
	require.NoError(t, err, "ffprobe should succeed on %s", filePath)

	var result struct {
		Chapters []json.RawMessage `json:"chapters"`
	}
	require.NoError(t, json.Unmarshal(output, &result))
	return len(result.Chapters)
}

// ffprobeValidate checks that a file is a valid audio file using ffprobe.
func ffprobeValidate(t *testing.T, filePath string) {
	t.Helper()
	ffprobePath, err := exec.LookPath("ffprobe")
	require.NoError(t, err)

	cmd := exec.Command(ffprobePath,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "ffprobe validation failed for %s: %s", filePath, string(output))

	var result struct {
		Format struct {
			FormatName string `json:"format_name"`
			Duration   string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
		} `json:"streams"`
	}
	require.NoError(t, json.Unmarshal(output, &result))
	assert.NotEmpty(t, result.Format.FormatName, "format_name should not be empty")
	assert.NotEmpty(t, result.Format.Duration, "duration should not be empty")

	// Verify at least one audio stream exists
	hasAudio := false
	for _, s := range result.Streams {
		if s.CodecType == "audio" {
			hasAudio = true
			break
		}
	}
	assert.True(t, hasAudio, "M4B should contain at least one audio stream")
}

// TestTranscodeIntegration_M4BConversion tests the Transcode function directly
// using real ffmpeg and real MP3 files from the testdata directory.
func TestTranscodeIntegration_M4BConversion(t *testing.T) {
	// Skip if ffmpeg is not available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not found")
	}

	// Set up integration environment with real SQLite
	env, cleanup := testutil.SetupIntegration(t)
	t.Cleanup(cleanup)

	repoRoot := testutil.FindRepoRoot(t)

	// Copy MP3 files to a temp directory (never modify originals)
	bookDir, copiedFiles := copyMP3sToTemp(t, repoRoot)

	// Create a Book record in the database
	book := &database.Book{
		Title:    "The Odyssey - Homer (Butler translation)",
		FilePath: bookDir,
		Format:   "mp3",
	}
	createdBook, err := env.Store.CreateBook(book)
	require.NoError(t, err, "CreateBook should succeed")
	require.NotEmpty(t, createdBook.ID, "Book should have an assigned ID")

	// Create BookSegment records for each MP3 file
	bookNumericID := int(crc32.ChecksumIEEE([]byte(createdBook.ID)))
	totalTracks := len(copiedFiles)

	for i, filePath := range copiedFiles {
		fi, err := os.Stat(filePath)
		require.NoError(t, err)

		trackNum := i + 1
		seg := &database.BookSegment{
			FilePath:    filePath,
			Format:      "mp3",
			SizeBytes:   fi.Size(),
			DurationSec: 0, // will be probed by ffmpeg
			TrackNumber: intPtr(trackNum),
			TotalTracks: intPtr(totalTracks),
			Active:      true,
		}
		created, err := env.Store.CreateBookSegment(bookNumericID, seg)
		require.NoError(t, err, "CreateBookSegment should succeed for track %d", trackNum)
		require.NotEmpty(t, created.ID, "Segment should have an assigned ID")
	}

	// Set up transcode options
	opts := transcode.TranscodeOpts{
		BookID:       createdBook.ID,
		OutputFormat: "m4b",
		Bitrate:      64,
		KeepOriginal: true,
	}

	// Create mock progress reporter
	progress := &mockProgressReporter{}

	// Run transcode with a generous timeout (5 minutes for 3 MP3 files)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	outputPath, err := transcode.Transcode(ctx, opts, env.Store, progress)
	require.NoError(t, err, "Transcode should complete without error")

	// Verify: output M4B file exists
	require.FileExists(t, outputPath, "Output M4B file should exist")

	// Verify: output path has .m4b extension
	assert.Equal(t, ".m4b", filepath.Ext(outputPath), "Output file should have .m4b extension")

	// Verify: the M4B file is a valid audio file
	ffprobeValidate(t, outputPath)

	// Verify: the M4B file has chapters (one per input MP3)
	chapterCount := ffprobeChapterCount(t, outputPath)
	assert.Equal(t, len(copiedFiles), chapterCount,
		"M4B should have %d chapters (one per input file), got %d", len(copiedFiles), chapterCount)

	// Verify: progress reporter received updates
	assert.NotEmpty(t, progress.progressUpdates, "Progress reporter should have received updates")
	assert.NotEmpty(t, progress.logEntries, "Progress reporter should have received log entries")

	// Verify: the last progress update indicates completion
	lastUpdate := progress.progressUpdates[len(progress.progressUpdates)-1]
	assert.Equal(t, 5, lastUpdate.current, "Final progress should be 5")
	assert.Equal(t, 5, lastUpdate.total, "Final progress total should be 5")
	assert.Equal(t, "Complete", lastUpdate.message, "Final progress message should be 'Complete'")

	// Verify: original MP3 files still exist (KeepOriginal=true)
	for _, mp3 := range copiedFiles {
		assert.FileExists(t, mp3, "Original MP3 should still exist when KeepOriginal=true")
	}

	// Clean up the output M4B
	t.Cleanup(func() { os.Remove(outputPath) })
}

// TestTranscodeIntegration_ServerFlow tests the full server-level transcode flow
// by replicating the logic from the server's startTranscode handler. It creates
// an operation record, enqueues the transcode through the operation queue (exactly
// as the POST /api/v1/operations/transcode endpoint does), polls for completion,
// and then verifies the resulting database state and files on disk.
func TestTranscodeIntegration_ServerFlow(t *testing.T) {
	// Skip if ffmpeg is not available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not found")
	}

	// Set up integration environment (configures GlobalStore, GlobalQueue, etc.)
	env, cleanup := testutil.SetupIntegration(t)
	t.Cleanup(cleanup)

	repoRoot := testutil.FindRepoRoot(t)

	// Copy MP3 files to temp directory
	bookDir, copiedFiles := copyMP3sToTemp(t, repoRoot)

	// Create a Book record
	book := &database.Book{
		Title:    "The Odyssey - Server Test",
		FilePath: bookDir,
		Format:   "mp3",
	}
	createdBook, err := env.Store.CreateBook(book)
	require.NoError(t, err)

	// Create BookSegment records
	bookNumericID := int(crc32.ChecksumIEEE([]byte(createdBook.ID)))
	totalTracks := len(copiedFiles)

	for i, filePath := range copiedFiles {
		fi, err := os.Stat(filePath)
		require.NoError(t, err)

		trackNum := i + 1
		seg := &database.BookSegment{
			FilePath:    filePath,
			Format:      "mp3",
			SizeBytes:   fi.Size(),
			DurationSec: 0,
			TrackNumber: intPtr(trackNum),
			TotalTracks: intPtr(totalTracks),
			Active:      true,
		}
		_, err = env.Store.CreateBookSegment(bookNumericID, seg)
		require.NoError(t, err)
	}

	// --- Replicate server's startTranscode handler logic ---

	// Create an operation record (as the server endpoint does)
	opID := ulid.Make().String()
	op, err := env.Store.CreateOperation(opID, "transcode", nil)
	require.NoError(t, err)
	require.NotEmpty(t, op.ID)

	bookID := createdBook.ID
	opts := transcode.TranscodeOpts{
		BookID:       bookID,
		OutputFormat: "m4b",
		Bitrate:      64,
		KeepOriginal: true,
	}

	// Build the operation function (mirrors server.go lines 5467-5551)
	operationFunc := func(ctx context.Context, progress operations.ProgressReporter) error {
		outputPath, err := transcode.Transcode(ctx, opts, database.GetGlobalStore(), progress)
		if err != nil {
			return err
		}

		// Get the original book to preserve its data
		originalBook, err := database.GetGlobalStore().GetBookByID(bookID)
		if err != nil {
			return fmt.Errorf("failed to get original book: %w", err)
		}

		// Set up version group
		groupID := ""
		if originalBook.VersionGroupID != nil && *originalBook.VersionGroupID != "" {
			groupID = *originalBook.VersionGroupID
		} else {
			groupID = ulid.Make().String()
		}

		// Mark original as non-primary
		notPrimary := false
		origNotes := "Original format"
		originalBook.IsPrimaryVersion = &notPrimary
		originalBook.VersionGroupID = &groupID
		originalBook.VersionNotes = &origNotes
		if _, err := database.GetGlobalStore().UpdateBook(bookID, originalBook); err != nil {
			progress.Log("warn", fmt.Sprintf("Failed to update original book version info: %v", err), nil)
		}

		// Create new M4B book record
		m4bFormat := "m4b"
		aacCodec := "aac"
		bitrateVal := opts.Bitrate
		if bitrateVal <= 0 {
			bitrateVal = 128
		}
		isPrimary := true
		m4bNotes := "Transcoded to M4B"

		newBook := &database.Book{
			ID:               ulid.Make().String(),
			Title:            originalBook.Title,
			FilePath:         outputPath,
			Format:           m4bFormat,
			Codec:            &aacCodec,
			Bitrate:          &bitrateVal,
			AuthorID:         originalBook.AuthorID,
			SeriesID:         originalBook.SeriesID,
			SeriesSequence:   originalBook.SeriesSequence,
			Duration:         originalBook.Duration,
			Narrator:         originalBook.Narrator,
			Publisher:        originalBook.Publisher,
			IsPrimaryVersion: &isPrimary,
			VersionGroupID:   &groupID,
			VersionNotes:     &m4bNotes,
		}
		if _, err := database.GetGlobalStore().CreateBook(newBook); err != nil {
			return fmt.Errorf("failed to create M4B book record: %w", err)
		}

		progress.Log("info", fmt.Sprintf("Created M4B version %s (group %s)", newBook.ID, groupID), nil)
		return nil
	}

	// Enqueue the operation (as the server does)
	err = operations.GlobalQueue.Enqueue(op.ID, "transcode", operations.PriorityNormal, operationFunc)
	require.NoError(t, err, "Enqueue should succeed")

	// Poll operation status until complete (timeout 5 minutes)
	testutil.WaitForOp(t, env.Store, op.ID, 5*time.Minute)

	// Verify operation completed successfully
	finalOp, err := env.Store.GetOperationByID(op.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", finalOp.Status,
		"Operation should complete successfully; error: %v", finalOp.ErrorMessage)

	// Verify: original book is now non-primary
	originalBook, err := env.Store.GetBookByID(createdBook.ID)
	require.NoError(t, err)
	require.NotNil(t, originalBook.IsPrimaryVersion, "Original book should have IsPrimaryVersion set")
	assert.False(t, *originalBook.IsPrimaryVersion, "Original book should be non-primary after transcode")

	// Verify: original book has version notes
	require.NotNil(t, originalBook.VersionNotes, "Original book should have VersionNotes set")
	assert.Equal(t, "Original format", *originalBook.VersionNotes)

	// Verify: original book has a version group ID
	require.NotNil(t, originalBook.VersionGroupID, "Original book should have VersionGroupID set")
	groupID := *originalBook.VersionGroupID
	assert.NotEmpty(t, groupID, "VersionGroupID should not be empty")

	// Verify: new M4B book record was created in the same version group
	versionBooks, err := env.Store.GetBooksByVersionGroup(groupID)
	require.NoError(t, err)
	require.Len(t, versionBooks, 2, "Version group should contain exactly 2 books (original + M4B)")

	// Find the new M4B book (the one that is NOT the original)
	var m4bBook *database.Book
	for i := range versionBooks {
		if versionBooks[i].ID != createdBook.ID {
			m4bBook = &versionBooks[i]
			break
		}
	}
	require.NotNil(t, m4bBook, "M4B book record should exist in version group")

	// Verify: new book is primary
	require.NotNil(t, m4bBook.IsPrimaryVersion, "M4B book should have IsPrimaryVersion set")
	assert.True(t, *m4bBook.IsPrimaryVersion, "M4B book should be the primary version")

	// Verify: new book has correct version notes
	require.NotNil(t, m4bBook.VersionNotes, "M4B book should have VersionNotes set")
	assert.Equal(t, "Transcoded to M4B", *m4bBook.VersionNotes)

	// Verify: both books share the same version group ID
	require.NotNil(t, m4bBook.VersionGroupID)
	assert.Equal(t, groupID, *m4bBook.VersionGroupID,
		"M4B book should share the same VersionGroupID as the original")

	// Verify: new book has m4b format
	assert.Equal(t, "m4b", m4bBook.Format, "M4B book format should be 'm4b'")

	// Verify: the M4B file exists on disk
	assert.FileExists(t, m4bBook.FilePath, "M4B file should exist on disk")

	// Verify: the M4B file is valid
	ffprobeValidate(t, m4bBook.FilePath)

	// Verify: M4B has chapters
	chapterCount := ffprobeChapterCount(t, m4bBook.FilePath)
	assert.Equal(t, len(copiedFiles), chapterCount,
		"M4B should have %d chapters, got %d", len(copiedFiles), chapterCount)

	// Clean up M4B file
	t.Cleanup(func() { os.Remove(m4bBook.FilePath) })
}
