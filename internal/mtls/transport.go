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
