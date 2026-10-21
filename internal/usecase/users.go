package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/NumerR/ElitStom/internal/domain"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

// CreateUserInput describes an account an admin opens on someone's behalf. Unlike
// self-service registration it may carry a privileged role.
type CreateUserInput struct {
	Email    string
	FullName string
	Phone    string
	Role     domain.Role
	Password string
}

// UpdateProfileInput replaces the mutable profile fields. Email is deliberately absent:
// it is the account identity and changing it needs a verified-ownership flow.
type UpdateProfileInput struct {
	FullName string
	Phone    string
}

// Users covers profile self-service and the admin-only account management that
// registration can never reach.
type Users struct {
	users   UserRepository
	refresh RefreshTokenStore
	hasher  PasswordHasher
	clock   Clock
}

func NewUsers(users UserRepository, refresh RefreshTokenStore, hasher PasswordHasher, clock Clock) (*Users, error) {
	missing := []string{}
	if users == nil {
		missing = append(missing, "users repository")
	}
	if refresh == nil {
		missing = append(missing, "refresh token store")
	}
	if hasher == nil {
		missing = append(missing, "password hasher")
	}
	if clock == nil {
		missing = append(missing, "clock")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("usecase: NewUsers is missing %v", missing)
	}
	return &Users{users: users, refresh: refresh, hasher: hasher, clock: clock}, nil
}

// CreateUser is the only path to a doctor or admin account, so it is admin-only.
func (u *Users) CreateUser(ctx context.Context, actor Actor, in CreateUserInput) (domain.User, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.User{}, err
	}
	if err := domain.ValidatePasswordStrength(in.Password); err != nil {
		return domain.User{}, err
	}
	// Validated before hashing for the same reason Register does it: bcrypt at cost 12 must
	// never be paid for a request that cannot succeed.
	if err := domain.ValidateAccount(in.Email, in.FullName, in.Phone, in.Role); err != nil {
		return domain.User{}, err
	}

	hash, err := u.hasher.Hash(in.Password)
	if err != nil {
		return domain.User{}, err
	}

	user, err := domain.NewUser(domain.UserParams{
		Email:        in.Email,
		FullName:     in.FullName,
		Phone:        in.Phone,
		Role:         in.Role,
		PasswordHash: hash,
	})
	if err != nil {
		return domain.User{}, err
	}

	created, err := u.users.Create(ctx, *user)
	if err != nil {
		return domain.User{}, err
	}
	return sanitize(created), nil
}

// Profile returns the account behind the authenticated caller.
func (u *Users) Profile(ctx context.Context, actor Actor) (domain.User, error) {
	user, err := u.users.ByID(ctx, actor.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.User{}, fmt.Errorf("%w: account no longer exists", domain.ErrUnauthorized)
		}
		return domain.User{}, err
	}
	if user.IsDisabled() {
		return domain.User{}, domain.ErrUserDisabled
	}
	return sanitize(user), nil
}

// UpdateProfile lets a caller rewrite their own name and phone. Elevation of privilege
// has no route through this method — role and status are not inputs here.
func (u *Users) UpdateProfile(ctx context.Context, actor Actor, in UpdateProfileInput) (domain.User, error) {
	user, err := u.users.ByID(ctx, actor.UserID)
	if err != nil {
		return domain.User{}, err
	}
	if user.IsDisabled() {
		return domain.User{}, domain.ErrUserDisabled
	}
	if err := user.UpdateProfile(in.FullName, in.Phone); err != nil {
		return domain.User{}, err
	}

	updated, err := u.users.Update(ctx, user)
	if err != nil {
		return domain.User{}, err
	}
	return sanitize(updated), nil
}

// ChangeRole rewrites an account's privileges and drops every refresh-token family it owns,
// because sessions issued before the change still carry the old role until they expire.
func (u *Users) ChangeRole(ctx context.Context, actor Actor, userID string, rawRole string) (domain.User, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.User{}, err
	}
	// An admin who demotes themself can lock the clinic out of the only remaining admin
	// account, and the downgrade takes effect immediately for their own session anyway.
	if strings.EqualFold(userID, actor.UserID) {
		return domain.User{}, fmt.Errorf("%w: an admin cannot change their own role", domain.ErrConflict)
	}

	role, err := domain.ParseRole(rawRole)
	if err != nil {
		return domain.User{}, err
	}

	user, err := u.users.ByID(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}
	if err := user.ChangeRole(role); err != nil {
		return domain.User{}, err
	}

	updated, err := u.users.Update(ctx, user)
	if err != nil {
		return domain.User{}, err
	}
	if err := u.dropSessions(ctx, updated.ID); err != nil {
		return domain.User{}, err
	}
	return sanitize(updated), nil
}

// SetStatus enables or disables an account. Disabling kills the account's sessions;
// re-enabling deliberately does not, since nobody is logged in to restore.
func (u *Users) SetStatus(ctx context.Context, actor Actor, userID string, rawStatus string) (domain.User, error) {
	if err := requireAdmin(actor); err != nil {
		return domain.User{}, err
	}
	status, err := domain.ParseUserStatus(rawStatus)
	if err != nil {
		return domain.User{}, err
	}

	user, err := u.users.ByID(ctx, userID)
	if err != nil {
		return domain.User{}, err
	}

	switch status {
	case domain.StatusDisabled:
		if strings.EqualFold(userID, actor.UserID) {
			return domain.User{}, fmt.Errorf("%w: an admin cannot disable their own account", domain.ErrConflict)
		}
		user.Disable()
	case domain.StatusActive:
		user.Enable()
	}

	updated, err := u.users.Update(ctx, user)
	if err != nil {
		return domain.User{}, err
	}
	if updated.IsDisabled() {
		if err := u.dropSessions(ctx, updated.ID); err != nil {
			return domain.User{}, err
		}
	}
	return sanitize(updated), nil
}

// List is the admin directory. The limit is clamped so a caller cannot ask for the whole
// table, and the filter is passed through as-is: the repository owns the WHERE clause.
func (u *Users) List(ctx context.Context, actor Actor, filter UserFilter) ([]domain.User, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	if filter.Limit <= 0 {
		filter.Limit = defaultListLimit
	}
	if filter.Limit > maxListLimit {
		filter.Limit = maxListLimit
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if filter.Role != "" && !filter.Role.IsValid() {
		return nil, domain.NewValidationError("role", "must be one of admin, doctor, patient")
	}
	if filter.Status != "" && filter.Status != domain.StatusActive && filter.Status != domain.StatusDisabled {
		return nil, domain.NewValidationError("status", "must be one of active, disabled")
	}

	listed, err := u.users.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Mapping over the result keeps the hash out of the slice that reaches delivery,
	// so a handler cannot serialise it by accident.
	users := make([]domain.User, 0, len(listed))
	for _, user := range listed {
		users = append(users, sanitize(user))
	}
	return users, nil
}

// dropSessions revokes every refresh token owned by userID. Access tokens stay valid for
// their remaining minutes by design; that window is why the TTL is short.
func (u *Users) dropSessions(ctx context.Context, userID string) error {
	if err := u.refresh.RevokeAllForUser(ctx, userID, u.clock.Now()); err != nil {
		return fmt.Errorf("usecase: revoke sessions for user: %w", err)
	}
	return nil
}

func requireAdmin(actor Actor) error {
	if !actor.Role.CanManageUsers() {
		return fmt.Errorf("%w: only an admin may manage clinic accounts", domain.ErrForbidden)
	}
	return nil
}

// sanitize clears the password hash from a copy of the entity. Every Users method returns
// through it; the repository still hands out the real hash for authentication.
func sanitize(user domain.User) domain.User {
	user.PasswordHash = ""
	return user
}
