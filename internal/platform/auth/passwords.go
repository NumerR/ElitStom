package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/NumerR/ElitStom/internal/domain"
)

// BcryptHasher hashes and verifies passwords.
type BcryptHasher struct {
	cost      int
	dummyHash string
}

// The cost window the service accepts. bcrypt itself is not a guard here: it silently raises
// anything below MinCost to MinCost, and a typo of 31 would take ~40 minutes per login.
// These bounds match config validation so a bad BCRYPT_COST fails at startup either way.
const (
	minCost = 10
	maxCost = 15
)

func NewBcryptHasher(cost int) (*BcryptHasher, error) {
	if cost < minCost || cost > maxCost {
		return nil, fmt.Errorf("auth: bcrypt cost must be between %d and %d, got %d", minCost, maxCost, cost)
	}

	// Verifying against this hash burns the same CPU as verifying a real one, so a login
	// attempt for an unknown email is indistinguishable from one for an existing account.
	// The placeholder is random and discarded: a fixed string would let a caller who guessed
	// it produce a successful comparison.
	placeholder, err := randomBytes(32)
	if err != nil {
		return nil, fmt.Errorf("auth: build dummy hash: %w", err)
	}
	dummy, err := bcrypt.GenerateFromPassword(placeholder, cost)
	if err != nil {
		return nil, fmt.Errorf("auth: build dummy hash: %w", err)
	}

	return &BcryptHasher{cost: cost, dummyHash: string(dummy)}, nil
}

func (h *BcryptHasher) Cost() int { return h.cost }

// DummyHash returns a hash that no plaintext matches, for constant-time login attempts
// against unknown users.
func (h *BcryptHasher) DummyHash() string { return h.dummyHash }

func (h *BcryptHasher) Hash(password string) (string, error) {
	if len(password) > domain.MaxPasswordBytes {
		return "", fmt.Errorf("%w: password must not exceed %d bytes", domain.ErrInvalidInput, domain.MaxPasswordBytes)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(hash), nil
}

// Verify returns domain.ErrInvalidCredentials only for a genuine mismatch. A corrupt or
// unreadable stored hash is a data problem and surfaces as an internal error, so it is
// never mistaken for a user's failed login.
func (h *BcryptHasher) Verify(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return domain.ErrInvalidCredentials
		}
		return fmt.Errorf("auth: verify password: %w", err)
	}
	return nil
}
