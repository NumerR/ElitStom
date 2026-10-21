package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NumerR/ElitStom/internal/config"
	"github.com/NumerR/ElitStom/internal/platform/logger"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

type stubPinger struct {
	err   error
	calls int
	seen  string
}

func (s *stubPinger) Ping(ctx context.Context) error {
	s.calls++
	s.seen, _ = ctx.Value(requestIDKey).(string)
	return s.err
}

// newTestLogger captures log output so assertions can cover observability, not just status codes.
func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return logger.New(config.Log{Level: slog.LevelDebug, Format: config.LogFormatText}, &buf), &buf
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "body must be valid JSON: %s", rec.Body.String())
	return body
}

func TestHealthLivenessReportsOKWithoutTouchingDependencies(t *testing.T) {
	db := &stubPinger{err: errors.New("postgres is down")}
	h := NewHealth(db, logger.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.Liveness(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "ok", decodeBody(t, rec)["status"])
	assert.Zero(t, db.calls, "liveness must never probe the database, or an outage would restart healthy pods")
}

func TestHealthReadiness(t *testing.T) {
	tests := []struct {
		name         string
		pingErr      error
		wantStatus   int
		wantState    string
		wantCheck    string
		wantCheckErr string
		wantLogged   string
	}{
		{
			name:       "database answers",
			pingErr:    nil,
			wantStatus: http.StatusOK,
			wantState:  "ready",
			wantCheck:  "up",
		},
		{
			name:         "database is unreachable",
			pingErr:      errors.New("dial tcp 10.0.0.7:5432: connect: connection refused"),
			wantStatus:   http.StatusServiceUnavailable,
			wantState:    "unavailable",
			wantCheck:    "down",
			wantCheckErr: "unreachable",
			wantLogged:   "10.0.0.7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, logBuf := newTestLogger()
			db := &stubPinger{err: tt.pingErr}
			h := NewHealth(db, log)

			ctx := context.WithValue(context.Background(), requestIDKey, "req-42")
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(ctx)
			rec := httptest.NewRecorder()

			h.Readiness(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, 1, db.calls)
			assert.Equal(t, "req-42", db.seen, "the request context must reach the dependency probe")

			body := decodeBody(t, rec)
			assert.Equal(t, tt.wantState, body["status"])

			checks, ok := body["checks"].(map[string]any)
			require.True(t, ok, "checks must be an object, got %T", body["checks"])
			pg, ok := checks["postgres"].(map[string]any)
			require.True(t, ok, "postgres check must be present")
			assert.Equal(t, tt.wantCheck, pg["status"])
			if tt.wantCheckErr == "" {
				assert.NotContains(t, pg, "error", "a healthy check must omit the error field")
			} else {
				assert.Equal(t, tt.wantCheckErr, pg["error"])
			}

			raw := rec.Body.String()
			if tt.pingErr != nil {
				assert.NotContains(t, raw, tt.wantLogged, "the raw driver error must stay in the logs")
				assert.Contains(t, logBuf.String(), tt.wantLogged)
				assert.Contains(t, logBuf.String(), "readiness probe failed")
			} else {
				assert.Empty(t, logBuf.String(), "a healthy probe must not log")
			}
		})
	}
}
