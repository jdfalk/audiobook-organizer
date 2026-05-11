// file: internal/importer/collision_test.go
// version: 1.0.0
// guid: 6d8e9f0a-1b2c-3d4e-5f6a-7b8c9d0e1f2a
// last-edited: 2026-05-11

package importer

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/merge"
)

// TestCollisionPreviewRequest_FilePath tests that FilePath is set correctly.
func TestCollisionPreviewRequest_FilePath(t *testing.T) {
	req := &CollisionPreviewRequest{
		FilePath:    "/path/to/file.m4b",
		TorrentHash: "",
	}

	if req.FilePath != "/path/to/file.m4b" {
		t.Errorf("expected FilePath='/path/to/file.m4b', got '%s'", req.FilePath)
	}
}

// TestCollisionPreviewRequest_TorrentHash tests that TorrentHash is set correctly.
func TestCollisionPreviewRequest_TorrentHash(t *testing.T) {
	req := &CollisionPreviewRequest{
		FilePath:    "/path/to/file.m4b",
		TorrentHash: "abc123def456",
	}

	if req.TorrentHash != "abc123def456" {
		t.Errorf("expected TorrentHash='abc123def456', got '%s'", req.TorrentHash)
	}
}

// TestCollisionPreviewResult_Structure validates the result fields are properly set.
func TestCollisionPreviewResult_Structure(t *testing.T) {
	candidates := []merge.CollisionCandidate{
		{
			BookID:    "book-1",
			Title:     "Test Book",
			MatchType: "title",
			FilePath:  "/library/test.m4b",
		},
		{
			BookID:    "book-2",
			Title:     "Another Book",
			MatchType: "file_hash",
			FilePath:  "/library/another.m4b",
		},
	}

	result := &CollisionPreviewResult{
		Collisions:   candidates,
		Count:        len(candidates),
		HasCollision: true,
	}

	if result.Count != 2 {
		t.Errorf("expected Count=2, got %d", result.Count)
	}
	if !result.HasCollision {
		t.Error("expected HasCollision=true")
	}
	if len(result.Collisions) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(result.Collisions))
	}
	if result.Collisions[0].BookID != "book-1" {
		t.Errorf("expected first candidate BookID='book-1', got '%s'", result.Collisions[0].BookID)
	}
}

// TestCollisionCandidate_MatchTypeVariants tests that all match types can be properly stored.
func TestCollisionCandidate_MatchTypeVariants(t *testing.T) {
	matchTypes := []string{"title", "file_hash", "fingerprint"}

	for _, matchType := range matchTypes {
		candidate := merge.CollisionCandidate{
			BookID:    "book-test",
			Title:     "Test",
			MatchType: matchType,
		}

		if candidate.MatchType != matchType {
			t.Errorf("expected MatchType='%s', got '%s'", matchType, candidate.MatchType)
		}
	}
}

// TestCollisionPreviewResult_EmptyCollisions tests behavior when there are no collisions.
func TestCollisionPreviewResult_EmptyCollisions(t *testing.T) {
	result := &CollisionPreviewResult{
		Collisions:   []merge.CollisionCandidate{},
		Count:        0,
		HasCollision: false,
	}

	if result.Count != 0 {
		t.Errorf("expected Count=0, got %d", result.Count)
	}
	if result.HasCollision {
		t.Error("expected HasCollision=false")
	}
	if result.Collisions == nil {
		t.Error("expected non-nil Collisions slice")
	}
}
