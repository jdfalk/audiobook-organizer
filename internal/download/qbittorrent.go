// file: internal/download/qbittorrent.go
// version: 1.0.0
// guid: b1275f4a-b460-48d6-9a95-ac95ac9056fb

package download

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

// QBittorrentClient implements TorrentClient for qBittorrent.
type QBittorrentClient struct {
	cfg config.QBittorrentConfig
}

// NewQBittorrentClient constructs a qBittorrent client adapter.
func NewQBittorrentClient(cfg config.QBittorrentConfig) *QBittorrentClient {
	return &QBittorrentClient{cfg: cfg}
}

// Connect validates credentials and connectivity for qBittorrent.
func (q *QBittorrentClient) Connect(ctx context.Context) error {
	_ = ctx
	return ErrNotImplemented
}

// GetTorrent returns detailed info for a torrent.
func (q *QBittorrentClient) GetTorrent(ctx context.Context, id string) (*TorrentInfo, error) {
	_ = ctx
	_ = id
	return nil, ErrNotImplemented
}

// GetUploadStats returns a lightweight stats snapshot.
func (q *QBittorrentClient) GetUploadStats(ctx context.Context, id string) (*UploadStats, error) {
	_ = ctx
	_ = id
	return nil, ErrNotImplemented
}

// SetDownloadPath moves a torrent to a new download directory.
func (q *QBittorrentClient) SetDownloadPath(ctx context.Context, id, newPath string) error {
	_ = ctx
	_ = id
	_ = newPath
	return ErrNotImplemented
}

// RemoveTorrent removes a torrent from the client.
func (q *QBittorrentClient) RemoveTorrent(ctx context.Context, id string, deleteFiles bool) error {
	_ = ctx
	_ = id
	_ = deleteFiles
	return ErrNotImplemented
}

// ListCompleted returns all completed torrents.
func (q *QBittorrentClient) ListCompleted(ctx context.Context) ([]TorrentInfo, error) {
	_ = ctx
	return nil, ErrNotImplemented
}

// ClientType returns the identifier for this client.
func (q *QBittorrentClient) ClientType() string {
	return "qbittorrent"
}
