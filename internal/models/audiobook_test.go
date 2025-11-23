// file: internal/models/audiobook_test.go
// version: 1.1.1
// guid: e5f6a7b8-9c0d-1e2f-3a4b-5c6d7e8f9a0b

package models

import (
	"encoding/json"
	"testing"
	"time"
)

// TestAuthorStruct tests the Author struct
func TestAuthorStruct(t *testing.T) {
	// Arrange
	author := Author{
		ID:   1,
		Name: "J.R.R. Tolkien",
	}

	// Act & Assert
	if author.ID != 1 {
		t.Errorf("Expected ID to be 1, got %d", author.ID)
	}

	if author.Name != "J.R.R. Tolkien" {
		t.Errorf("Expected Name to be 'J.R.R. Tolkien', got '%s'", author.Name)
	}
}

// TestAuthorJSON tests Author JSON serialization
func TestAuthorJSON(t *testing.T) {
	// Arrange
	author := Author{
		ID:   1,
		Name: "Test Author",
	}

	// Act - Marshal to JSON
	jsonData, err := json.Marshal(author)
	if err != nil {
		t.Fatalf("Failed to marshal author: %v", err)
	}

	// Unmarshal back
	var decoded Author
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal author: %v", err)
	}

	// Assert
	if decoded.ID != author.ID {
		t.Errorf("Expected ID %d, got %d", author.ID, decoded.ID)
	}

	if decoded.Name != author.Name {
		t.Errorf("Expected Name '%s', got '%s'", author.Name, decoded.Name)
	}
}

// TestSeriesStruct tests the Series struct
func TestSeriesStruct(t *testing.T) {
	// Arrange
	authorID := 1
	series := Series{
		ID:       1,
		Name:     "The Lord of the Rings",
		AuthorID: &authorID,
	}

	// Act & Assert
	if series.ID != 1 {
		t.Errorf("Expected ID to be 1, got %d", series.ID)
	}

	if series.Name != "The Lord of the Rings" {
		t.Errorf("Expected Name to be 'The Lord of the Rings', got '%s'", series.Name)
	}

	if series.AuthorID == nil || *series.AuthorID != authorID {
		t.Error("Expected AuthorID to be set to 1")
	}
}

// TestSeriesJSON tests Series JSON serialization
func TestSeriesJSON(t *testing.T) {
	// Arrange
	authorID := 1
	series := Series{
		ID:       1,
		Name:     "Test Series",
		AuthorID: &authorID,
	}

	// Act - Marshal to JSON
	jsonData, err := json.Marshal(series)
	if err != nil {
		t.Fatalf("Failed to marshal series: %v", err)
	}

	// Unmarshal back
	var decoded Series
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal series: %v", err)
	}

	// Assert
	if decoded.ID != series.ID {
		t.Errorf("Expected ID %d, got %d", series.ID, decoded.ID)
	}

	if decoded.Name != series.Name {
		t.Errorf("Expected Name '%s', got '%s'", series.Name, decoded.Name)
	}

	if decoded.AuthorID == nil || *decoded.AuthorID != *series.AuthorID {
		t.Error("AuthorID mismatch in decoded series")
	}
}

// TestAudiobookStruct tests the Audiobook struct
func TestAudiobookStruct(t *testing.T) {
	// Arrange
	authorID := 1
	seriesID := 1
	sequence := 1
	narrator := "Test Narrator"

	audiobook := Audiobook{
		ID:             1,
		Title:          "Test Audiobook",
		AuthorID:       &authorID,
		SeriesID:       &seriesID,
		SeriesSequence: &sequence,
		FilePath:       "/path/to/audiobook.mp3",
		Format:         "mp3",
		Narrator:       &narrator,
	}

	// Act & Assert
	if audiobook.ID != 1 {
		t.Errorf("Expected ID to be 1, got %d", audiobook.ID)
	}

	if audiobook.Title != "Test Audiobook" {
		t.Errorf("Expected Title to be 'Test Audiobook', got '%s'", audiobook.Title)
	}

	if audiobook.AuthorID == nil || *audiobook.AuthorID != authorID {
		t.Error("Expected AuthorID to be set")
	}

	if audiobook.Narrator == nil || *audiobook.Narrator != narrator {
		t.Error("Expected Narrator to be set")
	}
}

// TestAudiobookJSON tests Audiobook JSON serialization
func TestAudiobookJSON(t *testing.T) {
	// Arrange
	authorID := 1
	duration := 3600
	audiobook := Audiobook{
		ID:       1,
		Title:    "Test Book",
		AuthorID: &authorID,
		FilePath: "/test/book.mp3",
		Format:   "mp3",
		Duration: &duration,
	}

	// Act - Marshal to JSON
	jsonData, err := json.Marshal(audiobook)
	if err != nil {
		t.Fatalf("Failed to marshal audiobook: %v", err)
	}

	// Unmarshal back
	var decoded Audiobook
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal audiobook: %v", err)
	}

	// Assert
	if decoded.ID != audiobook.ID {
		t.Errorf("Expected ID %d, got %d", audiobook.ID, decoded.ID)
	}

	if decoded.Title != audiobook.Title {
		t.Errorf("Expected Title '%s', got '%s'", audiobook.Title, decoded.Title)
	}

	if decoded.Duration == nil || *decoded.Duration != duration {
		t.Error("Duration mismatch in decoded audiobook")
	}
}

// TestAudiobookVersionManagement tests version-related fields
func TestAudiobookVersionManagement(t *testing.T) {
	// Arrange
	isPrimary := true
	groupID := "version-group-123"
	notes := "Remastered 2024"

	audiobook := Audiobook{
		ID:               1,
		Title:            "Test Book",
		FilePath:         "/test/book.mp3",
		IsPrimaryVersion: &isPrimary,
		VersionGroupID:   &groupID,
		VersionNotes:     &notes,
	}

	// Act & Assert
	if audiobook.IsPrimaryVersion == nil || !*audiobook.IsPrimaryVersion {
		t.Error("Expected IsPrimaryVersion to be true")
	}

	if audiobook.VersionGroupID == nil || *audiobook.VersionGroupID != groupID {
		t.Error("Expected VersionGroupID to be set")
	}

	if audiobook.VersionNotes == nil || *audiobook.VersionNotes != notes {
		t.Error("Expected VersionNotes to be set")
	}
}

// TestAudiobookListRequest tests the request struct
func TestAudiobookListRequest(t *testing.T) {
	// Arrange
	req := AudiobookListRequest{
		Page:    1,
		Limit:   50,
		Search:  "tolkien",
		Author:  "J.R.R. Tolkien",
		SortBy:  "title",
		SortDir: "asc",
	}

	// Act & Assert
	if req.Page != 1 {
		t.Errorf("Expected Page to be 1, got %d", req.Page)
	}

	if req.Limit != 50 {
		t.Errorf("Expected Limit to be 50, got %d", req.Limit)
	}

	if req.Search != "tolkien" {
		t.Errorf("Expected Search to be 'tolkien', got '%s'", req.Search)
	}

	if req.SortBy != "title" {
		t.Errorf("Expected SortBy to be 'title', got '%s'", req.SortBy)
	}
}

// TestAudiobookListResponse tests the response struct
func TestAudiobookListResponse(t *testing.T) {
	// Arrange
	audiobooks := []Audiobook{
		{ID: 1, Title: "Book 1", FilePath: "/path1.mp3"},
		{ID: 2, Title: "Book 2", FilePath: "/path2.mp3"},
	}

	resp := AudiobookListResponse{
		Audiobooks: audiobooks,
		Total:      100,
		Page:       1,
		Limit:      50,
		Pages:      2,
	}

	// Act & Assert
	if len(resp.Audiobooks) != 2 {
		t.Errorf("Expected 2 audiobooks, got %d", len(resp.Audiobooks))
	}

	if resp.Total != 100 {
		t.Errorf("Expected Total to be 100, got %d", resp.Total)
	}

	if resp.Pages != 2 {
		t.Errorf("Expected Pages to be 2, got %d", resp.Pages)
	}
}

// TestAudiobookUpdateRequest tests the update request struct
func TestAudiobookUpdateRequest(t *testing.T) {
	// Arrange
	title := "Updated Title"
	author := "New Author"
	duration := 7200

	req := AudiobookUpdateRequest{
		Title:    &title,
		Author:   &author,
		Duration: &duration,
	}

	// Act & Assert
	if req.Title == nil || *req.Title != title {
		t.Error("Expected Title to be set")
	}

	if req.Author == nil || *req.Author != author {
		t.Error("Expected Author to be set")
	}

	if req.Duration == nil || *req.Duration != duration {
		t.Error("Expected Duration to be set")
	}
}

// TestBatchUpdateRequest tests the batch update request struct
func TestBatchUpdateRequest(t *testing.T) {
	// Arrange
	author := "Batch Author"
	req := BatchUpdateRequest{
		AudiobookIDs: []int{1, 2, 3},
		Updates: AudiobookUpdateRequest{
			Author: &author,
		},
	}

	// Act & Assert
	if len(req.AudiobookIDs) != 3 {
		t.Errorf("Expected 3 audiobook IDs, got %d", len(req.AudiobookIDs))
	}

	if req.Updates.Author == nil || *req.Updates.Author != author {
		t.Error("Expected Author to be set in Updates")
	}
}

// TestFileSystemItem tests the filesystem item struct
func TestFileSystemItem(t *testing.T) {
	// Arrange
	modTime := time.Now()
	item := FileSystemItem{
		Name:           "test.mp3",
		Path:           "/path/to/test.mp3",
		IsDirectory:    false,
		Size:           1024000,
		ModTime:        modTime,
		IsExcluded:     false,
		AudiobookCount: 1,
	}

	// Act & Assert
	if item.Name != "test.mp3" {
		t.Errorf("Expected Name to be 'test.mp3', got '%s'", item.Name)
	}

	if item.IsDirectory {
		t.Error("Expected IsDirectory to be false")
	}

	if item.Size != 1024000 {
		t.Errorf("Expected Size to be 1024000, got %d", item.Size)
	}

	if item.AudiobookCount != 1 {
		t.Errorf("Expected AudiobookCount to be 1, got %d", item.AudiobookCount)
	}
}

// TestBrowseRequest tests the browse request struct
func TestBrowseRequest(t *testing.T) {
	// Arrange
	req := BrowseRequest{
		Path: "/media/audiobooks",
	}

	// Act & Assert
	if req.Path != "/media/audiobooks" {
		t.Errorf("Expected Path to be '/media/audiobooks', got '%s'", req.Path)
	}
}

// TestExclusionRequest tests the exclusion request struct
func TestExclusionRequest(t *testing.T) {
	// Arrange
	req := ExclusionRequest{
		Path:   "/media/audiobooks/excluded",
		Reason: "Test exclusion",
	}

	// Act & Assert
	if req.Path != "/media/audiobooks/excluded" {
		t.Errorf("Expected Path to be '/media/audiobooks/excluded', got '%s'", req.Path)
	}

	if req.Reason != "Test exclusion" {
		t.Errorf("Expected Reason to be 'Test exclusion', got '%s'", req.Reason)
	}
}

// TestSystemStatus tests the system status struct
func TestSystemStatus(t *testing.T) {
	// Arrange
	diskUsage := map[string]int64{
		"total": 1000000000,
		"used":  500000000,
		"free":  500000000,
	}

	status := SystemStatus{
		Version:          "1.0.0",
		Uptime:           "2h30m",
		DatabasePath:     "/data/audiobooks.db",
		TotalBooks:       1000,
		TotalAuthors:     200,
		TotalSeries:      50,
		ImportPaths:      3,
		ActiveOperations: 0,
		DiskUsage:        diskUsage,
	}

	// Act & Assert
	if status.Version != "1.0.0" {
		t.Errorf("Expected Version to be '1.0.0', got '%s'", status.Version)
	}

	if status.TotalBooks != 1000 {
		t.Errorf("Expected TotalBooks to be 1000, got %d", status.TotalBooks)
	}

	if len(status.DiskUsage) != 3 {
		t.Errorf("Expected 3 disk usage entries, got %d", len(status.DiskUsage))
	}
}

// TestLogEntry tests the log entry struct
func TestLogEntry(t *testing.T) {
	// Arrange
	timestamp := time.Now()
	entry := LogEntry{
		Level:     "info",
		Timestamp: timestamp,
		Message:   "Test log message",
		Source:    "test",
	}

	// Act & Assert
	if entry.Level != "info" {
		t.Errorf("Expected Level to be 'info', got '%s'", entry.Level)
	}

	if entry.Message != "Test log message" {
		t.Errorf("Expected Message to be 'Test log message', got '%s'", entry.Message)
	}

	if entry.Source != "test" {
		t.Errorf("Expected Source to be 'test', got '%s'", entry.Source)
	}
}

// TestAudiobookMediaInfo tests media info fields
func TestAudiobookMediaInfo(t *testing.T) {
	// Arrange
	bitrate := 128
	codec := "AAC"
	sampleRate := 44100
	channels := 2
	quality := "Standard"

	audiobook := Audiobook{
		ID:         1,
		Title:      "Test Book",
		FilePath:   "/test/book.mp3",
		Bitrate:    &bitrate,
		Codec:      &codec,
		SampleRate: &sampleRate,
		Channels:   &channels,
		Quality:    &quality,
	}

	// Act & Assert
	if audiobook.Bitrate == nil || *audiobook.Bitrate != bitrate {
		t.Error("Expected Bitrate to be set")
	}

	if audiobook.Codec == nil || *audiobook.Codec != codec {
		t.Error("Expected Codec to be set")
	}

	if audiobook.SampleRate == nil || *audiobook.SampleRate != sampleRate {
		t.Error("Expected SampleRate to be set")
	}

	if audiobook.Channels == nil || *audiobook.Channels != channels {
		t.Error("Expected Channels to be set")
	}

	if audiobook.Quality == nil || *audiobook.Quality != quality {
		t.Error("Expected Quality to be set")
	}
}
