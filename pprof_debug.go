//go:build pprof

package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
)

func init() {
	go func() {
		log.Println("[INFO] pprof available at http://localhost:6060/debug/pprof/")
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			log.Printf("[WARN] pprof server failed: %v", err)
		}
	}()
}
