// file: internal/download/sabnzbd.go
// version: 1.1.0
// guid: 2670e805-a4a5-4cd0-870a-fe15f09bd4e8

package download

import (
	"context"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

// SABnzbdClient implements UsenetClient for SABnzbd.
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

// GetJob returns detailed info for a Usenet job.
func (s *SABnzbdClient) GetJob(ctx context.Context, id string) (*NZBInfo, error) {
	_ = ctx
	_ = id
	return nil, ErrNotImplemented
}

// GetQueueStats returns a lightweight stats snapshot.
func (s *SABnzbdClient) GetQueueStats(ctx context.Context, id string) (*UsenetStats, error) {
	_ = ctx
	_ = id
	return nil, ErrNotImplemented
}

// SetDownloadPath moves a Usenet job to a new download directory.
func (s *SABnzbdClient) SetDownloadPath(ctx context.Context, id, newPath string) error {
	_ = ctx
	_ = id
	_ = newPath
	return ErrNotImplemented
}

// RemoveJob removes a Usenet job from the client.
func (s *SABnzbdClient) RemoveJob(ctx context.Context, id string, deleteFiles bool) error {
	_ = ctx
	_ = id
	_ = deleteFiles
	return ErrNotImplemented
}

// ListCompleted returns all completed Usenet jobs.
func (s *SABnzbdClient) ListCompleted(ctx context.Context) ([]NZBInfo, error) {
	_ = ctx
	return nil, ErrNotImplemented
}

// ClientType returns the identifier for this client.
func (s *SABnzbdClient) ClientType() string {
	return "sabnzbd"
}
