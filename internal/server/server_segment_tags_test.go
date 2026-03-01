// file: internal/server/server_segment_tags_test.go
// version: 1.0.0
// guid: a7b8c9d0-e1f2-3456-7890-abcdef123456

package server

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSegmentTags_BookNotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audiobooks/nonexistent/segments/seg1/tags", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "not found")
}

func TestGetSegmentTags_SegmentNotFound(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a book first
	book, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Segment Tags Test Book",
		FilePath: "/tmp/test-segment-tags.m4b",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/audiobooks/%s/segments/nonexistent-seg/tags", book.ID), nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "segment not found")
}

func TestGetSegmentTags_Success(t *testing.T) {
	server, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a book
	book, err := database.GlobalStore.CreateBook(&database.Book{
		Title:    "Multi-File Book",
		FilePath: "/tmp/multi-file-book/part1.m4b",
	})
	require.NoError(t, err)

	// Create a segment
	bookNumericID := int(crc32.ChecksumIEEE([]byte(book.ID)))
	trackNum := 1
	totalTracks := 2
	seg, err := database.GlobalStore.CreateBookSegment(bookNumericID, &database.BookSegment{
		FilePath:    "/tmp/multi-file-book/part1.m4b",
		Format:      "m4b",
		SizeBytes:   1024,
		DurationSec: 3600,
		TrackNumber: &trackNum,
		TotalTracks: &totalTracks,
		Active:      true,
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/audiobooks/%s/segments/%s/tags", book.ID, seg.ID), nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// File doesn't exist on disk, so we expect a tags_read_error
	assert.NotEmpty(t, resp["tags_read_error"], "expected tags_read_error since file doesn't exist")
	assert.Contains(t, resp, "tags")
	assert.Contains(t, resp, "file_path")
}
