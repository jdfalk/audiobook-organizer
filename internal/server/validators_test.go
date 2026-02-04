// file: internal/server/validators_test.go
// version: 1.0.0
// guid: 0c1d2e3f-4a5b-6c7d-8e9f-0a1b2c3d4e5f

package server

import (
	"testing"
)

func TestValidateTitle_Valid(t *testing.T) {
	err := ValidateTitle("Test Title", 0, 0)
	if err != nil {
		t.Errorf("expected no error for valid title, got %v", err)
	}
}

func TestValidateTitle_Empty(t *testing.T) {
	err := ValidateTitle("", 0, 0)
	if err == nil {
		t.Error("expected error for empty title")
	}
	ve := err.(ValidationError)
	if ve.Code != "TITLE_REQUIRED" {
		t.Errorf("expected TITLE_REQUIRED code, got %q", ve.Code)
	}
}

func TestValidateTitle_TooShort(t *testing.T) {
	err := ValidateTitle("ab", 5, 0)
	if err == nil {
		t.Error("expected error for too short title")
	}
	ve := err.(ValidationError)
	if ve.Code != "TITLE_TOO_SHORT" {
		t.Errorf("expected TITLE_TOO_SHORT code, got %q", ve.Code)
	}
}

func TestValidateTitle_TooLong(t *testing.T) {
	err := ValidateTitle("a very long title that exceeds the limit", 0, 20)
	if err == nil {
		t.Error("expected error for too long title")
	}
	ve := err.(ValidationError)
	if ve.Code != "TITLE_TOO_LONG" {
		t.Errorf("expected TITLE_TOO_LONG code, got %q", ve.Code)
	}
}

func TestValidatePath_Valid(t *testing.T) {
	err := ValidatePath("/valid/path")
	if err != nil {
		t.Errorf("expected no error for valid path, got %v", err)
	}
}

func TestValidatePath_Empty(t *testing.T) {
	err := ValidatePath("")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestValidateID_Valid(t *testing.T) {
	err := ValidateID("abc123")
	if err != nil {
		t.Errorf("expected no error for valid ID, got %v", err)
	}
}

func TestValidateID_Empty(t *testing.T) {
	err := ValidateID("")
	if err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestValidateEmail_Valid(t *testing.T) {
	err := ValidateEmail("test@example.com")
	if err != nil {
		t.Errorf("expected no error for valid email, got %v", err)
	}
}

func TestValidateEmail_Invalid(t *testing.T) {
	err := ValidateEmail("invalid-email")
	if err == nil {
		t.Error("expected error for invalid email")
	}
}

func TestValidateEmail_Empty(t *testing.T) {
	err := ValidateEmail("")
	if err == nil {
		t.Error("expected error for empty email")
	}
}

func TestValidateInteger_Valid(t *testing.T) {
	err := ValidateInteger(50, "count", 0, 100)
	if err != nil {
		t.Errorf("expected no error for valid integer, got %v", err)
	}
}

func TestValidateInteger_BelowMin(t *testing.T) {
	err := ValidateInteger(5, "count", 10, 100)
	if err == nil {
		t.Error("expected error for integer below min")
	}
}

func TestValidateInteger_AboveMax(t *testing.T) {
	err := ValidateInteger(150, "count", 0, 100)
	if err == nil {
		t.Error("expected error for integer above max")
	}
}

func TestValidateSliceLength_Valid(t *testing.T) {
	slice := []string{"a", "b", "c"}
	err := ValidateSliceLength(slice, "items", 1, 10)
	if err != nil {
		t.Errorf("expected no error for valid slice, got %v", err)
	}
}

func TestValidateSliceLength_TooShort(t *testing.T) {
	slice := []string{"a"}
	err := ValidateSliceLength(slice, "items", 2, 10)
	if err == nil {
		t.Error("expected error for too short slice")
	}
}

func TestValidateSliceLength_TooLong(t *testing.T) {
	slice := []string{"a", "b", "c", "d"}
	err := ValidateSliceLength(slice, "items", 1, 3)
	if err == nil {
		t.Error("expected error for too long slice")
	}
}

func TestValidateStringInList_Valid(t *testing.T) {
	err := ValidateStringInList("red", "color", []string{"red", "blue", "green"})
	if err != nil {
		t.Errorf("expected no error for valid string, got %v", err)
	}
}

func TestValidateStringInList_Invalid(t *testing.T) {
	err := ValidateStringInList("yellow", "color", []string{"red", "blue", "green"})
	if err == nil {
		t.Error("expected error for invalid string")
	}
}

func TestValidateURL_Valid(t *testing.T) {
	err := ValidateURL("https://example.com")
	if err != nil {
		t.Errorf("expected no error for valid URL, got %v", err)
	}
}

func TestValidateURL_Invalid(t *testing.T) {
	err := ValidateURL("not-a-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestValidateYearRange_Valid(t *testing.T) {
	err := ValidateYearRange(2024)
	if err != nil {
		t.Errorf("expected no error for valid year, got %v", err)
	}
}

func TestValidateYearRange_TooOld(t *testing.T) {
	err := ValidateYearRange(1800)
	if err == nil {
		t.Error("expected error for year too old")
	}
}

func TestValidateYearRange_TooNew(t *testing.T) {
	err := ValidateYearRange(2200)
	if err == nil {
		t.Error("expected error for year too new")
	}
}

func TestValidateDuration_Valid(t *testing.T) {
	err := ValidateDuration(3600, "duration") // 1 hour
	if err != nil {
		t.Errorf("expected no error for valid duration, got %v", err)
	}
}

func TestValidateDuration_Negative(t *testing.T) {
	err := ValidateDuration(-100, "duration")
	if err == nil {
		t.Error("expected error for negative duration")
	}
}

func TestValidateDuration_TooLong(t *testing.T) {
	err := ValidateDuration(2000*3600, "duration") // 2000 hours
	if err == nil {
		t.Error("expected error for duration too long")
	}
}

func TestValidateRating_Valid(t *testing.T) {
	err := ValidateRating(7.5)
	if err != nil {
		t.Errorf("expected no error for valid rating, got %v", err)
	}
}

func TestValidateRating_TooLow(t *testing.T) {
	err := ValidateRating(-1)
	if err == nil {
		t.Error("expected error for rating too low")
	}
}

func TestValidateRating_TooHigh(t *testing.T) {
	err := ValidateRating(11)
	if err == nil {
		t.Error("expected error for rating too high")
	}
}

func TestValidateGenre_Valid(t *testing.T) {
	err := ValidateGenre("Science Fiction")
	if err != nil {
		t.Errorf("expected no error for valid genre, got %v", err)
	}
}

func TestValidateGenre_Empty(t *testing.T) {
	err := ValidateGenre("")
	if err == nil {
		t.Error("expected error for empty genre")
	}
}

func TestValidateLanguage_Valid(t *testing.T) {
	err := ValidateLanguage("en")
	if err != nil {
		t.Errorf("expected no error for valid language, got %v", err)
	}

	err = ValidateLanguage("en-US")
	if err != nil {
		t.Errorf("expected no error for valid language with region, got %v", err)
	}
}

func TestValidateLanguage_Invalid(t *testing.T) {
	err := ValidateLanguage("english")
	if err == nil {
		t.Error("expected error for invalid language")
	}
}
