// file: internal/server/file_pipeline_test.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-012345678901

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestComputeTargetPaths(t *testing.T) {
	intPtr := func(n int) *int { return &n }

	segments := []database.BookSegment{
		{
			ID:          "seg-1",
			FilePath:    "/library/old/file1.m4b",
			Format:      "m4b",
			Active:      true,
			TrackNumber: intPtr(1),
		},
		{
			ID:          "seg-2",
			FilePath:    "/library/old/file2.m4b",
			Format:      "m4b",
			Active:      true,
			TrackNumber: intPtr(2),
		},
	}

	vars := FormatVars{
		Author: "Author Name",
		Title:  "Book Title",
	}

	entries := ComputeTargetPaths(
		"/library",
		DefaultPathFormat,
		DefaultSegmentTitleFormat,
		&database.Book{ID: "test-id", Title: "Book Title"},
		segments,
		vars,
	)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	for _, e := range entries {
		if e.SourcePath == e.TargetPath {
			t.Errorf("source and target should differ: %s", e.SourcePath)
		}
		if e.SegmentID == "" {
			t.Error("segment ID should not be empty")
		}
	}
}

func TestComputeTargetPathsEmpty(t *testing.T) {
	entries := ComputeTargetPaths("", DefaultPathFormat, DefaultSegmentTitleFormat, &database.Book{}, nil, FormatVars{})
	if entries != nil {
		t.Errorf("expected nil for empty root dir, got %v", entries)
	}
}

func TestComputeTargetPathsInactiveSegments(t *testing.T) {
	segments := []database.BookSegment{
		{
			ID:       "seg-1",
			FilePath: "/library/old/file1.m4b",
			Format:   "m4b",
			Active:   false,
		},
	}

	entries := ComputeTargetPaths(
		"/library",
		DefaultPathFormat,
		DefaultSegmentTitleFormat,
		&database.Book{ID: "test-id", Title: "Book Title"},
		segments,
		FormatVars{Author: "Author", Title: "Book Title"},
	)

	if len(entries) != 0 {
		t.Errorf("expected 0 entries for inactive segments, got %d", len(entries))
	}
}
