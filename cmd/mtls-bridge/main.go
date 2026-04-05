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
