package domain

import (
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type UserStatus string

const (
	StatusActive   UserStatus = "active"
	StatusDisabled UserStatus = "disabled"
)

func ParseUserStatus(raw string) (UserStatus, error) {
	switch status := UserStatus(strings.ToLower(strings.TrimSpace(raw))); status {
	case StatusActive, StatusDisabled:
		return status, nil
	default:
		return "", NewValidationError("status", "must be one of active, disabled")
	}
}

const (
	// bcrypt silently hashes only the first 72 bytes, so a longer password must be
	// rejected outright instead of being quietly weakened to its prefix.
	MaxPasswordBytes = 72

	minPasswordRunes = 12
	minFullNameRunes = 2
	maxFullNameRunes = 100
	maxEmailRunes    = 254
	maxPhoneRunes    = 32
	minPhoneDigits   = 7
	maxPhoneDigits   = 15
)

// User is a clinic account. PasswordHash is never serialised to a client:
// delivery layers must map this entity onto an explicit view DTO first.
type User struct {
	ID           string
	Email        string
	FullName     string
	Phone        string
	Role         Role
	Status       UserStatus
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type UserParams struct {
	Email        string
	FullName     string
	Phone        string
	Role         Role
	PasswordHash string
}

// NewUser returns a validated, immediately usable user. A constructed user is never
// half-filled: any problem is reported as *ValidationError before allocation.
func NewUser(p UserParams) (*User, error) {
	fields, ve := validateAccount(p.Email, p.FullName, p.Phone, p.Role)
	if p.PasswordHash == "" {
		if ve == nil {
			ve = &ValidationError{}
		}
		ve.Add("password", "must be hashed before the user is created")
	}
	if ve != nil {
		return nil, ve
	}

	return &User{
		Email:        fields.Email,
		FullName:     fields.FullName,
		Phone:        fields.Phone,
		Role:         fields.Role,
		Status:       StatusActive,
		PasswordHash: p.PasswordHash,
	}, nil
}

// ValidateAccount normalises and checks the identity fields without touching the password.
// A service uses this before hashing so bcrypt is never paid for a request that cannot
// succeed — at cost 12 an unvalidated signup is a cheap way to burn server CPU.
func ValidateAccount(email, fullName, phone string, role Role) error {
	_, ve := validateAccount(email, fullName, phone, role)
	if ve != nil {
		return ve
	}
	return nil
}

type accountFields struct {
	Email    string
	FullName string
	Phone    string
	Role     Role
}

// validateAccount returns nil, nil when everything is valid, otherwise the accumulated errors.
func validateAccount(email, fullName, phone string, role Role) (accountFields, *ValidationError) {
	ve := &ValidationError{}

	fields := accountFields{
		Email:    normalizeEmail(email, ve),
		FullName: normalizeFullName(fullName, ve),
		Phone:    normalizePhone(phone, ve),
		Role:     role,
	}
	if !role.IsValid() {
		ve.Add("role", "must be one of admin, doctor, patient")
	}

	if len(ve.Fields) > 0 {
		return accountFields{}, ve
	}
	return fields, nil
}

// UpdateProfile replaces the mutable profile fields, leaving email, role and status alone.
func (u *User) UpdateProfile(fullName, phone string) error {
	ve := &ValidationError{}
	name := normalizeFullName(fullName, ve)
	number := normalizePhone(phone, ve)

	if len(ve.Fields) > 0 {
		return ve
	}
	u.FullName = name
	u.Phone = number
	return nil
}

func (u *User) ChangeRole(role Role) error {
	if !role.IsValid() {
		return NewValidationError("role", "must be one of admin, doctor, patient")
	}
	u.Role = role
	return nil
}

func (u *User) Disable() { u.Status = StatusDisabled }

func (u *User) Enable() { u.Status = StatusActive }

func (u User) IsDisabled() bool { return u.Status == StatusDisabled }

// ValidatePasswordStrength applies the password policy to plaintext input.
// Only length is enforced: NIST SP 800-63B discourages composition rules because they
// push users toward predictable patterns, and the real protection is bcrypt plus a
// breach-list check, which is out of scope here.
func ValidatePasswordStrength(plain string) error {
	ve := &ValidationError{}
	validatePassword(plain, ve)
	if len(ve.Fields) > 0 {
		return ve
	}
	return nil
}

func validatePassword(plain string, ve *ValidationError) {
	switch {
	case plain == "":
		ve.Add("password", "is required")
		return
	case len(plain) > MaxPasswordBytes:
		ve.Add("password", "must not exceed 72 bytes")
		return
	case utf8.RuneCountInString(plain) < minPasswordRunes:
		ve.Add("password", "must be at least 12 characters")
	}
}

// NormalizeEmail lowercases and validates an address, rejecting anything with display
// names, quoted local parts or non-ASCII characters — the login must match exactly what
// was stored at registration.
func NormalizeEmail(raw string) (string, error) {
	ve := &ValidationError{}
	email := normalizeEmail(raw, ve)
	if len(ve.Fields) > 0 {
		return "", ve
	}
	return email, nil
}

func normalizeEmail(raw string, ve *ValidationError) string {
	email := strings.ToLower(strings.TrimSpace(raw))

	switch {
	case email == "":
		ve.Add("email", "is required")
		return ""
	case utf8.RuneCountInString(email) > maxEmailRunes:
		ve.Add("email", "must be at most 254 characters")
		return ""
	}

	at := strings.LastIndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		ve.Add("email", "must be a valid address")
		return ""
	}

	domainPart := email[at+1:]
	if !strings.Contains(domainPart, ".") || strings.HasPrefix(domainPart, ".") || strings.HasSuffix(domainPart, ".") {
		ve.Add("email", "must have a valid domain")
		return ""
	}

	// net/mail accepts 8-bit local parts, so the ASCII rule has to be explicit: a Cyrillic "а"
	// in an address is indistinguishable from a Latin one to a human reader, which is exactly
	// what an impersonation attempt relies on. Internationalised addresses would need a
	// lowercase+punycode comparison in the login path to be safe, and that is not implemented.
	for _, r := range email {
		if r > unicode.MaxASCII {
			ve.Add("email", "must contain only ASCII characters")
			return ""
		}
	}

	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Name != "" || addr.Address != email {
		ve.Add("email", "must be a valid address")
		return ""
	}
	return email
}

func normalizeFullName(raw string, ve *ValidationError) string {
	name := strings.Join(strings.Fields(raw), " ")

	switch {
	case name == "":
		ve.Add("full_name", "is required")
		return ""
	case utf8.RuneCountInString(name) < minFullNameRunes:
		ve.Add("full_name", "must be at least 2 characters")
		return ""
	case utf8.RuneCountInString(name) > maxFullNameRunes:
		ve.Add("full_name", "must be at most 100 characters")
		return ""
	}

	for _, r := range name {
		if unicode.IsLetter(r) || r == ' ' || r == '-' || r == '.' || r == '\'' || r == '’' {
			continue
		}
		ve.Add("full_name", "must contain only letters, spaces, hyphens and apostrophes")
		return ""
	}
	return name
}

// normalizePhone strips formatting separators and keeps an E.164-style value.
// An empty input is valid: the phone is optional on a profile.
func normalizePhone(raw string, ve *ValidationError) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	var builder strings.Builder
	digits := 0
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			digits++
		case r == '+' || r == '-' || r == '(' || r == ')' || r == ' ' || r == '.':
			if r == '+' {
				if builder.Len() != 0 {
					ve.Add("phone", "may only start with a plus sign")
					return ""
				}
				builder.WriteRune(r)
			}
		default:
			ve.Add("phone", "must contain only digits, spaces, hyphens, dots and parentheses")
			return ""
		}
	}

	phone := builder.String()
	switch {
	case digits < minPhoneDigits:
		ve.Add("phone", "must contain at least 7 digits")
		return ""
	case digits > maxPhoneDigits:
		ve.Add("phone", "must contain at most 15 digits")
		return ""
	case utf8.RuneCountInString(phone) > maxPhoneRunes:
		ve.Add("phone", "must be at most 32 characters")
		return ""
	}
	return phone
}
