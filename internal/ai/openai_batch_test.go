// file: internal/ai/openai_batch_test.go
// version: 1.0.0
// guid: db58dc83-ecc9-4ef9-916b-7df5e0212459

package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBatchRawResults(t *testing.T) {
	raw := `{"custom_id":"chunk-000","response":{"body":{"choices":[{"message":{"content":"[{\"action\":\"merge_versions\"}]"}}]}}}
{"custom_id":"chunk-001","response":{"body":{"choices":[{"message":{"content":"[{\"action\":\"delete_orphan\"}]"}}]}}}
{"custom_id":"chunk-002","error":{"message":"rate limit exceeded"}}`
	results, err := ParseBatchRawResults([]byte(raw))
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "chunk-000", results[0].CustomID)
	assert.Contains(t, results[0].Content, "merge_versions")
	assert.Equal(t, "chunk-001", results[1].CustomID)
	assert.Contains(t, results[1].Content, "delete_orphan")
	assert.Equal(t, "chunk-002", results[2].CustomID)
	assert.Equal(t, "rate limit exceeded", results[2].Error)
	assert.Empty(t, results[2].Content)
}
