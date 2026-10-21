package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// appointmentColumns projects an appointment joined to both parties' names. Every read and
// every RETURNING path goes through it, so scanAppointment has exactly one shape to match.
const appointmentColumns = `a.id, a.patient_id, p.full_name, a.doctor_id, d.full_name,
	a.starts_at, a.ends_at, a.status, a.reason, a.cancel_reason, a.created_by, a.created_at, a.updated_at`

const joinParties = `
JOIN users p ON p.id = a.patient_id
JOIN users d ON d.id = a.doctor_id`

const insertAppointment = `WITH a AS (
	INSERT INTO appointments (patient_id, doctor_id, created_by, starts_at, ends_at, status, reason)
	VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7)
	RETURNING *
)
SELECT ` + appointmentColumns + ` FROM a` + joinParties

const selectAppointmentByID = `SELECT ` + appointmentColumns + `
FROM appointments a` + joinParties + `
WHERE a.id = $1::uuid`

// The filters bind NULL for "no predicate", so the WHERE clause is constant SQL. The two
// variants differ only in sort direction, which cannot be a bind parameter.
const listAppointmentsWhere = `
WHERE ($1::uuid IS NULL OR a.patient_id = $1::uuid)
  AND ($2::uuid IS NULL OR a.doctor_id = $2::uuid)
  AND ($3::text = '' OR a.status = $3::text)
  AND ($4::timestamptz IS NULL OR a.ends_at > $4::timestamptz)
  AND ($5::timestamptz IS NULL OR a.starts_at < $5::timestamptz)`

const listAppointmentsAsc = `SELECT ` + appointmentColumns + `
FROM appointments a` + joinParties + listAppointmentsWhere + `
ORDER BY a.starts_at ASC, a.id ASC
LIMIT $6 OFFSET $7`

const listAppointmentsDesc = `SELECT ` + appointmentColumns + `
FROM appointments a` + joinParties + listAppointmentsWhere + `
ORDER BY a.starts_at DESC, a.id DESC
LIMIT $6 OFFSET $7`

// transitionAppointment is the compare-and-set behind every status change: the WHERE on the
// old status makes two concurrent changes serialize, and the loser updates zero rows.
const transitionAppointment = `WITH a AS (
	UPDATE appointments
	SET status = $3,
	    cancel_reason = CASE WHEN $3 = 'cancelled' THEN $4 ELSE cancel_reason END,
	    updated_at = now()
	WHERE id = $1::uuid AND status = $2
	RETURNING *
)
SELECT ` + appointmentColumns + ` FROM a` + joinParties

const rescheduleAppointment = `WITH a AS (
	UPDATE appointments
	SET starts_at = $4, ends_at = $5, status = 'scheduled', updated_at = now()
	WHERE id = $1::uuid AND status = $2 AND starts_at = $3
	RETURNING *
)
SELECT ` + appointmentColumns + ` FROM a` + joinParties

const selectBusy = `SELECT starts_at, ends_at
FROM appointments
WHERE doctor_id = $1::uuid
  AND status IN ('scheduled', 'confirmed')
  AND ends_at > $2
  AND starts_at < $3
ORDER BY starts_at`

// Appointments implements usecase.AppointmentRepository on top of PostgreSQL.
type Appointments struct {
	db Executor
}

func NewAppointments(db Executor) *Appointments {
	return &Appointments{db: db}
}

var _ usecase.AppointmentRepository = (*Appointments)(nil)

func (r *Appointments) Create(ctx context.Context, appt domain.Appointment) (domain.Appointment, error) {
	if err := ctx.Err(); err != nil {
		return domain.Appointment{}, err
	}
	for _, id := range []string{appt.Patient.ID, appt.Doctor.ID, appt.CreatedBy} {
		if !isUUID(id) {
			return domain.Appointment{}, fmt.Errorf("%w: account id is malformed", domain.ErrInvalidInput)
		}
	}

	created, err := scanAppointment(r.db.QueryRow(ctx, insertAppointment,
		appt.Patient.ID, appt.Doctor.ID, appt.CreatedBy, appt.StartsAt, appt.EndsAt, string(appt.Status), appt.Reason,
	))
	if err != nil {
		return domain.Appointment{}, translate(err, "appointment")
	}
	return created, nil
}

func (r *Appointments) ByID(ctx context.Context, id string) (domain.Appointment, error) {
	if err := ctx.Err(); err != nil {
		return domain.Appointment{}, err
	}
	if !isUUID(id) {
		return domain.Appointment{}, fmt.Errorf("%w: appointment id is malformed", domain.ErrInvalidInput)
	}

	appt, err := scanAppointment(r.db.QueryRow(ctx, selectAppointmentByID, id))
	if err != nil {
		return domain.Appointment{}, translate(err, "appointment")
	}
	return appt, nil
}

func (r *Appointments) List(ctx context.Context, filter usecase.AppointmentFilter) ([]domain.Appointment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, id := range []string{filter.PatientID, filter.DoctorID} {
		if id != "" && !isUUID(id) {
			return nil, fmt.Errorf("%w: account id is malformed", domain.ErrInvalidInput)
		}
	}

	query := listAppointmentsAsc
	if filter.Descending {
		query = listAppointmentsDesc
	}
	rows, err := r.db.Query(ctx, query,
		nullable(filter.PatientID), nullable(filter.DoctorID), string(filter.Status),
		nullableTime(filter.From), nullableTime(filter.To), filter.Limit, filter.Offset,
	)
	if err != nil {
		return nil, translate(err, "appointment list")
	}
	defer rows.Close()

	appts := []domain.Appointment{}
	for rows.Next() {
		appt, err := scanAppointment(rows)
		if err != nil {
			return nil, translate(err, "appointment list")
		}
		appts = append(appts, appt)
	}
	if err := rows.Err(); err != nil {
		return nil, translate(err, "appointment list")
	}
	return appts, nil
}

func (r *Appointments) Transition(ctx context.Context, change usecase.StatusChange) (domain.Appointment, error) {
	if err := ctx.Err(); err != nil {
		return domain.Appointment{}, err
	}
	if !isUUID(change.ID) {
		return domain.Appointment{}, fmt.Errorf("%w: appointment id is malformed", domain.ErrInvalidInput)
	}

	appt, err := scanAppointment(r.db.QueryRow(ctx, transitionAppointment,
		change.ID, string(change.From), string(change.To), change.CancelReason,
	))
	if err != nil {
		return domain.Appointment{}, lostRace(err)
	}
	return appt, nil
}

func (r *Appointments) Reschedule(ctx context.Context, change usecase.RescheduleChange) (domain.Appointment, error) {
	if err := ctx.Err(); err != nil {
		return domain.Appointment{}, err
	}
	if !isUUID(change.ID) {
		return domain.Appointment{}, fmt.Errorf("%w: appointment id is malformed", domain.ErrInvalidInput)
	}

	appt, err := scanAppointment(r.db.QueryRow(ctx, rescheduleAppointment,
		change.ID, string(change.From), change.FromStartsAt, change.StartsAt, change.EndsAt,
	))
	if err != nil {
		return domain.Appointment{}, lostRace(err)
	}
	return appt, nil
}

func (r *Appointments) Busy(ctx context.Context, doctorID string, window domain.Interval) ([]domain.Interval, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !isUUID(doctorID) {
		return nil, fmt.Errorf("%w: doctor id is malformed", domain.ErrInvalidInput)
	}

	rows, err := r.db.Query(ctx, selectBusy, doctorID, window.Start, window.End)
	if err != nil {
		return nil, translate(err, "busy intervals")
	}
	defer rows.Close()

	busy := []domain.Interval{}
	for rows.Next() {
		var interval domain.Interval
		if err := rows.Scan(&interval.Start, &interval.End); err != nil {
			return nil, translate(err, "busy intervals")
		}
		busy = append(busy, domain.Interval{Start: interval.Start.UTC(), End: interval.End.UTC()})
	}
	if err := rows.Err(); err != nil {
		return nil, translate(err, "busy intervals")
	}
	return busy, nil
}

// lostRace maps "no row matched the compare-and-set" to a conflict. The use case read the row
// moments ago, so a miss means another request changed or moved it in between.
func lostRace(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: appointment changed concurrently", domain.ErrConflict)
	}
	return translate(err, "appointment")
}

func scanAppointment(row Row) (domain.Appointment, error) {
	var (
		appt   domain.Appointment
		status string
	)
	if err := row.Scan(
		&appt.ID,
		&appt.Patient.ID,
		&appt.Patient.FullName,
		&appt.Doctor.ID,
		&appt.Doctor.FullName,
		&appt.StartsAt,
		&appt.EndsAt,
		&status,
		&appt.Reason,
		&appt.CancelReason,
		&appt.CreatedBy,
		&appt.CreatedAt,
		&appt.UpdatedAt,
	); err != nil {
		return domain.Appointment{}, err
	}

	parsed, err := domain.ParseAppointmentStatus(status)
	if err != nil {
		return domain.Appointment{}, fmt.Errorf("repository: stored appointment status %q is unknown: %w", status, errors.Join(domain.ErrConflict, err))
	}
	appt.Status = parsed
	// pgx hands timestamptz back in time.Local; UTC keeps the API output independent of the host.
	appt.StartsAt, appt.EndsAt = appt.StartsAt.UTC(), appt.EndsAt.UTC()
	appt.CreatedAt, appt.UpdatedAt = appt.CreatedAt.UTC(), appt.UpdatedAt.UTC()
	return appt, nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}
