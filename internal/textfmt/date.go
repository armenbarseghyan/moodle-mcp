// Package textfmt formats times and text for human-readable tool output.
// All times are shown in Europe/Vienna.
package textfmt

import (
	"time"
	_ "time/tzdata" // embed the zone database: the binary must not depend on the host's
)

// Vienna is the time zone of every date shown to the user.
var Vienna = mustLoad("Europe/Vienna")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

var weekdays = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// Date formats t as "2026-10-07 17:15 (Wednesday)" in Vienna time.
func Date(t time.Time) string {
	v := t.In(Vienna)
	return v.Format("2006-01-02 15:04") + " (" + P().Sprintf(weekdays[v.Weekday()]) + ")"
}

// Day formats t as "2026-10-07" in Vienna time.
func Day(t time.Time) string { return t.In(Vienna).Format("2006-01-02") }

// DayStart returns Vienna midnight of t's calendar day shifted by offset days.
// It is DST-safe: DayStart(t, 1) is the next calendar midnight even when that
// day has 23 or 25 hours.
func DayStart(t time.Time, offset int) time.Time {
	v := t.In(Vienna)
	return time.Date(v.Year(), v.Month(), v.Day()+offset, 0, 0, 0, 0, Vienna)
}

// Relative describes t relative to now using Vienna calendar days:
// "in 40 min", "today, in 3 h", "tomorrow", "in 4 days",
// "just now", "15 min ago", "today, 3 h ago", "yesterday", "5 days ago".
func Relative(t, now time.Time) string {
	p := P()
	d := t.Sub(now)
	days := civilDay(t) - civilDay(now)
	if d >= 0 {
		switch {
		case d < time.Minute:
			return p.Sprintf("now")
		case d < time.Hour:
			return p.Sprintf("in %d min", int(d.Minutes()))
		case days == 0:
			return p.Sprintf("today, in %d h", int(d.Hours()))
		case days == 1:
			return p.Sprintf("tomorrow")
		default:
			return p.Sprintf("in %d days", days)
		}
	}
	d, days = -d, -days
	switch {
	case d < time.Minute:
		return p.Sprintf("just now")
	case d < time.Hour:
		return p.Sprintf("%d min ago", int(d.Minutes()))
	case days == 0:
		return p.Sprintf("today, %d h ago", int(d.Hours()))
	case days == 1:
		return p.Sprintf("yesterday")
	default:
		return p.Sprintf("%d days ago", days)
	}
}

// civilDay numbers Vienna calendar days so that differences count midnights.
func civilDay(t time.Time) int {
	y, m, d := t.In(Vienna).Date()
	return int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}
