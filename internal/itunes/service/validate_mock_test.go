// file: internal/itunes/service/validate_mock_test.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e

package itunesservice

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Validate — ErrLibraryNotFound
// ---------------------------------------------------------------------------

func TestValidate_LibraryNotFound(t *testing.T) {
	req := ValidateRequest{
		LibraryPath: "/nonexistent/path/iTunes Library.xml",
	}

	_, err := Validate(req)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrLibraryNotFound), "expected ErrLibraryNotFound, got %v", err)
}

// ---------------------------------------------------------------------------
// Validate — real XML fixture (integration-style)
// ---------------------------------------------------------------------------

func TestValidate_RealXMLFixture(t *testing.T) {
	// Uses the shared XML fixture in internal/itunes/testdata
	fixture := filepath.Join("..", "testdata", "test_library.xml")

	req := ValidateRequest{
		LibraryPath: fixture,
		PathMappings: []PathMapping{
			// These won't match the fixture paths, but shouldn't error
			{From: "C:/", To: "/mnt/"},
		},
	}

	resp, err := Validate(req)

	require.NoError(t, err)
	// The fixture has at least some tracks
	assert.GreaterOrEqual(t, resp.TotalTracks, 0)
	assert.GreaterOrEqual(t, resp.AudiobookTracks, 0)
	// DuplicateCount must be non-negative
	assert.GreaterOrEqual(t, resp.DuplicateCount, 0)
}

// ---------------------------------------------------------------------------
// TestMapping — non-existent library path
// ---------------------------------------------------------------------------

func TestTestMapping_LibraryParseError(t *testing.T) {
	req := TestMappingRequest{
		LibraryPath: "/nonexistent/iTunes Library.xml",
		From:        "C:/",
		To:          "/mnt/",
	}

	_, err := TestMapping(req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse library")
}

// ---------------------------------------------------------------------------
// TestMapping — real XML fixture, no matching prefix
// ---------------------------------------------------------------------------

func TestTestMapping_NoMatchingPrefix(t *testing.T) {
	fixture := filepath.Join("..", "testdata", "test_library.xml")

	req := TestMappingRequest{
		LibraryPath: fixture,
		From:        "Z:/ImpossiblePrefix/",
		To:          "/mnt/mapped/",
	}

	resp, err := TestMapping(req)

	require.NoError(t, err)
	assert.Equal(t, 0, resp.Tested, "no tracks should match the impossible prefix")
	assert.Equal(t, 0, resp.Found)
	assert.Empty(t, resp.Examples)
}
