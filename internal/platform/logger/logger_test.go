package logger

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NumerR/ElitStom/internal/config"
)

func TestNewEmitsTheConfiguredFormat(t *testing.T) {
	tests := []struct {
		name        string
		format      string
		wantInLog   []string
		wantNotIn   []string
		parseAsJSON bool
	}{
		{
			name:        "json is the machine-readable default",
			format:      config.LogFormatJSON,
			parseAsJSON: true,
		},
		{
			name:      "text is readable in a terminal",
			format:    config.LogFormatText,
			wantInLog: []string{"level=INFO", `msg="service started"`, "version=1.2.3"},
		},
		{
			name:        "an unknown format falls back to json",
			format:      "yaml",
			parseAsJSON: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := New(config.Log{Level: slog.LevelInfo, Format: tt.format}, &buf)

			log.Info("service started", slog.String("version", "1.2.3"))

			for _, want := range tt.wantInLog {
				assert.Contains(t, buf.String(), want)
			}
			for _, unwanted := range tt.wantNotIn {
				assert.NotContains(t, buf.String(), unwanted)
			}

			if tt.parseAsJSON {
				var entry map[string]any
				require.NoError(t, json.Unmarshal(buf.Bytes(), &entry), "output must be valid JSON: %s", buf.String())
				assert.Equal(t, "INFO", entry["level"])
				assert.Equal(t, "service started", entry["msg"])
				assert.Equal(t, "1.2.3", entry["version"])
				assert.NotContains(t, entry, "source", "info level keeps production logs lean")
			}
		})
	}
}

func TestNewHonoursLevelAndSource(t *testing.T) {
	tests := []struct {
		name          string
		level         slog.Level
		emit          func(*slog.Logger)
		wantInLog     string
		wantEmpty     bool
		wantSourceKey bool
	}{
		{
			name:      "warn suppresses info",
			level:     slog.LevelWarn,
			emit:      func(l *slog.Logger) { l.Info("noise") },
			wantEmpty: true,
		},
		{
			name:      "warn passes warnings",
			level:     slog.LevelWarn,
			emit:      func(l *slog.Logger) { l.Warn("careful") },
			wantInLog: "careful",
		},
		{
			name:          "debug adds source locations",
			level:         slog.LevelDebug,
			emit:          func(l *slog.Logger) { l.Debug("tracing") },
			wantInLog:     "tracing",
			wantSourceKey: true,
		},
		{
			name:      "error suppresses nothing",
			level:     slog.LevelError,
			emit:      func(l *slog.Logger) { l.Error("boom") },
			wantInLog: "boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := New(config.Log{Level: tt.level, Format: config.LogFormatJSON}, &buf)

			tt.emit(log)

			if tt.wantEmpty {
				assert.Empty(t, buf.String())
				return
			}
			assert.Contains(t, buf.String(), tt.wantInLog)

			var entry map[string]any
			require.NoError(t, json.Unmarshal(buf.Bytes(), &entry))
			if tt.wantSourceKey {
				assert.Contains(t, entry, "source")
			} else {
				assert.NotContains(t, entry, "source")
			}
		})
	}
}

func TestNewDefaultsToStderr(t *testing.T) {
	original := os.Stderr
	readEnd, writeEnd, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = writeEnd
	t.Cleanup(func() { os.Stderr = original })

	New(config.Log{Level: slog.LevelInfo, Format: config.LogFormatJSON}, nil).Info("written to stderr")

	require.NoError(t, writeEnd.Close())
	out, err := io.ReadAll(readEnd)
	require.NoError(t, err)

	assert.Contains(t, string(out), "written to stderr")
}

func TestNewNopReturnsUsableLogger(t *testing.T) {
	log := NewNop()

	require.NotNil(t, log)
	log.Info("discarded")
	log.Error("also discarded", slog.String("error", "boom"))
}
