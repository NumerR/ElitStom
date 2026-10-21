package domain

import "strings"

type Role string

const (
	RoleAdmin   Role = "admin"
	RoleDoctor  Role = "doctor"
	RolePatient Role = "patient"
)

// AllRoles is ordered from most to least privileged.
var AllRoles = []Role{RoleAdmin, RoleDoctor, RolePatient}

func ParseRole(raw string) (Role, error) {
	role := Role(strings.ToLower(strings.TrimSpace(raw)))
	if !role.IsValid() {
		return "", NewValidationError("role", "must be one of admin, doctor, patient")
	}
	return role, nil
}

func (r Role) IsValid() bool {
	switch r {
	case RoleAdmin, RoleDoctor, RolePatient:
		return true
	default:
		return false
	}
}

// CanManageUsers reports whether this role may create accounts and change roles.
// Patients and doctors must never reach the admin endpoints.
func (r Role) CanManageUsers() bool { return r == RoleAdmin }

func (r Role) String() string { return string(r) }
