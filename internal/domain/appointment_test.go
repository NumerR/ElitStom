package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAppointmentStatus(t *testing.T) {
	tests := []struct {
		raw     string
		want    AppointmentStatus
		wantErr bool
	}{
		{raw: "scheduled", want: AppointmentScheduled},
		{raw: " Confirmed ", want: AppointmentConfirmed},
		{raw: "COMPLETED", want: AppointmentCompleted},
		{raw: "cancelled", want: AppointmentCancelled},
		{raw: "no_show", want: AppointmentNoShow},
		{raw: "", wantErr: true},
		{raw: "canceled", wantErr: true},
		{raw: "no-show", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := ParseAppointmentStatus(tt.raw)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidInput)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAppointmentLifecycle(t *testing.T) {
	all := []AppointmentStatus{AppointmentScheduled, AppointmentConfirmed, AppointmentCompleted, AppointmentCancelled, AppointmentNoShow}
	allowed := map[AppointmentStatus][]AppointmentStatus{
		AppointmentScheduled: {AppointmentConfirmed, AppointmentCompleted, AppointmentCancelled, AppointmentNoShow},
		AppointmentConfirmed: {AppointmentCompleted, AppointmentCancelled, AppointmentNoShow},
	}

	for _, from := range all {
		for _, to := range all {
			want := false
			for _, ok := range allowed[from] {
				want = want || ok == to
			}
			assert.Equalf(t, want, from.CanTransitionTo(to), "%s -> %s", from, to)
		}
	}

	assert.True(t, AppointmentScheduled.IsActive())
	assert.True(t, AppointmentConfirmed.IsActive())
	for _, terminal := range []AppointmentStatus{AppointmentCompleted, AppointmentCancelled, AppointmentNoShow} {
		assert.Falsef(t, terminal.IsActive(), "%s must release its slot", terminal)
	}
}

func TestIntervalOverlaps(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 23, h, m, 0, 0, time.UTC) }
	base := Interval{Start: at(10, 0), End: at(10, 30)}

	tests := []struct {
		name  string
		other Interval
		want  bool
	}{
		{name: "identical", other: base, want: true},
		{name: "inside", other: Interval{at(10, 10), at(10, 20)}, want: true},
		{name: "covers", other: Interval{at(9, 0), at(11, 0)}, want: true},
		{name: "tail overlap", other: Interval{at(10, 29), at(11, 0)}, want: true},
		{name: "back to back after", other: Interval{at(10, 30), at(11, 0)}, want: false},
		{name: "back to back before", other: Interval{at(9, 30), at(10, 0)}, want: false},
		{name: "disjoint", other: Interval{at(12, 0), at(12, 30)}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, base.Overlaps(tt.other))
			assert.Equal(t, tt.want, tt.other.Overlaps(base), "overlap must be symmetric")
		})
	}
}

func TestNormalizeNote(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "empty is allowed", raw: "", want: ""},
		{name: "trimmed", raw: "  зубная боль  ", want: "зубная боль"},
		{name: "line breaks kept", raw: "line one\nline two", want: "line one\nline two"},
		{name: "exactly the limit", raw: strings.Repeat("я", MaxNoteRunes), want: strings.Repeat("я", MaxNoteRunes)},
		{name: "over the limit", raw: strings.Repeat("я", MaxNoteRunes+1), wantErr: "at most 500"},
		{name: "control character", raw: "ok\x00bad", wantErr: "control characters"},
		{name: "escape sequence", raw: "\x1b[31mred", wantErr: "control characters"},
		{name: "invalid utf-8", raw: "\xff\xfe", wantErr: "valid UTF-8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeNote("reason", tt.raw)
			if tt.wantErr != "" {
				require.ErrorIs(t, err, ErrInvalidInput)
				assert.Contains(t, err.Error(), "reason: ")
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
