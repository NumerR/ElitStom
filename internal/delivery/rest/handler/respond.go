package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/NumerR/ElitStom/internal/domain"
)

// ErrorPayload is the machine-readable body every failure response carries.
type ErrorPayload struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

type errorEnvelope struct {
	Error ErrorPayload `json:"error"`
}

// Responder renders JSON and translates domain errors into HTTP responses.
type Responder struct {
	log *slog.Logger
}

func NewResponder(log *slog.Logger) *Responder {
	return &Responder{log: log}
}

func (r *Responder) JSON(w http.ResponseWriter, req *http.Request, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	if body == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// The status line is already on the wire; all that is left is to record the failure.
		r.log.ErrorContext(req.Context(), "failed to encode response body",
			slog.String("error", err.Error()),
			slog.Int("status", status),
		)
	}
}

func (r *Responder) Error(w http.ResponseWriter, req *http.Request, status int, payload ErrorPayload) {
	r.JSON(w, req, status, errorEnvelope{Error: payload})
}

// FromError maps err onto a status code. Internal failures are logged with their
// real cause but reported to the client as an opaque message. err must be non-nil;
// handlers with nothing to report should call JSON directly.
func (r *Responder) FromError(w http.ResponseWriter, req *http.Request, err error) {
	status, code := statusFor(err)

	if status == http.StatusInternalServerError {
		r.log.ErrorContext(req.Context(), "request failed",
			slog.String("error", err.Error()),
			slog.String("method", req.Method),
			slog.String("path", req.URL.Path),
		)
		r.Error(w, req, status, ErrorPayload{Code: code, Message: "internal server error"})
		return
	}

	payload := ErrorPayload{Code: code, Message: clientMessage(code)}
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		payload.Details = ve.Details()
	}
	r.Error(w, req, status, payload)
}

// statusFor maps a domain sentinel onto a status and a machine-readable code. The internal
// error text never reaches the client: wrapped causes can quote driver messages, a column name
// or a fragment of the presented token, none of which belongs in a response body.
func statusFor(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest, "invalid_input"
	case errors.Is(err, domain.ErrInvalidCredentials),
		errors.Is(err, domain.ErrTokenInvalid),
		errors.Is(err, domain.ErrTokenExpired),
		errors.Is(err, domain.ErrSessionRevoked),
		errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, domain.ErrUserDisabled):
		return http.StatusForbidden, "account_disabled"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrAlreadyExists):
		return http.StatusConflict, "already_exists"
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "conflict"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}

func clientMessage(code string) string {
	switch code {
	case "invalid_input":
		return "request validation failed"
	case "unauthorized":
		return "authentication required"
	case "account_disabled":
		return "account is disabled"
	case "forbidden":
		return "insufficient permissions"
	case "not_found":
		return "resource not found"
	case "already_exists":
		return "resource already exists"
	case "conflict":
		return "request conflicts with the current state"
	default:
		return "internal server error"
	}
}
