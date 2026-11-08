package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SchedulesFile holds scheduled and recurring sessions (SES-012). The app
// writes each run's time into it, so it is apart from the YAML files the
// app watches; the engine reads it on every check, so a hand edit applies
// within a minute.
const SchedulesFile = "schedules.json"

// Repeat values for a schedule.
const (
	RepeatHourly   = "hourly"
	RepeatDaily    = "daily"
	RepeatWeekdays = "weekdays"
	RepeatWeekly   = "weekly"
)

// Schedule starts a new session with a prompt at set times (SES-012).
type Schedule struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
	// ProjectRoot is the folder the sessions run in; the profile,
	// provider, model and mode are as for a new session.
	ProjectRoot string `json:"projectRoot"`
	ProfileID   string `json:"profileId,omitempty"`
	ProviderID  string `json:"providerId,omitempty"`
	Model       string `json:"model,omitempty"`
	Mode        string `json:"mode,omitempty"`
	// Repeat is hourly, daily, weekdays or weekly. At is the local time
	// "15:04"; hourly uses only its minutes. Weekday (0 is Sunday) is for
	// weekly.
	Repeat  string `json:"repeat"`
	At      string `json:"at"`
	Weekday int    `json:"weekday,omitempty"`
	Enabled bool   `json:"enabled"`
	// AFK, when set, starts an AFK run of the project's backlog instead of a
	// session (WFL-005), on ProfileID, with the commands the user allowed for its
	// sessions when setting the schedule up.
	AFK *ScheduleAFK `json:"afk,omitempty"`
	// CreatedAt counts as the last run until there is one, so a new
	// schedule waits for its first time.
	CreatedAt   time.Time `json:"createdAt"`
	LastRun     time.Time `json:"lastRun,omitzero"`
	LastSession string    `json:"lastSession,omitempty"`
	LastError   string    `json:"lastError,omitempty"`
}

// ScheduleAFK is the AFK run a schedule starts.
type ScheduleAFK struct {
	WorkflowID      string   `json:"workflowId,omitempty"`
	ReviewProfileID string   `json:"reviewProfileId,omitempty"`
	Allow           []string `json:"allow,omitempty"`
}

// Validate reports the first setting that cannot work.
func (s Schedule) Validate() error {
	switch {
	case strings.TrimSpace(s.Name) == "":
		return errors.New("a schedule needs a name")
	case strings.TrimSpace(s.Prompt) == "" && s.AFK == nil:
		return errors.New("a schedule needs a prompt")
	case !filepath.IsAbs(s.ProjectRoot):
		return errors.New("a schedule needs a project folder")
	case s.Repeat != RepeatHourly && s.Repeat != RepeatDaily && s.Repeat != RepeatWeekdays && s.Repeat != RepeatWeekly:
		return fmt.Errorf("unknown repeat %q: use hourly, daily, weekdays or weekly", s.Repeat)
	case s.Weekday < 0 || s.Weekday > 6:
		return errors.New("the weekday is 0 (Sunday) to 6 (Saturday)")
	}
	if _, _, err := clock(s.At); err != nil {
		return err
	}
	return nil
}

func clock(at string) (hour, minute int, err error) {
	t, err := time.Parse("15:04", at)
	if err != nil {
		return 0, 0, fmt.Errorf("the time %q is not HH:MM", at)
	}
	return t.Hour(), t.Minute(), nil
}

// Next returns the first time after from that the schedule runs, in from's
// location.
func (s Schedule) Next(from time.Time) time.Time {
	hour, minute, err := clock(s.At)
	if err != nil {
		return time.Time{}
	}
	if s.Repeat == RepeatHourly {
		t := time.Date(from.Year(), from.Month(), from.Day(), from.Hour(), minute, 0, 0, from.Location())
		if !t.After(from) {
			t = t.Add(time.Hour)
		}
		return t
	}
	// A day at a time, so daylight saving changes keep the wall clock.
	for d := 0; d <= 8; d++ {
		day := from.AddDate(0, 0, d)
		t := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, from.Location())
		if !t.After(from) {
			continue
		}
		wd := t.Weekday()
		switch s.Repeat {
		case RepeatWeekdays:
			if wd == time.Saturday || wd == time.Sunday {
				continue
			}
		case RepeatWeekly:
			if int(wd) != s.Weekday {
				continue
			}
		}
		return t
	}
	return time.Time{}
}

// Due reports whether the schedule should run at now: a time it runs at
// has passed since its last run. Runs missed while the app was closed
// make one run, not one each.
func (s Schedule) Due(now time.Time) bool {
	last := s.LastRun
	if last.IsZero() {
		last = s.CreatedAt
	}
	next := s.Next(last.In(now.Location()))
	return s.Enabled && !next.IsZero() && !next.After(now)
}

// LoadSchedules returns the schedules, none when the file is missing.
func LoadSchedules(dir string) ([]Schedule, error) {
	b, err := os.ReadFile(filepath.Join(dir, SchedulesFile))
	if errors.Is(err, os.ErrNotExist) {
		return []Schedule{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Schedule
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", SchedulesFile, err)
	}
	if out == nil {
		out = []Schedule{}
	}
	return out, nil
}

// SaveSchedules replaces the schedules file.
func SaveSchedules(dir string, list []Schedule) error {
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, SchedulesFile), b)
}
