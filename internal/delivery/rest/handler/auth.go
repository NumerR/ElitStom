package handler

import (
	"log/slog"
	"net/http"

	"github.com/NumerR/ElitStom/internal/usecase"
)

// Auth serves the credential endpoints. It owns no business rule: whether a password is strong
// enough, whether a refresh token was replayed, whether an account is disabled — all of that
// stays in the use case.
type Auth struct {
	*Responder
	auth *usecase.Auth
}

func NewAuth(auth *usecase.Auth, log *slog.Logger) *Auth {
	return &Auth{Responder: NewResponder(log), auth: auth}
}

// Register creates a patient account and returns its first session. It is the only
// unauthenticated route that mints a session, and the role is fixed by the use case, so a
// request body cannot ask for admin.
func (h *Auth) Register(w http.ResponseWriter, r *http.Request) {
	var in registerRequest
	if err := h.decode(r, &in); err != nil {
		h.FromError(w, r, err)
		return
	}

	user, pair, err := h.auth.Register(r.Context(), usecase.RegisterInput{
		Email:    in.Email,
		FullName: in.FullName,
		Phone:    in.Phone,
		Password: in.Password,
	})
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	// 201 with the session attached: the caller is authenticated by the time it reads the
	// response, so signup needs no follow-up login round trip.
	h.JSON(w, r, http.StatusCreated, newAuthResponse(user, pair))
}

// Login exchanges credentials for a session. Failure is always 401 with the same body, so the
// endpoint cannot be used to enumerate accounts.
func (h *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var in loginRequest
	if err := h.decode(r, &in); err != nil {
		h.FromError(w, r, err)
		return
	}

	user, pair, err := h.auth.Login(r.Context(), usecase.LoginInput{
		Email:    in.Email,
		Password: in.Password,
	})
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	h.JSON(w, r, http.StatusOK, newAuthResponse(user, pair))
}

// Refresh rotates a refresh token. The presented secret is consumed by the use case and never
// reappears in the response.
func (h *Auth) Refresh(w http.ResponseWriter, r *http.Request) {
	var in sessionRequest
	if err := h.decode(r, &in); err != nil {
		h.FromError(w, r, err)
		return
	}

	user, pair, err := h.auth.Refresh(r.Context(), in.RefreshToken)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	h.JSON(w, r, http.StatusOK, newAuthResponse(user, pair))
}

// Logout ends the session behind the presented refresh token. An unknown or already revoked
// token is success in the use case, so this is 204 for every input: a client that cannot log
// out has no way to tell whether it leaked a session.
func (h *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	actor, err := h.actor(r)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	var in sessionRequest
	if err := h.decode(r, &in); err != nil {
		h.FromError(w, r, err)
		return
	}

	if err := h.auth.Logout(r.Context(), actor, in.RefreshToken); err != nil {
		h.FromError(w, r, err)
		return
	}

	h.JSON(w, r, http.StatusNoContent, nil)
}
