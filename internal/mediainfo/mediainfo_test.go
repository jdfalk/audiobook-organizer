// file: internal/mediainfo/mediainfo_test.go
// version: 1.0.0
// guid: a2b3c4d5-e6f7-8a9b-0c1d-2e3f4a5b6c7d

package mediainfo

import "testing"

func TestGenerateQualityString(t *testing.T) {
	info := &MediaInfo{Codec: "MP3", Bitrate: 320}
	result := generateQualityString(info)
	expected := "320kbps MP3"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestGetQualityTier(t *testing.T) {
	info := &MediaInfo{Codec: "FLAC", BitDepth: 24}
	tier := GetQualityTier(info)
	if tier != 100 {
		t.Errorf("expected 100, got %d", tier)
	}
}
