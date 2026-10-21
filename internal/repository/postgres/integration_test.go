//go:build integration

package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/repository/postgres"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// These tests need a real server because they assert what a fake cannot reproduce: constraint
// violations, the atomic compare-and-set under contention, and pgx's own scanning.
//
//	docker compose up -d postgres
//	DENTACORE_TEST_DATABASE_URL=postgres://dentacore:dentacore@localhost:5432/dentacore?sslmode=disable \
//		go test -race -tags integration ./internal/repository/postgres/
const envIntegrationDSN = "DENTACORE_TEST_DATABASE_URL"

var (
	uuidText   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	migrations = []string{"0001_users.up.sql", "0002_refresh_tokens.up.sql"}
)

type harness struct {
	pool   *pgxpool.Pool
	users  *postgres.Users
	toks   *postgres.RefreshTokens
	ctx    context.Context
	suffix string
}

func setupHarness(t *testing.T) *harness {
	t.Helper()

	dsn := os.Getenv(envIntegrationDSN)
	if dsn == "" {
		t.Skipf("%s is not set; skipping the PostgreSQL integration suite", envIntegrationDSN)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	// Each run gets its own schema, so the suite is repeatable against a shared developer
	// database and the row counts asserted below stay exact.
	schema := "dentacore_it_" + randomHex(t, 4)
	provision(t, ctx, dsn, schema)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		conn, err := pgx.Connect(cleanupCtx, dsn)
		if err != nil {
			t.Errorf("connect to drop schema %s: %v", schema, err)
			return
		}
		defer func() {
			if err := conn.Close(cleanupCtx); err != nil {
				t.Errorf("close cleanup connection: %v", err)
			}
		}()
		if _, err := conn.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})

	poolCfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err, "parse dsn")
	// Set before the pool exists: RuntimeParams are applied per connection at connect time,
	// so changing them after Ping would leave the first connections on the wrong schema.
	poolCfg.ConnConfig.RuntimeParams["search_path"] = schema
	poolCfg.MaxConns = 8

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	require.NoError(t, err, "create pool")
	require.NoError(t, pool.Ping(ctx), "ping")
	t.Cleanup(pool.Close)

	return &harness{
		pool:   pool,
		users:  postgres.NewUsers(postgres.NewExecutor(pool)),
		toks:   postgres.NewRefreshTokens(postgres.NewExecutor(pool)),
		ctx:    ctx,
		suffix: randomHex(t, 4),
	}
}

func provision(t *testing.T, ctx context.Context, dsn, schema string) {
	t.Helper()

	conn, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err, "connect for provisioning")
	defer func() {
		if err := conn.Close(ctx); err != nil {
			t.Errorf("close provisioning connection: %v", err)
		}
	}()

	for _, statement := range []string{"CREATE SCHEMA " + schema, "SET search_path TO " + schema} {
		_, err := conn.Exec(ctx, statement)
		require.NoErrorf(t, err, "provision: %q", statement)
	}

	for _, name := range migrations {
		body, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", name))
		require.NoErrorf(t, err, "read migration %s", name)
		_, err = conn.Exec(ctx, string(body))
		require.NoErrorf(t, err, "apply migration %s", name)
	}
}

func randomHex(t *testing.T, n int) string {
	t.Helper()

	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("read random bytes: %v", err)
	}
	return hex.EncodeToString(buf)
}

// digest is a stand-in for the SHA-256 of a refresh secret: 64 lower-case hex characters, which
// is the shape the column CHECK demands.
func (h *harness) digest(t *testing.T) string {
	t.Helper()
	return randomHex(t, 32)
}

// newAccount inserts a user the way the use case would: normalised fields and a real bcrypt
// envelope, so the hash-shape CHECK passes.
func (h *harness) newAccount(t *testing.T, role domain.Role) domain.User {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte("a-long-enough-passphrase"), bcrypt.MinCost)
	require.NoError(t, err, "hash password")

	user, err := h.users.Create(h.ctx, domain.User{
		Email:        fmt.Sprintf("it-%s-%s@example.com", role, h.suffix),
		FullName:     "Integration Patient",
		Phone:        "+15550100",
		Role:         role,
		Status:       domain.StatusActive,
		PasswordHash: string(hash),
	})
	require.NoErrorf(t, err, "create %s account", role)

	if !uuidText.MatchString(user.ID) {
		t.Fatalf("server-generated id %q is not a canonical uuid", user.ID)
	}
	return user
}

func (h *harness) insertToken(t *testing.T, userID, familyID string, createdAt time.Time) domain.RefreshToken {
	t.Helper()

	token, err := h.toks.Insert(h.ctx, domain.RefreshToken{
		UserID:    userID,
		FamilyID:  familyID,
		TokenHash: h.digest(t),
		ExpiresAt: createdAt.Add(24 * time.Hour),
		CreatedAt: createdAt,
	})
	require.NoError(t, err, "insert refresh token")
	return token
}

func TestIntegrationCreateReadRoundTrip(t *testing.T) {
	h := setupHarness(t)

	created := h.newAccount(t, domain.RoleDoctor)

	byID, err := h.users.ByID(h.ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created, byID, "the row read back must match what was written")

	byEmail, err := h.users.ByEmail(h.ctx, created.Email)
	require.NoError(t, err)
	require.Equal(t, created, byEmail)

	if byID.PasswordHash == "" {
		t.Error("the repository dropped the hash; projection to a safe payload is the use case's job")
	}
}

func TestIntegrationUnknownAccountIsNotFound(t *testing.T) {
	h := setupHarness(t)

	_, err := h.users.ByEmail(h.ctx, "nobody-"+h.suffix+"@example.com")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ByEmail() error = %v, want %v", err, domain.ErrNotFound)
	}
}

func TestIntegrationDuplicateEmailIsAlreadyExists(t *testing.T) {
	h := setupHarness(t)

	first := h.newAccount(t, domain.RolePatient)

	_, err := h.users.Create(h.ctx, domain.User{
		Email:        first.Email,
		FullName:     "Second Person",
		Phone:        "+15550199",
		Role:         domain.RoleAdmin,
		Status:       domain.StatusActive,
		PasswordHash: "$2b$04$" + randomHex(t, 20),
	})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("Create() error = %v, want %v", err, domain.ErrAlreadyExists)
	}
}

// TestIntegrationDatabaseEnforcesNormalisation covers the defence-in-depth half of the email and
// hash rules: even a caller that bypasses the domain cannot store a bad row.
func TestIntegrationDatabaseEnforcesNormalisation(t *testing.T) {
	h := setupHarness(t)

	cases := []struct {
		name  string
		email string
		hash  string
	}{
		{name: "upper-case email", email: "Ada@Example.COM", hash: "$2b$04$" + randomHex(t, 20)},
		{name: "padded email", email: " ada+" + h.suffix + "@example.com ", hash: "$2b$04$" + randomHex(t, 20)},
		{name: "plaintext password", email: "plain-" + h.suffix + "@example.com", hash: "hunter2hunter2"},
		{name: "unknown role", email: "role-" + h.suffix + "@example.com", hash: "$2b$04$" + randomHex(t, 20)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			role := domain.RolePatient
			if tc.name == "unknown role" {
				role = domain.Role("receptionist")
			}

			_, err := h.users.Create(h.ctx, domain.User{
				Email:        tc.email,
				FullName:     "Normalisation Test",
				Phone:        "+15550100",
				Role:         role,
				Status:       domain.StatusActive,
				PasswordHash: tc.hash,
			})
			if err == nil {
				t.Fatal("Create() = nil error, want the CHECK constraint to reject the row")
			}
			if errors.Is(err, domain.ErrAlreadyExists) || errors.Is(err, domain.ErrNotFound) {
				t.Errorf("Create() error = %v, want a constraint fault, not a mapped sentinel", err)
			}
		})
	}
}

func TestIntegrationUpdatePersistsMutableFields(t *testing.T) {
	h := setupHarness(t)

	user := h.newAccount(t, domain.RolePatient)

	require.NoError(t, user.ChangeRole(domain.RoleDoctor))
	user.Disable()
	user.Phone = "+15550177"

	updated, err := h.users.Update(h.ctx, user)
	require.NoError(t, err)

	require.Equal(t, domain.RoleDoctor, updated.Role)
	require.Equal(t, domain.StatusDisabled, updated.Status)
	require.Equal(t, "+15550177", updated.Phone)
	if !updated.UpdatedAt.After(updated.CreatedAt) {
		t.Errorf("UpdatedAt = %v, CreatedAt = %v, want the update to bump the timestamp",
			updated.UpdatedAt, updated.CreatedAt)
	}

	// Email is absent from the UPDATE statement by design, so a rewritten identity is ignored.
	_, err = h.users.Update(h.ctx, domain.User{
		ID: updated.ID, Email: "attacker@example.com", FullName: updated.FullName,
		Phone: updated.Phone, Role: updated.Role, Status: updated.Status,
		PasswordHash: updated.PasswordHash,
	})
	require.NoError(t, err)

	reloaded, err := h.users.ByID(h.ctx, updated.ID)
	require.NoError(t, err)
	require.Equal(t, updated.Email, reloaded.Email, "email is the account identity and must not be mutable")
}

func TestIntegrationUpdateUnknownAccountIsNotFound(t *testing.T) {
	h := setupHarness(t)

	_, err := h.users.Update(h.ctx, domain.User{
		ID:           "00000000-0000-4000-8000-000000000000",
		FullName:     "Ghost",
		Phone:        "+15550100",
		Role:         domain.RolePatient,
		Status:       domain.StatusActive,
		PasswordHash: "$2b$04$" + randomHex(t, 20),
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Update() error = %v, want %v", err, domain.ErrNotFound)
	}
}

func TestIntegrationListFiltersAndPages(t *testing.T) {
	h := setupHarness(t)

	for range 3 {
		h.newAccount(t, domain.RolePatient)
	}
	h.newAccount(t, domain.RoleDoctor)

	cases := []struct {
		name   string
		filter usecase.UserFilter
		want   int
	}{
		{name: "everyone", filter: usecase.UserFilter{Limit: 50}, want: 4},
		{name: "patients", filter: usecase.UserFilter{Role: domain.RolePatient, Limit: 50}, want: 3},
		{name: "doctors", filter: usecase.UserFilter{Role: domain.RoleDoctor, Limit: 50}, want: 1},
		{name: "admins", filter: usecase.UserFilter{Role: domain.RoleAdmin, Limit: 50}, want: 0},
		{name: "active", filter: usecase.UserFilter{Status: domain.StatusActive, Limit: 50}, want: 4},
		{name: "disabled", filter: usecase.UserFilter{Status: domain.StatusDisabled, Limit: 50}, want: 0},
		{name: "first page", filter: usecase.UserFilter{Limit: 2}, want: 2},
		{name: "second page", filter: usecase.UserFilter{Limit: 2, Offset: 2}, want: 2},
		{name: "past the end", filter: usecase.UserFilter{Limit: 2, Offset: 4}, want: 0},
	}

	seen := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			listed, err := h.users.List(h.ctx, tc.filter)
			require.NoError(t, err)
			require.Len(t, listed, tc.want)
			if listed == nil {
				t.Fatal("List() = nil, want an allocated empty page so JSON renders [] not null")
			}

			for _, user := range listed {
				if tc.filter.Role.IsValid() && user.Role != tc.filter.Role {
					t.Errorf("account %s has role %q, want %q", user.ID, user.Role, tc.filter.Role)
				}
				seen[user.ID] = true
			}
		})
	}

	// The two pages have to partition the table; this catches paging that repeats or skips a row.
	if len(seen) != 4 {
		t.Errorf("paging covered %d distinct accounts, want all 4", len(seen))
	}

	t.Run("newest first", func(t *testing.T) {
		listed, err := h.users.List(h.ctx, usecase.UserFilter{Limit: 50})
		require.NoError(t, err)
		for i := 1; i < len(listed); i++ {
			if listed[i-1].CreatedAt.Before(listed[i].CreatedAt) {
				t.Fatalf("row %d is older than row %d, want created_at DESC", i-1, i)
			}
		}
	})
}

func TestIntegrationRefreshTokenRoundTrip(t *testing.T) {
	h := setupHarness(t)

	user := h.newAccount(t, domain.RolePatient)
	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	token := h.insertToken(t, user.ID, randomHex(t, 16), createdAt)

	require.Equal(t, user.ID, token.UserID)
	require.True(t, token.UsedAt == nil, "UsedAt = %v, want NULL on a fresh token", token.UsedAt)
	require.True(t, token.RevokedAt == nil, "RevokedAt = %v, want NULL on a fresh token", token.RevokedAt)

	// char(64) blank-pads shorter values in some drivers; a digest read back differently could
	// never match the next lookup and would silently log the session out.
	reloaded, err := h.toks.ByHash(h.ctx, token.TokenHash)
	require.NoError(t, err)
	require.Equal(t, token, reloaded)

	if _, err := h.toks.ByHash(h.ctx, h.digest(t)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ByHash(unknown) error = %v, want %v", err, domain.ErrNotFound)
	}
}

func TestIntegrationRefreshTokenRejectsMissingAccount(t *testing.T) {
	h := setupHarness(t)

	now := time.Now().UTC()
	_, err := h.toks.Insert(h.ctx, domain.RefreshToken{
		UserID:    "00000000-0000-4000-8000-000000000000",
		FamilyID:  randomHex(t, 16),
		TokenHash: h.digest(t),
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now,
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Insert() error = %v, want %v for a foreign-key violation", err, domain.ErrConflict)
	}
}

func TestIntegrationReplayedDigestIsRejected(t *testing.T) {
	h := setupHarness(t)

	user := h.newAccount(t, domain.RolePatient)
	now := time.Now().UTC()
	digest := h.digest(t)

	_, err := h.toks.Insert(h.ctx, domain.RefreshToken{
		UserID: user.ID, FamilyID: randomHex(t, 16), TokenHash: digest,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	})
	require.NoError(t, err)

	_, err = h.toks.Insert(h.ctx, domain.RefreshToken{
		UserID: user.ID, FamilyID: randomHex(t, 16), TokenHash: digest,
		ExpiresAt: now.Add(2 * time.Hour), CreatedAt: now,
	})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("Insert() error = %v, want %v for a replayed digest", err, domain.ErrAlreadyExists)
	}
}

func TestIntegrationTokenRejectsShortExpiryAndNonHexDigest(t *testing.T) {
	h := setupHarness(t)

	user := h.newAccount(t, domain.RolePatient)
	now := time.Now().UTC()

	cases := []struct {
		name  string
		token domain.RefreshToken
	}{
		{
			name: "expires before creation",
			token: domain.RefreshToken{UserID: user.ID, FamilyID: randomHex(t, 16), TokenHash: h.digest(t),
				ExpiresAt: now.Add(-time.Hour), CreatedAt: now},
		},
		{
			name: "plaintext secret stored instead of a digest",
			token: domain.RefreshToken{UserID: user.ID, FamilyID: randomHex(t, 16), TokenHash: "super-secret-token",
				ExpiresAt: now.Add(time.Hour), CreatedAt: now},
		},
		{
			name: "family id is not 32 hex characters",
			token: domain.RefreshToken{UserID: user.ID, FamilyID: "family-1", TokenHash: h.digest(t),
				ExpiresAt: now.Add(time.Hour), CreatedAt: now},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := h.toks.Insert(h.ctx, tc.token); err == nil {
				t.Fatal("Insert() = nil error, want the CHECK constraint to reject the row")
			}
		})
	}
}

// TestIntegrationConsumeHasExactlyOneWinner is what makes rotation safe: the compare-and-set is
// decided by the server, so a stolen secret replayed in parallel cannot mint two sessions.
func TestIntegrationConsumeHasExactlyOneWinner(t *testing.T) {
	h := setupHarness(t)

	user := h.newAccount(t, domain.RolePatient)
	token := h.insertToken(t, user.ID, randomHex(t, 16), time.Now().UTC())

	const racers = 8
	start := make(chan struct{})
	errs := make(chan error, racers)
	var wins atomic.Int32
	var wg sync.WaitGroup

	for range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			<-start
			consumed, err := h.toks.Consume(h.ctx, token.ID, time.Now().UTC())
			if err != nil {
				errs <- err
				return
			}
			if consumed {
				wins.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("Consume() error = %v", err)
	}
	if got := wins.Load(); got != 1 {
		t.Fatalf("%d callers consumed the same token, want exactly 1", got)
	}

	again, err := h.toks.Consume(h.ctx, token.ID, time.Now().UTC())
	require.NoError(t, err)
	if again {
		t.Error("a consumed token is consumable again, so reuse detection can never fire")
	}

	stored, err := h.toks.ByHash(h.ctx, token.TokenHash)
	require.NoError(t, err)
	require.NotNil(t, stored.UsedAt)

	// The digest column is unique, so a re-issue cannot collide with the consumed row.
	rotated := h.insertToken(t, user.ID, token.FamilyID, time.Now().UTC())
	if rotated.TokenHash == token.TokenHash {
		t.Fatal("rotation reused the digest, so the old secret would still authenticate")
	}
}

func TestIntegrationRevocationsAreScopedAndIdempotent(t *testing.T) {
	h := setupHarness(t)

	patient := h.newAccount(t, domain.RolePatient)
	doctor := h.newAccount(t, domain.RoleDoctor)

	now := time.Now().UTC().Truncate(time.Microsecond)
	targetFamily := randomHex(t, 16)

	sibling := h.insertToken(t, patient.ID, randomHex(t, 16), now)
	target := h.insertToken(t, patient.ID, targetFamily, now)
	otherAccount := h.insertToken(t, doctor.ID, randomHex(t, 16), now)

	require.NoError(t, h.toks.RevokeFamily(h.ctx, targetFamily, now))

	revoked, err := h.toks.ByHash(h.ctx, target.TokenHash)
	require.NoError(t, err)
	require.NotNil(t, revoked.RevokedAt, "the target family must be revoked")

	firstRevocation := revoked.RevokedAt

	// Revoking twice must not move the timestamp: the first revocation is the auditable one.
	require.NoError(t, h.toks.RevokeFamily(h.ctx, targetFamily, now.Add(time.Hour)))
	again, err := h.toks.ByHash(h.ctx, target.TokenHash)
	require.NoError(t, err)
	require.True(t, again.RevokedAt.Equal(*firstRevocation),
		"revoked_at moved to %v, want the idempotent %v", again.RevokedAt, firstRevocation)

	for _, untouched := range []domain.RefreshToken{sibling, otherAccount} {
		stored, err := h.toks.ByHash(h.ctx, untouched.TokenHash)
		require.NoError(t, err)
		if stored.RevokedAt != nil {
			t.Errorf("token %s of family %s was revoked by a different family", stored.ID, stored.FamilyID)
		}
	}

	require.NoError(t, h.toks.RevokeAllForUser(h.ctx, patient.ID, now.Add(2*time.Hour)))

	byFamily, err := h.toks.ByHash(h.ctx, target.TokenHash)
	require.NoError(t, err)
	require.True(t, byFamily.RevokedAt.Equal(*firstRevocation),
		"revoke-all must leave an already revoked row at its original timestamp")

	for _, digest := range []string{sibling.TokenHash, otherAccount.TokenHash} {
		stored, err := h.toks.ByHash(h.ctx, digest)
		require.NoError(t, err)

		shouldBeRevoked := digest == sibling.TokenHash
		if shouldBeRevoked != (stored.RevokedAt != nil) {
			t.Errorf("token %s revoked=%v, wrong for account %s", stored.ID, stored.RevokedAt != nil, stored.UserID)
		}
	}
}

func TestIntegrationDeletingAnAccountCascadesItsSessions(t *testing.T) {
	h := setupHarness(t)

	user := h.newAccount(t, domain.RolePatient)
	token := h.insertToken(t, user.ID, randomHex(t, 16), time.Now().UTC())

	_, err := h.pool.Exec(h.ctx, "DELETE FROM users WHERE id = $1::uuid", user.ID)
	require.NoError(t, err)

	if _, err := h.toks.ByHash(h.ctx, token.TokenHash); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ByHash() error = %v, want the digest to be gone after the cascade", err)
	}

	var remaining int
	require.NoError(t, h.pool.QueryRow(h.ctx, "SELECT count(*) FROM refresh_tokens WHERE user_id = $1::uuid", user.ID).Scan(&remaining))
	if remaining != 0 {
		t.Errorf("%d sessions survived their account, ON DELETE CASCADE is not in effect", remaining)
	}
}

func TestIntegrationCancelledContextReachesNoServer(t *testing.T) {
	h := setupHarness(t)

	ctx, cancel := context.WithCancel(h.ctx)
	cancel()

	if _, err := h.users.ByID(ctx, "00000000-0000-4000-8000-000000000000"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ByID() error = %v, want the cancellation to surface unchanged", err)
	}
	if _, err := h.users.List(ctx, usecase.UserFilter{Limit: 1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v, want the cancellation to surface unchanged", err)
	}
}
