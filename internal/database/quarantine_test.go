// file: internal/database/quarantine_test.go
// version: 1.0.0

package database_test

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/stretchr/testify/require"
)

func TestBookQuarantineFields(t *testing.T) {
	reason := "taglib cannot parse file"
	b := database.Book{
		ID:               "test-id",
		Title:            "Test Book",
		FilePath:         "/library/.failed/Author/Book/book.m4b",
		QuarantineReason: &reason,
	}
	require.NotNil(t, b.QuarantineReason)
	require.Equal(t, "taglib cannot parse file", *b.QuarantineReason)
	require.Nil(t, b.QuarantinedAt)
}
