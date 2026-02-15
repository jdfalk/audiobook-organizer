// file: internal/download/qbittorrent.go
// version: 1.1.0
// guid: b1275f4a-b460-48d6-9a95-ac95ac9056fb

package download

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

// QBittorrentClient implements TorrentClient for qBittorrent via Web API v2.
type QBittorrentClient struct {
	cfg     config.QBittorrentConfig
	client  *http.Client
	baseURL string
}

// NewQBittorrentClient constructs a qBittorrent client adapter.
func NewQBittorrentClient(cfg config.QBittorrentConfig) *QBittorrentClient {
	scheme := "http"
	if cfg.UseHTTPS {
		scheme = "https"
	}
	return &QBittorrentClient{
		cfg:     cfg,
		baseURL: fmt.Sprintf("%s://%s:%d", scheme, cfg.Host, cfg.Port),
	}
}

// Connect authenticates with the qBittorrent Web API.
func (q *QBittorrentClient) Connect(ctx context.Context) error {
	jar, _ := cookiejar.New(nil)
	q.client = &http.Client{Jar: jar, Timeout: 30 * time.Second}

	data := url.Values{"username": {q.cfg.Username}, "password": {q.cfg.Password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.baseURL+"/api/v2/auth/login", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("qbittorrent: failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("qbittorrent: connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qbittorrent: login failed with status %d", resp.StatusCode)
	}
	return nil
}

type qbTorrent struct {
	Hash          string  `json:"hash"`
	Name          string  `json:"name"`
	SavePath      string  `json:"save_path"`
	State         string  `json:"state"`
	Progress      float64 `json:"progress"`
	Uploaded      int64   `json:"uploaded"`
	Downloaded    int64   `json:"downloaded"`
	AddedOn       int64   `json:"added_on"`
	DlSpeed       int64   `json:"dlspeed"`
	UpSpeed       int64   `json:"upspeed"`
	ContentPath   string  `json:"content_path"`
}

func (q *QBittorrentClient) fetchTorrents(ctx context.Context, params url.Values) ([]qbTorrent, error) {
	reqURL := q.baseURL + "/api/v2/torrents/info?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qbittorrent: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qbittorrent: API returned status %d", resp.StatusCode)
	}

	var torrents []qbTorrent
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return nil, fmt.Errorf("qbittorrent: failed to parse response: %w", err)
	}
	return torrents, nil
}

func mapQBState(state string) (TorrentStatus, bool) {
	switch state {
	case "downloading", "stalledDL", "metaDL", "forcedDL", "allocating":
		return StatusDownloading, false
	case "uploading", "stalledUP", "forcedUP", "checkingUP":
		return StatusSeeding, false
	case "pausedDL", "pausedUP":
		return StatusPaused, true
	case "stoppedDL", "stoppedUP", "checkingResumeData":
		return StatusStopped, false
	case "missingFiles", "error":
		return StatusNotFound, false
	default:
		return StatusDownloading, false
	}
}

func qbToTorrentInfo(t qbTorrent) TorrentInfo {
	status, paused := mapQBState(t.State)
	return TorrentInfo{
		ID:              t.Hash,
		Name:            t.Name,
		DownloadDir:     t.SavePath,
		Status:          status,
		Progress:        t.Progress,
		TotalUploaded:   t.Uploaded,
		TotalDownloaded: t.Downloaded,
		CreatedAt:       time.Unix(t.AddedOn, 0),
		IsPaused:        paused,
	}
}

// GetTorrent returns detailed info for a single torrent.
func (q *QBittorrentClient) GetTorrent(ctx context.Context, id string) (*TorrentInfo, error) {
	torrents, err := q.fetchTorrents(ctx, url.Values{"hashes": {id}})
	if err != nil {
		return nil, err
	}
	if len(torrents) == 0 {
		return nil, nil
	}
	info := qbToTorrentInfo(torrents[0])

	// Fetch file list
	filesURL := q.baseURL + "/api/v2/torrents/files?" + url.Values{"hash": {id}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, filesURL, nil)
	if err == nil {
		if resp, err := q.client.Do(req); err == nil {
			defer resp.Body.Close()
			var files []struct {
				Name string `json:"name"`
				Size int64  `json:"size"`
			}
			if json.NewDecoder(resp.Body).Decode(&files) == nil {
				for _, f := range files {
					info.Files = append(info.Files, TorrentFile{Path: f.Name, Size: f.Size})
				}
			}
		}
	}
	return &info, nil
}

// GetUploadStats returns lightweight stats for cleanup monitoring.
func (q *QBittorrentClient) GetUploadStats(ctx context.Context, id string) (*UploadStats, error) {
	torrents, err := q.fetchTorrents(ctx, url.Values{"hashes": {id}})
	if err != nil {
		return nil, err
	}
	if len(torrents) == 0 {
		return &UploadStats{Exists: false}, nil
	}
	_, paused := mapQBState(torrents[0].State)
	return &UploadStats{
		TotalUploaded: torrents[0].Uploaded,
		IsPaused:      paused,
		Exists:        true,
	}, nil
}

// SetDownloadPath moves a torrent to a new download directory.
func (q *QBittorrentClient) SetDownloadPath(ctx context.Context, id, newPath string) error {
	data := url.Values{"hashes": {id}, "location": {newPath}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.baseURL+"/api/v2/torrents/setLocation", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("qbittorrent: setLocation failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qbittorrent: setLocation returned status %d", resp.StatusCode)
	}
	return nil
}

// RemoveTorrent removes a torrent from the client.
func (q *QBittorrentClient) RemoveTorrent(ctx context.Context, id string, deleteFiles bool) error {
	deleteStr := "false"
	if deleteFiles {
		deleteStr = "true"
	}
	data := url.Values{"hashes": {id}, "deleteFiles": {deleteStr}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.baseURL+"/api/v2/torrents/delete", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("qbittorrent: delete failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qbittorrent: delete returned status %d", resp.StatusCode)
	}
	return nil
}

// ListCompleted returns all completed torrents.
func (q *QBittorrentClient) ListCompleted(ctx context.Context) ([]TorrentInfo, error) {
	torrents, err := q.fetchTorrents(ctx, url.Values{"filter": {"completed"}})
	if err != nil {
		return nil, err
	}
	result := make([]TorrentInfo, len(torrents))
	for i, t := range torrents {
		result[i] = qbToTorrentInfo(t)
	}
	return result, nil
}

// ClientType returns the identifier for this client.
func (q *QBittorrentClient) ClientType() string {
	return "qbittorrent"
}
