package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/usecase"
)

const tokenColumns = "id, user_id, family_id, token_hash, expires_at, created_at, used_at, revoked_at"

const insertRefreshToken = `INSERT INTO refresh_tokens (user_id, family_id, token_hash, expires_at, created_at)
VALUES ($1::uuid, $2, $3, $4, $5)
RETURNING ` + tokenColumns

const selectRefreshTokenByHash = `SELECT ` + tokenColumns + ` FROM refresh_tokens WHERE token_hash = $1`

// consumeRefreshToken is the atomic step of rotation. The WHERE clause is what makes it safe:
// two concurrent requests presenting the same secret cannot both see used_at IS NULL, so only
// one reports one row affected.
const consumeRefreshToken = `UPDATE refresh_tokens
SET used_at = $2
WHERE id = $1::uuid AND used_at IS NULL`

const revokeTokenFamily = `UPDATE refresh_tokens
SET revoked_at = $2
WHERE family_id = $1::text AND revoked_at IS NULL`

const revokeTokensForUser = `UPDATE refresh_tokens
SET revoked_at = $2
WHERE user_id = $1::uuid AND revoked_at IS NULL`

// RefreshTokens implements usecase.RefreshTokenStore. Nothing here can return a stored token
// by account: the plaintext secret exists only in the response that minted it.
type RefreshTokens struct {
	db Executor
}

func NewRefreshTokens(db Executor) *RefreshTokens {
	return &RefreshTokens{db: db}
}

var _ usecase.RefreshTokenStore = (*RefreshTokens)(nil)

func (r *RefreshTokens) Insert(ctx context.Context, token domain.RefreshToken) (domain.RefreshToken, error) {
	if err := ctx.Err(); err != nil {
		return domain.RefreshToken{}, err
	}
	if !isUUID(token.UserID) {
		return domain.RefreshToken{}, fmt.Errorf("%w: account id is malformed", domain.ErrInvalidInput)
	}

	stored, err := scanToken(r.db.QueryRow(ctx, insertRefreshToken,
		token.UserID, token.FamilyID, token.TokenHash, token.ExpiresAt, token.CreatedAt,
	))
	if err != nil {
		return domain.RefreshToken{}, translate(err, "refresh token")
	}
	return stored, nil
}

func (r *RefreshTokens) ByHash(ctx context.Context, hash string) (domain.RefreshToken, error) {
	if err := ctx.Err(); err != nil {
		return domain.RefreshToken{}, err
	}

	token, err := scanToken(r.db.QueryRow(ctx, selectRefreshTokenByHash, hash))
	if err != nil {
		return domain.RefreshToken{}, translate(err, "refresh token")
	}
	return token, nil
}

// Consume reports false when the token was already consumed, which the use case treats as
// evidence of a leaked secret.
func (r *RefreshTokens) Consume(ctx context.Context, id string, at time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !isUUID(id) {
		return false, fmt.Errorf("%w: token id is malformed", domain.ErrInvalidInput)
	}

	tag, err := r.db.Exec(ctx, consumeRefreshToken, id, at)
	if err != nil {
		return false, translate(err, "refresh token")
	}
	return tag.RowsAffected() == 1, nil
}

func (r *RefreshTokens) RevokeFamily(ctx context.Context, familyID string, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if _, err := r.db.Exec(ctx, revokeTokenFamily, familyID, at); err != nil {
		return translate(err, "token family")
	}
	return nil
}

func (r *RefreshTokens) RevokeAllForUser(ctx context.Context, userID string, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !isUUID(userID) {
		return fmt.Errorf("%w: account id is malformed", domain.ErrInvalidInput)
	}

	if _, err := r.db.Exec(ctx, revokeTokensForUser, userID, at); err != nil {
		return translate(err, "token family")
	}
	return nil
}

func scanToken(row Row) (domain.RefreshToken, error) {
	var token domain.RefreshToken

	if err := row.Scan(
		&token.ID,
		&token.UserID,
		&token.FamilyID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.CreatedAt,
		&token.UsedAt,
		&token.RevokedAt,
	); err != nil {
		return domain.RefreshToken{}, err
	}
	return token, nil
}
