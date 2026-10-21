// Package auth implements the credential primitives the usecase layer consumes:
// access-token issuing and verification, password hashing, and opaque secret minting.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/NumerR/ElitStom/internal/config"
	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// accessClaims is the wire format of an access token. Role travels in the token so that
// authorization decisions need no database round trip.
type accessClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// leeway absorbs clock drift between nodes. JWT timestamps have one-second resolution, so
// without it a token minted by a slightly-ahead peer would be rejected on arrival.
const leeway = 2 * time.Second

type TokenIssuer struct {
	secret []byte
	cfg    config.Auth
	now    func() time.Time
}

// NewTokenIssuer builds an issuer. The clock function is injected so expiry behaviour is
// testable without sleeping.
func NewTokenIssuer(cfg config.Auth, now func() time.Time) (*TokenIssuer, error) {
	if cfg.JWTSecret == "" {
		return nil, errors.New("auth: JWT secret must not be empty")
	}
	if len(cfg.JWTSecret) < minSecretBytes {
		return nil, fmt.Errorf("auth: JWT secret must be at least %d bytes, got %d", minSecretBytes, len(cfg.JWTSecret))
	}
	if now == nil {
		now = time.Now
	}
	return &TokenIssuer{secret: []byte(cfg.JWTSecret), cfg: cfg, now: now}, nil
}

// minSecretBytes mirrors the HS256 key-size floor enforced by config validation; the
// issuer re-checks it so it can never be built with a weak key from any call site.
const minSecretBytes = 32

// IssueAccessToken returns a signed token together with its expiry for the API response.
func (i *TokenIssuer) IssueAccessToken(userID string, role domain.Role) (string, time.Time, error) {
	if !role.IsValid() {
		return "", time.Time{}, fmt.Errorf("auth: cannot issue a token for role %q", string(role))
	}

	now := i.now()
	expiresAt := now.Add(i.cfg.AccessTokenTTL)

	tokenID, err := randomBytes(16)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: token id: %w", err)
	}

	claims := accessClaims{
		Role: role.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    i.cfg.Issuer,
			Audience:  jwt.ClaimStrings{i.cfg.Audience},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        base64.RawURLEncoding.EncodeToString(tokenID),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: sign access token: %w", err)
	}
	return signed, expiresAt, nil
}

// ParseAccessToken verifies signature, algorithm, issuer, audience and lifetime.
// A failure is always reported as a domain auth error so handlers never branch on
// library-specific types.
func (i *TokenIssuer) ParseAccessToken(raw string) (usecase.Actor, error) {
	claims := &accessClaims{}

	token, err := jwt.ParseWithClaims(raw, claims, i.keyFunc,
		// Pinning the algorithm is what blocks an "alg":"none" downgrade and an
		// HMAC/RSA confusion attack that treats a public key as a shared secret.
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(i.cfg.Issuer),
		jwt.WithAudience(i.cfg.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(leeway),
		jwt.WithTimeFunc(i.now),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return usecase.Actor{}, fmt.Errorf("%w: %w", domain.ErrTokenExpired, err)
		}
		return usecase.Actor{}, fmt.Errorf("%w: %w", domain.ErrTokenInvalid, err)
	}
	if !token.Valid {
		return usecase.Actor{}, fmt.Errorf("%w: validator rejected the token", domain.ErrTokenInvalid)
	}
	if claims.Subject == "" {
		return usecase.Actor{}, fmt.Errorf("%w: token has no subject", domain.ErrTokenInvalid)
	}

	role := domain.Role(claims.Role)
	if !role.IsValid() {
		// Deliberately not domain.ParseRole: its ValidationError would make this look like
		// client input rather than a forged or stale token.
		return usecase.Actor{}, fmt.Errorf("%w: token carries an unknown role", domain.ErrTokenInvalid)
	}

	return usecase.Actor{UserID: claims.Subject, Role: role}, nil
}

func (i *TokenIssuer) keyFunc(token *jwt.Token) (any, error) {
	if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("%w: unexpected signing method", domain.ErrTokenInvalid)
	}
	return i.secret, nil
}

func randomBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("read %d random bytes: %w", n, err)
	}
	return buf, nil
}
