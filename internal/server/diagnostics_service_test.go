// file: internal/server/diagnostics_service_test.go
// version: 1.0.0
// guid: d1a9n0st-1cs0-t3st-s3rv-1c3t3st0001

package server

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	dbmocks "github.com/jdfalk/audiobook-organizer/internal/database/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDiagnosticsMocks(t *testing.T) *dbmocks.MockStore {
	store := dbmocks.NewMockStore(t)
	store.EXPECT().GetAllBooks(10000, 0).Return([]database.Book{
		{ID: "book1", Title: "Test Book"},
	}, nil).Maybe()
	store.EXPECT().GetAllBooks(10000, 10000).Return([]database.Book{}, nil).Maybe()
	store.EXPECT().GetAllAuthors().Return([]database.Author{}, nil).Maybe()
	store.EXPECT().GetAllSeries().Return([]database.Series{}, nil).Maybe()
	store.EXPECT().GetAllAuthorBookCounts().Return(map[int]int{}, nil).Maybe()
	store.EXPECT().GetAllSeriesBookCounts().Return(map[int]int{}, nil).Maybe()
	store.EXPECT().GetAllAuthorFileCounts().Return(map[int]int{}, nil).Maybe()
	store.EXPECT().GetAllSeriesFileCounts().Return(map[int]int{}, nil).Maybe()
	store.EXPECT().CountBooks().Return(1, nil).Maybe()
	store.EXPECT().CountAuthors().Return(0, nil).Maybe()
	store.EXPECT().CountSeries().Return(0, nil).Maybe()
	store.EXPECT().GetSystemActivityLogs("", 10000).Return(nil, nil).Maybe()
	store.EXPECT().GetRecentOperations(100).Return(nil, nil).Maybe()
	return store
}

func readZipFile(t *testing.T, r *zip.ReadCloser, name string) []byte {
	t.Helper()
	for _, f := range r.File {
		if f.Name == name {
			rc, err := f.Open()
			require.NoError(t, err)
			defer rc.Close()
			data, err := io.ReadAll(rc)
			require.NoError(t, err)
			return data
		}
	}
	t.Fatalf("file %s not found in ZIP", name)
	return nil
}

func TestDiagnosticsService_GenerateExport_Deduplication(t *testing.T) {
	store := setupDiagnosticsMocks(t)

	svc := NewDiagnosticsService(store, nil, "")
	zipPath, err := svc.GenerateExport("deduplication", "test export")
	require.NoError(t, err)
	defer os.Remove(zipPath)

	r, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer r.Close()

	fileNames := make(map[string]bool)
	for _, f := range r.File {
		fileNames[f.Name] = true
	}

	// Common files always present
	assert.True(t, fileNames["system_info.json"], "missing system_info.json")
	assert.True(t, fileNames["books.json"], "missing books.json")
	assert.True(t, fileNames["authors.json"], "missing authors.json")
	assert.True(t, fileNames["series.json"], "missing series.json")
	assert.True(t, fileNames["batch.jsonl"], "missing batch.jsonl")

	// Dedup-specific files
	assert.True(t, fileNames["version_groups.json"], "missing version_groups.json")
	assert.True(t, fileNames["itunes_albums.json"], "missing itunes_albums.json")

	// Should NOT have error_analysis files
	assert.False(t, fileNames["logs.json"], "should not have logs.json for deduplication")
	assert.False(t, fileNames["operations.json"], "should not have operations.json for deduplication")

	// Verify system_info content
	data := readZipFile(t, r, "system_info.json")
	var sysInfo map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &sysInfo))
	assert.Equal(t, "deduplication", sysInfo["category"])
	assert.Equal(t, "test export", sysInfo["description"])
	assert.Equal(t, float64(1), sysInfo["book_count"])

	// Verify books.json has our test book
	booksData := readZipFile(t, r, "books.json")
	var books []map[string]interface{}
	require.NoError(t, json.Unmarshal(booksData, &books))
	require.Len(t, books, 1)
	assert.Equal(t, "book1", books[0]["id"])
	assert.Equal(t, "Test Book", books[0]["title"])
}

func TestDiagnosticsService_GenerateExport_ErrorAnalysis(t *testing.T) {
	store := setupDiagnosticsMocks(t)

	now := time.Now()
	store.EXPECT().GetSystemActivityLogs("", 10000).Unset()
	store.EXPECT().GetSystemActivityLogs("", 10000).Return([]database.SystemActivityLog{
		{ID: 1, Source: "scanner", Level: "error", Message: "scan failed", CreatedAt: now},
		{ID: 2, Source: "scanner", Level: "info", Message: "old log", CreatedAt: now.Add(-48 * time.Hour)},
	}, nil).Maybe()

	svc := NewDiagnosticsService(store, nil, "")
	zipPath, err := svc.GenerateExport("error_analysis", "debug errors")
	require.NoError(t, err)
	defer os.Remove(zipPath)

	r, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer r.Close()

	fileNames := make(map[string]bool)
	for _, f := range r.File {
		fileNames[f.Name] = true
	}

	// Error analysis specific files
	assert.True(t, fileNames["logs.json"], "missing logs.json")
	assert.True(t, fileNames["operations.json"], "missing operations.json")

	// Should NOT have dedup files
	assert.False(t, fileNames["version_groups.json"], "should not have version_groups.json for error_analysis")
	assert.False(t, fileNames["itunes_albums.json"], "should not have itunes_albums.json for error_analysis")

	// Verify logs are filtered to last 24h
	logsData := readZipFile(t, r, "logs.json")
	var logs []database.SystemActivityLog
	require.NoError(t, json.Unmarshal(logsData, &logs))
	assert.Len(t, logs, 1, "should only include logs from last 24h")
	assert.Equal(t, "scan failed", logs[0].Message)
}

func TestDiagnosticsService_GenerateExport_MetadataQuality(t *testing.T) {
	store := setupDiagnosticsMocks(t)

	authorID := 1
	seriesID := 1
	store.EXPECT().GetAllBooks(10000, 0).Unset()
	store.EXPECT().GetAllBooks(10000, 0).Return([]database.Book{
		{ID: "book1", Title: "Complete Book", AuthorID: &authorID, SeriesID: &seriesID},
		{ID: "book2", Title: "", AuthorID: nil},              // missing title, author, series
		{ID: "book3", Title: "No Author", AuthorID: nil},     // missing author, series
	}, nil).Maybe()
	store.EXPECT().GetAllBooks(10000, 10000).Unset()
	store.EXPECT().GetAllBooks(10000, 10000).Return([]database.Book{}, nil).Maybe()
	store.EXPECT().CountBooks().Unset()
	store.EXPECT().CountBooks().Return(3, nil).Maybe()

	svc := NewDiagnosticsService(store, nil, "")
	zipPath, err := svc.GenerateExport("metadata_quality", "check quality")
	require.NoError(t, err)
	defer os.Remove(zipPath)

	r, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer r.Close()

	fileNames := make(map[string]bool)
	for _, f := range r.File {
		fileNames[f.Name] = true
	}

	assert.True(t, fileNames["missing_fields.json"], "missing missing_fields.json")

	// Verify missing_fields content
	data := readZipFile(t, r, "missing_fields.json")
	var missingFields []map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &missingFields))
	// book2 missing title+author+series, book3 missing author+series
	assert.Len(t, missingFields, 2, "should have 2 books with missing fields")
}

func TestDiagnosticsService_GenerateExport_General(t *testing.T) {
	store := setupDiagnosticsMocks(t)

	svc := NewDiagnosticsService(store, nil, "")
	zipPath, err := svc.GenerateExport("general", "full export")
	require.NoError(t, err)
	defer os.Remove(zipPath)

	r, err := zip.OpenReader(zipPath)
	require.NoError(t, err)
	defer r.Close()

	fileNames := make(map[string]bool)
	for _, f := range r.File {
		fileNames[f.Name] = true
	}

	// General includes everything
	assert.True(t, fileNames["system_info.json"])
	assert.True(t, fileNames["books.json"])
	assert.True(t, fileNames["authors.json"])
	assert.True(t, fileNames["series.json"])
	assert.True(t, fileNames["batch.jsonl"])
	assert.True(t, fileNames["logs.json"], "general should include logs.json")
	assert.True(t, fileNames["operations.json"], "general should include operations.json")
	assert.True(t, fileNames["version_groups.json"], "general should include version_groups.json")
	assert.True(t, fileNames["itunes_albums.json"], "general should include itunes_albums.json")
	assert.True(t, fileNames["missing_fields.json"], "general should include missing_fields.json")
}
