package usecase

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/NumerR/ElitStom/internal/domain"
)

// fakeAppointments mirrors the SQL adapter's guarantees: the exclusion constraint (no two live
// bookings of one doctor or one patient overlap) and the compare-and-set on status changes.
type fakeAppointments struct {
	mu     sync.Mutex
	byID   map[string]domain.Appointment
	nextID int

	createErr error
	listErr   error
	busyErr   error
	lastList  AppointmentFilter
	listCalls int
	lastBusy  domain.Interval

	// beforeWrite runs inside Transition and Reschedule before the compare-and-set, standing in
	// for another request that commits between the use case's read and its write.
	beforeWrite func(appt *domain.Appointment)
}

func newFakeAppointments() *fakeAppointments {
	return &fakeAppointments{byID: map[string]domain.Appointment{}}
}

func (f *fakeAppointments) Create(_ context.Context, appt domain.Appointment) (domain.Appointment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.createErr != nil {
		return domain.Appointment{}, f.createErr
	}
	if f.lockedOverlaps(appt, "") {
		return domain.Appointment{}, fmt.Errorf("%w: appointment overlaps an existing booking", domain.ErrConflict)
	}
	f.nextID++
	appt.ID = fmt.Sprintf("appt-%d", f.nextID)
	f.byID[appt.ID] = appt
	return appt, nil
}

func (f *fakeAppointments) ByID(_ context.Context, id string) (domain.Appointment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	appt, ok := f.byID[id]
	if !ok {
		return domain.Appointment{}, fmt.Errorf("%w: appointment", domain.ErrNotFound)
	}
	return appt, nil
}

func (f *fakeAppointments) List(_ context.Context, filter AppointmentFilter) ([]domain.Appointment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.listCalls++
	f.lastList = filter
	if f.listErr != nil {
		return nil, f.listErr
	}

	listed := []domain.Appointment{}
	for _, appt := range f.byID {
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
	return listed, nil
}

func (f *fakeAppointments) Transition(_ context.Context, change StatusChange) (domain.Appointment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	appt, ok := f.byID[change.ID]
	if f.beforeWrite != nil && ok {
		f.beforeWrite(&appt)
		f.byID[change.ID] = appt
	}
	if !ok || appt.Status != change.From {
		return domain.Appointment{}, fmt.Errorf("%w: appointment changed concurrently", domain.ErrConflict)
	}
	appt.Status = change.To
	if change.To == domain.AppointmentCancelled {
		appt.CancelReason = change.CancelReason
	}
	f.byID[change.ID] = appt
	return appt, nil
}

func (f *fakeAppointments) Reschedule(_ context.Context, change RescheduleChange) (domain.Appointment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	appt, ok := f.byID[change.ID]
	if f.beforeWrite != nil && ok {
		f.beforeWrite(&appt)
		f.byID[change.ID] = appt
	}
	if !ok || appt.Status != change.From || !appt.StartsAt.Equal(change.FromStartsAt) {
		return domain.Appointment{}, fmt.Errorf("%w: appointment changed concurrently", domain.ErrConflict)
	}
	moved := appt
	moved.StartsAt, moved.EndsAt, moved.Status = change.StartsAt, change.EndsAt, domain.AppointmentScheduled
	if f.lockedOverlaps(moved, appt.ID) {
		return domain.Appointment{}, fmt.Errorf("%w: appointment overlaps an existing booking", domain.ErrConflict)
	}
	f.byID[change.ID] = moved
	return moved, nil
}

func (f *fakeAppointments) Busy(_ context.Context, doctorID string, window domain.Interval) ([]domain.Interval, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.lastBusy = window
	if f.busyErr != nil {
		return nil, f.busyErr
	}
	busy := []domain.Interval{}
	for _, appt := range f.byID {
		if appt.Doctor.ID == doctorID && appt.Status.IsActive() && appt.Interval().Overlaps(window) {
			busy = append(busy, appt.Interval())
		}
	}
	return busy, nil
}

func (f *fakeAppointments) lockedOverlaps(appt domain.Appointment, except string) bool {
	for id, other := range f.byID {
		if id == except || !other.Status.IsActive() {
			continue
		}
		sameParty := other.Doctor.ID == appt.Doctor.ID || other.Patient.ID == appt.Patient.ID
		if sameParty && other.Interval().Overlaps(appt.Interval()) {
			return true
		}
	}
	return false
}

func (f *fakeAppointments) get(id string) domain.Appointment {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byID[id]
}

func (f *fakeAppointments) setStatus(id string, status domain.AppointmentStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()

	appt := f.byID[id]
	appt.Status = status
	f.byID[id] = appt
}

var _ AppointmentRepository = (*fakeAppointments)(nil)

// clinicTime builds an instant from Moscow wall-clock time, which is how the clinic thinks.
func clinicTime(year int, month time.Month, day, hour, minute int) time.Time {
	moscow, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		panic(err)
	}
	return time.Date(year, month, day, hour, minute, 0, 0, moscow).UTC()
}
