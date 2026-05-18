// file: internal/telemetry/config.go
// version: 1.0.0
// guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d

package telemetry

import "os"

// Config holds OpenTelemetry configuration.
type Config struct {
	ExporterEndpoint string
	ServiceName      string
	Enabled          bool
}

// LoadConfig reads OTEL configuration from environment.
func LoadConfig(serviceName string) *Config {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	return &Config{
		ExporterEndpoint: endpoint,
		ServiceName:      serviceName,
		Enabled:          endpoint != "",
	}
}
