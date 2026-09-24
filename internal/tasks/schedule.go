// Package tasks runs scheduled tasks: one-time and periodic prompts that the
// engine executes as turns. Schedules use the Python app's JSON format, plus
// cron expressions.
package tasks

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // IANA zones even where the system has none

	"github.com/shaktsin/ufoundry/internal/protocol"
)

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "sunday": time.Sunday, "mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tuesday": time.Tuesday, "wed": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thursday": time.Thursday, "fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
}

// LoadZone resolves an IANA zone name ("" = the engine's local zone).
func LoadZone(name string) (*time.Location, error) {
	if name == "" || strings.EqualFold(name, "local") {
		return time.Local, nil
	}
	return time.LoadLocation(name)
}

// ZoneName returns a storable name for loc.
func ZoneName(loc *time.Location) string {
	if loc == time.Local {
		if tz := localZoneName(); tz != "" {
			return tz
		}
	}
	return loc.String()
}

// Validate checks a task type and schedule.
func Validate(taskType string, s protocol.Schedule, zone string) error {
	loc, err := LoadZone(zone)
	if err != nil {
		return fmt.Errorf("unknown timezone %q", zone)
	}
	switch taskType {
	case protocol.TaskOneTime:
		if _, err := parseRunAt(s.RunAt, loc); err != nil {
			return err
		}
	case protocol.TaskPeriodic:
		switch strings.ToLower(s.Frequency) {
		case "hourly":
			if s.Minute != nil && (*s.Minute < 0 || *s.Minute > 59) {
				return errors.New("minute must be 0–59")
			}
		case "daily", "":
			if _, _, err := parseHHMM(s.Time); err != nil {
				return err
			}
		case "weekly":
			if _, _, err := parseHHMM(s.Time); err != nil {
				return err
			}
			if _, ok := weekdays[strings.ToLower(orDefault(s.DayOfWeek, "mon"))]; !ok {
				return fmt.Errorf("unknown day_of_week %q", s.DayOfWeek)
			}
		case "cron":
			if _, err := ParseCron(s.Cron); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown frequency %q (hourly, daily, weekly or cron)", s.Frequency)
		}
	default:
		return fmt.Errorf("unknown task type %q (one_time or periodic)", taskType)
	}
	return nil
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

// Next returns the next run time strictly after (or, if !strict, at or after)
// now, or nil if the task has no further runs.
func Next(taskType string, s protocol.Schedule, zone string, now time.Time, strict bool) (*time.Time, error) {
	loc, err := LoadZone(zone)
	if err != nil {
		return nil, err
	}
	local := now.In(loc)
	after := func(c time.Time) bool {
		if strict {
			return c.After(now)
		}
		return !c.Before(now)
	}
	switch taskType {
	case protocol.TaskOneTime:
		at, err := parseRunAt(s.RunAt, loc)
		if err != nil {
			return nil, err
		}
		if strict && !at.After(now) {
			return nil, nil
		}
		return &at, nil
	case protocol.TaskPeriodic:
	default:
		return nil, fmt.Errorf("unknown task type %q", taskType)
	}
	var c time.Time
	switch strings.ToLower(orDefault(s.Frequency, "daily")) {
	case "hourly":
		m := 0
		if s.Minute != nil {
			m = *s.Minute
		}
		c = time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), m, 0, 0, loc)
		if !after(c) {
			c = c.Add(time.Hour)
		}
	case "daily":
		h, m, err := parseHHMM(s.Time)
		if err != nil {
			return nil, err
		}
		c = time.Date(local.Year(), local.Month(), local.Day(), h, m, 0, 0, loc)
		if !after(c) {
			c = c.AddDate(0, 0, 1)
		}
	case "weekly":
		h, m, err := parseHHMM(s.Time)
		if err != nil {
			return nil, err
		}
		wd := weekdays[strings.ToLower(orDefault(s.DayOfWeek, "mon"))]
		c = time.Date(local.Year(), local.Month(), local.Day(), h, m, 0, 0, loc)
		c = c.AddDate(0, 0, (int(wd)-int(c.Weekday())+7)%7)
		if !after(c) {
			c = c.AddDate(0, 0, 7)
		}
	case "cron":
		spec, err := ParseCron(s.Cron)
		if err != nil {
			return nil, err
		}
		start := now
		if !strict {
			start = now.Add(-time.Minute)
		}
		n, ok := spec.Next(start.In(loc))
		if !ok {
			return nil, nil
		}
		c = n
	default:
		return nil, fmt.Errorf("unknown frequency %q", s.Frequency)
	}
	u := c.UTC()
	return &u, nil
}

// parseHHMM parses "HH:MM" (default 09:00 when empty).
func parseHHMM(s string) (int, int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 9, 0, nil
	}
	hs, ms, ok := strings.Cut(s, ":")
	h, err1 := strconv.Atoi(hs)
	m, err2 := strconv.Atoi(ms)
	if !ok || err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("time must be HH:MM, got %q", s)
	}
	return h, m, nil
}

// parseRunAt accepts RFC 3339 or a local date-time ("2026-09-20T09:00", "2026-09-20 09:00").
func parseRunAt(s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("run_at is required for a one-time task")
	}
	if t, err := time.Parse(time.RFC3339, strings.Replace(s, "Z", "+00:00", 1)); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("run_at %q is not a date-time like 2026-09-20T09:00", s)
}

// ---- cron ----

// Cron is a parsed 5-field cron expression (minute hour day-of-month month day-of-week).
type Cron struct {
	min, hour, dom, month, dow [64]bool
	domStar, dowStar           bool
}

// ParseCron parses "m h dom mon dow". Supports *, lists, ranges, steps and
// month/day names; "@hourly", "@daily", "@weekly", "@monthly" shortcuts.
func ParseCron(expr string) (*Cron, error) {
	expr = strings.TrimSpace(expr)
	switch expr {
	case "@hourly":
		expr = "0 * * * *"
	case "@daily", "@midnight":
		expr = "0 0 * * *"
	case "@weekly":
		expr = "0 0 * * 0"
	case "@monthly":
		expr = "0 0 1 * *"
	}
	f := strings.Fields(expr)
	if len(f) != 5 {
		return nil, fmt.Errorf("cron %q: want 5 fields (minute hour day month weekday)", expr)
	}
	c := &Cron{domStar: f[2] == "*", dowStar: f[4] == "*"}
	months := map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}
	days := map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	specs := []struct {
		set      *[64]bool
		lo, hi   int
		names    map[string]int
		fieldIdx int
	}{{&c.min, 0, 59, nil, 0}, {&c.hour, 0, 23, nil, 1}, {&c.dom, 1, 31, nil, 2}, {&c.month, 1, 12, months, 3}, {&c.dow, 0, 7, days, 4}}
	for _, sp := range specs {
		if err := parseField(f[sp.fieldIdx], sp.lo, sp.hi, sp.names, sp.set); err != nil {
			return nil, fmt.Errorf("cron %q: %w", expr, err)
		}
	}
	if c.dow[7] {
		c.dow[0] = true
	}
	return c, nil
}

func parseField(field string, lo, hi int, names map[string]int, set *[64]bool) error {
	val := func(s string) (int, error) {
		if v, ok := names[strings.ToLower(s)]; ok {
			return v, nil
		}
		n, err := strconv.Atoi(s)
		if err != nil || n < lo || n > hi {
			return 0, fmt.Errorf("value %q out of range %d-%d", s, lo, hi)
		}
		return n, nil
	}
	for _, part := range strings.Split(field, ",") {
		step := 1
		if base, st, ok := strings.Cut(part, "/"); ok {
			n, err := strconv.Atoi(st)
			if err != nil || n <= 0 {
				return fmt.Errorf("bad step in %q", part)
			}
			step, part = n, base
		}
		a, b := lo, hi
		switch {
		case part == "*":
		case strings.Contains(part, "-"):
			x, y, _ := strings.Cut(part, "-")
			var err error
			if a, err = val(x); err != nil {
				return err
			}
			if b, err = val(y); err != nil {
				return err
			}
			if a > b {
				return fmt.Errorf("bad range %q", part)
			}
		default:
			n, err := val(part)
			if err != nil {
				return err
			}
			a, b = n, n
			if step > 1 {
				b = hi
			}
		}
		for i := a; i <= b; i += step {
			set[i] = true
		}
	}
	return nil
}

// Next returns the first matching minute strictly after t (within ~5 years).
func (c *Cron) Next(t time.Time) (time.Time, bool) {
	t = t.Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(5, 0, 0)
	for t.Before(limit) {
		if !c.month[int(t.Month())] {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !c.dayMatches(t) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !c.hour[t.Hour()] {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		if !c.min[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t, true
	}
	return time.Time{}, false
}

// dayMatches applies cron's rule: if both day fields are restricted, either may match.
func (c *Cron) dayMatches(t time.Time) bool {
	dom, dow := c.dom[t.Day()], c.dow[int(t.Weekday())]
	switch {
	case c.domStar && c.dowStar:
		return true
	case c.domStar:
		return dow
	case c.dowStar:
		return dom
	}
	return dom || dow
}
