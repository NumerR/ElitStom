package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/platform/logger"
)

type memoryUsers struct {
	byEmail map[string]domain.User
	failOn  string
}

func (m *memoryUsers) Create(_ context.Context, user domain.User) (domain.User, error) {
	if user.Email == m.failOn {
		return domain.User{}, errors.New("connection reset")
	}
	if _, ok := m.byEmail[user.Email]; ok {
		return domain.User{}, fmt.Errorf("%w: email already exists", domain.ErrAlreadyExists)
	}
	user.ID = fmt.Sprintf("id-%d", len(m.byEmail)+1)
	m.byEmail[user.Email] = user
	return user, nil
}

type prefixHasher struct{ err error }

func (h prefixHasher) Hash(password string) (string, error) {
	if h.err != nil {
		return "", h.err
	}
	return "hashed:" + password, nil
}

func TestSeed(t *testing.T) {
	cases := []struct {
		name        string
		existing    []string
		failOn      string
		password    string
		hasherErr   error
		wantCreated int
		wantErr     string
	}{
		{name: "creates every demo account on an empty database", password: defaultPassword, wantCreated: len(demoAccounts)},
		{name: "skips accounts that already exist", existing: []string{"admin@dentacore.local", "patient@dentacore.local"}, password: defaultPassword, wantCreated: len(demoAccounts) - 2},
		{name: "rejects a weak password before hashing", password: "short", wantErr: "password"},
		{name: "surfaces a hashing failure", password: defaultPassword, hasherErr: errors.New("bcrypt broke"), wantErr: "hash password: bcrypt broke"},
		{name: "stops at a storage failure and reports progress", failOn: "petrova@dentacore.local", password: defaultPassword, wantCreated: 2, wantErr: "create petrova@dentacore.local: connection reset"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &memoryUsers{byEmail: map[string]domain.User{}, failOn: tc.failOn}
			for _, email := range tc.existing {
				store.byEmail[email] = domain.User{Email: email, PasswordHash: "kept"}
			}

			created, err := seed(context.Background(), store, prefixHasher{err: tc.hasherErr}, demoAccounts, tc.password, logger.NewNop())

			assert.Equal(t, tc.wantCreated, created)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			for _, email := range tc.existing {
				assert.Equal(t, "kept", store.byEmail[email].PasswordHash, "re-seeding must not reset an existing password")
			}
		})
	}
}

func TestDemoAccountsAreValidAndCoverEveryRole(t *testing.T) {
	roles := map[domain.Role]int{}
	for _, a := range demoAccounts {
		_, err := domain.NewUser(domain.UserParams{Email: a.Email, FullName: a.FullName, Phone: a.Phone, Role: a.Role, PasswordHash: "x"})
		require.NoErrorf(t, err, "demo account %s is invalid", a.Email)
		roles[a.Role]++
	}
	assert.Equal(t, map[domain.Role]int{domain.RoleAdmin: 1, domain.RoleDoctor: 3, domain.RolePatient: 2}, roles)
}
