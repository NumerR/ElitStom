package rest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type stubPinger struct{ err error }

func (s stubPinger) Ping(context.Context) error { return s.err }

func TestNewRouter(t *testing.T) {
	tests := []struct {
		name       string
		dbErr      error
		method     string
		target     string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "liveness",
			method:     http.MethodGet,
			target:     "/healthz",
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ok"}`,
		},
		{
			name:       "liveness stays green while the database is down",
			dbErr:      errors.New("connection refused"),
			method:     http.MethodGet,
			target:     "/healthz",
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ok"}`,
		},
		{
			name:       "readiness with a healthy database",
			method:     http.MethodGet,
			target:     "/readyz",
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ready","checks":{"postgres":{"status":"up"}}}`,
		},
		{
			name:       "readiness with an unreachable database",
			dbErr:      errors.New("dial tcp 10.0.0.7:5432: i/o timeout"),
			method:     http.MethodGet,
			target:     "/readyz",
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"unavailable","checks":{"postgres":{"status":"down","error":"unreachable"}}}`,
		},
		{
			name:       "unknown route",
			method:     http.MethodGet,
			target:     "/api/v1/patients",
			wantStatus: http.StatusNotFound,
			wantBody:   `{"error":{"code":"not_found","message":"route not found"}}`,
		},
		{
			name:       "wrong method",
			method:     http.MethodPost,
			target:     "/healthz",
			wantStatus: http.StatusMethodNotAllowed,
			wantBody:   `{"error":{"code":"method_not_allowed","message":"method not allowed"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newAPIWithPinger(t, stubPinger{err: tt.dbErr}).server

			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.target, nil))

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.JSONEq(t, tt.wantBody, rec.Body.String())
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
		})
	}
}
