//go:build pprof

package main

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
)

// pprofAddrEnv names the env var that opts into the pprof debug listener.
// Even in the `pprof` build the listener stays OFF unless this is set, so a
// pprof-enabled binary never silently exposes unauthenticated goroutine dumps,
// heap profiles, source paths, and CPU profiling (pen-test finding HIGH-1).
// To enable for a local debugging session, bind to loopback explicitly:
//
//	ABK_PPROF_ADDR=localhost:6060 ./audiobook-organizer
//
// For a remote host, prefer SSH port-forwarding over binding a public address.
const pprofAddrEnv = "ABK_PPROF_ADDR"

func init() {
	addr := os.Getenv(pprofAddrEnv)
	if addr == "" {
		slog.Info("pprof build active but listener disabled; set " + pprofAddrEnv + " (e.g. localhost:6060) to enable")
		return
	}
	go func() {
		slog.Warn("pprof listener enabled — exposes unauthenticated profiling data; bind to loopback only", "addr", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			slog.Warn("pprof server failed", "error", err)
		}
	}()
}
