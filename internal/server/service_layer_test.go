// file: internal/server/service_layer_test.go
// version: 1.0.0

package server

import (
	"testing"
	"time"

	"github.com/jdfalk/audiobook-organizer/internal/config"
	"github.com/jdfalk/audiobook-organizer/internal/database"
	"github.com/jdfalk/audiobook-organizer/internal/database/mocks"
)

// TestConfigUpdateService_MaskSecrets tests MaskSecrets method
func TestConfigUpdateService_MaskSecrets(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewConfigUpdateService(mockStore)

	tests := []struct {
		name     string
		config   config.Config
		expected config.Config
	}{
		{
			name: "mask openai api key",
			config: config.Config{
				OpenAIAPIKey: "sk-1234567890abcdef",
			},
			expected: config.Config{
				OpenAIAPIKey: "sk-****cdef",
			},
		},
		{
			name: "mask goodreads api key",
			config: config.Config{
				APIKeys: struct {
					Goodreads string `json:"goodreads"`
				}{
					Goodreads: "gr-1234567890abcdef",
				},
			},
			expected: config.Config{
				APIKeys: struct {
					Goodreads string `json:"goodreads"`
				}{
					Goodreads: "gr-****cdef",
				},
			},
		},
		{
			name: "short secrets get masked to ****",
			config: config.Config{
				OpenAIAPIKey: "short",
			},
			expected: config.Config{
				OpenAIAPIKey: "****",
			},
		},
		{
			name: "empty secrets remain empty",
			config: config.Config{
				OpenAIAPIKey: "",
			},
			expected: config.Config{
				OpenAIAPIKey: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			masked := svc.MaskSecrets(tt.config)
			if masked.OpenAIAPIKey != tt.expected.OpenAIAPIKey {
				t.Errorf("expected OpenAIAPIKey %q, got %q", tt.expected.OpenAIAPIKey, masked.OpenAIAPIKey)
			}
			if masked.APIKeys.Goodreads != tt.expected.APIKeys.Goodreads {
				t.Errorf("expected Goodreads %q, got %q", tt.expected.APIKeys.Goodreads, masked.APIKeys.Goodreads)
			}
		})
	}
}

// TestConfigUpdateService_ApplyUpdates_ErrorCases tests error cases for ApplyUpdates
func TestConfigUpdateService_ApplyUpdates_ErrorCases(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewConfigUpdateService(mockStore)

	tests := []struct {
		name      string
		payload   map[string]any
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "nil payload returns error",
			payload:   nil,
			wantErr:   true,
			errSubstr: "configuration payload is required",
		},
		{
			name:      "database_type change rejected",
			payload:   map[string]any{"database_type": "mysql"},
			wantErr:   true,
			errSubstr: "database_type cannot be changed at runtime",
		},
		{
			name:      "enable_sqlite change rejected",
			payload:   map[string]any{"enable_sqlite": true},
			wantErr:   true,
			errSubstr: "enable_sqlite cannot be changed at runtime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ApplyUpdates(tt.payload)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errSubstr != "" && !contains(err.Error(), tt.errSubstr) {
					t.Errorf("expected error to contain %q, got %q", tt.errSubstr, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestConfigUpdateService_ApplyUpdates_ArrayFields tests array field updates
func TestConfigUpdateService_ApplyUpdates_ArrayFields(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewConfigUpdateService(mockStore)

	originalPatterns := config.AppConfig.ExcludePatterns
	originalExtensions := config.AppConfig.SupportedExtensions
	defer func() {
		config.AppConfig.ExcludePatterns = originalPatterns
		config.AppConfig.SupportedExtensions = originalExtensions
	}()

	payload := map[string]any{
		"exclude_patterns":      []any{"*.tmp", "*.bak", "*.cache"},
		"supported_extensions":  []any{".mp3", ".m4b", ".m4a"},
	}

	err := svc.ApplyUpdates(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(config.AppConfig.ExcludePatterns) != 3 {
		t.Errorf("expected 3 exclude patterns, got %d", len(config.AppConfig.ExcludePatterns))
	}
	if config.AppConfig.ExcludePatterns[0] != "*.tmp" {
		t.Errorf("expected first pattern '*.tmp', got %q", config.AppConfig.ExcludePatterns[0])
	}

	if len(config.AppConfig.SupportedExtensions) != 3 {
		t.Errorf("expected 3 supported extensions, got %d", len(config.AppConfig.SupportedExtensions))
	}
	if config.AppConfig.SupportedExtensions[0] != ".mp3" {
		t.Errorf("expected first extension '.mp3', got %q", config.AppConfig.SupportedExtensions[0])
	}
}

// TestAudiobookUpdateService_ExtractBoolField tests ExtractBoolField method
func TestAudiobookUpdateService_ExtractBoolField(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookUpdateService(mockStore)

	tests := []struct {
		name      string
		payload   map[string]any
		key       string
		wantValue bool
		wantOK    bool
	}{
		{
			name:      "extract true",
			payload:   map[string]any{"verified": true},
			key:       "verified",
			wantValue: true,
			wantOK:    true,
		},
		{
			name:      "extract false",
			payload:   map[string]any{"verified": false},
			key:       "verified",
			wantValue: false,
			wantOK:    true,
		},
		{
			name:      "missing key",
			payload:   map[string]any{"other": true},
			key:       "verified",
			wantValue: false,
			wantOK:    false,
		},
		{
			name:      "wrong type (string instead of bool)",
			payload:   map[string]any{"verified": "true"},
			key:       "verified",
			wantValue: false,
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := svc.ExtractBoolField(tt.payload, tt.key)
			if value != tt.wantValue {
				t.Errorf("expected value %v, got %v", tt.wantValue, value)
			}
			if ok != tt.wantOK {
				t.Errorf("expected ok %v, got %v", tt.wantOK, ok)
			}
		})
	}
}

// TestAudiobookUpdateService_ExtractOverrides_EdgeCases tests edge cases for ExtractOverrides
func TestAudiobookUpdateService_ExtractOverrides_EdgeCases(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookUpdateService(mockStore)

	tests := []struct {
		name      string
		payload   map[string]any
		wantOK    bool
		wantCount int
	}{
		{
			name:      "missing overrides key",
			payload:   map[string]any{"other": "value"},
			wantOK:    false,
			wantCount: 0,
		},
		{
			name:      "wrong type (not a map)",
			payload:   map[string]any{"overrides": "not a map"},
			wantOK:    false,
			wantCount: 0,
		},
		{
			name:      "empty overrides map",
			payload:   map[string]any{"overrides": map[string]any{}},
			wantOK:    true,
			wantCount: 0,
		},
		{
			name: "valid overrides with multiple fields",
			payload: map[string]any{
				"overrides": map[string]any{
					"title":  "value1",
					"author": "value2",
					"year":   "value3",
				},
			},
			wantOK:    true,
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := svc.ExtractOverrides(tt.payload)
			if ok != tt.wantOK {
				t.Errorf("expected ok %v, got %v", tt.wantOK, ok)
			}
			if ok && len(value) != tt.wantCount {
				t.Errorf("expected %d overrides, got %d", tt.wantCount, len(value))
			}
		})
	}
}

// TestAudiobookUpdateService_ApplyUpdatesToBook_AllFields tests applying all field types
func TestAudiobookUpdateService_ApplyUpdatesToBook_AllFields(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewAudiobookUpdateService(mockStore)

	book := &database.Book{
		ID:    "book-123",
		Title: "Original Title",
	}

	updates := map[string]any{
		"title":                  "New Title",
		"author_id":              float64(42),
		"series_id":              float64(10),
		"narrator":               "John Doe",
		"publisher":              "Test Publisher",
		"language":               "en",
		"audiobook_release_year": float64(2023),
		"isbn10":                 "1234567890",
		"isbn13":                 "9781234567890",
	}

	svc.ApplyUpdatesToBook(book, updates)

	if book.Title != "New Title" {
		t.Errorf("expected title 'New Title', got %q", book.Title)
	}
	if book.AuthorID == nil || *book.AuthorID != 42 {
		t.Errorf("expected author_id 42, got %v", book.AuthorID)
	}
	if book.SeriesID == nil || *book.SeriesID != 10 {
		t.Errorf("expected series_id 10, got %v", book.SeriesID)
	}
	if book.Narrator == nil || *book.Narrator != "John Doe" {
		t.Errorf("expected narrator 'John Doe', got %v", book.Narrator)
	}
	if book.Publisher == nil || *book.Publisher != "Test Publisher" {
		t.Errorf("expected publisher 'Test Publisher', got %v", book.Publisher)
	}
	if book.Language == nil || *book.Language != "en" {
		t.Errorf("expected language 'en', got %v", book.Language)
	}
	if book.AudiobookReleaseYear == nil || *book.AudiobookReleaseYear != 2023 {
		t.Errorf("expected audiobook_release_year 2023, got %v", book.AudiobookReleaseYear)
	}
	if book.ISBN10 == nil || *book.ISBN10 != "1234567890" {
		t.Errorf("expected isbn10 '1234567890', got %v", book.ISBN10)
	}
	if book.ISBN13 == nil || *book.ISBN13 != "9781234567890" {
		t.Errorf("expected isbn13 '9781234567890', got %v", book.ISBN13)
	}
}

// TestImportPathService_ValidatePath_Whitespace tests whitespace handling
func TestImportPathService_ValidatePath_Whitespace(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewImportPathService(mockStore)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid path",
			path:    "/test/path",
			wantErr: false,
		},
		{
			name:    "empty path returns error",
			path:    "",
			wantErr: true,
		},
		{
			name:    "whitespace only path returns error",
			path:    "   ",
			wantErr: true,
		},
		{
			name:    "tab only path returns error",
			path:    "\t\t",
			wantErr: true,
		},
		{
			name:    "path with leading/trailing spaces is valid",
			path:    "  /test/path  ",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.ValidatePath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if err != ErrImportPathEmpty {
					t.Errorf("expected ErrImportPathEmpty, got %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestSystemService_FilterLogsBySearch_CaseInsensitive tests case insensitivity
func TestSystemService_FilterLogsBySearch_CaseInsensitive(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewSystemService(mockStore)

	logs := []database.OperationLog{
		{Message: "Starting SCAN operation"},
		{Message: "Found 10 audiobooks"},
		{Message: "Error processing file"},
	}

	tests := []struct {
		name       string
		searchTerm string
		wantCount  int
	}{
		{
			name:       "uppercase search term",
			searchTerm: "SCAN",
			wantCount:  1,
		},
		{
			name:       "lowercase search term",
			searchTerm: "scan",
			wantCount:  1,
		},
		{
			name:       "mixed case search term",
			searchTerm: "ScAn",
			wantCount:  1,
		},
		{
			name:       "empty search returns all",
			searchTerm: "",
			wantCount:  3,
		},
		{
			name:       "no matches",
			searchTerm: "xyz",
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.FilterLogsBySearch(logs, tt.searchTerm)
			if len(result) != tt.wantCount {
				t.Errorf("expected %d results, got %d", tt.wantCount, len(result))
			}
		})
	}
}

// TestSystemService_SortLogsByTimestamp_EdgeCases tests edge cases for sorting
func TestSystemService_SortLogsByTimestamp_EdgeCases(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewSystemService(mockStore)

	now := time.Now()

	tests := []struct {
		name  string
		logs  []database.OperationLog
		check func(t *testing.T, result []database.OperationLog)
	}{
		{
			name: "already sorted logs remain sorted",
			logs: []database.OperationLog{
				{Message: "Third", CreatedAt: now.Add(2 * time.Hour)},
				{Message: "Second", CreatedAt: now.Add(1 * time.Hour)},
				{Message: "First", CreatedAt: now},
			},
			check: func(t *testing.T, result []database.OperationLog) {
				if result[0].Message != "Third" {
					t.Errorf("expected first message 'Third', got %q", result[0].Message)
				}
			},
		},
		{
			name: "identical timestamps",
			logs: []database.OperationLog{
				{Message: "A", CreatedAt: now},
				{Message: "B", CreatedAt: now},
				{Message: "C", CreatedAt: now},
			},
			check: func(t *testing.T, result []database.OperationLog) {
				if len(result) != 3 {
					t.Errorf("expected 3 logs, got %d", len(result))
				}
			},
		},
		{
			name: "empty logs",
			logs: []database.OperationLog{},
			check: func(t *testing.T, result []database.OperationLog) {
				if len(result) != 0 {
					t.Errorf("expected 0 logs, got %d", len(result))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.SortLogsByTimestamp(tt.logs)
			tt.check(t, result)
		})
	}
}

// TestSystemService_PaginateLogs_EdgeCases tests edge cases for pagination
func TestSystemService_PaginateLogs_EdgeCases(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewSystemService(mockStore)

	logs := make([]database.OperationLog, 50)
	for i := 0; i < 50; i++ {
		logs[i] = database.OperationLog{Message: "Log"}
	}

	tests := []struct {
		name      string
		logs      []database.OperationLog
		page      int
		pageSize  int
		wantCount int
	}{
		{
			name:      "negative page defaults to 1",
			logs:      logs,
			page:      -1,
			pageSize:  20,
			wantCount: 20,
		},
		{
			name:      "zero page defaults to 1",
			logs:      logs,
			page:      0,
			pageSize:  20,
			wantCount: 20,
		},
		{
			name:      "negative pageSize defaults to 20",
			logs:      logs,
			page:      1,
			pageSize:  -1,
			wantCount: 20,
		},
		{
			name:      "zero pageSize defaults to 20",
			logs:      logs,
			page:      1,
			pageSize:  0,
			wantCount: 20,
		},
		{
			name:      "page beyond range returns empty",
			logs:      logs,
			page:      100,
			pageSize:  20,
			wantCount: 0,
		},
		{
			name:      "last page partial results",
			logs:      logs,
			page:      3,
			pageSize:  20,
			wantCount: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.PaginateLogs(tt.logs, tt.page, tt.pageSize)
			if len(result) != tt.wantCount {
				t.Errorf("expected %d logs, got %d", tt.wantCount, len(result))
			}
		})
	}
}

// TestSystemService_GetFormattedUptime tests uptime formatting
func TestSystemService_GetFormattedUptime(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewSystemService(mockStore)

	tests := []struct {
		name      string
		startTime time.Time
		check     func(t *testing.T, uptime string)
	}{
		{
			name:      "very recent start time",
			startTime: time.Now(),
			check: func(t *testing.T, uptime string) {
				if uptime == "" {
					t.Error("expected non-empty uptime string")
				}
			},
		},
		{
			name:      "one hour ago",
			startTime: time.Now().Add(-1 * time.Hour),
			check: func(t *testing.T, uptime string) {
				if !contains(uptime, "h") && !contains(uptime, "m") {
					t.Errorf("expected uptime to contain hours or minutes, got %q", uptime)
				}
			},
		},
		{
			name:      "one day ago",
			startTime: time.Now().Add(-24 * time.Hour),
			check: func(t *testing.T, uptime string) {
				if !contains(uptime, "h") {
					t.Errorf("expected uptime to contain hours, got %q", uptime)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uptime := svc.GetFormattedUptime(tt.startTime)
			tt.check(t, uptime)
		})
	}
}

// TestApplyOverrideToPayload tests applyOverrideToPayload function
func TestApplyOverrideToPayload(t *testing.T) {
	t.Run("title", func(t *testing.T) {
		payload := &AudiobookUpdate{Book: &database.Book{}}
		applyOverrideToPayload(payload, "title", "New Title")
		if payload.Title != "New Title" {
			t.Errorf("expected 'New Title', got %q", payload.Title)
		}
	})

	t.Run("author_name", func(t *testing.T) {
		payload := &AudiobookUpdate{Book: &database.Book{}}
		applyOverrideToPayload(payload, "author_name", "Author")
		if payload.AuthorName == nil || *payload.AuthorName != "Author" {
			t.Errorf("expected 'Author', got %v", payload.AuthorName)
		}
	})

	t.Run("series_name", func(t *testing.T) {
		payload := &AudiobookUpdate{Book: &database.Book{}}
		applyOverrideToPayload(payload, "series_name", "Series")
		if payload.SeriesName == nil || *payload.SeriesName != "Series" {
			t.Errorf("expected 'Series', got %v", payload.SeriesName)
		}
	})

	t.Run("narrator", func(t *testing.T) {
		payload := &AudiobookUpdate{Book: &database.Book{}}
		applyOverrideToPayload(payload, "narrator", "Narrator")
		if payload.Narrator == nil || *payload.Narrator != "Narrator" {
			t.Errorf("expected 'Narrator', got %v", payload.Narrator)
		}
	})

	t.Run("publisher", func(t *testing.T) {
		payload := &AudiobookUpdate{Book: &database.Book{}}
		applyOverrideToPayload(payload, "publisher", "Pub")
		if payload.Publisher == nil || *payload.Publisher != "Pub" {
			t.Errorf("expected 'Pub', got %v", payload.Publisher)
		}
	})

	t.Run("language", func(t *testing.T) {
		payload := &AudiobookUpdate{Book: &database.Book{}}
		applyOverrideToPayload(payload, "language", "en")
		if payload.Language == nil || *payload.Language != "en" {
			t.Errorf("expected 'en', got %v", payload.Language)
		}
	})

	t.Run("audiobook_release_year float64", func(t *testing.T) {
		payload := &AudiobookUpdate{Book: &database.Book{}}
		applyOverrideToPayload(payload, "audiobook_release_year", float64(2023))
		if payload.AudiobookReleaseYear == nil || *payload.AudiobookReleaseYear != 2023 {
			t.Errorf("expected 2023, got %v", payload.AudiobookReleaseYear)
		}
	})

	t.Run("audiobook_release_year int", func(t *testing.T) {
		payload := &AudiobookUpdate{Book: &database.Book{}}
		applyOverrideToPayload(payload, "audiobook_release_year", 2024)
		if payload.AudiobookReleaseYear == nil || *payload.AudiobookReleaseYear != 2024 {
			t.Errorf("expected 2024, got %v", payload.AudiobookReleaseYear)
		}
	})

	t.Run("isbn10", func(t *testing.T) {
		payload := &AudiobookUpdate{Book: &database.Book{}}
		applyOverrideToPayload(payload, "isbn10", "1234567890")
		if payload.ISBN10 == nil || *payload.ISBN10 != "1234567890" {
			t.Errorf("expected '1234567890', got %v", payload.ISBN10)
		}
	})

	t.Run("isbn13", func(t *testing.T) {
		payload := &AudiobookUpdate{Book: &database.Book{}}
		applyOverrideToPayload(payload, "isbn13", "9781234567890")
		if payload.ISBN13 == nil || *payload.ISBN13 != "9781234567890" {
			t.Errorf("expected '9781234567890', got %v", payload.ISBN13)
		}
	})

	t.Run("wrong type is no-op", func(t *testing.T) {
		payload := &AudiobookUpdate{Book: &database.Book{}}
		applyOverrideToPayload(payload, "title", 123)
		if payload.Title != "" {
			t.Errorf("expected empty title, got %q", payload.Title)
		}
	})

	t.Run("unknown field is no-op", func(t *testing.T) {
		payload := &AudiobookUpdate{Book: &database.Book{}}
		applyOverrideToPayload(payload, "unknown_field", "value")
		// Should not panic or change anything
	})
}

// TestConfigUpdateService_UpdateConfig tests UpdateConfig method
func TestConfigUpdateService_UpdateConfig(t *testing.T) {
	t.Run("nil db returns error", func(t *testing.T) {
		svc := &ConfigUpdateService{db: nil}
		status, resp := svc.UpdateConfig(map[string]any{"root_dir": "/test"})
		if status != 500 {
			t.Errorf("expected 500, got %d", status)
		}
		if resp["error"] != "database not initialized" {
			t.Errorf("unexpected error: %v", resp["error"])
		}
	})

	t.Run("database_type rejected", func(t *testing.T) {
		mockStore := mocks.NewMockStore(t)
		svc := NewConfigUpdateService(mockStore)
		status, resp := svc.UpdateConfig(map[string]any{"database_type": "mysql"})
		if status != 400 {
			t.Errorf("expected 400, got %d", status)
		}
		if !contains(resp["error"].(string), "database_type") {
			t.Errorf("expected database_type error, got %v", resp["error"])
		}
	})

	t.Run("enable_sqlite rejected", func(t *testing.T) {
		mockStore := mocks.NewMockStore(t)
		svc := NewConfigUpdateService(mockStore)
		status, resp := svc.UpdateConfig(map[string]any{"enable_sqlite": true})
		if status != 400 {
			t.Errorf("expected 400, got %d", status)
		}
		if !contains(resp["error"].(string), "enable_sqlite") {
			t.Errorf("expected enable_sqlite error, got %v", resp["error"])
		}
	})
}

// TestConfigUpdateService_ApplyUpdates_FieldTypes tests applying different field types
func TestConfigUpdateService_ApplyUpdates_FieldTypes(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	svc := NewConfigUpdateService(mockStore)

	originalRootDir := config.AppConfig.RootDir
	originalAutoOrg := config.AppConfig.AutoOrganize
	originalScans := config.AppConfig.ConcurrentScans
	defer func() {
		config.AppConfig.RootDir = originalRootDir
		config.AppConfig.AutoOrganize = originalAutoOrg
		config.AppConfig.ConcurrentScans = originalScans
	}()

	err := svc.ApplyUpdates(map[string]any{
		"root_dir":         "/new/path",
		"auto_organize":    true,
		"concurrent_scans": float64(8),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.AppConfig.RootDir != "/new/path" {
		t.Errorf("expected root_dir '/new/path', got %q", config.AppConfig.RootDir)
	}
	if !config.AppConfig.AutoOrganize {
		t.Error("expected auto_organize true")
	}
	if config.AppConfig.ConcurrentScans != 8 {
		t.Errorf("expected concurrent_scans 8, got %d", config.AppConfig.ConcurrentScans)
	}
}
