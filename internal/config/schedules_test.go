package config

import (
	"testing"
	"time"
)

func TestScheduleNext(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip(err)
	}
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, c := range []struct {
		s          Schedule
		from, want string
	}{
		{Schedule{Repeat: RepeatDaily, At: "09:00"}, "2026-10-30 08:00", "2026-10-30 09:00"},
		{Schedule{Repeat: RepeatDaily, At: "09:00"}, "2026-10-30 09:00", "2026-10-31 09:00"},
		// Friday evening to Monday morning.
		{Schedule{Repeat: RepeatWeekdays, At: "09:00"}, "2026-10-30 18:00", "2026-11-02 09:00"},
		// Wednesday is 3.
		{Schedule{Repeat: RepeatWeekly, At: "07:30", Weekday: 3}, "2026-10-30 12:00", "2026-11-04 07:30"},
		{Schedule{Repeat: RepeatHourly, At: "00:15"}, "2026-10-30 10:20", "2026-10-30 11:15"},
		// Summer time ends on 2026-10-25: the wall clock stays at 09:00.
		{Schedule{Repeat: RepeatDaily, At: "09:00"}, "2026-10-24 10:00", "2026-10-25 09:00"},
	} {
		if got := c.s.Next(at(c.from)); !got.Equal(at(c.want)) {
			t.Errorf("%s %s from %s = %s, want %s", c.s.Repeat, c.s.At, c.from, got, c.want)
		}
	}
}

func TestScheduleDue(t *testing.T) {
	created := time.Date(2026, 10, 30, 8, 0, 0, 0, time.Local)
	s := Schedule{Repeat: RepeatDaily, At: "09:00", Enabled: true, CreatedAt: created}
	if s.Due(created.Add(30 * time.Minute)) {
		t.Error("due before its first time")
	}
	if !s.Due(created.Add(time.Hour)) {
		t.Error("not due at its time")
	}
	// Three missed days make one run.
	s.LastRun = created.Add(time.Hour)
	if !s.Due(created.Add(72*time.Hour)) || s.Due(created.Add(90*time.Minute)) {
		t.Error("wrong after a run")
	}
	s.Enabled = false
	if s.Due(created.Add(72 * time.Hour)) {
		t.Error("due while off")
	}
	for _, bad := range []Schedule{
		{Name: "x", Prompt: "p", ProjectRoot: "/p", Repeat: "monthly", At: "09:00"},
		{Name: "x", Prompt: "p", ProjectRoot: "/p", Repeat: RepeatDaily, At: "9am"},
		{Name: "x", Prompt: "p", ProjectRoot: "rel", Repeat: RepeatDaily, At: "09:00"},
		{Name: " ", Prompt: "p", ProjectRoot: "/p", Repeat: RepeatDaily, At: "09:00"},
	} {
		if bad.Validate() == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}
