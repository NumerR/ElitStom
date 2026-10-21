package domain

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type AppointmentStatus string

const (
	AppointmentScheduled AppointmentStatus = "scheduled"
	AppointmentConfirmed AppointmentStatus = "confirmed"
	AppointmentCompleted AppointmentStatus = "completed"
	AppointmentCancelled AppointmentStatus = "cancelled"
	AppointmentNoShow    AppointmentStatus = "no_show"
)

// MaxNoteRunes bounds the free-text reason and cancellation note. The database enforces the
// same limit, so the two can never disagree about what fits.
const MaxNoteRunes = 500

func ParseAppointmentStatus(raw string) (AppointmentStatus, error) {
	status := AppointmentStatus(strings.ToLower(strings.TrimSpace(raw)))
	switch status {
	case AppointmentScheduled, AppointmentConfirmed, AppointmentCompleted, AppointmentCancelled, AppointmentNoShow:
		return status, nil
	default:
		return "", NewValidationError("status", "must be one of scheduled, confirmed, completed, cancelled, no_show")
	}
}

// IsActive reports whether the appointment still holds its time slot. Only active
// appointments take part in the overlap rules.
func (s AppointmentStatus) IsActive() bool {
	return s == AppointmentScheduled || s == AppointmentConfirmed
}

// CanTransitionTo encodes the lifecycle. Completed, cancelled and no-show are terminal:
// an appointment that has happened, or was released, is history and must not be revived.
func (s AppointmentStatus) CanTransitionTo(next AppointmentStatus) bool {
	switch s {
	case AppointmentScheduled:
		return next == AppointmentConfirmed || next == AppointmentCompleted ||
			next == AppointmentCancelled || next == AppointmentNoShow
	case AppointmentConfirmed:
		return next == AppointmentCompleted || next == AppointmentCancelled || next == AppointmentNoShow
	default:
		return false
	}
}

// Party is the public projection of an account taking part in an appointment: enough to
// display, nothing that identifies the person beyond their name.
type Party struct {
	ID       string
	FullName string
}

type Appointment struct {
	ID           string
	Patient      Party
	Doctor       Party
	StartsAt     time.Time
	EndsAt       time.Time
	Status       AppointmentStatus
	Reason       string
	CancelReason string
	CreatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (a Appointment) Interval() Interval { return Interval{Start: a.StartsAt, End: a.EndsAt} }

// Interval is a half-open time range [Start, End), matching the tstzrange the database uses
// for its overlap constraint: back-to-back appointments touch but do not overlap.
type Interval struct {
	Start time.Time
	End   time.Time
}

func (i Interval) Overlaps(other Interval) bool {
	return i.Start.Before(other.End) && other.Start.Before(i.End)
}

// NormalizeNote trims a free-text note and rejects control characters other than line
// breaks and tabs, which have no place in a clinical note and can corrupt log lines.
func NormalizeNote(field, raw string) (string, error) {
	note := strings.TrimSpace(raw)
	if !utf8.ValidString(note) {
		return "", NewValidationError(field, "must be valid UTF-8")
	}
	if utf8.RuneCountInString(note) > MaxNoteRunes {
		return "", NewValidationError(field, "must be at most 500 characters")
	}
	for _, r := range note {
		if unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' {
			return "", NewValidationError(field, "must not contain control characters")
		}
	}
	return note, nil
}
