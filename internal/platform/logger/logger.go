// Package logger builds the process-wide structured logger.
package logger

import (
	"io"
	"log/slog"
	"os"

	"github.com/NumerR/ElitStom/internal/config"
)

// New returns a configured *slog.Logger. A nil writer defaults to stderr.
func New(cfg config.Log, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stderr
	}

	opts := &slog.HandlerOptions{
		Level: cfg.Level,
		// Source locations are noise in production logs but essential while debugging.
		AddSource: cfg.Level <= slog.LevelDebug,
	}

	var handler slog.Handler
	if cfg.Format == config.LogFormatText {
		handler = slog.NewTextHandler(w, opts)
	} else {
		handler = slog.NewJSONHandler(w, opts)
	}
	return slog.New(handler)
}

// NewNop returns a logger that discards everything; intended for tests.
func NewNop() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
