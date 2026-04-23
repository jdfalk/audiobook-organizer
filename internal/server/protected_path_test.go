// file: internal/server/protected_path_test.go
// version: 1.0.0

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsProtectedPath_FailedDir(t *testing.T) {
	cases := []struct {
		path     string
		expected bool
	}{
		{"/library/.failed/Author/Book/book.m4b", true},
		{"/library/.failed/book.m4b", true},
		{"/library/Author/Book/book.m4b", false},
		{"/library/.failedish/book.m4b", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.expected, isProtectedPath(tc.path), "path: %s", tc.path)
	}
}
