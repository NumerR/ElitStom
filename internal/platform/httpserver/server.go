// Package httpserver wraps net/http with the timeouts and graceful shutdown
// behaviour the rest of the application expects.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/NumerR/ElitStom/internal/config"
)

type Server struct {
	httpServer      *http.Server
	log             *slog.Logger
	shutdownTimeout time.Duration
}

func New(cfg config.HTTP, handler http.Handler, log *slog.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              cfg.Addr(),
			Handler:           handler,
			ReadHeaderTimeout: cfg.ReadHeaderTimeout,
			ReadTimeout:       cfg.ReadTimeout,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
		log:             log,
		shutdownTimeout: cfg.ShutdownTimeout,
	}
}

// Start blocks until the server stops. A graceful shutdown is reported as a nil error.
func (s *Server) Start() error {
	s.log.Info("http server listening", slog.String("addr", s.httpServer.Addr))
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// Shutdown drains in-flight requests, falling back to a hard close once ctx or
// the configured shutdown timeout expires.
func (s *Server) Shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, s.shutdownTimeout)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		s.log.Error("graceful shutdown incomplete, closing forcibly", slog.String("error", err.Error()))
		return errors.Join(err, s.httpServer.Close())
	}

	s.log.Info("http server stopped")
	return nil
}
