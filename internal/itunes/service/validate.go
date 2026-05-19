// file: internal/itunes/service/validate.go
// version: 1.1.0
// guid: 9e3a7f2b-5d1c-4b8e-a6f0-3c8d5e7b9a1f

package itunesservice

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jdfalk/audiobook-organizer/internal/itunes"
)

// ErrLibraryNotFound is returned by Validate when the library file does not exist.
// Callers can check with errors.Is to send a 400 rather than 500.
var ErrLibraryNotFound = errors.New("iTunes library file not found")

// Validate opens the iTunes library XML at req.LibraryPath, runs the
// validator, and returns a summary. Stateless — no Service needed.
func Validate(req ValidateRequest) (ValidateResponse, error) {
	if _, err := os.Stat(req.LibraryPath); os.IsNotExist(err) {
		return ValidateResponse{}, ErrLibraryNotFound
	}

	slog.Info("iTunes validate: library=%s, mappings=%d", req.LibraryPath, len(req.PathMappings))

	mappings := make([]itunes.PathMapping, len(req.PathMappings))
	for i, m := range req.PathMappings {
		mappings[i] = itunes.PathMapping{From: m.From, To: m.To}
	}
	opts := itunes.ImportOptions{
		LibraryPath:    req.LibraryPath,
		ImportMode:     itunes.ImportModeImport,
		SkipDuplicates: false,
		PathMappings:   mappings,
	}

	result, err := itunes.ValidateImport(opts)
	if err != nil {
		return ValidateResponse{}, fmt.Errorf("validation failed: %w", err)
	}

	duplicateCount := 0
	for _, titles := range result.DuplicateHashes {
		if len(titles) > 1 {
			duplicateCount += len(titles) - 1
		}
	}

	missingPaths := result.MissingPaths
	if len(missingPaths) > 100 {
		missingPaths = missingPaths[:100]
	}

	slog.Info("iTunes validate complete: %d audiobooks, %d found, %d missing, prefixes=%v",
		result.AudiobookTracks, result.FilesFound, result.FilesMissing, result.PathPrefixes)

	return ValidateResponse{
		TotalTracks:     result.TotalTracks,
		AudiobookTracks: result.AudiobookTracks,
		AudiobookCount:  result.AudiobookCount,
		FilesFound:      result.FilesFound,
		FilesMissing:    result.FilesMissing,
		MissingPaths:    missingPaths,
		PathPrefixes:    result.PathPrefixes,
		DuplicateCount:  duplicateCount,
		EstimatedTime:   result.EstimatedTime,
	}, nil
}

// TestMapping applies a single path mapping to a sample of iTunes tracks
// and returns the results. Stateless.
func TestMapping(req TestMappingRequest) (TestMappingResponse, error) {
	library, err := itunes.ParseLibrary(req.LibraryPath)
	if err != nil {
		return TestMappingResponse{}, fmt.Errorf("failed to parse library: %w", err)
	}

	slog.Info("iTunes test-mapping: from=%q to=%q", req.From, req.To)
	mapping := itunes.PathMapping{From: req.From, To: req.To}
	opts := itunes.ImportOptions{PathMappings: []itunes.PathMapping{mapping}}

	response := TestMappingResponse{Examples: []TestMappingItem{}}
	for _, track := range library.Tracks {
		if !itunes.IsAudiobook(track) {
			continue
		}
		if !strings.HasPrefix(track.Location, req.From) {
			continue
		}
		if response.Tested >= 20 {
			break
		}
		response.Tested++

		location := opts.RemapPath(track.Location)
		path, err := itunes.DecodeLocation(location)
		if err != nil {
			slog.Info("  [%d/20] decode error for %q: %v", response.Tested, track.Name, err)
			continue
		}
		if _, err := os.Stat(path); err == nil {
			response.Found++
			slog.Info("  [%d/20] FOUND: %q → %s", response.Tested, track.Name, path)
			if len(response.Examples) < 3 {
				response.Examples = append(response.Examples, TestMappingItem{
					Title: track.Name,
					Path:  path,
				})
			}
		} else {
			slog.Info("  [%d/20] MISSING: %q → %s", response.Tested, track.Name, path)
		}
	}

	slog.Info("iTunes test-mapping: tested=%d found=%d examples=%d", response.Tested, response.Found, len(response.Examples))
	return response, nil
}
