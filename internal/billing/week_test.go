package billing_test

import (
	"testing"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/billing"
)

// 2026-07-30 is a Thursday. Both expectations were confirmed against the live
// Pinestem data used to build this feature.
func TestWeekBoundsKnownDates(t *testing.T) {
	t.Parallel()

	thursday := time.Date(2026, 7, 30, 14, 22, 0, 0, time.UTC)

	cases := []struct {
		name      string
		startDay  time.Weekday
		wantStart string
		wantEnd   string
	}{
		{"monday start", time.Monday, "2026-07-27", "2026-08-02"},
		{"sunday start", time.Sunday, "2026-07-26", "2026-08-01"},
		// Start day == the day itself: the week begins today, not a week ago.
		{"thursday start", time.Thursday, "2026-07-30", "2026-08-05"},
		// Start day one ahead: the week began six days ago.
		{"friday start", time.Friday, "2026-07-24", "2026-07-30"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			start, end := billing.WeekBounds(thursday, tc.startDay)

			if got := start.Format(time.DateOnly); got != tc.wantStart {
				t.Errorf("start = %s, want %s", got, tc.wantStart)
			}
			if got := end.Format(time.DateOnly); got != tc.wantEnd {
				t.Errorf("end = %s, want %s", got, tc.wantEnd)
			}
		})
	}
}

func TestWeekBoundsInvariants(t *testing.T) {
	t.Parallel()

	// A spread of awkward dates: a year boundary, a leap day, a month end, and
	// a plain midweek date.
	instants := []time.Time{
		time.Date(2027, 1, 1, 0, 0, 1, 0, time.UTC),
		time.Date(2028, 2, 29, 23, 59, 59, 0, time.UTC),
		time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 30, 14, 22, 0, 0, time.UTC),
	}

	for _, now := range instants {
		for day := time.Sunday; day <= time.Saturday; day++ {
			start, end := billing.WeekBounds(now, day)

			if start.Weekday() != day {
				t.Errorf("%s/%s: start weekday = %s, want %s",
					now.Format(time.DateOnly), day, start.Weekday(), day)
			}
			if h, m, sec := start.Clock(); h != 0 || m != 0 || sec != 0 {
				t.Errorf("%s/%s: start = %s, want midnight", now.Format(time.DateOnly), day, start)
			}
			if h, m, sec := end.Clock(); h != 23 || m != 59 || sec != 59 {
				t.Errorf("%s/%s: end = %s, want end of day", now.Format(time.DateOnly), day, end)
			}
			// The window must contain the instant it was derived from, and span
			// exactly seven days.
			if now.Before(start) || now.After(end) {
				t.Errorf("%s/%s: window %s..%s excludes now",
					now.Format(time.DateOnly), day, start, end)
			}
			if got := end.Sub(start).Hours(); got < 167 || got > 169 {
				t.Errorf("%s/%s: span = %.1fh, want ~168", now.Format(time.DateOnly), day, got)
			}
		}
	}
}

// The year-boundary case spelled out: 2027-01-01 is a Friday, so an ISO week
// starts in the previous year.
func TestWeekBoundsCrossesYearBoundary(t *testing.T) {
	t.Parallel()

	start, end := billing.WeekBounds(
		time.Date(2027, 1, 1, 9, 0, 0, 0, time.UTC), time.Monday,
	)

	if got := start.Format(time.DateOnly); got != "2026-12-28" {
		t.Errorf("start = %s, want 2026-12-28", got)
	}
	if got := end.Format(time.DateOnly); got != "2027-01-03" {
		t.Errorf("end = %s, want 2027-01-03", got)
	}
}
