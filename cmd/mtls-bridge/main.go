// file: cmd/mtls-bridge/main.go
// version: 2.0.0

package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	mtls "github.com/jdfalk/audiobook-organizer/internal/mtls"
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
	serveCmd.MarkFlagRequired("powershell") //nolint:errcheck

	provisionCmd.Flags().BoolVar(&generatePSK, "generate-psk", false, "Generate a new pre-shared key")
	provisionCmd.Flags().BoolVar(&renewCerts, "renew", false, "Renew certs from existing CA")
	provisionCmd.Flags().BoolVar(&resetAll, "reset", false, "Delete all certs and optionally regenerate PSK")

	rootCmd.AddCommand(serveCmd, connectCmd, provisionCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	dir := mtls.NewDir(mtlsDir)

	state, err := dir.State()
	if err != nil {
		return fmt.Errorf("read dir state: %w", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}

	var listener net.Listener

	switch state {
	case mtls.DirStateEmpty:
		return fmt.Errorf("no PSK found — run 'provision --generate-psk' first")

	case mtls.DirStateProvisioning:
		fmt.Fprintf(os.Stderr, "[mtls-bridge] entering provisioning mode\n")
		ps, err := mtls.NewProvisioningServer(dir, hostname)
		if err != nil {
			return fmt.Errorf("create provisioning server: %w", err)
		}
		listener, err = tls.Listen("tcp", listenHost+":0", ps.TLSConfig)
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		fmt.Fprintf(os.Stderr, "[mtls-bridge] provisioning listener on :%d\n", port)

		if err := dir.WriteServerInfo(mtls.ServerInfo{Host: hostname, Port: port}); err != nil {
			listener.Close()
			return fmt.Errorf("write server.json: %w", err)
		}

		conn, err := listener.Accept()
		if err != nil {
			listener.Close()
			return fmt.Errorf("accept provisioning connection: %w", err)
		}
		if err := ps.HandleConnection(conn); err != nil {
			listener.Close()
			return fmt.Errorf("provisioning: %w", err)
		}
		listener.Close()
		fmt.Fprintf(os.Stderr, "[mtls-bridge] provisioning complete, switching to mTLS\n")

		// Re-check state — should now be Ready
		state, err = dir.State()
		if err != nil || state != mtls.DirStateReady {
			return fmt.Errorf("expected ready state after provisioning, got %d", state)
		}
		// Fall through to start mTLS listener below

	case mtls.DirStateReady:
		// handled below
	}

	// Load certs and create mTLS listener
	caCert, err := dir.ReadCert("ca.crt")
	if err != nil {
		return fmt.Errorf("read ca.crt: %w", err)
	}
	serverCert, err := dir.ReadCert("server.crt")
	if err != nil {
		return fmt.Errorf("read server.crt: %w", err)
	}
	serverKey, err := dir.ReadCert("server.key")
	if err != nil {
		return fmt.Errorf("read server.key: %w", err)
	}

	// Check cert expiry
	if warnings := dir.CheckCertExpiry(30 * 24 * time.Hour); len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] WARNING: %s\n", w)
		}
		fmt.Fprintf(os.Stderr, "[mtls-bridge] Run 'mtls-bridge provision --renew' to regenerate certs.\n")
	}

	tlsCfg, err := mtls.ServerTLSConfig(caCert, serverCert, serverKey)
	if err != nil {
		return fmt.Errorf("create TLS config: %w", err)
	}

	listener, err = tls.Listen("tcp", listenHost+":0", tlsCfg)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	fmt.Fprintf(os.Stderr, "[mtls-bridge] mTLS listener on :%d\n", port)

	if err := dir.WriteServerInfo(mtls.ServerInfo{Host: hostname, Port: port}); err != nil {
		return fmt.Errorf("write server.json: %w", err)
	}

	// Accept connections in a loop
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] accept error: %v\n", err)
			continue
		}
		fmt.Fprintf(os.Stderr, "[mtls-bridge] accepted connection from %s\n", conn.RemoteAddr())

		// Spawn PowerShell subprocess
		ps := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", powershellPath)
		ps.Stderr = os.Stderr

		stdin, err := ps.StdinPipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] stdin pipe: %v\n", err)
			conn.Close()
			continue
		}
		stdout, err := ps.StdoutPipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] stdout pipe: %v\n", err)
			conn.Close()
			continue
		}

		if err := ps.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] start powershell: %v\n", err)
			conn.Close()
			continue
		}

		if err := mtls.Bridge(conn, stdout, stdin); err != nil {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] bridge error: %v\n", err)
		}

		if err := ps.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] powershell exit: %v\n", err)
		}

		fmt.Fprintf(os.Stderr, "[mtls-bridge] connection closed\n")
	}
}

func runConnect(cmd *cobra.Command, args []string) error {
	dir := mtls.NewDir(mtlsDir)

	state, err := dir.State()
	if err != nil {
		return fmt.Errorf("read dir state: %w", err)
	}

	switch state {
	case mtls.DirStateEmpty:
		return fmt.Errorf("no PSK found — run 'provision --generate-psk' first")

	case mtls.DirStateProvisioning:
		fmt.Fprintf(os.Stderr, "[mtls-bridge] running provisioning client\n")
		info, err := dir.ReadServerInfo()
		if err != nil {
			return fmt.Errorf("read server.json: %w", err)
		}
		addr := fmt.Sprintf("%s:%d", info.Host, info.Port)
		if err := mtls.RunProvisioningClient(dir, addr); err != nil {
			return fmt.Errorf("provisioning: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[mtls-bridge] provisioning complete, waiting for server restart\n")
		time.Sleep(2 * time.Second)

	case mtls.DirStateReady:
		// proceed directly
	}

	// Load certs and connect
	caCert, err := dir.ReadCert("ca.crt")
	if err != nil {
		return fmt.Errorf("read ca.crt: %w", err)
	}
	clientCert, err := dir.ReadCert("client.crt")
	if err != nil {
		return fmt.Errorf("read client.crt: %w", err)
	}
	clientKey, err := dir.ReadCert("client.key")
	if err != nil {
		return fmt.Errorf("read client.key: %w", err)
	}

	info, err := dir.ReadServerInfo()
	if err != nil {
		return fmt.Errorf("read server.json: %w", err)
	}

	// Check cert expiry
	if warnings := dir.CheckCertExpiry(30 * 24 * time.Hour); len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "[mtls-bridge] WARNING: %s\n", w)
		}
		fmt.Fprintf(os.Stderr, "[mtls-bridge] Run 'mtls-bridge provision --renew' to regenerate certs.\n")
	}

	tlsCfg, err := mtls.ClientTLSConfig(caCert, clientCert, clientKey, info.Host)
	if err != nil {
		return fmt.Errorf("create TLS config: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", info.Host, info.Port)
	fmt.Fprintf(os.Stderr, "[mtls-bridge] connecting to %s\n", addr)

	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	fmt.Fprintf(os.Stderr, "[mtls-bridge] connected, bridging stdio\n")
	return mtls.BridgeStdio(conn, os.Stdin, os.Stdout)
}

func runProvision(cmd *cobra.Command, args []string) error {
	dir := mtls.NewDir(mtlsDir)

	if !generatePSK && !renewCerts && !resetAll {
		return fmt.Errorf("specify --generate-psk, --renew, or --reset")
	}

	if resetAll {
		fmt.Fprintf(os.Stderr, "[mtls-bridge] resetting all files in %s\n", mtlsDir)
		if err := dir.Reset(); err != nil {
			return fmt.Errorf("reset: %w", err)
		}
		if generatePSK {
			psk, err := dir.GeneratePSK()
			if err != nil {
				return fmt.Errorf("generate PSK: %w", err)
			}
			fmt.Fprintf(os.Stderr, "[mtls-bridge] new PSK: %s\n", psk)
		}
		return nil
	}

	if generatePSK {
		psk, err := dir.GeneratePSK()
		if err != nil {
			return fmt.Errorf("generate PSK: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[mtls-bridge] PSK: %s\n", psk)
		return nil
	}

	if renewCerts {
		fmt.Fprintf(os.Stderr, "[mtls-bridge] renewing certificates\n")
		caKeyPEM, err := dir.ReadCert("ca.key")
		if err != nil {
			return fmt.Errorf("read ca.key: %w", err)
		}
		caCertPEM, err := dir.ReadCert("ca.crt")
		if err != nil {
			return fmt.Errorf("read ca.crt: %w", err)
		}

		ca, err := mtls.LoadKeyPair(caCertPEM, caKeyPEM)
		if err != nil {
			return fmt.Errorf("load CA keypair: %w", err)
		}

		hostname, err := os.Hostname()
		if err != nil {
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

		fmt.Fprintf(os.Stderr, "[mtls-bridge] certificates renewed\n")
		return nil
	}

	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
