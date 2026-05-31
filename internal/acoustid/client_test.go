// file: internal/acoustid/client_test.go
// version: 1.0.0
// guid: 7f809182-9a2b-3c4d-5e6f-7081a2b3c4d5
// last-edited: 2026-05-31

package acoustid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLookup_NoAPIKey(t *testing.T) {
	c := NewClient("")
	_, err := c.Lookup(context.Background(), "abc", 600)
	if err != ErrNoAPIKey {
		t.Fatalf("expected ErrNoAPIKey, got %v", err)
	}
}

func TestLookup_TopScoreWins(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("client") != "TESTKEY" {
			t.Errorf("client param missing")
		}
		if r.FormValue("duration") != "600" {
			t.Errorf("duration param wrong: %q", r.FormValue("duration"))
		}
		if r.FormValue("fingerprint") != "FP" {
			t.Errorf("fingerprint param wrong")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": "ok",
			"results": [
				{"id":"acoustid-1","score":0.42,"recordings":[{"id":"mb-low"}]},
				{"id":"acoustid-2","score":0.91,"recordings":[{"id":"mb-high"}]},
				{"id":"acoustid-3","score":0.55,"recordings":[{"id":"mb-mid"}]}
			]
		}`))
	}))
	defer srv.Close()

	c := NewClient("TESTKEY")
	c.BaseURL = srv.URL

	res, err := c.Lookup(context.Background(), "FP", 600)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if res.RecordingID != "mb-high" {
		t.Errorf("RecordingID: got %q want mb-high", res.RecordingID)
	}
	if res.Score != 0.91 {
		t.Errorf("Score: got %v want 0.91", res.Score)
	}
}

func TestLookup_NoResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","results":[]}`))
	}))
	defer srv.Close()

	c := NewClient("TESTKEY")
	c.BaseURL = srv.URL
	res, err := c.Lookup(context.Background(), "FP", 600)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if res.RecordingID != "" || res.Score != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
}

func TestLookup_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","error":{"code":3,"message":"invalid fingerprint"}}`))
	}))
	defer srv.Close()

	c := NewClient("TESTKEY")
	c.BaseURL = srv.URL
	_, err := c.Lookup(context.Background(), "FP", 600)
	if err == nil || !strings.Contains(err.Error(), "invalid fingerprint") {
		t.Fatalf("expected API error, got %v", err)
	}
}

func TestLookup_HTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("server down"))
	}))
	defer srv.Close()

	c := NewClient("TESTKEY")
	c.BaseURL = srv.URL
	_, err := c.Lookup(context.Background(), "FP", 600)
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected HTTP error, got %v", err)
	}
}
