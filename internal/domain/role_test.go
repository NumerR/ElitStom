package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseRole(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    Role
		wantErr bool
	}{
		{name: "admin", raw: "admin", want: RoleAdmin},
		{name: "doctor upper case", raw: "DOCTOR", want: RoleDoctor},
		{name: "padded patient", raw: "  Patient ", want: RolePatient},
		{name: "empty", raw: "", wantErr: true},
		{name: "unknown", raw: "receptionist", wantErr: true},
		{name: "sql keyword", raw: "null", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRole(tc.raw)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseRole(%q) = %q, want an error", tc.raw, got)
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("error = %v, want errors.Is(err, ErrInvalidInput)", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRole(%q) error = %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("ParseRole(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestRoleIsValid(t *testing.T) {
	cases := []struct {
		role Role
		want bool
	}{
		{role: RoleAdmin, want: true},
		{role: RoleDoctor, want: true},
		{role: RolePatient, want: true},
		{role: Role(""), want: false},
		{role: Role("Admin"), want: false},
	}

	for _, tc := range cases {
		t.Run(string(tc.role), func(t *testing.T) {
			if got := tc.role.IsValid(); got != tc.want {
				t.Errorf("Role(%q).IsValid() = %v, want %v", tc.role, got, tc.want)
			}
		})
	}
}

func TestCanManageUsers(t *testing.T) {
	cases := []struct {
		role Role
		want bool
	}{
		{role: RoleAdmin, want: true},
		{role: RoleDoctor, want: false},
		{role: RolePatient, want: false},
		{role: Role(""), want: false},
	}

	for _, tc := range cases {
		t.Run(tc.role.String(), func(t *testing.T) {
			if got := tc.role.CanManageUsers(); got != tc.want {
				t.Errorf("Role(%q).CanManageUsers() = %v, want %v", tc.role, got, tc.want)
			}
		})
	}
}

func TestAllRolesCoversEveryRole(t *testing.T) {
	seen := map[Role]bool{}
	for _, role := range AllRoles {
		if !role.IsValid() {
			t.Errorf("AllRoles contains the invalid role %q", role)
		}
		if seen[role] {
			t.Errorf("AllRoles lists %q twice", role)
		}
		seen[role] = true
	}

	for _, role := range []Role{RoleAdmin, RoleDoctor, RolePatient} {
		if !seen[role] {
			t.Errorf("AllRoles is missing %q", role)
		}
	}
	if strings.TrimSpace(AllRoles[0].String()) != "admin" {
		t.Errorf("AllRoles[0] = %q, want the most privileged role first", AllRoles[0])
	}
}
