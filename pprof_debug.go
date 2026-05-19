//go:build pprof

package main

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof"
)

func init() {
	go func() {
		slog.Info("pprof available", "addr", "http://localhost:6060/debug/pprof/")
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			slog.Warn("pprof server failed", "error", err)
		}
	}()
}
