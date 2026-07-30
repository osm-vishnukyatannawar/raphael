package monitor

import "time"

// Working days are Monday to Friday.
//
// Not configurable, and not holiday-aware: Raphael has no access to the company
// calendar or anyone's leave, so any cleverness here would be a guess presented
// as a fact. A plain weekday count is easy to sanity-check against a calendar,
// which matters more for a number you are going to plan around.
func isWorkingDay(t time.Time) bool {
	switch t.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	default:
		return true
	}
}

// MonthBounds returns midnight on the first of now's month and 23:59:59 on the
// last day, both in now's location.
func MonthBounds(now time.Time) (time.Time, time.Time) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// Day 0 of next month is the last day of this one, which avoids having to
	// know month lengths or leap years.
	last := start.AddDate(0, 1, -1)
	end := time.Date(last.Year(), last.Month(), last.Day(), 23, 59, 59, 0, last.Location())

	return start, end
}

// WorkingDays counts Mon–Fri in [from, to], inclusive at both ends. An inverted
// range counts zero rather than going negative.
func WorkingDays(from, to time.Time) int {
	cur := startOfDay(from)
	last := startOfDay(to)

	count := 0
	for !cur.After(last) {
		if isWorkingDay(cur) {
			count++
		}
		cur = cur.AddDate(0, 0, 1)
	}

	return count
}

// RemainingWorkingDays counts the working days left after today, up to and
// including end.
//
// Today is excluded deliberately — see the note on the catch-up figure in
// Progress. A caller dividing a shortfall by this must handle zero.
func RemainingWorkingDays(now, end time.Time) int {
	return WorkingDays(startOfDay(now).AddDate(0, 0, 1), end)
}

// ElapsedWorkingDays counts working days from start through today inclusive,
// which is the basis for "how much should have been billed by now".
func ElapsedWorkingDays(start, now time.Time) int {
	return WorkingDays(start, now)
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
