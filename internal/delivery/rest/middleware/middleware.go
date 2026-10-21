// Package middleware holds the HTTP middleware shared by every route.
package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// AccessLog records one structured line per completed request.
// It must be registered outside Recoverer, otherwise a recovered panic unwinds
// past it and the request never gets logged.
func AccessLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			// Only the path is logged: a raw query string can carry tokens or patient identifiers.
			log.InfoContext(r.Context(), "request handled",
				slog.String("request_id", chimiddleware.GetReqID(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(start)),
				slog.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}

// Recoverer turns a panic into a 500 response plus a logged stack trace,
// keeping one bad request from taking the process down.
func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	const body = `{"error":{"code":"internal_error","message":"internal server error"}}` + "\n"

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}

				log.ErrorContext(r.Context(), "panic recovered",
					slog.String("request_id", chimiddleware.GetReqID(r.Context())),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
				)

				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				if _, err := w.Write([]byte(body)); err != nil {
					log.ErrorContext(r.Context(), "failed to write panic response",
						slog.String("error", err.Error()),
					)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
