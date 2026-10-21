package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/assert"

	"github.com/NumerR/ElitStom/internal/config"
	"github.com/NumerR/ElitStom/internal/platform/logger"
)

func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return logger.New(config.Log{Level: slog.LevelDebug, Format: config.LogFormatText}, &buf), &buf
}

func TestAccessLog(t *testing.T) {
	tests := []struct {
		name         string
		target       string
		method       string
		handler      http.HandlerFunc
		wantInLog    []string
		wantNotInLog []string
	}{
		{
			name:   "records method, path, status and size",
			target: "/api/v1/patients",
			method: http.MethodPost,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
				if _, err := w.Write([]byte("created")); err != nil {
					t.Errorf("write response body: %v", err)
				}
			},
			wantInLog: []string{
				"request handled", "method=POST", "path=/api/v1/patients",
				"status=201", "bytes=7", "duration=", "remote_addr=",
			},
		},
		{
			name:   "omits the query string, which may carry tokens",
			target: "/api/v1/auth/refresh?token=super-secret-value",
			method: http.MethodGet,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			wantInLog:    []string{"path=/api/v1/auth/refresh"},
			wantNotInLog: []string{"super-secret-value", "token="},
		},
		{
			name:   "logs failures with their real status",
			target: "/api/v1/patients/1",
			method: http.MethodGet,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			wantInLog: []string{"status=404"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, buf := newTestLogger()
			// RequestID first, exactly as the router registers it.
			h := chimiddleware.RequestID(AccessLog(log)(tt.handler))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.target, nil))

			line := buf.String()
			for _, want := range tt.wantInLog {
				assert.Contains(t, line, want)
			}
			for _, unwanted := range tt.wantNotInLog {
				assert.NotContains(t, line, unwanted)
			}
			assert.NotContains(t, line, `request_id=""`, "a request id must be present")
		})
	}
}

func TestRecovererReturnsOpaque500AndLogsTheStack(t *testing.T) {
	log, buf := newTestLogger()
	h := chimiddleware.RequestID(Recoverer(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("nil dereference in billing calculation")
	})))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/invoices", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	assert.JSONEq(t, `{"error":{"code":"internal_error","message":"internal server error"}}`, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "billing calculation")

	line := buf.String()
	assert.Contains(t, line, "panic recovered")
	assert.Contains(t, line, "nil dereference in billing calculation")
	assert.Contains(t, line, "stack=")
	assert.Contains(t, line, "path=/api/v1/invoices")
}

func TestRecovererPassesThroughWithoutPanic(t *testing.T) {
	log, buf := newTestLogger()
	h := Recoverer(log)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	assert.Equal(t, http.StatusTeapot, rec.Code)
	assert.Empty(t, buf.String())
}

// TestAccessLogOutsideRecovererLogsRecoveredPanics pins the middleware ordering the
// router depends on: if Recoverer wrapped AccessLog, panics would never be logged.
func TestAccessLogOutsideRecovererLogsRecoveredPanics(t *testing.T) {
	log, buf := newTestLogger()
	h := AccessLog(log)(Recoverer(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	line := buf.String()
	assert.Contains(t, line, "panic recovered")
	assert.Contains(t, line, "request handled")
	assert.Contains(t, line, "status=500")
}
