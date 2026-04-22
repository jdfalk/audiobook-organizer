// file: cmd/pebble-inject-skip/main.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-e1f2a3b4c5d6
//
// One-shot tool: writes transcode skip flags directly into PebbleDB.
// Run while the service is stopped.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/cockroachdb/pebble/v2"
)

type Setting struct {
	Key      string
	Value    string
	Type     string
	IsSecret bool
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: pebble-inject-skip <pebble-db-path>")
	}
	dbPath := os.Args[1]

	db, err := pebble.Open(dbPath, &pebble.Options{})
	if err != nil {
		log.Fatalf("open pebble: %v", err)
	}
	defer db.Close()

	keys := []string{
		"transcode_skip_5a89c8d2857e4899", // David Petrie
		"transcode_skip_2b2d16b94596fe1a", // Eric Ugland "One More Last Time"
	}

	for _, k := range keys {
		s := Setting{Key: k, Value: "true", Type: "bool", IsSecret: false}
		val, err := json.Marshal(s)
		if err != nil {
			log.Fatalf("marshal %s: %v", k, err)
		}
		dbKey := "setting:" + k
		if err := db.Set([]byte(dbKey), val, pebble.Sync); err != nil {
			log.Fatalf("set %s: %v", dbKey, err)
		}
		fmt.Printf("SET %s = %s\n", dbKey, val)
	}

	fmt.Println("Done.")
}
