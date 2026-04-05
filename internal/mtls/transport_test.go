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
	ca, err := GenerateCA(1 * time.Hour)
	require.NoError(t, err)
	serverKP, err := GenerateSignedCert(ca, "server", []string{"localhost"}, 1*time.Hour)
	require.NoError(t, err)
	clientKP, err := GenerateSignedCert(ca, "client", nil, 1*time.Hour)
	require.NoError(t, err)

	serverTLS, err := ServerTLSConfig(ca.CertPEM, serverKP.CertPEM, serverKP.KeyPEM)
	require.NoError(t, err)
	clientTLS, err := ClientTLSConfig(ca.CertPEM, clientKP.CertPEM, clientKP.KeyPEM, "localhost")
	require.NoError(t, err)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(conn, conn)
	}()

	conn, err := tls.Dial("tcp", ln.Addr().String(), clientTLS)
	require.NoError(t, err)
	defer conn.Close()

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

	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	noClientTLS := &tls.Config{
		RootCAs:    serverTLS.ClientCAs,
		ServerName: "localhost",
	}
	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 2*time.Second)
	require.NoError(t, err)
	tlsConn := tls.Client(conn, noClientTLS)
	err = tlsConn.Handshake()
	assert.Error(t, err)
	conn.Close()
}
