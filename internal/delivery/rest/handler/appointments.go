package handler

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// maxDurationMinutes rejects absurd durations before they are multiplied into a time.Duration,
// where a large enough value would overflow into a negative or wrapped length.
const maxDurationMinutes = 24 * 60

// Appointments serves the clinic calendar, the doctor directory and the booking lifecycle.
type Appointments struct {
	*Responder
	appts *usecase.Appointments
}

func NewAppointments(appts *usecase.Appointments, log *slog.Logger) *Appointments {
	return &Appointments{Responder: NewResponder(log), appts: appts}
}

// Schedule publishes the clinic calendar so a client can render the same grid the server
// validates against, instead of hard-coding opening hours.
func (h *Appointments) Schedule(w http.ResponseWriter, r *http.Request) {
	s := h.appts.Schedule()
	h.JSON(w, r, http.StatusOK, scheduleResponse{
		Timezone:           s.Location().String(),
		OpensAt:            s.Opens().String(),
		ClosesAt:           s.Closes().String(),
		Workdays:           s.Workdays(),
		SlotMinutes:        int(s.Slot() / time.Minute),
		BookingHorizonDays: s.HorizonDays(),
	})
}

func (h *Appointments) Doctors(w http.ResponseWriter, r *http.Request) {
	actor, err := h.actor(r)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	doctors, err := h.appts.Doctors(r.Context(), actor)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	out := make([]partyResponse, 0, len(doctors))
	for _, doctor := range doctors {
		out = append(out, newPartyResponse(doctor))
	}
	h.JSON(w, r, http.StatusOK, doctorsResponse{Doctors: out})
}

func (h *Appointments) Slots(w http.ResponseWriter, r *http.Request) {
	actor, err := h.actor(r)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		h.FromError(w, r, domain.NewValidationError("date", "is required"))
		return
	}

	slots, err := h.appts.FreeSlots(r.Context(), actor, chi.URLParam(r, "id"), date)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	out := make([]slotResponse, 0, len(slots))
	for _, slot := range slots {
		out = append(out, slotResponse{StartsAt: slot.Start, EndsAt: slot.End})
	}
	h.JSON(w, r, http.StatusOK, slotsResponse{Date: date, Slots: out})
}

func (h *Appointments) Book(w http.ResponseWriter, r *http.Request) {
	actor, err := h.actor(r)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	var in bookRequest
	if err := h.decode(r, &in); err != nil {
		h.FromError(w, r, err)
		return
	}

	startsAt, duration, err := parseSlotRequest(in.StartsAt, in.DurationMinutes)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	appt, err := h.appts.Book(r.Context(), actor, usecase.BookInput{
		DoctorID:  strings.TrimSpace(in.DoctorID),
		PatientID: strings.TrimSpace(in.PatientID),
		StartsAt:  startsAt,
		Duration:  duration,
		Reason:    in.Reason,
	})
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	h.JSON(w, r, http.StatusCreated, newAppointmentResponse(appt))
}

// List validates every filter here, so an unknown status or a malformed timestamp is a 400
// naming the parameter, never a silently ignored predicate that widens the result.
func (h *Appointments) List(w http.ResponseWriter, r *http.Request) {
	actor, err := h.actor(r)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	filter, err := parseAppointmentFilter(r.URL.Query())
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	page, err := h.appts.List(r.Context(), actor, filter)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	out := make([]appointmentResponse, 0, len(page.Appointments))
	for _, appt := range page.Appointments {
		out = append(out, newAppointmentResponse(appt))
	}
	h.JSON(w, r, http.StatusOK, listAppointmentsResponse{Appointments: out, Limit: page.Limit, Offset: page.Offset})
}

func (h *Appointments) Get(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, func(actor usecase.Actor, id string) (domain.Appointment, error) {
		return h.appts.Get(r.Context(), actor, id)
	})
}

func (h *Appointments) Cancel(w http.ResponseWriter, r *http.Request) {
	var in cancelRequest
	if err := h.decode(r, &in); err != nil {
		h.FromError(w, r, err)
		return
	}

	h.act(w, r, func(actor usecase.Actor, id string) (domain.Appointment, error) {
		return h.appts.Cancel(r.Context(), actor, id, in.Reason)
	})
}

func (h *Appointments) Confirm(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, func(actor usecase.Actor, id string) (domain.Appointment, error) {
		return h.appts.Confirm(r.Context(), actor, id)
	})
}

func (h *Appointments) Complete(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, func(actor usecase.Actor, id string) (domain.Appointment, error) {
		return h.appts.Complete(r.Context(), actor, id)
	})
}

func (h *Appointments) NoShow(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, func(actor usecase.Actor, id string) (domain.Appointment, error) {
		return h.appts.NoShow(r.Context(), actor, id)
	})
}

func (h *Appointments) Reschedule(w http.ResponseWriter, r *http.Request) {
	var in rescheduleRequest
	if err := h.decode(r, &in); err != nil {
		h.FromError(w, r, err)
		return
	}

	startsAt, duration, err := parseSlotRequest(in.StartsAt, in.DurationMinutes)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	h.act(w, r, func(actor usecase.Actor, id string) (domain.Appointment, error) {
		return h.appts.Reschedule(r.Context(), actor, id, usecase.RescheduleInput{StartsAt: startsAt, Duration: duration})
	})
}

// act runs an operation on the appointment named by the path and renders the result. The
// lifecycle endpoints differ only in the use-case call, so the envelope lives in one place.
func (h *Appointments) act(w http.ResponseWriter, r *http.Request, op func(usecase.Actor, string) (domain.Appointment, error)) {
	actor, err := h.actor(r)
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	appt, err := op(actor, chi.URLParam(r, "id"))
	if err != nil {
		h.FromError(w, r, err)
		return
	}

	h.JSON(w, r, http.StatusOK, newAppointmentResponse(appt))
}

// parseSlotRequest reads a requested start and length. A missing duration stays 0 so the use
// case applies its own default; a supplied one must be a positive, bounded number of minutes.
func parseSlotRequest(rawStart string, minutes *int) (time.Time, time.Duration, error) {
	errs := &domain.ValidationError{}

	var startsAt time.Time
	if raw := strings.TrimSpace(rawStart); raw == "" {
		errs.Add("starts_at", "is required")
	} else if parsed, err := time.Parse(time.RFC3339, raw); err != nil {
		errs.Add("starts_at", "must be an RFC 3339 timestamp")
	} else {
		startsAt = parsed
	}

	var duration time.Duration
	if minutes != nil {
		if *minutes <= 0 || *minutes > maxDurationMinutes {
			errs.Add("duration_minutes", "must be a positive number of minutes")
		} else {
			duration = time.Duration(*minutes) * time.Minute
		}
	}

	if len(errs.Fields) > 0 {
		return time.Time{}, 0, errs
	}
	return startsAt, duration, nil
}

func parseAppointmentFilter(query url.Values) (usecase.AppointmentFilter, error) {
	errs := &domain.ValidationError{}
	filter := usecase.AppointmentFilter{
		PatientID: strings.TrimSpace(query.Get("patient_id")),
		DoctorID:  strings.TrimSpace(query.Get("doctor_id")),
	}

	if raw := strings.TrimSpace(query.Get("status")); raw != "" {
		status, err := domain.ParseAppointmentStatus(raw)
		if err != nil {
			errs.Add("status", "must be one of scheduled, confirmed, completed, cancelled, no_show")
		}
		filter.Status = status
	}

	filter.From = optionalTime(query, "from", errs)
	filter.To = optionalTime(query, "to", errs)

	switch strings.ToLower(strings.TrimSpace(query.Get("order"))) {
	case "", "asc":
	case "desc":
		filter.Descending = true
	default:
		errs.Add("order", "must be asc or desc")
	}

	filter.Limit, filter.Offset = paging(query)

	if len(errs.Fields) > 0 {
		return usecase.AppointmentFilter{}, errs
	}
	return filter, nil
}

func optionalTime(query url.Values, name string, errs *domain.ValidationError) time.Time {
	raw := strings.TrimSpace(query.Get(name))
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		errs.Add(name, "must be an RFC 3339 timestamp")
		return time.Time{}
	}
	return parsed
}
