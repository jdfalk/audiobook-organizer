// file: internal/mtls/provisioning_test.go
// version: 1.0.0

package mtls

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvisioningExchange(t *testing.T) {
	serverDir := t.TempDir()
	clientDir := t.TempDir()

	psk := "dGVzdHBza3Rlc3Rwc2t0ZXN0cHNrdGVzdHBzaw=="
	os.WriteFile(filepath.Join(serverDir, "psk.txt"), []byte(psk), 0600)
	os.WriteFile(filepath.Join(clientDir, "psk.txt"), []byte(psk), 0600)

	serverD := NewDir(serverDir)
	clientD := NewDir(clientDir)

	ps, err := NewProvisioningServer(serverD, "localhost")
	require.NoError(t, err)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", ps.TLSConfig)
	require.NoError(t, err)
	defer ln.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		serverDone <- ps.HandleConnection(conn)
	}()

	err = RunProvisioningClient(clientD, ln.Addr().String())
	require.NoError(t, err)

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

	sLn, err := tls.Listen("tcp", "127.0.0.1:0", sTLS)
	require.NoError(t, err)
	defer sLn.Close()

	go func() {
		c, err := sLn.Accept()
		if err != nil {
			return
		}
		// Complete the TLS handshake before closing so the client Dial succeeds.
		if tlsConn, ok := c.(*tls.Conn); ok {
			tlsConn.Handshake()
		}
		c.Close()
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
