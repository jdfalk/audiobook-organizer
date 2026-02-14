// file: internal/download/download_test.go
// version: 1.0.0

package download

import (
	"context"
	"errors"
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/config"
)

// TestTorrentClientInterface verifies that all torrent client implementations
// satisfy the TorrentClient interface.
func TestTorrentClientInterface(t *testing.T) {
	tests := []struct {
		name   string
		client TorrentClient
	}{
		{
			name:   "DelugeClient",
			client: &DelugeClient{},
		},
		{
			name:   "QBittorrentClient",
			client: &QBittorrentClient{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just checking that the type assertions work
			if tt.client == nil {
				t.Error("client should not be nil")
			}
		})
	}
}

// TestUsenetClientInterface verifies that all Usenet client implementations
// satisfy the UsenetClient interface.
func TestUsenetClientInterface(t *testing.T) {
	tests := []struct {
		name   string
		client UsenetClient
	}{
		{
			name:   "SABnzbdClient",
			client: &SABnzbdClient{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just checking that the type assertions work
			if tt.client == nil {
				t.Error("client should not be nil")
			}
		})
	}
}

// TestNewTorrentClientFromConfig tests the torrent client factory function.
func TestNewTorrentClientFromConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		wantType    string
		wantErr     bool
		wantNil     bool
		errContains string
	}{
		{
			name: "deluge client",
			cfg: &config.Config{
				DownloadClient: config.DownloadClientConfig{
					Torrent: config.TorrentClientConfig{
						Type: "deluge",
						Deluge: config.DelugeConfig{
							Host:     "localhost",
							Port:     8112,
							Password: "deluge",
						},
					},
				},
			},
			wantType: "deluge",
			wantErr:  false,
		},
		{
			name: "qbittorrent client",
			cfg: &config.Config{
				DownloadClient: config.DownloadClientConfig{
					Torrent: config.TorrentClientConfig{
						Type: "qbittorrent",
						QBittorrent: config.QBittorrentConfig{
							Host:     "localhost",
							Port:     8080,
							Username: "admin",
							Password: "admin",
						},
					},
				},
			},
			wantType: "qbittorrent",
			wantErr:  false,
		},
		{
			name: "empty type returns nil",
			cfg: &config.Config{
				DownloadClient: config.DownloadClientConfig{
					Torrent: config.TorrentClientConfig{
						Type: "",
					},
				},
			},
			wantNil: true,
			wantErr: false,
		},
		{
			name: "unsupported type",
			cfg: &config.Config{
				DownloadClient: config.DownloadClientConfig{
					Torrent: config.TorrentClientConfig{
						Type: "transmission",
					},
				},
			},
			wantErr:     true,
			errContains: "unsupported torrent client type: transmission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewTorrentClientFromConfig(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if tt.errContains != "" && err.Error() != tt.errContains {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.wantNil {
				if client != nil {
					t.Errorf("expected nil client, got %T", client)
				}
				return
			}

			if client == nil {
				t.Error("expected non-nil client")
				return
			}

			if got := client.ClientType(); got != tt.wantType {
				t.Errorf("expected client type %q, got %q", tt.wantType, got)
			}
		})
	}
}

// TestNewUsenetClientFromConfig tests the Usenet client factory function.
func TestNewUsenetClientFromConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		wantType    string
		wantErr     bool
		wantNil     bool
		errContains string
	}{
		{
			name: "sabnzbd client",
			cfg: &config.Config{
				DownloadClient: config.DownloadClientConfig{
					Usenet: config.UsenetClientConfig{
						Type: "sabnzbd",
						SABnzbd: config.SABnzbdConfig{
							Host:   "localhost",
							Port:   8085,
							APIKey: "test-api-key",
						},
					},
				},
			},
			wantType: "sabnzbd",
			wantErr:  false,
		},
		{
			name: "empty type returns nil",
			cfg: &config.Config{
				DownloadClient: config.DownloadClientConfig{
					Usenet: config.UsenetClientConfig{
						Type: "",
					},
				},
			},
			wantNil: true,
			wantErr: false,
		},
		{
			name: "unsupported type",
			cfg: &config.Config{
				DownloadClient: config.DownloadClientConfig{
					Usenet: config.UsenetClientConfig{
						Type: "nzbget",
					},
				},
			},
			wantErr:     true,
			errContains: "unsupported usenet client type: nzbget",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewUsenetClientFromConfig(tt.cfg)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if tt.errContains != "" && err.Error() != tt.errContains {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.wantNil {
				if client != nil {
					t.Errorf("expected nil client, got %T", client)
				}
				return
			}

			if client == nil {
				t.Error("expected non-nil client")
				return
			}

			if got := client.ClientType(); got != tt.wantType {
				t.Errorf("expected client type %q, got %q", tt.wantType, got)
			}
		})
	}
}

// TestErrNotImplemented verifies the error constant.
func TestErrNotImplemented(t *testing.T) {
	if ErrNotImplemented == nil {
		t.Error("ErrNotImplemented should not be nil")
	}

	expected := "download client not implemented"
	if ErrNotImplemented.Error() != expected {
		t.Errorf("expected error message %q, got %q", expected, ErrNotImplemented.Error())
	}

	// Verify it's a sentinel error that can be compared
	err := ErrNotImplemented
	if !errors.Is(err, ErrNotImplemented) {
		t.Error("errors.Is should recognize ErrNotImplemented")
	}
}

// TestDelugeClientStub tests that the Deluge client stub behaves correctly.
func TestDelugeClientStub(t *testing.T) {
	cfg := config.DelugeConfig{
		Host:     "localhost",
		Port:     8112,
		Password: "deluge",
	}
	client := NewDelugeClient(cfg)

	if client == nil {
		t.Fatal("NewDelugeClient returned nil")
	}

	if got := client.ClientType(); got != "deluge" {
		t.Errorf("expected client type 'deluge', got %q", got)
	}

	ctx := context.Background()

	// All methods should return ErrNotImplemented
	t.Run("Connect", func(t *testing.T) {
		err := client.Connect(ctx)
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
	})

	t.Run("GetTorrent", func(t *testing.T) {
		info, err := client.GetTorrent(ctx, "test-id")
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
		if info != nil {
			t.Errorf("expected nil info, got %v", info)
		}
	})

	t.Run("GetUploadStats", func(t *testing.T) {
		stats, err := client.GetUploadStats(ctx, "test-id")
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
		if stats != nil {
			t.Errorf("expected nil stats, got %v", stats)
		}
	})

	t.Run("SetDownloadPath", func(t *testing.T) {
		err := client.SetDownloadPath(ctx, "test-id", "/new/path")
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
	})

	t.Run("RemoveTorrent", func(t *testing.T) {
		err := client.RemoveTorrent(ctx, "test-id", true)
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
	})

	t.Run("ListCompleted", func(t *testing.T) {
		torrents, err := client.ListCompleted(ctx)
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
		if torrents != nil {
			t.Errorf("expected nil torrents, got %v", torrents)
		}
	})
}

// TestQBittorrentClientStub tests that the qBittorrent client stub behaves correctly.
func TestQBittorrentClientStub(t *testing.T) {
	cfg := config.QBittorrentConfig{
		Host:     "localhost",
		Port:     8080,
		Username: "admin",
		Password: "admin",
	}
	client := NewQBittorrentClient(cfg)

	if client == nil {
		t.Fatal("NewQBittorrentClient returned nil")
	}

	if got := client.ClientType(); got != "qbittorrent" {
		t.Errorf("expected client type 'qbittorrent', got %q", got)
	}

	ctx := context.Background()

	// All methods should return ErrNotImplemented
	t.Run("Connect", func(t *testing.T) {
		err := client.Connect(ctx)
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
	})

	t.Run("GetTorrent", func(t *testing.T) {
		info, err := client.GetTorrent(ctx, "test-id")
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
		if info != nil {
			t.Errorf("expected nil info, got %v", info)
		}
	})

	t.Run("GetUploadStats", func(t *testing.T) {
		stats, err := client.GetUploadStats(ctx, "test-id")
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
		if stats != nil {
			t.Errorf("expected nil stats, got %v", stats)
		}
	})

	t.Run("SetDownloadPath", func(t *testing.T) {
		err := client.SetDownloadPath(ctx, "test-id", "/new/path")
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
	})

	t.Run("RemoveTorrent", func(t *testing.T) {
		err := client.RemoveTorrent(ctx, "test-id", true)
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
	})

	t.Run("ListCompleted", func(t *testing.T) {
		torrents, err := client.ListCompleted(ctx)
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
		if torrents != nil {
			t.Errorf("expected nil torrents, got %v", torrents)
		}
	})
}

// TestSABnzbdClientStub tests that the SABnzbd client stub behaves correctly.
func TestSABnzbdClientStub(t *testing.T) {
	cfg := config.SABnzbdConfig{
		Host:   "localhost",
		Port:   8085,
		APIKey: "test-api-key",
	}
	client := NewSABnzbdClient(cfg)

	if client == nil {
		t.Fatal("NewSABnzbdClient returned nil")
	}

	if got := client.ClientType(); got != "sabnzbd" {
		t.Errorf("expected client type 'sabnzbd', got %q", got)
	}

	ctx := context.Background()

	// All methods should return ErrNotImplemented
	t.Run("Connect", func(t *testing.T) {
		err := client.Connect(ctx)
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
	})

	t.Run("GetJob", func(t *testing.T) {
		info, err := client.GetJob(ctx, "test-id")
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
		if info != nil {
			t.Errorf("expected nil info, got %v", info)
		}
	})

	t.Run("GetQueueStats", func(t *testing.T) {
		stats, err := client.GetQueueStats(ctx, "test-id")
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
		if stats != nil {
			t.Errorf("expected nil stats, got %v", stats)
		}
	})

	t.Run("SetDownloadPath", func(t *testing.T) {
		err := client.SetDownloadPath(ctx, "test-id", "/new/path")
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
	})

	t.Run("RemoveJob", func(t *testing.T) {
		err := client.RemoveJob(ctx, "test-id", true)
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
	})

	t.Run("ListCompleted", func(t *testing.T) {
		jobs, err := client.ListCompleted(ctx)
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}
		if jobs != nil {
			t.Errorf("expected nil jobs, got %v", jobs)
		}
	})
}
