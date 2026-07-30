package billing_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/billing"
	"github.com/osm-vishnukyatannawar/raphael/internal/db"
	"github.com/osm-vishnukyatannawar/raphael/internal/identity"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
	"github.com/osm-vishnukyatannawar/raphael/internal/settings"
)

// now is fixed at Thursday 2026-07-30 so "today"/"yesterday"/"this week" are
// deterministic. The entries below are the real shape of company 453's data for
// EmpID 2286 that week.
var now = time.Date(2026, 7, 30, 15, 0, 0, 0, time.Local)

type stubFetcher struct {
	entries []pinestem.BillingEntry
	err     error

	gotReq pinestem.BillingRequest
	calls  int
}

func (s *stubFetcher) BillingEntries(
	_ context.Context, req pinestem.BillingRequest,
) ([]pinestem.BillingEntry, error) {
	s.calls++
	s.gotReq = req
	if s.err != nil {
		return nil, s.err
	}

	return s.entries, nil
}

type stubCreds struct {
	creds *identity.Credentials
	err   error
}

func (s stubCreds) Credentials(context.Context) (*identity.Credentials, error) {
	if s.err != nil {
		return nil, s.err
	}

	return s.creds, nil
}

func newService(t *testing.T, fetcher *stubFetcher) (*billing.Service, *settings.Service) {
	t.Helper()

	conn, err := db.OpenAt(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	creds := stubCreds{creds: &identity.Credentials{
		Token: "tok", CompanyID: 453, UserID: 2286,
		RoleID: 7, TimeZone: "India Standard Time",
	}}
	set := settings.New(conn)

	svc := billing.New(conn, fetcher, creds, set, billing.WithClock(func() time.Time { return now }))

	return svc, set
}

// Mirrors the live week: 9.5h on the 27th, 14h on the 28th, 12.5h on the 29th,
// nothing yet on the 30th. Totals 36.00h, which GetBillingTotalHours
// independently confirmed as 2160 minutes.
func sampleEntries() []pinestem.BillingEntry {
	return []pinestem.BillingEntry{
		{Date: "2026-07-29 22:20:12", BillableMinutes: 300, ProjectCode: "RES"},
		{Date: "2026-07-29 15:44:16", BillableMinutes: 450, ProjectCode: "SYS"},
		{Date: "2026-07-28 17:00:00", BillableMinutes: 780, ProjectCode: "RES"},
		{Date: "2026-07-28 09:00:00", BillableMinutes: 60, ProjectCode: "RES"},
		{Date: "2026-07-27 09:30:00", BillableMinutes: 540, NonBillableMinutes: 30, ProjectCode: "RES"},
	}
}

func TestCachedIsEmptyBeforeAnyRefresh(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t, &stubFetcher{})

	got, err := svc.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}
	if got.WeekMinutes != 0 || got.TodayMinutes != 0 {
		t.Errorf("totals = %+v, want zero", got)
	}
	// The week is always seven rows so the UI can render a fixed grid.
	if len(got.Days) != 7 {
		t.Errorf("got %d days, want 7 zero-filled", len(got.Days))
	}
}

func TestRefreshTotals(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t, &stubFetcher{entries: sampleEntries()})

	got, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// 2160 minutes = 36.00h, cross-checked live against GetBillingTotalHours.
	if got.WeekMinutes != 2160 {
		t.Errorf("WeekMinutes = %d, want 2160 (36.00h)", got.WeekMinutes)
	}
	if got.WeekBillableMinutes != 2130 || got.WeekNonBillableMinutes != 30 {
		t.Errorf("split = %d billable / %d non-billable, want 2130/30",
			got.WeekBillableMinutes, got.WeekNonBillableMinutes)
	}
	// Nothing logged yet today.
	if got.TodayMinutes != 0 {
		t.Errorf("TodayMinutes = %d, want 0", got.TodayMinutes)
	}
	if got.YesterdayMinutes != 750 {
		t.Errorf("YesterdayMinutes = %d, want 750 (12.50h on the 29th)", got.YesterdayMinutes)
	}

	want := map[string]int64{
		"2026-07-27": 570, // 9.50h
		"2026-07-28": 840, // 14.00h
		"2026-07-29": 750, // 12.50h
		"2026-07-30": 0,
		"2026-07-31": 0,
		"2026-08-01": 0,
		"2026-08-02": 0,
	}
	if len(got.Days) != 7 {
		t.Fatalf("got %d days, want 7", len(got.Days))
	}
	for _, d := range got.Days {
		w, ok := want[d.Day]
		if !ok {
			t.Errorf("unexpected day %s in the week", d.Day)

			continue
		}
		if d.Minutes() != w {
			t.Errorf("%s = %d minutes, want %d", d.Day, d.Minutes(), w)
		}
	}
	// Ordered so the UI can render them straight through.
	if got.Days[0].Day != "2026-07-27" || got.Days[6].Day != "2026-08-02" {
		t.Errorf("days not in order: %s .. %s", got.Days[0].Day, got.Days[6].Day)
	}
	if got.WeekStart != "2026-07-27" || got.WeekEnd != "2026-08-02" {
		t.Errorf("week = %s..%s, want 2026-07-27..2026-08-02", got.WeekStart, got.WeekEnd)
	}
}

func TestRefreshRequestsTheWholeWindowForTheConfiguredWeek(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: sampleEntries()}
	svc, set := newService(t, fetcher)

	if _, err := set.Save(t.Context(), settings.Settings{
		WeekStartDay: int64(time.Sunday),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// A Sunday week start moves the window back one day, and the 26th has no
	// entries, so the total is unchanged but the boundaries are not.
	if got.WeekStart != "2026-07-26" || got.WeekEnd != "2026-08-01" {
		t.Errorf("week = %s..%s, want 2026-07-26..2026-08-01", got.WeekStart, got.WeekEnd)
	}
	if got.WeekMinutes != 2160 {
		t.Errorf("WeekMinutes = %d, want 2160", got.WeekMinutes)
	}

	// One call covers the whole window; EmpID must be the stored per-company ID.
	if fetcher.calls != 1 {
		t.Errorf("made %d API calls, want 1", fetcher.calls)
	}
	if fetcher.gotReq.EmpID != 2286 {
		t.Errorf("EmpID = %d, want 2286", fetcher.gotReq.EmpID)
	}
	if fetcher.gotReq.TimeZone != "India Standard Time" || fetcher.gotReq.RoleID != 7 {
		t.Errorf("account context not forwarded: %+v", fetcher.gotReq)
	}
	if got := fetcher.gotReq.Start.Format(time.DateOnly); got != "2026-07-26" {
		t.Errorf("request start = %s, want 2026-07-26", got)
	}
	if got := fetcher.gotReq.End.Format(time.DateOnly); got != "2026-07-30" {
		t.Errorf("request end = %s, want today", got)
	}
}

// On the first day of the week, yesterday falls in the *previous* week. The
// window has to stretch back to cover it, or "Yesterday" silently reads zero.
func TestRefreshWindowCoversYesterdayOnTheFirstDayOfTheWeek(t *testing.T) {
	t.Parallel()

	monday := time.Date(2026, 7, 27, 10, 0, 0, 0, time.Local)
	fetcher := &stubFetcher{entries: []pinestem.BillingEntry{
		{Date: "2026-07-26 11:00:00", BillableMinutes: 120},
	}}

	conn, err := db.OpenAt(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	creds := stubCreds{creds: &identity.Credentials{Token: "tok", CompanyID: 453, UserID: 2286}}
	svc := billing.New(conn, fetcher, creds, settings.New(conn),
		billing.WithClock(func() time.Time { return monday }))

	got, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if start := fetcher.gotReq.Start.Format(time.DateOnly); start != "2026-07-26" {
		t.Errorf("request start = %s, want 2026-07-26 (Sunday, to cover yesterday)", start)
	}
	if got.YesterdayMinutes != 120 {
		t.Errorf("YesterdayMinutes = %d, want 120", got.YesterdayMinutes)
	}
	// Sunday is outside a Monday-start week, so it must not inflate the total.
	if got.WeekMinutes != 0 {
		t.Errorf("WeekMinutes = %d, want 0 — yesterday is in the previous week", got.WeekMinutes)
	}
}

func TestRefreshReplacesTheCache(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: sampleEntries()}
	svc, _ := newService(t, fetcher)

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}

	// An entry was deleted in Pinestem; the cached day must drop with it.
	fetcher.entries = sampleEntries()[:1]
	got, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("second Refresh: %v", err)
	}

	if got.WeekMinutes != 300 {
		t.Errorf("WeekMinutes = %d, want 300 — stale days survived the refresh", got.WeekMinutes)
	}
}

// A dropped network must leave the previous numbers readable, not blank them.
func TestRefreshFailureKeepsPreviousCache(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: sampleEntries()}
	svc, _ := newService(t, fetcher)

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}

	fetcher.err = errors.New("network is unreachable")
	if _, err := svc.Refresh(t.Context()); err == nil {
		t.Fatal("expected the refresh to fail")
	}

	cached, err := svc.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}
	if cached.WeekMinutes != 2160 {
		t.Errorf("WeekMinutes = %d after a failed refresh, want the previous 2160", cached.WeekMinutes)
	}
}

func TestRefreshRecordsSyncTime(t *testing.T) {
	t.Parallel()

	svc, set := newService(t, &stubFetcher{entries: sampleEntries()})

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	got, err := set.Get(t.Context())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.BillingSyncedAt == "" {
		t.Error("BillingSyncedAt still empty after a refresh")
	}
}

// An arbitrary date is answered live and must not disturb the cached week.
func TestForDateIsNotCached(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: sampleEntries()}
	svc, _ := newService(t, fetcher)

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	fetcher.entries = []pinestem.BillingEntry{
		{Date: "2026-06-15 10:00:00", BillableMinutes: 90, NonBillableMinutes: 30},
	}

	day, err := svc.ForDate(t.Context(), "2026-06-15")
	if err != nil {
		t.Fatalf("ForDate: %v", err)
	}
	if day.Day != "2026-06-15" || day.Minutes() != 120 {
		t.Errorf("ForDate = %+v, want 2026-06-15 with 120 minutes", day)
	}
	// The request must be bounded to that single day.
	if s := fetcher.gotReq.Start.Format(time.DateOnly); s != "2026-06-15" {
		t.Errorf("request start = %s", s)
	}
	if e := fetcher.gotReq.End.Format(time.DateOnly); e != "2026-06-15" {
		t.Errorf("request end = %s", e)
	}

	cached, err := svc.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}
	if cached.WeekMinutes != 2160 {
		t.Errorf("ForDate overwrote the cached week: %d", cached.WeekMinutes)
	}
}

func TestForDateRejectsGarbage(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t, &stubFetcher{})

	if _, err := svc.ForDate(t.Context(), "not-a-date"); err == nil {
		t.Fatal("expected an error for an unparseable date")
	}
}
