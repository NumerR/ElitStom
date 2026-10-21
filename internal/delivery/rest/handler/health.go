package handler

import (
	"context"
	"log/slog"
	"net/http"
)

// Pinger is the health-probe contract for infrastructure dependencies.
// *pgxpool.Pool satisfies it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Health serves the liveness and readiness probes.
type Health struct {
	*Responder
	db Pinger
}

func NewHealth(db Pinger, log *slog.Logger) *Health {
	return &Health{Responder: NewResponder(log), db: db}
}

type checkResult struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type readinessResponse struct {
	Status string                 `json:"status"`
	Checks map[string]checkResult `json:"checks"`
}

// Liveness reports that the process is running. It deliberately touches no dependency:
// a database outage must not make the orchestrator restart a healthy pod.
func (h *Health) Liveness(w http.ResponseWriter, r *http.Request) {
	h.JSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

// Readiness reports whether the process can serve traffic right now.
func (h *Health) Readiness(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]checkResult, 1)
	status := http.StatusOK
	state := "ready"

	if err := h.db.Ping(r.Context()); err != nil {
		h.log.ErrorContext(r.Context(), "readiness probe failed",
			slog.String("dependency", "postgres"),
			slog.String("error", err.Error()),
		)
		checks["postgres"] = checkResult{Status: "down", Error: "unreachable"}
		status = http.StatusServiceUnavailable
		state = "unavailable"
	} else {
		checks["postgres"] = checkResult{Status: "up"}
	}

	h.JSON(w, r, status, readinessResponse{Status: state, Checks: checks})
}
