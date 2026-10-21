package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/NumerR/ElitStom/internal/domain"
)

// BookInput is a booking request. PatientID is required from an admin booking on someone's
// behalf and must be empty or the caller's own id from a patient. Zero Duration means one slot.
type BookInput struct {
	DoctorID  string
	PatientID string
	StartsAt  time.Time
	Duration  time.Duration
	Reason    string
}

// RescheduleInput moves an appointment. Zero Duration keeps the current length.
type RescheduleInput struct {
	StartsAt time.Time
	Duration time.Duration
}

type AppointmentPage struct {
	Appointments []domain.Appointment
	Limit        int
	Offset       int
}

// Appointments enforces who may book, see and change a visit, and when. The calendar rules
// live in domain.Schedule; double booking is ultimately prevented by the database.
type Appointments struct {
	appts    AppointmentRepository
	users    UserRepository
	schedule domain.Schedule
	clock    Clock
}

func NewAppointments(appts AppointmentRepository, users UserRepository, schedule domain.Schedule, clock Clock) (*Appointments, error) {
	missing := []string{}
	if appts == nil {
		missing = append(missing, "appointment repository")
	}
	if users == nil {
		missing = append(missing, "users repository")
	}
	if schedule.Location() == nil {
		missing = append(missing, "schedule")
	}
	if clock == nil {
		missing = append(missing, "clock")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("usecase: NewAppointments is missing %v", missing)
	}
	return &Appointments{appts: appts, users: users, schedule: schedule, clock: clock}, nil
}

func (a *Appointments) Schedule() domain.Schedule { return a.schedule }

// Doctors lists the doctors a booking may name. Any signed-in role may read it.
func (a *Appointments) Doctors(ctx context.Context, actor Actor) ([]domain.Party, error) {
	if !actor.Role.IsValid() {
		return nil, fmt.Errorf("%w: unknown role", domain.ErrForbidden)
	}
	doctors, err := a.users.List(ctx, UserFilter{Role: domain.RoleDoctor, Status: domain.StatusActive, Limit: maxListLimit})
	if err != nil {
		return nil, err
	}
	parties := make([]domain.Party, 0, len(doctors))
	for _, doctor := range doctors {
		parties = append(parties, domain.Party{ID: doctor.ID, FullName: doctor.FullName})
	}
	slices.SortStableFunc(parties, func(x, y domain.Party) int { return strings.Compare(x.FullName, y.FullName) })
	return parties, nil
}

// FreeSlots lists the slots of one clinic day that a new booking with this doctor could take
// right now: on the grid, inside the booking window, and not overlapping a live booking.
func (a *Appointments) FreeSlots(ctx context.Context, actor Actor, doctorID, date string) ([]domain.Interval, error) {
	if !actor.Role.IsValid() {
		return nil, fmt.Errorf("%w: unknown role", domain.ErrForbidden)
	}
	day, err := a.schedule.ParseDate(date)
	if err != nil {
		return nil, err
	}
	if _, err := a.activeParty(ctx, doctorID, domain.RoleDoctor, "doctor_id"); err != nil {
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			return nil, fmt.Errorf("%w: doctor", domain.ErrNotFound)
		}
		return nil, err
	}

	free := []domain.Interval{}
	slots := a.schedule.Slots(day)
	if len(slots) == 0 {
		return free, nil
	}

	busy, err := a.appts.Busy(ctx, doctorID, a.schedule.Day(day))
	if err != nil {
		return nil, err
	}

	window := a.schedule.BookingWindow(a.clock.Now())
	for _, slot := range slots {
		if !slot.Start.After(window.Start) || !slot.Start.Before(window.End) || overlapsAny(slot, busy) {
			continue
		}
		free = append(free, slot)
	}
	return free, nil
}

func (a *Appointments) Book(ctx context.Context, actor Actor, in BookInput) (domain.Appointment, error) {
	patientID := in.PatientID
	switch actor.Role {
	case domain.RolePatient:
		if patientID != "" && patientID != actor.UserID {
			return domain.Appointment{}, fmt.Errorf("%w: a patient may only book for themselves", domain.ErrForbidden)
		}
		patientID = actor.UserID
	case domain.RoleAdmin:
		if patientID == "" {
			return domain.Appointment{}, domain.NewValidationError("patient_id", "is required")
		}
	default:
		return domain.Appointment{}, fmt.Errorf("%w: only a patient or an admin may book", domain.ErrForbidden)
	}

	reason, err := domain.NormalizeNote("reason", in.Reason)
	if err != nil {
		return domain.Appointment{}, err
	}

	want, err := a.bookable(in.StartsAt, in.Duration, a.schedule.Slot())
	if err != nil {
		return domain.Appointment{}, err
	}

	doctor, err := a.activeParty(ctx, in.DoctorID, domain.RoleDoctor, "doctor_id")
	if err != nil {
		return domain.Appointment{}, err
	}
	patient, err := a.activeParty(ctx, patientID, domain.RolePatient, "patient_id")
	if err != nil {
		return domain.Appointment{}, err
	}

	return a.appts.Create(ctx, domain.Appointment{
		Patient:   patient,
		Doctor:    doctor,
		StartsAt:  want.Start,
		EndsAt:    want.End,
		Status:    domain.AppointmentScheduled,
		Reason:    reason,
		CreatedBy: actor.UserID,
	})
}

// List scopes the listing by role: a patient sees their own visits, a doctor their own
// patients' visits, an admin everything. Asking for someone else's scope is forbidden rather
// than silently rewritten, so a client bug cannot masquerade as an empty result.
func (a *Appointments) List(ctx context.Context, actor Actor, filter AppointmentFilter) (AppointmentPage, error) {
	switch actor.Role {
	case domain.RolePatient:
		if filter.PatientID != "" && filter.PatientID != actor.UserID {
			return AppointmentPage{}, fmt.Errorf("%w: a patient may only list their own appointments", domain.ErrForbidden)
		}
		filter.PatientID = actor.UserID
	case domain.RoleDoctor:
		if filter.DoctorID != "" && filter.DoctorID != actor.UserID {
			return AppointmentPage{}, fmt.Errorf("%w: a doctor may only list their own appointments", domain.ErrForbidden)
		}
		filter.DoctorID = actor.UserID
	case domain.RoleAdmin:
	default:
		return AppointmentPage{}, fmt.Errorf("%w: unknown role", domain.ErrForbidden)
	}

	if filter.Status != "" {
		if _, err := domain.ParseAppointmentStatus(string(filter.Status)); err != nil {
			return AppointmentPage{}, err
		}
	}
	if !filter.From.IsZero() && !filter.To.IsZero() && !filter.To.After(filter.From) {
		return AppointmentPage{}, domain.NewValidationError("to", "must be later than from")
	}
	if filter.Limit <= 0 {
		filter.Limit = defaultListLimit
	}
	if filter.Limit > maxListLimit {
		filter.Limit = maxListLimit
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	appts, err := a.appts.List(ctx, filter)
	if err != nil {
		return AppointmentPage{}, err
	}
	return AppointmentPage{Appointments: appts, Limit: filter.Limit, Offset: filter.Offset}, nil
}

// Get answers ErrNotFound for an appointment the actor may not see, so ids of other people's
// visits cannot be probed.
func (a *Appointments) Get(ctx context.Context, actor Actor, id string) (domain.Appointment, error) {
	appt, err := a.appts.ByID(ctx, id)
	if err != nil {
		return domain.Appointment{}, err
	}
	if !canSee(actor, appt) {
		return domain.Appointment{}, fmt.Errorf("%w: appointment", domain.ErrNotFound)
	}
	return appt, nil
}

// Cancel releases the slot. A patient may cancel only before the visit starts; staff may
// cancel until it ends.
func (a *Appointments) Cancel(ctx context.Context, actor Actor, id, reason string) (domain.Appointment, error) {
	appt, err := a.Get(ctx, actor, id)
	if err != nil {
		return domain.Appointment{}, err
	}
	note, err := domain.NormalizeNote("reason", reason)
	if err != nil {
		return domain.Appointment{}, err
	}

	now := a.clock.Now()
	if actor.Role == domain.RolePatient && !now.Before(appt.StartsAt) {
		return domain.Appointment{}, fmt.Errorf("%w: a patient can only cancel before the visit starts", domain.ErrConflict)
	}
	if !now.Before(appt.EndsAt) {
		return domain.Appointment{}, fmt.Errorf("%w: the visit is already over", domain.ErrConflict)
	}
	return a.transition(ctx, appt, domain.AppointmentCancelled, note)
}

func (a *Appointments) Confirm(ctx context.Context, actor Actor, id string) (domain.Appointment, error) {
	appt, err := a.staffOnly(ctx, actor, id)
	if err != nil {
		return domain.Appointment{}, err
	}
	if !a.clock.Now().Before(appt.EndsAt) {
		return domain.Appointment{}, fmt.Errorf("%w: the visit is already over", domain.ErrConflict)
	}
	return a.transition(ctx, appt, domain.AppointmentConfirmed, "")
}

func (a *Appointments) Complete(ctx context.Context, actor Actor, id string) (domain.Appointment, error) {
	return a.close(ctx, actor, id, domain.AppointmentCompleted)
}

func (a *Appointments) NoShow(ctx context.Context, actor Actor, id string) (domain.Appointment, error) {
	return a.close(ctx, actor, id, domain.AppointmentNoShow)
}

// Reschedule moves a live appointment. It resets the status to scheduled: a confirmation was
// for the old time and the doctor has to confirm again.
func (a *Appointments) Reschedule(ctx context.Context, actor Actor, id string, in RescheduleInput) (domain.Appointment, error) {
	appt, err := a.Get(ctx, actor, id)
	if err != nil {
		return domain.Appointment{}, err
	}
	switch actor.Role {
	case domain.RolePatient:
		if !a.clock.Now().Before(appt.StartsAt) {
			return domain.Appointment{}, fmt.Errorf("%w: a patient can only reschedule before the visit starts", domain.ErrConflict)
		}
	case domain.RoleAdmin:
	default:
		return domain.Appointment{}, fmt.Errorf("%w: only the patient or an admin may reschedule", domain.ErrForbidden)
	}
	if !appt.Status.IsActive() {
		return domain.Appointment{}, fmt.Errorf("%w: a %s appointment cannot be rescheduled", domain.ErrConflict, appt.Status)
	}

	want, err := a.bookable(in.StartsAt, in.Duration, appt.EndsAt.Sub(appt.StartsAt))
	if err != nil {
		return domain.Appointment{}, err
	}
	return a.appts.Reschedule(ctx, RescheduleChange{
		ID:           appt.ID,
		From:         appt.Status,
		FromStartsAt: appt.StartsAt,
		StartsAt:     want.Start,
		EndsAt:       want.End,
	})
}

// close records the outcome of a visit, which only makes sense once it has started.
func (a *Appointments) close(ctx context.Context, actor Actor, id string, outcome domain.AppointmentStatus) (domain.Appointment, error) {
	appt, err := a.staffOnly(ctx, actor, id)
	if err != nil {
		return domain.Appointment{}, err
	}
	if a.clock.Now().Before(appt.StartsAt) {
		return domain.Appointment{}, fmt.Errorf("%w: the visit has not started yet", domain.ErrConflict)
	}
	return a.transition(ctx, appt, outcome, "")
}

func (a *Appointments) staffOnly(ctx context.Context, actor Actor, id string) (domain.Appointment, error) {
	appt, err := a.Get(ctx, actor, id)
	if err != nil {
		return domain.Appointment{}, err
	}
	if actor.Role != domain.RoleDoctor && actor.Role != domain.RoleAdmin {
		return domain.Appointment{}, fmt.Errorf("%w: only the doctor or an admin may do this", domain.ErrForbidden)
	}
	return appt, nil
}

func (a *Appointments) transition(ctx context.Context, appt domain.Appointment, to domain.AppointmentStatus, note string) (domain.Appointment, error) {
	if !appt.Status.CanTransitionTo(to) {
		return domain.Appointment{}, fmt.Errorf("%w: a %s appointment cannot become %s", domain.ErrConflict, appt.Status, to)
	}
	return a.appts.Transition(ctx, StatusChange{ID: appt.ID, From: appt.Status, To: to, CancelReason: note})
}

func (a *Appointments) bookable(startsAt time.Time, duration, fallback time.Duration) (domain.Interval, error) {
	if startsAt.IsZero() {
		return domain.Interval{}, domain.NewValidationError("starts_at", "is required")
	}
	if duration == 0 {
		duration = fallback
	}
	want := domain.Interval{Start: startsAt.UTC(), End: startsAt.UTC().Add(duration)}
	if err := a.schedule.CheckBookable(want, a.clock.Now()); err != nil {
		return domain.Interval{}, err
	}
	return want, nil
}

// activeParty resolves an account that must exist, be active and hold role. Every failure to
// qualify is reported as the same field error, so a booking cannot enumerate accounts.
func (a *Appointments) activeParty(ctx context.Context, id string, role domain.Role, field string) (domain.Party, error) {
	invalid := domain.NewValidationError(field, fmt.Sprintf("must reference an active %s", role))
	if id == "" {
		return domain.Party{}, invalid
	}
	user, err := a.users.ByID(ctx, id)
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrInvalidInput) {
		return domain.Party{}, invalid
	}
	if err != nil {
		return domain.Party{}, err
	}
	if user.Role != role || user.Status != domain.StatusActive {
		return domain.Party{}, invalid
	}
	return domain.Party{ID: user.ID, FullName: user.FullName}, nil
}

func canSee(actor Actor, appt domain.Appointment) bool {
	switch actor.Role {
	case domain.RoleAdmin:
		return true
	case domain.RoleDoctor:
		return appt.Doctor.ID == actor.UserID
	case domain.RolePatient:
		return appt.Patient.ID == actor.UserID
	default:
		return false
	}
}

func overlapsAny(slot domain.Interval, busy []domain.Interval) bool {
	for _, b := range busy {
		if slot.Overlaps(b) {
			return true
		}
	}
	return false
}
