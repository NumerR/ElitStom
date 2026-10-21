package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NumerR/ElitStom/internal/domain"
)

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) ErrorPayload {
	t.Helper()
	var envelope struct {
		Error ErrorPayload `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope), "body must be a JSON error envelope: %s", rec.Body.String())
	return envelope.Error
}

func TestResponderFromError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		hide        string
		wantStatus  int
		wantCode    string
		wantMessage string
		wantLogged  bool
	}{
		{
			name:        "invalid input",
			err:         fmt.Errorf("create patient: %w: email is required", domain.ErrInvalidInput),
			hide:        "email is required",
			wantStatus:  http.StatusBadRequest,
			wantCode:    "invalid_input",
			wantMessage: "request validation failed",
		},
		{
			name:        "unauthorized",
			err:         domain.ErrUnauthorized,
			wantStatus:  http.StatusUnauthorized,
			wantCode:    "unauthorized",
			wantMessage: "authentication required",
		},
		{
			name:        "expired access token",
			err:         fmt.Errorf("%w: token is expired by 3m12s", domain.ErrTokenExpired),
			hide:        "token is expired",
			wantStatus:  http.StatusUnauthorized,
			wantCode:    "unauthorized",
			wantMessage: "authentication required",
		},
		{
			name:        "forbidden",
			err:         fmt.Errorf("%w: only a doctor may read this record", domain.ErrForbidden),
			hide:        "only a doctor may read this record",
			wantStatus:  http.StatusForbidden,
			wantCode:    "forbidden",
			wantMessage: "insufficient permissions",
		},
		{
			name:        "disabled account",
			err:         fmt.Errorf("%w: account 0b8f5c1a-2d3e-4a5b-8c9d-0e1f2a3b4c5d", domain.ErrUserDisabled),
			hide:        "0b8f5c1a",
			wantStatus:  http.StatusForbidden,
			wantCode:    "account_disabled",
			wantMessage: "account is disabled",
		},
		{
			name:        "not found",
			err:         fmt.Errorf("appointment 42: %w", domain.ErrNotFound),
			hide:        "appointment 42",
			wantStatus:  http.StatusNotFound,
			wantCode:    "not_found",
			wantMessage: "resource not found",
		},
		{
			name:        "already exists",
			err:         fmt.Errorf("%w: account", domain.ErrAlreadyExists),
			wantStatus:  http.StatusConflict,
			wantCode:    "already_exists",
			wantMessage: "resource already exists",
		},
		{
			name:        "conflicting state",
			err:         fmt.Errorf("%w: the slot is already booked", domain.ErrConflict),
			hide:        "the slot is already booked",
			wantStatus:  http.StatusConflict,
			wantCode:    "conflict",
			wantMessage: "request conflicts with the current state",
		},
		{
			name:        "unknown failure becomes an opaque 500",
			err:         errors.New(`dial tcp 10.0.0.7:5432: password authentication failed for user "admin"`),
			wantStatus:  http.StatusInternalServerError,
			wantCode:    "internal_error",
			wantMessage: "internal server error",
			wantLogged:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, logBuf := newTestLogger()
			r := NewResponder(log)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/patients", nil)
			rec := httptest.NewRecorder()

			r.FromError(rec, req, tt.err)

			assert.Equal(t, tt.wantStatus, rec.Code)
			payload := decodeError(t, rec)
			assert.Equal(t, tt.wantCode, payload.Code)
			assert.Equal(t, tt.wantMessage, payload.Message)

			// The wrapped cause stays out of the body whatever the status: it can quote a
			// driver message, a column name or the identifier the client just presented.
			if tt.hide != "" {
				assert.NotContains(t, rec.Body.String(), tt.hide)
			}

			if tt.wantLogged {
				assert.NotContains(t, rec.Body.String(), "password authentication failed",
					"internal detail must never reach the client")
				assert.Contains(t, logBuf.String(), "request failed")
				assert.Contains(t, logBuf.String(), "password authentication failed")
				assert.Contains(t, logBuf.String(), "/api/v1/patients")
			} else {
				assert.Empty(t, logBuf.String(), "a client error is not a server-side failure")
			}
		})
	}
}

func TestResponderJSON(t *testing.T) {
	tests := []struct {
		name            string
		status          int
		body            any
		wantBody        string
		wantEncodeError bool
	}{
		{
			name:   "encodes a payload",
			status: http.StatusOK,
			body:   map[string]any{"status": "ok"},
			// httptest records the trailing newline json.Encoder always emits.
			wantBody: "{\"status\":\"ok\"}\n",
		},
		{
			name:   "nil body writes headers only",
			status: http.StatusNoContent,
			body:   nil,
		},
		{
			name:            "an unencodable payload is logged, not swallowed",
			status:          http.StatusOK,
			body:            make(chan int),
			wantEncodeError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, logBuf := newTestLogger()
			r := NewResponder(log)

			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			rec := httptest.NewRecorder()

			r.JSON(rec, req, tt.status, tt.body)

			assert.Equal(t, tt.status, rec.Code)
			assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
			assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
			assert.Equal(t, tt.wantBody, rec.Body.String())

			if tt.wantEncodeError {
				assert.Contains(t, logBuf.String(), "failed to encode response body")
			} else {
				assert.Empty(t, logBuf.String())
			}
		})
	}
}

func TestResponderErrorCarriesValidationDetails(t *testing.T) {
	log, _ := newTestLogger()
	r := NewResponder(log)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/patients", nil)
	rec := httptest.NewRecorder()

	r.Error(rec, req, http.StatusBadRequest, ErrorPayload{
		Code:    "invalid_input",
		Message: "validation failed",
		Details: []string{"email: required", "birth_date: must be in the past"},
	})

	payload := decodeError(t, rec)
	assert.Equal(t, "invalid_input", payload.Code)
	assert.Equal(t, []string{"email: required", "birth_date: must be in the past"}, payload.Details)
}
