package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type usersHarness struct {
	t       *testing.T
	users   *fakeUsers
	refresh *fakeRefresh
	hasher  *fakeHasher
	clock   *fakeClock
	service *Users
}

func newUsersHarness(t *testing.T) *usersHarness {
	t.Helper()

	h := &usersHarness{
		t:       t,
		users:   newFakeUsers(),
		refresh: newFakeRefresh(),
		hasher:  newFakeHasher(),
		clock:   newFakeClock(time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)),
	}
	service, err := NewUsers(h.users, h.refresh, h.hasher, h.clock)
	require.NoError(t, err)
	h.service = service
	return h
}

// seedAccount stores an account directly so a test can arrange any role, and returns its id.
func (h *usersHarness) seedAccount(role domain.Role, status domain.UserStatus) string {
	h.t.Helper()

	return h.users.seed(domain.User{
		Email:        string(role) + "@clinic.test",
		FullName:     "Seeded Account",
		Phone:        "+15550100",
		Role:         role,
		Status:       status,
		PasswordHash: h.hasher.seed(testPassword),
		CreatedAt:    h.clock.Now(),
	})
}

func (h *usersHarness) admin() Actor {
	return Actor{UserID: h.seedAccount(domain.RoleAdmin, domain.StatusActive), Role: domain.RoleAdmin}
}

// seedSession stores one live refresh token for an account so revocation has something to kill.
func (h *usersHarness) seedSession(ctx context.Context, userID string) {
	h.t.Helper()

	_, err := h.refresh.Insert(ctx, domain.RefreshToken{
		ID:        "token-1",
		UserID:    userID,
		FamilyID:  "family-1",
		TokenHash: "digest-1",
		ExpiresAt: h.clock.Now().Add(time.Hour),
		CreatedAt: h.clock.Now(),
	})
	require.NoError(h.t, err)
}

func TestNewUsersRejectsMissingDependencies(t *testing.T) {
	cases := []struct {
		name    string
		wantErr string
		deps    func(h *usersHarness) (UserRepository, RefreshTokenStore, PasswordHasher, Clock)
	}{
		{
			name:    "nil repository",
			wantErr: "users repository",
			deps: func(h *usersHarness) (UserRepository, RefreshTokenStore, PasswordHasher, Clock) {
				return nil, h.refresh, h.hasher, h.clock
			},
		},
		{
			name:    "nil refresh store",
			wantErr: "refresh token store",
			deps: func(h *usersHarness) (UserRepository, RefreshTokenStore, PasswordHasher, Clock) {
				return h.users, nil, h.hasher, h.clock
			},
		},
		{
			name:    "nil hasher",
			wantErr: "password hasher",
			deps: func(h *usersHarness) (UserRepository, RefreshTokenStore, PasswordHasher, Clock) {
				return h.users, h.refresh, nil, h.clock
			},
		},
		{
			name:    "nil clock",
			wantErr: "clock",
			deps: func(h *usersHarness) (UserRepository, RefreshTokenStore, PasswordHasher, Clock) {
				return h.users, h.refresh, h.hasher, nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newUsersHarness(t)
			users, refresh, hasher, clock := tc.deps(h)

			service, err := NewUsers(users, refresh, hasher, clock)

			assert.Nil(t, service)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestCreateUserGrantsPrivilegedRoles(t *testing.T) {
	cases := []struct {
		name string
		role domain.Role
	}{
		{name: "doctor", role: domain.RoleDoctor},
		{name: "admin", role: domain.RoleAdmin},
		{name: "patient", role: domain.RolePatient},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newUsersHarness(t)

			user, err := h.service.CreateUser(context.Background(), h.admin(), CreateUserInput{
				Email:    tc.role.String() + ".new@clinic.test",
				FullName: "Nadia   Okonjo",
				Phone:    "+1 (555) 010-0",
				Role:     tc.role,
				Password: testPassword,
			})
			require.NoError(t, err)

			assert.Equal(t, tc.role, user.Role)
			assert.Equal(t, domain.StatusActive, user.Status)
			assert.Empty(t, user.PasswordHash, "the hash must never leave the service")
			assert.Equal(t, "Nadia Okonjo", user.FullName, "runs of spaces must collapse")
			assert.Equal(t, "+15550100", user.Phone)
		})
	}
}

func TestCreateUserRejections(t *testing.T) {
	cases := []struct {
		name    string
		actor   domain.Role
		input   CreateUserInput
		wantErr error
	}{
		{
			name:    "doctor may not create accounts",
			actor:   domain.RoleDoctor,
			input:   CreateUserInput{Email: "a@b.co", FullName: "Ab", Role: domain.RolePatient, Password: testPassword},
			wantErr: domain.ErrForbidden,
		},
		{
			name:    "patient may not create accounts",
			actor:   domain.RolePatient,
			input:   CreateUserInput{Email: "a@b.co", FullName: "Ab", Role: domain.RolePatient, Password: testPassword},
			wantErr: domain.ErrForbidden,
		},
		{
			name:    "unknown role",
			actor:   domain.RoleAdmin,
			input:   CreateUserInput{Email: "a@b.co", FullName: "Ab", Role: domain.Role("receptionist"), Password: testPassword},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name:    "weak password",
			actor:   domain.RoleAdmin,
			input:   CreateUserInput{Email: "a@b.co", FullName: "Ab", Role: domain.RoleDoctor, Password: "short"},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name:    "invalid email",
			actor:   domain.RoleAdmin,
			input:   CreateUserInput{Email: "not-an-email", FullName: "Ab", Role: domain.RoleDoctor, Password: testPassword},
			wantErr: domain.ErrInvalidInput,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newUsersHarness(t)
			actor := Actor{UserID: h.seedAccount(tc.actor, domain.StatusActive), Role: tc.actor}

			user, err := h.service.CreateUser(context.Background(), actor, tc.input)

			assert.Equal(t, domain.User{}, user)
			assert.ErrorIs(t, err, tc.wantErr)
			assert.Zero(t, h.users.created, "a rejected request must not write")

			if tc.wantErr == domain.ErrForbidden {
				assert.Zero(t, h.hasher.hashes, "a forbidden request must not reach bcrypt")
			}
		})
	}
}

func TestCreateUserRejectsDuplicateEmail(t *testing.T) {
	h := newUsersHarness(t)
	ctx := context.Background()

	_, err := h.service.CreateUser(ctx, h.admin(), CreateUserInput{
		Email: "dup@clinic.test", FullName: "Ab", Role: domain.RoleDoctor, Password: testPassword,
	})
	require.NoError(t, err)

	_, err = h.service.CreateUser(ctx, h.admin(), CreateUserInput{
		Email: "dup@clinic.test", FullName: "Cd", Role: domain.RolePatient, Password: testPassword,
	})
	assert.ErrorIs(t, err, domain.ErrAlreadyExists)
}

func TestCreateUserValidatesBeforeHashing(t *testing.T) {
	h := newUsersHarness(t)

	_, err := h.service.CreateUser(context.Background(), h.admin(), CreateUserInput{
		Email:    "broken",
		FullName: "Ab",
		Role:     domain.RoleDoctor,
		Password: testPassword,
	})
	require.Error(t, err)
	assert.Zero(t, h.hasher.hashes, "bcrypt must not be paid for a request that cannot succeed")
}

func TestProfile(t *testing.T) {
	cases := []struct {
		name    string
		status  domain.UserStatus
		delete  bool
		repoErr error
		wantErr error
	}{
		{name: "active account", status: domain.StatusActive},
		{name: "disabled account", status: domain.StatusDisabled, wantErr: domain.ErrUserDisabled},
		{name: "deleted account", status: domain.StatusActive, delete: true, wantErr: domain.ErrUnauthorized},
		{name: "repository failure", status: domain.StatusActive, repoErr: errStoreUnavailable, wantErr: errStoreUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newUsersHarness(t)
			id := h.seedAccount(domain.RoleDoctor, tc.status)
			if tc.delete {
				require.NoError(t, h.users.deleteAccount(id))
			}
			if tc.repoErr != nil {
				h.users.byIDErr = tc.repoErr
			}

			user, err := h.service.Profile(context.Background(), Actor{UserID: id, Role: domain.RoleDoctor})

			if tc.wantErr != nil {
				assert.Equal(t, domain.User{}, user)
				assert.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, id, user.ID)
			assert.Empty(t, user.PasswordHash)
		})
	}
}

func TestUpdateProfileReplacesOnlyMutableFields(t *testing.T) {
	h := newUsersHarness(t)
	ctx := context.Background()
	id := h.seedAccount(domain.RoleDoctor, domain.StatusActive)
	before, err := h.users.ByID(ctx, id)
	require.NoError(t, err)

	user, err := h.service.UpdateProfile(ctx, Actor{UserID: id, Role: domain.RoleDoctor}, UpdateProfileInput{
		FullName: "  Nadia   Okonjo ",
		Phone:    "+1 (555) 010-1",
	})
	require.NoError(t, err)

	assert.Equal(t, "Nadia Okonjo", user.FullName)
	assert.Equal(t, "+15550101", user.Phone)
	assert.Equal(t, before.Email, user.Email, "email is the account identity and is not mutable here")
	assert.Equal(t, before.Role, user.Role)
	assert.Equal(t, before.Status, user.Status)
	assert.Empty(t, user.PasswordHash)
	assert.Equal(t, 1, h.users.updated)
}

func TestUpdateProfileRejections(t *testing.T) {
	cases := []struct {
		name    string
		input   UpdateProfileInput
		wantErr error
	}{
		{name: "empty name", input: UpdateProfileInput{FullName: "   ", Phone: "+15550100"}, wantErr: domain.ErrInvalidInput},
		{name: "one-letter name", input: UpdateProfileInput{FullName: "A", Phone: ""}, wantErr: domain.ErrInvalidInput},
		{name: "letters in phone", input: UpdateProfileInput{FullName: "Nadia Okonjo", Phone: "+1-555-call-us"}, wantErr: domain.ErrInvalidInput},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newUsersHarness(t)
			id := h.seedAccount(domain.RolePatient, domain.StatusActive)

			user, err := h.service.UpdateProfile(context.Background(), Actor{UserID: id, Role: domain.RolePatient}, tc.input)

			assert.Equal(t, domain.User{}, user)
			assert.ErrorIs(t, err, tc.wantErr)
			assert.Zero(t, h.users.updated, "an invalid profile must not be written")
		})
	}
}

func TestUpdateProfileRejectsDisabledAccount(t *testing.T) {
	h := newUsersHarness(t)
	id := h.seedAccount(domain.RolePatient, domain.StatusDisabled)

	_, err := h.service.UpdateProfile(context.Background(), Actor{UserID: id, Role: domain.RolePatient}, UpdateProfileInput{
		FullName: "Nadia Okonjo",
	})
	assert.ErrorIs(t, err, domain.ErrUserDisabled)
	assert.Zero(t, h.users.updated)
}

func TestChangeRoleDropsExistingSessions(t *testing.T) {
	h := newUsersHarness(t)
	ctx := context.Background()
	id := h.seedAccount(domain.RolePatient, domain.StatusActive)
	h.seedSession(ctx, id)

	user, err := h.service.ChangeRole(ctx, h.admin(), id, "doctor")
	require.NoError(t, err)

	assert.Equal(t, domain.RoleDoctor, user.Role)
	assert.Empty(t, user.PasswordHash)
	assert.Equal(t, []string{id}, h.refresh.revokedUsers, "sessions carrying the old role must be revoked")
	assert.True(t, h.refresh.isRevoked("digest-1"))
}

func TestChangeRoleRejections(t *testing.T) {
	cases := []struct {
		name    string
		actor   domain.Role
		role    string
		wantErr error
	}{
		{name: "doctor may not change roles", actor: domain.RoleDoctor, role: "admin", wantErr: domain.ErrForbidden},
		{name: "unknown role", actor: domain.RoleAdmin, role: "receptionist", wantErr: domain.ErrInvalidInput},
		{name: "empty role", actor: domain.RoleAdmin, role: "", wantErr: domain.ErrInvalidInput},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newUsersHarness(t)
			target := h.seedAccount(domain.RolePatient, domain.StatusActive)
			admin := h.seedAccount(tc.actor, domain.StatusActive)

			user, err := h.service.ChangeRole(context.Background(), Actor{UserID: admin, Role: tc.actor}, target, tc.role)

			assert.Equal(t, domain.User{}, user)
			assert.ErrorIs(t, err, tc.wantErr)
			assert.Equal(t, domain.RolePatient, h.users.roleOf(target), "the stored role must be untouched")
			assert.Zero(t, h.users.updated)
		})
	}
}

func TestChangeRoleRefusesSelfDemotion(t *testing.T) {
	h := newUsersHarness(t)
	admin := h.admin()

	user, err := h.service.ChangeRole(context.Background(), admin, admin.UserID, "patient")
	assert.ErrorIs(t, err, domain.ErrConflict)
	assert.Equal(t, domain.User{}, user)
	assert.Equal(t, domain.RoleAdmin, h.users.roleOf(admin.UserID))
	assert.Zero(t, h.users.updated)
}

func TestChangeRoleUnknownAccount(t *testing.T) {
	h := newUsersHarness(t)

	_, err := h.service.ChangeRole(context.Background(), h.admin(), "missing-user", "doctor")
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Empty(t, h.refresh.revokedUsers)
}

func TestSetStatus(t *testing.T) {
	cases := []struct {
		name        string
		start       domain.UserStatus
		raw         string
		wantStatus  domain.UserStatus
		wantRevoked bool
	}{
		{name: "disable active account", start: domain.StatusActive, raw: "disabled", wantStatus: domain.StatusDisabled, wantRevoked: true},
		{name: "disable is idempotent", start: domain.StatusDisabled, raw: "disabled", wantStatus: domain.StatusDisabled, wantRevoked: true},
		{name: "re-enable does not reconnect anyone", start: domain.StatusDisabled, raw: "active", wantStatus: domain.StatusActive, wantRevoked: false},
		{name: "case and padding are tolerated", start: domain.StatusActive, raw: "  DISABLED ", wantStatus: domain.StatusDisabled, wantRevoked: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newUsersHarness(t)
			ctx := context.Background()
			id := h.seedAccount(domain.RoleDoctor, tc.start)
			h.seedSession(ctx, id)

			user, err := h.service.SetStatus(ctx, h.admin(), id, tc.raw)
			require.NoError(t, err)

			assert.Equal(t, tc.wantStatus, user.Status)
			assert.Equal(t, tc.wantStatus, h.users.statusOf(id))
			assert.Empty(t, user.PasswordHash)
			assert.Equal(t, tc.wantRevoked, h.refresh.isRevoked("digest-1"))
		})
	}
}

func TestSetStatusRejections(t *testing.T) {
	cases := []struct {
		name    string
		actor   domain.Role
		raw     string
		wantErr error
	}{
		{name: "doctor may not change status", actor: domain.RoleDoctor, raw: "disabled", wantErr: domain.ErrForbidden},
		{name: "unknown status", actor: domain.RoleAdmin, raw: "suspended", wantErr: domain.ErrInvalidInput},
		{name: "empty status", actor: domain.RoleAdmin, raw: "", wantErr: domain.ErrInvalidInput},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newUsersHarness(t)
			target := h.seedAccount(domain.RolePatient, domain.StatusActive)
			actorID := h.seedAccount(tc.actor, domain.StatusActive)

			user, err := h.service.SetStatus(context.Background(), Actor{UserID: actorID, Role: tc.actor}, target, tc.raw)

			assert.Equal(t, domain.User{}, user)
			assert.ErrorIs(t, err, tc.wantErr)
			assert.Equal(t, domain.StatusActive, h.users.statusOf(target), "the stored status must be untouched")
			assert.Zero(t, h.users.updated)
		})
	}
}

func TestSetStatusRefusesAdminSelfDisable(t *testing.T) {
	h := newUsersHarness(t)
	admin := h.admin()

	_, err := h.service.SetStatus(context.Background(), admin, admin.UserID, "disabled")
	assert.ErrorIs(t, err, domain.ErrConflict)
	assert.Equal(t, domain.StatusActive, h.users.statusOf(admin.UserID))
	assert.Zero(t, h.users.updated)
}

func TestSetStatusPropagatesRevocationFailure(t *testing.T) {
	h := newUsersHarness(t)
	id := h.seedAccount(domain.RoleDoctor, domain.StatusActive)
	h.refresh.failOn = "revoke-all"
	h.refresh.failErr = errStoreUnavailable

	_, err := h.service.SetStatus(context.Background(), h.admin(), id, "disabled")
	assert.ErrorIs(t, err, errStoreUnavailable)

	// The status write already happened; the next admin attempt can retry the revocation.
	assert.Equal(t, domain.StatusDisabled, h.users.statusOf(id))
}

func TestListIsAdminOnlyAndClampsPaging(t *testing.T) {
	cases := []struct {
		name       string
		actor      domain.Role
		filter     UserFilter
		wantErr    error
		wantLimit  int
		wantOffset int
	}{
		{name: "patient is refused", actor: domain.RolePatient, wantErr: domain.ErrForbidden},
		{name: "doctor is refused", actor: domain.RoleDoctor, wantErr: domain.ErrForbidden},
		{name: "default limit", actor: domain.RoleAdmin, wantLimit: defaultListLimit},
		{name: "limit above the ceiling is clamped", actor: domain.RoleAdmin, filter: UserFilter{Limit: 5000}, wantLimit: maxListLimit},
		{name: "negative limit falls back to the default", actor: domain.RoleAdmin, filter: UserFilter{Limit: -3}, wantLimit: defaultListLimit},
		{name: "negative offset falls back to zero", actor: domain.RoleAdmin, filter: UserFilter{Offset: -10}, wantLimit: defaultListLimit, wantOffset: 0},
		{name: "explicit limit is honoured", actor: domain.RoleAdmin, filter: UserFilter{Limit: 5}, wantLimit: 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newUsersHarness(t)
			actor := Actor{UserID: h.seedAccount(tc.actor, domain.StatusActive), Role: tc.actor}

			listed, err := h.service.List(context.Background(), actor, tc.filter)

			if tc.wantErr != nil {
				assert.Nil(t, listed)
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Zero(t, h.users.lastList.Limit, "a refused request must not reach the repository")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantLimit, h.users.lastList.Limit)
			assert.Equal(t, tc.wantOffset, h.users.lastList.Offset)
			for _, user := range listed {
				assert.Empty(t, user.PasswordHash, "a listing must never carry hashes")
			}
		})
	}
}

func TestListValidatesFilters(t *testing.T) {
	cases := []struct {
		name   string
		filter UserFilter
		field  string
	}{
		{name: "unknown role", filter: UserFilter{Role: domain.Role("receptionist")}, field: "role"},
		{name: "unknown status", filter: UserFilter{Status: domain.UserStatus("archived")}, field: "status"},
	}

	h := newUsersHarness(t)
	admin := h.admin()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			listed, err := h.service.List(context.Background(), admin, tc.filter)

			assert.Nil(t, listed)
			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrInvalidInput)

			var ve *domain.ValidationError
			require.True(t, errors.As(err, &ve))
			assert.Contains(t, ve.Error(), tc.field)
		})
	}
}

func TestListPropagatesRepositoryFailure(t *testing.T) {
	h := newUsersHarness(t)
	h.users.listErr = errStoreUnavailable

	listed, err := h.service.List(context.Background(), h.admin(), UserFilter{})
	assert.Nil(t, listed)
	assert.ErrorIs(t, err, errStoreUnavailable)
}
