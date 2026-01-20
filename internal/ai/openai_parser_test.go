// file: internal/ai/openai_parser_test.go
// version: 1.0.1
// guid: 1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d

package ai

import (
	"testing"
)

func TestNewOpenAIParser_Disabled(t *testing.T) {
	// Test with empty API key
	parser := NewOpenAIParser("", true)
	if parser.enabled {
		t.Error("Expected parser to be disabled with empty API key")
	}

	// Test with enabled=false
	parser = NewOpenAIParser("test-key", false)
	if parser.enabled {
		t.Error("Expected parser to be disabled when enabled=false")
	}
}
