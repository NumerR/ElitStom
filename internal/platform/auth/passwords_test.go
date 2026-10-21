package auth

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// testCost is the cheapest cost this service allows. It keeps the suite in the seconds
// range while still exercising real bcrypt.
const testCost = 10

func newTestHasher(t *testing.T, cost int) *BcryptHasher {
	t.Helper()

	hasher, err := NewBcryptHasher(cost)
	if err != nil {
		t.Fatalf("NewBcryptHasher(%d) error = %v", cost, err)
	}
	return hasher
}

func TestNewBcryptHasherRejectsInvalidCosts(t *testing.T) {
	cases := []struct {
		name  string
		cost  int
		valid bool
	}{
		{name: "below the floor", cost: 3},
		{name: "zero", cost: 0},
		{name: "negative", cost: -1},
		{name: "one below the floor", cost: 9},
		{name: "bcrypt default is below the floor", cost: bcrypt.MinCost},
		{name: "one above the ceiling", cost: 16},
		{name: "bcrypt maximum is refused as a typo", cost: bcrypt.MaxCost},
		{name: "floor", cost: 10, valid: true},
		{name: "production default", cost: 12, valid: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hasher, err := NewBcryptHasher(tc.cost)

			if !tc.valid {
				if err == nil {
					t.Fatalf("NewBcryptHasher(%d) = %+v, want an error", tc.cost, hasher)
				}
				if hasher != nil {
					t.Error("NewBcryptHasher returned a hasher alongside an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewBcryptHasher(%d) error = %v", tc.cost, err)
			}
			if hasher.Cost() != tc.cost {
				t.Errorf("Cost() = %d, want %d", hasher.Cost(), tc.cost)
			}
			if hasher.DummyHash() == "" {
				t.Error("DummyHash() is empty, so unknown-account logins would skip the bcrypt cost")
			}
		})
	}
}

func TestHashAndVerifyRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		password string
	}{
		{name: "ascii passphrase", password: "correct horse battery staple"},
		{name: "minimum policy length", password: "twelve-chars!!"},
		{name: "multibyte", password: "пароль-с-кириллицей-12"},
		{name: "emoji", password: "🦷-dentist-queue"},
		{name: "exactly the bcrypt limit", password: strings.Repeat("k", domain.MaxPasswordBytes)},
		{name: "spaces only but long", password: strings.Repeat(" ", 20)},
	}

	hasher := newTestHasher(t, testCost)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := hasher.Hash(tc.password)
			if err != nil {
				t.Fatalf("Hash() error = %v", err)
			}
			if hash == tc.password {
				t.Error("Hash() returned the plaintext")
			}
			if !strings.HasPrefix(hash, "$2") {
				t.Errorf("Hash() = %q, want a bcrypt digest", hash)
			}
			if err := hasher.Verify(hash, tc.password); err != nil {
				t.Errorf("Verify() error = %v, want the round trip to succeed", err)
			}
		})
	}
}

func TestHashSaltsEachPassword(t *testing.T) {
	hasher := newTestHasher(t, testCost)
	const password = "same-password-twice"

	first, err := hasher.Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	second, err := hasher.Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	if first == second {
		t.Error("two hashes of the same password are identical, so bcrypt is not salting")
	}
	if err := hasher.Verify(first, password); err != nil {
		t.Errorf("Verify(first) error = %v", err)
	}
	if err := hasher.Verify(second, password); err != nil {
		t.Errorf("Verify(second) error = %v", err)
	}
}

func TestHashRejectsOverlongPassword(t *testing.T) {
	hasher := newTestHasher(t, testCost)

	// bcrypt truncates silently at 72 bytes; accepting that would make a 200-byte password
	// and its first 72 bytes interchangeable secrets.
	hash, err := hasher.Hash(strings.Repeat("k", domain.MaxPasswordBytes+1))

	if err == nil {
		t.Fatal("Hash() = nil error, want rejection of an overlong password")
	}
	if hash != "" {
		t.Errorf("Hash() = %q, want empty on failure", hash)
	}
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("error = %v, want errors.Is(err, domain.ErrInvalidInput)", err)
	}
}

func TestVerifyReportsMismatchAsInvalidCredentials(t *testing.T) {
	cases := []struct {
		name     string
		password string
	}{
		{name: "wrong password", password: "wrong-password-here"},
		{name: "empty password", password: ""},
		{name: "prefix of the real password", password: "correct"},
		{name: "case differs", password: "CORRECT HORSE BATTERY STAPLE"},
	}

	hasher := newTestHasher(t, testCost)
	hash, err := hasher.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := hasher.Verify(hash, tc.password)
			if !errors.Is(err, domain.ErrInvalidCredentials) {
				t.Errorf("Verify() error = %v, want %v", err, domain.ErrInvalidCredentials)
			}
		})
	}
}

func TestVerifyTreatsACorruptHashAsAnInternalError(t *testing.T) {
	cases := []struct {
		name string
		hash string
	}{
		{name: "empty hash", hash: ""},
		{name: "not a bcrypt digest", hash: "plaintext-stored-by-mistake"},
		{name: "truncated digest", hash: "$2a$10$abbreviated"},
		{name: "unknown version prefix", hash: "$9z$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"},
	}

	hasher := newTestHasher(t, testCost)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := hasher.Verify(tc.hash, "any-password")
			if err == nil {
				t.Fatal("Verify() = nil, want an error for a corrupt hash")
			}
			// Reporting it as invalid credentials would tell an attacker that the account is
			// reachable and turn a data bug into an authentication bypass investigation.
			if errors.Is(err, domain.ErrInvalidCredentials) {
				t.Errorf("error = %v, must not be reported as a bad password", err)
			}
		})
	}
}

func TestDummyHashMatchesNothing(t *testing.T) {
	hasher := newTestHasher(t, testCost)

	for _, guess := range []string{"", "timing-equaliser", "correct horse battery staple", "password123456"} {
		err := hasher.Verify(hasher.DummyHash(), guess)
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Errorf("Verify(dummy, %q) error = %v, want %v", guess, err, domain.ErrInvalidCredentials)
		}
	}

	// The dummy must be a real bcrypt hash at the configured cost, or the timing equaliser
	// short-circuits and account enumeration works again.
	cost, err := bcrypt.Cost([]byte(hasher.DummyHash()))
	if err != nil {
		t.Fatalf("DummyHash() is not a bcrypt digest: %v", err)
	}
	if cost != testCost {
		t.Errorf("DummyHash() cost = %d, want %d", cost, testCost)
	}
}

func TestDummyHashIsStableWithinOneHasher(t *testing.T) {
	hasher := newTestHasher(t, testCost)

	if hasher.DummyHash() != hasher.DummyHash() {
		t.Error("DummyHash() must be precomputed once, not minted per login")
	}
}

func TestHasherSatisfiesTheUsecasePort(t *testing.T) {
	var hasher usecase.PasswordHasher = newTestHasher(t, testCost)

	if hasher == nil {
		t.Error("BcryptHasher must implement usecase.PasswordHasher")
	}
}

func TestSecretsAndIssuerSatisfyTheUsecasePorts(t *testing.T) {
	var (
		issuer  usecase.TokenIssuer     = newTestIssuer(t, testAuthConfig())
		secrets usecase.SecretGenerator = NewSecretGenerator()
	)

	if issuer == nil {
		t.Error("TokenIssuer must implement usecase.TokenIssuer")
	}
	if secrets == nil {
		t.Error("SecretGenerator must implement usecase.SecretGenerator")
	}
}
