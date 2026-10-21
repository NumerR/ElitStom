package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// userColumns is the single source of truth for the read projection. Every query that returns
// a user selects it, so a scanned column can never drift from the list below.
const userColumns = "id, email, full_name, phone, role, status, password_hash, created_at, updated_at"

const selectUserByID = `SELECT ` + userColumns + ` FROM users WHERE id = $1::uuid`

const selectUserByEmail = `SELECT ` + userColumns + ` FROM users WHERE email = $1`

const insertUser = `INSERT INTO users (email, full_name, phone, role, status, password_hash)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING ` + userColumns

// updateUser writes the mutable fields. Email is absent by design: it is the account identity
// and changing it needs a verified-ownership flow, not an UPDATE.
const updateUser = `UPDATE users
SET full_name = $2, phone = $3, role = $4, status = $5, password_hash = $6, updated_at = now()
WHERE id = $1::uuid
RETURNING ` + userColumns

// listUsers binds role and status as values; an empty filter string means "no predicate", so
// there is no string interpolation anywhere near the WHERE clause.
const listUsers = `SELECT ` + userColumns + `
FROM users
WHERE ($1 = '' OR role = $1)
  AND ($2 = '' OR status = $2)
ORDER BY created_at DESC, id DESC
LIMIT $3
OFFSET $4`

// Users implements usecase.UserRepository on top of PostgreSQL.
type Users struct {
	db Executor
}

func NewUsers(db Executor) *Users {
	return &Users{db: db}
}

var _ usecase.UserRepository = (*Users)(nil)

func (r *Users) Create(ctx context.Context, user domain.User) (domain.User, error) {
	if err := ctx.Err(); err != nil {
		return domain.User{}, err
	}

	created, err := scanUser(r.db.QueryRow(ctx, insertUser,
		user.Email, user.FullName, user.Phone, string(user.Role), string(user.Status), user.PasswordHash,
	))
	if err != nil {
		return domain.User{}, translate(err, "account")
	}
	return created, nil
}

func (r *Users) ByID(ctx context.Context, id string) (domain.User, error) {
	if err := ctx.Err(); err != nil {
		return domain.User{}, err
	}
	if !isUUID(id) {
		return domain.User{}, fmt.Errorf("%w: account id is malformed", domain.ErrInvalidInput)
	}

	user, err := scanUser(r.db.QueryRow(ctx, selectUserByID, id))
	if err != nil {
		return domain.User{}, translate(err, "account")
	}
	return user, nil
}

func (r *Users) ByEmail(ctx context.Context, email string) (domain.User, error) {
	if err := ctx.Err(); err != nil {
		return domain.User{}, err
	}

	// The column constraint guarantees the stored value is already lower-case and trimmed, so
	// an exact match is both correct and able to use the unique index.
	user, err := scanUser(r.db.QueryRow(ctx, selectUserByEmail, email))
	if err != nil {
		return domain.User{}, translate(err, "account")
	}
	return user, nil
}

func (r *Users) Update(ctx context.Context, user domain.User) (domain.User, error) {
	if err := ctx.Err(); err != nil {
		return domain.User{}, err
	}
	if !isUUID(user.ID) {
		return domain.User{}, fmt.Errorf("%w: account id is malformed", domain.ErrInvalidInput)
	}

	updated, err := scanUser(r.db.QueryRow(ctx, updateUser,
		user.ID, user.FullName, user.Phone, string(user.Role), string(user.Status), user.PasswordHash,
	))
	if err != nil {
		return domain.User{}, translate(err, "account")
	}
	return updated, nil
}

func (r *Users) List(ctx context.Context, filter usecase.UserFilter) ([]domain.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, listUsers, filter.Role.String(), string(filter.Status), filter.Limit, filter.Offset)
	if err != nil {
		return nil, translate(err, "account list")
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, translate(err, "account list")
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, translate(err, "account list")
	}
	// A nil slice becomes JSON null, so an empty page is allocated explicitly.
	if users == nil {
		users = []domain.User{}
	}
	return users, nil
}

// scanUser reads one row of userColumns. Anything that can Scan — a pgx.Row, a pgx.Rows, or a
// canned row in a unit test — satisfies the mapper.
func scanUser(row Row) (domain.User, error) {
	var (
		user domain.User
		role string
	)

	if err := row.Scan(
		&user.ID,
		&user.Email,
		&user.FullName,
		&user.Phone,
		&role,
		&user.Status,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	); err != nil {
		return domain.User{}, err
	}

	// A stored role the domain no longer recognises is a data problem, not client input.
	parsed, err := domain.ParseRole(role)
	if err != nil {
		return domain.User{}, fmt.Errorf("repository: stored role %q is unknown: %w", role, errors.Join(domain.ErrConflict, err))
	}
	user.Role = parsed
	return user, nil
}
