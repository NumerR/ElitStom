package usecase

import (
	"context"
	"time"

	"github.com/NumerR/ElitStom/internal/domain"
)

// Actor identifies the authenticated principal behind a request. Transport builds it from
// a verified access token — never from a request body or header the client controls.
type Actor struct {
	UserID string
	Role   domain.Role
}

// Clock makes time injectable so expiry and rotation are testable without sleeping.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production clock. UTC everywhere keeps Go's clock and Postgres
// timestamptz comparable.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

// TokenIssuer mints and verifies short-lived access tokens.
type TokenIssuer interface {
	IssueAccessToken(userID string, role domain.Role) (token string, expiresAt time.Time, err error)
	ParseAccessToken(raw string) (Actor, error)
}

// PasswordHasher protects stored credentials. DummyHash exists so a login for an unknown
// account can burn the same CPU time as one for a known account.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(hash, password string) error
	DummyHash() string
}

// SecretGenerator mints opaque refresh-token secrets and the digests that get stored.
type SecretGenerator interface {
	NewRefreshToken() (string, error)
	NewFamilyID() (string, error)
	HashToken(token string) string
}

// UserFilter narrows an admin listing. Zero Limit means "use the default", not "no rows".
type UserFilter struct {
	Role   domain.Role
	Status domain.UserStatus
	Limit  int
	Offset int
}

// UserRepository is the account persistence port. Update writes only the mutable fields
// (profile, role, status, password hash) and never email or created_at.
type UserRepository interface {
	Create(ctx context.Context, user domain.User) (domain.User, error)
	ByID(ctx context.Context, id string) (domain.User, error)
	ByEmail(ctx context.Context, email string) (domain.User, error)
	Update(ctx context.Context, user domain.User) (domain.User, error)
	List(ctx context.Context, filter UserFilter) ([]domain.User, error)
}

// RefreshTokenStore keeps refresh-token digests. Nothing here returns a stored token by
// user id: the plaintext exists only in the response that minted it.
type RefreshTokenStore interface {
	Insert(ctx context.Context, token domain.RefreshToken) (domain.RefreshToken, error)

	// ByHash returns domain.ErrNotFound when the digest is unknown.
	ByHash(ctx context.Context, hash string) (domain.RefreshToken, error)

	// Consume atomically marks a token used. It reports false when another request already
	// consumed it, which is how concurrent replay of the same token is detected.
	Consume(ctx context.Context, id string, at time.Time) (bool, error)

	RevokeFamily(ctx context.Context, familyID string, at time.Time) error
	RevokeAllForUser(ctx context.Context, userID string, at time.Time) error
}

// AppointmentFilter narrows a listing. Empty ids and zero times mean "no predicate"; From and
// To select appointments that overlap [From, To), so a visit running across From still shows.
type AppointmentFilter struct {
	PatientID  string
	DoctorID   string
	Status     domain.AppointmentStatus
	From       time.Time
	To         time.Time
	Descending bool
	Limit      int
	Offset     int
}

// StatusChange is a compare-and-set: it applies only while the stored status is still From.
type StatusChange struct {
	ID           string
	From         domain.AppointmentStatus
	To           domain.AppointmentStatus
	CancelReason string
}

// RescheduleChange moves an appointment and resets it to scheduled, but only while it still has
// the status and start time the caller read — two concurrent moves cannot both win.
type RescheduleChange struct {
	ID           string
	From         domain.AppointmentStatus
	FromStartsAt time.Time
	StartsAt     time.Time
	EndsAt       time.Time
}

// AppointmentRepository is the booking persistence port. Create and Reschedule return
// domain.ErrConflict when the slot overlaps a live booking of the same doctor or patient;
// Transition and Reschedule return it when the row changed since it was read.
type AppointmentRepository interface {
	Create(ctx context.Context, appt domain.Appointment) (domain.Appointment, error)
	ByID(ctx context.Context, id string) (domain.Appointment, error)
	List(ctx context.Context, filter AppointmentFilter) ([]domain.Appointment, error)
	Transition(ctx context.Context, change StatusChange) (domain.Appointment, error)
	Reschedule(ctx context.Context, change RescheduleChange) (domain.Appointment, error)

	// Busy lists the intervals a doctor's live bookings occupy within window.
	Busy(ctx context.Context, doctorID string, window domain.Interval) ([]domain.Interval, error)
}
