// file: internal/mediainfo/mediainfo.go
// version: 1.3.0
// guid: f1e2d3c4-b5a6-7c8d-9e0f-1a2b3c4d5e6f

package mediainfo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
)

// MediaInfo holds technical audio file information
type MediaInfo struct {
	Bitrate    int
	Codec      string
	SampleRate int
	Channels   int
	BitDepth   int
	Quality    string
	Format     string
	Duration   int
}

// Extract reads media information from an audio file.
// It opens the file, reads the tags, and calls BuildFromTag internally.
func Extract(filePath string) (*MediaInfo, error) {
	info := &MediaInfo{}
	ext := strings.ToLower(filepath.Ext(filePath))
	info.Format = strings.TrimPrefix(ext, ".")

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return inferFromFormat(filePath, info)
	}

	fi, err := f.Stat()
	var fileSize int64
	if err == nil {
		fileSize = fi.Size()
	}

	return BuildFromTag(m, filePath, fileSize), nil
}

// BuildFromTag builds a MediaInfo struct from an already-parsed tag.Metadata.
// filePath is used for format inference; fileSize is used for duration estimation.
// The file is not re-opened. This is the shared builder used by ProcessFile (single-pass).
func BuildFromTag(m tag.Metadata, filePath string, fileSize int64) *MediaInfo {
	info := &MediaInfo{}
	ext := strings.ToLower(filepath.Ext(filePath))
	info.Format = strings.TrimPrefix(ext, ".")

	fileType := m.FileType()

	switch fileType {
	case tag.MP3:
		extractMP3Info(m, info)
	case tag.M4A, tag.M4B:
		extractM4AInfo(m, info)
	case tag.FLAC:
		extractFLACInfo(m, info)
	case tag.OGG:
		extractOGGInfo(m, info)
	default:
		// Fall back to format inference — inferFromFormat needs a file path but we
		// already know the size, so we call it and accept it may re-stat the file.
		result, err := inferFromFormat(filePath, info)
		if err != nil {
			// Return a best-effort struct rather than nil
			info.Quality = generateQualityString(info)
			return info
		}
		return result
	}

	// Estimate duration from file size and bitrate when the tag library
	// doesn't provide it directly (which is most cases).
	if info.Duration == 0 && info.Bitrate > 0 && fileSize > 0 {
		// bitrate is in kbps; convert to bytes/sec then divide file size
		bytesPerSec := (info.Bitrate * 1000) / 8
		if bytesPerSec > 0 {
			info.Duration = int(fileSize) / bytesPerSec
		}
	}

	info.Quality = generateQualityString(info)
	return info
}

func extractMP3Info(m tag.Metadata, info *MediaInfo) {
	info.Codec = "MP3"
	raw := m.Raw()

	if bitrate, ok := raw["bitrate"]; ok {
		if br, ok := bitrate.(int); ok {
			info.Bitrate = br / 1000
		}
	}

	if info.Bitrate == 0 {
		info.Bitrate = 192
	}

	if sampleRate, ok := raw["sample_rate"]; ok {
		if sr, ok := sampleRate.(int); ok {
			info.SampleRate = sr
		}
	}
	if info.SampleRate == 0 {
		info.SampleRate = 44100
	}

	info.Channels = 2
}

func extractM4AInfo(m tag.Metadata, info *MediaInfo) {
	info.Codec = "AAC"
	raw := m.Raw()

	if bitrate, ok := raw["bitrate"]; ok {
		if br, ok := bitrate.(int); ok {
			info.Bitrate = br / 1000
		}
	}

	if info.Bitrate == 0 {
		info.Bitrate = 128
	}

	if sampleRate, ok := raw["sample_rate"]; ok {
		if sr, ok := sampleRate.(int); ok {
			info.SampleRate = sr
		}
	}
	if info.SampleRate == 0 {
		info.SampleRate = 44100
	}

	info.Channels = 2
}

func extractFLACInfo(m tag.Metadata, info *MediaInfo) {
	info.Codec = "FLAC"
	info.Channels = 2
	raw := m.Raw()

	if sampleRate, ok := raw["sample_rate"]; ok {
		if sr, ok := sampleRate.(int); ok {
			info.SampleRate = sr
		}
	}
	if info.SampleRate == 0 {
		info.SampleRate = 44100
	}

	if bitDepth, ok := raw["bits_per_sample"]; ok {
		if bd, ok := bitDepth.(int); ok {
			info.BitDepth = bd
		}
	}
	if info.BitDepth == 0 {
		info.BitDepth = 16
	}

	info.Bitrate = (info.SampleRate * info.BitDepth * info.Channels) / 1000
}

func extractOGGInfo(m tag.Metadata, info *MediaInfo) {
	info.Codec = "Vorbis"
	raw := m.Raw()

	if bitrate, ok := raw["bitrate"]; ok {
		if br, ok := bitrate.(int); ok {
			info.Bitrate = br / 1000
		}
	}

	if info.Bitrate == 0 {
		info.Bitrate = 160
	}

	if sampleRate, ok := raw["sample_rate"]; ok {
		if sr, ok := sampleRate.(int); ok {
			info.SampleRate = sr
		}
	}
	if info.SampleRate == 0 {
		info.SampleRate = 44100
	}

	info.Channels = 2
}

func estimateDurationFromFile(filePath string, bitrateKbps int) int {
	if bitrateKbps <= 0 {
		return 0
	}
	fi, err := os.Stat(filePath)
	if err != nil || fi.Size() == 0 {
		return 0
	}
	bytesPerSec := (bitrateKbps * 1000) / 8
	if bytesPerSec <= 0 {
		return 0
	}
	return int(fi.Size()) / bytesPerSec
}

func inferFromFormat(filePath string, info *MediaInfo) (*MediaInfo, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".mp3":
		info.Codec = "MP3"
		info.Bitrate = 192
		info.SampleRate = 44100
		info.Channels = 2
		info.Quality = "192kbps MP3"

	case ".m4a", ".m4b":
		info.Codec = "AAC"
		info.Bitrate = 128
		info.SampleRate = 44100
		info.Channels = 2
		info.Quality = "128kbps AAC"

	case ".flac":
		info.Codec = "FLAC"
		info.SampleRate = 44100
		info.BitDepth = 16
		info.Channels = 2
		info.Bitrate = (44100 * 16 * 2) / 1000
		info.Quality = "FLAC Lossless (16-bit/44.1kHz)"

	case ".ogg", ".oga":
		info.Codec = "Vorbis"
		info.Bitrate = 160
		info.SampleRate = 44100
		info.Channels = 2
		info.Quality = "160kbps Vorbis"

	default:
		return nil, fmt.Errorf("unsupported format: %s", ext)
	}

	// Estimate duration from file size and bitrate
	if info.Duration == 0 && info.Bitrate > 0 {
		info.Duration = estimateDurationFromFile(filePath, info.Bitrate)
	}

	return info, nil
}

func generateQualityString(info *MediaInfo) string {
	if info.Codec == "FLAC" {
		sampleRateKHz := float64(info.SampleRate) / 1000.0
		return fmt.Sprintf("FLAC Lossless (%d-bit/%.1fkHz)", info.BitDepth, sampleRateKHz)
	}

	return fmt.Sprintf("%dkbps %s", info.Bitrate, info.Codec)
}

// GetQualityTier returns a numeric quality tier for comparison
func GetQualityTier(info *MediaInfo) int {
	if info.Codec == "FLAC" {
		if info.BitDepth >= 24 {
			return 100
		}
		return 90
	}

	switch {
	case info.Bitrate >= 320:
		return 80
	case info.Bitrate >= 256:
		return 70
	case info.Bitrate >= 192:
		return 60
	case info.Bitrate >= 128:
		return 50
	default:
		return 30
	}
}
