// file: internal/server/import_service_test.go
// version: 1.0.0
// guid: e1f2a3b4-c5d6-7e8f-9a0b-1c2d3e4f5a6b

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestImportService_ImportFile_MissingFile(t *testing.T) {
	mockDB := &database.MockStore{}
	is := NewImportService(mockDB)

	req := &ImportFileRequest{
		FilePath: "/nonexistent/file.m4b",
	}

	_, err := is.ImportFile(req)

	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestImportService_ImportFile_UnsupportedExtension(t *testing.T) {
	mockDB := &database.MockStore{}
	is := NewImportService(mockDB)

	req := &ImportFileRequest{
		FilePath: "test.txt",
	}

	_, err := is.ImportFile(req)

	if err == nil {
		t.Error("expected error for unsupported file type")
	}
}
