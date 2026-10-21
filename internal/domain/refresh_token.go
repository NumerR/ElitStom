package domain

import "time"

// RefreshToken is one member of a rotation family. Only the SHA-256 hash of the opaque
// secret is persisted, so a stolen database dump cannot be replayed as a session.
//
// Family semantics: login creates family F with token T1. Every refresh marks the
// presented token used and issues a successor in the same family. Presenting an
// already-used token means the secret leaked, and the whole family is revoked.
type RefreshToken struct {
	ID        string
	UserID    string
	FamilyID  string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
}

type RefreshTokenParams struct {
	UserID    string
	FamilyID  string
	TokenHash string
	ExpiresAt time.Time
	Now       time.Time
}

func NewRefreshToken(p RefreshTokenParams) (*RefreshToken, error) {
	ve := &ValidationError{}

	if p.UserID == "" {
		ve.Add("user_id", "is required")
	}
	if p.FamilyID == "" {
		ve.Add("family_id", "is required")
	}
	if p.TokenHash == "" {
		ve.Add("refresh_token", "is required")
	}
	if !p.ExpiresAt.After(p.Now) {
		ve.Add("expires_at", "must be after the creation time")
	}

	if len(ve.Fields) > 0 {
		return nil, ve
	}

	return &RefreshToken{
		UserID:    p.UserID,
		FamilyID:  p.FamilyID,
		TokenHash: p.TokenHash,
		ExpiresAt: p.ExpiresAt,
		CreatedAt: p.Now,
	}, nil
}

func (t *RefreshToken) IsExpired(now time.Time) bool { return !now.Before(t.ExpiresAt) }

func (t *RefreshToken) IsRevoked() bool { return t.RevokedAt != nil }

func (t *RefreshToken) IsUsed() bool { return t.UsedAt != nil }

// IsUsable reports whether this exact token may still be exchanged for a new pair.
func (t *RefreshToken) IsUsable(now time.Time) bool {
	return !t.IsRevoked() && !t.IsUsed() && !t.IsExpired(now)
}

// MarkUsed records the rotation that consumed this token.
func (t *RefreshToken) MarkUsed(at time.Time) { t.UsedAt = &at }

// Revoke is idempotent: the first revocation keeps its timestamp.
func (t *RefreshToken) Revoke(at time.Time) {
	if t.RevokedAt == nil {
		t.RevokedAt = &at
	}
}
