// Package textfmt formats times and text for human-readable tool output.
// All times are shown in Europe/Vienna.
package textfmt

import (
	"fmt"
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

var weekdays = [...]string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}

// Date formats t as "2026-10-07 17:15 (среда)" in Vienna time.
func Date(t time.Time) string {
	v := t.In(Vienna)
	return fmt.Sprintf("%s (%s)", v.Format("2006-01-02 15:04"), weekdays[v.Weekday()])
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

// Relative describes t relative to now in Russian, using Vienna calendar days:
// "через 40 мин", "сегодня, через 3 ч", "завтра", "через 4 дня",
// "только что", "2 ч назад", "вчера", "5 дней назад".
func Relative(t, now time.Time) string {
	d := t.Sub(now)
	days := civilDay(t) - civilDay(now)
	if d >= 0 {
		switch {
		case d < time.Minute:
			return "сейчас"
		case d < time.Hour:
			return fmt.Sprintf("через %d мин", int(d.Minutes()))
		case days == 0:
			return fmt.Sprintf("сегодня, через %d ч", int(d.Hours()))
		case days == 1:
			return "завтра"
		default:
			return fmt.Sprintf("через %d %s", days, Plural(days, "день", "дня", "дней"))
		}
	}
	d, days = -d, -days
	switch {
	case d < time.Minute:
		return "только что"
	case d < time.Hour:
		return fmt.Sprintf("%d мин назад", int(d.Minutes()))
	case days == 0:
		return fmt.Sprintf("сегодня, %d ч назад", int(d.Hours()))
	case days == 1:
		return "вчера"
	default:
		return fmt.Sprintf("%d %s назад", days, Plural(days, "день", "дня", "дней"))
	}
}

// civilDay numbers Vienna calendar days so that differences count midnights.
func civilDay(t time.Time) int {
	y, m, d := t.In(Vienna).Date()
	return int(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400)
}

// Plural picks the Russian plural form for n: 1 день, 2 дня, 5 дней, 21 день.
func Plural(n int, one, few, many string) string {
	if n < 0 {
		n = -n
	}
	switch n10, n100 := n%10, n%100; {
	case n10 == 1 && n100 != 11:
		return one
	case n10 >= 2 && n10 <= 4 && (n100 < 12 || n100 > 14):
		return few
	default:
		return many
	}
}
