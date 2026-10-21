package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewRefreshToken(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name    string
		params  RefreshTokenParams
		wantErr string
	}{
		{
			name:   "valid",
			params: RefreshTokenParams{UserID: "user-1", FamilyID: "family-1", TokenHash: "digest", ExpiresAt: now.Add(time.Hour), Now: now},
		},
		{name: "missing user", params: RefreshTokenParams{FamilyID: "f", TokenHash: "d", ExpiresAt: now.Add(time.Hour), Now: now}, wantErr: "user_id"},
		{name: "missing family", params: RefreshTokenParams{UserID: "u", TokenHash: "d", ExpiresAt: now.Add(time.Hour), Now: now}, wantErr: "family_id"},
		{name: "missing hash", params: RefreshTokenParams{UserID: "u", FamilyID: "f", ExpiresAt: now.Add(time.Hour), Now: now}, wantErr: "refresh_token"},
		{name: "already expired", params: RefreshTokenParams{UserID: "u", FamilyID: "f", TokenHash: "d", ExpiresAt: now, Now: now}, wantErr: "expires_at"},
		{name: "expires in the past", params: RefreshTokenParams{UserID: "u", FamilyID: "f", TokenHash: "d", ExpiresAt: now.Add(-time.Second), Now: now}, wantErr: "expires_at"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token, err := NewRefreshToken(tc.params)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("NewRefreshToken() = nil error, want %q", tc.wantErr)
				}
				var ve *ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("error = %T, want *ValidationError", err)
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("error = %v, want errors.Is(err, ErrInvalidInput)", err)
				}
				var found bool
				for _, field := range ve.Fields {
					if field.Field == tc.wantErr {
						found = true
					}
				}
				if !found {
					t.Errorf("validation fields = %v, want one for %q", ve.Details(), tc.wantErr)
				}
				if token != nil {
					t.Errorf("NewRefreshToken() returned %+v alongside an error", *token)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewRefreshToken() error = %v", err)
			}
			if token.CreatedAt != now {
				t.Errorf("CreatedAt = %v, want the injected clock value %v", token.CreatedAt, now)
			}
			if token.ID != "" {
				t.Errorf("ID = %q, the repository assigns the id", token.ID)
			}
			if token.UsedAt != nil || token.RevokedAt != nil {
				t.Error("a new token must be neither used nor revoked")
			}
			if !token.IsUsable(now) {
				t.Error("IsUsable() = false for a fresh token")
			}
		})
	}
}

func TestRefreshTokenStateTransitions(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	expiry := now.Add(time.Hour)

	cases := []struct {
		name        string
		mutate      func(*RefreshToken)
		wantUsable  bool
		wantUsed    bool
		wantRevoked bool
		wantExpired bool
		at          time.Time
	}{
		{name: "fresh", wantUsable: true, at: now},
		{name: "one second before expiry", wantUsable: true, at: expiry.Add(-time.Nanosecond)},
		{name: "exactly at expiry", wantExpired: true, at: expiry},
		{name: "after expiry", wantExpired: true, at: expiry.Add(time.Minute)},
		{
			name:     "consumed",
			mutate:   func(tok *RefreshToken) { tok.MarkUsed(now.Add(time.Minute)) },
			wantUsed: true,
			at:       now,
		},
		{
			name:        "revoked",
			mutate:      func(tok *RefreshToken) { tok.Revoke(now) },
			wantRevoked: true,
			at:          now,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token := &RefreshToken{UserID: "u", FamilyID: "f", TokenHash: "d", ExpiresAt: expiry, CreatedAt: now}
			if tc.mutate != nil {
				tc.mutate(token)
			}

			if got := token.IsUsable(tc.at); got != tc.wantUsable {
				t.Errorf("IsUsable(%v) = %v, want %v", tc.at, got, tc.wantUsable)
			}
			if got := token.IsUsed(); got != tc.wantUsed {
				t.Errorf("IsUsed() = %v, want %v", got, tc.wantUsed)
			}
			if got := token.IsRevoked(); got != tc.wantRevoked {
				t.Errorf("IsRevoked() = %v, want %v", got, tc.wantRevoked)
			}
			if got := token.IsExpired(tc.at); got != tc.wantExpired {
				t.Errorf("IsExpired(%v) = %v, want %v", tc.at, got, tc.wantExpired)
			}
		})
	}
}

func TestRefreshTokenRevokeIsIdempotent(t *testing.T) {
	first := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	later := first.Add(time.Hour)

	token := &RefreshToken{ExpiresAt: later}
	token.Revoke(first)
	token.Revoke(later)

	if token.RevokedAt == nil {
		t.Fatal("RevokedAt = nil, want the first revocation timestamp")
	}
	if !token.RevokedAt.Equal(first) {
		t.Errorf("RevokedAt = %v, want it to stay %v so the audit trail shows when the leak was caught", *token.RevokedAt, first)
	}
}

func TestRefreshTokenMarkUsedRecordsTheRotation(t *testing.T) {
	at := time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC)

	token := &RefreshToken{ExpiresAt: at.Add(time.Hour)}
	token.MarkUsed(at)

	if token.UsedAt == nil {
		t.Fatal("UsedAt = nil after MarkUsed")
	}
	if !token.UsedAt.Equal(at) {
		t.Errorf("UsedAt = %v, want %v", *token.UsedAt, at)
	}
	if token.IsUsable(at) {
		t.Error("a consumed token must not be usable")
	}
}
