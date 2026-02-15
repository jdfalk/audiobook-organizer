// file: internal/testutil/mock_openlibrary.go
// version: 1.0.0
// guid: c3d4e5f6-a7b8-9012-cdef-345678901abc

package testutil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// MockOpenLibraryServer creates an httptest.Server that mimics OpenLibrary API.
// The responses map keys are matched against the request URL using Contains.
func MockOpenLibraryServer(t *testing.T, responses map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for pattern, body := range responses {
			if strings.Contains(r.URL.String(), pattern) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
				return
			}
		}
		http.NotFound(w, r)
	}))
}

// OpenLibraryHobbitResponse is a standard search response for "The Hobbit".
const OpenLibraryHobbitResponse = `{
	"numFound": 1,
	"start": 0,
	"docs": [{
		"title": "The Hobbit",
		"author_name": ["J.R.R. Tolkien"],
		"first_publish_year": 1937,
		"publisher": ["Houghton Mifflin"],
		"language": ["eng"],
		"isbn": ["0618260307"]
	}]
}`

// OpenLibraryEmptyResponse returns no results.
const OpenLibraryEmptyResponse = `{"numFound":0,"start":0,"docs":[]}`
