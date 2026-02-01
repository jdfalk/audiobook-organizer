// file: internal/download/sabnzbd.go
// version: 1.0.0
// guid: 2670e805-a4a5-4cd0-870a-fe15f09bd4e8

package download

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

// SABnzbdClient implements TorrentClient for SABnzbd.
type SABnzbdClient struct {
	cfg config.SABnzbdConfig
}

// NewSABnzbdClient constructs a SABnzbd client adapter.
func NewSABnzbdClient(cfg config.SABnzbdConfig) *SABnzbdClient {
	return &SABnzbdClient{cfg: cfg}
}

// Connect validates credentials and connectivity for SABnzbd.
func (s *SABnzbdClient) Connect(ctx context.Context) error {
	_ = ctx
	return ErrNotImplemented
}

// GetTorrent returns detailed info for a torrent.
func (s *SABnzbdClient) GetTorrent(ctx context.Context, id string) (*TorrentInfo, error) {
	_ = ctx
	_ = id
	return nil, ErrNotImplemented
}

// GetUploadStats returns a lightweight stats snapshot.
func (s *SABnzbdClient) GetUploadStats(ctx context.Context, id string) (*UploadStats, error) {
	_ = ctx
	_ = id
	return nil, ErrNotImplemented
}

// SetDownloadPath moves a torrent to a new download directory.
func (s *SABnzbdClient) SetDownloadPath(ctx context.Context, id, newPath string) error {
	_ = ctx
	_ = id
	_ = newPath
	return ErrNotImplemented
}

// RemoveTorrent removes a torrent from the client.
func (s *SABnzbdClient) RemoveTorrent(ctx context.Context, id string, deleteFiles bool) error {
	_ = ctx
	_ = id
	_ = deleteFiles
	return ErrNotImplemented
}

// ListCompleted returns all completed torrents.
func (s *SABnzbdClient) ListCompleted(ctx context.Context) ([]TorrentInfo, error) {
	_ = ctx
	return nil, ErrNotImplemented
}

// ClientType returns the identifier for this client.
func (s *SABnzbdClient) ClientType() string {
	return "sabnzbd"
}
