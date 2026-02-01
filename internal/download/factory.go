// file: internal/download/factory.go
// version: 1.1.0
// guid: be6a33cc-3062-42b7-b395-1892d8829540

package download

import (
	"fmt"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

// NewTorrentClientFromConfig builds a torrent client from application configuration.
func NewTorrentClientFromConfig(cfg *config.Config) (TorrentClient, error) {
	switch cfg.DownloadClient.Torrent.Type {
	case "deluge":
		return NewDelugeClient(cfg.DownloadClient.Torrent.Deluge), nil
	case "qbittorrent":
		return NewQBittorrentClient(cfg.DownloadClient.Torrent.QBittorrent), nil
	case "":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported torrent client type: %s", cfg.DownloadClient.Torrent.Type)
	}
}

// NewUsenetClientFromConfig builds a Usenet client from application configuration.
func NewUsenetClientFromConfig(cfg *config.Config) (UsenetClient, error) {
	switch cfg.DownloadClient.Usenet.Type {
	case "sabnzbd":
		return NewSABnzbdClient(cfg.DownloadClient.Usenet.SABnzbd), nil
	case "":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported usenet client type: %s", cfg.DownloadClient.Usenet.Type)
	}
}
