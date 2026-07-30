package monitor_test

import (
	"testing"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/monitor"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.Local)
}

func TestMonthBounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name               string
		now                time.Time
		wantStart, wantEnd string
	}{
		{"mid month", day(2026, 7, 30), "2026-07-01", "2026-07-31"},
		{"first day", day(2026, 7, 1), "2026-07-01", "2026-07-31"},
		{"last day", day(2026, 7, 31), "2026-07-01", "2026-07-31"},
		{"30-day month", day(2026, 9, 15), "2026-09-01", "2026-09-30"},
		{"february, non-leap", day(2026, 2, 10), "2026-02-01", "2026-02-28"},
		{"february, leap", day(2028, 2, 10), "2028-02-01", "2028-02-29"},
		{"december rolls the year", day(2026, 12, 20), "2026-12-01", "2026-12-31"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			start, end := monitor.MonthBounds(tc.now)

			if got := start.Format(time.DateOnly); got != tc.wantStart {
				t.Errorf("start = %s, want %s", got, tc.wantStart)
			}
			if got := end.Format(time.DateOnly); got != tc.wantEnd {
				t.Errorf("end = %s, want %s", got, tc.wantEnd)
			}
			if h, m, s := start.Clock(); h != 0 || m != 0 || s != 0 {
				t.Errorf("start = %s, want midnight", start)
			}
			if h, m, s := end.Clock(); h != 23 || m != 59 || s != 59 {
				t.Errorf("end = %s, want end of day", end)
			}
		})
	}
}

func TestWorkingDays(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		from, to time.Time
		want     int
	}{
		// July 2026: 1st is a Wednesday, 31st a Friday → 23 weekdays.
		{"whole of july 2026", day(2026, 7, 1), day(2026, 7, 31), 23},
		{"single weekday", day(2026, 7, 30), day(2026, 7, 30), 1},
		{"single saturday", day(2026, 8, 1), day(2026, 8, 1), 0},
		{"single sunday", day(2026, 8, 2), day(2026, 8, 2), 0},
		{"a full week", day(2026, 7, 27), day(2026, 8, 2), 5},
		{"weekend only", day(2026, 8, 1), day(2026, 8, 2), 0},
		{"inverted range is empty", day(2026, 7, 31), day(2026, 7, 1), 0},
		// August 2026 starts on a Saturday — the awkward case for a month that
		// opens on a weekend.
		{"august 2026", day(2026, 8, 1), day(2026, 8, 31), 21},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := monitor.WorkingDays(tc.from, tc.to); got != tc.want {
				t.Errorf("WorkingDays = %d, want %d", got, tc.want)
			}
		})
	}
}

// Today is excluded on purpose: the catch-up figure must not assume the shortfall
// can still be absorbed in whatever is left of this evening.
func TestRemainingWorkingDaysExcludesToday(t *testing.T) {
	t.Parallel()

	_, end := monitor.MonthBounds(day(2026, 7, 30))

	// Thursday the 30th: only Friday the 31st is left.
	if got := monitor.RemainingWorkingDays(day(2026, 7, 30), end); got != 1 {
		t.Errorf("on Thu 30 July got %d, want 1 (Fri 31 only)", got)
	}
	// Friday the 31st is the last weekday of the month — nothing remains.
	if got := monitor.RemainingWorkingDays(day(2026, 7, 31), end); got != 0 {
		t.Errorf("on the last working day got %d, want 0", got)
	}
	// From the 1st, every weekday except the 1st itself.
	if got := monitor.RemainingWorkingDays(day(2026, 7, 1), end); got != 22 {
		t.Errorf("on 1 July got %d, want 22", got)
	}
}

func TestElapsedWorkingDaysIncludesToday(t *testing.T) {
	t.Parallel()

	start, _ := monitor.MonthBounds(day(2026, 7, 30))

	// 1 July is a Wednesday, so the first day already counts as one.
	if got := monitor.ElapsedWorkingDays(start, day(2026, 7, 1)); got != 1 {
		t.Errorf("on 1 July got %d, want 1", got)
	}
	// Through Thursday the 30th: 23 weekdays in the month, minus Friday the 31st.
	if got := monitor.ElapsedWorkingDays(start, day(2026, 7, 30)); got != 22 {
		t.Errorf("on 30 July got %d, want 22", got)
	}
	// A weekend contributes nothing, so Saturday reads the same as the Friday.
	friday := monitor.ElapsedWorkingDays(start, day(2026, 7, 3))
	saturday := monitor.ElapsedWorkingDays(start, day(2026, 7, 4))
	if friday != saturday {
		t.Errorf("Saturday advanced the count: %d vs %d", saturday, friday)
	}
}

// Elapsed and remaining must together account for the month, or the pace figure
// and the catch-up figure would disagree about how long the month is.
func TestElapsedPlusRemainingIsTheWholeMonth(t *testing.T) {
	t.Parallel()

	for _, d := range []int{1, 2, 5, 15, 25, 30, 31} {
		now := day(2026, 7, d)
		start, end := monitor.MonthBounds(now)

		total := monitor.WorkingDays(start, end)
		elapsed := monitor.ElapsedWorkingDays(start, now)
		remaining := monitor.RemainingWorkingDays(now, end)

		if elapsed+remaining != total {
			t.Errorf("%s: elapsed %d + remaining %d = %d, want %d",
				now.Format(time.DateOnly), elapsed, remaining, elapsed+remaining, total)
		}
	}
}
