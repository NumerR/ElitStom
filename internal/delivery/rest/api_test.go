package rest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NumerR/ElitStom/internal/config"
	"github.com/NumerR/ElitStom/internal/delivery/rest/handler"
	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/platform/auth"
	"github.com/NumerR/ElitStom/internal/platform/logger"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// The suite wires the real router, middleware, handlers and use cases against in-memory ports.
// Only the account store and the password hash are stubbed: JWT issuance, token verification,
// RBAC and rotation run for real, so a wiring mistake surfaces here instead of in production.

const (
	testJWTSecret  = "0123456789abcdef0123456789abcdef"
	testPassword   = "correct horse battery staple"
	accessTokenTTL = 15 * time.Minute
	refreshTTL     = 72 * time.Hour
	testIssuedAt   = "2026-10-21T09:00:00Z"
)

var testClockAt = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)

func testAuthConfig() config.Auth {
	return config.Auth{
		JWTSecret:       testJWTSecret,
		AccessTokenTTL:  accessTokenTTL,
		RefreshTokenTTL: refreshTTL,
		Issuer:          "dentacore-test",
		Audience:        "dentacore-api",
		BcryptCost:      10,
	}
}

// memoryUsers is an account store, not a mirror of the SQL layer: it exists so the use cases
// have somewhere to read and write.
type memoryUsers struct {
	byID    map[string]domain.User
	byEmail map[string]string
	nextID  int
}

func newMemoryUsers() *memoryUsers {
	return &memoryUsers{byID: map[string]domain.User{}, byEmail: map[string]string{}}
}

func (m *memoryUsers) Create(_ context.Context, user domain.User) (domain.User, error) {
	if _, exists := m.byEmail[user.Email]; exists {
		return domain.User{}, fmt.Errorf("%w: account", domain.ErrAlreadyExists)
	}

	m.nextID++
	user.ID = fmt.Sprintf("00000000-0000-4000-8000-%012d", m.nextID)
	user.CreatedAt = testClockAt
	user.UpdatedAt = testClockAt

	m.byID[user.ID] = user
	m.byEmail[user.Email] = user.ID
	return user, nil
}

func (m *memoryUsers) ByID(_ context.Context, id string) (domain.User, error) {
	user, ok := m.byID[id]
	if !ok {
		return domain.User{}, fmt.Errorf("%w: account", domain.ErrNotFound)
	}
	return user, nil
}

func (m *memoryUsers) ByEmail(ctx context.Context, email string) (domain.User, error) {
	id, ok := m.byEmail[email]
	if !ok {
		return domain.User{}, fmt.Errorf("%w: account", domain.ErrNotFound)
	}
	return m.ByID(ctx, id)
}

func (m *memoryUsers) Update(_ context.Context, user domain.User) (domain.User, error) {
	stored, ok := m.byID[user.ID]
	if !ok {
		return domain.User{}, fmt.Errorf("%w: account", domain.ErrNotFound)
	}

	// Email is deliberately not written, mirroring the SQL statement: the identity column is
	// immutable, and a test double that let it change would hide a real privilege bug.
	stored.FullName = user.FullName
	stored.Phone = user.Phone
	stored.Role = user.Role
	stored.Status = user.Status
	stored.PasswordHash = user.PasswordHash
	stored.UpdatedAt = testClockAt.Add(time.Second)

	m.byID[user.ID] = stored
	return stored, nil
}

func (m *memoryUsers) List(_ context.Context, filter usecase.UserFilter) ([]domain.User, error) {
	listed := make([]domain.User, 0, len(m.byID))
	for _, user := range m.byID {
		if filter.Role.IsValid() && user.Role != filter.Role {
			continue
		}
		if filter.Status != "" && user.Status != filter.Status {
			continue
		}
		listed = append(listed, user)
	}

	// Ordered and paged exactly like the SQL statement, so a handler bug cannot hide behind a
	// double that ignores LIMIT and OFFSET.
	sort.Slice(listed, func(i, j int) bool {
		if !listed[i].CreatedAt.Equal(listed[j].CreatedAt) {
			return listed[i].CreatedAt.After(listed[j].CreatedAt)
		}
		return listed[i].ID > listed[j].ID
	})

	if filter.Offset >= len(listed) {
		return []domain.User{}, nil
	}
	listed = listed[filter.Offset:]
	if filter.Limit < len(listed) {
		listed = listed[:filter.Limit]
	}
	return listed, nil
}

// memorySessions stores digests only, mirroring the persistence contract: nothing here can hand
// back a plaintext secret.
type memorySessions struct {
	byHash map[string]domain.RefreshToken
}

func newMemorySessions() *memorySessions {
	return &memorySessions{byHash: map[string]domain.RefreshToken{}}
}

func (s *memorySessions) Insert(_ context.Context, token domain.RefreshToken) (domain.RefreshToken, error) {
	if _, exists := s.byHash[token.TokenHash]; exists {
		return domain.RefreshToken{}, fmt.Errorf("%w: session", domain.ErrAlreadyExists)
	}

	token.ID = fmt.Sprintf("00000000-0000-4000-9000-%012d", len(s.byHash)+1)
	s.byHash[token.TokenHash] = token
	return token, nil
}

func (s *memorySessions) ByHash(_ context.Context, hash string) (domain.RefreshToken, error) {
	token, ok := s.byHash[hash]
	if !ok {
		return domain.RefreshToken{}, fmt.Errorf("%w: session", domain.ErrNotFound)
	}
	return token, nil
}

func (s *memorySessions) Consume(_ context.Context, id string, at time.Time) (bool, error) {
	for hash, token := range s.byHash {
		if token.ID != id {
			continue
		}
		if token.UsedAt != nil {
			return false, nil
		}
		token.UsedAt = &at
		s.byHash[hash] = token
		return true, nil
	}
	return false, nil
}

func (s *memorySessions) RevokeFamily(_ context.Context, familyID string, at time.Time) error {
	for hash, token := range s.byHash {
		if token.FamilyID == familyID && token.RevokedAt == nil {
			token.RevokedAt = &at
			s.byHash[hash] = token
		}
	}
	return nil
}

func (s *memorySessions) RevokeAllForUser(_ context.Context, userID string, at time.Time) error {
	for hash, token := range s.byHash {
		if token.UserID == userID && token.RevokedAt == nil {
			token.RevokedAt = &at
			s.byHash[hash] = token
		}
	}
	return nil
}

func (s *memorySessions) isRevoked(hash string) bool {
	token, ok := s.byHash[hash]
	return ok && token.RevokedAt != nil
}

// stubHasher keeps the suite fast: bcrypt at a production cost would dominate every request.
// Hashing and verification are covered by the platform and usecase packages.
type stubHasher struct{}

func (stubHasher) Hash(password string) (string, error) {
	sum := sha256.Sum256([]byte("stub:" + password))
	return hex.EncodeToString(sum[:]), nil
}

func (h stubHasher) Verify(hash, password string) error {
	wanted, err := h.Hash(password)
	if err != nil {
		return err
	}
	if hash != wanted {
		return domain.ErrInvalidCredentials
	}
	return nil
}

func (stubHasher) DummyHash() string { return "dummy-hash-matches-nothing" }

type fixedClock struct{}

func (fixedClock) Now() time.Time { return testClockAt }

type readyPinger struct{}

func (readyPinger) Ping(context.Context) error { return nil }

type api struct {
	t            *testing.T
	server       http.Handler
	users        *memoryUsers
	sessions     *memorySessions
	appointments *memoryAppointments
	secrets      auth.SecretGenerator
	issuer       *auth.TokenIssuer

	adminID         string
	adminTokenValue string

	// bobAccess is carried between subtests: logout has to run before the cross-account probe.
	bobAccess string
}

func newAPI(t *testing.T) *api {
	t.Helper()
	return newAPIWithPinger(t, readyPinger{})
}

// newAPIWithPinger lets the readiness tests substitute a failing probe while keeping the rest of
// the wiring real.
func newAPIWithPinger(t *testing.T, db handler.Pinger) *api {
	t.Helper()

	cfg := testAuthConfig()

	issuer, err := auth.NewTokenIssuer(cfg, fixedClock{}.Now)
	require.NoError(t, err)

	hasher := stubHasher{}
	store := newMemoryUsers()
	sessions := newMemorySessions()

	authService, err := usecase.NewAuth(store, sessions, issuer, hasher, auth.NewSecretGenerator(), fixedClock{}, usecase.AuthConfig{
		AccessTokenTTL:  cfg.AccessTokenTTL,
		RefreshTokenTTL: cfg.RefreshTokenTTL,
	})
	require.NoError(t, err)

	userService, err := usecase.NewUsers(store, sessions, hasher, fixedClock{})
	require.NoError(t, err)

	appts := newMemoryAppointments(store)
	apptService, err := usecase.NewAppointments(appts, store, testSchedule(t), fixedClock{})
	require.NoError(t, err)

	return &api{
		t: t,
		server: NewRouter(Deps{
			DB:           db,
			Log:          logger.NewNop(),
			Tokens:       issuer,
			Auth:         authService,
			Users:        userService,
			Appointments: apptService,
		}),
		users:        store,
		sessions:     sessions,
		appointments: appts,
		secrets:      auth.NewSecretGenerator(),
		issuer:       issuer,
	}
}

// seedAccount stores an account with an arbitrary role and returns its id plus a verified
// access token. Minting through the real issuer is what makes the RBAC assertions meaningful.
func (a *api) seedAccount(role domain.Role, status domain.UserStatus, email string) (userID, accessToken string) {
	a.t.Helper()

	hash, err := stubHasher{}.Hash(testPassword)
	require.NoError(a.t, err)

	user, err := a.users.Create(context.Background(), domain.User{
		Email:        email,
		FullName:     "Seed Account",
		Phone:        "+15550101",
		Role:         role,
		Status:       status,
		PasswordHash: hash,
	})
	require.NoError(a.t, err)

	token, _, err := a.issuer.IssueAccessToken(user.ID, user.Role)
	require.NoError(a.t, err)
	return user.ID, token
}

// adminToken returns the same administrator for every call within one api: the account store is
// keyed by email, so re-seeding would collide with itself and report a phantom conflict.
func (a *api) adminToken() (userID, accessToken string) {
	a.t.Helper()

	if a.adminID != "" {
		return a.adminID, a.adminTokenValue
	}

	a.adminID, a.adminTokenValue = a.seedAccount(domain.RoleAdmin, domain.StatusActive, "admin@example.com")
	return a.adminID, a.adminTokenValue
}

// request drives the router, so middleware ordering and the route guards are part of what every
// assertion below covers.
func (a *api) request(method, path, body, bearer string) *httptest.ResponseRecorder {
	a.t.Helper()

	header := map[string]string{}
	if body != "" {
		header["Content-Type"] = "application/json"
	}
	if bearer != "" {
		header["Authorization"] = "Bearer " + bearer
	}
	return a.raw(method, path, header, body)
}

func (a *api) raw(method, path string, header map[string]string, body string) *httptest.ResponseRecorder {
	a.t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for key, value := range header {
		req.Header.Set(key, value)
	}

	rec := httptest.NewRecorder()
	a.server.ServeHTTP(rec, req)
	return rec
}

func jsonBody(t *testing.T, values map[string]any) string {
	t.Helper()

	encoded, err := json.Marshal(values)
	require.NoError(t, err)
	return string(encoded)
}

func decodeMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var payload map[string]any
	require.NoErrorf(t, json.Unmarshal(rec.Body.Bytes(), &payload), "body is not a JSON object: %s", rec.Body.String())
	return payload
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	failure, ok := decodeMap(t, rec)["error"].(map[string]any)
	require.Truef(t, ok, "body has no error object: %s", rec.Body.String())

	code, ok := failure["code"].(string)
	require.Truef(t, ok, "error object has no code: %s", rec.Body.String())
	return code
}

// issuedTokens pulls the session out of an auth response.
func issuedTokens(t *testing.T, rec *httptest.ResponseRecorder) (accessToken, refreshToken string) {
	t.Helper()

	session, ok := decodeMap(t, rec)["tokens"].(map[string]any)
	require.Truef(t, ok, "body has no tokens object: %s", rec.Body.String())

	accessToken, _ = session["access_token"].(string)
	refreshToken, _ = session["refresh_token"].(string)
	require.NotEmpty(t, accessToken, "access token is missing")
	require.NotEmpty(t, refreshToken, "refresh token is missing")
	return accessToken, refreshToken
}

// signUp registers a patient through the public endpoint and returns its session.
func (a *api) signUp(email string) (accessToken, refreshToken string) {
	a.t.Helper()

	rec := a.request(http.MethodPost, "/api/v1/auth/register", jsonBody(a.t, map[string]any{
		"email":     email,
		"full_name": "Ada Lovelace",
		"phone":     "+15550100",
		"password":  testPassword,
	}), "")
	require.Equalf(a.t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())

	return issuedTokens(a.t, rec)
}

func TestAuthHappyPath(t *testing.T) {
	a := newAPI(t)

	access, refresh := a.signUp("ada@example.com")

	t.Run("the issued access token authenticates", func(t *testing.T) {
		rec := a.request(http.MethodGet, "/api/v1/users/me", "", access)
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

		profile := decodeMap(t, rec)
		assert.Equal(t, "ada@example.com", profile["email"])
		assert.Equal(t, "patient", profile["role"])
		assert.Equal(t, "active", profile["status"])
		assert.NotContains(t, profile, "password_hash")
	})

	t.Run("login returns a new session", func(t *testing.T) {
		rec := a.request(http.MethodPost, "/api/v1/auth/login",
			jsonBody(t, map[string]any{"email": "ada@example.com", "password": testPassword}), "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

		reissued, _ := issuedTokens(t, rec)
		assert.NotEqual(t, access, reissued, "two access tokens are byte-identical, so jti is not random")
	})

	t.Run("refresh rotates the secret", func(t *testing.T) {
		rec := a.request(http.MethodPost, "/api/v1/auth/refresh",
			jsonBody(t, map[string]any{"refresh_token": refresh}), "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

		_, rotated := issuedTokens(t, rec)
		assert.NotEqual(t, refresh, rotated)

		// The consumed secret must not be replayable: reuse revokes the whole family.
		replay := a.request(http.MethodPost, "/api/v1/auth/refresh",
			jsonBody(t, map[string]any{"refresh_token": refresh}), "")
		assert.Equal(t, http.StatusUnauthorized, replay.Code)
		assert.Equal(t, "unauthorized", errorCode(t, replay))

		fresh := a.request(http.MethodPost, "/api/v1/auth/refresh",
			jsonBody(t, map[string]any{"refresh_token": rotated}), "")
		assert.Equalf(t, http.StatusUnauthorized, fresh.Code,
			"reuse of a consumed token must revoke the family, body: %s", fresh.Body.String())
	})

	t.Run("logout revokes the presented session", func(t *testing.T) {
		bobAccess, secret := a.signUp("bob@example.com")
		a.bobAccess = bobAccess

		rec := a.request(http.MethodPost, "/api/v1/auth/logout",
			jsonBody(t, map[string]any{"refresh_token": secret}), bobAccess)
		require.Equal(t, http.StatusNoContent, rec.Code, "body: %s", rec.Body.String())
		assert.Empty(t, rec.Body.String())

		assert.True(t, a.sessions.isRevoked(a.secrets.HashToken(secret)))

		after := a.request(http.MethodPost, "/api/v1/auth/refresh",
			jsonBody(t, map[string]any{"refresh_token": secret}), "")
		assert.Equal(t, http.StatusUnauthorized, after.Code)

		// The access token is not revoked with it, which is why logout also drops the session.
		assert.Equal(t, http.StatusOK, a.request(http.MethodGet, "/api/v1/users/me", "", bobAccess).Code)
	})

	t.Run("a session cannot be logged out by another account", func(t *testing.T) {
		_, otherSecret := a.signUp("carol@example.com")

		rec := a.request(http.MethodPost, "/api/v1/auth/logout",
			jsonBody(t, map[string]any{"refresh_token": otherSecret}), a.bobAccess)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestAuthenticationFailures(t *testing.T) {
	a := newAPI(t)
	access, _ := a.signUp("ada@example.com")

	cases := []struct {
		name   string
		bearer string
		header map[string]string
	}{
		{name: "no header", header: map[string]string{}},
		{name: "empty header", header: map[string]string{"Authorization": ""}},
		{name: "scheme only", header: map[string]string{"Authorization": "Bearer"}},
		{name: "blank credential", header: map[string]string{"Authorization": "Bearer   "}},
		{name: "basic scheme", header: map[string]string{"Authorization": "Basic dXNlcjpwYXNz"}},
		// A bare credential has no scheme, so it is refused before the token is even parsed.
		{name: "bare token", header: map[string]string{"Authorization": access}},
		{name: "foreign signature", bearer: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJmb3JnZWQifQ.not-a-signature"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.bearer != "" {
				tc.header = map[string]string{"Authorization": "Bearer " + tc.bearer}
			}

			rec := a.raw(http.MethodGet, "/api/v1/users/me", tc.header, "")
			assert.Equal(t, http.StatusUnauthorized, rec.Code, "body: %s", rec.Body.String())
			assert.Equal(t, "unauthorized", errorCode(t, rec))
		})
	}

	t.Run("lowercase bearer scheme is accepted", func(t *testing.T) {
		rec := a.raw(http.MethodGet, "/api/v1/users/me",
			map[string]string{"Authorization": "bearer " + access}, "")
		assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	})

	t.Run("a token for a deleted account is rejected", func(t *testing.T) {
		const deletedAccount = "00000000-0000-4000-8fff-ffffffffffff"
		orphan, _, err := a.issuer.IssueAccessToken(deletedAccount, domain.RolePatient)
		require.NoError(t, err)

		rec := a.request(http.MethodGet, "/api/v1/users/me", "", orphan)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Equal(t, "unauthorized", errorCode(t, rec))
	})
}

func TestRBAC(t *testing.T) {
	a := newAPI(t)

	adminID, admin := a.adminToken()
	doctorID, doctor := a.seedAccount(domain.RoleDoctor, domain.StatusActive, "doc@example.com")
	patientID, patient := a.seedAccount(domain.RolePatient, domain.StatusActive, "pat@example.com")

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		token      string
		wantStatus int
	}{
		{name: "admin lists the directory", method: http.MethodGet, path: "/api/v1/admin/users", token: admin, wantStatus: http.StatusOK},
		{name: "admin creates an account", method: http.MethodPost, path: "/api/v1/admin/users",
			body:       jsonBody(t, map[string]any{"email": "new@example.com", "full_name": "New Doctor", "phone": "+15550103", "role": "doctor", "password": testPassword}),
			token:      admin,
			wantStatus: http.StatusCreated},
		{name: "admin changes a role", method: http.MethodPatch, path: "/api/v1/admin/users/" + patientID + "/role",
			body: jsonBody(t, map[string]any{"role": "doctor"}), token: admin, wantStatus: http.StatusOK},
		{name: "admin disables an account", method: http.MethodPatch, path: "/api/v1/admin/users/" + doctorID + "/status",
			body: jsonBody(t, map[string]any{"status": "disabled"}), token: admin, wantStatus: http.StatusOK},

		{name: "doctor cannot list", method: http.MethodGet, path: "/api/v1/admin/users", token: doctor, wantStatus: http.StatusForbidden},
		{name: "doctor cannot create", method: http.MethodPost, path: "/api/v1/admin/users",
			body:  jsonBody(t, map[string]any{"email": "x@example.com", "full_name": "X", "phone": "+15550104", "role": "doctor", "password": testPassword}),
			token: doctor, wantStatus: http.StatusForbidden},
		{name: "doctor cannot change roles", method: http.MethodPatch, path: "/api/v1/admin/users/" + adminID + "/role",
			body: jsonBody(t, map[string]any{"role": "doctor"}), token: doctor, wantStatus: http.StatusForbidden},
		{name: "patient cannot list", method: http.MethodGet, path: "/api/v1/admin/users", token: patient, wantStatus: http.StatusForbidden},
		{name: "patient cannot self-promote", method: http.MethodPatch, path: "/api/v1/admin/users/" + patientID + "/role",
			body: jsonBody(t, map[string]any{"role": "admin"}), token: patient, wantStatus: http.StatusForbidden},
		{name: "anonymous cannot list", method: http.MethodGet, path: "/api/v1/admin/users", wantStatus: http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.request(tc.method, tc.path, tc.body, tc.token)
			require.Equalf(t, tc.wantStatus, rec.Code, "body: %s", rec.Body.String())

			if tc.wantStatus == http.StatusForbidden {
				assert.Equal(t, "forbidden", errorCode(t, rec))
			}
		})
	}

	t.Run("an admin cannot demote itself", func(t *testing.T) {
		rec := a.request(http.MethodPatch, "/api/v1/admin/users/"+adminID+"/role",
			jsonBody(t, map[string]any{"role": "patient"}), admin)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	t.Run("an admin cannot disable itself", func(t *testing.T) {
		rec := a.request(http.MethodPatch, "/api/v1/admin/users/"+adminID+"/status",
			jsonBody(t, map[string]any{"status": "disabled"}), admin)
		assert.Equal(t, http.StatusConflict, rec.Code)
	})

	// Role travels in the signed access token, so revocation reaches refresh tokens immediately
	// but an already-issued access token stays valid for the rest of its TTL. This pins that
	// window down as a known, bounded trade-off rather than an accident.
	t.Run("demotion leaves an issued access token valid until it expires", func(t *testing.T) {
		otherAdminID, otherAdmin := a.seedAccount(domain.RoleAdmin, domain.StatusActive, "admin2@example.com")

		rec := a.request(http.MethodPatch, "/api/v1/admin/users/"+otherAdminID+"/role",
			jsonBody(t, map[string]any{"role": "doctor"}), admin)
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

		stillAdmin := a.request(http.MethodGet, "/api/v1/admin/users", "", otherAdmin)
		assert.Equal(t, http.StatusOK, stillAdmin.Code,
			"the stale token still authorizes for up to %s; a new session would not", accessTokenTTL)
	})
}

func TestRequestValidationAtTheBoundary(t *testing.T) {
	a := newAPI(t)
	_, admin := a.adminToken()
	// ada exists so the wrong-password case below fails on the credential, not on the lookup.
	a.signUp("ada@example.com")

	cases := []struct {
		name   string
		method string
		path   string
		header map[string]string
		body   string
		token  string

		wantStatus int
		wantCode   string
	}{
		{
			name: "missing content type", method: http.MethodPost, path: "/api/v1/auth/register",
			header:     map[string]string{},
			body:       `{"email":"a@b.co","full_name":"A B","phone":"+15550100","password":"` + testPassword + `"}`,
			wantStatus: http.StatusBadRequest, wantCode: "invalid_input",
		},
		{
			name: "wrong content type", method: http.MethodPost, path: "/api/v1/auth/register",
			header:     map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
			body:       "email=a@b.co",
			wantStatus: http.StatusBadRequest, wantCode: "invalid_input",
		},
		{
			name: "empty body", method: http.MethodPost, path: "/api/v1/auth/register",
			header:     map[string]string{"Content-Type": "application/json"},
			body:       "",
			wantStatus: http.StatusBadRequest, wantCode: "invalid_input",
		},
		{
			name: "malformed json", method: http.MethodPost, path: "/api/v1/auth/register",
			body: `{"email": "unterminated`,

			wantStatus: http.StatusBadRequest, wantCode: "invalid_input",
		},
		{
			name: "trailing second document", method: http.MethodPost, path: "/api/v1/auth/register",
			body:       `{"email":"a@b.co","full_name":"A B","phone":"+15550100","password":"` + testPassword + `"} {"email":"c@d.co"}`,
			wantStatus: http.StatusBadRequest, wantCode: "invalid_input",
		},
		{
			// Self-promotion has to fail rather than be quietly dropped by the decoder.
			name: "unknown field is rejected not ignored", method: http.MethodPost, path: "/api/v1/auth/register",
			body:       `{"email":"a@b.co","full_name":"A B","phone":"+15550100","password":"` + testPassword + `","role":"admin"}`,
			wantStatus: http.StatusBadRequest, wantCode: "invalid_input",
		},
		{
			name: "wrong field type", method: http.MethodPost, path: "/api/v1/auth/register",
			body:       `{"email":12345,"full_name":"A B","phone":"+15550100","password":"` + testPassword + `"}`,
			wantStatus: http.StatusBadRequest, wantCode: "invalid_input",
		},
		{
			name: "short password", method: http.MethodPost, path: "/api/v1/auth/register",
			body: jsonBody(t, map[string]any{"email": "a@b.co", "full_name": "A B", "phone": "+15550100", "password": "short"}),

			wantStatus: http.StatusBadRequest, wantCode: "invalid_input",
		},
		{
			name: "unknown role filter", method: http.MethodGet, path: "/api/v1/admin/users?role=receptionist",
			token:      admin,
			wantStatus: http.StatusBadRequest, wantCode: "invalid_input",
		},
		{
			name: "unknown status filter", method: http.MethodGet, path: "/api/v1/admin/users?status=pending",
			token:      admin,
			wantStatus: http.StatusBadRequest, wantCode: "invalid_input",
		},
		{
			name: "unknown target account", method: http.MethodPatch, path: "/api/v1/admin/users/nonsense/role",
			body:       jsonBody(t, map[string]any{"role": "doctor"}),
			token:      admin,
			wantStatus: http.StatusNotFound, wantCode: "not_found",
		},
		{
			name: "unknown refresh secret", method: http.MethodPost, path: "/api/v1/auth/refresh",
			body:       jsonBody(t, map[string]any{"refresh_token": "an-opaque-secret-nobody-issued"}),
			wantStatus: http.StatusUnauthorized, wantCode: "unauthorized",
		},
		{
			name: "wrong password", method: http.MethodPost, path: "/api/v1/auth/login",
			body:       jsonBody(t, map[string]any{"email": "ada@example.com", "password": "a wrong passphrase totally"}),
			wantStatus: http.StatusUnauthorized, wantCode: "unauthorized",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			header := tc.header
			if header == nil {
				header = map[string]string{}
				if tc.body != "" {
					header["Content-Type"] = "application/json"
				}
			}
			if tc.token != "" {
				header["Authorization"] = "Bearer " + tc.token
			}

			rec := a.raw(tc.method, tc.path, header, tc.body)
			require.Equalf(t, tc.wantStatus, rec.Code, "body: %s", rec.Body.String())
			assert.Equal(t, tc.wantCode, errorCode(t, rec))
		})
	}

	t.Run("a disabled account cannot log in", func(t *testing.T) {
		targetID, _ := a.seedAccount(domain.RoleDoctor, domain.StatusActive, "sleepy@example.com")

		rec := a.request(http.MethodPatch, "/api/v1/admin/users/"+targetID+"/status",
			jsonBody(t, map[string]any{"status": "disabled"}), admin)
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

		login := a.request(http.MethodPost, "/api/v1/auth/login",
			jsonBody(t, map[string]any{"email": "sleepy@example.com", "password": testPassword}), "")
		assert.Equal(t, http.StatusForbidden, login.Code)
		assert.Equal(t, "account_disabled", errorCode(t, login))
	})

	t.Run("login failure is indistinguishable from an unknown account", func(t *testing.T) {
		known := a.request(http.MethodPost, "/api/v1/auth/login",
			jsonBody(t, map[string]any{"email": "ada@example.com", "password": "a wrong passphrase totally"}), "")
		unknown := a.request(http.MethodPost, "/api/v1/auth/login",
			jsonBody(t, map[string]any{"email": "never-signed-up@example.com", "password": "a wrong passphrase totally"}), "")

		assert.Equal(t, known.Code, unknown.Code, "the status alone reveals whether the account exists")
		assert.JSONEq(t, known.Body.String(), unknown.Body.String(), "the bodies differ, so accounts are enumerable")
	})
}

func TestProfileUpdate(t *testing.T) {
	a := newAPI(t)
	access, _ := a.signUp("ada@example.com")

	rec := a.request(http.MethodPatch, "/api/v1/users/me",
		jsonBody(t, map[string]any{"full_name": "Ada King", "phone": "+15550999"}), access)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	profile := decodeMap(t, rec)
	assert.Equal(t, "Ada King", profile["full_name"])
	assert.Equal(t, "+15550999", profile["phone"])
	assert.Equal(t, "ada@example.com", profile["email"], "email is the identity and must not move")

	t.Run("the update only ever touches the caller's own account", func(t *testing.T) {
		otherID, otherAccess := a.seedAccount(domain.RolePatient, domain.StatusActive, "other@example.com")

		rec := a.request(http.MethodPatch, "/api/v1/users/me",
			jsonBody(t, map[string]any{"full_name": "Mallory", "phone": "+15550888"}), otherAccess)
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

		// The subject comes from the token, so ada's profile is the only one that moved.
		ada := a.request(http.MethodGet, "/api/v1/users/me", "", access)
		require.Equal(t, http.StatusOK, ada.Code)
		assert.Equal(t, "Ada King", decodeMap(t, ada)["full_name"])

		assert.NotEqual(t, "Mallory", decodeMap(t, ada)["full_name"])
		assert.NotEmpty(t, otherID)
	})

	t.Run("an invalid name is rejected", func(t *testing.T) {
		rec := a.request(http.MethodPatch, "/api/v1/users/me",
			jsonBody(t, map[string]any{"full_name": "A", "phone": "+15550100"}), access)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestAdminDirectoryPaging(t *testing.T) {
	a := newAPI(t)
	_, admin := a.adminToken()

	for i := range 3 {
		a.seedAccount(domain.RoleDoctor, domain.StatusActive, fmt.Sprintf("doc%d@example.com", i))
	}

	rec := a.request(http.MethodGet, "/api/v1/admin/users?limit=2&offset=0", "", admin)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	payload := decodeMap(t, rec)
	listed, ok := payload["users"].([]any)
	require.Truef(t, ok, "users is not an array: %s", rec.Body.String())
	assert.Len(t, listed, 2)
	assert.EqualValues(t, 2, payload["limit"])
	assert.EqualValues(t, 0, payload["offset"])

	t.Run("an empty page is an array not null", func(t *testing.T) {
		rec := a.request(http.MethodGet, "/api/v1/admin/users?offset=1000", "", admin)
		require.Equal(t, http.StatusOK, rec.Code)

		// json.Unmarshal turns `null` into a nil interface and `[]` into an empty slice, so this
		// distinguishes the two shapes the client has to handle.
		listed, ok := decodeMap(t, rec)["users"].([]any)
		require.Truef(t, ok, "users must be [] rather than null: %s", rec.Body.String())
		assert.Empty(t, listed)
	})
}

func TestBodySizeLimit(t *testing.T) {
	a := newAPI(t)

	huge := `{"email":"` + strings.Repeat("a", 400_000) + `@example.com","full_name":"A B","phone":"+15550100","password":"` + testPassword + `"}`

	rec := a.request(http.MethodPost, "/api/v1/auth/register", huge, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", truncate(rec.Body.String()))
	assert.Equal(t, "invalid_input", errorCode(t, rec))
}

func TestUnknownRoutesAndMethods(t *testing.T) {
	a := newAPI(t)

	_, admin := a.adminToken()

	rec := a.request(http.MethodGet, "/api/v1/admin/patients", "", admin)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "not_found", errorCode(t, rec))

	rec = a.request(http.MethodDelete, "/api/v1/admin/users", "", admin)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, "method_not_allowed", errorCode(t, rec))

	t.Run("a protected route is never reachable by GET on the collection", func(t *testing.T) {
		anonymous := a.raw(http.MethodPost, "/api/v1/admin/users", map[string]string{"Content-Type": "application/json"},
			jsonBody(t, map[string]any{"email": "e@x.co", "full_name": "E X", "phone": "+15550105", "role": "admin", "password": testPassword}))
		assert.Equal(t, http.StatusUnauthorized, anonymous.Code)
	})
}

func TestRouterRejectsIncompleteDependencies(t *testing.T) {
	cases := []struct {
		name      string
		deps      Deps
		wantPanic string
	}{
		{
			name:      "nothing supplied",
			deps:      Deps{},
			wantPanic: "rest: NewRouter is missing database pinger, logger, token issuer, auth service, users service, appointments service",
		},
		{
			name:      "no collaborators",
			deps:      Deps{DB: readyPinger{}, Log: logger.NewNop()},
			wantPanic: "rest: NewRouter is missing token issuer, auth service, users service, appointments service",
		},
		{
			name:      "no users service",
			deps:      Deps{DB: readyPinger{}, Log: logger.NewNop(), Tokens: fixedIssuer(t)},
			wantPanic: "rest: NewRouter is missing auth service, users service, appointments service",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.PanicsWithValuef(t, tc.wantPanic, func() { NewRouter(tc.deps) },
				"a router with missing collaborators must fail at startup, not on the first request")
		})
	}
}

// helpers

// fixedIssuer builds an issuer for the dependency checks, which only care that the field is set.
func fixedIssuer(t *testing.T) *auth.TokenIssuer {
	t.Helper()

	issuer, err := auth.NewTokenIssuer(testAuthConfig(), fixedClock{}.Now)
	require.NoError(t, err)
	return issuer
}

func truncate(body string) string {
	if len(body) > 200 {
		return body[:200] + "..."
	}
	return body
}
