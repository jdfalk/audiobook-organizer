// file: internal/deluge/client.go
// version: 1.0.0
// guid: 9a7b8c6d-0e1f-4a70-b8c5-3d7e0f1b9a99
//
// Deluge Web JSON-RPC client (backlog 6.1).
//
// Communicates with Deluge's Web UI at /json using JSON-RPC 2.0.
// Session-based auth via cookie. Supports:
//   - Authentication (auth.login)
//   - Listing torrents (core.get_torrents_status)
//   - Getting single torrent info (core.get_torrent_status)
//   - Moving torrent storage (core.move_storage)
//
// Reference: https://deluge.readthedocs.io/en/latest/reference/webapi.html

package deluge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"sync/atomic"
)

// Client talks to a Deluge Web UI instance via JSON-RPC.
type Client struct {
	baseURL  string
	password string
	client   *http.Client
	mu       sync.Mutex
	authed   bool
	reqID    atomic.Int64
}

// TorrentStatus holds the fields we care about from Deluge.
type TorrentStatus struct {
	Hash     string `json:"hash"`
	Name     string `json:"name"`
	SavePath string `json:"save_path"`
	State    string `json:"state"`
	Progress float64 `json:"progress"`
}

type rpcRequest struct {
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
	ID     int64         `json:"id"`
}

type rpcResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// New creates a Deluge Web JSON-RPC client.
// baseURL is the Deluge Web UI URL (e.g. "http://172.16.2.30:8112").
// password is the Web UI password (default: "deluge").
func New(baseURL, password string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL:  baseURL,
		password: password,
		client:   &http.Client{Jar: jar},
	}, nil
}

// call sends a JSON-RPC request and decodes the result.
func (c *Client) call(method string, params ...interface{}) (json.RawMessage, error) {
	if params == nil {
		params = []interface{}{}
	}
	id := c.reqID.Add(1)
	body, _ := json.Marshal(rpcRequest{
		Method: method,
		Params: params,
		ID:     id,
	})

	resp, err := c.client.Post(c.baseURL+"/json", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("deluge rpc %s: %w", method, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(raw, &rpcResp); err != nil {
		return nil, fmt.Errorf("decode response: %w (body: %s)", err, string(raw[:min(200, len(raw))]))
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("deluge error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

// Login authenticates with the Deluge Web UI. Must be called before
// other methods. Idempotent — skips if already authenticated.
func (c *Client) Login() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.authed {
		return nil
	}
	result, err := c.call("auth.login", c.password)
	if err != nil {
		return fmt.Errorf("auth.login: %w", err)
	}
	var ok bool
	if err := json.Unmarshal(result, &ok); err != nil || !ok {
		return fmt.Errorf("auth.login failed (result: %s)", string(result))
	}
	c.authed = true
	return nil
}

// ListTorrents returns all torrents with the requested fields.
func (c *Client) ListTorrents() (map[string]TorrentStatus, error) {
	if err := c.Login(); err != nil {
		return nil, err
	}
	fields := []string{"hash", "name", "save_path", "state", "progress"}
	result, err := c.call("core.get_torrents_status", map[string]interface{}{}, fields)
	if err != nil {
		return nil, err
	}
	var torrents map[string]TorrentStatus
	if err := json.Unmarshal(result, &torrents); err != nil {
		return nil, fmt.Errorf("decode torrents: %w", err)
	}
	return torrents, nil
}

// GetTorrent returns status for a single torrent by hash.
func (c *Client) GetTorrent(torrentID string) (*TorrentStatus, error) {
	if err := c.Login(); err != nil {
		return nil, err
	}
	fields := []string{"hash", "name", "save_path", "state", "progress"}
	result, err := c.call("core.get_torrent_status", torrentID, fields)
	if err != nil {
		return nil, err
	}
	var status TorrentStatus
	if err := json.Unmarshal(result, &status); err != nil {
		return nil, fmt.Errorf("decode torrent: %w", err)
	}
	return &status, nil
}

// MoveStorage moves a torrent's data to a new location on disk.
// This is the key integration point for library centralization —
// when a book version is swapped or reorganized, the torrent's
// storage path needs to follow.
func (c *Client) MoveStorage(torrentIDs []string, destPath string) error {
	if err := c.Login(); err != nil {
		return err
	}
	_, err := c.call("core.move_storage", torrentIDs, destPath)
	return err
}

// Connected checks whether the Web UI is connected to a daemon.
func (c *Client) Connected() (bool, error) {
	if err := c.Login(); err != nil {
		return false, err
	}
	result, err := c.call("web.connected")
	if err != nil {
		return false, err
	}
	var connected bool
	_ = json.Unmarshal(result, &connected)
	return connected, nil
}
