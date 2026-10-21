package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NumerR/ElitStom/internal/domain"
)

// apptNow is Wednesday 2026-10-21, 12:00 in Moscow: mid-way through a working day, so "today"
// has both past and future slots.
var apptNow = clinicTime(2026, 10, 21, 12, 0)

// tomorrow10 is Thursday 10:00 Moscow, a plain bookable slot.
var tomorrow10 = clinicTime(2026, 10, 22, 10, 0)

type apptHarness struct {
	t       *testing.T
	users   *fakeUsers
	appts   *fakeAppointments
	clock   *fakeClock
	service *Appointments

	admin    Actor
	doctor   Actor
	doctor2  Actor
	patient  Actor
	patient2 Actor
}

func newApptHarness(t *testing.T) *apptHarness {
	t.Helper()

	schedule, err := domain.NewSchedule("Europe/Moscow", "09:00", "20:00", []int{1, 2, 3, 4, 5, 6}, 30*time.Minute, 60)
	require.NoError(t, err)

	h := &apptHarness{t: t, users: newFakeUsers(), appts: newFakeAppointments(), clock: newFakeClock(apptNow)}
	h.service, err = NewAppointments(h.appts, h.users, schedule, h.clock)
	require.NoError(t, err)

	h.admin = h.account(domain.RoleAdmin, domain.StatusActive, "admin")
	h.doctor = h.account(domain.RoleDoctor, domain.StatusActive, "house")
	h.doctor2 = h.account(domain.RoleDoctor, domain.StatusActive, "wilson")
	h.patient = h.account(domain.RolePatient, domain.StatusActive, "ada")
	h.patient2 = h.account(domain.RolePatient, domain.StatusActive, "grace")
	return h
}

func (h *apptHarness) account(role domain.Role, status domain.UserStatus, name string) Actor {
	id := h.users.seed(domain.User{
		Email: name + "@clinic.test", FullName: "Full " + name, Role: role, Status: status,
	})
	return Actor{UserID: id, Role: role}
}

// book creates an appointment as the patient, failing the test if the clinic refuses.
func (h *apptHarness) book(patient, doctor Actor, at time.Time) domain.Appointment {
	h.t.Helper()
	appt, err := h.service.Book(context.Background(), patient, BookInput{DoctorID: doctor.UserID, StartsAt: at})
	require.NoError(h.t, err)
	return appt
}

func requireField(t *testing.T, err error, field string) {
	t.Helper()
	var ve *domain.ValidationError
	require.ErrorAs(t, err, &ve)
	require.Len(t, ve.Fields, 1)
	assert.Equal(t, field, ve.Fields[0].Field)
}

func TestNewAppointmentsRejectsMissingDependencies(t *testing.T) {
	schedule, err := domain.NewSchedule("UTC", "09:00", "17:00", []int{1}, time.Hour, 7)
	require.NoError(t, err)

	cases := []struct {
		name     string
		appts    AppointmentRepository
		users    UserRepository
		schedule domain.Schedule
		clock    Clock
		want     string
	}{
		{name: "appointment repository", users: newFakeUsers(), schedule: schedule, clock: SystemClock{}, want: "appointment repository"},
		{name: "users repository", appts: newFakeAppointments(), schedule: schedule, clock: SystemClock{}, want: "users repository"},
		{name: "zero schedule", appts: newFakeAppointments(), users: newFakeUsers(), clock: SystemClock{}, want: "schedule"},
		{name: "clock", appts: newFakeAppointments(), users: newFakeUsers(), schedule: schedule, want: "clock"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, err := NewAppointments(tc.appts, tc.users, tc.schedule, tc.clock)
			require.Error(t, err)
			assert.Nil(t, service)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestScheduleIsExposedForTheCalendarEndpoint(t *testing.T) {
	h := newApptHarness(t)
	assert.Equal(t, 30*time.Minute, h.service.Schedule().Slot())
}

func TestDoctorsListsOnlyActiveDoctors(t *testing.T) {
	h := newApptHarness(t)
	h.account(domain.RoleDoctor, domain.StatusDisabled, "retired")

	doctors, err := h.service.Doctors(context.Background(), h.patient)

	require.NoError(t, err)
	assert.ElementsMatch(t, []domain.Party{
		{ID: h.doctor.UserID, FullName: "Full house"},
		{ID: h.doctor2.UserID, FullName: "Full wilson"},
	}, doctors)
	assert.Equal(t, maxListLimit, h.users.lastList.Limit)

	t.Run("an unknown role is refused", func(t *testing.T) {
		_, err := h.service.Doctors(context.Background(), Actor{UserID: "x", Role: "nurse"})
		assert.ErrorIs(t, err, domain.ErrForbidden)
	})

	t.Run("a repository failure surfaces", func(t *testing.T) {
		h.users.listErr = errors.New("db down")
		_, err := h.service.Doctors(context.Background(), h.patient)
		assert.ErrorContains(t, err, "db down")
	})
}

func TestFreeSlots(t *testing.T) {
	cases := []struct {
		name      string
		date      string
		arrange   func(h *apptHarness)
		wantCount int
		wantFirst time.Time
		wantGone  []time.Time
	}{
		{name: "a whole free working day", date: "2026-10-22", wantCount: 22, wantFirst: clinicTime(2026, 10, 22, 9, 0)},
		{
			name:      "today offers only what has not started",
			date:      "2026-10-21",
			wantCount: 15,
			wantFirst: clinicTime(2026, 10, 21, 12, 30),
		},
		{name: "a day off has no slots", date: "2026-10-25", wantCount: 0},
		{name: "a past day has no slots", date: "2026-10-20", wantCount: 0},
		{name: "a day beyond the horizon has no slots", date: "2027-01-15", wantCount: 0},
		{
			name: "a live booking blocks the slots it covers",
			date: "2026-10-22",
			arrange: func(h *apptHarness) {
				_, err := h.service.Book(context.Background(), h.patient, BookInput{
					DoctorID: h.doctor.UserID, StartsAt: tomorrow10, Duration: time.Hour,
				})
				require.NoError(h.t, err)
			},
			wantCount: 20,
			wantGone:  []time.Time{tomorrow10, tomorrow10.Add(30 * time.Minute)},
		},
		{
			name: "a cancelled booking frees its slot",
			date: "2026-10-22",
			arrange: func(h *apptHarness) {
				appt := h.book(h.patient, h.doctor, tomorrow10)
				_, err := h.service.Cancel(context.Background(), h.patient, appt.ID, "")
				require.NoError(h.t, err)
			},
			wantCount: 22,
		},
		{
			name: "another doctor's booking does not block",
			date: "2026-10-22",
			arrange: func(h *apptHarness) {
				h.book(h.patient, h.doctor2, tomorrow10)
			},
			wantCount: 22,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newApptHarness(t)
			if tc.arrange != nil {
				tc.arrange(h)
			}

			slots, err := h.service.FreeSlots(context.Background(), h.patient, h.doctor.UserID, tc.date)

			require.NoError(t, err)
			require.NotNil(t, slots, "an empty day must render as [] rather than null")
			assert.Len(t, slots, tc.wantCount)
			if !tc.wantFirst.IsZero() {
				assert.Equal(t, tc.wantFirst, slots[0].Start)
			}
			for _, gone := range tc.wantGone {
				for _, slot := range slots {
					assert.NotEqual(t, gone, slot.Start, "a booked slot was offered again")
				}
			}
		})
	}
}

func TestFreeSlotsQueriesTheWholeClinicDay(t *testing.T) {
	h := newApptHarness(t)

	_, err := h.service.FreeSlots(context.Background(), h.patient, h.doctor.UserID, "2026-10-22")

	require.NoError(t, err)
	assert.Equal(t, clinicTime(2026, 10, 22, 0, 0), h.appts.lastBusy.Start.UTC())
	assert.Equal(t, clinicTime(2026, 10, 23, 0, 0), h.appts.lastBusy.End.UTC())
}

func TestFreeSlotsRejections(t *testing.T) {
	h := newApptHarness(t)
	disabled := h.account(domain.RoleDoctor, domain.StatusDisabled, "gone")

	cases := []struct {
		name     string
		actor    Actor
		doctorID string
		date     string
		wantIs   error
		field    string
	}{
		{name: "malformed date", actor: h.patient, doctorID: h.doctor.UserID, date: "22.10.2026", field: "date"},
		{name: "unknown doctor", actor: h.patient, doctorID: "user-999", date: "2026-10-22", wantIs: domain.ErrNotFound},
		{name: "a patient is not a doctor", actor: h.patient, doctorID: h.patient2.UserID, date: "2026-10-22", wantIs: domain.ErrNotFound},
		{name: "a disabled doctor takes no bookings", actor: h.patient, doctorID: disabled.UserID, date: "2026-10-22", wantIs: domain.ErrNotFound},
		{name: "unknown role", actor: Actor{UserID: "x", Role: "nurse"}, doctorID: h.doctor.UserID, date: "2026-10-22", wantIs: domain.ErrForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.service.FreeSlots(context.Background(), tc.actor, tc.doctorID, tc.date)
			if tc.field != "" {
				requireField(t, err, tc.field)
				return
			}
			assert.ErrorIs(t, err, tc.wantIs)
		})
	}

	t.Run("a busy-lookup failure surfaces", func(t *testing.T) {
		h.appts.busyErr = errors.New("db down")
		_, err := h.service.FreeSlots(context.Background(), h.patient, h.doctor.UserID, "2026-10-22")
		assert.ErrorContains(t, err, "db down")
	})

	t.Run("an account lookup failure is not disguised as a missing doctor", func(t *testing.T) {
		h.users.byIDErr = errors.New("db down")
		_, err := h.service.FreeSlots(context.Background(), h.patient, h.doctor.UserID, "2026-10-22")
		assert.ErrorContains(t, err, "db down")
		assert.NotErrorIs(t, err, domain.ErrNotFound)
	})
}

func TestBookAuthorization(t *testing.T) {
	h := newApptHarness(t)

	cases := []struct {
		name        string
		actor       Actor
		patientID   string
		at          time.Time
		wantPatient string
		wantIs      error
		field       string
	}{
		{name: "a patient books for themselves", actor: h.patient, at: tomorrow10, wantPatient: h.patient.UserID},
		{name: "a patient may name themselves explicitly", actor: h.patient, patientID: h.patient.UserID, at: tomorrow10.Add(time.Hour), wantPatient: h.patient.UserID},
		{name: "a patient may not book for someone else", actor: h.patient, patientID: h.patient2.UserID, at: tomorrow10, wantIs: domain.ErrForbidden},
		{name: "an admin books on a patient's behalf", actor: h.admin, patientID: h.patient2.UserID, at: tomorrow10.Add(2 * time.Hour), wantPatient: h.patient2.UserID},
		{name: "an admin must name the patient", actor: h.admin, at: tomorrow10, field: "patient_id"},
		{name: "a doctor does not book", actor: h.doctor, patientID: h.patient.UserID, at: tomorrow10, wantIs: domain.ErrForbidden},
		{name: "an unknown role does not book", actor: Actor{UserID: "x", Role: "nurse"}, at: tomorrow10, wantIs: domain.ErrForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appt, err := h.service.Book(context.Background(), tc.actor, BookInput{
				DoctorID: h.doctor.UserID, PatientID: tc.patientID, StartsAt: tc.at, Reason: "  checkup  ",
			})

			switch {
			case tc.field != "":
				requireField(t, err, tc.field)
			case tc.wantIs != nil:
				assert.ErrorIs(t, err, tc.wantIs)
			default:
				require.NoError(t, err)
				assert.Equal(t, tc.wantPatient, appt.Patient.ID)
				assert.Equal(t, h.doctor.UserID, appt.Doctor.ID)
				assert.Equal(t, "Full house", appt.Doctor.FullName, "the response carries display names")
				assert.Equal(t, tc.actor.UserID, appt.CreatedBy, "the audit trail records who booked")
				assert.Equal(t, domain.AppointmentScheduled, appt.Status)
				assert.Equal(t, "checkup", appt.Reason)
			}
		})
	}
}

func TestBookValidation(t *testing.T) {
	h := newApptHarness(t)
	retired := h.account(domain.RoleDoctor, domain.StatusDisabled, "retired")
	inactive := h.account(domain.RolePatient, domain.StatusDisabled, "inactive")
	long := make([]rune, domain.MaxNoteRunes+1)
	for i := range long {
		long[i] = 'a'
	}

	cases := []struct {
		name  string
		actor Actor
		in    BookInput
		field string
	}{
		{name: "missing start", actor: h.patient, in: BookInput{DoctorID: h.doctor.UserID}, field: "starts_at"},
		{name: "a start in the past", actor: h.patient, in: BookInput{DoctorID: h.doctor.UserID, StartsAt: clinicTime(2026, 10, 21, 10, 0)}, field: "starts_at"},
		{name: "a Sunday", actor: h.patient, in: BookInput{DoctorID: h.doctor.UserID, StartsAt: clinicTime(2026, 10, 25, 10, 0)}, field: "starts_at"},
		{name: "off the slot grid", actor: h.patient, in: BookInput{DoctorID: h.doctor.UserID, StartsAt: tomorrow10.Add(10 * time.Minute)}, field: "starts_at"},
		{name: "past closing time", actor: h.patient, in: BookInput{DoctorID: h.doctor.UserID, StartsAt: clinicTime(2026, 10, 22, 19, 30), Duration: time.Hour}, field: "starts_at"},
		{name: "not a whole number of slots", actor: h.patient, in: BookInput{DoctorID: h.doctor.UserID, StartsAt: tomorrow10, Duration: 45 * time.Minute}, field: "duration_minutes"},
		{name: "a negative duration", actor: h.patient, in: BookInput{DoctorID: h.doctor.UserID, StartsAt: tomorrow10, Duration: -time.Hour}, field: "duration_minutes"},
		{name: "a reason that is too long", actor: h.patient, in: BookInput{DoctorID: h.doctor.UserID, StartsAt: tomorrow10, Reason: string(long)}, field: "reason"},
		{name: "a missing doctor", actor: h.patient, in: BookInput{StartsAt: tomorrow10}, field: "doctor_id"},
		{name: "an unknown doctor", actor: h.patient, in: BookInput{DoctorID: "user-999", StartsAt: tomorrow10}, field: "doctor_id"},
		{name: "a patient named as the doctor", actor: h.patient, in: BookInput{DoctorID: h.patient2.UserID, StartsAt: tomorrow10}, field: "doctor_id"},
		{name: "a disabled doctor", actor: h.patient, in: BookInput{DoctorID: retired.UserID, StartsAt: tomorrow10}, field: "doctor_id"},
		{name: "a doctor named as the patient", actor: h.admin, in: BookInput{DoctorID: h.doctor.UserID, PatientID: h.doctor2.UserID, StartsAt: tomorrow10}, field: "patient_id"},
		{name: "a disabled patient", actor: h.admin, in: BookInput{DoctorID: h.doctor.UserID, PatientID: inactive.UserID, StartsAt: tomorrow10}, field: "patient_id"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.service.Book(context.Background(), tc.actor, tc.in)
			requireField(t, err, tc.field)
		})
	}
	assert.Empty(t, h.appts.byID, "a rejected booking must never reach the repository")
}

func TestBookDurations(t *testing.T) {
	h := newApptHarness(t)

	oneSlot := h.book(h.patient, h.doctor, tomorrow10)
	assert.Equal(t, 30*time.Minute, oneSlot.EndsAt.Sub(oneSlot.StartsAt), "an omitted duration means one slot")

	long, err := h.service.Book(context.Background(), h.patient, BookInput{
		DoctorID: h.doctor.UserID, StartsAt: tomorrow10.Add(time.Hour), Duration: 90 * time.Minute,
	})
	require.NoError(t, err)
	assert.Equal(t, tomorrow10.Add(150*time.Minute), long.EndsAt)
}

func TestBookRefusesDoubleBooking(t *testing.T) {
	h := newApptHarness(t)
	h.book(h.patient, h.doctor, tomorrow10)

	t.Run("the doctor is already taken", func(t *testing.T) {
		_, err := h.service.Book(context.Background(), h.patient2, BookInput{DoctorID: h.doctor.UserID, StartsAt: tomorrow10})
		assert.ErrorIs(t, err, domain.ErrConflict)
	})

	t.Run("the patient is already elsewhere", func(t *testing.T) {
		_, err := h.service.Book(context.Background(), h.patient, BookInput{DoctorID: h.doctor2.UserID, StartsAt: tomorrow10})
		assert.ErrorIs(t, err, domain.ErrConflict)
	})

	t.Run("back-to-back visits do not collide", func(t *testing.T) {
		_, err := h.service.Book(context.Background(), h.patient2, BookInput{DoctorID: h.doctor.UserID, StartsAt: tomorrow10.Add(30 * time.Minute)})
		assert.NoError(t, err)
	})
}

// TestConcurrentBookingsOfOneSlotHaveOneWinner: free-slot checks are advisory; the repository's
// overlap rule is what decides, so racing patients cannot both get the same chair.
func TestConcurrentBookingsOfOneSlotHaveOneWinner(t *testing.T) {
	h := newApptHarness(t)
	patients := make([]Actor, 8)
	for i := range patients {
		patients[i] = h.account(domain.RolePatient, domain.StatusActive, "racer"+string(rune('a'+i)))
	}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		won       int
		conflicts int
	)
	for _, p := range patients {
		wg.Add(1)
		go func(p Actor) {
			defer wg.Done()
			_, err := h.service.Book(context.Background(), p, BookInput{DoctorID: h.doctor.UserID, StartsAt: tomorrow10})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				won++
			case errors.Is(err, domain.ErrConflict):
				conflicts++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}(p)
	}
	wg.Wait()

	assert.Equal(t, 1, won)
	assert.Equal(t, len(patients)-1, conflicts)
}

func TestBookSurfacesInfrastructureFailures(t *testing.T) {
	t.Run("account lookup", func(t *testing.T) {
		h := newApptHarness(t)
		h.users.byIDErr = errors.New("db down")
		_, err := h.service.Book(context.Background(), h.patient, BookInput{DoctorID: h.doctor.UserID, StartsAt: tomorrow10})
		assert.ErrorContains(t, err, "db down")
	})

	t.Run("insert", func(t *testing.T) {
		h := newApptHarness(t)
		h.appts.createErr = errors.New("disk full")
		_, err := h.service.Book(context.Background(), h.patient, BookInput{DoctorID: h.doctor.UserID, StartsAt: tomorrow10})
		assert.ErrorContains(t, err, "disk full")
	})
}

func TestListScopesByRole(t *testing.T) {
	h := newApptHarness(t)
	mine := h.book(h.patient, h.doctor, tomorrow10)
	theirs := h.book(h.patient2, h.doctor2, tomorrow10)

	cases := []struct {
		name   string
		actor  Actor
		filter AppointmentFilter
		want   []string
		wantIs error
	}{
		{name: "a patient sees only their own", actor: h.patient, want: []string{mine.ID}},
		{name: "a patient may filter on themselves", actor: h.patient, filter: AppointmentFilter{PatientID: h.patient.UserID}, want: []string{mine.ID}},
		{name: "a patient may not list someone else", actor: h.patient, filter: AppointmentFilter{PatientID: h.patient2.UserID}, wantIs: domain.ErrForbidden},
		{name: "a doctor sees only their own diary", actor: h.doctor2, want: []string{theirs.ID}},
		{name: "a doctor may not read a colleague's diary", actor: h.doctor, filter: AppointmentFilter{DoctorID: h.doctor2.UserID}, wantIs: domain.ErrForbidden},
		{name: "an admin sees the whole clinic", actor: h.admin, want: []string{mine.ID, theirs.ID}},
		{name: "an admin may narrow by doctor", actor: h.admin, filter: AppointmentFilter{DoctorID: h.doctor.UserID}, want: []string{mine.ID}},
		{name: "an unknown role sees nothing", actor: Actor{UserID: "x", Role: "nurse"}, wantIs: domain.ErrForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, err := h.service.List(context.Background(), tc.actor, tc.filter)
			if tc.wantIs != nil {
				assert.ErrorIs(t, err, tc.wantIs)
				return
			}
			require.NoError(t, err)
			ids := make([]string, 0, len(page.Appointments))
			for _, appt := range page.Appointments {
				ids = append(ids, appt.ID)
			}
			assert.ElementsMatch(t, tc.want, ids)
		})
	}
}

func TestListValidatesAndClampsTheFilter(t *testing.T) {
	from := clinicTime(2026, 10, 22, 0, 0)

	cases := []struct {
		name       string
		filter     AppointmentFilter
		field      string
		wantLimit  int
		wantOffset int
	}{
		{name: "defaults the page size", filter: AppointmentFilter{}, wantLimit: 50},
		{name: "caps the page size", filter: AppointmentFilter{Limit: 5000, Offset: 10}, wantLimit: 200, wantOffset: 10},
		{name: "floors a negative offset", filter: AppointmentFilter{Limit: 5, Offset: -3}, wantLimit: 5},
		{name: "an open-ended range is fine", filter: AppointmentFilter{From: from}, wantLimit: 50},
		{name: "an unknown status", filter: AppointmentFilter{Status: "postponed"}, field: "status"},
		{name: "a range that ends before it starts", filter: AppointmentFilter{From: from, To: from}, field: "to"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newApptHarness(t)

			page, err := h.service.List(context.Background(), h.admin, tc.filter)

			if tc.field != "" {
				requireField(t, err, tc.field)
				assert.Zero(t, h.appts.listCalls, "a refused filter must not reach the repository")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantLimit, page.Limit)
			assert.Equal(t, tc.wantOffset, page.Offset)
			assert.Equal(t, tc.wantLimit, h.appts.lastList.Limit)
		})
	}

	t.Run("a repository failure surfaces", func(t *testing.T) {
		h := newApptHarness(t)
		h.appts.listErr = errors.New("db down")
		_, err := h.service.List(context.Background(), h.admin, AppointmentFilter{})
		assert.ErrorContains(t, err, "db down")
	})
}

func TestGetHidesOtherPeoplesAppointments(t *testing.T) {
	h := newApptHarness(t)
	appt := h.book(h.patient, h.doctor, tomorrow10)

	cases := []struct {
		name   string
		actor  Actor
		id     string
		wantIs error
	}{
		{name: "the patient", actor: h.patient, id: appt.ID},
		{name: "the doctor", actor: h.doctor, id: appt.ID},
		{name: "an admin", actor: h.admin, id: appt.ID},
		{name: "another patient", actor: h.patient2, id: appt.ID, wantIs: domain.ErrNotFound},
		{name: "another doctor", actor: h.doctor2, id: appt.ID, wantIs: domain.ErrNotFound},
		{name: "an unknown role", actor: Actor{UserID: h.patient.UserID, Role: "nurse"}, id: appt.ID, wantIs: domain.ErrNotFound},
		{name: "an unknown id", actor: h.admin, id: "appt-999", wantIs: domain.ErrNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := h.service.Get(context.Background(), tc.actor, tc.id)
			if tc.wantIs != nil {
				assert.ErrorIs(t, err, tc.wantIs)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, appt.ID, got.ID)
		})
	}
}

func TestCancel(t *testing.T) {
	cases := []struct {
		name    string
		actor   func(h *apptHarness) Actor
		advance time.Duration
		status  domain.AppointmentStatus
		reason  string
		wantIs  error
		field   string
	}{
		{name: "a patient cancels ahead of time", actor: func(h *apptHarness) Actor { return h.patient }, reason: " feeling better "},
		{name: "a patient cannot cancel once the visit started", actor: func(h *apptHarness) Actor { return h.patient }, advance: 22*time.Hour + 5*time.Minute, wantIs: domain.ErrConflict},
		{name: "the doctor may cancel a visit in progress", actor: func(h *apptHarness) Actor { return h.doctor }, advance: 22*time.Hour + 5*time.Minute},
		{name: "an admin may cancel a confirmed visit", actor: func(h *apptHarness) Actor { return h.admin }, status: domain.AppointmentConfirmed},
		{name: "nobody cancels a visit that is over", actor: func(h *apptHarness) Actor { return h.admin }, advance: 23 * time.Hour, wantIs: domain.ErrConflict},
		{name: "a completed visit stays completed", actor: func(h *apptHarness) Actor { return h.admin }, status: domain.AppointmentCompleted, wantIs: domain.ErrConflict},
		{name: "a cancelled visit cannot be cancelled twice", actor: func(h *apptHarness) Actor { return h.patient }, status: domain.AppointmentCancelled, wantIs: domain.ErrConflict},
		{name: "another patient cannot even see it", actor: func(h *apptHarness) Actor { return h.patient2 }, wantIs: domain.ErrNotFound},
		{name: "a reason with control characters", actor: func(h *apptHarness) Actor { return h.patient }, reason: "bell\a", field: "reason"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newApptHarness(t)
			appt := h.book(h.patient, h.doctor, tomorrow10)
			if tc.status != "" {
				h.appts.setStatus(appt.ID, tc.status)
			}
			h.clock.Advance(tc.advance)

			got, err := h.service.Cancel(context.Background(), tc.actor(h), appt.ID, tc.reason)

			switch {
			case tc.field != "":
				requireField(t, err, tc.field)
			case tc.wantIs != nil:
				assert.ErrorIs(t, err, tc.wantIs)
			default:
				require.NoError(t, err)
				assert.Equal(t, domain.AppointmentCancelled, got.Status)
				assert.Equal(t, strings.TrimSpace(tc.reason), got.CancelReason)
			}
		})
	}
}

func TestConfirm(t *testing.T) {
	cases := []struct {
		name    string
		actor   func(h *apptHarness) Actor
		advance time.Duration
		status  domain.AppointmentStatus
		wantIs  error
	}{
		{name: "the doctor confirms", actor: func(h *apptHarness) Actor { return h.doctor }},
		{name: "an admin confirms", actor: func(h *apptHarness) Actor { return h.admin }},
		{name: "a patient cannot confirm their own visit", actor: func(h *apptHarness) Actor { return h.patient }, wantIs: domain.ErrForbidden},
		{name: "another doctor cannot see it", actor: func(h *apptHarness) Actor { return h.doctor2 }, wantIs: domain.ErrNotFound},
		{name: "confirming twice is a conflict", actor: func(h *apptHarness) Actor { return h.doctor }, status: domain.AppointmentConfirmed, wantIs: domain.ErrConflict},
		{name: "a visit that is over cannot be confirmed", actor: func(h *apptHarness) Actor { return h.doctor }, advance: 23 * time.Hour, wantIs: domain.ErrConflict},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newApptHarness(t)
			appt := h.book(h.patient, h.doctor, tomorrow10)
			if tc.status != "" {
				h.appts.setStatus(appt.ID, tc.status)
			}
			h.clock.Advance(tc.advance)

			got, err := h.service.Confirm(context.Background(), tc.actor(h), appt.ID)

			if tc.wantIs != nil {
				assert.ErrorIs(t, err, tc.wantIs)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, domain.AppointmentConfirmed, got.Status)
		})
	}
}

func TestCloseRecordsTheOutcome(t *testing.T) {
	type closer func(s *Appointments, ctx context.Context, actor Actor, id string) (domain.Appointment, error)
	outcomes := map[domain.AppointmentStatus]closer{
		domain.AppointmentCompleted: (*Appointments).Complete,
		domain.AppointmentNoShow:    (*Appointments).NoShow,
	}

	for outcome, close := range outcomes {
		cases := []struct {
			name    string
			actor   func(h *apptHarness) Actor
			advance time.Duration
			status  domain.AppointmentStatus
			wantIs  error
		}{
			{name: "the doctor after the start", actor: func(h *apptHarness) Actor { return h.doctor }, advance: 22 * time.Hour},
			{name: "an admin after the end", actor: func(h *apptHarness) Actor { return h.admin }, advance: 48 * time.Hour},
			{name: "from confirmed", actor: func(h *apptHarness) Actor { return h.doctor }, advance: 22 * time.Hour, status: domain.AppointmentConfirmed},
			{name: "not before the visit starts", actor: func(h *apptHarness) Actor { return h.doctor }, wantIs: domain.ErrConflict},
			{name: "not by the patient", actor: func(h *apptHarness) Actor { return h.patient }, advance: 22 * time.Hour, wantIs: domain.ErrForbidden},
			{name: "not for a cancelled visit", actor: func(h *apptHarness) Actor { return h.doctor }, advance: 22 * time.Hour, status: domain.AppointmentCancelled, wantIs: domain.ErrConflict},
		}

		for _, tc := range cases {
			t.Run(string(outcome)+"/"+tc.name, func(t *testing.T) {
				h := newApptHarness(t)
				appt := h.book(h.patient, h.doctor, tomorrow10)
				if tc.status != "" {
					h.appts.setStatus(appt.ID, tc.status)
				}
				h.clock.Advance(tc.advance)

				got, err := close(h.service, context.Background(), tc.actor(h), appt.ID)

				if tc.wantIs != nil {
					assert.ErrorIs(t, err, tc.wantIs)
					return
				}
				require.NoError(t, err)
				assert.Equal(t, outcome, got.Status)
			})
		}
	}
}

// TestConcurrentOutcomesHaveOneWinner: a doctor double-clicking "completed" and "no-show" must
// not leave the visit flipping between the two; the compare-and-set lets exactly one land.
func TestConcurrentOutcomesHaveOneWinner(t *testing.T) {
	h := newApptHarness(t)
	appt := h.book(h.patient, h.doctor, tomorrow10)
	h.clock.Advance(22 * time.Hour)

	var (
		wg   sync.WaitGroup
		errs = make([]error, 2)
	)
	for i, close := range []func(context.Context, Actor, string) (domain.Appointment, error){h.service.Complete, h.service.NoShow} {
		wg.Add(1)
		go func(i int, close func(context.Context, Actor, string) (domain.Appointment, error)) {
			defer wg.Done()
			_, errs[i] = close(context.Background(), h.doctor, appt.ID)
		}(i, close)
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		assert.ErrorIs(t, err, domain.ErrConflict)
	}
	assert.Equal(t, 1, succeeded)
}

func TestTransitionLosesToAConcurrentWriter(t *testing.T) {
	h := newApptHarness(t)
	appt := h.book(h.patient, h.doctor, tomorrow10)
	h.appts.beforeWrite = func(a *domain.Appointment) { a.Status = domain.AppointmentCancelled }

	_, err := h.service.Confirm(context.Background(), h.doctor, appt.ID)

	assert.ErrorIs(t, err, domain.ErrConflict)
	assert.Equal(t, domain.AppointmentCancelled, h.appts.get(appt.ID).Status, "the concurrent cancellation must stand")
}

func TestReschedule(t *testing.T) {
	friday11 := clinicTime(2026, 10, 23, 11, 0)

	cases := []struct {
		name    string
		actor   func(h *apptHarness) Actor
		in      RescheduleInput
		status  domain.AppointmentStatus
		advance time.Duration
		long    bool
		wantEnd time.Time
		wantIs  error
		field   string
	}{
		{name: "a patient moves their visit", actor: func(h *apptHarness) Actor { return h.patient }, in: RescheduleInput{StartsAt: friday11}, wantEnd: friday11.Add(30 * time.Minute)},
		{name: "the length is kept when omitted", actor: func(h *apptHarness) Actor { return h.patient }, in: RescheduleInput{StartsAt: friday11}, long: true, wantEnd: friday11.Add(90 * time.Minute)},
		{name: "a new length may be given", actor: func(h *apptHarness) Actor { return h.patient }, in: RescheduleInput{StartsAt: friday11, Duration: time.Hour}, wantEnd: friday11.Add(time.Hour)},
		{name: "a confirmation is reset", actor: func(h *apptHarness) Actor { return h.admin }, in: RescheduleInput{StartsAt: friday11}, status: domain.AppointmentConfirmed, wantEnd: friday11.Add(30 * time.Minute)},
		{name: "an admin may move a visit in progress", actor: func(h *apptHarness) Actor { return h.admin }, in: RescheduleInput{StartsAt: friday11}, advance: 22*time.Hour + 5*time.Minute, wantEnd: friday11.Add(30 * time.Minute)},
		{name: "a patient cannot move a visit in progress", actor: func(h *apptHarness) Actor { return h.patient }, in: RescheduleInput{StartsAt: friday11}, advance: 22*time.Hour + 5*time.Minute, wantIs: domain.ErrConflict},
		{name: "a doctor does not reschedule", actor: func(h *apptHarness) Actor { return h.doctor }, in: RescheduleInput{StartsAt: friday11}, wantIs: domain.ErrForbidden},
		{name: "another patient cannot see it", actor: func(h *apptHarness) Actor { return h.patient2 }, in: RescheduleInput{StartsAt: friday11}, wantIs: domain.ErrNotFound},
		{name: "a cancelled visit stays cancelled", actor: func(h *apptHarness) Actor { return h.admin }, in: RescheduleInput{StartsAt: friday11}, status: domain.AppointmentCancelled, wantIs: domain.ErrConflict},
		{name: "the new time must be bookable", actor: func(h *apptHarness) Actor { return h.patient }, in: RescheduleInput{StartsAt: clinicTime(2026, 10, 25, 11, 0)}, field: "starts_at"},
		{name: "the new time is required", actor: func(h *apptHarness) Actor { return h.patient }, field: "starts_at"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newApptHarness(t)
			in := BookInput{DoctorID: h.doctor.UserID, StartsAt: tomorrow10}
			if tc.long {
				in.Duration = 90 * time.Minute
			}
			appt, err := h.service.Book(context.Background(), h.patient, in)
			require.NoError(t, err)
			if tc.status != "" {
				h.appts.setStatus(appt.ID, tc.status)
			}
			h.clock.Advance(tc.advance)

			got, err := h.service.Reschedule(context.Background(), tc.actor(h), appt.ID, tc.in)

			switch {
			case tc.field != "":
				requireField(t, err, tc.field)
			case tc.wantIs != nil:
				assert.ErrorIs(t, err, tc.wantIs)
			default:
				require.NoError(t, err)
				assert.Equal(t, tc.in.StartsAt, got.StartsAt)
				assert.Equal(t, tc.wantEnd, got.EndsAt)
				assert.Equal(t, domain.AppointmentScheduled, got.Status)
			}
		})
	}
}

func TestRescheduleIntoATakenSlotIsAConflict(t *testing.T) {
	h := newApptHarness(t)
	appt := h.book(h.patient, h.doctor, tomorrow10)
	h.book(h.patient2, h.doctor, tomorrow10.Add(2*time.Hour))

	_, err := h.service.Reschedule(context.Background(), h.patient, appt.ID, RescheduleInput{StartsAt: tomorrow10.Add(2 * time.Hour)})

	assert.ErrorIs(t, err, domain.ErrConflict)
	assert.Equal(t, tomorrow10, h.appts.get(appt.ID).StartsAt, "a refused move must leave the visit where it was")
}

func TestRescheduleLosesToAConcurrentMove(t *testing.T) {
	h := newApptHarness(t)
	appt := h.book(h.patient, h.doctor, tomorrow10)
	elsewhere := tomorrow10.Add(4 * time.Hour)
	h.appts.beforeWrite = func(a *domain.Appointment) {
		a.StartsAt, a.EndsAt = elsewhere, elsewhere.Add(30*time.Minute)
	}

	_, err := h.service.Reschedule(context.Background(), h.patient, appt.ID, RescheduleInput{StartsAt: tomorrow10.Add(time.Hour)})

	assert.ErrorIs(t, err, domain.ErrConflict)
	assert.Equal(t, elsewhere, h.appts.get(appt.ID).StartsAt)
}
