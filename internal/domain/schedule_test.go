package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 2026-10-21 is a Wednesday; 12:00 in Moscow (UTC+3, no DST).
var scheduleNow = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)

func moscowSchedule(t *testing.T) Schedule {
	t.Helper()
	s, err := NewSchedule("Europe/Moscow", "09:00", "20:00", []int{1, 2, 3, 4, 5, 6}, 30*time.Minute, 60)
	require.NoError(t, err)
	return s
}

func TestNewScheduleRejectsInconsistentCalendars(t *testing.T) {
	tests := []struct {
		name      string
		timezone  string
		opens     string
		closes    string
		workdays  []int
		slot      time.Duration
		horizon   int
		wantField string
	}{
		{name: "unknown zone", timezone: "Mars/Olympus", opens: "09:00", closes: "18:00", workdays: []int{1}, slot: 30 * time.Minute, horizon: 30, wantField: "timezone"},
		{name: "blank zone", timezone: " ", opens: "09:00", closes: "18:00", workdays: []int{1}, slot: 30 * time.Minute, horizon: 30, wantField: "timezone"},
		{name: "malformed opening", timezone: "UTC", opens: "9am", closes: "18:00", workdays: []int{1}, slot: 30 * time.Minute, horizon: 30, wantField: "opens_at"},
		{name: "hour out of range", timezone: "UTC", opens: "09:00", closes: "24:00", workdays: []int{1}, slot: 30 * time.Minute, horizon: 30, wantField: "closes_at"},
		{name: "closes before opening", timezone: "UTC", opens: "18:00", closes: "09:00", workdays: []int{1}, slot: 30 * time.Minute, horizon: 30, wantField: "closes_at"},
		{name: "no workdays", timezone: "UTC", opens: "09:00", closes: "18:00", workdays: nil, slot: 30 * time.Minute, horizon: 30, wantField: "workdays"},
		{name: "weekday zero", timezone: "UTC", opens: "09:00", closes: "18:00", workdays: []int{0, 1}, slot: 30 * time.Minute, horizon: 30, wantField: "workdays"},
		{name: "slot too short", timezone: "UTC", opens: "09:00", closes: "18:00", workdays: []int{1}, slot: time.Minute, horizon: 30, wantField: "slot"},
		{name: "fractional slot", timezone: "UTC", opens: "09:00", closes: "18:00", workdays: []int{1}, slot: 90 * time.Second, horizon: 30, wantField: "slot"},
		{name: "slot does not divide the day", timezone: "UTC", opens: "09:00", closes: "18:00", workdays: []int{1}, slot: 40 * time.Minute, horizon: 30, wantField: "slot"},
		{name: "no horizon", timezone: "UTC", opens: "09:00", closes: "18:00", workdays: []int{1}, slot: 30 * time.Minute, horizon: 0, wantField: "booking_horizon_days"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSchedule(tt.timezone, tt.opens, tt.closes, tt.workdays, tt.slot, tt.horizon)
			require.ErrorIs(t, err, ErrInvalidInput)
			var ve *ValidationError
			require.ErrorAs(t, err, &ve)
			fields := make([]string, 0, len(ve.Fields))
			for _, f := range ve.Fields {
				fields = append(fields, f.Field)
			}
			assert.Contains(t, fields, tt.wantField)
		})
	}
}

func TestScheduleAccessors(t *testing.T) {
	s, err := NewSchedule("Europe/Moscow", "09:00", "20:00", []int{6, 1, 3}, 30*time.Minute, 60)
	require.NoError(t, err)

	assert.Equal(t, "Europe/Moscow", s.Location().String())
	assert.Equal(t, "09:00", s.Opens().String())
	assert.Equal(t, "20:00", s.Closes().String())
	assert.Equal(t, []int{1, 3, 6}, s.Workdays(), "workdays are reported in calendar order")
	assert.Equal(t, 30*time.Minute, s.Slot())
	assert.Equal(t, 60, s.HorizonDays())
}

func TestScheduleParseDate(t *testing.T) {
	s := moscowSchedule(t)

	day, err := s.ParseDate("2026-10-23")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 10, 22, 21, 0, 0, 0, time.UTC), day.UTC(), "midnight in Moscow is 21:00 UTC the day before")

	for _, raw := range []string{"", "23.10.2026", "2026-13-01", "2026-10-23T10:00:00Z"} {
		_, err := s.ParseDate(raw)
		assert.ErrorIsf(t, err, ErrInvalidInput, "%q must be rejected", raw)
	}
}

func TestScheduleSlots(t *testing.T) {
	s := moscowSchedule(t)

	friday := time.Date(2026, 10, 23, 12, 0, 0, 0, s.Location())
	slots := s.Slots(friday)
	require.Len(t, slots, 22, "09:00–20:00 in 30-minute slots")
	assert.Equal(t, time.Date(2026, 10, 23, 6, 0, 0, 0, time.UTC), slots[0].Start)
	assert.Equal(t, time.Date(2026, 10, 23, 17, 0, 0, 0, time.UTC), slots[len(slots)-1].End)
	for i := 1; i < len(slots); i++ {
		assert.Equal(t, slots[i-1].End, slots[i].Start, "slots must tile the day without gaps")
	}

	sunday := time.Date(2026, 10, 25, 12, 0, 0, 0, s.Location())
	assert.Empty(t, s.Slots(sunday), "a day off has no slots")
}

// Berlin leaves summer time on 2026-10-25. Opening hours are wall-clock times, so 09:00 must
// stay 09:00 locally while its UTC instant moves by an hour.
func TestScheduleSurvivesADaylightSavingChange(t *testing.T) {
	s, err := NewSchedule("Europe/Berlin", "09:00", "12:00", []int{6, 7}, time.Hour, 30)
	require.NoError(t, err)

	saturday := s.Slots(time.Date(2026, 10, 24, 12, 0, 0, 0, s.Location()))
	sunday := s.Slots(time.Date(2026, 10, 25, 12, 0, 0, 0, s.Location()))
	require.Len(t, saturday, 3)
	require.Len(t, sunday, 3)

	assert.Equal(t, time.Date(2026, 10, 24, 7, 0, 0, 0, time.UTC), saturday[0].Start, "CEST is UTC+2")
	assert.Equal(t, time.Date(2026, 10, 25, 8, 0, 0, 0, time.UTC), sunday[0].Start, "CET is UTC+1")

	day := s.Day(time.Date(2026, 10, 25, 12, 0, 0, 0, s.Location()))
	assert.Equal(t, 25*time.Hour, day.End.Sub(day.Start), "the change-over day is 25 hours long")
}

func TestScheduleCheckBookable(t *testing.T) {
	s := moscowSchedule(t)
	msk := s.Location()
	at := func(month time.Month, day, hour, minute int) time.Time {
		return time.Date(2026, month, day, hour, minute, 0, 0, msk)
	}
	slot := 30 * time.Minute

	tests := []struct {
		name      string
		start     time.Time
		length    time.Duration
		wantField string
		wantText  string
	}{
		{name: "one slot on a working day", start: at(10, 23, 10, 0), length: slot},
		{name: "two consecutive slots", start: at(10, 23, 10, 0), length: 2 * slot},
		{name: "the last slot of the day", start: at(10, 23, 19, 30), length: slot},
		{name: "later today", start: at(10, 21, 12, 30), length: slot},
		{name: "zero length", start: at(10, 23, 10, 0), length: 0, wantField: "duration_minutes"},
		{name: "not a whole slot", start: at(10, 23, 10, 0), length: 45 * time.Minute, wantField: "duration_minutes"},
		{name: "too many slots", start: at(10, 23, 10, 0), length: 9 * slot, wantField: "duration_minutes"},
		{name: "in the past", start: at(10, 20, 10, 0), length: slot, wantField: "starts_at", wantText: "future"},
		{name: "exactly now", start: scheduleNow, length: slot, wantField: "starts_at", wantText: "future"},
		{name: "beyond the horizon", start: at(12, 21, 10, 0), length: slot, wantField: "starts_at", wantText: "within 60 days"},
		{name: "on a day off", start: at(10, 25, 10, 0), length: slot, wantField: "starts_at", wantText: "working day"},
		{name: "before opening", start: at(10, 23, 8, 30), length: slot, wantField: "starts_at", wantText: "opening hours"},
		{name: "runs past closing", start: at(10, 23, 19, 30), length: 2 * slot, wantField: "starts_at", wantText: "opening hours"},
		{name: "off the slot grid", start: at(10, 23, 10, 15), length: slot, wantField: "starts_at", wantText: "slot grid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.CheckBookable(Interval{Start: tt.start, End: tt.start.Add(tt.length)}, scheduleNow)
			if tt.wantField == "" {
				require.NoError(t, err)
				return
			}
			var ve *ValidationError
			require.ErrorAs(t, err, &ve)
			require.Len(t, ve.Fields, 1)
			assert.Equal(t, tt.wantField, ve.Fields[0].Field)
			assert.Contains(t, ve.Fields[0].Message, tt.wantText)
		})
	}
}
