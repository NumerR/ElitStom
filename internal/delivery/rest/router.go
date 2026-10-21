// Package rest wires the HTTP delivery layer: routing, middleware and handlers.
package rest

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/NumerR/ElitStom/internal/delivery/rest/handler"
	"github.com/NumerR/ElitStom/internal/delivery/rest/middleware"
	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// Deps carries everything the router needs. Each field is required: a router without its use
// cases would mount routes that answer 500 on the first request.
type Deps struct {
	DB     handler.Pinger
	Log    *slog.Logger
	Tokens usecase.TokenIssuer
	Auth   *usecase.Auth
	Users  *usecase.Users
}

// missing names the collaborators that were not supplied, for the startup check.
func (d Deps) missing() []string {
	var missing []string
	if d.DB == nil {
		missing = append(missing, "database pinger")
	}
	if d.Log == nil {
		missing = append(missing, "logger")
	}
	if d.Tokens == nil {
		missing = append(missing, "token issuer")
	}
	if d.Auth == nil {
		missing = append(missing, "auth service")
	}
	if d.Users == nil {
		missing = append(missing, "users service")
	}
	return missing
}

func NewRouter(d Deps) http.Handler { // A missing collaborator is a wiring bug, and the only honest place to report it is at
	// startup. Building the handlers anyway would mount routes that panic on the first request.
	if missing := d.missing(); len(missing) > 0 {
		panic("rest: NewRouter is missing " + strings.Join(missing, ", "))
	}

	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	// RealIP trusts X-Forwarded-For / X-Real-IP, so the process must only ever be
	// exposed through a proxy we control; otherwise clients can spoof their address
	// in the audit log and in any IP-based rate limiting.
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.AccessLog(d.Log))
	r.Use(middleware.Recoverer(d.Log))

	responder := handler.NewResponder(d.Log)
	health := handler.NewHealth(d.DB, d.Log)
	auth := handler.NewAuth(d.Auth, d.Log)
	users := handler.NewUsers(d.Users, d.Log)

	r.Get("/healthz", health.Liveness)
	r.Get("/readyz", health.Readiness)

	r.Route("/api/v1", func(api chi.Router) {
		// Anonymous: the credential endpoints are the only ones that do not read the
		// Authorization header.
		api.Post("/auth/register", auth.Register)
		api.Post("/auth/login", auth.Login)
		api.Post("/auth/refresh", auth.Refresh)

		api.Group(func(authenticated chi.Router) {
			authenticated.Use(middleware.Authenticate(d.Tokens, responder))

			authenticated.Post("/auth/logout", auth.Logout)

			authenticated.Get("/users/me", users.Me)
			authenticated.Patch("/users/me", users.UpdateProfile)

			// Admin-only. Keeping the guard on the group rather than in each handler means a
			// new admin route is denied by default instead of by remembering to add a check.
			authenticated.Route("/admin", func(admin chi.Router) {
				admin.Use(middleware.RequireRoles(responder, domain.RoleAdmin))

				admin.Get("/users", users.List)
				admin.Post("/users", users.CreateAccount)
				admin.Patch("/users/{id}/role", users.ChangeRole)
				admin.Patch("/users/{id}/status", users.SetStatus)
			})
		})
	})

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		responder.Error(w, req, http.StatusNotFound, handler.ErrorPayload{
			Code:    "not_found",
			Message: "route not found",
		})
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		responder.Error(w, req, http.StatusMethodNotAllowed, handler.ErrorPayload{
			Code:    "method_not_allowed",
			Message: "method not allowed",
		})
	})

	return r
}
