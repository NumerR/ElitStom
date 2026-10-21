package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// Users serves the profile and admin directory endpoints.
type Users struct {
	*Responder
	users *usecase.Users
}

func NewUsers(users *usecase.Users, log *slog.Logger) *Users {
	return &Users{Responder: NewResponder(log), users: users}
}

// Me returns the authenticated account. The id comes from the verified token, never from the
// path, so `/users/me` cannot be pointed at somebody else.
func (h *Users) Me(w http.ResponseWriter, r *http.Request) {
	actor, err := h.actor(r)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	user, err := h.users.Profile(r.Context(), actor)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	h.JSON(w, r, http.StatusOK, newUserResponse(user))
}

// UpdateProfile rewrites the authenticated account's own name and phone. Role and status are
// absent from the body type: an account must not be able to promote itself by sending a field
// it happens to have a name for.
func (h *Users) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	actor, err := h.actor(r)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	var in profileRequest
	if err := h.decode(r, &in); err != nil {
		h.FromError(w, r, err)
		return
	}

	user, err := h.users.UpdateProfile(r.Context(), actor, usecase.UpdateProfileInput{
		FullName: in.FullName,
		Phone:    in.Phone,
	})
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	h.JSON(w, r, http.StatusOK, newUserResponse(user))
}

// CreateAccount registers a doctor or admin. The use case enforces the admin-only rule; the
// route is additionally mounted behind RequireRoles so an unauthorized caller never reaches
// bcrypt at all.
func (h *Users) CreateAccount(w http.ResponseWriter, r *http.Request) {
	actor, err := h.actor(r)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	var in createUserRequest
	if err := h.decode(r, &in); err != nil {
		h.FromError(w, r, err)
		return
	}

	user, err := h.users.CreateUser(r.Context(), actor, usecase.CreateUserInput{
		Email:    in.Email,
		FullName: in.FullName,
		Phone:    in.Phone,
		Role:     domain.Role(in.Role),
		Password: in.Password,
	})
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	h.JSON(w, r, http.StatusCreated, newUserResponse(user))
}

// List returns a page of the directory. role and status are validated here rather than passed
// through as arbitrary strings, so an unknown filter is a 400 and never a silent empty page.
func (h *Users) List(w http.ResponseWriter, r *http.Request) {
	actor, err := h.actor(r)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	query := r.URL.Query()
	filter := usecase.UserFilter{}

	if raw := query.Get("role"); raw != "" {
		role, err := domain.ParseRole(raw)
		if err != nil {
			h.FromError(w, r, err)
			return
		}
		filter.Role = role
	}
	if raw := query.Get("status"); raw != "" {
		status, err := domain.ParseUserStatus(raw)
		if err != nil {
			h.FromError(w, r, err)
			return
		}
		filter.Status = status
	}

	filter.Limit, filter.Offset = paging(query)

	listed, err := h.users.List(r.Context(), actor, filter)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	// The projection loop is where a leak would happen, so it is the one place the response
	// shape is written: userResponse has no field that could carry a hash.
	out := make([]userResponse, 0, len(listed))
	for _, user := range listed {
		out = append(out, newUserResponse(user))
	}

	h.JSON(w, r, http.StatusOK, listUsersResponse{Users: out, Limit: filter.Limit, Offset: filter.Offset})
}

// ChangeRole promotes or demotes an account. Sessions are dropped by the use case because the
// old role is baked into live access tokens.
func (h *Users) ChangeRole(w http.ResponseWriter, r *http.Request) {
	actor, err := h.actor(r)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	var in roleRequest
	if err := h.decode(r, &in); err != nil {
		h.FromError(w, r, err)
		return
	}

	user, err := h.users.ChangeRole(r.Context(), actor, chi.URLParam(r, "id"), in.Role)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	h.JSON(w, r, http.StatusOK, newUserResponse(user))
}

// SetStatus enables or disables an account.
func (h *Users) SetStatus(w http.ResponseWriter, r *http.Request) {
	actor, err := h.actor(r)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	var in statusRequest
	if err := h.decode(r, &in); err != nil {
		h.FromError(w, r, err)
		return
	}

	user, err := h.users.SetStatus(r.Context(), actor, chi.URLParam(r, "id"), in.Status)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	h.JSON(w, r, http.StatusOK, newUserResponse(user))
}
