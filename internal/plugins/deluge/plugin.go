// file: internal/plugins/deluge/plugin.go
// version: 1.0.0

package deluge

import (
	"context"
	"fmt"

	delugeclient "github.com/jdfalk/audiobook-organizer/internal/deluge"
	"github.com/jdfalk/audiobook-organizer/internal/plugin"
)

// Plugin wraps the existing Deluge Web JSON-RPC client as a plugin.
type Plugin struct {
	client *delugeclient.Client
}

func init() { plugin.Register(&Plugin{}) }

func (p *Plugin) ID() string      { return "deluge" }
func (p *Plugin) Name() string    { return "Deluge" }
func (p *Plugin) Version() string { return "1.0.0" }

func (p *Plugin) Capabilities() []plugin.Capability {
	return []plugin.Capability{plugin.CapDownloadClient}
}

func (p *Plugin) Init(ctx context.Context, deps plugin.Deps) error {
	url := deps.Config["web_url"]
	password := deps.Config["password"]
	if url == "" {
		return fmt.Errorf("deluge: web_url is required")
	}
	client, err := delugeclient.New(url, password)
	if err != nil {
		return fmt.Errorf("deluge: failed to create client: %w", err)
	}
	p.client = client
	return nil
}

func (p *Plugin) Shutdown(ctx context.Context) error {
	p.client = nil
	return nil
}

func (p *Plugin) HealthCheck() error {
	if p.client == nil {
		return fmt.Errorf("deluge: not initialized")
	}
	connected, err := p.client.Connected()
	if err != nil {
		return fmt.Errorf("deluge: health check failed: %w", err)
	}
	if !connected {
		return fmt.Errorf("deluge: web UI not connected to daemon")
	}
	return nil
}

// --- DownloadClient interface ---

func (p *Plugin) TestConnection() error {
	if p.client == nil {
		return fmt.Errorf("deluge: not initialized")
	}
	_, err := p.client.Connected()
	return err
}

func (p *Plugin) ListTorrents() ([]plugin.TorrentInfo, error) {
	if p.client == nil {
		return nil, fmt.Errorf("deluge: not initialized")
	}
	torrents, err := p.client.ListTorrents()
	if err != nil {
		return nil, err
	}
	result := make([]plugin.TorrentInfo, 0, len(torrents))
	for _, t := range torrents {
		result = append(result, plugin.TorrentInfo{
			Hash:     t.Hash,
			Name:     t.Name,
			SavePath: t.SavePath,
			Progress: t.Progress,
			State:    t.State,
		})
	}
	return result, nil
}

func (p *Plugin) MoveStorage(torrentHash, newPath string) error {
	if p.client == nil {
		return fmt.Errorf("deluge: not initialized")
	}
	return p.client.MoveStorage([]string{torrentHash}, newPath)
}

// Ensure Plugin satisfies DownloadClient at compile time.
var _ plugin.DownloadClient = (*Plugin)(nil)
