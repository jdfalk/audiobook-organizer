// file: internal/server/validate_path_test.go
// version: 1.0.0
// guid: a1b2c3d4-e5f6-7890-abcd-ef1234567890
// last-edited: 2026-05-16

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateAbsolutePath_Valid(t *testing.T) {
	cases := []string{
		"/tmp/audio.m4b",
		"/mnt/bigdata/books/itunes/iTunes Library.xml",
		"/var/lib/audiobook-organizer",
	}
	for _, p := range cases {
		assert.NoError(t, validateAbsolutePath(p), "expected no error for %q", p)
	}
}

func TestValidateAbsolutePath_Relative(t *testing.T) {
	err := validateAbsolutePath("relative/path/to/file.xml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")
}

func TestValidateAbsolutePath_TraversalSequence(t *testing.T) {
	cases := []string{
		"/tmp/../../etc/passwd",
		"/mnt/books/../../../root/.ssh/authorized_keys",
	}
	for _, p := range cases {
		err := validateAbsolutePath(p)
		assert.Error(t, err, "expected error for traversal path %q", p)
	}
}

func TestValidateAbsolutePath_Empty(t *testing.T) {
	err := validateAbsolutePath("")
	assert.Error(t, err)
}
