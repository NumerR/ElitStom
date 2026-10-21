package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAccessTTL  = 15 * time.Minute
	testRefreshTTL = 72 * time.Hour
	testPassword   = "c0rrect-horse-battery"
)

type authHarness struct {
	t       *testing.T
	users   *fakeUsers
	refresh *fakeRefresh
	issuer  *fakeIssuer
	hasher  *fakeHasher
	secrets *fakeSecrets
	clock   *fakeClock
	auth    *Auth
}

func newAuthHarness(t *testing.T) *authHarness {
	t.Helper()

	h := &authHarness{
		t:       t,
		users:   newFakeUsers(),
		refresh: newFakeRefresh(),
		issuer:  newFakeIssuer(),
		hasher:  newFakeHasher(),
		secrets: &fakeSecrets{},
		clock:   newFakeClock(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)),
	}
	h.rebuild()
	return h
}

// rebuild wires Auth from whatever the fakes currently are, so a test can swap in a
// misbehaving port after the harness exists.
func (h *authHarness) rebuild() {
	h.t.Helper()

	auth, err := NewAuth(h.users, h.refresh, h.issuer, h.hasher, h.secrets, h.clock, AuthConfig{
		AccessTokenTTL:  testAccessTTL,
		RefreshTokenTTL: testRefreshTTL,
	})
	require.NoError(h.t, err)
	h.auth = auth
}

// register stores a patient account through the service so tests use the production path.
func (h *authHarness) register() (domain.User, TokenPair) {
	h.t.Helper()

	user, pair, err := h.auth.Register(context.Background(), RegisterInput{
		Email:    "patient@clinic.test",
		FullName: "Ira Vale",
		Phone:    "+1 555 0100",
		Password: testPassword,
	})
	require.NoError(h.t, err)
	return user, pair
}

func TestNewAuthRejectsMissingDependencies(t *testing.T) {
	cases := []struct {
		name    string
		wantErr string
		deps    func(h *authHarness) (UserRepository, RefreshTokenStore, TokenIssuer, PasswordHasher, SecretGenerator, Clock)
	}{
		{
			name:    "nil repository",
			wantErr: "users repository",
			deps: func(h *authHarness) (UserRepository, RefreshTokenStore, TokenIssuer, PasswordHasher, SecretGenerator, Clock) {
				return nil, h.refresh, h.issuer, h.hasher, h.secrets, h.clock
			},
		},
		{
			name:    "nil refresh store",
			wantErr: "refresh token store",
			deps: func(h *authHarness) (UserRepository, RefreshTokenStore, TokenIssuer, PasswordHasher, SecretGenerator, Clock) {
				return h.users, nil, h.issuer, h.hasher, h.secrets, h.clock
			},
		},
		{
			name:    "nil issuer",
			wantErr: "token issuer",
			deps: func(h *authHarness) (UserRepository, RefreshTokenStore, TokenIssuer, PasswordHasher, SecretGenerator, Clock) {
				return h.users, h.refresh, nil, h.hasher, h.secrets, h.clock
			},
		},
		{
			name:    "nil hasher",
			wantErr: "password hasher",
			deps: func(h *authHarness) (UserRepository, RefreshTokenStore, TokenIssuer, PasswordHasher, SecretGenerator, Clock) {
				return h.users, h.refresh, h.issuer, nil, h.secrets, h.clock
			},
		},
		{
			name:    "nil secret generator",
			wantErr: "secret generator",
			deps: func(h *authHarness) (UserRepository, RefreshTokenStore, TokenIssuer, PasswordHasher, SecretGenerator, Clock) {
				return h.users, h.refresh, h.issuer, h.hasher, nil, h.clock
			},
		},
		{
			name:    "nil clock",
			wantErr: "clock",
			deps: func(h *authHarness) (UserRepository, RefreshTokenStore, TokenIssuer, PasswordHasher, SecretGenerator, Clock) {
				return h.users, h.refresh, h.issuer, h.hasher, h.secrets, nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newAuthHarness(t)
			users, refresh, issuer, hasher, secrets, clock := tc.deps(h)

			auth, err := NewAuth(users, refresh, issuer, hasher, secrets, clock, AuthConfig{
				AccessTokenTTL:  testAccessTTL,
				RefreshTokenTTL: testRefreshTTL,
			})

			assert.Nil(t, auth)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestNewAuthRejectsNonPositiveTokenTTLs(t *testing.T) {
	cases := []struct {
		name       string
		accessTTL  time.Duration
		refreshTTL time.Duration
	}{
		{name: "zero access ttl", accessTTL: 0, refreshTTL: testRefreshTTL},
		{name: "negative access ttl", accessTTL: -time.Minute, refreshTTL: testRefreshTTL},
		{name: "refresh equal to access", accessTTL: testAccessTTL, refreshTTL: testAccessTTL},
		{name: "refresh shorter than access", accessTTL: testAccessTTL, refreshTTL: time.Minute},
	}

	h := newAuthHarness(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			auth, err := NewAuth(h.users, h.refresh, h.issuer, h.hasher, h.secrets, h.clock, AuthConfig{
				AccessTokenTTL:  tc.accessTTL,
				RefreshTokenTTL: tc.refreshTTL,
			})

			assert.Nil(t, auth)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must exceed")
		})
	}
}

func TestRegisterCreatesPatientSession(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()

	user, pair, err := h.auth.Register(ctx, RegisterInput{
		Email:    "  Patient@Clinic.Test ",
		FullName: "Ira   Vale",
		Phone:    "+1 (555) 010-0",
		Password: testPassword,
	})
	require.NoError(t, err)

	assert.Equal(t, "patient@clinic.test", user.Email, "email must be normalised before storage")
	assert.Equal(t, "Ira Vale", user.FullName, "runs of spaces must collapse")
	assert.Equal(t, domain.RolePatient, user.Role, "self-service signup can never choose a privileged role")
	assert.Equal(t, domain.StatusActive, user.Status)
	assert.Empty(t, user.PasswordHash, "the service must not hand a hash back to transport")

	assert.NotEmpty(t, pair.AccessToken)
	assert.NotEmpty(t, pair.RefreshToken)
	// The access expiry comes from the issuer; the refresh expiry is the service's own arithmetic.
	assert.False(t, pair.AccessExpiresAt.IsZero())
	assert.Equal(t, h.clock.Now().Add(testRefreshTTL), pair.RefreshExpiresAt)

	stored, err := h.refresh.ByHash(ctx, h.secrets.HashToken(pair.RefreshToken))
	require.NoError(t, err)
	assert.Equal(t, user.ID, stored.UserID)
	assert.NotEqual(t, pair.RefreshToken, stored.TokenHash, "only the digest may be persisted")
}

func TestRegisterRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name  string
		input RegisterInput
		field string
	}{
		{name: "short password", input: RegisterInput{Email: "a@b.co", FullName: "Ab", Password: "short"}, field: "password"},
		{name: "missing password", input: RegisterInput{Email: "a@b.co", FullName: "Ab"}, field: "password"},
		{
			name:  "overlong password",
			input: RegisterInput{Email: "a@b.co", FullName: "Ab", Password: strings.Repeat("k", domain.MaxPasswordBytes+1)},
			field: "password",
		},
		{name: "missing email", input: RegisterInput{FullName: "Ab", Password: testPassword}, field: "email"},
		{name: "domainless email", input: RegisterInput{Email: "no-dot@localhost", FullName: "Ab", Password: testPassword}, field: "email"},
		{name: "missing name", input: RegisterInput{Email: "a@b.co", Password: testPassword}, field: "full_name"},
		{name: "digits in name", input: RegisterInput{Email: "a@b.co", FullName: "Ab2", Password: testPassword}, field: "full_name"},
		{name: "too few phone digits", input: RegisterInput{Email: "a@b.co", FullName: "Ab", Phone: "12345", Password: testPassword}, field: "phone"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newAuthHarness(t)

			user, pair, err := h.auth.Register(context.Background(), tc.input)

			assert.Equal(t, domain.User{}, user)
			assert.Equal(t, TokenPair{}, pair)
			require.Error(t, err)
			assert.ErrorIs(t, err, domain.ErrInvalidInput)

			var ve *domain.ValidationError
			require.True(t, errors.As(err, &ve), "must be a validation error")
			assert.Contains(t, ve.Error(), tc.field)
			assert.Zero(t, h.users.created, "nothing may be written")
			assert.Zero(t, h.hasher.hashes, "an invalid password must not be hashed")
		})
	}
}

func TestRegisterRejectsDuplicateEmailBeforeSession(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()

	first, _, err := h.auth.Register(ctx, RegisterInput{Email: "dup@clinic.test", FullName: "Ab", Password: testPassword})
	require.NoError(t, err)
	sessions := h.refresh.inserts()

	_, pair, err := h.auth.Register(ctx, RegisterInput{Email: "dup@clinic.test", FullName: "Cd", Password: testPassword})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrAlreadyExists)
	assert.Equal(t, TokenPair{}, pair, "a rejected signup must not mint a session")
	assert.Equal(t, sessions, h.refresh.inserts(), "no refresh token may be stored")

	stored, err := h.users.ByID(ctx, first.ID)
	require.NoError(t, err)
	assert.Equal(t, "Ab", stored.FullName, "the failed attempt must not overwrite the account")
}

func TestRegisterSurfacesRepositoryFailure(t *testing.T) {
	h := newAuthHarness(t)
	wantErr := errors.New("db down")
	h.users.createErr = wantErr

	_, _, err := h.auth.Register(context.Background(), RegisterInput{Email: "a@b.co", FullName: "Ab", Password: testPassword})
	assert.ErrorIs(t, err, wantErr)
	assert.Equal(t, 1, h.hasher.hashes, "the password is hashed before the write is attempted")
}

func TestRegisterPropagatesHasherFailure(t *testing.T) {
	h := newAuthHarness(t)
	h.hasher.err = errors.New("bcrypt unavailable")

	_, _, err := h.auth.Register(context.Background(), RegisterInput{Email: "a@b.co", FullName: "Ab", Password: testPassword})
	assert.ErrorIs(t, err, h.hasher.err)
	assert.Zero(t, h.users.created)
}

func TestLoginAcceptsCorrectPassword(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	stored, first := h.register()

	user, pair, err := h.auth.Login(ctx, LoginInput{Email: "PATIENT@clinic.test", Password: testPassword})
	require.NoError(t, err)

	assert.Equal(t, stored.ID, user.ID)
	assert.Empty(t, user.PasswordHash)
	assert.NotEqual(t, first.RefreshToken, pair.RefreshToken, "each login mints its own secret")
	assert.Equal(t, domain.RolePatient, h.issuer.actorFor(t, pair.AccessToken).Role)
	assert.Equal(t, stored.ID, h.issuer.actorFor(t, pair.AccessToken).UserID)

	original, err := h.refresh.ByHash(ctx, "digest:"+first.RefreshToken)
	require.NoError(t, err)
	rotated, err := h.refresh.ByHash(ctx, "digest:"+pair.RefreshToken)
	require.NoError(t, err)
	assert.NotEqual(t, original.FamilyID, rotated.FamilyID, "a login opens a new family")
}

func TestLoginRejections(t *testing.T) {
	cases := []struct {
		name      string
		email     string
		password  string
		wantErr   error
		wantDummy bool
		disable   bool
	}{
		{name: "wrong password", email: "patient@clinic.test", password: "wrong-password-here", wantErr: domain.ErrInvalidCredentials},
		{name: "unknown email", email: "ghost@clinic.test", password: testPassword, wantErr: domain.ErrInvalidCredentials, wantDummy: true},
		{name: "malformed email", email: "not-an-email", password: testPassword, wantErr: domain.ErrInvalidInput},
		{name: "disabled account with correct password", email: "patient@clinic.test", password: testPassword, wantErr: domain.ErrUserDisabled, disable: true},
		{name: "disabled account with wrong password", email: "patient@clinic.test", password: "wrong-password-here", wantErr: domain.ErrInvalidCredentials, disable: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newAuthHarness(t)
			user, _ := h.register()
			if tc.disable {
				h.users.forceStatus(user.ID, domain.StatusDisabled)
			}
			sessions := h.refresh.inserts()

			got, pair, err := h.auth.Login(context.Background(), LoginInput{Email: tc.email, Password: tc.password})

			assert.Equal(t, domain.User{}, got)
			assert.Equal(t, TokenPair{}, pair)
			assert.ErrorIs(t, err, tc.wantErr)
			assert.Equal(t, sessions, h.refresh.inserts(), "a failed login must not create a session")

			if tc.wantDummy {
				assert.Equal(t, 1, h.hasher.dummyChecks, "an unknown account must still burn bcrypt time")
			} else {
				assert.Zero(t, h.hasher.dummyChecks)
			}
		})
	}
}

func TestLoginPropagatesRepositoryFailure(t *testing.T) {
	h := newAuthHarness(t)
	wantErr := errors.New("connection reset")
	h.users.emailLookupErr = wantErr

	_, _, err := h.auth.Login(context.Background(), LoginInput{Email: "patient@clinic.test", Password: testPassword})
	assert.ErrorIs(t, err, wantErr)
	assert.Zero(t, h.hasher.dummyChecks, "a failure must not be reported as a bad password")
}

func TestRefreshRotatesWithinTheSameFamily(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	user, first := h.register()

	rotated, pair, err := h.auth.Refresh(ctx, first.RefreshToken)
	require.NoError(t, err)

	assert.Equal(t, user.ID, rotated.ID)
	assert.Empty(t, rotated.PasswordHash)
	assert.NotEqual(t, first.RefreshToken, pair.RefreshToken)
	assert.WithinDuration(t, h.clock.Now().Add(testRefreshTTL), pair.RefreshExpiresAt, time.Second)

	assert.True(t, h.refresh.isUsed("digest:"+first.RefreshToken), "the presented token must be consumed")
	assert.False(t, h.refresh.isUsed("digest:"+pair.RefreshToken))
	assert.False(t, h.refresh.isRevoked("digest:"+pair.RefreshToken))

	// The successor stays in the original family so reuse detection has something to revoke.
	old, err := h.refresh.ByHash(ctx, "digest:"+first.RefreshToken)
	require.NoError(t, err)
	fresh, err := h.refresh.ByHash(ctx, "digest:"+pair.RefreshToken)
	require.NoError(t, err)
	assert.Equal(t, old.FamilyID, fresh.FamilyID)
}

func TestRefreshRejections(t *testing.T) {
	cases := []struct {
		name    string
		token   func(h *authHarness, pair TokenPair) string
		wantErr error
		prep    func(h *authHarness, pair TokenPair)
	}{
		{name: "empty token", token: func(*authHarness, TokenPair) string { return "" }, wantErr: domain.ErrTokenInvalid},
		{name: "unknown token", token: func(*authHarness, TokenPair) string { return "never-minted" }, wantErr: domain.ErrTokenInvalid},
		{
			name:    "expired token",
			token:   func(_ *authHarness, pair TokenPair) string { return pair.RefreshToken },
			wantErr: domain.ErrTokenExpired,
			prep:    func(h *authHarness, _ TokenPair) { h.clock.Advance(testRefreshTTL + time.Minute) },
		},
		{
			name:    "revoked token",
			token:   func(_ *authHarness, pair TokenPair) string { return pair.RefreshToken },
			wantErr: domain.ErrSessionRevoked,
			prep: func(h *authHarness, pair TokenPair) {
				stored, err := h.refresh.ByHash(context.Background(), "digest:"+pair.RefreshToken)
				require.NoError(h.t, err)
				require.NoError(h.t, h.refresh.RevokeFamily(context.Background(), stored.FamilyID, h.clock.Now()))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newAuthHarness(t)
			_, pair := h.register()
			if tc.prep != nil {
				tc.prep(h, pair)
			}

			user, next, err := h.auth.Refresh(context.Background(), tc.token(h, pair))

			assert.Equal(t, domain.User{}, user)
			assert.Equal(t, TokenPair{}, next)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestRefreshReuseRevokesTheWholeFamily(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	_, pair := h.register()

	// Legitimate rotation consumes the first token and issues descendants in the same family.
	_, second, err := h.auth.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err)
	_, third, err := h.auth.Refresh(ctx, second.RefreshToken)
	require.NoError(t, err)

	// Replaying the leaked first token must kill every descendant.
	user, next, err := h.auth.Refresh(ctx, pair.RefreshToken)
	assert.ErrorIs(t, err, domain.ErrSessionRevoked)
	assert.Equal(t, domain.User{}, user)
	assert.Equal(t, TokenPair{}, next)

	assert.True(t, h.refresh.isRevoked("digest:"+pair.RefreshToken))
	assert.True(t, h.refresh.isRevoked("digest:"+second.RefreshToken))
	assert.True(t, h.refresh.isRevoked("digest:"+third.RefreshToken), "the newest token must be revoked too")

	_, _, err = h.auth.Refresh(ctx, third.RefreshToken)
	assert.ErrorIs(t, err, domain.ErrSessionRevoked)
}

func TestRefreshConcurrentReplayRevokesTheFamily(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	_, pair := h.register()

	// Two requests presented the same secret at once: the atomic consume hands the token to
	// one and reports false to the other. A "false" is treated as a leak, not a retry.
	h.refresh.consumeAlwaysLoses = true

	_, next, err := h.auth.Refresh(ctx, pair.RefreshToken)
	assert.ErrorIs(t, err, domain.ErrSessionRevoked)
	assert.Equal(t, TokenPair{}, next)
	assert.True(t, h.refresh.isRevoked("digest:"+pair.RefreshToken))
}

func TestRefreshRejectsDisabledAccount(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	user, pair := h.register()
	h.users.forceStatus(user.ID, domain.StatusDisabled)

	got, next, err := h.auth.Refresh(ctx, pair.RefreshToken)
	assert.ErrorIs(t, err, domain.ErrUserDisabled)
	assert.Equal(t, domain.User{}, got)
	assert.Equal(t, TokenPair{}, next)
	assert.True(t, h.refresh.isUsed("digest:"+pair.RefreshToken), "a consumed token stays consumed")
}

func TestRefreshUnknownAccountRevokesItsFamily(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	user, pair := h.register()

	// A deleted account leaves an orphaned token behind; rotating it must not mint a session.
	require.NoError(t, h.users.deleteAccount(user.ID))

	_, _, err := h.auth.Refresh(ctx, pair.RefreshToken)
	assert.ErrorIs(t, err, domain.ErrSessionRevoked)
	assert.True(t, h.refresh.isRevoked("digest:"+pair.RefreshToken))
}

func TestRefreshPropagatesStoreFailures(t *testing.T) {
	cases := []struct {
		name     string
		failOn   string
		wantText string
	}{
		{name: "lookup fails", failOn: "by-hash", wantText: ""},
		{name: "consume fails", failOn: "consume", wantText: "consume refresh token"},
		{name: "insert fails", failOn: "insert", wantText: "store refresh token"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newAuthHarness(t)
			_, pair := h.register()
			h.refresh.failOn = tc.failOn
			h.refresh.failErr = errStoreUnavailable

			_, _, err := h.auth.Refresh(context.Background(), pair.RefreshToken)
			assert.ErrorIs(t, err, errStoreUnavailable)
			if tc.wantText != "" {
				assert.Contains(t, err.Error(), tc.wantText)
			}
		})
	}
}

func TestRefreshReuseRevocationFailureSurfaces(t *testing.T) {
	h := newAuthHarness(t)
	ctx := context.Background()
	_, pair := h.register()

	// Consume correctly says "somebody got here first"; the family revocation then fails.
	_, _, err := h.auth.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err)

	h.refresh.failOn = "revoke-family"
	h.refresh.failErr = errStoreUnavailable

	_, _, err = h.auth.Refresh(ctx, pair.RefreshToken)
	assert.ErrorIs(t, err, errStoreUnavailable)
}

func TestLogout(t *testing.T) {
	cases := []struct {
		name        string
		token       func(h *authHarness, pair TokenPair) string
		actor       func(h *authHarness, user domain.User) Actor
		wantErr     error
		wantRevoked bool
	}{
		{
			name:        "own session",
			token:       func(_ *authHarness, pair TokenPair) string { return pair.RefreshToken },
			actor:       func(_ *authHarness, user domain.User) Actor { return Actor{UserID: user.ID, Role: user.Role} },
			wantErr:     nil,
			wantRevoked: true,
		},
		{
			name:        "unknown token is idempotent",
			token:       func(_ *authHarness, _ TokenPair) string { return "not-a-real-token" },
			actor:       func(_ *authHarness, user domain.User) Actor { return Actor{UserID: user.ID, Role: user.Role} },
			wantErr:     nil,
			wantRevoked: false,
		},
		{
			name:        "empty token",
			token:       func(_ *authHarness, _ TokenPair) string { return "" },
			actor:       func(_ *authHarness, user domain.User) Actor { return Actor{UserID: user.ID, Role: user.Role} },
			wantErr:     domain.ErrTokenInvalid,
			wantRevoked: false,
		},
		{
			name:  "another account's token",
			token: func(_ *authHarness, pair TokenPair) string { return pair.RefreshToken },
			actor: func(*authHarness, domain.User) Actor {
				return Actor{UserID: "attacker-1", Role: domain.RoleAdmin}
			},
			wantErr:     domain.ErrTokenInvalid,
			wantRevoked: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newAuthHarness(t)
			user, pair := h.register()

			err := h.auth.Logout(context.Background(), tc.actor(h, user), tc.token(h, pair))
			assert.ErrorIs(t, err, tc.wantErr)
			assert.Equal(t, tc.wantRevoked, h.refresh.isRevoked("digest:"+pair.RefreshToken))
		})
	}
}
