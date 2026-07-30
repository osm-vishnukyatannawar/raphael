package billing

import "time"

// WeekBounds returns midnight on the first day of the week containing now, and
// 23:59:59 on its last day, both in now's location.
//
// startDay is configurable because there is no universal answer: ISO-8601 says
// Monday, much of the world reports Sunday–Saturday, and payroll weeks sometimes
// start elsewhere entirely.
func WeekBounds(now time.Time, startDay time.Weekday) (time.Time, time.Time) {
	start := StartOfDay(now)

	// How many days back the week began. The +7 keeps the result non-negative
	// when the current weekday is numerically before startDay.
	delta := (int(start.Weekday()) - int(startDay) + 7) % 7
	start = start.AddDate(0, 0, -delta)

	// Built from a calendar date rather than start.Add(7*24h) so a DST
	// transition inside the week can't shift the boundary by an hour.
	last := start.AddDate(0, 0, 6)
	end := time.Date(last.Year(), last.Month(), last.Day(), 23, 59, 59, 0, last.Location())

	return start, end
}

// StartOfDay is midnight at the beginning of t's calendar day, in t's location.
func StartOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// EndOfDay is 23:59:59 on t's calendar day, in t's location.
func EndOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location())
}
