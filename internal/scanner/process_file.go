// file: internal/scanner/process_file.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890

// Package scanner provides file scanning and processing utilities for the
// audiobook organizer. ProcessFile is the single-pass entry point that opens
// a file exactly once and extracts metadata, media info, and a content hash.
package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/dhowden/tag"
	"github.com/jdfalk/audiobook-organizer/internal/mediainfo"
	"github.com/jdfalk/audiobook-organizer/internal/metadata"
)

const (
	hashThreshold = 100 * 1024 * 1024 // 100 MB — files above this get a partial hash
	hashChunkSize = 10 * 1024 * 1024  // 10 MB chunks for the partial hash
)

// ProcessFile opens filePath exactly once and returns:
//   - meta: extracted audio metadata (never nil on success)
//   - mi:   technical media info (nil for directories or when tags cannot be read)
//   - hash: SHA-256 hex string of the file content (empty for directories)
//
// The hash algorithm matches ComputeFileHash: full SHA-256 for files ≤100 MB,
// and first-10MB + last-10MB + file-size for larger files.
//
// Existing callers of metadata.ExtractMetadata, mediainfo.Extract, and
// ComputeFileHash are unaffected — those functions continue to work as before.
func ProcessFile(filePath string) (*metadata.Metadata, *mediainfo.MediaInfo, string, error) {
	if filePath == "" {
		return nil, nil, "", fmt.Errorf("ProcessFile: empty file path")
	}

	// stat first — catches non-existence and distinguishes dirs from files
	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("ProcessFile: stat %q: %w", filePath, err)
	}

	// Directories: fall back to metadata-only extraction (no mediainfo, no hash)
	if fi.IsDir() {
		log.Printf("[DEBUG] scanner.ProcessFile: %s is a directory, extracting path metadata only", filePath)
		meta, err := metadata.ExtractMetadata(filePath, nil)
		if err != nil {
			return nil, nil, "", fmt.Errorf("ProcessFile: directory metadata for %q: %w", filePath, err)
		}
		return &meta, nil, "", nil
	}

	// Open the file once
	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("ProcessFile: open %q: %w", filePath, err)
	}
	defer f.Close()

	fileSize := fi.Size()

	// Read tags — on failure we still need to hash, so don't abort yet
	tagMeta, tagErr := tag.ReadFrom(f)

	// Extract metadata
	var meta metadata.Metadata
	var mi *mediainfo.MediaInfo

	if tagErr != nil {
		log.Printf("[WARN] scanner.ProcessFile: tag read failed for %s: %v; using filename fallback", filePath, tagErr)
		meta, err = metadata.ExtractMetadata(filePath, nil) // opens file again — rare error path
		if err != nil {
			log.Printf("[WARN] scanner.ProcessFile: filename fallback also failed for %s: %v", filePath, err)
		}
		// mi stays nil — we have no tag to build from
	} else {
		meta = metadata.BuildMetadataFromTag(tagMeta, filePath, nil)
		mi = mediainfo.BuildFromTag(tagMeta, filePath, fileSize)
	}

	// Seek back to start for hashing
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return &meta, mi, "", fmt.Errorf("ProcessFile: seek to start for hashing %q: %w", filePath, err)
	}

	// Compute hash (matches ComputeFileHash logic exactly)
	hash, err := computeHashFromReader(f, fileSize)
	if err != nil {
		return &meta, mi, "", fmt.Errorf("ProcessFile: hash %q: %w", filePath, err)
	}

	log.Printf("[DEBUG] scanner.ProcessFile: done %s (title=%q author=%q hash=%s...)", filePath, meta.Title, meta.Artist, hash[:8])
	return &meta, mi, hash, nil
}

// computeHashFromReader hashes content from an open file reader.
// For files ≤ hashThreshold it hashes all bytes; for larger files it hashes
// the first hashChunkSize bytes + last hashChunkSize bytes + the file size.
// This is the same algorithm as ComputeFileHash.
func computeHashFromReader(f *os.File, fileSize int64) (string, error) {
	if fileSize > hashThreshold {
		h := sha256.New()

		// First chunk
		first := make([]byte, hashChunkSize)
		n, err := f.Read(first)
		if err != nil && err != io.EOF {
			return "", err
		}
		h.Write(first[:n])

		// Last chunk
		if fileSize > hashChunkSize {
			if _, err := f.Seek(-hashChunkSize, io.SeekEnd); err != nil {
				return "", err
			}
			last := make([]byte, hashChunkSize)
			n, err = f.Read(last)
			if err != nil && err != io.EOF {
				return "", err
			}
			h.Write(last[:n])
		}

		// Include size in hash
		h.Write([]byte(fmt.Sprintf("%d", fileSize)))

		return hex.EncodeToString(h.Sum(nil)), nil
	}

	// Full hash for smaller files
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
