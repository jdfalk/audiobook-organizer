// file: internal/mtls/config.go
// version: 1.0.0

package mtls

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DirState represents the provisioning state of the .mtls directory.
type DirState int

const (
	DirStateEmpty        DirState = iota // No PSK, no certs
	DirStateProvisioning                 // PSK exists, no certs
	DirStateReady                        // Certs exist, ready for mTLS
)

// ServerInfo is written to server.json by the serve command.
type ServerInfo struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// Dir manages the .mtls directory contents.
type Dir struct {
	path string
}

// NewDir creates a Dir pointing at the given directory path.
func NewDir(path string) *Dir {
	return &Dir{path: path}
}

// EnsureDir creates the .mtls directory with 0700 permissions if it doesn't exist.
func (d *Dir) EnsureDir() error {
	return os.MkdirAll(d.path, 0700)
}

// Path returns the full path to a file in the .mtls directory.
func (d *Dir) Path(name string) string {
	return filepath.Join(d.path, name)
}

// State determines the current provisioning state.
func (d *Dir) State() (DirState, error) {
	hasPSK := fileExists(d.Path("psk.txt"))
	hasCACert := fileExists(d.Path("ca.crt"))
	hasServerCert := fileExists(d.Path("server.crt"))
	hasServerKey := fileExists(d.Path("server.key"))

	if hasCACert && hasServerCert && hasServerKey {
		return DirStateReady, nil
	}
	if hasPSK {
		return DirStateProvisioning, nil
	}
	return DirStateEmpty, nil
}

// GeneratePSK creates a random 32-byte PSK, base64-encodes it, and writes to psk.txt.
func (d *Dir) GeneratePSK() (string, error) {
	if err := d.EnsureDir(); err != nil {
		return "", err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	psk := base64.StdEncoding.EncodeToString(b)
	if err := os.WriteFile(d.Path("psk.txt"), []byte(psk), 0600); err != nil {
		return "", fmt.Errorf("write psk.txt: %w", err)
	}
	return psk, nil
}

// ReadPSK reads and returns the PSK from psk.txt.
func (d *Dir) ReadPSK() (string, error) {
	data, err := os.ReadFile(d.Path("psk.txt"))
	if err != nil {
		return "", fmt.Errorf("read psk.txt: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// DeletePSK removes the psk.txt file.
func (d *Dir) DeletePSK() error {
	return os.Remove(d.Path("psk.txt"))
}

// WriteServerInfo writes server.json with host and port.
func (d *Dir) WriteServerInfo(info ServerInfo) error {
	if err := d.EnsureDir(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(d.Path("server.json"), data, 0644)
}

// ReadServerInfo reads server.json.
func (d *Dir) ReadServerInfo() (ServerInfo, error) {
	data, err := os.ReadFile(d.Path("server.json"))
	if err != nil {
		return ServerInfo{}, fmt.Errorf("read server.json: %w", err)
	}
	var info ServerInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return ServerInfo{}, fmt.Errorf("parse server.json: %w", err)
	}
	return info, nil
}

// WriteCert writes a PEM-encoded cert or key to a named file.
func (d *Dir) WriteCert(name string, data []byte) error {
	if err := d.EnsureDir(); err != nil {
		return err
	}
	return os.WriteFile(d.Path(name), data, 0600)
}

// ReadCert reads a PEM file from the directory.
func (d *Dir) ReadCert(name string) ([]byte, error) {
	return os.ReadFile(d.Path(name))
}

// Reset deletes all files in the .mtls directory.
func (d *Dir) Reset() error {
	entries, err := os.ReadDir(d.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		os.Remove(filepath.Join(d.path, e.Name()))
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
