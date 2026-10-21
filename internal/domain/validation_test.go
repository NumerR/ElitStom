package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidationErrorIsSentinelCompatible(t *testing.T) {
	err := NewValidationError("email", "is required")

	if !errors.Is(err, ErrInvalidInput) {
		t.Error("errors.Is(err, ErrInvalidInput) = false, want true so delivery can map HTTP 400")
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("a validation error must not match ErrNotFound")
	}

	var target *ValidationError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As failed for %T", err)
	}
	if len(target.Fields) != 1 {
		t.Errorf("Fields = %v, want exactly one failure", target.Details())
	}
}

func TestValidationErrorAddAccumulates(t *testing.T) {
	ve := &ValidationError{}
	returned := ve.Add("email", "is required").Add("password", "is too short")

	if returned != ve {
		t.Error("Add must return the receiver so calls can chain")
	}
	if got := ve.Error(); got != "invalid input: email: is required; password: is too short" {
		t.Errorf("Error() = %q", got)
	}
	want := []string{"email: is required", "password: is too short"}
	details := ve.Details()
	if len(details) != len(want) {
		t.Fatalf("Details() = %v, want %d entries", details, len(want))
	}
	for i, entry := range want {
		if details[i] != entry {
			t.Errorf("Details()[%d] = %q, want %q", i, details[i], entry)
		}
	}
}

func TestValidationErrorEmptyForms(t *testing.T) {
	ve := &ValidationError{}

	if ve.Error() != "invalid input" {
		t.Errorf("Error() = %q, want the generic message", ve.Error())
	}
	if ve.Details() != nil {
		t.Errorf("Details() = %v, want nil so the JSON envelope omits the field", ve.Details())
	}
	if !errors.Is(ve, ErrInvalidInput) {
		t.Error("an empty validation error must still map to invalid input")
	}
}

func TestValidationErrorDetailsAreFieldSafe(t *testing.T) {
	secret := "hunter2-super-secret-password"
	ve := NewValidationError("password", "must be at least 12 characters")

	for _, detail := range append(ve.Details(), ve.Error()) {
		if strings.Contains(detail, secret) {
			t.Errorf("detail %q leaks the submitted value", detail)
		}
	}
}
