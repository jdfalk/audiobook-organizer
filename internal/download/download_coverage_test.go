// file: internal/download/download_coverage_test.go
// version: 1.0.0

package download

import (
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

// --- Coverage for mapping functions ---

func TestCoverage_mapDelugeState(t *testing.T) {
	tests := []struct {
		name   string
		state  string
		paused bool
		want   TorrentStatus
	}{
		{"downloading", "Downloading", false, StatusDownloading},
		{"checking", "Checking", false, StatusDownloading},
		{"seeding", "Seeding", false, StatusSeeding},
		{"paused state", "Paused", false, StatusPaused},
		{"paused flag", "Downloading", true, StatusPaused},
		{"unknown state", "SomethingElse", false, StatusStopped},
		{"empty state", "", false, StatusStopped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapDelugeState(tt.state, tt.paused)
			if got != tt.want {
				t.Errorf("mapDelugeState(%q, %v) = %d, want %d", tt.state, tt.paused, got, tt.want)
			}
		})
	}
}

func TestCoverage_delugeToTorrentInfo(t *testing.T) {
	dt := delugeTorrent{
		Name:          "Test Torrent",
		SavePath:      "/downloads",
		State:         "Seeding",
		Progress:      100.0,
		TotalUploaded: 5000,
		TotalSize:     10000,
		TimeAdded:     1700000000,
		Paused:        false,
		Files: []struct {
			Path string `json:"path"`
			Size int64  `json:"size"`
		}{
			{Path: "file1.mp3", Size: 1000},
			{Path: "file2.mp3", Size: 2000},
		},
	}

	info := delugeToTorrentInfo("hash123", dt)
	if info.ID != "hash123" {
		t.Errorf("ID = %q, want 'hash123'", info.ID)
	}
	if info.Name != "Test Torrent" {
		t.Errorf("Name = %q, want 'Test Torrent'", info.Name)
	}
	if info.DownloadDir != "/downloads" {
		t.Errorf("DownloadDir = %q, want '/downloads'", info.DownloadDir)
	}
	if info.Status != StatusSeeding {
		t.Errorf("Status = %d, want StatusSeeding", info.Status)
	}
	if info.Progress != 1.0 {
		t.Errorf("Progress = %f, want 1.0", info.Progress)
	}
	if len(info.Files) != 2 {
		t.Errorf("Files count = %d, want 2", len(info.Files))
	}
	if info.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestCoverage_mapQBState(t *testing.T) {
	tests := []struct {
		name       string
		state      string
		wantStatus TorrentStatus
		wantPaused bool
	}{
		{"downloading", "downloading", StatusDownloading, false},
		{"stalledDL", "stalledDL", StatusDownloading, false},
		{"metaDL", "metaDL", StatusDownloading, false},
		{"forcedDL", "forcedDL", StatusDownloading, false},
		{"allocating", "allocating", StatusDownloading, false},
		{"uploading", "uploading", StatusSeeding, false},
		{"stalledUP", "stalledUP", StatusSeeding, false},
		{"forcedUP", "forcedUP", StatusSeeding, false},
		{"checkingUP", "checkingUP", StatusSeeding, false},
		{"pausedDL", "pausedDL", StatusPaused, true},
		{"pausedUP", "pausedUP", StatusPaused, true},
		{"stoppedDL", "stoppedDL", StatusStopped, false},
		{"stoppedUP", "stoppedUP", StatusStopped, false},
		{"checkingResumeData", "checkingResumeData", StatusStopped, false},
		{"missingFiles", "missingFiles", StatusNotFound, false},
		{"error", "error", StatusNotFound, false},
		{"unknown", "unknown_state", StatusDownloading, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotPaused := mapQBState(tt.state)
			if gotStatus != tt.wantStatus {
				t.Errorf("mapQBState(%q) status = %d, want %d", tt.state, gotStatus, tt.wantStatus)
			}
			if gotPaused != tt.wantPaused {
				t.Errorf("mapQBState(%q) paused = %v, want %v", tt.state, gotPaused, tt.wantPaused)
			}
		})
	}
}

func TestCoverage_qbToTorrentInfo(t *testing.T) {
	qt := qbTorrent{
		Hash:        "abc123",
		Name:        "Test QB Torrent",
		SavePath:    "/downloads/qbt",
		State:       "uploading",
		Progress:    1.0,
		Uploaded:    50000,
		Downloaded:  100000,
		AddedOn:     1700000000,
		ContentPath: "/downloads/qbt/test",
	}

	info := qbToTorrentInfo(qt)
	if info.ID != "abc123" {
		t.Errorf("ID = %q, want 'abc123'", info.ID)
	}
	if info.Name != "Test QB Torrent" {
		t.Errorf("Name = %q, want 'Test QB Torrent'", info.Name)
	}
	if info.Status != StatusSeeding {
		t.Errorf("Status = %d, want StatusSeeding", info.Status)
	}
	if info.IsPaused {
		t.Error("should not be paused")
	}
}

func TestCoverage_mapSABStatus(t *testing.T) {
	tests := []struct {
		status string
		want   UsenetStatus
	}{
		{"Queued", UsenetStatusQueued},
		{"Downloading", UsenetStatusDownloading},
		{"Extracting", UsenetStatusDownloading},
		{"Verifying", UsenetStatusDownloading},
		{"Repairing", UsenetStatusDownloading},
		{"Completed", UsenetStatusCompleted},
		{"Paused", UsenetStatusPaused},
		{"Failed", UsenetStatusFailed},
		{"Unknown", UsenetStatusQueued},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := mapSABStatus(tt.status)
			if got != tt.want {
				t.Errorf("mapSABStatus(%q) = %d, want %d", tt.status, got, tt.want)
			}
		})
	}
}

func TestCoverage_sabSlotToNZBInfo(t *testing.T) {
	slot := sabSlot{
		NzoID:      "nzo-123",
		Filename:   "Test NZB",
		Storage:    "/downloads/nzb",
		Status:     "Completed",
		Percentage: "100",
	}

	info := sabSlotToNZBInfo(slot)
	if info.ID != "nzo-123" {
		t.Errorf("ID = %q, want 'nzo-123'", info.ID)
	}
	if info.Name != "Test NZB" {
		t.Errorf("Name = %q, want 'Test NZB'", info.Name)
	}
	if info.Status != UsenetStatusCompleted {
		t.Errorf("Status = %d, want UsenetStatusCompleted", info.Status)
	}
	if info.IsPaused {
		t.Error("should not be paused")
	}
}

func TestCoverage_sabSlotToNZBInfo_Paused(t *testing.T) {
	slot := sabSlot{
		NzoID:      "nzo-456",
		Filename:   "Paused NZB",
		Status:     "Paused",
		Percentage: "50",
	}

	info := sabSlotToNZBInfo(slot)
	if !info.IsPaused {
		t.Error("should be paused")
	}
}

// --- Coverage for constructor functions ---

func TestCoverage_NewDelugeClient(t *testing.T) {
	cfg := config.DelugeConfig{
		Host:     "remote.host",
		Port:     9999,
		Username: "user",
		Password: "pass",
	}
	client := NewDelugeClient(cfg)
	if client.baseURL != "http://remote.host:9999/json" {
		t.Errorf("baseURL = %q", client.baseURL)
	}
}

func TestCoverage_NewQBittorrentClient_HTTPS(t *testing.T) {
	cfg := config.QBittorrentConfig{
		Host:     "secure.host",
		Port:     8443,
		UseHTTPS: true,
	}
	client := NewQBittorrentClient(cfg)
	if client.baseURL != "https://secure.host:8443" {
		t.Errorf("baseURL = %q, want 'https://secure.host:8443'", client.baseURL)
	}
}

func TestCoverage_NewSABnzbdClient_HTTPS(t *testing.T) {
	cfg := config.SABnzbdConfig{
		Host:     "sab.host",
		Port:     9090,
		UseHTTPS: true,
	}
	client := NewSABnzbdClient(cfg)
	if client.baseURL != "https://sab.host:9090/api" {
		t.Errorf("baseURL = %q, want 'https://sab.host:9090/api'", client.baseURL)
	}
}

// --- Coverage for struct fields ---

func TestCoverage_TorrentInfo_Fields(t *testing.T) {
	info := TorrentInfo{
		ID:              "id-1",
		Name:            "test",
		DownloadDir:     "/tmp",
		Status:          StatusSeeding,
		Progress:        0.5,
		TotalUploaded:   1000,
		TotalDownloaded: 2000,
		Files:           []TorrentFile{{Path: "a.mp3", Size: 100}},
		CreatedAt:       time.Now(),
		IsPaused:        false,
	}
	if info.Files[0].Path != "a.mp3" {
		t.Error("TorrentFile path not set")
	}
}

func TestCoverage_NZBInfo_Fields(t *testing.T) {
	info := NZBInfo{
		ID:          "nzb-1",
		Name:        "test",
		DownloadDir: "/tmp",
		Status:      UsenetStatusCompleted,
		Progress:    1.0,
		TotalBytes:  5000,
		Files:       []NZBFile{{Path: "a.rar", Size: 100}},
		CreatedAt:   time.Now(),
		IsPaused:    false,
	}
	if info.Files[0].Path != "a.rar" {
		t.Error("NZBFile path not set")
	}
}

func TestCoverage_UploadStats(t *testing.T) {
	stats := UploadStats{
		TotalUploaded: 5000,
		IsPaused:      true,
		Exists:        true,
	}
	if !stats.Exists {
		t.Error("should exist")
	}
}

func TestCoverage_UsenetStats(t *testing.T) {
	stats := UsenetStats{
		TotalDownloaded: 3000,
		IsPaused:        false,
		Exists:          false,
	}
	if stats.Exists {
		t.Error("should not exist")
	}
}
