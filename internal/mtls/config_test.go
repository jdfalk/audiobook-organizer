// file: internal/mtls/config_test.go
// version: 1.1.0

package mtls

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
	ca, err := GenerateCA(10 * 24 * time.Hour)
	require.NoError(t, err)

	dir := t.TempDir()
	d := NewDir(dir)
	d.WriteCert("ca.crt", ca.CertPEM)

	warnings := d.CheckCertExpiry(30 * 24 * time.Hour)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "ca.crt")
}
