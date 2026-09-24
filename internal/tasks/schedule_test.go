package tasks

import (
	"testing"
	"time"

	"github.com/shaktsin/ufoundry/internal/protocol"
)

func mustNext(t *testing.T, typ string, s protocol.Schedule, zone string, now time.Time, strict bool) time.Time {
	t.Helper()
	n, err := Next(typ, s, zone, now, strict)
	if err != nil || n == nil {
		t.Fatalf("Next(%+v) = %v, %v", s, n, err)
	}
	return *n
}

func TestNextPeriodic(t *testing.T) {
	la, _ := time.LoadLocation("America/Los_Angeles")
	now := time.Date(2026, 9, 17, 10, 30, 0, 0, la) // Thursday 10:30 PDT
	cases := []struct {
		s    protocol.Schedule
		want time.Time
	}{
		{protocol.Schedule{Frequency: "daily", Time: "09:00"}, time.Date(2026, 9, 18, 9, 0, 0, 0, la)},
		{protocol.Schedule{Frequency: "daily", Time: "11:00"}, time.Date(2026, 9, 17, 11, 0, 0, 0, la)},
		{protocol.Schedule{Frequency: "weekly", DayOfWeek: "mon", Time: "08:15"}, time.Date(2026, 9, 21, 8, 15, 0, 0, la)},
		{protocol.Schedule{Frequency: "weekly", DayOfWeek: "thu", Time: "10:00"}, time.Date(2026, 9, 24, 10, 0, 0, 0, la)},
		{protocol.Schedule{Frequency: "hourly", Minute: ptr(45)}, time.Date(2026, 9, 17, 10, 45, 0, 0, la)},
		{protocol.Schedule{Frequency: "hourly", Minute: ptr(15)}, time.Date(2026, 9, 17, 11, 15, 0, 0, la)},
		{protocol.Schedule{Frequency: "cron", Cron: "0 9 * * mon-fri"}, time.Date(2026, 9, 18, 9, 0, 0, 0, la)},
		{protocol.Schedule{Frequency: "cron", Cron: "*/20 * * * *"}, time.Date(2026, 9, 17, 10, 40, 0, 0, la)},
		{protocol.Schedule{Frequency: "cron", Cron: "0 0 1 jan *"}, time.Date(2027, 1, 1, 0, 0, 0, 0, la)},
	}
	for _, c := range cases {
		got := mustNext(t, protocol.TaskPeriodic, c.s, "America/Los_Angeles", now, true)
		if !got.Equal(c.want) {
			t.Errorf("%+v: got %s want %s", c.s, got.In(la), c.want)
		}
	}
	// Strict: a run exactly at now moves to the next slot.
	at9 := time.Date(2026, 9, 17, 9, 0, 0, 0, la)
	if got := mustNext(t, protocol.TaskPeriodic, protocol.Schedule{Frequency: "daily", Time: "09:00"}, "America/Los_Angeles", at9, true); !got.Equal(at9.AddDate(0, 0, 1)) {
		t.Errorf("strict daily = %s", got)
	}
	if got := mustNext(t, protocol.TaskPeriodic, protocol.Schedule{Frequency: "daily", Time: "09:00"}, "America/Los_Angeles", at9, false); !got.Equal(at9) {
		t.Errorf("non-strict daily = %s", got)
	}
	// DST: daily 09:00 across the November change stays 09:00 local.
	nov := time.Date(2026, 10, 31, 12, 0, 0, 0, la)
	got := mustNext(t, protocol.TaskPeriodic, protocol.Schedule{Frequency: "daily", Time: "09:00"}, "America/Los_Angeles", nov.AddDate(0, 0, 1), true)
	if got.In(la).Hour() != 9 {
		t.Errorf("DST daily = %s", got.In(la))
	}
}

func TestOneTimeAndValidate(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	got := mustNext(t, protocol.TaskOneTime, protocol.Schedule{RunAt: "2026-09-20T09:00"}, "Asia/Kolkata", now, false)
	if !got.Equal(time.Date(2026, 9, 20, 3, 30, 0, 0, time.UTC)) {
		t.Errorf("one_time local = %s", got)
	}
	got = mustNext(t, protocol.TaskOneTime, protocol.Schedule{RunAt: "2026-09-20T09:00:00Z"}, "Asia/Kolkata", now, false)
	if !got.Equal(time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("one_time utc = %s", got)
	}
	if n, _ := Next(protocol.TaskOneTime, protocol.Schedule{RunAt: "2026-09-01T09:00"}, "UTC", now, true); n != nil {
		t.Error("past one_time should have no next run")
	}
	bad := []struct {
		typ string
		s   protocol.Schedule
		tz  string
	}{
		{protocol.TaskOneTime, protocol.Schedule{}, "UTC"},
		{protocol.TaskPeriodic, protocol.Schedule{Frequency: "daily", Time: "25:00"}, "UTC"},
		{protocol.TaskPeriodic, protocol.Schedule{Frequency: "weekly", DayOfWeek: "funday"}, "UTC"},
		{protocol.TaskPeriodic, protocol.Schedule{Frequency: "cron", Cron: "* * *"}, "UTC"},
		{protocol.TaskPeriodic, protocol.Schedule{Frequency: "cron", Cron: "61 * * * *"}, "UTC"},
		{protocol.TaskPeriodic, protocol.Schedule{Frequency: "fortnightly"}, "UTC"},
		{protocol.TaskPeriodic, protocol.Schedule{Frequency: "daily"}, "Mars/Olympus"},
	}
	for _, b := range bad {
		if err := Validate(b.typ, b.s, b.tz); err == nil {
			t.Errorf("Validate(%s, %+v, %s) should fail", b.typ, b.s, b.tz)
		}
	}
}

func ptr(i int) *int { return &i }
