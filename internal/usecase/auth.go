package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/NumerR/ElitStom/internal/domain"
)

// AuthConfig carries the token policy the service enforces.
type AuthConfig struct {
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

// TokenPair is a freshly minted session. RefreshToken is the only place a plaintext
// refresh secret ever leaves the system.
type TokenPair struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

type RegisterInput struct {
	Email    string
	FullName string
	Phone    string
	Password string
}

type LoginInput struct {
	Email    string
	Password string
}

// Auth implements registration, login, refresh-token rotation and logout.
type Auth struct {
	users   UserRepository
	refresh RefreshTokenStore
	issuer  TokenIssuer
	hasher  PasswordHasher
	secrets SecretGenerator
	clock   Clock
	cfg     AuthConfig
}

func NewAuth(
	users UserRepository,
	refresh RefreshTokenStore,
	issuer TokenIssuer,
	hasher PasswordHasher,
	secrets SecretGenerator,
	clock Clock,
	cfg AuthConfig,
) (*Auth, error) {
	missing := []string{}
	if users == nil {
		missing = append(missing, "users repository")
	}
	if refresh == nil {
		missing = append(missing, "refresh token store")
	}
	if issuer == nil {
		missing = append(missing, "token issuer")
	}
	if hasher == nil {
		missing = append(missing, "password hasher")
	}
	if secrets == nil {
		missing = append(missing, "secret generator")
	}
	if clock == nil {
		missing = append(missing, "clock")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("usecase: NewAuth is missing %v", missing)
	}
	if cfg.AccessTokenTTL <= 0 || cfg.RefreshTokenTTL <= cfg.AccessTokenTTL {
		return nil, fmt.Errorf("usecase: refresh token TTL (%s) must exceed access token TTL (%s)", cfg.RefreshTokenTTL, cfg.AccessTokenTTL)
	}

	return &Auth{
		users:   users,
		refresh: refresh,
		issuer:  issuer,
		hasher:  hasher,
		secrets: secrets,
		clock:   clock,
		cfg:     cfg,
	}, nil
}

// Register creates a self-service account. The role is always patient: privilege is only
// ever granted through an admin endpoint.
func (a *Auth) Register(ctx context.Context, in RegisterInput) (domain.User, TokenPair, error) {
	if err := domain.ValidatePasswordStrength(in.Password); err != nil {
		return domain.User{}, TokenPair{}, err
	}
	if err := domain.ValidateAccount(in.Email, in.FullName, in.Phone, domain.RolePatient); err != nil {
		return domain.User{}, TokenPair{}, err
	}

	hash, err := a.hasher.Hash(in.Password)
	if err != nil {
		return domain.User{}, TokenPair{}, err
	}

	user, err := domain.NewUser(domain.UserParams{
		Email:        in.Email,
		FullName:     in.FullName,
		Phone:        in.Phone,
		Role:         domain.RolePatient,
		PasswordHash: hash,
	})
	if err != nil {
		return domain.User{}, TokenPair{}, err
	}

	created, err := a.users.Create(ctx, *user)
	if err != nil {
		return domain.User{}, TokenPair{}, err
	}

	// A session failure after the account was stored surfaces as a server error: the
	// account exists and the client should continue with login rather than retry signup.
	pair, err := a.newSession(ctx, created, "")
	if err != nil {
		return domain.User{}, TokenPair{}, err
	}
	return sanitize(created), pair, nil
}

// Login authenticates by email and password and starts a new session family.
func (a *Auth) Login(ctx context.Context, in LoginInput) (domain.User, TokenPair, error) {
	email, err := domain.NormalizeEmail(in.Email)
	if err != nil {
		return domain.User{}, TokenPair{}, err
	}

	user, err := a.users.ByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Verify against a hash no password matches so an unknown account costs the same
			// CPU time as a known one and cannot be told apart by response timing.
			if verifyErr := a.hasher.Verify(a.hasher.DummyHash(), in.Password); verifyErr != nil &&
				!errors.Is(verifyErr, domain.ErrInvalidCredentials) {
				return domain.User{}, TokenPair{}, verifyErr
			}
			return domain.User{}, TokenPair{}, domain.ErrInvalidCredentials
		}
		return domain.User{}, TokenPair{}, err
	}

	if err := a.hasher.Verify(user.PasswordHash, in.Password); err != nil {
		return domain.User{}, TokenPair{}, err
	}
	// Checked after the password so only someone who already knows it learns the account is off.
	if user.IsDisabled() {
		return domain.User{}, TokenPair{}, domain.ErrUserDisabled
	}

	pair, err := a.newSession(ctx, user, "")
	if err != nil {
		return domain.User{}, TokenPair{}, err
	}
	return sanitize(user), pair, nil
}

// Refresh rotates a refresh token: the presented one is retired and a successor in the same
// family is issued. Presenting an already-consumed token revokes the entire family.
func (a *Auth) Refresh(ctx context.Context, refreshToken string) (domain.User, TokenPair, error) {
	if refreshToken == "" {
		return domain.User{}, TokenPair{}, domain.ErrTokenInvalid
	}

	stored, err := a.refresh.ByHash(ctx, a.secrets.HashToken(refreshToken))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.User{}, TokenPair{}, domain.ErrTokenInvalid
		}
		return domain.User{}, TokenPair{}, err
	}

	now := a.clock.Now()

	switch {
	case stored.IsUsed():
		return a.killFamily(ctx, stored, now, domain.ErrSessionRevoked)
	case stored.IsRevoked():
		return domain.User{}, TokenPair{}, domain.ErrSessionRevoked
	case stored.IsExpired(now):
		return domain.User{}, TokenPair{}, domain.ErrTokenExpired
	}

	consumed, err := a.refresh.Consume(ctx, stored.ID, now)
	if err != nil {
		return domain.User{}, TokenPair{}, fmt.Errorf("usecase: consume refresh token: %w", err)
	}
	if !consumed {
		// Two requests presented the same secret concurrently: treat it as a leak.
		return a.killFamily(ctx, stored, now, domain.ErrSessionRevoked)
	}

	user, err := a.users.ByID(ctx, stored.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return a.killFamily(ctx, stored, now, domain.ErrSessionRevoked)
		}
		return domain.User{}, TokenPair{}, err
	}
	if user.IsDisabled() {
		return domain.User{}, TokenPair{}, domain.ErrUserDisabled
	}

	pair, err := a.newSession(ctx, user, stored.FamilyID)
	if err != nil {
		return domain.User{}, TokenPair{}, err
	}
	return sanitize(user), pair, nil
}

// Logout revokes the family a refresh token belongs to. It is idempotent, and an unknown or
// foreign token is never acknowledged: the caller learns nothing about other sessions.
func (a *Auth) Logout(ctx context.Context, actor Actor, refreshToken string) error {
	if refreshToken == "" {
		return domain.ErrTokenInvalid
	}

	stored, err := a.refresh.ByHash(ctx, a.secrets.HashToken(refreshToken))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}
	if stored.UserID != actor.UserID {
		if revokeErr := a.refresh.RevokeFamily(ctx, stored.FamilyID, a.clock.Now()); revokeErr != nil {
			return fmt.Errorf("usecase: revoke token family: %w", revokeErr)
		}
		return domain.ErrTokenInvalid
	}

	if err := a.refresh.RevokeFamily(ctx, stored.FamilyID, a.clock.Now()); err != nil {
		return fmt.Errorf("usecase: revoke token family: %w", err)
	}
	return nil
}

// killFamily revokes every token descended from stored and reports cause.
func (a *Auth) killFamily(ctx context.Context, stored domain.RefreshToken, at time.Time, cause error) (domain.User, TokenPair, error) {
	if err := a.refresh.RevokeFamily(ctx, stored.FamilyID, at); err != nil {
		return domain.User{}, TokenPair{}, fmt.Errorf("usecase: revoke token family: %w", err)
	}
	return domain.User{}, TokenPair{}, cause
}

// newSession issues one access/refresh pair. An empty familyID starts a new family (login);
// a non-empty one continues an existing family (rotation).
func (a *Auth) newSession(ctx context.Context, user domain.User, familyID string) (TokenPair, error) {
	if user.ID == "" {
		return TokenPair{}, errors.New("usecase: cannot open a session for a user without an id")
	}

	now := a.clock.Now()

	access, accessExpiresAt, err := a.issuer.IssueAccessToken(user.ID, user.Role)
	if err != nil {
		return TokenPair{}, fmt.Errorf("usecase: issue access token: %w", err)
	}

	if familyID == "" {
		if familyID, err = a.secrets.NewFamilyID(); err != nil {
			return TokenPair{}, fmt.Errorf("usecase: new token family: %w", err)
		}
	}

	secret, err := a.secrets.NewRefreshToken()
	if err != nil {
		return TokenPair{}, fmt.Errorf("usecase: new refresh token: %w", err)
	}

	refreshExpiresAt := now.Add(a.cfg.RefreshTokenTTL)
	token, err := domain.NewRefreshToken(domain.RefreshTokenParams{
		UserID:    user.ID,
		FamilyID:  familyID,
		TokenHash: a.secrets.HashToken(secret),
		ExpiresAt: refreshExpiresAt,
		Now:       now,
	})
	if err != nil {
		return TokenPair{}, err
	}

	if _, err := a.refresh.Insert(ctx, *token); err != nil {
		return TokenPair{}, fmt.Errorf("usecase: store refresh token: %w", err)
	}

	return TokenPair{
		AccessToken:      access,
		AccessExpiresAt:  accessExpiresAt,
		RefreshToken:     secret,
		RefreshExpiresAt: refreshExpiresAt,
	}, nil
}
