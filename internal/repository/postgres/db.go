package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NumerR/ElitStom/internal/domain"
)

// Row and Rows are the narrowest shapes the adapters need from pgx. Declaring them here —
// rather than naming pgx.Row and pgx.Rows — lets the mapping code be unit-tested with a
// canned row instead of a live database.
type Row interface {
	Scan(dest ...any) error
}

type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

// Executor is the subset of pgxpool.Pool an adapter may use. It deliberately omits Begin:
// a repository must not open a transaction the use case does not know about.
type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) Row
}

// pool adapts *pgxpool.Pool to Executor.
type pool struct {
	db *pgxpool.Pool
}

func (p pool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return p.db.Exec(ctx, sql, args...)
}

func (p pool) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	rows, err := p.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (p pool) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return p.db.QueryRow(ctx, sql, args...)
}

// NewExecutor wraps a connection pool so the adapters depend on the narrow interface.
func NewExecutor(db *pgxpool.Pool) Executor {
	return pool{db: db}
}

// PostgreSQL error codes the adapters translate.
const (
	codeUniqueViolation           = "23505"
	codeForeignKeyViolation       = "23503"
	codeInvalidTextRepresentation = "22P02"
)

// translate turns a driver error into a domain sentinel so the use case and transport layers
// never branch on pgx types. Context cancellation is passed through untouched: swallowing it
// would hide a client disconnect as a server fault.
func translate(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", domain.ErrNotFound, what)
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case codeUniqueViolation:
			return fmt.Errorf("%w: %s already exists", domain.ErrAlreadyExists, what)
		case codeForeignKeyViolation:
			return fmt.Errorf("%w: %s references a missing account", domain.ErrConflict, what)
		case codeInvalidTextRepresentation:
			// Reaching the server with a malformed uuid means validation upstream was skipped.
			return fmt.Errorf("%w: %s is malformed", domain.ErrInvalidInput, what)
		}
	}
	return fmt.Errorf("repository: %s: %w", what, err)
}

// isUUID reports whether value is a canonical 8-4-4-4-12 hex uuid. Checking it here keeps a
// forged path parameter from becoming a database round trip.
func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !isHexDigit(byte(r)) {
				return false
			}
		}
	}
	return true
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
