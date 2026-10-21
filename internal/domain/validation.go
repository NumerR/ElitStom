package domain

import (
	"fmt"
	"strings"
)

// FieldError ties one validation failure to the input field that caused it.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError collects every field problem of a single input so the client can fix
// all of them in one round trip. errors.Is(err, ErrInvalidInput) succeeds for it, so the
// delivery layer maps it to HTTP 400 like any other invalid input.
type ValidationError struct {
	Fields []FieldError
}

func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Fields: []FieldError{{Field: field, Message: message}}}
}

// Add appends a failure and returns the receiver, so validators can accumulate instead of
// failing on the first problem.
func (e *ValidationError) Add(field, message string) *ValidationError {
	e.Fields = append(e.Fields, FieldError{Field: field, Message: message})
	return e
}

func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "invalid input"
	}
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, fmt.Sprintf("%s: %s", f.Field, f.Message))
	}
	return "invalid input: " + strings.Join(parts, "; ")
}

// Is makes errors.Is(err, ErrInvalidInput) true for any validation failure.
func (e *ValidationError) Is(target error) bool { return target == ErrInvalidInput }

// Details renders the per-field failures for the HTTP error envelope.
func (e *ValidationError) Details() []string {
	if len(e.Fields) == 0 {
		return nil
	}
	out := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		out = append(out, fmt.Sprintf("%s: %s", f.Field, f.Message))
	}
	return out
}
