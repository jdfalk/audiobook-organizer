// file: internal/server/activity_writer_test.go
// version: 1.0.0
// guid: f7e8d9c0-b1a2-4e3f-9c8d-7b6a5e4f3d2c

package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestParseLogLine(t *testing.T) {
	cases := []struct {
		name        string
		line        string
		wantLevel   string
		wantSource  string
		wantMsgPfx  string // prefix check (empty = exact match with wantMsg)
		wantMsg     string // exact match if wantMsgPfx is empty
	}{
		{
			name:       "info scheduler",
			line:       "2026/03/25 17:35:08 logger.go:103: [info] scheduler: Next sync in 28m",
			wantLevel:  "info",
			wantSource: "scheduler",
			wantMsg:    "Next sync in 28m",
		},
		{
			name:       "warn server",
			line:       "2026/03/25 17:35:08 server.go:874: [warn] server: No params found",
			wantLevel:  "warn",
			wantSource: "server",
			wantMsg:    "No params found",
		},
		{
			name:       "debug metadata",
			line:       "2026/03/25 17:35:08 logger.go:103: [debug] metadata: extracting tags",
			wantLevel:  "debug",
			wantSource: "metadata",
			wantMsg:    "extracting tags",
		},
		{
			name:       "gin 200 response",
			line:       "[GIN] 2026/03/25 - 17:35:11 | 200 |    1.44s |    172.16.3.164 | GET      \"/api/v1/health\"",
			wantLevel:  "info",
			wantSource: "gin",
			wantMsgPfx: "200",
		},
		{
			name:       "plain server message",
			line:       "2026/03/25 17:35:08 server.go:965: Starting HTTPS server on 0.0.0.0:8484",
			wantLevel:  "info",
			wantSource: "server",
			wantMsg:    "Starting HTTPS server on 0.0.0.0:8484",
		},
		{
			name:       "unexpected format",
			line:       "something unexpected",
			wantLevel:  "info",
			wantSource: "server",
			wantMsg:    "something unexpected",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			level, source, msg := parseLogLine(tc.line)
			if level != tc.wantLevel {
				t.Errorf("level: got %q, want %q", level, tc.wantLevel)
			}
			if source != tc.wantSource {
				t.Errorf("source: got %q, want %q", source, tc.wantSource)
			}
			if tc.wantMsgPfx != "" {
				if !strings.HasPrefix(msg, tc.wantMsgPfx) {
					t.Errorf("message: got %q, want prefix %q", msg, tc.wantMsgPfx)
				}
			} else if msg != tc.wantMsg {
				t.Errorf("message: got %q, want %q", msg, tc.wantMsg)
			}
		})
	}
}

func TestActivityWriter_CapturesLogs(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "activity_test.db")

	store, err := database.NewActivityStore(dbPath)
	if err != nil {
		t.Fatalf("NewActivityStore: %v", err)
	}
	defer store.Close()

	// Redirect stdout to discard during test to keep output clean
	origStdout := os.Stdout
	devNull, _ := os.Open(os.DevNull)
	os.Stdout = devNull
	defer func() {
		os.Stdout = origStdout
		devNull.Close()
	}()

	w := newActivityWriter(store, 100)
	w.stdout = devNull
	w.Start()

	lines := []string{
		"2026/03/25 17:35:08 logger.go:103: [info] scheduler: Next sync in 28m",
		"2026/03/25 17:35:08 server.go:874: [warn] server: No params found",
		"2026/03/25 17:35:08 logger.go:103: [debug] metadata: extracting tags",
	}

	for _, line := range lines {
		fmt.Fprintln(w, line)
	}

	w.Stop()

	entries, total, err := store.Query(database.ActivityFilter{Limit: 50})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 3 {
		t.Fatalf("total: got %d, want 3", total)
	}
	if len(entries) != 3 {
		t.Fatalf("entries len: got %d, want 3", len(entries))
	}

	// Entries are returned newest-first; map by source for easier checking
	bySource := make(map[string]database.ActivityEntry)
	for _, e := range entries {
		bySource[e.Source] = e
	}

	if e, ok := bySource["scheduler"]; !ok {
		t.Error("missing scheduler entry")
	} else if e.Level != "info" {
		t.Errorf("scheduler level: got %q, want info", e.Level)
	}

	if e, ok := bySource["server"]; !ok {
		t.Error("missing server entry")
	} else if e.Level != "warn" {
		t.Errorf("server level: got %q, want warn", e.Level)
	}

	if e, ok := bySource["metadata"]; !ok {
		t.Error("missing metadata entry")
	} else if e.Level != "debug" {
		t.Errorf("metadata level: got %q, want debug", e.Level)
	}
}

func TestActivityWriter_DropsDebugOnBackpressure(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "activity_bp_test.db")

	store, err := database.NewActivityStore(dbPath)
	if err != nil {
		t.Fatalf("NewActivityStore: %v", err)
	}
	defer store.Close()

	devNull, _ := os.Open(os.DevNull)
	defer devNull.Close()

	// chanSize=2, drain goroutine NOT started — channel will fill up
	w := newActivityWriter(store, 2)
	w.stdout = devNull

	done := make(chan struct{})
	go func() {
		defer close(done)

		// Fill the channel with 2 info entries
		fmt.Fprintln(w, "2026/03/25 17:35:08 server.go:1: [info] server: entry one")
		fmt.Fprintln(w, "2026/03/25 17:35:08 server.go:2: [info] server: entry two")

		// This debug entry should be silently dropped (not block)
		fmt.Fprintln(w, "2026/03/25 17:35:08 logger.go:1: [debug] metadata: should be dropped")
	}()

	select {
	case <-done:
		// success: no hang
	case <-time.After(2 * time.Second):
		t.Fatal("test timed out: Write blocked instead of dropping debug entry")
	}
}
