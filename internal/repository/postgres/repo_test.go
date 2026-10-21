package postgres

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/usecase"
)

var (
	fakeID        = "0b8f5c1a-2d3e-4a5b-8c9d-0e1f2a3b4c5d"
	fakeFamilyID  = strings.Repeat("ab", 16)
	fakeCreatedAt = time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	fakeExpiry    = fakeCreatedAt.Add(72 * time.Hour)
)

// fakeRow is a canned result set row: Scan copies the prepared values into the destinations,
// which is all the mappers need.
type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("canned row has %d values but the query scans %d columns", len(r.values), len(dest))
	}
	for i, d := range dest {
		target := reflect.ValueOf(d)
		if target.Kind() != reflect.Pointer {
			return fmt.Errorf("column %d: destination is not a pointer", i)
		}
		slot := target.Elem()

		if r.values[i] == nil {
			if !slot.CanSet() || slot.Kind() != reflect.Pointer {
				return fmt.Errorf("column %d: a NULL needs a settable pointer destination, got %s", i, slot.Type())
			}
			slot.Set(reflect.Zero(slot.Type()))
			continue
		}

		src := reflect.ValueOf(r.values[i])
		// A driver hands back a plain string or time.Time, while the entity declares a named
		// type such as domain.UserStatus and the nullable columns are read through *time.Time.
		// Matching on Kind keeps the fake faithful without letting, say, a string land in an int.
		switch {
		case src.Type() == slot.Type():
			slot.Set(src)
		case slot.Kind() == reflect.Pointer && src.Kind() == slot.Type().Elem().Kind():
			value := reflect.New(slot.Type().Elem())
			value.Elem().Set(src)
			slot.Set(value)
		case src.Kind() == slot.Kind() && src.Type().ConvertibleTo(slot.Type()):
			slot.Set(src.Convert(slot.Type()))
		default:
			return fmt.Errorf("column %d: canned value %T cannot be scanned into %s", i, r.values[i], slot.Type())
		}
	}
	return nil
}

type fakeRows struct {
	rows     [][]any
	pos      int
	scanErr  error
	nextErr  error
	closeCnt int
}

func (r *fakeRows) Next() bool {
	if r.nextErr != nil && r.pos == 0 {
		return false
	}
	if r.pos >= len(r.rows) {
		return false
	}
	r.pos++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	return fakeRow{values: r.rows[r.pos-1]}.Scan(dest...)
}

func (r *fakeRows) Err() error { return r.nextErr }

func (r *fakeRows) Close() { r.closeCnt++ }

// recordedQuery is one call the adapter made, so a test can assert on the SQL text and the
// bound parameters instead of trusting comments.
type recordedQuery struct {
	sql  string
	args []any
}

type fakeExecutor struct {
	row     fakeRow
	rows    *fakeRows
	tag     pgconn.CommandTag
	execErr error
	queryEr error

	queries []recordedQuery
}

func (f *fakeExecutor) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.queries = append(f.queries, recordedQuery{sql: sql, args: args})
	if f.execErr != nil {
		return pgconn.CommandTag{}, f.execErr
	}
	return f.tag, nil
}

func (f *fakeExecutor) Query(_ context.Context, sql string, args ...any) (Rows, error) {
	f.queries = append(f.queries, recordedQuery{sql: sql, args: args})
	if f.queryEr != nil {
		return nil, f.queryEr
	}
	if f.rows == nil {
		return &fakeRows{}, nil
	}
	return f.rows, nil
}

func (f *fakeExecutor) QueryRow(_ context.Context, sql string, args ...any) Row {
	f.queries = append(f.queries, recordedQuery{sql: sql, args: args})
	return f.row
}

func userValues(role, status string) []any {
	return []any{fakeID, "ada@example.com", "Ada Lovelace", "+15550100", role, status, "$2b$10$hash", fakeCreatedAt, fakeCreatedAt}
}

func TestScanUser(t *testing.T) {
	driverFailure := errors.New("connection closed")

	cases := []struct {
		name    string
		row     fakeRow
		wantID  string
		wantErr error
	}{
		{
			name:   "doctor",
			row:    fakeRow{values: userValues("doctor", "active")},
			wantID: fakeID,
		},
		{
			name:   "disabled admin",
			row:    fakeRow{values: userValues("admin", "disabled")},
			wantID: fakeID,
		},
		{
			name:    "driver error passes through as a scan failure",
			row:     fakeRow{err: driverFailure},
			wantErr: driverFailure,
		},
		{
			name:    "unknown stored role is a data fault, not client input",
			row:     fakeRow{values: userValues("receptionist", "active")},
			wantErr: domain.ErrConflict,
		},
		{
			name:    "empty stored role",
			row:     fakeRow{values: userValues("", "active")},
			wantErr: domain.ErrConflict,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user, err := scanUser(tc.row)

			if tc.wantErr != nil {
				if err == nil || !errors.Is(err, tc.wantErr) {
					t.Fatalf("scanUser() error = %v, want %v", err, tc.wantErr)
				}
				if user != (domain.User{}) {
					t.Errorf("scanUser() = %+v alongside an error", user)
				}
				return
			}
			if err != nil {
				t.Fatalf("scanUser() error = %v", err)
			}
			if user.ID != tc.wantID {
				t.Errorf("ID = %q, want %q", user.ID, tc.wantID)
			}
			if user.CreatedAt != fakeCreatedAt {
				t.Errorf("CreatedAt = %v, want %v", user.CreatedAt, fakeCreatedAt)
			}
		})
	}
}

func TestScanUserColumnCountMatchesTheProjection(t *testing.T) {
	// userColumns and scanUser are maintained by hand, so a drift must fail here rather than at
	// runtime against a live server.
	const want = 9

	cols := strings.Split(userColumns, ",")
	if len(cols) != want {
		t.Fatalf("userColumns has %d entries, want %d", len(cols), want)
	}

	var scanned []string
	for _, col := range cols {
		scanned = append(scanned, strings.TrimSpace(col))
	}
	wantCols := []string{"id", "email", "full_name", "phone", "role", "status", "password_hash", "created_at", "updated_at"}
	if !reflect.DeepEqual(scanned, wantCols) {
		t.Errorf("userColumns = %v, want %v", scanned, wantCols)
	}

	if _, err := scanUser(fakeRow{values: userValues("patient", "active")[:8]}); err == nil {
		t.Error("scanUser accepted a short row, so the projection and the mapper disagree")
	}
}

func TestUsersCreateBindsTheNormalisedFields(t *testing.T) {
	db := &fakeExecutor{row: fakeRow{values: userValues("patient", "active")}}
	repo := NewUsers(db)

	user, err := repo.Create(context.Background(), domain.User{
		Email:        "ada@example.com",
		FullName:     "Ada Lovelace",
		Phone:        "+15550100",
		Role:         domain.RolePatient,
		Status:       domain.StatusActive,
		PasswordHash: "$2b$10$hash",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if user.ID != fakeID {
		t.Errorf("ID = %q, want the server-generated uuid", user.ID)
	}

	if len(db.queries) != 1 {
		t.Fatalf("issued %d queries, want 1", len(db.queries))
	}
	want := []any{"ada@example.com", "Ada Lovelace", "+15550100", "patient", "active", "$2b$10$hash"}
	if !reflect.DeepEqual(db.queries[0].args, want) {
		t.Errorf("args = %v, want %v", db.queries[0].args, want)
	}
	assertNoInterpolation(t, db.queries[0].sql)
}

func TestUsersLookupsRejectMalformedIDs(t *testing.T) {
	cases := []struct {
		name string
		id   string
	}{
		{name: "empty", id: ""},
		{name: "sql injection attempt", id: "1; DROP TABLE users"},
		{name: "quoted uuid", id: "'0b8f5c1a-2d3e-4a5b-8c9d-0e1f2a3b4c5d'"},
		{name: "wrong length", id: "0b8f5c1a-2d3e-4a5b-8c9d-0e1f2a3b4c5"},
		{name: "dash in the wrong place", id: "0b8f5c1a2d3e-4a5b-8c9d-0e1f2a3b4c5d"},
		{name: "non hex", id: "zzzzzzzz-2d3e-4a5b-8c9d-0e1f2a3b4c5d"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeExecutor{row: fakeRow{values: userValues("admin", "active")}}
			repo := NewUsers(db)

			if _, err := repo.ByID(context.Background(), tc.id); !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("ByID(%q) error = %v, want %v", tc.id, err, domain.ErrInvalidInput)
			}
			if _, err := repo.Update(context.Background(), domain.User{ID: tc.id}); !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("Update(%q) error = %v, want %v", tc.id, err, domain.ErrInvalidInput)
			}
			if len(db.queries) != 0 {
				t.Errorf("issued %d queries, want the id rejected before the database is touched", len(db.queries))
			}
		})
	}
}

func TestUsersByEmailMapsNoRowsToNotFound(t *testing.T) {
	db := &fakeExecutor{row: fakeRow{err: pgx.ErrNoRows}}
	repo := NewUsers(db)

	_, err := repo.ByEmail(context.Background(), "ghost@example.com")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ByEmail() error = %v, want %v", err, domain.ErrNotFound)
	}
	if !strings.Contains(db.queries[0].sql, "email = $1") {
		t.Errorf("sql = %q, want a bound parameter for the email", db.queries[0].sql)
	}
}

func TestUsersCreateMapsUniqueViolationToAlreadyExists(t *testing.T) {
	db := &fakeExecutor{row: fakeRow{err: &pgconn.PgError{Code: codeUniqueViolation, ConstraintName: "users_email_unique"}}}
	repo := NewUsers(db)

	_, err := repo.Create(context.Background(), domain.User{Email: "dup@example.com"})
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("Create() error = %v, want %v", err, domain.ErrAlreadyExists)
	}
}

func TestTranslate(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name     string
		err      error
		wantIs   error
		wantSame bool
	}{
		{name: "nil stays nil", err: nil},
		{name: "no rows", err: pgx.ErrNoRows, wantIs: domain.ErrNotFound},
		{name: "unique violation", err: &pgconn.PgError{Code: codeUniqueViolation}, wantIs: domain.ErrAlreadyExists},
		{name: "foreign key", err: &pgconn.PgError{Code: codeForeignKeyViolation}, wantIs: domain.ErrConflict},
		{name: "overlapping booking", err: &pgconn.PgError{Code: codeExclusionViolation}, wantIs: domain.ErrConflict},
		{name: "bad uuid text", err: &pgconn.PgError{Code: codeInvalidTextRepresentation}, wantIs: domain.ErrInvalidInput},
		{name: "client disconnect", err: context.Canceled, wantSame: true},
		{name: "deadline", err: context.DeadlineExceeded, wantSame: true},
		{name: "cancelled context", err: canceled.Err(), wantSame: true},
		{name: "unknown server error", err: &pgconn.PgError{Code: "57014", Message: "query was cancelled by administrator"}, wantIs: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := translate(tc.err, "account")

			if tc.err == nil {
				if got != nil {
					t.Fatalf("translate(nil) = %v, want nil", got)
				}
				return
			}
			if tc.wantSame {
				// A cancellation must stay identical, not become a new wrapped error, so the
				// HTTP server can still tell a disconnect from a fault.
				if got != tc.err {
					t.Errorf("translate() = %v, want the original error unchanged", got)
				}
				return
			}
			if tc.wantIs == nil {
				if !strings.Contains(got.Error(), "repository: account") {
					t.Errorf("translate() = %v, want it labelled as a repository failure", got)
				}
				if errors.Is(got, domain.ErrNotFound) || errors.Is(got, domain.ErrAlreadyExists) {
					t.Errorf("translate() = %v, must not map an unrelated server error to a sentinel", got)
				}
				return
			}
			if !errors.Is(got, tc.wantIs) {
				t.Errorf("translate() = %v, want errors.Is(err, %v)", got, tc.wantIs)
			}
		})
	}
}

func TestUsersListBindsFiltersAndPages(t *testing.T) {
	cases := []struct {
		name   string
		filter usecase.UserFilter
		want   []any
	}{
		{
			name:   "no filter",
			filter: usecase.UserFilter{Limit: 50},
			want:   []any{"", "", 50, 0},
		},
		{
			name:   "doctors only",
			filter: usecase.UserFilter{Role: domain.RoleDoctor, Limit: 20, Offset: 40},
			want:   []any{"doctor", "", 20, 40},
		},
		{
			name:   "disabled accounts",
			filter: usecase.UserFilter{Status: domain.StatusDisabled, Limit: 5, Offset: 1},
			want:   []any{"", "disabled", 5, 1},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeExecutor{rows: &fakeRows{rows: [][]any{userValues("doctor", "active")}}}
			repo := NewUsers(db)

			listed, err := repo.List(context.Background(), tc.filter)
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if len(listed) != 1 {
				t.Fatalf("List() returned %d users, want 1", len(listed))
			}
			if !reflect.DeepEqual(db.queries[0].args, tc.want) {
				t.Errorf("args = %v, want %v", db.queries[0].args, tc.want)
			}
			assertNoInterpolation(t, db.queries[0].sql)
			if db.rows.closeCnt != 1 {
				t.Error("rows were not closed, so the connection leaks back to the pool dirty")
			}
		})
	}
}

func TestUsersListReturnsAnEmptySliceNotNil(t *testing.T) {
	db := &fakeExecutor{rows: &fakeRows{}}
	repo := NewUsers(db)

	listed, err := repo.List(context.Background(), usecase.UserFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if listed == nil {
		t.Fatal("List() = nil, want an allocated empty page so JSON renders [] not null")
	}
	if len(listed) != 0 {
		t.Errorf("List() = %v, want no rows", listed)
	}
}

func TestUsersListPropagatesRowErrors(t *testing.T) {
	cases := []struct {
		name string
		rows *fakeRows
	}{
		{name: "query interrupted", rows: &fakeRows{rows: [][]any{userValues("admin", "active")}, nextErr: errors.New("server closed the connection")}},
		{name: "short row", rows: &fakeRows{rows: [][]any{{"only-one-value"}}}},
	}

	db := &fakeExecutor{}
	repo := NewUsers(db)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db.rows = tc.rows

			if _, err := repo.List(context.Background(), usecase.UserFilter{}); err == nil {
				t.Fatal("List() = nil error, want the row failure to surface")
			}
			if tc.rows.closeCnt != 1 {
				t.Errorf("Close() called %d times, want exactly 1 even on the error path", tc.rows.closeCnt)
			}
		})
	}
}

func TestUsersHonoursContextCancellation(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	db := &fakeExecutor{row: fakeRow{values: userValues("patient", "active")}}
	repo := NewUsers(db)

	calls := []func() error{
		func() error { _, err := repo.Create(canceled, domain.User{}); return err },
		func() error { _, err := repo.ByID(canceled, fakeID); return err },
		func() error { _, err := repo.ByEmail(canceled, "a@b.co"); return err },
		func() error { _, err := repo.Update(canceled, domain.User{ID: fakeID}); return err },
		func() error { _, err := repo.List(canceled, usecase.UserFilter{}); return err },
	}

	for i, call := range calls {
		if err := call(); !errors.Is(err, context.Canceled) {
			t.Errorf("call %d error = %v, want context.Canceled", i, err)
		}
	}
	if len(db.queries) != 0 {
		t.Errorf("issued %d queries after cancellation, want 0", len(db.queries))
	}
}

// assertNoInterpolation keeps every statement a constant with numbered placeholders. Values
// only ever travel as bound parameters, which is the whole defence against SQL injection.
func assertNoInterpolation(t *testing.T, sql string) {
	t.Helper()

	if strings.Contains(sql, "%s") || strings.Contains(sql, "?") {
		t.Errorf("sql = %q, want numbered placeholders only", sql)
	}
}

func TestRefreshTokenInsertAndLookup(t *testing.T) {
	values := []any{fakeID, fakeID, fakeFamilyID, strings.Repeat("cd", 32), fakeExpiry, fakeCreatedAt, nil, nil}
	db := &fakeExecutor{row: fakeRow{values: values}}
	repo := NewRefreshTokens(db)

	token, err := repo.Insert(context.Background(), domain.RefreshToken{
		UserID:    fakeID,
		FamilyID:  fakeFamilyID,
		TokenHash: strings.Repeat("cd", 32),
		ExpiresAt: fakeExpiry,
		CreatedAt: fakeCreatedAt,
	})
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if token.UsedAt != nil || token.RevokedAt != nil {
		t.Errorf("UsedAt = %v, RevokedAt = %v, want both NULL on a fresh token", token.UsedAt, token.RevokedAt)
	}

	byHash := strings.Repeat("cd", 32)

	lookup, err := repo.ByHash(context.Background(), byHash)
	if err != nil {
		t.Fatalf("ByHash() error = %v", err)
	}
	if lookup.ID != fakeID {
		t.Errorf("ID = %q, want the server-generated uuid", lookup.ID)
	}
	if len(db.queries) != 2 {
		t.Fatalf("issued %d statements, want an insert and a lookup", len(db.queries))
	}
	if db.queries[1].args[0] != byHash {
		t.Errorf("ByHash args = %v, want the digest bound as a parameter", db.queries[1].args)
	}
	assertNoInterpolation(t, db.queries[0].sql)
	assertNoInterpolation(t, db.queries[1].sql)
}

func TestScanTokenReadsNullableTimestamps(t *testing.T) {
	used := fakeCreatedAt.Add(time.Minute)

	cases := []struct {
		name        string
		values      []any
		wantUsed    *time.Time
		wantRevoked *time.Time
	}{
		{name: "fresh", values: []any{fakeID, fakeID, fakeFamilyID, "hash", fakeExpiry, fakeCreatedAt, nil, nil}},
		{name: "consumed", values: []any{fakeID, fakeID, fakeFamilyID, "hash", fakeExpiry, fakeCreatedAt, used, nil}, wantUsed: &used},
		{name: "revoked", values: []any{fakeID, fakeID, fakeFamilyID, "hash", fakeExpiry, fakeCreatedAt, nil, used}, wantRevoked: &used},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token, err := scanToken(fakeRow{values: tc.values})
			if err != nil {
				t.Fatalf("scanToken() error = %v", err)
			}
			if !equalTimePtr(token.UsedAt, tc.wantUsed) {
				t.Errorf("UsedAt = %v, want %v", token.UsedAt, tc.wantUsed)
			}
			if !equalTimePtr(token.RevokedAt, tc.wantRevoked) {
				t.Errorf("RevokedAt = %v, want %v", token.RevokedAt, tc.wantRevoked)
			}
		})
	}
}

func equalTimePtr(a, b *time.Time) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Equal(*b)
	}
}

func TestRefreshTokenConsumeReportsTheAtomicResult(t *testing.T) {
	cases := []struct {
		name    string
		tag     string
		want    bool
		execErr error
		wantErr error
	}{
		{name: "one row means this caller won the token", tag: "UPDATE 1", want: true},
		{name: "zero rows means it was already consumed", tag: "UPDATE 0", want: false},
		{name: "driver failure", execErr: errors.New("deadlock detected"), wantErr: errors.New("deadlock detected")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeExecutor{tag: pgconn.NewCommandTag(tc.tag), execErr: tc.execErr}
			repo := NewRefreshTokens(db)

			consumed, err := repo.Consume(context.Background(), fakeID, fakeCreatedAt)

			if tc.wantErr != nil {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr.Error()) {
					t.Fatalf("Consume() error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Consume() error = %v", err)
			}
			if consumed != tc.want {
				t.Errorf("Consume() = %v, want %v", consumed, tc.want)
			}
			if !strings.Contains(db.queries[0].sql, "used_at IS NULL") {
				t.Error("the consume statement lost its compare-and-set guard")
			}
		})
	}
}

func TestRefreshTokenConsumeRejectsMalformedID(t *testing.T) {
	db := &fakeExecutor{tag: pgconn.NewCommandTag("UPDATE 1")}
	repo := NewRefreshTokens(db)

	if _, err := repo.Consume(context.Background(), "1 OR 1=1", fakeCreatedAt); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("Consume() error = %v, want %v", err, domain.ErrInvalidInput)
	}
	if len(db.queries) != 0 {
		t.Error("a malformed id must never reach the database")
	}
}

func TestRefreshTokenRevocationsBindTheFamilyAndAccount(t *testing.T) {
	db := &fakeExecutor{tag: pgconn.NewCommandTag("UPDATE 3")}
	repo := NewRefreshTokens(db)

	if err := repo.RevokeFamily(context.Background(), fakeFamilyID, fakeCreatedAt); err != nil {
		t.Fatalf("RevokeFamily() error = %v", err)
	}
	if err := repo.RevokeAllForUser(context.Background(), fakeID, fakeCreatedAt); err != nil {
		t.Fatalf("RevokeAllForUser() error = %v", err)
	}

	if len(db.queries) != 2 {
		t.Fatalf("issued %d statements, want 2", len(db.queries))
	}
	if !reflect.DeepEqual(db.queries[0].args, []any{fakeFamilyID, fakeCreatedAt}) {
		t.Errorf("family args = %v", db.queries[0].args)
	}
	if !reflect.DeepEqual(db.queries[1].args, []any{fakeID, fakeCreatedAt}) {
		t.Errorf("user args = %v", db.queries[1].args)
	}
	for _, statement := range db.queries {
		if !strings.Contains(statement.sql, "revoked_at IS NULL") {
			t.Errorf("sql = %q, want the revoke to stay idempotent and cheap", statement.sql)
		}
		assertNoInterpolation(t, statement.sql)
	}
}

func TestRefreshTokenRevokeAllRejectsMalformedUserID(t *testing.T) {
	db := &fakeExecutor{tag: pgconn.NewCommandTag("UPDATE 0")}
	repo := NewRefreshTokens(db)

	if err := repo.RevokeAllForUser(context.Background(), "not-a-uuid", fakeCreatedAt); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("error = %v, want %v", err, domain.ErrInvalidInput)
	}
	if len(db.queries) != 0 {
		t.Error("a malformed id must never reach the database")
	}
}

func TestRefreshTokenInsertRejectsMalformedUserID(t *testing.T) {
	db := &fakeExecutor{row: fakeRow{values: []any{fakeID, fakeID, fakeFamilyID, "hash", fakeExpiry, fakeCreatedAt, nil, nil}}}
	repo := NewRefreshTokens(db)

	if _, err := repo.Insert(context.Background(), domain.RefreshToken{UserID: "Robert'); DROP TABLE"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("error = %v, want %v", err, domain.ErrInvalidInput)
	}
	if len(db.queries) != 0 {
		t.Error("a malformed id must never reach the database")
	}
}

func TestRefreshTokensHonourContextCancellation(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	db := &fakeExecutor{row: fakeRow{values: []any{fakeID, fakeID, fakeFamilyID, "hash", fakeExpiry, fakeCreatedAt, nil, nil}}, tag: pgconn.NewCommandTag("UPDATE 1")}
	repo := NewRefreshTokens(db)

	calls := []func() error{
		func() error { _, err := repo.Insert(canceled, domain.RefreshToken{UserID: fakeID}); return err },
		func() error { _, err := repo.ByHash(canceled, "hash"); return err },
		func() error { _, err := repo.Consume(canceled, fakeID, fakeCreatedAt); return err },
		func() error { return repo.RevokeFamily(canceled, fakeFamilyID, fakeCreatedAt) },
		func() error { return repo.RevokeAllForUser(canceled, fakeID, fakeCreatedAt) },
	}

	for i, call := range calls {
		if err := call(); !errors.Is(err, context.Canceled) {
			t.Errorf("call %d error = %v, want context.Canceled", i, err)
		}
	}
	if len(db.queries) != 0 {
		t.Errorf("issued %d statements after cancellation, want 0", len(db.queries))
	}
}

func TestIsUUID(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{value: fakeID, want: true},
		{value: strings.ToUpper(fakeID), want: true},
		{value: "", want: false},
		{value: fakeID[:35], want: false},
		{value: fakeID + "0", want: false},
		{value: "0b8f5c1a2d3e4a5b8c9d0e1f2a3b4c5d", want: false},
		{value: "0b8f5c1a-2d3e-4a5b-8c9d-0e1f2a3b4c5e ", want: false},
		{value: "0b8f5c1a-2d3e-4a5b-8c9d-0e1f2a3b4c5g", want: false},
		{value: "0b8f5c1a-2d3e-4a5b-8c9d-0e1f2a3b4c5ä", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			if got := isUUID(tc.value); got != tc.want {
				t.Errorf("isUUID(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
