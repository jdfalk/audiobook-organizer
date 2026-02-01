// file: internal/download/factory.go
// version: 1.0.0
// guid: be6a33cc-3062-42b7-b395-1892d8829540

package download

import (
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

// NewClientFromConfig builds a torrent client from application configuration.
func NewClientFromConfig(cfg *config.Config) (TorrentClient, error) {
	switch cfg.DownloadClient.Type {
	case "deluge":
		return NewDelugeClient(cfg.DownloadClient.Deluge), nil
	case "qbittorrent":
		return NewQBittorrentClient(cfg.DownloadClient.QBittorrent), nil
	case "sabnzbd":
		return NewSABnzbdClient(cfg.DownloadClient.SABnzbd), nil
	case "":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported download client type: %s", cfg.DownloadClient.Type)
	}
}
