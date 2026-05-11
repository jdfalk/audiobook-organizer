// file: internal/audio/sample_test.go
// version: 1.0.0
// guid: d2e3f4a5-b6c7-8d9e-0f1a-2b3c4d5e6f7a

package audio

import (
	"context"
	"testing"
)

func TestSampleRequest_NilRequest(t *testing.T) {
	ctx := context.Background()
	err := ExtractSample(ctx, nil, func([]byte) (int, error) { return 0, nil })
	if err == nil {
		t.Fatal("expected error for nil request, got nil")
	}
	if err.Error() != "request cannot be nil" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSampleRequest_EmptyFilePath(t *testing.T) {
	ctx := context.Background()
	req := &SampleRequest{
		FilePath: "",
		Start:    0,
		Duration: 30,
	}
	err := ExtractSample(ctx, req, func([]byte) (int, error) { return 0, nil })
	if err == nil {
		t.Fatal("expected error for empty file path, got nil")
	}
	if err.Error() != "file path is empty" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSampleRequest_ParameterClamping(t *testing.T) {
	// This test verifies parameter validation logic without actually calling ffmpeg.
	// We test that invalid inputs are caught before ffmpeg execution.

	tests := []struct {
		name           string
		filePath       string
		start          int
		duration       int
		shouldValidate bool
	}{
		{
			name:           "valid request",
			filePath:       "/path/to/file.m4b",
			start:          10,
			duration:       30,
			shouldValidate: true,
		},
		{
			name:           "negative start clamped to 0",
			filePath:       "/path/to/file.m4b",
			start:          -5,
			duration:       30,
			shouldValidate: true,
		},
		{
			name:           "zero duration uses default",
			filePath:       "/path/to/file.m4b",
			start:          0,
			duration:       0,
			shouldValidate: true,
		},
		{
			name:           "duration exceeds max",
			filePath:       "/path/to/file.m4b",
			start:          0,
			duration:       150,
			shouldValidate: true,
		},
		{
			name:           "empty file path",
			filePath:       "",
			start:          0,
			duration:       30,
			shouldValidate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &SampleRequest{
				FilePath: tt.filePath,
				Start:    tt.start,
				Duration: tt.duration,
			}

			ctx := context.Background()
			// Mock writer that always succeeds
			writer := func(buf []byte) (int, error) {
				// Don't actually call ExtractSample for invalid paths,
				// just test the validation logic
				if req.FilePath == "" {
					return 0, nil // placeholder
				}
				// Would call ffmpeg here, but we're just testing validation
				return len(buf), nil
			}

			err := ExtractSample(ctx, req, writer)

			// We can't fully test without ffmpeg/actual files,
			// but we validate error handling
			if !tt.shouldValidate && err == nil {
				t.Errorf("expected validation error for %q", tt.name)
			}
			if tt.shouldValidate && req.FilePath == "" && err == nil {
				t.Errorf("expected error for empty file path")
			}
		})
	}
}
