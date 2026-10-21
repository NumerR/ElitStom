package auth

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/NumerR/ElitStom/internal/config"
	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/usecase"
)

const testSecret = "0123456789abcdef0123456789abcdef"

var testIssuedAt = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func testAuthConfig() config.Auth {
	return config.Auth{
		JWTSecret:       testSecret,
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 72 * time.Hour,
		Issuer:          "dentacore",
		Audience:        "dentacore-api",
		BcryptCost:      10,
	}
}

func newTestIssuer(t *testing.T, cfg config.Auth) *TokenIssuer {
	t.Helper()

	issuer, err := NewTokenIssuer(cfg, func() time.Time { return testIssuedAt })
	if err != nil {
		t.Fatalf("NewTokenIssuer() error = %v", err)
	}
	return issuer
}

// claimsFor builds a token with the given clock so a test can place expiry in the past or
// the future without waiting for it.
func claimsFor(t *testing.T, cfg config.Auth, at time.Time, mutate func(*accessClaims)) string {
	t.Helper()

	claims := accessClaims{
		Role: domain.RoleDoctor.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			Issuer:    cfg.Issuer,
			Audience:  jwt.ClaimStrings{cfg.Audience},
			ExpiresAt: jwt.NewNumericDate(at.Add(cfg.AccessTokenTTL)),
			NotBefore: jwt.NewNumericDate(at),
			IssuedAt:  jwt.NewNumericDate(at),
			ID:        "test-jti",
		},
	}
	if mutate != nil {
		mutate(&claims)
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return signed
}

func TestNewTokenIssuerRejectsWeakSecrets(t *testing.T) {
	cases := []struct {
		name    string
		secret  string
		wantErr string
	}{
		{name: "empty", secret: "", wantErr: "must not be empty"},
		{name: "shorter than the HS256 floor", secret: "short-secret", wantErr: "at least 32 bytes"},
		{name: "one byte short", secret: strings.Repeat("k", 31), wantErr: "at least 32 bytes"},
		{name: "exactly the floor", secret: strings.Repeat("k", 32)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testAuthConfig()
			cfg.JWTSecret = tc.secret

			issuer, err := NewTokenIssuer(cfg, nil)

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("NewTokenIssuer() error = %v, want it to contain %q", err, tc.wantErr)
				}
				if issuer != nil {
					t.Error("NewTokenIssuer() returned an issuer alongside an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewTokenIssuer() error = %v", err)
			}
		})
	}
}

func TestIssueAndParseAccessTokenRoundTrip(t *testing.T) {
	cfg := testAuthConfig()
	issuer := newTestIssuer(t, cfg)

	cases := []struct {
		name string
		role domain.Role
	}{
		{name: "admin", role: domain.RoleAdmin},
		{name: "doctor", role: domain.RoleDoctor},
		{name: "patient", role: domain.RolePatient},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, expiresAt, err := issuer.IssueAccessToken("user-42", tc.role)
			if err != nil {
				t.Fatalf("IssueAccessToken() error = %v", err)
			}
			if want := testIssuedAt.Add(cfg.AccessTokenTTL); !expiresAt.Equal(want) {
				t.Errorf("expiresAt = %v, want %v", expiresAt, want)
			}

			actor, err := issuer.ParseAccessToken(raw)
			if err != nil {
				t.Fatalf("ParseAccessToken() error = %v", err)
			}
			if actor != (usecase.Actor{UserID: "user-42", Role: tc.role}) {
				t.Errorf("actor = %+v, want the issued identity", actor)
			}
		})
	}
}

func TestIssueAccessTokenRejectsUnknownRole(t *testing.T) {
	cases := []struct {
		name string
		role domain.Role
	}{
		{name: "empty role", role: domain.Role("")},
		{name: "unknown role", role: domain.Role("receptionist")},
		{name: "upper case is not normalised", role: domain.Role("ADMIN")},
	}

	issuer := newTestIssuer(t, testAuthConfig())
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, expiresAt, err := issuer.IssueAccessToken("user-1", tc.role)
			if err == nil {
				t.Fatal("IssueAccessToken() = nil error, want a refusal")
			}
			if raw != "" {
				t.Errorf("token = %q, want empty on failure", raw)
			}
			if !expiresAt.IsZero() {
				t.Errorf("expiresAt = %v, want the zero time", expiresAt)
			}
		})
	}
}

func TestIssueAccessTokenUsesAFreshID(t *testing.T) {
	issuer := newTestIssuer(t, testAuthConfig())
	seen := map[string]bool{}

	for range 25 {
		raw, _, err := issuer.IssueAccessToken("user-1", domain.RolePatient)
		if err != nil {
			t.Fatalf("IssueAccessToken() error = %v", err)
		}
		if seen[raw] {
			t.Fatal("two tokens are byte-identical, so the jti is not random")
		}
		seen[raw] = true
	}
}

func TestParseAccessTokenRejectsForgedTokens(t *testing.T) {
	cfg := testAuthConfig()
	issuer := newTestIssuer(t, cfg)

	unsignedHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	unsignedPayload := base64.RawURLEncoding.EncodeToString([]byte(`{"role":"admin","sub":"user-1","iss":"dentacore","aud":"dentacore-api","exp":9999999999}`))

	cases := []struct {
		name    string
		raw     string
		wantErr error
	}{
		{name: "empty string", raw: "", wantErr: domain.ErrTokenInvalid},
		{name: "not a token", raw: "hello", wantErr: domain.ErrTokenInvalid},
		{name: "two segments", raw: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c2VyLTEifQ.", wantErr: domain.ErrTokenInvalid},
		{name: "alg none downgrade", raw: unsignedHeader + "." + unsignedPayload + ".", wantErr: domain.ErrTokenInvalid},
		{name: "payload tampered after signing", raw: tamper(t, cfg), wantErr: domain.ErrTokenInvalid},
		{name: "signed with another secret", raw: claimsForWithSecret(t, cfg, "the-attacker-guessed-a-different-secret-32b"), wantErr: domain.ErrTokenInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actor, err := issuer.ParseAccessToken(tc.raw)

			if err == nil {
				t.Fatalf("ParseAccessToken(%q) accepted a forged token: %+v", truncate(tc.raw), actor)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("error = %v, want errors.Is(err, %v)", err, tc.wantErr)
			}
			if actor != (usecase.Actor{}) {
				t.Errorf("actor = %+v, want the zero value on failure", actor)
			}
			// The library error stays wrapped for logs but must not be a validation error,
			// or transport would answer 400 instead of 401.
			if errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("error = %v, must not map to invalid input", err)
			}
		})
	}
}

func TestParseAccessTokenRejectsWrongAlgorithm(t *testing.T) {
	cfg := testAuthConfig()
	issuer := newTestIssuer(t, cfg)

	cases := []struct {
		name   string
		method jwt.SigningMethod
	}{
		// HMAC with the same key but a different hash: the key callback would happily return
		// the secret, so only WithValidMethods stops it.
		{name: "HS512", method: jwt.SigningMethodHS512},
		{name: "HS384", method: jwt.SigningMethodHS384},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := accessClaims{
				Role: domain.RoleAdmin.String(),
				RegisteredClaims: jwt.RegisteredClaims{
					Subject:   "user-1",
					Issuer:    cfg.Issuer,
					Audience:  jwt.ClaimStrings{cfg.Audience},
					ExpiresAt: jwt.NewNumericDate(testIssuedAt.Add(cfg.AccessTokenTTL)),
				},
			}
			raw, err := jwt.NewWithClaims(tc.method, claims).SignedString([]byte(cfg.JWTSecret))
			if err != nil {
				t.Fatalf("SignedString() error = %v", err)
			}

			if _, err := issuer.ParseAccessToken(raw); !errors.Is(err, domain.ErrTokenInvalid) {
				t.Errorf("ParseAccessToken() error = %v, want %v", err, domain.ErrTokenInvalid)
			}
		})
	}
}

func TestParseAccessTokenLifetime(t *testing.T) {
	cfg := testAuthConfig()

	cases := []struct {
		name     string
		at       time.Time
		mutate   func(*accessClaims)
		wantErr  error
		wantPass bool
	}{
		{
			name: "inside the window",
			at:   testIssuedAt,
		},
		{
			name:    "expired",
			at:      testIssuedAt,
			mutate:  func(c *accessClaims) { c.ExpiresAt = jwt.NewNumericDate(testIssuedAt.Add(-time.Minute)) },
			wantErr: domain.ErrTokenExpired,
		},
		{
			// The exp claim has one-second resolution, so a peer whose clock runs a second
			// ahead must still be accepted.
			name:    "one second past expiry is inside the leeway",
			at:      testIssuedAt,
			mutate:  func(c *accessClaims) { c.ExpiresAt = jwt.NewNumericDate(testIssuedAt.Add(time.Second)) },
			wantErr: nil,
		},
		{
			name:    "well past expiry",
			at:      testIssuedAt,
			mutate:  func(c *accessClaims) { c.ExpiresAt = jwt.NewNumericDate(testIssuedAt.Add(-time.Hour)) },
			wantErr: domain.ErrTokenExpired,
		},
		{
			name:    "no expiry claim at all",
			at:      testIssuedAt,
			mutate:  func(c *accessClaims) { c.ExpiresAt = nil },
			wantErr: domain.ErrTokenInvalid,
		},
		{
			name:    "not yet valid",
			at:      testIssuedAt,
			mutate:  func(c *accessClaims) { c.NotBefore = jwt.NewNumericDate(testIssuedAt.Add(time.Hour)) },
			wantErr: domain.ErrTokenInvalid,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := claimsFor(t, cfg, tc.at, tc.mutate)
			// Every token here is verified at the moment it was minted.
			issuer := newTestIssuer(t, cfg)

			actor, err := issuer.ParseAccessToken(raw)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ParseAccessToken() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAccessToken() error = %v", err)
			}
			if actor.UserID != "user-1" {
				t.Errorf("actor.UserID = %q, want user-1", actor.UserID)
			}
		})
	}
}

func TestParseAccessTokenRejectsWrongAudienceOrIssuer(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*config.Auth, *accessClaims)
	}{
		{
			name:   "different audience",
			mutate: func(_ *config.Auth, c *accessClaims) { c.Audience = jwt.ClaimStrings{"some-other-api"} },
		},
		{
			name:   "no audience claim",
			mutate: func(_ *config.Auth, c *accessClaims) { c.Audience = nil },
		},
		{
			name:   "different issuer",
			mutate: func(_ *config.Auth, c *accessClaims) { c.Issuer = "evil-issuer" },
		},
		{
			name:   "no issuer claim",
			mutate: func(_ *config.Auth, c *accessClaims) { c.Issuer = "" },
		},
	}

	cfg := testAuthConfig()
	issuer := newTestIssuer(t, cfg)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := claimsFor(t, cfg, testIssuedAt, func(c *accessClaims) { tc.mutate(&cfg, c) })

			if _, err := issuer.ParseAccessToken(raw); !errors.Is(err, domain.ErrTokenInvalid) {
				t.Errorf("ParseAccessToken() error = %v, want %v", err, domain.ErrTokenInvalid)
			}
		})
	}
}

func TestParseAccessTokenRejectsANonSubjectToken(t *testing.T) {
	cfg := testAuthConfig()
	issuer := newTestIssuer(t, cfg)

	cases := []struct {
		name   string
		mutate func(*accessClaims)
	}{
		{name: "empty subject", mutate: func(c *accessClaims) { c.Subject = "" }},
		{name: "missing role", mutate: func(c *accessClaims) { c.Role = "" }},
		{name: "unknown role", mutate: func(c *accessClaims) { c.Role = "receptionist" }},
		{name: "role forged to a new value", mutate: func(c *accessClaims) { c.Role = "superuser" }},
		{name: "role is not a string", mutate: func(c *accessClaims) { c.Role = "true" }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := claimsFor(t, cfg, testIssuedAt, tc.mutate)

			actor, err := issuer.ParseAccessToken(raw)
			if !errors.Is(err, domain.ErrTokenInvalid) {
				t.Errorf("ParseAccessToken() error = %v, want %v", err, domain.ErrTokenInvalid)
			}
			if errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("error = %v, a forged token must not look like bad client input", err)
			}
			if actor != (usecase.Actor{}) {
				t.Errorf("actor = %+v, want the zero value", actor)
			}
		})
	}
}

// tamper re-signs nothing: it swaps the payload for one that escalates privileges and keeps
// the original signature, which is exactly what a naive verifier would accept.
func tamper(t *testing.T, cfg config.Auth) string {
	t.Helper()

	raw := claimsFor(t, cfg, testIssuedAt, nil)
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d segments, want 3", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	escalated := strings.Replace(string(payload), `"role":"doctor"`, `"role":"admin"`, 1)
	if escalated == string(payload) {
		t.Fatal("fixture payload does not carry the doctor role")
	}

	return parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(escalated)) + "." + parts[2]
}

func claimsForWithSecret(t *testing.T, cfg config.Auth, secret string) string {
	t.Helper()

	forged := cfg
	forged.JWTSecret = secret
	return claimsFor(t, forged, testIssuedAt, func(c *accessClaims) { c.Role = domain.RoleAdmin.String() })
}

func truncate(value string) string {
	if len(value) > 40 {
		return value[:40] + "…"
	}
	return value
}
