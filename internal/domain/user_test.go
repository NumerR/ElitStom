package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewUser(t *testing.T) {
	cases := []struct {
		name    string
		params  UserParams
		wantErr string
		check   func(*testing.T, *User)
	}{
		{
			name:   "valid patient",
			params: UserParams{Email: "Ada@Example.COM", FullName: "  Ada   Lovelace ", Phone: "+1 (555) 010-0", Role: RolePatient, PasswordHash: "bcrypt"},
			check: func(t *testing.T, u *User) {
				t.Helper()
				if u.Email != "ada@example.com" {
					t.Errorf("Email = %q, want normalised lower-case", u.Email)
				}
				if u.FullName != "Ada Lovelace" {
					t.Errorf("FullName = %q, want collapsed whitespace", u.FullName)
				}
				if u.Phone != "+15550100" {
					t.Errorf("Phone = %q, want digits only", u.Phone)
				}
				if u.Status != StatusActive {
					t.Errorf("Status = %q, a new account must start active", u.Status)
				}
				if u.ID != "" {
					t.Errorf("ID = %q, the repository assigns the id", u.ID)
				}
			},
		},
		{
			name:    "missing everything",
			params:  UserParams{},
			wantErr: "email: is required",
		},
		{
			name:    "unhashed password",
			params:  UserParams{Email: "a@b.co", FullName: "Ab", Role: RolePatient},
			wantErr: "password: must be hashed before the user is created",
		},
		{
			name:    "unknown role",
			params:  UserParams{Email: "a@b.co", FullName: "Ab", Role: Role("receptionist"), PasswordHash: "bcrypt"},
			wantErr: "role: must be one of",
		},
		{
			name:    "empty role",
			params:  UserParams{Email: "a@b.co", FullName: "Ab", PasswordHash: "bcrypt"},
			wantErr: "role: must be one of",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user, err := NewUser(tc.params)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("NewUser() = nil error, want %q", tc.wantErr)
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("NewUser() error = %v, want it to satisfy errors.Is(err, ErrInvalidInput)", err)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("NewUser() error = %q, want it to contain %q", err, tc.wantErr)
				}
				if user != nil {
					t.Errorf("NewUser() returned %+v alongside an error, want nil", *user)
				}
				return
			}

			if err != nil {
				t.Fatalf("NewUser() error = %v", err)
			}
			tc.check(t, user)
		})
	}
}

func TestNewUserReportsEveryFieldProblemAtOnce(t *testing.T) {
	_, err := NewUser(UserParams{Email: "no-at-sign", FullName: "", Phone: "abc", PasswordHash: "bcrypt"})

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %T, want *ValidationError", err)
	}
	want := map[string]bool{"email": false, "full_name": false, "phone": false}
	for _, field := range ve.Fields {
		if _, ok := want[field.Field]; ok {
			want[field.Field] = true
		}
	}
	for field, seen := range want {
		if !seen {
			t.Errorf("validation did not report %q; fields = %v", field, ve.Details())
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "lower-cased and trimmed", raw: "  Ada.L@Example.COM  ", want: "ada.l@example.com"},
		{name: "plus addressing survives", raw: "ada+clinic@example.com", want: "ada+clinic@example.com"},
		{name: "empty", raw: "", wantErr: "email: is required"},
		{name: "no at sign", raw: "ada.example.com", wantErr: "email: must be a valid address"},
		{name: "no local part", raw: "@example.com", wantErr: "email: must be a valid address"},
		{name: "no domain part", raw: "ada@", wantErr: "email: must be a valid address"},
		{name: "domain without a dot", raw: "ada@localhost", wantErr: "email: must have a valid domain"},
		{name: "domain ends with a dot", raw: "ada@example.com.", wantErr: "email: must have a valid domain"},
		{name: "domain starts with a dot", raw: "ada@.example.com", wantErr: "email: must have a valid domain"},
		{name: "two at signs", raw: "ada@bad@example.com", wantErr: "email: must be a valid address"},
		{name: "display name is rejected", raw: "Ada Lovelace <ada@example.com>", wantErr: "email: must be a valid address"},
		{name: "quoted local part is rejected", raw: `"ada"@example.com`, wantErr: "email: must be a valid address"},
		{name: "space inside is rejected", raw: "ada l@example.com", wantErr: "email: must be a valid address"},
		{name: "non-ascii local part is rejected", raw: "ада@example.com", wantErr: "email: must contain only ASCII characters"},
		{name: "non-ascii domain is rejected", raw: "ada@exämple.com", wantErr: "email: must contain only ASCII characters"},
		{name: "over the RFC length", raw: strings.Repeat("a", 250) + "@example.com", wantErr: "email: must be at most 254 characters"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeEmail(tc.raw)

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("NormalizeEmail(%q) error = %v, want %q", tc.raw, err, tc.wantErr)
				}
				if got != "" {
					t.Errorf("NormalizeEmail(%q) = %q, want empty on failure", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeEmail(%q) error = %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeEmail(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestNormalizeFullName(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "inner whitespace collapses", raw: "Jean\tLuc  Picard", want: "Jean Luc Picard"},
		{name: "hyphen and apostrophe", raw: "Anne-Marie O’Neil", want: "Anne-Marie O’Neil"},
		{name: "period for a suffix", raw: "Dr. Who", want: "Dr. Who"},
		{name: "unicode letters", raw: "Ірина Коваль", want: "Ірина Коваль"},
		{name: "too short", raw: "A", wantErr: true},
		{name: "digits", raw: "Ada 42", wantErr: true},
		{name: "email injected", raw: "ada@example.com", wantErr: true},
		{name: "over the limit", raw: strings.Repeat("a", 101), wantErr: true},
		{name: "exactly at the limit", raw: strings.Repeat("a", 100), want: strings.Repeat("a", 100)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ve := &ValidationError{}
			got := normalizeFullName(tc.raw, ve)

			if tc.wantErr {
				if len(ve.Fields) == 0 {
					t.Fatalf("normalizeFullName(%q) accepted invalid input", tc.raw)
				}
				if got != "" {
					t.Errorf("normalizeFullName(%q) = %q, want empty on failure", tc.raw, got)
				}
				return
			}
			if len(ve.Fields) > 0 {
				t.Fatalf("normalizeFullName(%q) rejected valid input: %v", tc.raw, ve.Details())
			}
			if got != tc.want {
				t.Errorf("normalizeFullName(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "empty is allowed", raw: "", want: ""},
		{name: "whitespace only is allowed", raw: "   ", want: ""},
		{name: "international with formatting", raw: "+1 (555) 010-0123", want: "+15550100123"},
		{name: "dots are separators", raw: "555.010.0123", want: "5550100123"},
		{name: "plus must come first", raw: "555+010", wantErr: "phone: may only start with a plus sign"},
		{name: "letters are rejected", raw: "555-CALL-US", wantErr: "phone: must contain only digits"},
		{name: "too few digits", raw: "5550", wantErr: "phone: must contain at least 7 digits"},
		{name: "too many digits", raw: "1234567890123456", wantErr: "phone: must contain at most 15 digits"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ve := &ValidationError{}
			got := normalizePhone(tc.raw, ve)

			if tc.wantErr != "" {
				if len(ve.Fields) == 0 {
					t.Fatalf("normalizePhone(%q) accepted invalid input", tc.raw)
				}
				if !strings.Contains(ve.Error(), tc.wantErr) {
					t.Errorf("normalizePhone(%q) error = %q, want it to contain %q", tc.raw, ve, tc.wantErr)
				}
				if got != "" {
					t.Errorf("normalizePhone(%q) = %q, want empty on failure", tc.raw, got)
				}
				return
			}
			if len(ve.Fields) > 0 {
				t.Fatalf("normalizePhone(%q) rejected valid input: %v", tc.raw, ve.Details())
			}
			if got != tc.want {
				t.Errorf("normalizePhone(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	cases := []struct {
		name    string
		plain   string
		wantErr string
	}{
		{name: "long passphrase", plain: "correct horse battery staple"},
		{name: "minimum length", plain: strings.Repeat("k", 12)},
		{name: "one byte short", plain: strings.Repeat("k", 11), wantErr: "password: must be at least 12 characters"},
		{name: "empty", plain: "", wantErr: "password: is required"},
		{name: "exactly the bcrypt limit", plain: strings.Repeat("k", MaxPasswordBytes)},
		{name: "past the bcrypt limit", plain: strings.Repeat("k", MaxPasswordBytes+1), wantErr: "password: must not exceed 72 bytes"},
		{name: "multibyte counted by runes", plain: strings.Repeat("ä", 12)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePasswordStrength(tc.plain)

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ValidatePasswordStrength(%q) error = %v, want %q", tc.plain, err, tc.wantErr)
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("error = %v, want errors.Is(err, ErrInvalidInput)", err)
				}
				return
			}
			if err != nil {
				t.Errorf("ValidatePasswordStrength(%q) error = %v", tc.plain, err)
			}
		})
	}
}

func TestUserMutations(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*testing.T, *User)
		wantErr bool
		check   func(*testing.T, *User)
	}{
		{
			name: "profile update keeps the role",
			mutate: func(t *testing.T, u *User) {
				t.Helper()
				if err := u.UpdateProfile("Jean-Luc Picard", "+15550100123"); err != nil {
					t.Fatalf("UpdateProfile() error = %v", err)
				}
				if u.Role != RoleDoctor {
					t.Errorf("Role = %q, UpdateProfile must not change it", u.Role)
				}
			},
		},
		{
			name: "profile update rejects a bad phone",
			mutate: func(t *testing.T, u *User) {
				t.Helper()
				err := u.UpdateProfile("Valid Name", "not-a-phone")
				if err == nil {
					t.Fatal("UpdateProfile() = nil, want an error")
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("error = %v, want errors.Is(err, ErrInvalidInput)", err)
				}
				if u.FullName == "New Name" {
					t.Error("a rejected update must leave the entity unchanged")
				}
			},
		},
		{
			name: "change role to admin",
			mutate: func(t *testing.T, u *User) {
				t.Helper()
				if err := u.ChangeRole(RoleAdmin); err != nil {
					t.Fatalf("ChangeRole() error = %v", err)
				}
				if u.Role != RoleAdmin {
					t.Errorf("Role = %q, want admin", u.Role)
				}
			},
		},
		{
			name: "change role rejects unknown value",
			mutate: func(t *testing.T, u *User) {
				t.Helper()
				if err := u.ChangeRole(Role("biller")); err == nil {
					t.Fatal("ChangeRole() = nil, want an error")
				}
			},
		},
		{
			name: "disable then enable",
			mutate: func(t *testing.T, u *User) {
				t.Helper()
				u.Disable()
				if !u.IsDisabled() {
					t.Error("IsDisabled() = false after Disable()")
				}
				u.Enable()
				if u.IsDisabled() {
					t.Error("IsDisabled() = true after Enable()")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := &User{Email: "a@b.co", FullName: "Ab", Role: RoleDoctor, Status: StatusActive}
			tc.mutate(t, user)
			if tc.check != nil {
				tc.check(t, user)
			}
		})
	}
}

func TestParseUserStatus(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    UserStatus
		wantErr bool
	}{
		{name: "active", raw: "active", want: StatusActive},
		{name: "disabled upper case", raw: "DISABLED", want: StatusDisabled},
		{name: "padded", raw: "  Active ", want: StatusActive},
		{name: "unknown", raw: "archived", wantErr: true},
		{name: "empty", raw: "", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseUserStatus(tc.raw)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseUserStatus(%q) = %q, want an error", tc.raw, got)
				}
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("error = %v, want errors.Is(err, ErrInvalidInput)", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseUserStatus(%q) error = %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("ParseUserStatus(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestValidateAccount(t *testing.T) {
	cases := []struct {
		name    string
		email   string
		name2   string
		phone   string
		role    Role
		wantErr bool
	}{
		{name: "valid", email: "a@b.co", name2: "Ab", phone: "", role: RoleDoctor},
		{name: "bad email", email: "nope", name2: "Ab", role: RoleDoctor, wantErr: true},
		{name: "bad role", email: "a@b.co", name2: "Ab", role: Role(""), wantErr: true},
		{name: "password is not inspected", email: "a@b.co", name2: "Ab", role: RolePatient},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAccount(tc.email, tc.name2, tc.phone, tc.role)

			if tc.wantErr && err == nil {
				t.Fatal("ValidateAccount() = nil, want an error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateAccount() error = %v", err)
			}
		})
	}
}
