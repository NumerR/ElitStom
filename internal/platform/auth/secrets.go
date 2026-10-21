package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

const (
	refreshTokenBytes = 32
	familyIDBytes     = 16
)

// SecretGenerator mints opaque refresh-token secrets and the digests stored for them.
type SecretGenerator struct{}

func NewSecretGenerator() SecretGenerator { return SecretGenerator{} }

func (SecretGenerator) NewRefreshToken() (string, error) {
	buf, err := randomBytes(refreshTokenBytes)
	if err != nil {
		return "", fmt.Errorf("auth: refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (SecretGenerator) NewFamilyID() (string, error) {
	buf, err := randomBytes(familyIDBytes)
	if err != nil {
		return "", fmt.Errorf("auth: family id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// HashToken digests a secret before it is persisted. A refresh token carries 256 bits of
// entropy, so plain SHA-256 is the right tool here — unlike passwords, which must use bcrypt.
func (SecretGenerator) HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
