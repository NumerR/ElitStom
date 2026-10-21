package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// MaxAppointmentSlots caps one booking at a few consecutive slots; a longer treatment is
// booked as several visits. The database independently caps an appointment at 8 hours.
const MaxAppointmentSlots = 8

// ClockTime is a wall-clock time of day in the clinic's timezone.
type ClockTime struct {
	Hour   int
	Minute int
}

func ParseClockTime(field, raw string) (ClockTime, error) {
	hh, mm, ok := strings.Cut(strings.TrimSpace(raw), ":")
	hour, hourErr := strconv.Atoi(hh)
	minute, minuteErr := strconv.Atoi(mm)
	if !ok || len(hh) != 2 || len(mm) != 2 || hourErr != nil || minuteErr != nil ||
		hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return ClockTime{}, NewValidationError(field, "must be a time like 09:00")
	}
	return ClockTime{Hour: hour, Minute: minute}, nil
}

func (c ClockTime) String() string { return fmt.Sprintf("%02d:%02d", c.Hour, c.Minute) }

func (c ClockTime) minutes() int { return c.Hour*60 + c.Minute }

// Schedule is the clinic's booking calendar: opening hours on working days, cut into slots.
// Wall-clock times are resolved per calendar day with time.Date, so opening hours stay at
// 09:00 local time across a DST change instead of drifting by an hour.
type Schedule struct {
	location *time.Location
	opens    ClockTime
	closes   ClockTime
	workdays [8]bool // indexed by ISO weekday, 1 = Monday … 7 = Sunday
	slot     time.Duration
	horizon  int
}

func NewSchedule(timezone, opens, closes string, workdays []int, slot time.Duration, horizonDays int) (Schedule, error) {
	ve := &ValidationError{}
	s := Schedule{slot: slot, horizon: horizonDays}

	loc, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil || strings.TrimSpace(timezone) == "" {
		ve.Add("timezone", "must be an IANA zone such as Europe/Moscow")
	}
	s.location = loc

	var openErr, closeErr error
	s.opens, openErr = ParseClockTime("opens_at", opens)
	s.closes, closeErr = ParseClockTime("closes_at", closes)
	for _, e := range []error{openErr, closeErr} {
		if fieldErr, ok := e.(*ValidationError); ok {
			ve.Fields = append(ve.Fields, fieldErr.Fields...)
		}
	}
	if openErr == nil && closeErr == nil && s.closes.minutes() <= s.opens.minutes() {
		ve.Add("closes_at", "must be later than opens_at")
	}

	if len(workdays) == 0 {
		ve.Add("workdays", "must name at least one ISO weekday")
	}
	for _, day := range workdays {
		if day < 1 || day > 7 {
			ve.Add("workdays", "must be ISO weekdays between 1 (Monday) and 7 (Sunday)")
			break
		}
		s.workdays[day] = true
	}

	switch {
	case slot < 5*time.Minute || slot > 4*time.Hour || slot%time.Minute != 0:
		ve.Add("slot", "must be a whole number of minutes between 5m and 4h")
	case openErr == nil && closeErr == nil && s.closes.minutes() > s.opens.minutes() &&
		(s.closes.minutes()-s.opens.minutes())%int(slot/time.Minute) != 0:
		ve.Add("slot", "must divide the opening hours evenly")
	}

	if horizonDays < 1 || horizonDays > 365 {
		ve.Add("booking_horizon_days", "must be between 1 and 365")
	}

	if len(ve.Fields) > 0 {
		return Schedule{}, ve
	}
	return s, nil
}

func (s Schedule) Location() *time.Location { return s.location }
func (s Schedule) Opens() ClockTime         { return s.opens }
func (s Schedule) Closes() ClockTime        { return s.closes }
func (s Schedule) Slot() time.Duration      { return s.slot }
func (s Schedule) HorizonDays() int         { return s.horizon }

func (s Schedule) Workdays() []int {
	days := make([]int, 0, 7)
	for day := 1; day <= 7; day++ {
		if s.workdays[day] {
			days = append(days, day)
		}
	}
	return days
}

// ParseDate reads a calendar date in the clinic's timezone and returns its local midnight.
func (s Schedule) ParseDate(raw string) (time.Time, error) {
	day, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(raw), s.location)
	if err != nil {
		return time.Time{}, NewValidationError("date", "must be a date like 2026-10-23")
	}
	return day, nil
}

// Day returns the whole local calendar day containing t, which is 23 or 25 hours long on a
// DST change.
func (s Schedule) Day(t time.Time) Interval {
	local := t.In(s.location)
	y, m, d := local.Date()
	return Interval{
		Start: time.Date(y, m, d, 0, 0, 0, 0, s.location),
		End:   time.Date(y, m, d+1, 0, 0, 0, 0, s.location),
	}
}

func (s Schedule) IsWorkday(t time.Time) bool {
	weekday := int(t.In(s.location).Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return s.workdays[weekday]
}

func (s Schedule) openingHours(t time.Time) Interval {
	y, m, d := t.In(s.location).Date()
	return Interval{
		Start: time.Date(y, m, d, s.opens.Hour, s.opens.Minute, 0, 0, s.location),
		End:   time.Date(y, m, d, s.closes.Hour, s.closes.Minute, 0, 0, s.location),
	}
}

// Slots lists every slot of the local day containing day, regardless of bookings.
func (s Schedule) Slots(day time.Time) []Interval {
	if !s.IsWorkday(day) {
		return nil
	}
	hours := s.openingHours(day)
	slots := make([]Interval, 0, int(hours.End.Sub(hours.Start)/s.slot))
	for start := hours.Start; !start.Add(s.slot).After(hours.End); start = start.Add(s.slot) {
		slots = append(slots, Interval{Start: start.UTC(), End: start.Add(s.slot).UTC()})
	}
	return slots
}

// BookingWindow is the period a new booking may start in: after now and no further ahead
// than the horizon.
func (s Schedule) BookingWindow(now time.Time) Interval {
	return Interval{Start: now, End: now.AddDate(0, 0, s.horizon)}
}

// CheckBookable validates a requested appointment against the calendar: a working day,
// inside opening hours, on the slot grid, a whole number of slots, in the bookable future.
func (s Schedule) CheckBookable(want Interval, now time.Time) error {
	length := want.End.Sub(want.Start)
	if length <= 0 || length%s.slot != 0 || length > s.slot*MaxAppointmentSlots {
		return NewValidationError("duration_minutes",
			fmt.Sprintf("must be 1 to %d slots of %d minutes", MaxAppointmentSlots, int(s.slot/time.Minute)))
	}

	window := s.BookingWindow(now)
	if !want.Start.After(window.Start) {
		return NewValidationError("starts_at", "must be in the future")
	}
	if !want.Start.Before(window.End) {
		return NewValidationError("starts_at", fmt.Sprintf("must be within %d days", s.horizon))
	}

	if !s.IsWorkday(want.Start) {
		return NewValidationError("starts_at", "must fall on a working day")
	}
	hours := s.openingHours(want.Start)
	if want.Start.Before(hours.Start) || want.End.After(hours.End) {
		return NewValidationError("starts_at",
			fmt.Sprintf("must fit within opening hours %s–%s", s.opens, s.closes))
	}
	if want.Start.Sub(hours.Start)%s.slot != 0 {
		return NewValidationError("starts_at", "must be aligned to the slot grid")
	}
	return nil
}
