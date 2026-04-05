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

// ProvisioningServer handles one-time PSK-authenticated cert exchange.
type ProvisioningServer struct {
	dir       *Dir
	hostname  string
	TLSConfig *tls.Config
}

// NewProvisioningServer creates a ProvisioningServer with an ephemeral TLS config.
func NewProvisioningServer(dir *Dir, hostname string) (*ProvisioningServer, error) {
	tlsCfg, err := EphemeralTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("create ephemeral TLS: %w", err)
	}
	return &ProvisioningServer{dir: dir, hostname: hostname, TLSConfig: tlsCfg}, nil
}

// HandleConnection processes a single provisioning connection: verifies PSK,
// generates CA + server + client certs, writes server-side certs to disk,
// and sends client-side certs over the connection.
func (ps *ProvisioningServer) HandleConnection(conn net.Conn) error {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

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

	serverPSK, err := ps.dir.ReadPSK()
	if err != nil {
		return fmt.Errorf("read server PSK: %w", err)
	}

	if subtle.ConstantTimeCompare([]byte(pskMsg.PSK), []byte(serverPSK)) != 1 {
		errData, _ := json.Marshal(errorMessage{Error: "PSK mismatch"})
		enc.Encode(provisioningMessage{Type: "error", Data: errData})
		return fmt.Errorf("PSK mismatch")
	}

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

	certsData, _ := json.Marshal(certsMessage{
		CACert:    string(ca.CertPEM),
		ClientCrt: string(clientCert.CertPEM),
		ClientKey: string(clientCert.KeyPEM),
	})
	if err := enc.Encode(provisioningMessage{Type: "certs", Data: certsData}); err != nil {
		return fmt.Errorf("send certs: %w", err)
	}

	ps.dir.DeletePSK()
	return nil
}

// RunProvisioningClient connects to the provisioning server at addr, authenticates
// with the PSK from dir, and writes the returned CA + client certs to dir.
func RunProvisioningClient(dir *Dir, addr string) error {
	psk, err := dir.ReadPSK()
	if err != nil {
		return fmt.Errorf("read PSK: %w", err)
	}

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

	pskData, _ := json.Marshal(pskMessage{PSK: psk})
	if err := enc.Encode(provisioningMessage{Type: "psk", Data: pskData}); err != nil {
		return fmt.Errorf("send PSK: %w", err)
	}

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

	if err := dir.WriteCert("ca.crt", []byte(certs.CACert)); err != nil {
		return err
	}
	if err := dir.WriteCert("client.crt", []byte(certs.ClientCrt)); err != nil {
		return err
	}
	if err := dir.WriteCert("client.key", []byte(certs.ClientKey)); err != nil {
		return err
	}

	dir.DeletePSK()
	return nil
}
