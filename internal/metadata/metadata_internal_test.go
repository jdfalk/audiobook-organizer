// file: internal/metadata/metadata_internal_test.go
// version: 1.1.0
// guid: a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d

package metadata

import "testing"

func TestGetRawStringCaseInsensitive(t *testing.T) {
	raw := map[string]interface{}{
		"Publisher": []string{"Podium Audio", "Other"},
	}

	got := getRawString(raw, "publisher")
	if got != "Podium Audio" {
		t.Fatalf("expected Podium Audio, got %q", got)
	}
}

func TestGetRawStringSkipsReleaseGroupTag(t *testing.T) {
	raw := map[string]interface{}{
		"aART": []string{"[PZG]", "Greg Chun"},
	}

	got := getRawString(raw, "aART")
	if got != "Greg Chun" {
		t.Fatalf("expected Greg Chun, got %q", got)
	}
}
