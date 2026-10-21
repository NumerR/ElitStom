// Package domain holds the entities, value objects and errors shared by every layer.
// It must not import any other internal package.
package domain

import "errors"

// Sentinel errors returned by the usecase and repository layers.
// The delivery layer maps them onto HTTP status codes; wrapping with %w preserves the mapping.
var (
	ErrNotFound      = errors.New("resource not found")
	ErrAlreadyExists = errors.New("resource already exists")
	ErrConflict      = errors.New("conflicting state")
	ErrInvalidInput  = errors.New("invalid input")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
)

// Authentication failures. ErrInvalidCredentials is deliberately returned for both an
// unknown email and a wrong password, and login is rate-limited, so accounts cannot be
// enumerated from the response alone.
var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrTokenInvalid       = errors.New("token is invalid")
	ErrTokenExpired       = errors.New("token has expired")
	ErrSessionRevoked     = errors.New("session has been revoked")
	ErrUserDisabled       = errors.New("account is disabled")
)
