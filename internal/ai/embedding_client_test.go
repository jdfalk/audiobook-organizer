// file: internal/ai/embedding_client_test.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901

package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildEmbeddingText_Book(t *testing.T) {
	result := BuildEmbeddingText("book", "The Way of Kings", "Brandon Sanderson", "Michael Kramer")
	assert.Equal(t, "The Way of Kings by Brandon Sanderson narrated by Michael Kramer", result)
}

func TestBuildEmbeddingText_BookNoNarrator(t *testing.T) {
	result := BuildEmbeddingText("book", "Dune", "Frank Herbert", "")
	assert.Equal(t, "Dune by Frank Herbert", result)
}

func TestBuildEmbeddingText_Author(t *testing.T) {
	result := BuildEmbeddingText("author", "Brandon Sanderson", "", "")
	assert.Equal(t, "Brandon Sanderson", result)
}

func TestTextHash(t *testing.T) {
	h1 := TextHash("hello world")
	h2 := TextHash("hello world")
	h3 := TextHash("different input")

	assert.Equal(t, h1, h2, "same input should produce same hash")
	assert.NotEqual(t, h1, h3, "different input should produce different hash")
	assert.Len(t, h1, 64, "SHA-256 hex digest should be 64 characters")
}
