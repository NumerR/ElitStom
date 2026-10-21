package postgres

import (
	"context"
	"errors"
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
	fakePatientID = "1b8f5c1a-2d3e-4a5b-8c9d-0e1f2a3b4c5d"
	fakeDoctorID  = "2b8f5c1a-2d3e-4a5b-8c9d-0e1f2a3b4c5d"
	fakeStartsAt  = time.Date(2026, 10, 23, 7, 0, 0, 0, time.UTC)
	fakeEndsAt    = fakeStartsAt.Add(30 * time.Minute)
)

func appointmentValues(status string) []any {
	return []any{
		fakeID, fakePatientID, "Ada Lovelace", fakeDoctorID, "Dr. Grace Hopper",
		fakeStartsAt, fakeEndsAt, status, "Toothache", "", fakePatientID, fakeCreatedAt, fakeCreatedAt,
	}
}

func TestScanAppointment(t *testing.T) {
	moscow := time.FixedZone("MSK", 3*60*60)

	cases := []struct {
		name       string
		row        fakeRow
		wantStatus domain.AppointmentStatus
		wantIs     error
	}{
		{name: "maps a scheduled row", row: fakeRow{values: appointmentValues("scheduled")}, wantStatus: domain.AppointmentScheduled},
		{name: "maps a terminal row", row: fakeRow{values: appointmentValues("no_show")}, wantStatus: domain.AppointmentNoShow},
		{name: "rejects an unknown stored status", row: fakeRow{values: appointmentValues("postponed")}, wantIs: domain.ErrConflict},
		{name: "surfaces a scan failure", row: fakeRow{err: errors.New("conn reset")}},
		{
			name: "normalises driver-local timestamps to UTC",
			row: fakeRow{values: []any{
				fakeID, fakePatientID, "Ada Lovelace", fakeDoctorID, "Dr. Grace Hopper",
				fakeStartsAt.In(moscow), fakeEndsAt.In(moscow), "confirmed", "", "", fakePatientID,
				fakeCreatedAt.In(moscow), fakeCreatedAt.In(moscow),
			}},
			wantStatus: domain.AppointmentConfirmed,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appt, err := scanAppointment(tc.row)

			if tc.wantStatus == "" {
				if err == nil {
					t.Fatal("scanAppointment() = nil error, want a failure")
				}
				if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
					t.Errorf("error = %v, want errors.Is(err, %v)", err, tc.wantIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("scanAppointment() error = %v", err)
			}
			if appt.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", appt.Status, tc.wantStatus)
			}
			if appt.Patient != (domain.Party{ID: fakePatientID, FullName: "Ada Lovelace"}) {
				t.Errorf("patient = %+v", appt.Patient)
			}
			if appt.Doctor != (domain.Party{ID: fakeDoctorID, FullName: "Dr. Grace Hopper"}) {
				t.Errorf("doctor = %+v", appt.Doctor)
			}
			for name, ts := range map[string]time.Time{
				"starts_at": appt.StartsAt, "ends_at": appt.EndsAt, "created_at": appt.CreatedAt, "updated_at": appt.UpdatedAt,
			} {
				if ts.Location() != time.UTC {
					t.Errorf("%s location = %s, want UTC", name, ts.Location())
				}
			}
			if !appt.StartsAt.Equal(fakeStartsAt) {
				t.Errorf("starts_at = %s, want %s", appt.StartsAt, fakeStartsAt)
			}
		})
	}
}

func TestAppointmentColumnCountMatchesTheProjection(t *testing.T) {
	columns := strings.Split(appointmentColumns, ",")
	if got, want := len(columns), len(appointmentValues("scheduled")); got != want {
		t.Fatalf("appointmentColumns lists %d columns but scanAppointment reads %d", got, want)
	}
}

func TestAppointmentsCreateBindsTheBooking(t *testing.T) {
	db := &fakeExecutor{row: fakeRow{values: appointmentValues("scheduled")}}
	repo := NewAppointments(db)

	created, err := repo.Create(context.Background(), domain.Appointment{
		Patient:   domain.Party{ID: fakePatientID},
		Doctor:    domain.Party{ID: fakeDoctorID},
		CreatedBy: fakePatientID,
		StartsAt:  fakeStartsAt,
		EndsAt:    fakeEndsAt,
		Status:    domain.AppointmentScheduled,
		Reason:    "Toothache",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID != fakeID {
		t.Errorf("id = %q, want the database-assigned %q", created.ID, fakeID)
	}

	want := []any{fakePatientID, fakeDoctorID, fakePatientID, fakeStartsAt, fakeEndsAt, "scheduled", "Toothache"}
	if !reflect.DeepEqual(db.queries[0].args, want) {
		t.Errorf("args = %v, want %v", db.queries[0].args, want)
	}
	assertNoInterpolation(t, db.queries[0].sql)
}

func TestAppointmentsCreateMapsOverlapToConflict(t *testing.T) {
	db := &fakeExecutor{row: fakeRow{err: &pgconn.PgError{Code: codeExclusionViolation, ConstraintName: "appointments_doctor_no_overlap"}}}
	repo := NewAppointments(db)

	_, err := repo.Create(context.Background(), domain.Appointment{
		Patient: domain.Party{ID: fakePatientID}, Doctor: domain.Party{ID: fakeDoctorID}, CreatedBy: fakePatientID,
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Create() error = %v, want ErrConflict", err)
	}
}

func TestAppointmentsRejectMalformedIDsWithoutAQuery(t *testing.T) {
	db := &fakeExecutor{row: fakeRow{values: appointmentValues("scheduled")}}
	repo := NewAppointments(db)
	ctx := context.Background()

	calls := map[string]func() error{
		"create with a forged patient": func() error {
			_, err := repo.Create(ctx, domain.Appointment{
				Patient: domain.Party{ID: "1 OR 1=1"}, Doctor: domain.Party{ID: fakeDoctorID}, CreatedBy: fakeDoctorID,
			})
			return err
		},
		"by id":   func() error { _, err := repo.ByID(ctx, "not-a-uuid"); return err },
		"list":    func() error { _, err := repo.List(ctx, usecase.AppointmentFilter{DoctorID: "x"}); return err },
		"busy":    func() error { _, err := repo.Busy(ctx, "x", domain.Interval{}); return err },
		"cancel":  func() error { _, err := repo.Transition(ctx, usecase.StatusChange{ID: "x"}); return err },
		"move":    func() error { _, err := repo.Reschedule(ctx, usecase.RescheduleChange{ID: "x"}); return err },
		"patient": func() error { _, err := repo.List(ctx, usecase.AppointmentFilter{PatientID: "x"}); return err },
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("error = %v, want ErrInvalidInput", err)
			}
		})
	}
	if len(db.queries) != 0 {
		t.Errorf("issued %d queries for malformed ids, want 0", len(db.queries))
	}
}

func TestAppointmentsByIDMapsNoRowsToNotFound(t *testing.T) {
	repo := NewAppointments(&fakeExecutor{row: fakeRow{err: pgx.ErrNoRows}})

	if _, err := repo.ByID(context.Background(), fakeID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("ByID() error = %v, want ErrNotFound", err)
	}
}

func TestAppointmentsListBindsFiltersAndDirection(t *testing.T) {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)

	cases := []struct {
		name      string
		filter    usecase.AppointmentFilter
		want      []any
		wantOrder string
	}{
		{
			name:      "no predicates bind NULL",
			filter:    usecase.AppointmentFilter{Limit: 50},
			want:      []any{nil, nil, "", nil, nil, 50, 0},
			wantOrder: "ORDER BY a.starts_at ASC",
		},
		{
			name: "every predicate",
			filter: usecase.AppointmentFilter{
				PatientID: fakePatientID, DoctorID: fakeDoctorID, Status: domain.AppointmentConfirmed,
				From: from, To: to, Limit: 10, Offset: 20,
			},
			want:      []any{fakePatientID, fakeDoctorID, "confirmed", from, to, 10, 20},
			wantOrder: "ORDER BY a.starts_at ASC",
		},
		{
			name:      "newest first",
			filter:    usecase.AppointmentFilter{Descending: true, Limit: 5},
			want:      []any{nil, nil, "", nil, nil, 5, 0},
			wantOrder: "ORDER BY a.starts_at DESC",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := &fakeExecutor{rows: &fakeRows{rows: [][]any{appointmentValues("scheduled")}}}
			repo := NewAppointments(db)

			listed, err := repo.List(context.Background(), tc.filter)
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if len(listed) != 1 {
				t.Fatalf("List() returned %d appointments, want 1", len(listed))
			}
			if !reflect.DeepEqual(db.queries[0].args, tc.want) {
				t.Errorf("args = %#v, want %#v", db.queries[0].args, tc.want)
			}
			if !strings.Contains(db.queries[0].sql, tc.wantOrder) {
				t.Errorf("sql does not contain %q", tc.wantOrder)
			}
			assertNoInterpolation(t, db.queries[0].sql)
			if db.rows.closeCnt != 1 {
				t.Error("rows were not closed")
			}
		})
	}
}

func TestAppointmentsListReturnsAnEmptySliceNotNil(t *testing.T) {
	repo := NewAppointments(&fakeExecutor{rows: &fakeRows{}})

	listed, err := repo.List(context.Background(), usecase.AppointmentFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if listed == nil || len(listed) != 0 {
		t.Fatalf("List() = %#v, want an allocated empty slice", listed)
	}
}

func TestAppointmentsListAndBusySurfaceRowFailures(t *testing.T) {
	cases := []struct {
		name string
		rows func() *fakeRows
		qerr error
	}{
		{name: "query fails", rows: func() *fakeRows { return nil }, qerr: errors.New("connection refused")},
		{name: "iteration interrupted", rows: func() *fakeRows {
			return &fakeRows{rows: [][]any{appointmentValues("scheduled")}, nextErr: errors.New("server closed")}
		}},
		{name: "scan fails", rows: func() *fakeRows {
			return &fakeRows{rows: [][]any{appointmentValues("scheduled")}, scanErr: errors.New("bad column")}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			list := NewAppointments(&fakeExecutor{rows: tc.rows(), queryEr: tc.qerr})
			if _, err := list.List(context.Background(), usecase.AppointmentFilter{}); err == nil {
				t.Error("List() = nil error, want the failure to surface")
			}

			busy := NewAppointments(&fakeExecutor{rows: tc.rows(), queryEr: tc.qerr})
			if _, err := busy.Busy(context.Background(), fakeDoctorID, domain.Interval{}); err == nil {
				t.Error("Busy() = nil error, want the failure to surface")
			}
		})
	}
}

func TestAppointmentsTransitionIsACompareAndSet(t *testing.T) {
	t.Run("binds the expected and the new status", func(t *testing.T) {
		db := &fakeExecutor{row: fakeRow{values: appointmentValues("cancelled")}}
		repo := NewAppointments(db)

		appt, err := repo.Transition(context.Background(), usecase.StatusChange{
			ID: fakeID, From: domain.AppointmentScheduled, To: domain.AppointmentCancelled, CancelReason: "feeling better",
		})
		if err != nil {
			t.Fatalf("Transition() error = %v", err)
		}
		if appt.Status != domain.AppointmentCancelled {
			t.Errorf("status = %q", appt.Status)
		}
		want := []any{fakeID, "scheduled", "cancelled", "feeling better"}
		if !reflect.DeepEqual(db.queries[0].args, want) {
			t.Errorf("args = %v, want %v", db.queries[0].args, want)
		}
		if !strings.Contains(db.queries[0].sql, "WHERE id = $1::uuid AND status = $2") {
			t.Error("the UPDATE must be guarded by the status the caller read")
		}
	})

	t.Run("a lost race is a conflict, not a missing row", func(t *testing.T) {
		repo := NewAppointments(&fakeExecutor{row: fakeRow{err: pgx.ErrNoRows}})

		_, err := repo.Transition(context.Background(), usecase.StatusChange{ID: fakeID})
		if !errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("Transition() error = %v, want ErrConflict only", err)
		}
	})

	t.Run("other failures are translated", func(t *testing.T) {
		repo := NewAppointments(&fakeExecutor{row: fakeRow{err: context.DeadlineExceeded}})

		if _, err := repo.Transition(context.Background(), usecase.StatusChange{ID: fakeID}); err != context.DeadlineExceeded {
			t.Fatalf("Transition() error = %v, want the deadline unchanged", err)
		}
	})
}

func TestAppointmentsRescheduleGuardsStatusAndStart(t *testing.T) {
	newStart := fakeStartsAt.Add(24 * time.Hour)

	t.Run("binds the guard and the new interval", func(t *testing.T) {
		db := &fakeExecutor{row: fakeRow{values: appointmentValues("scheduled")}}
		repo := NewAppointments(db)

		_, err := repo.Reschedule(context.Background(), usecase.RescheduleChange{
			ID: fakeID, From: domain.AppointmentConfirmed, FromStartsAt: fakeStartsAt,
			StartsAt: newStart, EndsAt: newStart.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("Reschedule() error = %v", err)
		}
		want := []any{fakeID, "confirmed", fakeStartsAt, newStart, newStart.Add(time.Hour)}
		if !reflect.DeepEqual(db.queries[0].args, want) {
			t.Errorf("args = %v, want %v", db.queries[0].args, want)
		}
		if !strings.Contains(db.queries[0].sql, "status = 'scheduled'") {
			t.Error("a move must reset the confirmation")
		}
	})

	t.Run("an overlap at the new time is a conflict", func(t *testing.T) {
		repo := NewAppointments(&fakeExecutor{row: fakeRow{err: &pgconn.PgError{Code: codeExclusionViolation}}})

		if _, err := repo.Reschedule(context.Background(), usecase.RescheduleChange{ID: fakeID}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("Reschedule() error = %v, want ErrConflict", err)
		}
	})

	t.Run("a concurrent move is a conflict", func(t *testing.T) {
		repo := NewAppointments(&fakeExecutor{row: fakeRow{err: pgx.ErrNoRows}})

		if _, err := repo.Reschedule(context.Background(), usecase.RescheduleChange{ID: fakeID}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("Reschedule() error = %v, want ErrConflict", err)
		}
	})
}

func TestAppointmentsBusyReturnsUTCIntervals(t *testing.T) {
	moscow := time.FixedZone("MSK", 3*60*60)
	db := &fakeExecutor{rows: &fakeRows{rows: [][]any{
		{fakeStartsAt.In(moscow), fakeEndsAt.In(moscow)},
	}}}
	repo := NewAppointments(db)
	window := domain.Interval{Start: fakeStartsAt.Truncate(24 * time.Hour), End: fakeStartsAt.Truncate(24 * time.Hour).Add(24 * time.Hour)}

	busy, err := repo.Busy(context.Background(), fakeDoctorID, window)
	if err != nil {
		t.Fatalf("Busy() error = %v", err)
	}
	if len(busy) != 1 || busy[0].Start.Location() != time.UTC || !busy[0].Start.Equal(fakeStartsAt) {
		t.Fatalf("Busy() = %+v, want one UTC interval at %s", busy, fakeStartsAt)
	}
	want := []any{fakeDoctorID, window.Start, window.End}
	if !reflect.DeepEqual(db.queries[0].args, want) {
		t.Errorf("args = %v, want %v", db.queries[0].args, want)
	}
	if !strings.Contains(db.queries[0].sql, "status IN ('scheduled', 'confirmed')") {
		t.Error("only live bookings may block a slot")
	}
}

func TestAppointmentsHonourContextCancellation(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	db := &fakeExecutor{row: fakeRow{values: appointmentValues("scheduled")}}
	repo := NewAppointments(db)

	calls := []func() error{
		func() error { _, err := repo.Create(canceled, domain.Appointment{}); return err },
		func() error { _, err := repo.ByID(canceled, fakeID); return err },
		func() error { _, err := repo.List(canceled, usecase.AppointmentFilter{}); return err },
		func() error { _, err := repo.Transition(canceled, usecase.StatusChange{ID: fakeID}); return err },
		func() error { _, err := repo.Reschedule(canceled, usecase.RescheduleChange{ID: fakeID}); return err },
		func() error { _, err := repo.Busy(canceled, fakeDoctorID, domain.Interval{}); return err },
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
