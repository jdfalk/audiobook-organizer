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
	subIn, subInW := io.Pipe()
	subOutR, subOut := io.Pipe()

	server, client := net.Pipe()

	// Subprocess: echo back
	go func() {
		io.Copy(subOut, subIn)
		subOut.Close()
	}()

	done := make(chan error, 1)
	go func() {
		done <- Bridge(server, subOutR, subInW)
	}()

	msg := []byte("Content-Length: 5\r\n\r\nhello")
	_, err := client.Write(msg)
	require.NoError(t, err)

	buf := make([]byte, len(msg))
	_, err = io.ReadFull(client, buf)
	require.NoError(t, err)
	assert.Equal(t, msg, buf)

	client.Close()

	select {
	case err := <-done:
		if err != nil && err != io.EOF {
			t.Errorf("unexpected bridge error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("bridge did not return")
	}
}

func TestBridgeStdio(t *testing.T) {
	stdinData := []byte("request data")
	stdin := bytes.NewReader(stdinData)
	stdout := &bytes.Buffer{}

	server, client := net.Pipe()
	go func() {
		io.Copy(server, server)
		server.Close()
	}()

	err := BridgeStdio(client, stdin, stdout)
	assert.Nil(t, err)
	assert.Equal(t, stdinData, stdout.Bytes())
}
