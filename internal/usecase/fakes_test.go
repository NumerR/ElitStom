package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/stretchr/testify/require"
)

// The fakes below stand in for the adapters so the business rules can be exercised without
// Postgres or a network. They are deliberately simple but honest: state transitions match
// what the SQL adapters do, including the atomicity Consume relies on.

type fakeClock struct {
	mu sync.Mutex
	at time.Time
}

func newFakeClock(at time.Time) *fakeClock { return &fakeClock{at: at.UTC()} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *fakeClock) Advance(by time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(by)
	return c.at
}

type fakeUsers struct {
	mu      sync.Mutex
	byID    map[string]domain.User
	emailID map[string]string
	nextID  int
	created int
	updated int

	createErr        error
	updateErr        error
	listErr          error
	byIDErr          error
	emailLookupErr   error
	lastList         UserFilter
	lastUpdatedEmail string
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byID: map[string]domain.User{}, emailID: map[string]string{}}
}

func (f *fakeUsers) Create(_ context.Context, user domain.User) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.createErr != nil {
		return domain.User{}, f.createErr
	}
	if _, exists := f.emailID[user.Email]; exists {
		return domain.User{}, fmt.Errorf("%w: email %s", domain.ErrAlreadyExists, user.Email)
	}

	f.nextID++
	f.created++
	user.ID = fmt.Sprintf("user-%d", f.nextID)
	f.byID[user.ID] = user
	f.emailID[user.Email] = user.ID
	return user, nil
}

func (f *fakeUsers) ByID(_ context.Context, id string) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.byIDErr != nil {
		return domain.User{}, f.byIDErr
	}
	return f.lockedByID(id)
}

func (f *fakeUsers) lockedByID(id string) (domain.User, error) {
	user, ok := f.byID[id]
	if !ok {
		return domain.User{}, fmt.Errorf("%w: user %s", domain.ErrNotFound, id)
	}
	return user, nil
}

func (f *fakeUsers) ByEmail(_ context.Context, email string) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.emailLookupErr != nil {
		return domain.User{}, f.emailLookupErr
	}
	id, ok := f.emailID[strings.ToLower(email)]
	if !ok {
		return domain.User{}, fmt.Errorf("%w: email %s", domain.ErrNotFound, email)
	}
	return f.lockedByID(id)
}

func (f *fakeUsers) Update(_ context.Context, user domain.User) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.updateErr != nil {
		return domain.User{}, f.updateErr
	}
	if _, ok := f.byID[user.ID]; !ok {
		return domain.User{}, fmt.Errorf("%w: user %s", domain.ErrNotFound, user.ID)
	}
	f.updated++
	f.lastUpdatedEmail = user.Email
	f.byID[user.ID] = user
	return user, nil
}

func (f *fakeUsers) List(_ context.Context, filter UserFilter) ([]domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.listErr != nil {
		return nil, f.listErr
	}
	f.lastList = filter

	ids := make([]string, 0, len(f.byID))
	for id := range f.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	listed := make([]domain.User, 0, len(ids))
	for _, id := range ids {
		user := f.byID[id]
		if (filter.Role != "" && user.Role != filter.Role) || (filter.Status != "" && user.Status != filter.Status) {
			continue
		}
		listed = append(listed, user)
	}
	return listed, nil
}

func (f *fakeUsers) seed(user domain.User) string {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextID++
	user.ID = fmt.Sprintf("user-%d", f.nextID)
	f.byID[user.ID] = user
	f.emailID[user.Email] = user.ID
	return user.ID
}

func (f *fakeUsers) statusOf(id string) domain.UserStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byID[id].Status
}

func (f *fakeUsers) roleOf(id string) domain.Role {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byID[id].Role
}

func (f *fakeUsers) forceStatus(id string, status domain.UserStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()

	user := f.byID[id]
	user.Status = status
	f.byID[id] = user
}

// deleteAccount simulates a hard delete, leaving refresh tokens orphaned.
func (f *fakeUsers) deleteAccount(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.byID[id]; !ok {
		return fmt.Errorf("%w: user %s", domain.ErrNotFound, id)
	}
	delete(f.byID, id)
	return nil
}

// onlyID returns the single stored account's id, failing loudly if the fixture is not unique.
func (f *fakeUsers) onlyID() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.byID) != 1 {
		panic(fmt.Sprintf("fixture expects exactly one account, found %d", len(f.byID)))
	}
	for id := range f.byID {
		return id
	}
	return ""
}

type fakeRefresh struct {
	mu              sync.Mutex
	byID            map[string]domain.RefreshToken
	byHash          map[string]string
	nextID          int
	revokedFamilies []string
	revokedUsers    []string

	// failOn names the method that should error, the way a dead connection or a constraint
	// violation would surface from the SQL adapter.
	failOn             string
	failErr            error
	consumeAlwaysLoses bool
}

// errStoreUnavailable stands in for any driver-level failure.
var errStoreUnavailable = errors.New("refresh store unavailable")

func (f *fakeRefresh) failure(op string) error {
	if f.failOn == op {
		return f.failErr
	}
	return nil
}

func newFakeRefresh() *fakeRefresh {
	return &fakeRefresh{byID: map[string]domain.RefreshToken{}, byHash: map[string]string{}}
}

func (f *fakeRefresh) Insert(_ context.Context, token domain.RefreshToken) (domain.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.failure("insert"); err != nil {
		return domain.RefreshToken{}, err
	}
	f.nextID++
	token.ID = fmt.Sprintf("token-%d", f.nextID)
	f.byID[token.ID] = token
	f.byHash[token.TokenHash] = token.ID
	return token, nil
}

func (f *fakeRefresh) ByHash(_ context.Context, hash string) (domain.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.failure("by-hash"); err != nil {
		return domain.RefreshToken{}, err
	}
	id, ok := f.byHash[hash]
	if !ok {
		return domain.RefreshToken{}, fmt.Errorf("%w: token digest", domain.ErrNotFound)
	}
	return f.byID[id], nil
}

// Consume mirrors `UPDATE ... WHERE used_at IS NULL RETURNING 1`: the check and the write
// happen together, so only one concurrent caller can win the token.
func (f *fakeRefresh) Consume(_ context.Context, id string, at time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.failure("consume"); err != nil {
		return false, err
	}
	token, ok := f.byID[id]
	if !ok {
		return false, fmt.Errorf("%w: token %s", domain.ErrNotFound, id)
	}
	if token.UsedAt != nil || f.consumeAlwaysLoses {
		return false, nil
	}
	token.MarkUsed(at)
	f.byID[id] = token
	return true, nil
}

func (f *fakeRefresh) RevokeFamily(_ context.Context, familyID string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.failure("revoke-family"); err != nil {
		return err
	}
	f.revokedFamilies = append(f.revokedFamilies, familyID)
	for id, token := range f.byID {
		if token.FamilyID == familyID {
			token.Revoke(at)
			f.byID[id] = token
		}
	}
	return nil
}

func (f *fakeRefresh) RevokeAllForUser(_ context.Context, userID string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.failure("revoke-all"); err != nil {
		return err
	}
	f.revokedUsers = append(f.revokedUsers, userID)
	for id, token := range f.byID {
		if token.UserID == userID {
			token.Revoke(at)
			f.byID[id] = token
		}
	}
	return nil
}

func (f *fakeRefresh) isRevoked(hash string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	id, ok := f.byHash[hash]
	return ok && f.byID[id].RevokedAt != nil
}

// inserts counts the sessions the store has been asked to persist.
func (f *fakeRefresh) inserts() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.nextID
}

func (f *fakeRefresh) isUsed(hash string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	id, ok := f.byHash[hash]
	return ok && f.byID[id].UsedAt != nil
}

// fakeIssuer produces recognisable tokens and keeps a registry, so a minted access token can
// be round-tripped back to an Actor without pulling a signing library into the test.
type fakeIssuer struct {
	mu     sync.Mutex
	issued map[string]Actor
	serial int
	err    error
}

func newFakeIssuer() *fakeIssuer { return &fakeIssuer{issued: map[string]Actor{}} }

func (f *fakeIssuer) IssueAccessToken(userID string, role domain.Role) (string, time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return "", time.Time{}, f.err
	}
	f.serial++
	token := fmt.Sprintf("access-%d", f.serial)
	f.issued[token] = Actor{UserID: userID, Role: role}
	return token, time.Now().UTC().Add(time.Hour), nil
}

func (f *fakeIssuer) ParseAccessToken(raw string) (Actor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	actor, ok := f.issued[raw]
	if !ok {
		return Actor{}, domain.ErrTokenInvalid
	}
	return actor, nil
}

// actorFor decodes a token a test received, failing the test when the issuer never minted it.
func (f *fakeIssuer) actorFor(t *testing.T, raw string) Actor {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()

	actor, ok := f.issued[raw]
	require.Truef(t, ok, "token %s was never issued", raw)
	return actor
}

// fakeHasher is a reversible stand-in for bcrypt. The security properties under test are
// which hash gets verified and in what order, not the cost of the KDF.
type fakeHasher struct {
	mu           sync.Mutex
	plainForHash map[string]string
	hashes       int
	dummyChecks  int
	err          error
}

func newFakeHasher() *fakeHasher {
	return &fakeHasher{plainForHash: map[string]string{fakeDummyHash: "never-matches"}}
}

const fakeDummyHash = "dummy-hash"

func (f *fakeHasher) Hash(password string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return "", f.err
	}
	f.hashes++
	hash := "hash:" + password
	f.plainForHash[hash] = password
	return hash, nil
}

func (f *fakeHasher) Verify(hash, password string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if hash == fakeDummyHash {
		f.dummyChecks++
	}
	if f.plainForHash[hash] != password {
		return domain.ErrInvalidCredentials
	}
	return nil
}

func (f *fakeHasher) DummyHash() string { return fakeDummyHash }

func (f *fakeHasher) seed(password string) string {
	f.mu.Lock()
	defer f.mu.Unlock()

	hash := "hash:" + password
	f.plainForHash[hash] = password
	return hash
}

// fakeSecrets mints deterministic, unique secrets so rotation is reproducible.
type fakeSecrets struct {
	mu     sync.Mutex
	serial int
	err    error
}

func (f *fakeSecrets) NewRefreshToken() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return "", f.err
	}
	f.serial++
	return fmt.Sprintf("refresh-%d", f.serial), nil
}

func (f *fakeSecrets) NewFamilyID() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return "", f.err
	}
	f.serial++
	return fmt.Sprintf("family-%d", f.serial), nil
}

func (f *fakeSecrets) HashToken(token string) string { return "digest:" + token }
