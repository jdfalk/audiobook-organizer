// file: internal/server/validators_test.go
// version: 2.0.0
// guid: 0c1d2e3f-4a5b-6c7d-8e9f-0a1b2c3d4e5f

package server

import (
	"strings"
	"testing"
)

// TestValidationError_Error tests the Error() method of ValidationError
func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      ValidationError
		expected string
	}{
		{
			name: "basic error",
			err: ValidationError{
				Field:   "title",
				Message: "title is required",
				Code:    "TITLE_REQUIRED",
			},
			expected: "title: title is required",
		},
		{
			name: "error with special characters",
			err: ValidationError{
				Field:   "email",
				Message: "email format is invalid",
				Code:    "EMAIL_INVALID",
			},
			expected: "email: email format is invalid",
		},
		{
			name: "empty field",
			err: ValidationError{
				Field:   "",
				Message: "some error",
				Code:    "SOME_ERROR",
			},
			expected: ": some error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestValidateTitle tests the ValidateTitle function
func TestValidateTitle(t *testing.T) {
	tests := []struct {
		name      string
		title     string
		minLength int
		maxLength int
		wantErr   bool
		errCode   string
	}{
		{
			name:      "valid title",
			title:     "Valid Title",
			minLength: 1,
			maxLength: 100,
			wantErr:   false,
		},
		{
			name:      "empty title",
			title:     "",
			minLength: 1,
			maxLength: 100,
			wantErr:   true,
			errCode:   "TITLE_REQUIRED",
		},
		{
			name:      "whitespace only title",
			title:     "   ",
			minLength: 1,
			maxLength: 100,
			wantErr:   true,
			errCode:   "TITLE_REQUIRED",
		},
		{
			name:      "title too short",
			title:     "Hi",
			minLength: 5,
			maxLength: 100,
			wantErr:   true,
			errCode:   "TITLE_TOO_SHORT",
		},
		{
			name:      "title too long",
			title:     strings.Repeat("a", 101),
			minLength: 1,
			maxLength: 100,
			wantErr:   true,
			errCode:   "TITLE_TOO_LONG",
		},
		{
			name:      "title at min length boundary",
			title:     "12345",
			minLength: 5,
			maxLength: 100,
			wantErr:   false,
		},
		{
			name:      "title at max length boundary",
			title:     strings.Repeat("a", 100),
			minLength: 1,
			maxLength: 100,
			wantErr:   false,
		},
		{
			name:      "no min length constraint",
			title:     "A",
			minLength: 0,
			maxLength: 100,
			wantErr:   false,
		},
		{
			name:      "no max length constraint",
			title:     strings.Repeat("a", 1000),
			minLength: 1,
			maxLength: 0,
			wantErr:   false,
		},
		{
			name:      "title with leading/trailing spaces",
			title:     "  Valid Title  ",
			minLength: 5,
			maxLength: 100,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTitle(tt.title, tt.minLength, tt.maxLength)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTitle() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				validationErr, ok := err.(ValidationError)
				if !ok {
					t.Errorf("ValidateTitle() error is not ValidationError")
					return
				}
				if validationErr.Code != tt.errCode {
					t.Errorf("ValidateTitle() error code = %v, want %v", validationErr.Code, tt.errCode)
				}
			}
		})
	}
}

// TestValidatePath tests the ValidatePath function
func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
		errCode string
	}{
		{
			name:    "valid absolute path",
			path:    "/home/user/audiobooks",
			wantErr: false,
		},
		{
			name:    "valid relative path",
			path:    "./audiobooks",
			wantErr: false,
		},
		{
			name:    "valid windows path",
			path:    "C:\\Users\\audiobooks",
			wantErr: false,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
			errCode: "PATH_REQUIRED",
		},
		{
			name:    "whitespace only path",
			path:    "   ",
			wantErr: true,
			errCode: "PATH_REQUIRED",
		},
		{
			name:    "path too long",
			path:    strings.Repeat("a", 4097),
			wantErr: true,
			errCode: "PATH_TOO_LONG",
		},
		{
			name:    "path at max length",
			path:    strings.Repeat("a", 4096),
			wantErr: false,
		},
		{
			name:    "path with spaces",
			path:    "/home/user/my audiobooks",
			wantErr: false,
		},
		{
			name:    "path with leading/trailing spaces",
			path:    "  /home/user/audiobooks  ",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				validationErr, ok := err.(ValidationError)
				if !ok {
					t.Errorf("ValidatePath() error is not ValidationError")
					return
				}
				if validationErr.Code != tt.errCode {
					t.Errorf("ValidatePath() error code = %v, want %v", validationErr.Code, tt.errCode)
				}
			}
		})
	}
}

// TestValidateID tests the ValidateID function
func TestValidateID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
		errCode string
	}{
		{
			name:    "valid UUID",
			id:      "550e8400-e29b-41d4-a716-446655440000",
			wantErr: false,
		},
		{
			name:    "valid numeric ID",
			id:      "12345",
			wantErr: false,
		},
		{
			name:    "valid alphanumeric ID",
			id:      "abc123def",
			wantErr: false,
		},
		{
			name:    "empty ID",
			id:      "",
			wantErr: true,
			errCode: "ID_REQUIRED",
		},
		{
			name:    "whitespace only ID",
			id:      "   ",
			wantErr: true,
			errCode: "ID_REQUIRED",
		},
		{
			name:    "ID too long",
			id:      strings.Repeat("a", 257),
			wantErr: true,
			errCode: "ID_TOO_LONG",
		},
		{
			name:    "ID at max length",
			id:      strings.Repeat("a", 256),
			wantErr: false,
		},
		{
			name:    "ID with leading/trailing spaces",
			id:      "  12345  ",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				validationErr, ok := err.(ValidationError)
				if !ok {
					t.Errorf("ValidateID() error is not ValidationError")
					return
				}
				if validationErr.Code != tt.errCode {
					t.Errorf("ValidateID() error code = %v, want %v", validationErr.Code, tt.errCode)
				}
			}
		})
	}
}

// TestValidateEmail tests the ValidateEmail function
func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
		errCode string
	}{
		{
			name:    "valid email",
			email:   "user@example.com",
			wantErr: false,
		},
		{
			name:    "valid email with subdomain",
			email:   "user@mail.example.com",
			wantErr: false,
		},
		{
			name:    "valid email with plus",
			email:   "user+tag@example.com",
			wantErr: false,
		},
		{
			name:    "valid email with dots",
			email:   "first.last@example.com",
			wantErr: false,
		},
		{
			name:    "valid email with numbers",
			email:   "user123@example456.com",
			wantErr: false,
		},
		{
			name:    "empty email",
			email:   "",
			wantErr: true,
			errCode: "EMAIL_REQUIRED",
		},
		{
			name:    "whitespace only email",
			email:   "   ",
			wantErr: true,
			errCode: "EMAIL_REQUIRED",
		},
		{
			name:    "missing @ symbol",
			email:   "userexample.com",
			wantErr: true,
			errCode: "EMAIL_INVALID",
		},
		{
			name:    "missing domain",
			email:   "user@",
			wantErr: true,
			errCode: "EMAIL_INVALID",
		},
		{
			name:    "missing local part",
			email:   "@example.com",
			wantErr: true,
			errCode: "EMAIL_INVALID",
		},
		{
			name:    "missing TLD",
			email:   "user@example",
			wantErr: true,
			errCode: "EMAIL_INVALID",
		},
		{
			name:    "invalid characters",
			email:   "user name@example.com",
			wantErr: true,
			errCode: "EMAIL_INVALID",
		},
		{
			name:    "multiple @ symbols",
			email:   "user@@example.com",
			wantErr: true,
			errCode: "EMAIL_INVALID",
		},
		{
			name:    "email with leading/trailing spaces",
			email:   "  user@example.com  ",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEmail() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				validationErr, ok := err.(ValidationError)
				if !ok {
					t.Errorf("ValidateEmail() error is not ValidationError")
					return
				}
				if validationErr.Code != tt.errCode {
					t.Errorf("ValidateEmail() error code = %v, want %v", validationErr.Code, tt.errCode)
				}
			}
		})
	}
}

// TestValidateInteger tests the ValidateInteger function
func TestValidateInteger(t *testing.T) {
	tests := []struct {
		name      string
		value     int
		fieldName string
		minValue  int
		maxValue  int
		wantErr   bool
		errCode   string
	}{
		{
			name:      "valid value in range",
			value:     50,
			fieldName: "age",
			minValue:  0,
			maxValue:  100,
			wantErr:   false,
		},
		{
			name:      "value at min boundary",
			value:     0,
			fieldName: "count",
			minValue:  0,
			maxValue:  100,
			wantErr:   false,
		},
		{
			name:      "value at max boundary",
			value:     100,
			fieldName: "count",
			minValue:  0,
			maxValue:  100,
			wantErr:   false,
		},
		{
			name:      "value below minimum",
			value:     -1,
			fieldName: "count",
			minValue:  0,
			maxValue:  100,
			wantErr:   true,
			errCode:   "COUNT_TOO_SMALL",
		},
		{
			name:      "value above maximum",
			value:     101,
			fieldName: "count",
			minValue:  0,
			maxValue:  100,
			wantErr:   true,
			errCode:   "COUNT_TOO_LARGE",
		},
		{
			name:      "no min constraint (negative min)",
			value:     -100,
			fieldName: "offset",
			minValue:  -1,
			maxValue:  100,
			wantErr:   false,
		},
		{
			name:      "no max constraint (negative max)",
			value:     1000,
			fieldName: "limit",
			minValue:  0,
			maxValue:  -1,
			wantErr:   false,
		},
		{
			name:      "both constraints disabled",
			value:     999,
			fieldName: "value",
			minValue:  -1,
			maxValue:  -1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInteger(tt.value, tt.fieldName, tt.minValue, tt.maxValue)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInteger() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				validationErr, ok := err.(ValidationError)
				if !ok {
					t.Errorf("ValidateInteger() error is not ValidationError")
					return
				}
				if validationErr.Code != tt.errCode {
					t.Errorf("ValidateInteger() error code = %v, want %v", validationErr.Code, tt.errCode)
				}
			}
		})
	}
}

// TestValidateSliceLength tests the ValidateSliceLength function
func TestValidateSliceLength(t *testing.T) {
	tests := []struct {
		name      string
		slice     any
		fieldName string
		minLength int
		maxLength int
		wantErr   bool
		errCode   string
	}{
		{
			name:      "valid string slice",
			slice:     []string{"a", "b", "c"},
			fieldName: "tags",
			minLength: 1,
			maxLength: 5,
			wantErr:   false,
		},
		{
			name:      "valid int slice",
			slice:     []int{1, 2, 3},
			fieldName: "ids",
			minLength: 1,
			maxLength: 5,
			wantErr:   false,
		},
		{
			name:      "valid any slice",
			slice:     []any{"a", 1, true},
			fieldName: "items",
			minLength: 1,
			maxLength: 5,
			wantErr:   false,
		},
		{
			name:      "empty slice below minimum",
			slice:     []string{},
			fieldName: "tags",
			minLength: 1,
			maxLength: 5,
			wantErr:   true,
			errCode:   "TAGS_TOO_SHORT",
		},
		{
			name:      "slice too long",
			slice:     []string{"a", "b", "c", "d", "e", "f"},
			fieldName: "tags",
			minLength: 1,
			maxLength: 5,
			wantErr:   true,
			errCode:   "TAGS_TOO_LONG",
		},
		{
			name:      "slice at min boundary",
			slice:     []string{"a"},
			fieldName: "tags",
			minLength: 1,
			maxLength: 5,
			wantErr:   false,
		},
		{
			name:      "slice at max boundary",
			slice:     []string{"a", "b", "c", "d", "e"},
			fieldName: "tags",
			minLength: 1,
			maxLength: 5,
			wantErr:   false,
		},
		{
			name:      "no min constraint",
			slice:     []string{},
			fieldName: "tags",
			minLength: 0,
			maxLength: 5,
			wantErr:   false,
		},
		{
			name:      "no max constraint",
			slice:     []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
			fieldName: "tags",
			minLength: 1,
			maxLength: 0,
			wantErr:   false,
		},
		{
			name:      "unsupported slice type",
			slice:     []float64{1.0, 2.0, 3.0},
			fieldName: "values",
			minLength: 1,
			maxLength: 5,
			wantErr:   true,
			errCode:   "INVALID_TYPE",
		},
		{
			name:      "non-slice type",
			slice:     "not a slice",
			fieldName: "data",
			minLength: 1,
			maxLength: 5,
			wantErr:   true,
			errCode:   "INVALID_TYPE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSliceLength(tt.slice, tt.fieldName, tt.minLength, tt.maxLength)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSliceLength() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				validationErr, ok := err.(ValidationError)
				if !ok {
					t.Errorf("ValidateSliceLength() error is not ValidationError")
					return
				}
				if validationErr.Code != tt.errCode {
					t.Errorf("ValidateSliceLength() error code = %v, want %v", validationErr.Code, tt.errCode)
				}
			}
		})
	}
}

// TestValidateStringInList tests the ValidateStringInList function
func TestValidateStringInList(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		fieldName string
		allowed   []string
		wantErr   bool
		errCode   string
	}{
		{
			name:      "valid value",
			value:     "option1",
			fieldName: "type",
			allowed:   []string{"option1", "option2", "option3"},
			wantErr:   false,
		},
		{
			name:      "valid value with spaces trimmed",
			value:     "  option1  ",
			fieldName: "type",
			allowed:   []string{"option1", "option2", "option3"},
			wantErr:   false,
		},
		{
			name:      "invalid value",
			value:     "option4",
			fieldName: "type",
			allowed:   []string{"option1", "option2", "option3"},
			wantErr:   true,
			errCode:   "TYPE_INVALID_VALUE",
		},
		{
			name:      "case sensitive mismatch",
			value:     "OPTION1",
			fieldName: "type",
			allowed:   []string{"option1", "option2", "option3"},
			wantErr:   true,
			errCode:   "TYPE_INVALID_VALUE",
		},
		{
			name:      "empty value",
			value:     "",
			fieldName: "status",
			allowed:   []string{"active", "inactive"},
			wantErr:   true,
			errCode:   "STATUS_INVALID_VALUE",
		},
		{
			name:      "empty allowed list",
			value:     "anything",
			fieldName: "field",
			allowed:   []string{},
			wantErr:   true,
			errCode:   "FIELD_INVALID_VALUE",
		},
		{
			name:      "single allowed value - match",
			value:     "only",
			fieldName: "choice",
			allowed:   []string{"only"},
			wantErr:   false,
		},
		{
			name:      "single allowed value - mismatch",
			value:     "other",
			fieldName: "choice",
			allowed:   []string{"only"},
			wantErr:   true,
			errCode:   "CHOICE_INVALID_VALUE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStringInList(tt.value, tt.fieldName, tt.allowed)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateStringInList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				validationErr, ok := err.(ValidationError)
				if !ok {
					t.Errorf("ValidateStringInList() error is not ValidationError")
					return
				}
				if validationErr.Code != tt.errCode {
					t.Errorf("ValidateStringInList() error code = %v, want %v", validationErr.Code, tt.errCode)
				}
			}
		})
	}
}

// TestValidateURL tests the ValidateURL function
func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errCode string
	}{
		{
			name:    "valid http URL",
			url:     "http://example.com",
			wantErr: false,
		},
		{
			name:    "valid https URL",
			url:     "https://example.com",
			wantErr: false,
		},
		{
			name:    "valid URL with path",
			url:     "https://example.com/path/to/resource",
			wantErr: false,
		},
		{
			name:    "valid URL with query params",
			url:     "https://example.com?key=value&foo=bar",
			wantErr: false,
		},
		{
			name:    "valid URL with fragment",
			url:     "https://example.com#section",
			wantErr: false,
		},
		{
			name:    "valid URL with port",
			url:     "https://example.com:8080",
			wantErr: false,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
			errCode: "URL_REQUIRED",
		},
		{
			name:    "whitespace only URL",
			url:     "   ",
			wantErr: true,
			errCode: "URL_REQUIRED",
		},
		{
			name:    "missing protocol",
			url:     "example.com",
			wantErr: true,
			errCode: "URL_INVALID",
		},
		{
			name:    "ftp protocol",
			url:     "ftp://example.com",
			wantErr: true,
			errCode: "URL_INVALID",
		},
		{
			name:    "URL too long",
			url:     "https://" + strings.Repeat("a", 2041),
			wantErr: true,
			errCode: "URL_TOO_LONG",
		},
		{
			name:    "URL at max length",
			url:     "https://" + strings.Repeat("a", 2040),
			wantErr: false,
		},
		{
			name:    "URL with leading/trailing spaces",
			url:     "  https://example.com  ",
			wantErr: false,
		},
		{
			name:    "localhost URL",
			url:     "http://localhost:3000",
			wantErr: false,
		},
		{
			name:    "IP address URL",
			url:     "http://192.168.1.1",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				validationErr, ok := err.(ValidationError)
				if !ok {
					t.Errorf("ValidateURL() error is not ValidationError")
					return
				}
				if validationErr.Code != tt.errCode {
					t.Errorf("ValidateURL() error code = %v, want %v", validationErr.Code, tt.errCode)
				}
			}
		})
	}
}

// TestValidateYearRange tests the ValidateYearRange function
func TestValidateYearRange(t *testing.T) {
	tests := []struct {
		name    string
		year    int
		wantErr bool
		errCode string
	}{
		{
			name:    "valid year 2000",
			year:    2000,
			wantErr: false,
		},
		{
			name:    "valid year at min boundary",
			year:    1900,
			wantErr: false,
		},
		{
			name:    "valid year at max boundary",
			year:    2100,
			wantErr: false,
		},
		{
			name:    "year below minimum",
			year:    1899,
			wantErr: true,
			errCode: "YEAR_OUT_OF_RANGE",
		},
		{
			name:    "year above maximum",
			year:    2101,
			wantErr: true,
			errCode: "YEAR_OUT_OF_RANGE",
		},
		{
			name:    "year far in past",
			year:    1000,
			wantErr: true,
			errCode: "YEAR_OUT_OF_RANGE",
		},
		{
			name:    "year far in future",
			year:    3000,
			wantErr: true,
			errCode: "YEAR_OUT_OF_RANGE",
		},
		{
			name:    "negative year",
			year:    -100,
			wantErr: true,
			errCode: "YEAR_OUT_OF_RANGE",
		},
		{
			name:    "zero year",
			year:    0,
			wantErr: true,
			errCode: "YEAR_OUT_OF_RANGE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateYearRange(tt.year)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateYearRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				validationErr, ok := err.(ValidationError)
				if !ok {
					t.Errorf("ValidateYearRange() error is not ValidationError")
					return
				}
				if validationErr.Code != tt.errCode {
					t.Errorf("ValidateYearRange() error code = %v, want %v", validationErr.Code, tt.errCode)
				}
			}
		})
	}
}

// TestValidateDuration tests the ValidateDuration function
func TestValidateDuration(t *testing.T) {
	tests := []struct {
		name      string
		duration  int64
		fieldName string
		wantErr   bool
		errCode   string
	}{
		{
			name:      "valid duration 1 hour",
			duration:  3600,
			fieldName: "duration",
			wantErr:   false,
		},
		{
			name:      "valid duration 10 hours",
			duration:  36000,
			fieldName: "duration",
			wantErr:   false,
		},
		{
			name:      "valid duration zero",
			duration:  0,
			fieldName: "duration",
			wantErr:   false,
		},
		{
			name:      "negative duration",
			duration:  -1,
			fieldName: "duration",
			wantErr:   true,
			errCode:   "DURATION_NEGATIVE",
		},
		{
			name:      "duration exceeds max",
			duration:  int64(1000*3600 + 1),
			fieldName: "duration",
			wantErr:   true,
			errCode:   "DURATION_TOO_LONG",
		},
		{
			name:      "duration at max boundary",
			duration:  int64(1000 * 3600),
			fieldName: "duration",
			wantErr:   false,
		},
		{
			name:      "duration just below max",
			duration:  int64(1000*3600 - 1),
			fieldName: "duration",
			wantErr:   false,
		},
		{
			name:      "very large negative duration",
			duration:  -999999,
			fieldName: "duration",
			wantErr:   true,
			errCode:   "DURATION_NEGATIVE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDuration(tt.duration, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDuration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				validationErr, ok := err.(ValidationError)
				if !ok {
					t.Errorf("ValidateDuration() error is not ValidationError")
					return
				}
				if validationErr.Code != tt.errCode {
					t.Errorf("ValidateDuration() error code = %v, want %v", validationErr.Code, tt.errCode)
				}
			}
		})
	}
}

// TestValidateRating tests the ValidateRating function
func TestValidateRating(t *testing.T) {
	tests := []struct {
		name    string
		rating  float64
		wantErr bool
		errCode string
	}{
		{
			name:    "valid rating 5.0",
			rating:  5.0,
			wantErr: false,
		},
		{
			name:    "valid rating at min boundary",
			rating:  0.0,
			wantErr: false,
		},
		{
			name:    "valid rating at max boundary",
			rating:  10.0,
			wantErr: false,
		},
		{
			name:    "valid rating with decimal",
			rating:  7.5,
			wantErr: false,
		},
		{
			name:    "rating below minimum",
			rating:  -0.1,
			wantErr: true,
			errCode: "RATING_OUT_OF_RANGE",
		},
		{
			name:    "rating above maximum",
			rating:  10.1,
			wantErr: true,
			errCode: "RATING_OUT_OF_RANGE",
		},
		{
			name:    "negative rating",
			rating:  -5.0,
			wantErr: true,
			errCode: "RATING_OUT_OF_RANGE",
		},
		{
			name:    "rating far above max",
			rating:  100.0,
			wantErr: true,
			errCode: "RATING_OUT_OF_RANGE",
		},
		{
			name:    "very small positive rating",
			rating:  0.001,
			wantErr: false,
		},
		{
			name:    "rating just below max",
			rating:  9.999,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRating(tt.rating)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRating() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				validationErr, ok := err.(ValidationError)
				if !ok {
					t.Errorf("ValidateRating() error is not ValidationError")
					return
				}
				if validationErr.Code != tt.errCode {
					t.Errorf("ValidateRating() error code = %v, want %v", validationErr.Code, tt.errCode)
				}
			}
		})
	}
}

// TestValidateGenre tests the ValidateGenre function
func TestValidateGenre(t *testing.T) {
	tests := []struct {
		name    string
		genre   string
		wantErr bool
		errCode string
	}{
		{
			name:    "valid genre",
			genre:   "Science Fiction",
			wantErr: false,
		},
		{
			name:    "valid single word genre",
			genre:   "Mystery",
			wantErr: false,
		},
		{
			name:    "valid genre with hyphen",
			genre:   "Science-Fiction",
			wantErr: false,
		},
		{
			name:    "empty genre",
			genre:   "",
			wantErr: true,
			errCode: "GENRE_REQUIRED",
		},
		{
			name:    "whitespace only genre",
			genre:   "   ",
			wantErr: true,
			errCode: "GENRE_REQUIRED",
		},
		{
			name:    "genre too long",
			genre:   strings.Repeat("a", 256),
			wantErr: true,
			errCode: "GENRE_TOO_LONG",
		},
		{
			name:    "genre at max length",
			genre:   strings.Repeat("a", 255),
			wantErr: false,
		},
		{
			name:    "genre with leading/trailing spaces",
			genre:   "  Mystery  ",
			wantErr: false,
		},
		{
			name:    "genre with special characters",
			genre:   "Science Fiction & Fantasy",
			wantErr: false,
		},
		{
			name:    "genre with numbers",
			genre:   "Top 40",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGenre(tt.genre)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateGenre() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				validationErr, ok := err.(ValidationError)
				if !ok {
					t.Errorf("ValidateGenre() error is not ValidationError")
					return
				}
				if validationErr.Code != tt.errCode {
					t.Errorf("ValidateGenre() error code = %v, want %v", validationErr.Code, tt.errCode)
				}
			}
		})
	}
}

// TestValidateLanguage tests the ValidateLanguage function
func TestValidateLanguage(t *testing.T) {
	tests := []struct {
		name    string
		lang    string
		wantErr bool
		errCode string
	}{
		{
			name:    "valid ISO 639-1 code",
			lang:    "en",
			wantErr: false,
		},
		{
			name:    "valid ISO 639-1 code with region",
			lang:    "en-US",
			wantErr: false,
		},
		{
			name:    "valid ISO 639-1 code with different region",
			lang:    "en-GB",
			wantErr: false,
		},
		{
			name:    "valid two-letter code",
			lang:    "fr",
			wantErr: false,
		},
		{
			name:    "valid code with region",
			lang:    "zh-CN",
			wantErr: false,
		},
		{
			name:    "empty language",
			lang:    "",
			wantErr: true,
			errCode: "LANGUAGE_REQUIRED",
		},
		{
			name:    "whitespace only language",
			lang:    "   ",
			wantErr: true,
			errCode: "LANGUAGE_REQUIRED",
		},
		{
			name:    "uppercase language code",
			lang:    "EN",
			wantErr: true,
			errCode: "LANGUAGE_INVALID",
		},
		{
			name:    "single letter code",
			lang:    "e",
			wantErr: true,
			errCode: "LANGUAGE_INVALID",
		},
		{
			name:    "three letter code",
			lang:    "eng",
			wantErr: true,
			errCode: "LANGUAGE_INVALID",
		},
		{
			name:    "invalid region format lowercase",
			lang:    "en-us",
			wantErr: true,
			errCode: "LANGUAGE_INVALID",
		},
		{
			name:    "invalid region format single letter",
			lang:    "en-U",
			wantErr: true,
			errCode: "LANGUAGE_INVALID",
		},
		{
			name:    "invalid region format three letters",
			lang:    "en-USA",
			wantErr: true,
			errCode: "LANGUAGE_INVALID",
		},
		{
			name:    "language with leading/trailing spaces",
			lang:    "  en  ",
			wantErr: false,
		},
		{
			name:    "language with numbers",
			lang:    "e1",
			wantErr: true,
			errCode: "LANGUAGE_INVALID",
		},
		{
			name:    "language with special characters",
			lang:    "en_US",
			wantErr: true,
			errCode: "LANGUAGE_INVALID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLanguage(tt.lang)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLanguage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				validationErr, ok := err.(ValidationError)
				if !ok {
					t.Errorf("ValidateLanguage() error is not ValidationError")
					return
				}
				if validationErr.Code != tt.errCode {
					t.Errorf("ValidateLanguage() error code = %v, want %v", validationErr.Code, tt.errCode)
				}
			}
		})
	}
}
