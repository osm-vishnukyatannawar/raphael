package monitor_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/db"
	"github.com/osm-vishnukyatannawar/raphael/internal/identity"
	"github.com/osm-vishnukyatannawar/raphael/internal/monitor"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
)

// Thursday 30 July 2026: 23 working days in the month, 22 elapsed, 1 remaining.
// Every expectation below is derived from those three numbers.
var now = day(2026, 7, 30)

const (
	projectX = int64(773)
	projectY = int64(782)
	empA     = int64(4001)
	empB     = int64(4002)
)

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

type stubCreds struct{}

func (stubCreds) Credentials(context.Context) (*identity.Credentials, error) {
	return &identity.Credentials{Token: "tok", CompanyID: 453, UserID: empA}, nil
}

func newService(t *testing.T, fetcher *stubFetcher) *monitor.Service {
	t.Helper()

	conn, err := db.OpenAt(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return monitor.New(conn, fetcher, stubCreds{},
		monitor.WithClock(func() time.Time { return now }))
}

// The example from the request: projects X and Y for one client, A committed to
// 60h and B to 140h, 200h together.
func sampleMonitor() monitor.Monitor {
	return monitor.Monitor{
		Name: "Client Acme",
		Projects: []monitor.Project{
			{ProjectID: projectX, Code: "RAD", Name: "Radiovision"},
			{ProjectID: projectY, Code: "RES", Name: "Research and Development"},
		},
		Targets: []monitor.Target{
			{EmpID: empA, EmpName: "Member A", ProjectID: monitor.AllProjects, TargetMinutes: 60 * 60},
			{EmpID: empB, EmpName: "Member B", ProjectID: monitor.AllProjects, TargetMinutes: 140 * 60},
		},
	}
}

func TestSaveAndGetRoundTrip(t *testing.T) {
	t.Parallel()

	svc := newService(t, &stubFetcher{})

	saved, err := svc.Save(t.Context(), sampleMonitor())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.ID == 0 {
		t.Fatal("Save returned no ID")
	}

	got, err := svc.Get(t.Context(), saved.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Client Acme" || len(got.Projects) != 2 || len(got.Targets) != 2 {
		t.Errorf("round trip lost data: %+v", got)
	}
}

func TestSaveReplacesChildrenRatherThanAppending(t *testing.T) {
	t.Parallel()

	svc := newService(t, &stubFetcher{})

	saved, err := svc.Save(t.Context(), sampleMonitor())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Drop a project and a person.
	edited := *saved
	edited.Projects = saved.Projects[:1]
	edited.Targets = saved.Targets[:1]

	again, err := svc.Save(t.Context(), edited)
	if err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if len(again.Projects) != 1 || len(again.Targets) != 1 {
		t.Errorf("children accumulated instead of being replaced: %+v", again)
	}
}

func TestSaveRejectsAnEmptyName(t *testing.T) {
	t.Parallel()

	svc := newService(t, &stubFetcher{})

	if _, err := svc.Save(t.Context(), monitor.Monitor{}); err == nil {
		t.Fatal("expected an error for a monitor with no name")
	}
}

func TestDeleteCascades(t *testing.T) {
	t.Parallel()

	svc := newService(t, &stubFetcher{})

	saved, err := svc.Save(t.Context(), sampleMonitor())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := svc.Delete(t.Context(), saved.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := svc.Get(t.Context(), saved.ID); !errors.Is(err, monitor.ErrNotFound) {
		t.Errorf("Get after delete = %v, want ErrNotFound", err)
	}

	list, err := svc.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("got %d monitors after delete, want 0", len(list))
	}
}

func TestRefreshAggregatesAcrossProjects(t *testing.T) {
	t.Parallel()

	// A: 30h on X + 10h on Y = 40h against a 60h target → 20h short.
	// B: 100h on X against 140h → 40h short.
	fetcher := &stubFetcher{entries: []pinestem.BillingEntry{
		{EmpID: empA, ProjectID: projectX, BillableMinutes: 30 * 60},
		{EmpID: empA, ProjectID: projectY, BillableMinutes: 10 * 60, NonBillableMinutes: 90},
		{EmpID: empB, ProjectID: projectX, BillableMinutes: 100 * 60},
		// A project nobody monitors must be ignored entirely.
		{EmpID: empA, ProjectID: 99999, BillableMinutes: 500 * 60},
	}}
	svc := newService(t, fetcher)

	if _, err := svc.Save(t.Context(), sampleMonitor()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d monitors, want 1", len(got))
	}
	p := got[0]

	if p.TargetMinutes != 200*60 {
		t.Errorf("target = %d, want %d (60h + 140h)", p.TargetMinutes, 200*60)
	}
	if p.BillableMinutes != 140*60 {
		t.Errorf("billable = %d, want %d — the unmonitored project leaked in",
			p.BillableMinutes, 140*60)
	}
	if p.NonBillableMinutes != 90 {
		t.Errorf("non-billable = %d, want 90", p.NonBillableMinutes)
	}
	if p.ShortfallMinutes != 60*60 {
		t.Errorf("shortfall = %d, want %d (200h - 140h)", p.ShortfallMinutes, 60*60)
	}
	// One working day left, so the whole shortfall lands on it.
	if p.RemainingWorkingDays != 1 {
		t.Errorf("remaining working days = %d, want 1", p.RemainingWorkingDays)
	}
	if p.NeededPerDayMinutes != 60*60 {
		t.Errorf("per day = %d, want %d", p.NeededPerDayMinutes, 60*60)
	}
	// 22 of 23 working days elapsed, so nearly the whole target was due.
	if want := int64(200 * 60 * 22 / 23); p.ExpectedByNowMinutes != want {
		t.Errorf("expected-by-now = %d, want %d", p.ExpectedByNowMinutes, want)
	}
	if p.OnTrack {
		t.Error("reported on track while 60h short with one day left")
	}
	if p.PeriodStart != "2026-07-01" || p.PeriodEnd != "2026-07-31" {
		t.Errorf("period = %s..%s, want the calendar month", p.PeriodStart, p.PeriodEnd)
	}

	// One request covers every monitored project, and must not filter by member.
	if fetcher.calls != 1 {
		t.Errorf("made %d API calls, want 1", fetcher.calls)
	}
	if fetcher.gotReq.EmpID != 0 {
		t.Errorf("EmpID = %d, want 0 — a monitor covers the whole team", fetcher.gotReq.EmpID)
	}
	if len(fetcher.gotReq.ProjectIDs) != 2 {
		t.Errorf("ProjectIDs = %v, want both monitored projects", fetcher.gotReq.ProjectIDs)
	}
	if fetcher.gotReq.CallerID != empA {
		t.Errorf("CallerID = %d, want the signed-in user", fetcher.gotReq.CallerID)
	}
	if s := fetcher.gotReq.Start.Format(time.DateOnly); s != "2026-07-01" {
		t.Errorf("request start = %s, want the 1st", s)
	}
}

// A per-project target must count only that project, not the person's whole
// contribution to the monitor.
func TestPerProjectTargetsAreScopedToTheirProject(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: []pinestem.BillingEntry{
		{EmpID: empA, ProjectID: projectX, BillableMinutes: 30 * 60},
		{EmpID: empA, ProjectID: projectY, BillableMinutes: 10 * 60},
	}}
	svc := newService(t, fetcher)

	m := sampleMonitor()
	m.Targets = []monitor.Target{
		{EmpID: empA, EmpName: "Member A", ProjectID: projectX, TargetMinutes: 40 * 60},
		{EmpID: empA, EmpName: "Member A", ProjectID: projectY, TargetMinutes: 20 * 60},
	}
	if _, err := svc.Save(t.Context(), m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	byProject := map[int64]int64{}
	for _, row := range got[0].Rows {
		byProject[row.ProjectID] = row.BillableMinutes
	}
	if byProject[projectX] != 30*60 {
		t.Errorf("X row = %d, want %d", byProject[projectX], 30*60)
	}
	if byProject[projectY] != 10*60 {
		t.Errorf("Y row = %d, want %d", byProject[projectY], 10*60)
	}
	// Totals must not double-count when targets are split by project.
	if got[0].BillableMinutes != 40*60 {
		t.Errorf("monitor total = %d, want %d", got[0].BillableMinutes, 40*60)
	}
}

// Two monitors may legitimately watch the same project; the entry counts for
// both rather than being consumed by whichever is processed first.
func TestOverlappingMonitorsBothCountTheSameEntry(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: []pinestem.BillingEntry{
		{EmpID: empA, ProjectID: projectX, BillableMinutes: 10 * 60},
	}}
	svc := newService(t, fetcher)

	for _, name := range []string{"First", "Second"} {
		_, err := svc.Save(t.Context(), monitor.Monitor{
			Name:     name,
			Projects: []monitor.Project{{ProjectID: projectX, Code: "RAD", Name: "Radiovision"}},
			Targets: []monitor.Target{
				{EmpID: empA, EmpName: "Member A", ProjectID: monitor.AllProjects, TargetMinutes: 20 * 60},
			},
		})
		if err != nil {
			t.Fatalf("Save %s: %v", name, err)
		}
	}

	got, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d monitors, want 2", len(got))
	}
	for _, p := range got {
		if p.BillableMinutes != 10*60 {
			t.Errorf("%s billable = %d, want %d", p.Name, p.BillableMinutes, 10*60)
		}
	}
	// Still a single API call — the fetch is over the union of projects.
	if fetcher.calls != 1 {
		t.Errorf("made %d calls, want 1", fetcher.calls)
	}
}

func TestBeingAheadReportsNoShortfall(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: []pinestem.BillingEntry{
		{EmpID: empA, ProjectID: projectX, BillableMinutes: 300 * 60},
		{EmpID: empB, ProjectID: projectX, BillableMinutes: 300 * 60},
	}}
	svc := newService(t, fetcher)

	if _, err := svc.Save(t.Context(), sampleMonitor()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	p := got[0]

	if p.ShortfallMinutes != 0 {
		t.Errorf("shortfall = %d, want 0 when ahead", p.ShortfallMinutes)
	}
	if p.NeededPerDayMinutes != 0 {
		t.Errorf("per day = %d, want 0 when ahead", p.NeededPerDayMinutes)
	}
	if !p.OnTrack {
		t.Error("not on track despite exceeding the target")
	}
}

// On the last working day nothing remains to spread the shortfall over. The
// figure must be 0 rather than a divide-by-zero or a fake "do it all today".
func TestNoWorkingDaysLeft(t *testing.T) {
	t.Parallel()

	conn, err := db.OpenAt(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	lastWorkingDay := day(2026, 7, 31)
	svc := monitor.New(conn, &stubFetcher{}, stubCreds{},
		monitor.WithClock(func() time.Time { return lastWorkingDay }))

	if _, err := svc.Save(t.Context(), sampleMonitor()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got[0].RemainingWorkingDays != 0 {
		t.Errorf("remaining = %d, want 0", got[0].RemainingWorkingDays)
	}
	if got[0].NeededPerDayMinutes != 0 {
		t.Errorf("per day = %d, want 0 with no days left", got[0].NeededPerDayMinutes)
	}
}

// The per-day figure rounds up: rounding down would leave the target short.
func TestPerDayRoundsUp(t *testing.T) {
	t.Parallel()

	conn, err := db.OpenAt(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// Wednesday 1 July: 22 working days remain after today.
	svc := monitor.New(conn, &stubFetcher{}, stubCreds{},
		monitor.WithClock(func() time.Time { return day(2026, 7, 1) }))

	m := sampleMonitor()
	m.Targets = []monitor.Target{
		// 100 minutes over 22 days is 4.54; it must not floor to 4.
		{EmpID: empA, EmpName: "Member A", ProjectID: monitor.AllProjects, TargetMinutes: 100},
	}
	if _, err := svc.Save(t.Context(), m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if got[0].NeededPerDayMinutes != 5 {
		t.Errorf("per day = %d, want 5 (100 over 22 days, rounded up)",
			got[0].NeededPerDayMinutes)
	}
}

func TestCachedSurvivesWithoutNetwork(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: []pinestem.BillingEntry{
		{EmpID: empA, ProjectID: projectX, BillableMinutes: 40 * 60},
	}}
	svc := newService(t, fetcher)

	if _, err := svc.Save(t.Context(), sampleMonitor()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	before := fetcher.calls
	got, err := svc.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}
	if fetcher.calls != before {
		t.Error("Cached hit the network")
	}
	if got[0].BillableMinutes != 40*60 {
		t.Errorf("cached billable = %d, want %d", got[0].BillableMinutes, 40*60)
	}
}

// Editing a monitor must not silently wipe the figures already on screen.
func TestActualsSurviveAnEdit(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: []pinestem.BillingEntry{
		{EmpID: empA, ProjectID: projectX, BillableMinutes: 40 * 60},
	}}
	svc := newService(t, fetcher)

	saved, err := svc.Save(t.Context(), sampleMonitor())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	renamed := *saved
	renamed.Name = "Client Acme (renamed)"
	if _, err := svc.Save(t.Context(), renamed); err != nil {
		t.Fatalf("rename: %v", err)
	}

	got, err := svc.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}
	if got[0].BillableMinutes != 40*60 {
		t.Errorf("billable = %d after an edit, want %d", got[0].BillableMinutes, 40*60)
	}
}

func TestRefreshWithNoMonitorsSkipsTheCall(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{}
	svc := newService(t, fetcher)

	got, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d progress rows, want 0", len(got))
	}
	if fetcher.calls != 0 {
		t.Error("called the API with nothing to monitor")
	}
}

func TestRefreshFailurePropagates(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{err: errors.New("network is unreachable")}
	svc := newService(t, fetcher)

	if _, err := svc.Save(t.Context(), sampleMonitor()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := svc.Refresh(t.Context()); err == nil {
		t.Fatal("expected the refresh to fail")
	}
}
