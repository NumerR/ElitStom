package rest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/platform/logger"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// testClockAt is 12:00 on Wednesday 2026-10-21 in Moscow, so tomorrow 10:00 is a bookable slot
// and this morning is already history.
const (
	tomorrow10 = "2026-10-22T07:00:00Z"
	tomorrow11 = "2026-10-22T08:00:00Z"
)

func testSchedule(t *testing.T) domain.Schedule {
	t.Helper()

	schedule, err := domain.NewSchedule("Europe/Moscow", "09:00", "20:00", []int{1, 2, 3, 4, 5, 6}, 30*time.Minute, 60)
	require.NoError(t, err)
	return schedule
}

// memoryAppointments enforces the two guarantees the database owns — no overlapping active
// bookings and compare-and-set transitions — so the HTTP layer is tested against the same
// conflicts it will see in production.
type memoryAppointments struct {
	users  *memoryUsers
	byID   map[string]domain.Appointment
	nextID int
}

func newMemoryAppointments(users *memoryUsers) *memoryAppointments {
	return &memoryAppointments{users: users, byID: map[string]domain.Appointment{}}
}

func (m *memoryAppointments) overlaps(appt domain.Appointment) bool {
	for id, other := range m.byID {
		if id == appt.ID || !other.Status.IsActive() {
			continue
		}
		sameParty := other.Doctor.ID == appt.Doctor.ID || other.Patient.ID == appt.Patient.ID
		if sameParty && other.Interval().Overlaps(appt.Interval()) {
			return true
		}
	}
	return false
}

func (m *memoryAppointments) name(id string) string {
	return m.users.byID[id].FullName
}

func (m *memoryAppointments) Create(_ context.Context, appt domain.Appointment) (domain.Appointment, error) {
	if appt.Status.IsActive() && m.overlaps(appt) {
		return domain.Appointment{}, fmt.Errorf("%w: appointment overlaps an existing booking", domain.ErrConflict)
	}

	m.nextID++
	appt.ID = fmt.Sprintf("00000000-0000-4000-a000-%012d", m.nextID)
	appt.Patient.FullName = m.name(appt.Patient.ID)
	appt.Doctor.FullName = m.name(appt.Doctor.ID)
	appt.CreatedAt = testClockAt
	appt.UpdatedAt = testClockAt
	m.byID[appt.ID] = appt
	return appt, nil
}

func (m *memoryAppointments) ByID(_ context.Context, id string) (domain.Appointment, error) {
	appt, ok := m.byID[id]
	if !ok {
		return domain.Appointment{}, fmt.Errorf("%w: appointment", domain.ErrNotFound)
	}
	return appt, nil
}

func (m *memoryAppointments) List(_ context.Context, filter usecase.AppointmentFilter) ([]domain.Appointment, error) {
	listed := make([]domain.Appointment, 0, len(m.byID))
	for _, appt := range m.byID {
		switch {
		case filter.PatientID != "" && appt.Patient.ID != filter.PatientID,
			filter.DoctorID != "" && appt.Doctor.ID != filter.DoctorID,
			filter.Status != "" && appt.Status != filter.Status,
			!filter.From.IsZero() && !appt.EndsAt.After(filter.From),
			!filter.To.IsZero() && !appt.StartsAt.Before(filter.To):
			continue
		}
		listed = append(listed, appt)
	}

	sort.Slice(listed, func(i, j int) bool {
		if filter.Descending {
			return listed[i].StartsAt.After(listed[j].StartsAt)
		}
		return listed[i].StartsAt.Before(listed[j].StartsAt)
	})

	if filter.Offset >= len(listed) {
		return []domain.Appointment{}, nil
	}
	listed = listed[filter.Offset:]
	if filter.Limit < len(listed) {
		listed = listed[:filter.Limit]
	}
	return listed, nil
}

func (m *memoryAppointments) Transition(_ context.Context, change usecase.StatusChange) (domain.Appointment, error) {
	appt, ok := m.byID[change.ID]
	if !ok || appt.Status != change.From {
		return domain.Appointment{}, fmt.Errorf("%w: appointment changed concurrently", domain.ErrConflict)
	}

	appt.Status = change.To
	if change.To == domain.AppointmentCancelled {
		appt.CancelReason = change.CancelReason
	}
	appt.UpdatedAt = testClockAt.Add(time.Second)
	m.byID[appt.ID] = appt
	return appt, nil
}

func (m *memoryAppointments) Reschedule(_ context.Context, change usecase.RescheduleChange) (domain.Appointment, error) {
	appt, ok := m.byID[change.ID]
	if !ok || appt.Status != change.From || !appt.StartsAt.Equal(change.FromStartsAt) {
		return domain.Appointment{}, fmt.Errorf("%w: appointment changed concurrently", domain.ErrConflict)
	}

	moved := appt
	moved.StartsAt, moved.EndsAt, moved.Status = change.StartsAt, change.EndsAt, domain.AppointmentScheduled
	if m.overlaps(moved) {
		return domain.Appointment{}, fmt.Errorf("%w: appointment overlaps an existing booking", domain.ErrConflict)
	}
	moved.UpdatedAt = testClockAt.Add(time.Second)
	m.byID[moved.ID] = moved
	return moved, nil
}

func (m *memoryAppointments) Busy(_ context.Context, doctorID string, window domain.Interval) ([]domain.Interval, error) {
	busy := []domain.Interval{}
	for _, appt := range m.byID {
		if appt.Doctor.ID == doctorID && appt.Status.IsActive() && appt.Interval().Overlaps(window) {
			busy = append(busy, appt.Interval())
		}
	}
	return busy, nil
}

// seed stores an appointment directly, bypassing the booking rules, so outcome endpoints can be
// exercised on a visit that already started without moving the clock.
func (m *memoryAppointments) seed(t *testing.T, patientID, doctorID string, startsAt time.Time, status domain.AppointmentStatus) string {
	t.Helper()

	appt, err := m.Create(context.Background(), domain.Appointment{
		Patient:   domain.Party{ID: patientID},
		Doctor:    domain.Party{ID: doctorID},
		StartsAt:  startsAt,
		EndsAt:    startsAt.Add(30 * time.Minute),
		Status:    status,
		CreatedBy: patientID,
	})
	require.NoError(t, err)
	return appt.ID
}

// clinic is the cast of an appointment scenario, each with a verified access token.
type clinic struct {
	*api
	adminID, admin       string
	doctorID, doctor     string
	doctor2ID, doctor2   string
	patientID, patient   string
	patient2ID, patient2 string
}

func newClinic(t *testing.T) *clinic {
	t.Helper()

	a := newAPI(t)
	c := &clinic{api: a}
	c.adminID, c.admin = a.adminToken()
	c.doctorID, c.doctor = a.seedAccount(domain.RoleDoctor, domain.StatusActive, "house@example.com")
	c.doctor2ID, c.doctor2 = a.seedAccount(domain.RoleDoctor, domain.StatusActive, "wilson@example.com")
	c.patientID, c.patient = a.seedAccount(domain.RolePatient, domain.StatusActive, "ada@example.com")
	c.patient2ID, c.patient2 = a.seedAccount(domain.RolePatient, domain.StatusActive, "grace@example.com")
	return c
}

// book creates a booking through the API as the first patient and returns its id.
func (c *clinic) book(startsAt string) string {
	c.t.Helper()

	rec := c.request(http.MethodPost, "/api/v1/appointments", jsonBody(c.t, map[string]any{
		"doctor_id": c.doctorID,
		"starts_at": startsAt,
		"reason":    "toothache",
	}), c.patient)
	require.Equalf(c.t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())

	id, ok := decodeMap(c.t, rec)["id"].(string)
	require.True(c.t, ok, "booking has no id")
	return id
}

func errorDetails(t *testing.T, rec *httptest.ResponseRecorder) []any {
	t.Helper()

	failure, ok := decodeMap(t, rec)["error"].(map[string]any)
	require.Truef(t, ok, "body has no error object: %s", rec.Body.String())
	details, _ := failure["details"].([]any)
	return details
}

func TestClinicScheduleIsPublishedToSignedInUsers(t *testing.T) {
	c := newClinic(t)

	rec := c.request(http.MethodGet, "/api/v1/clinic/schedule", "", c.patient)
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.JSONEq(t, `{
		"timezone": "Europe/Moscow",
		"opens_at": "09:00",
		"closes_at": "20:00",
		"workdays": [1, 2, 3, 4, 5, 6],
		"slot_minutes": 30,
		"booking_horizon_days": 60
	}`, rec.Body.String())

	anonymous := c.request(http.MethodGet, "/api/v1/clinic/schedule", "", "")
	assert.Equal(t, http.StatusUnauthorized, anonymous.Code)
}

func TestDoctorsListExposesOnlyNamesOfActiveDoctors(t *testing.T) {
	c := newClinic(t)
	c.seedAccount(domain.RoleDoctor, domain.StatusDisabled, "retired@example.com")

	rec := c.request(http.MethodGet, "/api/v1/doctors", "", c.patient)
	require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	doctors, ok := decodeMap(t, rec)["doctors"].([]any)
	require.True(t, ok)
	require.Len(t, doctors, 2, "the disabled doctor must not be offered for booking")

	ids := make([]string, 0, len(doctors))
	for _, raw := range doctors {
		doctor, ok := raw.(map[string]any)
		require.True(t, ok)
		assert.ElementsMatch(t, []string{"id", "full_name"}, keys(doctor),
			"a doctor entry must not carry contact details or account state")
		ids = append(ids, fmt.Sprint(doctor["id"]))
	}
	assert.ElementsMatch(t, []string{c.doctorID, c.doctor2ID}, ids)
}

func TestSlotsEndpoint(t *testing.T) {
	c := newClinic(t)
	c.book(tomorrow10)

	t.Run("a working day lists the free grid minus bookings", func(t *testing.T) {
		rec := c.request(http.MethodGet, "/api/v1/doctors/"+c.doctorID+"/slots?date=2026-10-22", "", c.patient2)
		require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

		body := decodeMap(t, rec)
		assert.Equal(t, "2026-10-22", body["date"])
		slots, ok := body["slots"].([]any)
		require.True(t, ok)
		assert.Len(t, slots, 21, "22 half-hour slots between 09:00 and 20:00, one taken")

		first, ok := slots[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "2026-10-22T06:00:00Z", first["starts_at"])
		assert.Equal(t, "2026-10-22T06:30:00Z", first["ends_at"])
		for _, raw := range slots {
			slot, ok := raw.(map[string]any)
			require.True(t, ok)
			assert.NotEqual(t, tomorrow10, slot["starts_at"], "the booked slot must not be offered")
		}
	})

	t.Run("a day off renders an empty array, not null", func(t *testing.T) {
		rec := c.request(http.MethodGet, "/api/v1/doctors/"+c.doctorID+"/slots?date=2026-10-25", "", c.patient)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"date":"2026-10-25","slots":[]}`, rec.Body.String())
	})

	rejections := []struct {
		name     string
		path     string
		wantCode int
		wantErr  string
	}{
		{name: "missing date", path: "/api/v1/doctors/" + c.doctorID + "/slots", wantCode: http.StatusBadRequest, wantErr: "invalid_input"},
		{name: "malformed date", path: "/api/v1/doctors/" + c.doctorID + "/slots?date=22.10.2026", wantCode: http.StatusBadRequest, wantErr: "invalid_input"},
		{name: "a patient is not a doctor", path: "/api/v1/doctors/" + c.patientID + "/slots?date=2026-10-22", wantCode: http.StatusNotFound, wantErr: "not_found"},
		{name: "unknown doctor", path: "/api/v1/doctors/00000000-0000-4000-8000-999999999999/slots?date=2026-10-22", wantCode: http.StatusNotFound, wantErr: "not_found"},
	}
	for _, tc := range rejections {
		t.Run(tc.name, func(t *testing.T) {
			rec := c.request(http.MethodGet, tc.path, "", c.patient)
			assert.Equalf(t, tc.wantCode, rec.Code, "body: %s", rec.Body.String())
			assert.Equal(t, tc.wantErr, errorCode(t, rec))
		})
	}
}

func TestBookingRendersTheAppointment(t *testing.T) {
	c := newClinic(t)

	rec := c.request(http.MethodPost, "/api/v1/appointments", jsonBody(t, map[string]any{
		"doctor_id":        c.doctorID,
		"starts_at":        "2026-10-22T10:00:00+03:00",
		"duration_minutes": 60,
		"reason":           "  check-up  ",
	}), c.patient)
	require.Equalf(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())

	body := decodeMap(t, rec)
	assert.ElementsMatch(t, []string{
		"id", "patient", "doctor", "starts_at", "ends_at", "status",
		"reason", "cancel_reason", "created_at", "updated_at",
	}, keys(body), "the appointment shape is a contract with the web client")
	assert.Equal(t, tomorrow10, body["starts_at"], "times are rendered in UTC whatever offset the client sent")
	assert.Equal(t, tomorrow11, body["ends_at"])
	assert.Equal(t, "scheduled", body["status"])
	assert.Equal(t, "check-up", body["reason"])
	assert.Equal(t, map[string]any{"id": c.patientID, "full_name": "Seed Account"}, body["patient"])
	assert.Equal(t, map[string]any{"id": c.doctorID, "full_name": "Seed Account"}, body["doctor"])
}

func TestBookingRules(t *testing.T) {
	c := newClinic(t)
	c.book(tomorrow10)

	cases := []struct {
		name       string
		bearer     func() string
		body       map[string]any
		wantCode   int
		wantErr    string
		wantDetail string
	}{
		{
			name:     "the same slot is taken",
			bearer:   func() string { return c.patient2 },
			body:     map[string]any{"doctor_id": c.doctorID, "starts_at": tomorrow10, "reason": ""},
			wantCode: http.StatusConflict, wantErr: "conflict",
		},
		{
			name:     "a patient cannot book for somebody else",
			bearer:   func() string { return c.patient2 },
			body:     map[string]any{"doctor_id": c.doctor2ID, "patient_id": c.patientID, "starts_at": tomorrow11},
			wantCode: http.StatusForbidden, wantErr: "forbidden",
		},
		{
			name:     "a doctor cannot book",
			bearer:   func() string { return c.doctor },
			body:     map[string]any{"doctor_id": c.doctorID, "patient_id": c.patientID, "starts_at": tomorrow11},
			wantCode: http.StatusForbidden, wantErr: "forbidden",
		},
		{
			name:       "an admin must name the patient",
			bearer:     func() string { return c.admin },
			body:       map[string]any{"doctor_id": c.doctorID, "starts_at": tomorrow11},
			wantCode:   http.StatusBadRequest,
			wantErr:    "invalid_input",
			wantDetail: "patient_id",
		},
		{
			name:       "starts_at must be RFC 3339",
			bearer:     func() string { return c.patient2 },
			body:       map[string]any{"doctor_id": c.doctorID, "starts_at": "tomorrow at ten"},
			wantCode:   http.StatusBadRequest,
			wantErr:    "invalid_input",
			wantDetail: "starts_at: must be an RFC 3339 timestamp",
		},
		{
			name:       "starts_at is required",
			bearer:     func() string { return c.patient2 },
			body:       map[string]any{"doctor_id": c.doctorID},
			wantCode:   http.StatusBadRequest,
			wantErr:    "invalid_input",
			wantDetail: "starts_at: is required",
		},
		{
			name:       "a zero duration is not a request for the default",
			bearer:     func() string { return c.patient2 },
			body:       map[string]any{"doctor_id": c.doctorID, "starts_at": tomorrow11, "duration_minutes": 0},
			wantCode:   http.StatusBadRequest,
			wantErr:    "invalid_input",
			wantDetail: "duration_minutes",
		},
		{
			name:       "an overflowing duration is refused before arithmetic",
			bearer:     func() string { return c.patient2 },
			body:       map[string]any{"doctor_id": c.doctorID, "starts_at": tomorrow11, "duration_minutes": 1 << 40},
			wantCode:   http.StatusBadRequest,
			wantErr:    "invalid_input",
			wantDetail: "duration_minutes",
		},
		{
			name:       "the duration must be whole slots",
			bearer:     func() string { return c.patient2 },
			body:       map[string]any{"doctor_id": c.doctorID, "starts_at": tomorrow11, "duration_minutes": 45},
			wantCode:   http.StatusBadRequest,
			wantErr:    "invalid_input",
			wantDetail: "duration_minutes",
		},
		{
			name:       "the past is not bookable",
			bearer:     func() string { return c.patient2 },
			body:       map[string]any{"doctor_id": c.doctorID, "starts_at": "2026-10-21T06:00:00Z"},
			wantCode:   http.StatusBadRequest,
			wantErr:    "invalid_input",
			wantDetail: "starts_at: must be in the future",
		},
		{
			name:       "an unknown field is refused, not ignored",
			bearer:     func() string { return c.patient2 },
			body:       map[string]any{"doctor_id": c.doctorID, "starts_at": tomorrow11, "status": "confirmed"},
			wantCode:   http.StatusBadRequest,
			wantErr:    "invalid_input",
			wantDetail: "status",
		},
		{
			name:       "the doctor must be a doctor",
			bearer:     func() string { return c.patient2 },
			body:       map[string]any{"doctor_id": c.patientID, "starts_at": tomorrow11},
			wantCode:   http.StatusBadRequest,
			wantErr:    "invalid_input",
			wantDetail: "doctor_id: must reference an active doctor",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := c.request(http.MethodPost, "/api/v1/appointments", jsonBody(t, tc.body), tc.bearer())

			require.Equalf(t, tc.wantCode, rec.Code, "body: %s", rec.Body.String())
			assert.Equal(t, tc.wantErr, errorCode(t, rec))
			if tc.wantDetail != "" {
				assert.Containsf(t, fmt.Sprint(errorDetails(t, rec)), tc.wantDetail, "body: %s", rec.Body.String())
			}
		})
	}

	t.Run("an admin books on a patient's behalf", func(t *testing.T) {
		rec := c.request(http.MethodPost, "/api/v1/appointments", jsonBody(t, map[string]any{
			"doctor_id": c.doctor2ID, "patient_id": c.patient2ID, "starts_at": tomorrow10,
		}), c.admin)
		require.Equalf(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())
		assert.Equal(t, c.patient2ID, decodeMap(t, rec)["patient"].(map[string]any)["id"])
	})
}

func TestAppointmentListing(t *testing.T) {
	c := newClinic(t)
	mine := c.book(tomorrow10)
	c.appointments.seed(t, c.patient2ID, c.doctor2ID, mustTime(t, tomorrow11), domain.AppointmentScheduled)

	listIDs := func(t *testing.T, path, bearer string) []string {
		t.Helper()

		rec := c.request(http.MethodGet, path, "", bearer)
		require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		listed, ok := decodeMap(t, rec)["appointments"].([]any)
		require.True(t, ok, "appointments must be an array, never null")

		ids := make([]string, 0, len(listed))
		for _, raw := range listed {
			appt, ok := raw.(map[string]any)
			require.True(t, ok)
			ids = append(ids, fmt.Sprint(appt["id"]))
		}
		return ids
	}

	t.Run("each role sees its own scope", func(t *testing.T) {
		assert.Equal(t, []string{mine}, listIDs(t, "/api/v1/appointments", c.patient))
		assert.Equal(t, []string{mine}, listIDs(t, "/api/v1/appointments", c.doctor))
		assert.Len(t, listIDs(t, "/api/v1/appointments", c.admin), 2)
		assert.Empty(t, listIDs(t, "/api/v1/appointments?status=cancelled", c.admin))
	})

	t.Run("filters and paging reach the store", func(t *testing.T) {
		all := listIDs(t, "/api/v1/appointments?order=desc", c.admin)
		require.Len(t, all, 2)
		assert.NotEqual(t, mine, all[0], "descending order puts the later visit first")

		assert.Equal(t, []string{mine}, listIDs(t, "/api/v1/appointments?limit=1", c.admin))
		assert.Equal(t, []string{mine}, listIDs(t, "/api/v1/appointments?to="+tomorrow11, c.admin))
		assert.Equal(t, []string{mine}, listIDs(t, "/api/v1/appointments?doctor_id="+c.doctorID, c.admin))

		rec := c.request(http.MethodGet, "/api/v1/appointments?limit=1000&offset=1", "", c.admin)
		require.Equal(t, http.StatusOK, rec.Code)
		body := decodeMap(t, rec)
		assert.EqualValues(t, 200, body["limit"], "the page size is clamped by the use case")
		assert.EqualValues(t, 1, body["offset"])
	})

	rejections := []struct {
		name       string
		query      string
		bearer     func() string
		wantCode   int
		wantDetail string
	}{
		{name: "unknown status", query: "?status=lost", bearer: func() string { return c.admin }, wantCode: http.StatusBadRequest, wantDetail: "status"},
		{name: "malformed from", query: "?from=yesterday", bearer: func() string { return c.admin }, wantCode: http.StatusBadRequest, wantDetail: "from"},
		{name: "malformed to", query: "?to=2026-10-22", bearer: func() string { return c.admin }, wantCode: http.StatusBadRequest, wantDetail: "to"},
		{name: "unknown order", query: "?order=sideways", bearer: func() string { return c.admin }, wantCode: http.StatusBadRequest, wantDetail: "order"},
		{name: "an empty range", query: "?from=" + tomorrow11 + "&to=" + tomorrow10, bearer: func() string { return c.admin }, wantCode: http.StatusBadRequest, wantDetail: "to: must be later than from"},
		{name: "a patient asking for someone else", query: "?patient_id=" + c.patient2ID, bearer: func() string { return c.patient }, wantCode: http.StatusForbidden},
		{name: "a doctor asking for a colleague", query: "?doctor_id=" + c.doctor2ID, bearer: func() string { return c.doctor }, wantCode: http.StatusForbidden},
	}
	for _, tc := range rejections {
		t.Run(tc.name, func(t *testing.T) {
			rec := c.request(http.MethodGet, "/api/v1/appointments"+tc.query, "", tc.bearer())
			require.Equalf(t, tc.wantCode, rec.Code, "body: %s", rec.Body.String())
			if tc.wantDetail != "" {
				assert.Contains(t, fmt.Sprint(errorDetails(t, rec)), tc.wantDetail)
			}
		})
	}
}

func TestAppointmentDetailIsHiddenFromStrangers(t *testing.T) {
	c := newClinic(t)
	id := c.book(tomorrow10)

	for name, bearer := range map[string]string{"patient": c.patient, "doctor": c.doctor, "admin": c.admin} {
		rec := c.request(http.MethodGet, "/api/v1/appointments/"+id, "", bearer)
		assert.Equalf(t, http.StatusOK, rec.Code, "%s must see the visit: %s", name, rec.Body.String())
	}

	for name, bearer := range map[string]string{"other patient": c.patient2, "other doctor": c.doctor2} {
		rec := c.request(http.MethodGet, "/api/v1/appointments/"+id, "", bearer)
		assert.Equalf(t, http.StatusNotFound, rec.Code, "%s must not learn the visit exists", name)
	}
}

func TestAppointmentLifecycleEndpoints(t *testing.T) {
	c := newClinic(t)
	past := mustTime(t, "2026-10-21T07:00:00Z") // 10:00 this morning in Moscow

	cases := []struct {
		name       string
		setup      func(t *testing.T) string
		action     string
		body       string
		bearer     func() string
		wantCode   int
		wantStatus string
	}{
		{
			name:     "the doctor confirms",
			setup:    func(*testing.T) string { return c.book(tomorrow10) },
			action:   "confirm",
			bearer:   func() string { return c.doctor },
			wantCode: http.StatusOK, wantStatus: "confirmed",
		},
		{
			name:     "a patient cannot confirm",
			setup:    func(*testing.T) string { return c.book(tomorrow10) },
			action:   "confirm",
			bearer:   func() string { return c.patient },
			wantCode: http.StatusForbidden,
		},
		{
			name:     "the patient cancels with a note",
			setup:    func(*testing.T) string { return c.book(tomorrow10) },
			action:   "cancel",
			body:     `{"reason":"feeling better"}`,
			bearer:   func() string { return c.patient },
			wantCode: http.StatusOK, wantStatus: "cancelled",
		},
		{
			name:     "cancel requires a JSON body",
			setup:    func(*testing.T) string { return c.book(tomorrow10) },
			action:   "cancel",
			bearer:   func() string { return c.patient },
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "a visit that has not started cannot be completed",
			setup:    func(*testing.T) string { return c.book(tomorrow10) },
			action:   "complete",
			bearer:   func() string { return c.doctor },
			wantCode: http.StatusConflict,
		},
		{
			name: "the doctor completes a visit that started",
			setup: func(t *testing.T) string {
				return c.appointments.seed(t, c.patientID, c.doctorID, past, domain.AppointmentConfirmed)
			},
			action:   "complete",
			bearer:   func() string { return c.doctor },
			wantCode: http.StatusOK, wantStatus: "completed",
		},
		{
			name: "the doctor records a no-show",
			setup: func(t *testing.T) string {
				return c.appointments.seed(t, c.patientID, c.doctorID, past.Add(time.Hour), domain.AppointmentScheduled)
			},
			action:   "no-show",
			bearer:   func() string { return c.doctor },
			wantCode: http.StatusOK, wantStatus: "no_show",
		},
		{
			name: "a closed visit cannot be revived",
			setup: func(t *testing.T) string {
				return c.appointments.seed(t, c.patientID, c.doctorID, past.Add(-time.Hour), domain.AppointmentCompleted)
			},
			action:   "no-show",
			bearer:   func() string { return c.admin },
			wantCode: http.StatusConflict,
		},
		{
			name:     "the patient moves the visit",
			setup:    func(*testing.T) string { return c.book(tomorrow10) },
			action:   "reschedule",
			body:     `{"starts_at":"` + tomorrow11 + `"}`,
			bearer:   func() string { return c.patient },
			wantCode: http.StatusOK, wantStatus: "scheduled",
		},
		{
			name:     "reschedule validates the new start at the boundary",
			setup:    func(*testing.T) string { return c.book(tomorrow10) },
			action:   "reschedule",
			body:     `{"starts_at":"soon","duration_minutes":-30}`,
			bearer:   func() string { return c.patient },
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "a doctor cannot move a visit",
			setup:    func(*testing.T) string { return c.book(tomorrow10) },
			action:   "reschedule",
			body:     `{"starts_at":"` + tomorrow11 + `"}`,
			bearer:   func() string { return c.doctor },
			wantCode: http.StatusForbidden,
		},
		{
			name:     "a stranger gets not found, not forbidden",
			setup:    func(*testing.T) string { return c.book(tomorrow10) },
			action:   "cancel",
			body:     `{"reason":""}`,
			bearer:   func() string { return c.patient2 },
			wantCode: http.StatusNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Every case starts from an empty calendar so bookings of earlier cases cannot collide.
			clear(c.appointments.byID)
			id := tc.setup(t)

			rec := c.request(http.MethodPost, "/api/v1/appointments/"+id+"/"+tc.action, tc.body, tc.bearer())
			require.Equalf(t, tc.wantCode, rec.Code, "body: %s", rec.Body.String())
			if tc.wantStatus != "" {
				assert.Equal(t, tc.wantStatus, decodeMap(t, rec)["status"])
			}
		})
	}

	t.Run("cancel records the trimmed note and reschedule the new times", func(t *testing.T) {
		clear(c.appointments.byID)
		moved := c.book(tomorrow10)
		rec := c.request(http.MethodPost, "/api/v1/appointments/"+moved+"/reschedule",
			`{"starts_at":"`+tomorrow11+`","duration_minutes":60}`, c.admin)
		require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		body := decodeMap(t, rec)
		assert.Equal(t, tomorrow11, body["starts_at"])
		assert.Equal(t, "2026-10-22T09:00:00Z", body["ends_at"])

		rec = c.request(http.MethodPost, "/api/v1/appointments/"+moved+"/cancel", `{"reason":"  away  "}`, c.patient)
		require.Equalf(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.Equal(t, "away", decodeMap(t, rec)["cancel_reason"])
	})
}

func TestAppointmentRoutesRequireAuthentication(t *testing.T) {
	a := newAPI(t)

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/doctors"},
		{http.MethodGet, "/api/v1/doctors/00000000-0000-4000-8000-000000000001/slots?date=2026-10-22"},
		{http.MethodGet, "/api/v1/appointments"},
		{http.MethodPost, "/api/v1/appointments"},
		{http.MethodGet, "/api/v1/appointments/00000000-0000-4000-a000-000000000001"},
		{http.MethodPost, "/api/v1/appointments/00000000-0000-4000-a000-000000000001/cancel"},
		{http.MethodPost, "/api/v1/appointments/00000000-0000-4000-a000-000000000001/confirm"},
		{http.MethodPost, "/api/v1/appointments/00000000-0000-4000-a000-000000000001/complete"},
		{http.MethodPost, "/api/v1/appointments/00000000-0000-4000-a000-000000000001/no-show"},
		{http.MethodPost, "/api/v1/appointments/00000000-0000-4000-a000-000000000001/reschedule"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			rec := a.request(route.method, route.path, "", "")
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

func TestRouterRequiresTheAppointmentsService(t *testing.T) {
	a := newAPI(t)
	authService, err := usecase.NewAuth(a.users, a.sessions, a.issuer, stubHasher{}, a.secrets, fixedClock{}, usecase.AuthConfig{
		AccessTokenTTL: accessTokenTTL, RefreshTokenTTL: refreshTTL,
	})
	require.NoError(t, err)
	userService, err := usecase.NewUsers(a.users, a.sessions, stubHasher{}, fixedClock{})
	require.NoError(t, err)

	assert.PanicsWithValue(t, "rest: NewRouter is missing appointments service", func() {
		NewRouter(Deps{DB: readyPinger{}, Log: logger.NewNop(), Tokens: a.issuer, Auth: authService, Users: userService})
	})
}

func mustTime(t *testing.T, raw string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, raw)
	require.NoError(t, err)
	return parsed
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}
