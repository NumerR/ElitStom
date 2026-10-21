//go:build integration

package postgres_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/repository/postgres"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// The overlap rule and the compare-and-set live in the database, so these are the only tests
// that prove them: the use-case suite runs against a fake that merely imitates them.

// apptBase is a fixed point in the future so the rows never depend on the wall clock.
var apptBase = time.Date(2030, 3, 4, 9, 0, 0, 0, time.UTC)

type apptFixture struct {
	*harness
	appts             *postgres.Appointments
	doctor, doctor2   domain.User
	patient, patient2 domain.User
}

func setupAppointments(t *testing.T) *apptFixture {
	t.Helper()

	h := setupHarness(t)
	return &apptFixture{
		harness:  h,
		appts:    postgres.NewAppointments(postgres.NewExecutor(h.pool)),
		doctor:   h.newAccount(t, domain.RoleDoctor),
		doctor2:  h.newAccount(t, domain.RoleDoctor),
		patient:  h.newAccount(t, domain.RolePatient),
		patient2: h.newAccount(t, domain.RolePatient),
	}
}

func (f *apptFixture) book(patient, doctor domain.User, startOffset, length time.Duration) (domain.Appointment, error) {
	return f.appts.Create(f.ctx, domain.Appointment{
		Patient:   domain.Party{ID: patient.ID},
		Doctor:    domain.Party{ID: doctor.ID},
		StartsAt:  apptBase.Add(startOffset),
		EndsAt:    apptBase.Add(startOffset + length),
		Status:    domain.AppointmentScheduled,
		Reason:    "check-up",
		CreatedBy: patient.ID,
	})
}

func (f *apptFixture) mustBook(t *testing.T, patient, doctor domain.User, startOffset, length time.Duration) domain.Appointment {
	t.Helper()

	appt, err := f.book(patient, doctor, startOffset, length)
	require.NoError(t, err, "book")
	return appt
}

func TestIntegrationAppointmentRoundTrip(t *testing.T) {
	f := setupAppointments(t)

	created := f.mustBook(t, f.patient, f.doctor, 0, 30*time.Minute)
	assert.Regexp(t, uuidText, created.ID)
	assert.Equal(t, f.patient.FullName, created.Patient.FullName, "the RETURNING path must join the party names")
	assert.Equal(t, domain.AppointmentScheduled, created.Status)
	assert.Equal(t, time.UTC, created.StartsAt.Location(), "times are normalised to UTC")
	assert.True(t, created.StartsAt.Equal(apptBase))

	read, err := f.appts.ByID(f.ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created, read)

	_, err = f.appts.ByID(f.ctx, "00000000-0000-4000-8000-000000000000")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestIntegrationExclusionConstraintsRefuseOverlaps(t *testing.T) {
	f := setupAppointments(t)
	f.mustBook(t, f.patient, f.doctor, time.Hour, time.Hour) // 10:00–11:00

	cases := []struct {
		name        string
		patient     func() domain.User
		doctor      func() domain.User
		offset      time.Duration
		length      time.Duration
		wantOverlap bool
	}{
		{name: "same doctor, overlapping", patient: func() domain.User { return f.patient2 }, doctor: func() domain.User { return f.doctor }, offset: 90 * time.Minute, length: time.Hour, wantOverlap: true},
		{name: "same doctor, enclosing", patient: func() domain.User { return f.patient2 }, doctor: func() domain.User { return f.doctor }, offset: 0, length: 3 * time.Hour, wantOverlap: true},
		{name: "same patient, another doctor", patient: func() domain.User { return f.patient }, doctor: func() domain.User { return f.doctor2 }, offset: 75 * time.Minute, length: 30 * time.Minute, wantOverlap: true},
		{name: "back-to-back before", patient: func() domain.User { return f.patient2 }, doctor: func() domain.User { return f.doctor }, offset: 30 * time.Minute, length: 30 * time.Minute},
		{name: "back-to-back after", patient: func() domain.User { return f.patient2 }, doctor: func() domain.User { return f.doctor }, offset: 2 * time.Hour, length: 30 * time.Minute},
		{name: "other doctor and patient, same time", patient: func() domain.User { return f.patient2 }, doctor: func() domain.User { return f.doctor2 }, offset: time.Hour, length: time.Hour},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appt, err := f.book(tc.patient(), tc.doctor(), tc.offset, tc.length)
			if tc.wantOverlap {
				require.ErrorIs(t, err, domain.ErrConflict)
				return
			}
			require.NoError(t, err)

			// Release the slot again so the cases stay independent of each other.
			_, err = f.appts.Transition(f.ctx, usecase.StatusChange{ID: appt.ID, From: appt.Status, To: domain.AppointmentCancelled})
			require.NoError(t, err)
		})
	}
}

func TestIntegrationReleasedSlotsCanBeRebooked(t *testing.T) {
	f := setupAppointments(t)

	for _, outcome := range []domain.AppointmentStatus{domain.AppointmentCancelled, domain.AppointmentNoShow, domain.AppointmentCompleted} {
		t.Run(string(outcome), func(t *testing.T) {
			held := f.mustBook(t, f.patient, f.doctor, 0, 30*time.Minute)
			_, err := f.appts.Transition(f.ctx, usecase.StatusChange{ID: held.ID, From: held.Status, To: outcome})
			require.NoError(t, err)

			again := f.mustBook(t, f.patient2, f.doctor, 0, 30*time.Minute)
			_, err = f.appts.Transition(f.ctx, usecase.StatusChange{ID: again.ID, From: again.Status, To: domain.AppointmentCancelled})
			require.NoError(t, err)
		})
	}
}

func TestIntegrationCheckAndForeignKeyConstraints(t *testing.T) {
	f := setupAppointments(t)

	_, err := f.book(f.doctor, f.doctor, 0, 30*time.Minute)
	assert.Error(t, err, "a doctor cannot be their own patient")

	_, err = f.book(f.patient, f.doctor, 0, 9*time.Hour)
	assert.Error(t, err, "an appointment longer than 8 hours is refused")

	_, err = f.book(f.patient, f.doctor, time.Hour, -30*time.Minute)
	assert.Error(t, err, "an appointment must end after it starts")

	ghost := domain.User{ID: "00000000-0000-4000-8000-000000000000"}
	_, err = f.book(f.patient, ghost, 0, 30*time.Minute)
	assert.ErrorIs(t, err, domain.ErrConflict, "a missing doctor violates the foreign key")
}

// TestIntegrationConcurrentBookingsOfOneSlotHaveOneWinner is the reason the overlap rule lives in
// an exclusion constraint: a read-then-insert check in Go would let every racer through.
func TestIntegrationConcurrentBookingsOfOneSlotHaveOneWinner(t *testing.T) {
	f := setupAppointments(t)

	patients := make([]domain.User, 8)
	for i := range patients {
		patients[i] = f.newAccount(t, domain.RolePatient)
	}

	start := make(chan struct{})
	unexpected := make(chan error, len(patients))
	var wins, conflicts atomic.Int32
	var wg sync.WaitGroup

	for _, patient := range patients {
		wg.Add(1)
		go func() {
			defer wg.Done()

			<-start
			_, err := f.book(patient, f.doctor, 0, 30*time.Minute)
			switch {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, domain.ErrConflict):
				conflicts.Add(1)
			default:
				unexpected <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(unexpected)

	for err := range unexpected {
		t.Fatalf("Create() error = %v, want nil or ErrConflict", err)
	}
	assert.EqualValues(t, 1, wins.Load(), "exactly one booking may hold the slot")
	assert.EqualValues(t, len(patients)-1, conflicts.Load())
}

func TestIntegrationTransitionIsACompareAndSet(t *testing.T) {
	f := setupAppointments(t)
	appt := f.mustBook(t, f.patient, f.doctor, 0, 30*time.Minute)

	confirmed, err := f.appts.Transition(f.ctx, usecase.StatusChange{
		ID: appt.ID, From: domain.AppointmentScheduled, To: domain.AppointmentConfirmed, CancelReason: "ignored",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.AppointmentConfirmed, confirmed.Status)
	assert.Empty(t, confirmed.CancelReason, "a cancel note is only written by a cancellation")
	assert.True(t, confirmed.UpdatedAt.After(appt.UpdatedAt) || confirmed.UpdatedAt.Equal(appt.UpdatedAt))

	_, err = f.appts.Transition(f.ctx, usecase.StatusChange{ID: appt.ID, From: domain.AppointmentScheduled, To: domain.AppointmentCancelled})
	assert.ErrorIs(t, err, domain.ErrConflict, "a stale expected status must lose")

	const racers = 8
	start := make(chan struct{})
	unexpected := make(chan error, racers)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			<-start
			_, err := f.appts.Transition(f.ctx, usecase.StatusChange{
				ID: appt.ID, From: domain.AppointmentConfirmed, To: domain.AppointmentCancelled, CancelReason: "away",
			})
			switch {
			case err == nil:
				wins.Add(1)
			case !errors.Is(err, domain.ErrConflict):
				unexpected <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(unexpected)

	for err := range unexpected {
		t.Fatalf("Transition() error = %v, want nil or ErrConflict", err)
	}
	assert.EqualValues(t, 1, wins.Load(), "exactly one concurrent transition may win")

	stored, err := f.appts.ByID(f.ctx, appt.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AppointmentCancelled, stored.Status)
	assert.Equal(t, "away", stored.CancelReason)
}

func TestIntegrationRescheduleGuardsTheOldStartAndTheOverlap(t *testing.T) {
	f := setupAppointments(t)
	appt := f.mustBook(t, f.patient, f.doctor, 0, 30*time.Minute)
	f.mustBook(t, f.patient2, f.doctor, 2*time.Hour, 30*time.Minute)

	_, err := f.appts.Transition(f.ctx, usecase.StatusChange{ID: appt.ID, From: domain.AppointmentScheduled, To: domain.AppointmentConfirmed})
	require.NoError(t, err)

	move := func(from time.Time, offset time.Duration) (domain.Appointment, error) {
		return f.appts.Reschedule(f.ctx, usecase.RescheduleChange{
			ID:           appt.ID,
			From:         domain.AppointmentConfirmed,
			FromStartsAt: from,
			StartsAt:     apptBase.Add(offset),
			EndsAt:       apptBase.Add(offset + 30*time.Minute),
		})
	}

	_, err = move(apptBase, 2*time.Hour+15*time.Minute)
	assert.ErrorIs(t, err, domain.ErrConflict, "moving into a taken slot is refused by the constraint")

	_, err = move(apptBase.Add(time.Minute), time.Hour)
	assert.ErrorIs(t, err, domain.ErrConflict, "a stale start time means somebody else moved it first")

	moved, err := move(apptBase, time.Hour)
	require.NoError(t, err)
	assert.True(t, moved.StartsAt.Equal(apptBase.Add(time.Hour)))
	assert.Equal(t, domain.AppointmentScheduled, moved.Status, "a moved visit needs confirming again")
}

func TestIntegrationListAndBusy(t *testing.T) {
	f := setupAppointments(t)
	first := f.mustBook(t, f.patient, f.doctor, 0, 30*time.Minute)
	second := f.mustBook(t, f.patient2, f.doctor, time.Hour, 30*time.Minute)
	other := f.mustBook(t, f.patient, f.doctor2, 3*time.Hour, time.Hour)
	_, err := f.appts.Transition(f.ctx, usecase.StatusChange{ID: second.ID, From: second.Status, To: domain.AppointmentCancelled})
	require.NoError(t, err)

	ids := func(t *testing.T, filter usecase.AppointmentFilter) []string {
		t.Helper()

		if filter.Limit == 0 {
			filter.Limit = 50
		}
		listed, err := f.appts.List(f.ctx, filter)
		require.NoError(t, err)
		out := make([]string, 0, len(listed))
		for _, appt := range listed {
			out = append(out, appt.ID)
		}
		return out
	}

	assert.Equal(t, []string{first.ID, second.ID}, ids(t, usecase.AppointmentFilter{DoctorID: f.doctor.ID}))
	assert.Equal(t, []string{second.ID, first.ID}, ids(t, usecase.AppointmentFilter{DoctorID: f.doctor.ID, Descending: true}))
	assert.Equal(t, []string{first.ID, other.ID}, ids(t, usecase.AppointmentFilter{PatientID: f.patient.ID}))
	assert.Equal(t, []string{second.ID}, ids(t, usecase.AppointmentFilter{DoctorID: f.doctor.ID, Status: domain.AppointmentCancelled}))
	assert.Equal(t, []string{second.ID, other.ID}, ids(t, usecase.AppointmentFilter{From: apptBase.Add(30 * time.Minute)}),
		"from keeps visits that end after it")
	assert.Equal(t, []string{first.ID}, ids(t, usecase.AppointmentFilter{To: apptBase.Add(time.Hour)}),
		"to keeps visits that start before it")
	assert.Equal(t, []string{second.ID}, ids(t, usecase.AppointmentFilter{Limit: 1, Offset: 1}))
	assert.Empty(t, ids(t, usecase.AppointmentFilter{Offset: 10}))

	busy, err := f.appts.Busy(f.ctx, f.doctor.ID, domain.Interval{Start: apptBase.Add(-time.Hour), End: apptBase.Add(5 * time.Hour)})
	require.NoError(t, err)
	assert.Equal(t, []domain.Interval{first.Interval()}, busy, "a cancelled visit no longer blocks the doctor")
}
