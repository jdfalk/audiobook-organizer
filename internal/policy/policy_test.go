// file: internal/policy/policy_test.go
// version: 1.0.0
// guid: b2c3d4e5-f6a7-8901-bcde-f12345678901

package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvaluatePolicy_Empty(t *testing.T) {
	p := EvaluatePolicy(nil)
	assert.False(t, p.NoOrganize)
	assert.False(t, p.NoWriteback)
	assert.False(t, p.NoMetadataFetch)
	assert.Empty(t, p.PreferredSource)
	assert.Zero(t, p.Priority)
}

func TestEvaluatePolicy_NoOrganize(t *testing.T) {
	p := EvaluatePolicy([]string{TagNoOrganize})
	assert.True(t, p.NoOrganize)
	assert.False(t, p.NoWriteback)
}

func TestEvaluatePolicy_NoWriteback(t *testing.T) {
	p := EvaluatePolicy([]string{TagNoWriteback})
	assert.True(t, p.NoWriteback)
}

func TestEvaluatePolicy_NoMetadata(t *testing.T) {
	p := EvaluatePolicy([]string{TagNoMetadata})
	assert.True(t, p.NoMetadataFetch)
}

func TestEvaluatePolicy_PreferredSource(t *testing.T) {
	p := EvaluatePolicy([]string{TagSourceAudible})
	assert.Equal(t, "audible", p.PreferredSource)

	p = EvaluatePolicy([]string{TagSourceGoogle})
	assert.Equal(t, "google", p.PreferredSource)
}

func TestEvaluatePolicy_Priority(t *testing.T) {
	p := EvaluatePolicy([]string{TagPriorityHigh})
	assert.Equal(t, 10, p.Priority)

	p = EvaluatePolicy([]string{TagPriorityLow})
	assert.Equal(t, -10, p.Priority)
}

func TestEvaluatePolicy_MultipleFlags(t *testing.T) {
	p := EvaluatePolicy([]string{TagNoOrganize, TagNoWriteback, TagSourceAudible, TagPriorityHigh})
	assert.True(t, p.NoOrganize)
	assert.True(t, p.NoWriteback)
	assert.False(t, p.NoMetadataFetch)
	assert.Equal(t, "audible", p.PreferredSource)
	assert.Equal(t, 10, p.Priority)
}

func TestEvaluatePolicy_UnknownTagsIgnored(t *testing.T) {
	p := EvaluatePolicy([]string{"unknown-tag", "dedup:merge-survivor"})
	assert.False(t, p.NoOrganize)
	assert.False(t, p.NoWriteback)
	assert.Empty(t, p.PreferredSource)
}

func TestKnownPolicyTags_NotEmpty(t *testing.T) {
	tags := KnownPolicyTags()
	assert.NotEmpty(t, tags)
	for _, ti := range tags {
		assert.NotEmpty(t, ti.Tag)
		assert.NotEmpty(t, ti.Description)
	}
}
