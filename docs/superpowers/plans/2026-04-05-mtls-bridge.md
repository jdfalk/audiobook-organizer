# mTLS Bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace SSH-piped stdio with a Go mTLS bridge binary (`mtls-bridge`) that wraps the PowerShell iTunes MCP server on Windows and provides a stdio-to-mTLS client on Mac, bootstrapped from a PSK.

**Architecture:** A single Go binary with three subcommands (`serve`, `connect`, `provision`). The server spawns PowerShell as a subprocess and bridges mTLS TCP connections to its stdin/stdout. The client reads certs and server address from a shared `.mtls/` directory and bridges local stdin/stdout to the mTLS connection. Provisioning uses a PSK to bootstrap cert generation on first use.

**Tech Stack:** Go 1.26, `crypto/x509`, `crypto/tls`, `crypto/ecdsa`, `encoding/pem`, `cobra` CLI framework. No external dependencies beyond what's already in go.mod.

**Spec:** `docs/superpowers/specs/2026-04-05-mtls-bridge-design.md`

---

## File Structure

```
cmd/mtls-bridge/
  main.go             # CLI entry point, cobra root + subcommands
internal/mtls/
  certs.go            # CA + cert generation (GenerateCA, GenerateSignedCert)
  certs_test.go       # Tests for cert generation
  config.go           # .mtls/ directory management, server.json read/write
  config_test.go      # Tests for config read/write
  provisioning.go     # PSK exchange protocol (server + client sides)
  provisioning_test.go
  transport.go        # mTLS listener + dialer factories
  transport_test.go
  bridge.go           # Bidirectional byte pipe (TCP ↔ stdio/subprocess)
  bridge_test.go
.mtls/                # Created at runtime, gitignored
.gitignore            # Add .mtls/ entry
.mcp.json             # Update from SSH to mtls-bridge connect
Makefile              # Add build-mtls-bridge and build-mtls-bridge-windows targets
```

---

### Task 1: Project Scaffolding

**Files:**
- Create: `cmd/mtls-bridge/main.go`
- Modify: `.gitignore`
- Modify: `Makefile`

- [ ] **Step 1: Add `.mtls/` to `.gitignore`**

Append to `.gitignore`:

```
# mTLS bridge certificates and config
.mtls/
```

- [ ] **Step 2: Add Makefile targets**

Append to `Makefile` before the `clean` target:

```makefile
## build-mtls-bridge: Build the mTLS bridge binary (macOS)
build-mtls-bridge:
	@echo "Building mtls-bridge..."
	@go build -ldflags="$(LDFLAGS)" -o mtls-bridge ./cmd/mtls-bridge

## build-mtls-bridge-windows: Cross-compile mTLS bridge for Windows amd64
build-mtls-bridge-windows:
	@echo "Building mtls-bridge.exe for Windows..."
	@GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o mtls-bridge.exe ./cmd/mtls-bridge
```

- [ ] **Step 3: Create `cmd/mtls-bridge/main.go`**

```go
// file: cmd/mtls-bridge/main.go
// version: 1.0.0

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var mtlsDir string

var rootCmd = &cobra.Command{
	Use:   "mtls-bridge",
	Short: "mTLS bridge for iTunes MCP server",
	Long:  "Bridges MCP protocol over mTLS between Claude Code (Mac) and iTunes MCP server (Windows).",
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start mTLS server wrapping PowerShell MCP script",
	RunE:  runServe,
}

var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Connect to mTLS server, bridge to stdio",
	RunE:  runConnect,
}

var provisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Manage PSK and certificate provisioning",
	RunE:  runProvision,
}

var (
	powershellPath string
	listenHost     string
	generatePSK    bool
	renewCerts     bool
	resetAll       bool
)

func init() {
	rootCmd.PersistentFlags().StringVar(&mtlsDir, "mtls-dir", ".mtls", "Directory for certificates and config")

	serveCmd.Flags().StringVar(&powershellPath, "powershell", "", "Path to PowerShell MCP script")
	serveCmd.Flags().StringVar(&listenHost, "host", "0.0.0.0", "Host to listen on")
	serveCmd.MarkFlagRequired("powershell")

	provisionCmd.Flags().BoolVar(&generatePSK, "generate-psk", false, "Generate a new pre-shared key")
	provisionCmd.Flags().BoolVar(&renewCerts, "renew", false, "Renew certs from existing CA")
	provisionCmd.Flags().BoolVar(&resetAll, "reset", false, "Delete all certs and optionally regenerate PSK")

	rootCmd.AddCommand(serveCmd, connectCmd, provisionCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	fmt.Fprintln(os.Stderr, "serve: not yet implemented")
	return nil
}

func runConnect(cmd *cobra.Command, args []string) error {
	fmt.Fprintln(os.Stderr, "connect: not yet implemented")
	return nil
}

func runProvision(cmd *cobra.Command, args []string) error {
	fmt.Fprintln(os.Stderr, "provision: not yet implemented")
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Verify it builds**

Run: `cd /Users/jdfalk/repos/github.com/jdfalk/audiobook-organizer && go build ./cmd/mtls-bridge`
Expected: Clean build, no errors.

- [ ] **Step 5: Commit**

```bash
git add cmd/mtls-bridge/main.go .gitignore Makefile
git commit -m "feat: scaffold mtls-bridge binary with cobra subcommands"
```

---

### Task 2: Certificate Generation (`internal/mtls/certs.go`)

**Files:**
- Create: `internal/mtls/certs.go`
- Create: `internal/mtls/certs_test.go`

- [ ] **Step 1: Write failing tests for CA generation**

Create `internal/mtls/certs_test.go`:

```go
// file: internal/mtls/certs_test.go
// version: 1.0.0

package mtls

import (
	"crypto/x509"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateCA(t *testing.T) {
	ca, err := GenerateCA(10 * 365 * 24 * time.Hour)
	require.NoError(t, err)
	assert.True(t, ca.Cert.IsCA)
	assert.Equal(t, "mtls-bridge CA", ca.Cert.Subject.CommonName)
	assert.NotNil(t, ca.Key)
	assert.NotEmpty(t, ca.CertPEM)
	assert.NotEmpty(t, ca.KeyPEM)
}

func TestGenerateSignedCert(t *testing.T) {
	ca, err := GenerateCA(10 * 365 * 24 * time.Hour)
	require.NoError(t, err)

	cert, err := GenerateSignedCert(ca, "server", []string{"unimatrixzero.local"}, 365*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, "server", cert.Cert.Subject.CommonName)
	assert.Contains(t, cert.Cert.DNSNames, "unimatrixzero.local")
	assert.NotNil(t, cert.Key)

	// Verify cert is signed by CA
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	_, err = cert.Cert.Verify(x509.VerifyOptions{Roots: pool})
	assert.NoError(t, err)
}

func TestGenerateClientCert(t *testing.T) {
	ca, err := GenerateCA(10 * 365 * 24 * time.Hour)
	require.NoError(t, err)

	cert, err := GenerateSignedCert(ca, "client", nil, 365*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, "client", cert.Cert.Subject.CommonName)
	assert.Contains(t, cert.Cert.ExtKeyUsage, x509.ExtKeyUsageClientAuth)

	// Verify cert is signed by CA
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	_, err = cert.Cert.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/mtls/ -v -run TestGenerate`
Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Implement cert generation**

Create `internal/mtls/certs.go`:

```go
// file: internal/mtls/certs.go
// version: 1.0.0

package mtls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// KeyPair holds a parsed certificate, its private key, and PEM-encoded forms.
type KeyPair struct {
	Cert    *x509.Certificate
	Key     *ecdsa.PrivateKey
	CertPEM []byte
	KeyPEM  []byte
}

// GenerateCA creates a self-signed CA certificate with the given validity duration.
func GenerateCA(validity time.Duration) (*KeyPair, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "mtls-bridge CA"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(validity),
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:         true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create CA cert: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse CA cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal CA key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return &KeyPair{Cert: cert, Key: key, CertPEM: certPEM, KeyPEM: keyPEM}, nil
}

// GenerateSignedCert creates a certificate signed by the given CA.
// If sans is non-empty, they are added as DNS SANs and the cert gets ServerAuth usage.
// If sans is empty, the cert gets ClientAuth usage only.
func GenerateSignedCert(ca *KeyPair, commonName string, sans []string, validity time.Duration) (*KeyPair, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(validity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	if len(sans) > 0 {
		template.DNSNames = sans
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.Cert, &key.PublicKey, ca.Key)
	if err != nil {
		return nil, fmt.Errorf("create cert: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return &KeyPair{Cert: cert, Key: key, CertPEM: certPEM, KeyPEM: keyPEM}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/mtls/ -v -run TestGenerate`
Expected: All 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mtls/certs.go internal/mtls/certs_test.go
git commit -m "feat: add mTLS certificate generation (CA + signed certs)"
```

---

### Task 3: Config Management (`internal/mtls/config.go`)

**Files:**
- Create: `internal/mtls/config.go`
- Create: `internal/mtls/config_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/mtls/config_test.go`:

```go
// file: internal/mtls/config_test.go
// version: 1.0.0

package mtls

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirState_Empty(t *testing.T) {
	dir := t.TempDir()
	d := NewDir(dir)
	state, err := d.State()
	require.NoError(t, err)
	assert.Equal(t, DirStateEmpty, state)
}

func TestDirState_HasPSK(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "psk.txt"), []byte("dGVzdA=="), 0600)
	d := NewDir(dir)
	state, err := d.State()
	require.NoError(t, err)
	assert.Equal(t, DirStateProvisioning, state)
}

func TestDirState_HasCerts(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"ca.crt", "server.crt", "server.key"} {
		os.WriteFile(filepath.Join(dir, f), []byte("fake"), 0600)
	}
	d := NewDir(dir)
	state, err := d.State()
	require.NoError(t, err)
	assert.Equal(t, DirStateReady, state)
}

func TestServerJSON_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	d := NewDir(dir)

	info := ServerInfo{Host: "unimatrixzero.local", Port: 9372}
	err := d.WriteServerInfo(info)
	require.NoError(t, err)

	got, err := d.ReadServerInfo()
	require.NoError(t, err)
	assert.Equal(t, info, got)
}

func TestGeneratePSK(t *testing.T) {
	dir := t.TempDir()
	d := NewDir(dir)
	psk, err := d.GeneratePSK()
	require.NoError(t, err)
	assert.Len(t, psk, 44) // 32 bytes base64 = 44 chars

	// Verify it was written
	data, err := os.ReadFile(filepath.Join(dir, "psk.txt"))
	require.NoError(t, err)
	assert.Equal(t, psk, string(data))
}

func TestReadPSK(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "psk.txt"), []byte("dGVzdHRlc3R0ZXN0dGVzdHRlc3R0ZXN0dGVzdHQ="), 0600)
	d := NewDir(dir)
	psk, err := d.ReadPSK()
	require.NoError(t, err)
	assert.Equal(t, "dGVzdHRlc3R0ZXN0dGVzdHRlc3R0ZXN0dGVzdHQ=", psk)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/mtls/ -v -run "TestDir|TestServer|TestGenerate.*PSK|TestRead"`
Expected: FAIL — types not defined yet.

- [ ] **Step 3: Implement config management**

Create `internal/mtls/config.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/mtls/ -v -run "TestDir|TestServer|TestGenerate.*PSK|TestRead"`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mtls/config.go internal/mtls/config_test.go
git commit -m "feat: add .mtls directory management and server.json config"
```

---

### Task 4: mTLS Transport (`internal/mtls/transport.go`)

**Files:**
- Create: `internal/mtls/transport.go`
- Create: `internal/mtls/transport_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/mtls/transport_test.go`:

```go
// file: internal/mtls/transport_test.go
// version: 1.0.0

package mtls

import (
	"crypto/tls"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMTLSRoundTrip(t *testing.T) {
	// Generate CA + server + client certs
	ca, err := GenerateCA(1 * time.Hour)
	require.NoError(t, err)
	serverKP, err := GenerateSignedCert(ca, "server", []string{"localhost"}, 1*time.Hour)
	require.NoError(t, err)
	clientKP, err := GenerateSignedCert(ca, "client", nil, 1*time.Hour)
	require.NoError(t, err)

	// Build TLS configs
	serverTLS, err := ServerTLSConfig(ca.CertPEM, serverKP.CertPEM, serverKP.KeyPEM)
	require.NoError(t, err)
	clientTLS, err := ClientTLSConfig(ca.CertPEM, clientKP.CertPEM, clientKP.KeyPEM, "localhost")
	require.NoError(t, err)

	// Start listener
	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	require.NoError(t, err)
	defer ln.Close()

	// Server goroutine: echo back
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(conn, conn)
	}()

	// Client connects
	conn, err := tls.Dial("tcp", ln.Addr().String(), clientTLS)
	require.NoError(t, err)
	defer conn.Close()

	// Send data and verify echo
	msg := []byte("hello mTLS")
	_, err = conn.Write(msg)
	require.NoError(t, err)

	buf := make([]byte, len(msg))
	_, err = io.ReadFull(conn, buf)
	require.NoError(t, err)
	assert.Equal(t, msg, buf)
}

func TestMTLSRejectsWithoutClientCert(t *testing.T) {
	ca, err := GenerateCA(1 * time.Hour)
	require.NoError(t, err)
	serverKP, err := GenerateSignedCert(ca, "server", []string{"localhost"}, 1*time.Hour)
	require.NoError(t, err)

	serverTLS, err := ServerTLSConfig(ca.CertPEM, serverKP.CertPEM, serverKP.KeyPEM)
	require.NoError(t, err)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	require.NoError(t, err)
	defer ln.Close()

	// Server goroutine
	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	// Client with no client cert — should fail handshake
	noClientTLS := &tls.Config{
		RootCAs:    serverTLS.ClientCAs, // trust the CA
		ServerName: "localhost",
	}
	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 2*time.Second)
	require.NoError(t, err)
	tlsConn := tls.Client(conn, noClientTLS)
	err = tlsConn.Handshake()
	assert.Error(t, err)
	conn.Close()
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/mtls/ -v -run "TestMTLS"`
Expected: FAIL — `ServerTLSConfig` and `ClientTLSConfig` not defined.

- [ ] **Step 3: Implement transport**

Create `internal/mtls/transport.go`:

```go
// file: internal/mtls/transport.go
// version: 1.0.0

package mtls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"
)

// ServerTLSConfig creates a TLS config for the server that requires client certs
// signed by the given CA.
func ServerTLSConfig(caCertPEM, serverCertPEM, serverKeyPEM []byte) (*tls.Config, error) {
	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("load server cert: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCertPEM) {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// ClientTLSConfig creates a TLS config for the client with a client cert
// that verifies the server cert against the given CA.
func ClientTLSConfig(caCertPEM, clientCertPEM, clientKeyPEM []byte, serverName string) (*tls.Config, error) {
	clientCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("load client cert: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCertPEM) {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// EphemeralTLSConfig creates a TLS config with a self-signed cert for the provisioning
// handshake. Encryption only — no identity verification.
func EphemeralTLSConfig() (*tls.Config, error) {
	// Generate a throwaway CA and cert
	ca, err := GenerateCA(24 * time.Hour)
	if err != nil {
		return nil, err
	}
	cert, err := GenerateSignedCert(ca, "provisioning", []string{"localhost"}, 24*time.Hour)
	if err != nil {
		return nil, err
	}
	tlsCert, err := tls.X509KeyPair(cert.CertPEM, cert.KeyPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS13,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/mtls/ -v -run "TestMTLS"`
Expected: Both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mtls/transport.go internal/mtls/transport_test.go
git commit -m "feat: add mTLS transport config (server, client, ephemeral)"
```

---

### Task 5: Bidirectional Bridge (`internal/mtls/bridge.go`)

**Files:**
- Create: `internal/mtls/bridge.go`
- Create: `internal/mtls/bridge_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/mtls/bridge_test.go`:

```go
// file: internal/mtls/bridge_test.go
// version: 1.0.0

package mtls

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBridge(t *testing.T) {
	// Create a pipe pair simulating subprocess stdin/stdout
	subIn, subInW := io.Pipe()   // bridge writes to subInW → subprocess reads from subIn
	subOutR, subOut := io.Pipe() // subprocess writes to subOut → bridge reads from subOutR

	// Create a TCP pipe simulating the network connection
	server, client := net.Pipe()

	// Subprocess: echo back whatever it reads
	go func() {
		io.Copy(subOut, subIn)
		subOut.Close()
	}()

	// Bridge: connect network ↔ subprocess
	done := make(chan error, 1)
	go func() {
		done <- Bridge(server, subOutR, subInW)
	}()

	// Client sends data
	msg := []byte("Content-Length: 5\r\n\r\nhello")
	_, err := client.Write(msg)
	require.NoError(t, err)

	// Client reads echoed data back
	buf := make([]byte, len(msg))
	_, err = io.ReadFull(client, buf)
	require.NoError(t, err)
	assert.Equal(t, msg, buf)

	// Close client side
	client.Close()

	select {
	case err := <-done:
		// Bridge should return nil or io.EOF on clean close
		if err != nil && err != io.EOF {
			t.Errorf("unexpected bridge error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("bridge did not return")
	}
}

func TestBridgeStdio(t *testing.T) {
	// Simulate stdin/stdout as buffers
	stdinData := []byte("request data")
	stdin := bytes.NewReader(stdinData)
	stdout := &bytes.Buffer{}

	// Server side: echo
	server, client := net.Pipe()
	go func() {
		io.Copy(server, server)
		server.Close()
	}()

	err := BridgeStdio(client, stdin, stdout)
	// Should complete when stdin is exhausted
	assert.Nil(t, err)
	assert.Equal(t, stdinData, stdout.Bytes())
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/mtls/ -v -run "TestBridge"`
Expected: FAIL — `Bridge` and `BridgeStdio` not defined.

- [ ] **Step 3: Implement bridge**

Create `internal/mtls/bridge.go`:

```go
// file: internal/mtls/bridge.go
// version: 1.0.0

package mtls

import (
	"io"
	"net"
	"sync"
)

// Bridge copies data bidirectionally between a network connection and
// a subprocess's stdout (read) and stdin (write).
// Returns when either direction encounters an error or EOF.
func Bridge(conn net.Conn, subStdout io.Reader, subStdin io.Writer) error {
	var wg sync.WaitGroup
	errs := make(chan error, 2)

	wg.Add(2)

	// Network → subprocess stdin
	go func() {
		defer wg.Done()
		_, err := io.Copy(subStdin, conn)
		errs <- err
	}()

	// Subprocess stdout → network
	go func() {
		defer wg.Done()
		_, err := io.Copy(conn, subStdout)
		errs <- err
	}()

	// Wait for first error/EOF, then close connection to unblock the other goroutine
	err := <-errs
	conn.Close()
	wg.Wait()

	if err == io.EOF {
		return nil
	}
	return err
}

// BridgeStdio copies data bidirectionally between a network connection and
// local stdin/stdout. Used by the connect command.
// Returns when stdin is exhausted (EOF) or the connection closes.
func BridgeStdio(conn net.Conn, stdin io.Reader, stdout io.Writer) error {
	var wg sync.WaitGroup
	errs := make(chan error, 2)

	wg.Add(2)

	// Local stdin → network
	go func() {
		defer wg.Done()
		_, err := io.Copy(conn, stdin)
		// stdin EOF means Claude Code closed, shut down the connection
		conn.Close()
		errs <- err
	}()

	// Network → local stdout
	go func() {
		defer wg.Done()
		_, err := io.Copy(stdout, conn)
		errs <- err
	}()

	err := <-errs
	conn.Close()
	wg.Wait()

	if err == io.EOF {
		return nil
	}
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/mtls/ -v -run "TestBridge"`
Expected: Both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mtls/bridge.go internal/mtls/bridge_test.go
git commit -m "feat: add bidirectional bridge for TCP-to-stdio piping"
```

---

### Task 6: PSK Provisioning Protocol (`internal/mtls/provisioning.go`)

**Files:**
- Create: `internal/mtls/provisioning.go`
- Create: `internal/mtls/provisioning_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/mtls/provisioning_test.go`:

```go
// file: internal/mtls/provisioning_test.go
// version: 1.0.0

package mtls

import (
	"crypto/tls"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvisioningExchange(t *testing.T) {
	serverDir := t.TempDir()
	clientDir := t.TempDir()

	// Write same PSK to both dirs
	psk := "dGVzdHBza3Rlc3Rwc2t0ZXN0cHNrdGVzdHBzaw=="
	os.WriteFile(filepath.Join(serverDir, "psk.txt"), []byte(psk), 0600)
	os.WriteFile(filepath.Join(clientDir, "psk.txt"), []byte(psk), 0600)

	serverD := NewDir(serverDir)
	clientD := NewDir(clientDir)

	// Start provisioning server
	ps, err := NewProvisioningServer(serverD, "localhost")
	require.NoError(t, err)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", ps.TLSConfig)
	require.NoError(t, err)
	defer ln.Close()

	// Run server in background
	serverDone := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		serverDone <- ps.HandleConnection(conn)
	}()

	// Run client provisioning
	err = RunProvisioningClient(clientD, ln.Addr().String())
	require.NoError(t, err)

	// Wait for server
	err = <-serverDone
	require.NoError(t, err)

	// Verify certs were written to both dirs
	assert.FileExists(t, filepath.Join(serverDir, "ca.crt"))
	assert.FileExists(t, filepath.Join(serverDir, "ca.key"))
	assert.FileExists(t, filepath.Join(serverDir, "server.crt"))
	assert.FileExists(t, filepath.Join(serverDir, "server.key"))
	assert.FileExists(t, filepath.Join(clientDir, "ca.crt"))
	assert.FileExists(t, filepath.Join(clientDir, "client.crt"))
	assert.FileExists(t, filepath.Join(clientDir, "client.key"))

	// Verify PSK was deleted
	assert.NoFileExists(t, filepath.Join(serverDir, "psk.txt"))
	assert.NoFileExists(t, filepath.Join(clientDir, "psk.txt"))

	// Verify the generated certs actually work for mTLS
	caCert, _ := os.ReadFile(filepath.Join(serverDir, "ca.crt"))
	serverCert, _ := os.ReadFile(filepath.Join(serverDir, "server.crt"))
	serverKey, _ := os.ReadFile(filepath.Join(serverDir, "server.key"))
	clientCert, _ := os.ReadFile(filepath.Join(clientDir, "client.crt"))
	clientKey, _ := os.ReadFile(filepath.Join(clientDir, "client.key"))

	sTLS, err := ServerTLSConfig(caCert, serverCert, serverKey)
	require.NoError(t, err)
	cTLS, err := ClientTLSConfig(caCert, clientCert, clientKey, "localhost")
	require.NoError(t, err)

	// Quick mTLS handshake test
	sLn, err := tls.Listen("tcp", "127.0.0.1:0", sTLS)
	require.NoError(t, err)
	defer sLn.Close()

	go func() {
		c, _ := sLn.Accept()
		if c != nil {
			c.Close()
		}
	}()

	conn, err := tls.Dial("tcp", sLn.Addr().String(), cTLS)
	require.NoError(t, err)
	conn.Close()
}

func TestProvisioningWrongPSK(t *testing.T) {
	serverDir := t.TempDir()
	clientDir := t.TempDir()

	os.WriteFile(filepath.Join(serverDir, "psk.txt"), []byte("c2VydmVycHNr"), 0600)
	os.WriteFile(filepath.Join(clientDir, "psk.txt"), []byte("Y2xpZW50cHNr"), 0600)

	serverD := NewDir(serverDir)
	clientD := NewDir(clientDir)

	ps, err := NewProvisioningServer(serverD, "localhost")
	require.NoError(t, err)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", ps.TLSConfig)
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		ps.HandleConnection(conn)
	}()

	err = RunProvisioningClient(clientD, ln.Addr().String())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PSK")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/mtls/ -v -run "TestProvisioning" -timeout 30s`
Expected: FAIL — types not defined.

- [ ] **Step 3: Implement provisioning protocol**

Create `internal/mtls/provisioning.go`:

```go
// file: internal/mtls/provisioning.go
// version: 1.0.0

package mtls

import (
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

// provisioningMessage is the JSON envelope used during PSK exchange.
type provisioningMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type pskMessage struct {
	PSK string `json:"psk"`
}

type certsMessage struct {
	CACert    string `json:"ca_crt"`
	ClientCrt string `json:"client_crt"`
	ClientKey string `json:"client_key"`
}

type errorMessage struct {
	Error string `json:"error"`
}

// ProvisioningServer handles the server side of PSK-based cert provisioning.
type ProvisioningServer struct {
	dir       *Dir
	hostname  string
	TLSConfig *tls.Config
}

// NewProvisioningServer creates a provisioning server with an ephemeral TLS cert.
func NewProvisioningServer(dir *Dir, hostname string) (*ProvisioningServer, error) {
	tlsCfg, err := EphemeralTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("create ephemeral TLS: %w", err)
	}
	return &ProvisioningServer{dir: dir, hostname: hostname, TLSConfig: tlsCfg}, nil
}

// HandleConnection performs the provisioning handshake on an accepted connection.
func (ps *ProvisioningServer) HandleConnection(conn net.Conn) error {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	// Read PSK from client
	var msg provisioningMessage
	if err := dec.Decode(&msg); err != nil {
		return fmt.Errorf("read client message: %w", err)
	}
	if msg.Type != "psk" {
		return fmt.Errorf("expected psk message, got %s", msg.Type)
	}

	var pskMsg pskMessage
	if err := json.Unmarshal(msg.Data, &pskMsg); err != nil {
		return fmt.Errorf("parse psk data: %w", err)
	}

	// Validate PSK
	serverPSK, err := ps.dir.ReadPSK()
	if err != nil {
		return fmt.Errorf("read server PSK: %w", err)
	}

	if subtle.ConstantTimeCompare([]byte(pskMsg.PSK), []byte(serverPSK)) != 1 {
		errData, _ := json.Marshal(errorMessage{Error: "PSK mismatch"})
		enc.Encode(provisioningMessage{Type: "error", Data: errData})
		return fmt.Errorf("PSK mismatch")
	}

	// Generate CA + certs
	ca, err := GenerateCA(10 * 365 * 24 * time.Hour)
	if err != nil {
		return fmt.Errorf("generate CA: %w", err)
	}

	serverCert, err := GenerateSignedCert(ca, "server", []string{ps.hostname}, 365*24*time.Hour)
	if err != nil {
		return fmt.Errorf("generate server cert: %w", err)
	}

	clientCert, err := GenerateSignedCert(ca, "client", nil, 365*24*time.Hour)
	if err != nil {
		return fmt.Errorf("generate client cert: %w", err)
	}

	// Save server-side certs
	if err := ps.dir.WriteCert("ca.crt", ca.CertPEM); err != nil {
		return err
	}
	if err := ps.dir.WriteCert("ca.key", ca.KeyPEM); err != nil {
		return err
	}
	if err := ps.dir.WriteCert("server.crt", serverCert.CertPEM); err != nil {
		return err
	}
	if err := ps.dir.WriteCert("server.key", serverCert.KeyPEM); err != nil {
		return err
	}

	// Send client certs
	certsData, _ := json.Marshal(certsMessage{
		CACert:    string(ca.CertPEM),
		ClientCrt: string(clientCert.CertPEM),
		ClientKey: string(clientCert.KeyPEM),
	})
	if err := enc.Encode(provisioningMessage{Type: "certs", Data: certsData}); err != nil {
		return fmt.Errorf("send certs: %w", err)
	}

	// Delete PSK
	ps.dir.DeletePSK()
	return nil
}

// RunProvisioningClient connects to a provisioning server, sends PSK, and saves received certs.
func RunProvisioningClient(dir *Dir, addr string) error {
	psk, err := dir.ReadPSK()
	if err != nil {
		return fmt.Errorf("read PSK: %w", err)
	}

	// Connect with TLS but skip server verification (ephemeral cert)
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 10 * time.Second},
		"tcp",
		addr,
		&tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13},
	)
	if err != nil {
		return fmt.Errorf("connect to provisioning server: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	// Send PSK
	pskData, _ := json.Marshal(pskMessage{PSK: psk})
	if err := enc.Encode(provisioningMessage{Type: "psk", Data: pskData}); err != nil {
		return fmt.Errorf("send PSK: %w", err)
	}

	// Read response
	var msg provisioningMessage
	if err := dec.Decode(&msg); err != nil {
		if err == io.EOF {
			return fmt.Errorf("server closed connection — PSK likely rejected")
		}
		return fmt.Errorf("read server response: %w", err)
	}

	if msg.Type == "error" {
		var errMsg errorMessage
		json.Unmarshal(msg.Data, &errMsg)
		return fmt.Errorf("provisioning failed: PSK rejected: %s", errMsg.Error)
	}

	if msg.Type != "certs" {
		return fmt.Errorf("expected certs message, got %s", msg.Type)
	}

	var certs certsMessage
	if err := json.Unmarshal(msg.Data, &certs); err != nil {
		return fmt.Errorf("parse certs: %w", err)
	}

	// Save certs
	if err := dir.WriteCert("ca.crt", []byte(certs.CACert)); err != nil {
		return err
	}
	if err := dir.WriteCert("client.crt", []byte(certs.ClientCrt)); err != nil {
		return err
	}
	if err := dir.WriteCert("client.key", []byte(certs.ClientKey)); err != nil {
		return err
	}

	// Delete PSK
	dir.DeletePSK()
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/mtls/ -v -run "TestProvisioning" -timeout 30s`
Expected: Both tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mtls/provisioning.go internal/mtls/provisioning_test.go
git commit -m "feat: add PSK-based provisioning protocol for mTLS cert exchange"
```

---

### Task 7: `serve` Subcommand

**Files:**
- Modify: `cmd/mtls-bridge/main.go`

- [ ] **Step 1: Implement `runServe`**

Replace the `runServe` stub in `cmd/mtls-bridge/main.go` with:

```go
func runServe(cmd *cobra.Command, args []string) error {
	dir := mtls.NewDir(mtlsDir)

	state, err := dir.State()
	if err != nil {
		return fmt.Errorf("check .mtls state: %w", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	var tlsCfg *tls.Config

	switch state {
	case mtls.DirStateEmpty:
		return fmt.Errorf("no PSK or certs found in %s — run 'mtls-bridge provision --generate-psk' first", mtlsDir)

	case mtls.DirStateProvisioning:
		fmt.Fprintf(os.Stderr, "[mtls-bridge] Provisioning mode — waiting for client to exchange PSK...\n")
		ps, err := mtls.NewProvisioningServer(dir, hostname)
		if err != nil {
			return err
		}

		ln, err := tls.Listen("tcp", net.JoinHostPort(listenHost, "0"), ps.TLSConfig)
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		port := ln.Addr().(*net.TCPAddr).Port

		if err := dir.WriteServerInfo(mtls.ServerInfo{Host: hostname, Port: port}); err != nil {
			ln.Close()
			return err
		}
		fmt.Fprintf(os.Stderr, "[mtls-bridge] Listening on %s:%d (provisioning)\n", listenHost, port)

		conn, err := ln.Accept()
		ln.Close()
		if err != nil {
			return fmt.Errorf("accept provisioning connection: %w", err)
		}
		if err := ps.HandleConnection(conn); err != nil {
			return fmt.Errorf("provisioning failed: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[mtls-bridge] Provisioning complete! Switching to mTLS mode...\n")

		// Fall through to start mTLS server
		state = mtls.DirStateReady
		fallthrough

	case mtls.DirStateReady:
		caCert, err := dir.ReadCert("ca.crt")
		if err != nil {
			return err
		}
		serverCert, err := dir.ReadCert("server.crt")
		if err != nil {
			return err
		}
		serverKey, err := dir.ReadCert("server.key")
		if err != nil {
			return err
		}
		tlsCfg, err = mtls.ServerTLSConfig(caCert, serverCert, serverKey)
		if err != nil {
			return err
		}
	}

	ln, err := tls.Listen("tcp", net.JoinHostPort(listenHost, "0"), tlsCfg)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	if err := dir.WriteServerInfo(mtls.ServerInfo{Host: hostname, Port: port}); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[mtls-bridge] mTLS server listening on %s:%d\n", listenHost, port)

	// Lazy-start PowerShell subprocess
	var psCmd *exec.Cmd
	var psStdin io.WriteCloser
	var psStdout io.ReadCloser

	startPowerShell := func() error {
		if psCmd != nil && psCmd.Process != nil {
			// Check if still running
			if psCmd.ProcessState == nil {
				return nil
			}
		}
		fmt.Fprintf(os.Stderr, "[mtls-bridge] Starting PowerShell: %s\n", powershellPath)
		psCmd = exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", powershellPath)
		var err error
		psStdin, err = psCmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("stdin pipe: %w", err)
		}
		psStdout, err = psCmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("stdout pipe: %w", err)
		}
		psCmd.Stderr = os.Stderr
		if err := psCmd.Start(); err != nil {
			return fmt.Errorf("start powershell: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[mtls-bridge] PowerShell started (PID %d)\n", psCmd.Process.Pid)
		return nil
	}

	// Accept connections in a loop
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] Accept error: %v\n", err)
			continue
		}
		fmt.Fprintf(os.Stderr, "[mtls-bridge] Client connected from %s\n", conn.RemoteAddr())

		if err := startPowerShell(); err != nil {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] Failed to start PowerShell: %v\n", err)
			conn.Close()
			continue
		}

		// Bridge this connection to PowerShell — blocks until connection drops
		if err := mtls.Bridge(conn, psStdout, psStdin); err != nil {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] Bridge closed: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] Client disconnected\n")
		}
	}
}
```

Add these imports to the file:

```go
import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"

	mtls "github.com/jdfalk/audiobook-organizer/internal/mtls"
	"github.com/spf13/cobra"
)
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./cmd/mtls-bridge`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add cmd/mtls-bridge/main.go
git commit -m "feat: implement serve subcommand with mTLS listener + PowerShell bridge"
```

---

### Task 8: `connect` Subcommand

**Files:**
- Modify: `cmd/mtls-bridge/main.go`

- [ ] **Step 1: Implement `runConnect`**

Replace the `runConnect` stub:

```go
func runConnect(cmd *cobra.Command, args []string) error {
	dir := mtls.NewDir(mtlsDir)

	state, err := dir.State()
	if err != nil {
		return fmt.Errorf("check .mtls state: %w", err)
	}

	switch state {
	case mtls.DirStateEmpty:
		return fmt.Errorf("no PSK or certs found in %s — run 'mtls-bridge provision --generate-psk' first", mtlsDir)

	case mtls.DirStateProvisioning:
		// PSK exists but no certs — perform provisioning first
		info, err := dir.ReadServerInfo()
		if err != nil {
			return fmt.Errorf("read server.json (is the server running in provisioning mode?): %w", err)
		}
		addr := net.JoinHostPort(info.Host, fmt.Sprintf("%d", info.Port))
		fmt.Fprintf(os.Stderr, "[mtls-bridge] Provisioning: exchanging PSK with %s...\n", addr)
		if err := mtls.RunProvisioningClient(dir, addr); err != nil {
			return fmt.Errorf("provisioning failed: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[mtls-bridge] Provisioning complete! Connecting with mTLS...\n")
		// Re-read server info — server may have restarted on a new port
		time.Sleep(2 * time.Second)
	}

	// Normal mTLS connection
	info, err := dir.ReadServerInfo()
	if err != nil {
		return fmt.Errorf("read server.json (is the server running?): %w", err)
	}

	caCert, err := dir.ReadCert("ca.crt")
	if err != nil {
		return err
	}
	clientCert, err := dir.ReadCert("client.crt")
	if err != nil {
		return err
	}
	clientKey, err := dir.ReadCert("client.key")
	if err != nil {
		return err
	}

	tlsCfg, err := mtls.ClientTLSConfig(caCert, clientCert, clientKey, info.Host)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(info.Host, fmt.Sprintf("%d", info.Port))
	fmt.Fprintf(os.Stderr, "[mtls-bridge] Connecting to %s...\n", addr)

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 10 * time.Second},
		"tcp",
		addr,
		tlsCfg,
	)
	if err != nil {
		return fmt.Errorf("connect to server: %w (is mtls-bridge serve running?)", err)
	}
	defer conn.Close()

	fmt.Fprintf(os.Stderr, "[mtls-bridge] Connected. Bridging stdio...\n")
	return mtls.BridgeStdio(conn, os.Stdin, os.Stdout)
}
```

Add `"time"` and `"crypto/tls"` to the imports if not already present.

- [ ] **Step 2: Verify it compiles**

Run: `go build ./cmd/mtls-bridge`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add cmd/mtls-bridge/main.go
git commit -m "feat: implement connect subcommand with mTLS client + stdio bridge"
```

---

### Task 9: `provision` Subcommand

**Files:**
- Modify: `cmd/mtls-bridge/main.go`

- [ ] **Step 1: Implement `runProvision`**

Replace the `runProvision` stub:

```go
func runProvision(cmd *cobra.Command, args []string) error {
	dir := mtls.NewDir(mtlsDir)

	if resetAll {
		fmt.Fprintf(os.Stderr, "[mtls-bridge] Resetting %s...\n", mtlsDir)
		if err := dir.Reset(); err != nil {
			return fmt.Errorf("reset: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[mtls-bridge] All certs and config deleted.\n")
		if generatePSK {
			// Fall through to generate new PSK
		} else {
			return nil
		}
	}

	if generatePSK {
		psk, err := dir.GeneratePSK()
		if err != nil {
			return fmt.Errorf("generate PSK: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[mtls-bridge] PSK written to %s\n", dir.Path("psk.txt"))
		fmt.Fprintf(os.Stderr, "[mtls-bridge] PSK: %s\n", psk)
		fmt.Fprintf(os.Stderr, "[mtls-bridge] If not using a shared filesystem, copy .mtls/psk.txt to the other machine.\n")
		return nil
	}

	if renewCerts {
		caKeyPEM, err := dir.ReadCert("ca.key")
		if err != nil {
			return fmt.Errorf("read ca.key (does it exist?): %w", err)
		}
		caCertPEM, err := dir.ReadCert("ca.crt")
		if err != nil {
			return fmt.Errorf("read ca.crt: %w", err)
		}

		ca, err := mtls.LoadKeyPair(caCertPEM, caKeyPEM)
		if err != nil {
			return fmt.Errorf("load CA: %w", err)
		}

		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "localhost"
		}

		serverCert, err := mtls.GenerateSignedCert(ca, "server", []string{hostname}, 365*24*time.Hour)
		if err != nil {
			return fmt.Errorf("generate server cert: %w", err)
		}
		clientCert, err := mtls.GenerateSignedCert(ca, "client", nil, 365*24*time.Hour)
		if err != nil {
			return fmt.Errorf("generate client cert: %w", err)
		}

		if err := dir.WriteCert("server.crt", serverCert.CertPEM); err != nil {
			return err
		}
		if err := dir.WriteCert("server.key", serverCert.KeyPEM); err != nil {
			return err
		}
		if err := dir.WriteCert("client.crt", clientCert.CertPEM); err != nil {
			return err
		}
		if err := dir.WriteCert("client.key", clientCert.KeyPEM); err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "[mtls-bridge] Certs renewed. Restart the server to use new certs.\n")
		return nil
	}

	return fmt.Errorf("specify --generate-psk, --renew, or --reset")
}
```

- [ ] **Step 2: Add `LoadKeyPair` to `internal/mtls/certs.go`**

Append to `internal/mtls/certs.go`:

```go
// LoadKeyPair parses PEM-encoded cert and key back into a KeyPair.
func LoadKeyPair(certPEM, keyPEM []byte) (*KeyPair, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in cert")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse cert: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("no PEM block in key")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse key: %w", err)
	}

	return &KeyPair{Cert: cert, Key: key, CertPEM: certPEM, KeyPEM: keyPEM}, nil
}
```

- [ ] **Step 3: Add test for `LoadKeyPair`**

Append to `internal/mtls/certs_test.go`:

```go
func TestLoadKeyPair(t *testing.T) {
	ca, err := GenerateCA(1 * time.Hour)
	require.NoError(t, err)

	loaded, err := LoadKeyPair(ca.CertPEM, ca.KeyPEM)
	require.NoError(t, err)
	assert.Equal(t, ca.Cert.Subject.CommonName, loaded.Cert.Subject.CommonName)
	assert.True(t, loaded.Cert.IsCA)
}
```

- [ ] **Step 4: Run all tests**

Run: `go test ./internal/mtls/ -v`
Expected: All tests PASS.

- [ ] **Step 5: Verify full binary compiles**

Run: `go build ./cmd/mtls-bridge`
Expected: Clean build.

- [ ] **Step 6: Commit**

```bash
git add cmd/mtls-bridge/main.go internal/mtls/certs.go internal/mtls/certs_test.go
git commit -m "feat: implement provision subcommand (generate-psk, renew, reset)"
```

---

### Task 10: Update `.mcp.json` and Integration Test

**Files:**
- Modify: `.mcp.json`

- [ ] **Step 1: Update `.mcp.json`**

Replace the contents of `.mcp.json`:

```json
{
  "mcpServers": {
    "itunes": {
      "command": "./mtls-bridge",
      "args": ["connect"]
    }
  }
}
```

- [ ] **Step 2: Build for both platforms**

Run:
```bash
make build-mtls-bridge
make build-mtls-bridge-windows
```
Expected: Both `mtls-bridge` (macOS) and `mtls-bridge.exe` (Windows) built.

- [ ] **Step 3: Add binaries to `.gitignore`**

Append to `.gitignore`:

```
# mTLS bridge binaries
mtls-bridge
mtls-bridge.exe
```

- [ ] **Step 4: Run full test suite to verify no regressions**

Run: `go test ./internal/mtls/ -v -count=1`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add .mcp.json .gitignore
git commit -m "feat: update MCP config from SSH to mtls-bridge, add binary to gitignore"
```

---

### Task 11: Cert Expiry Warning

**Files:**
- Modify: `internal/mtls/config.go`
- Create test in: `internal/mtls/config_test.go`

- [ ] **Step 1: Write failing test**

Append to `internal/mtls/config_test.go`:

```go
func TestCheckCertExpiry_NotExpiring(t *testing.T) {
	ca, err := GenerateCA(10 * 365 * 24 * time.Hour)
	require.NoError(t, err)

	dir := t.TempDir()
	d := NewDir(dir)
	d.WriteCert("ca.crt", ca.CertPEM)

	warnings := d.CheckCertExpiry(30 * 24 * time.Hour)
	assert.Empty(t, warnings)
}

func TestCheckCertExpiry_Expiring(t *testing.T) {
	// Generate a cert that expires in 10 days
	ca, err := GenerateCA(10 * 24 * time.Hour)
	require.NoError(t, err)

	dir := t.TempDir()
	d := NewDir(dir)
	d.WriteCert("ca.crt", ca.CertPEM)

	warnings := d.CheckCertExpiry(30 * 24 * time.Hour)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "ca.crt")
}
```

Add `"time"` to imports in config_test.go if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mtls/ -v -run "TestCheckCertExpiry"`
Expected: FAIL — method not defined.

- [ ] **Step 3: Implement expiry check**

Append to `internal/mtls/config.go`:

```go
import (
	"crypto/x509"
	"encoding/pem"
	"time"
)

// CheckCertExpiry checks all .crt files in the directory and returns warnings
// for any that expire within the given threshold duration.
func (d *Dir) CheckCertExpiry(threshold time.Duration) []string {
	var warnings []string
	certFiles := []string{"ca.crt", "server.crt", "client.crt"}

	for _, name := range certFiles {
		data, err := d.ReadCert(name)
		if err != nil {
			continue // File doesn't exist
		}
		block, _ := pem.Decode(data)
		if block == nil {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		remaining := time.Until(cert.NotAfter)
		if remaining < threshold {
			warnings = append(warnings, fmt.Sprintf("%s expires in %d days", name, int(remaining.Hours()/24)))
		}
	}
	return warnings
}
```

Update the import block in config.go to include `"crypto/x509"`, `"encoding/pem"`, and `"time"`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/mtls/ -v -run "TestCheckCertExpiry"`
Expected: Both tests PASS.

- [ ] **Step 5: Wire expiry check into serve and connect commands**

In `runServe`, after the `DirStateReady` cert loading block and before the listener starts, add:

```go
		// Check cert expiry
		if warnings := dir.CheckCertExpiry(30 * 24 * time.Hour); len(warnings) > 0 {
			for _, w := range warnings {
				fmt.Fprintf(os.Stderr, "[mtls-bridge] WARNING: %s\n", w)
			}
			fmt.Fprintf(os.Stderr, "[mtls-bridge] Run 'mtls-bridge provision --renew' to regenerate certs.\n")
		}
```

Add the same block in `runConnect` after loading certs in the `DirStateReady` path.

- [ ] **Step 6: Commit**

```bash
git add internal/mtls/config.go internal/mtls/config_test.go cmd/mtls-bridge/main.go
git commit -m "feat: add certificate expiry warnings (30 day threshold)"
```

---

### Task 12: Documentation Update

**Files:**
- Modify: `scripts/README-itunes-mcp.md`

- [ ] **Step 1: Update README**

Replace the contents of `scripts/README-itunes-mcp.md` with:

```markdown
# iTunes MCP Server

An MCP (Model Context Protocol) server that exposes the iTunes COM API on the
Windows machine (`unimatrixzero.local`) for remote control from Claude Code.

## Overview

The server runs as a PowerShell script, wrapped by `mtls-bridge serve` which
provides mTLS encryption over TCP. Claude Code connects via `mtls-bridge connect`
which bridges stdio to the mTLS connection.

## Architecture

```
Claude Code ←stdio→ mtls-bridge connect ←mTLS/TCP→ mtls-bridge serve ←stdio→ PowerShell ←COM→ iTunes
```

## Setup

### Prerequisites

- **Windows machine** with iTunes for Windows installed
- **PowerShell 5.1+** (built into Windows)
- **Shared filesystem** between Mac and Windows (the repo directory)

### 1. Build the bridge binary

On Mac:
```bash
make build-mtls-bridge              # macOS binary
make build-mtls-bridge-windows      # Windows binary (cross-compiled)
```

Copy `mtls-bridge.exe` to the Windows machine (or it's already there via shared filesystem).

### 2. Generate PSK and provision certificates

On either machine (shared filesystem means both see it):
```bash
./mtls-bridge provision --generate-psk
```

Start the server on Windows:
```powershell
.\mtls-bridge.exe serve --powershell "W:\audiobook-organizer\scripts\itunes-mcp-server.ps1"
```

On Mac, run connect (or let Claude Code trigger it via `.mcp.json`):
```bash
./mtls-bridge connect
```

The first connection exchanges the PSK for mTLS certificates. Subsequent connections use mTLS directly.

### 3. Normal usage

Start server on Windows:
```powershell
.\mtls-bridge.exe serve --powershell "W:\audiobook-organizer\scripts\itunes-mcp-server.ps1"
```

Claude Code automatically connects via `.mcp.json` configuration.

### Certificate Management

```bash
# Renew certs (when expiry warning appears)
./mtls-bridge provision --renew

# Full reset (re-provision from scratch)
./mtls-bridge provision --reset --generate-psk
```

## Available Tools

| Tool | Description |
|------|-------------|
| `itunes_open_library(path)` | Set registry and launch iTunes with specified library folder |
| `itunes_close()` | Quit iTunes via COM and release resources |
| `itunes_get_track_count()` | Return total track count |
| `itunes_get_tracks(offset, limit)` | Paginated track list |
| `itunes_verify_files(limit)` | Check if track file locations exist on disk |
| `itunes_get_library_info()` | Library path, iTunes version, track/playlist counts |
| `itunes_search(query, limit)` | Search tracks by name |
| `itunes_run_test(test_folder)` | Run a single test case, return results |

## Troubleshooting

**"no PSK or certs found"**: Run `mtls-bridge provision --generate-psk` first.

**"server unreachable"**: Ensure `mtls-bridge serve` is running on Windows.
Check that `.mtls/server.json` exists and has the correct host/port.

**"TLS handshake failure"**: Certs may be mismatched. Run `mtls-bridge provision --reset --generate-psk` and re-provision.

**"cert expires in N days"**: Run `mtls-bridge provision --renew` on either machine, then restart the server.

**"iTunes COM object failed"**: iTunes may not be installed, or another instance
is running. The server will try to kill existing instances before launching.
```

- [ ] **Step 2: Commit**

```bash
git add scripts/README-itunes-mcp.md
git commit -m "docs: update iTunes MCP readme for mTLS bridge architecture"
```
