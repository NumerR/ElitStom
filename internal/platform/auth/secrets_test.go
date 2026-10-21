package auth

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestNewRefreshTokenIsHighEntropyAndURLSafe(t *testing.T) {
	secrets := NewSecretGenerator()
	seen := map[string]bool{}

	for range 50 {
		token, err := secrets.NewRefreshToken()
		if err != nil {
			t.Fatalf("NewRefreshToken() error = %v", err)
		}
		if seen[token] {
			t.Fatalf("NewRefreshToken() repeated %q, so sessions collide", token)
		}
		seen[token] = true

		// 32 random bytes in base64.RawURLEncoding is 43 characters; anything shorter means
		// the generator lost entropy.
		if len(token) < 43 {
			t.Errorf("NewRefreshToken() = %d characters, want a full 256-bit secret", len(token))
		}
		if strings.ContainsAny(token, "+/=") {
			t.Errorf("NewRefreshToken() = %q, must be URL-safe with no padding", token)
		}
		if _, err := base64.RawURLEncoding.DecodeString(token); err != nil {
			t.Errorf("NewRefreshToken() = %q is not valid base64url: %v", token, err)
		}
	}
}

func TestNewFamilyIDIsUniqueHex(t *testing.T) {
	secrets := NewSecretGenerator()
	seen := map[string]bool{}

	for range 20 {
		family, err := secrets.NewFamilyID()
		if err != nil {
			t.Fatalf("NewFamilyID() error = %v", err)
		}
		if seen[family] {
			t.Errorf("NewFamilyID() repeated %q", family)
		}
		seen[family] = true

		if len(family) != 32 {
			t.Errorf("NewFamilyID() = %q (%d chars), want 16 bytes as hex", family, len(family))
		}
		if strings.ToLower(family) != family {
			t.Errorf("NewFamilyID() = %q, want lower-case hex so lookups compare exactly", family)
		}
	}
}

func TestHashTokenIsDeterministicAndOneWay(t *testing.T) {
	secrets := NewSecretGenerator()

	cases := []struct {
		name  string
		token string
	}{
		{name: "typical secret", token: "an-opaque-refresh-secret"},
		{name: "empty input still digests", token: ""},
		{name: "multibyte", token: "secret-with-🦷"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hash := secrets.HashToken(tc.token)

			if secrets.HashToken(tc.token) != hash {
				t.Error("HashToken is not deterministic, so a lookup by digest can never find the row")
			}
			if len(hash) != 64 || strings.ToLower(hash) != hash {
				t.Errorf("HashToken = %q, want 32 raw bytes as lower-case hex", hash)
			}
			if strings.Contains(hash, tc.token) && tc.token != "" {
				t.Error("HashToken leaked the secret")
			}

			if other := secrets.HashToken(tc.token + "x"); other == hash {
				t.Error("HashToken ignored an appended character")
			}
		})
	}
}

func TestHashTokenSeparatesDistinctSecrets(t *testing.T) {
	secrets := NewSecretGenerator()
	hashes := map[string]bool{}

	for range 100 {
		token, err := secrets.NewRefreshToken()
		if err != nil {
			t.Fatalf("NewRefreshToken() error = %v", err)
		}
		hash := secrets.HashToken(token)
		if hashes[hash] {
			t.Fatal("two distinct secrets produced the same digest")
		}
		hashes[hash] = true
	}
}
