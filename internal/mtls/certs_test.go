// file: internal/mtls/certs_test.go
// version: 1.1.0

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

func TestLoadKeyPair(t *testing.T) {
	ca, err := GenerateCA(1 * time.Hour)
	require.NoError(t, err)

	loaded, err := LoadKeyPair(ca.CertPEM, ca.KeyPEM)
	require.NoError(t, err)
	assert.Equal(t, ca.Cert.Subject.CommonName, loaded.Cert.Subject.CommonName)
	assert.True(t, loaded.Cert.IsCA)
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
