package team_test

import (
	"testing"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
	"github.com/osm-vishnukyatannawar/raphael/internal/team"
)

func entry(day string, empID, projectID, billable, nonBillable int64) pinestem.BillingEntry {
	return pinestem.BillingEntry{
		Date:               day + " 00:00:00",
		EmpID:              empID,
		ProjectID:          projectID,
		BillableMinutes:    billable,
		NonBillableMinutes: nonBillable,
	}
}

// Omitting EmpID is how the whole team's rows come back; the slicing per member
// has to happen locally, and getting the request wrong would silently report
// only the signed-in user.
func TestBillingRefreshAsksForTheWholeTeamOnce(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: []pinestem.BillingEntry{
		entry("2026-08-05", 4001, 782, 300, 60),
		entry("2026-08-05", 4002, 782, 120, 0),
	}}
	svc, _ := newService(t, fetcher, pinnedNow)

	if _, err := svc.Save(t.Context(), billingBoard()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if fetcher.billingCalls != 1 {
		t.Errorf("made %d billing calls, want 1", fetcher.billingCalls)
	}
	if fetcher.gotBillingEmpID != 0 {
		t.Errorf("EmpID = %d, want 0 so the whole team comes back", fetcher.gotBillingEmpID)
	}
	if len(fetcher.gotBillingProject) != 1 || fetcher.gotBillingProject[0] != 782 {
		t.Errorf("ProjectIDs = %v, want [782]", fetcher.gotBillingProject)
	}
}

// Two boards still cost one request: adding a board is free, only widening the
// project set is not.
func TestTwoBillingBoardsShareOneFetch(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: []pinestem.BillingEntry{
		entry("2026-08-05", 4001, 782, 300, 0),
	}}
	svc, _ := newService(t, fetcher, pinnedNow)

	first := billingBoard()
	second := billingBoard()
	second.Name = "Another board"
	second.Projects = []team.ProjectRef{{ProjectID: 851, Code: "AMP", Name: "Amphenol"}}

	for _, b := range []team.Board{first, second} {
		if _, err := svc.Save(t.Context(), b); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if fetcher.billingCalls != 1 {
		t.Errorf("made %d billing calls for 2 boards, want 1", fetcher.billingCalls)
	}
	if len(fetcher.gotBillingProject) != 2 {
		t.Errorf("ProjectIDs = %v, want the union of both boards", fetcher.gotBillingProject)
	}
}

// The shared fetch spans every board's projects. A board scoped to one project
// must not be credited with hours logged on another board's project.
func TestHoursAreAttributedToTheRightBoard(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: []pinestem.BillingEntry{
		entry("2026-08-05", 4001, 782, 300, 0), // this board's project
		entry("2026-08-05", 4001, 851, 480, 0), // the other board's
	}}
	svc, _ := newService(t, fetcher, pinnedNow)

	first := billingBoard()
	second := billingBoard()
	second.Name = "Amphenol hours"
	second.Projects = []team.ProjectRef{{ProjectID: 851, Code: "AMP", Name: "Amphenol"}}

	for _, b := range []team.Board{first, second} {
		if _, err := svc.Save(t.Context(), b); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	views, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("got %d views", len(views))
	}
	if got := views[0].Rows[0].TodayMinutes; got != 300 {
		t.Errorf("first board today = %d, want 300", got)
	}
	if got := views[1].Rows[0].TodayMinutes; got != 480 {
		t.Errorf("second board today = %d, want 480", got)
	}
}

// Rows for a member land under that member, not smeared across the board.
func TestHoursAreBucketedPerMember(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: []pinestem.BillingEntry{
		entry("2026-08-05", 4001, 782, 300, 60),
		entry("2026-08-05", 4002, 782, 120, 0),
		entry("2026-08-04", 4001, 782, 420, 0),
		// Somebody not on the board. The fetch covers the whole team, so this
		// row arrives and must be ignored.
		entry("2026-08-05", 9999, 782, 999, 0),
	}}
	svc, _ := newService(t, fetcher, pinnedNow)

	if _, err := svc.Save(t.Context(), billingBoard()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	views, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	rows := views[0].Rows
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want one per configured member", rows)
	}
	if rows[0].EmpID != 4001 || rows[0].TodayMinutes != 360 {
		t.Errorf("first row = %+v, want 4001 with 360 today", rows[0])
	}
	if rows[0].YesterdayMinutes != 420 {
		t.Errorf("yesterday = %d, want 420", rows[0].YesterdayMinutes)
	}
	if rows[0].WeekMinutes != 780 || rows[0].WeekBillableMinutes != 720 {
		t.Errorf("week = %d (%d billable), want 780 (720)",
			rows[0].WeekMinutes, rows[0].WeekBillableMinutes)
	}
	if rows[1].EmpID != 4002 || rows[1].TodayMinutes != 120 {
		t.Errorf("second row = %+v", rows[1])
	}
}

// The grid is always seven zero-filled days so the UI never handles gaps.
func TestWeekGridIsAlwaysSevenDays(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{entries: []pinestem.BillingEntry{
		entry("2026-08-05", 4001, 782, 300, 0),
	}}
	svc, _ := newService(t, fetcher, pinnedNow)

	if _, err := svc.Save(t.Context(), billingBoard()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	views, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	for _, row := range views[0].Rows {
		if len(row.Days) != 7 {
			t.Errorf("%s has %d days, want 7", row.EmpName, len(row.Days))
		}
	}
	if views[0].WeekStart != "2026-08-03" || views[0].WeekEnd != "2026-08-09" {
		t.Errorf("week = %s..%s, want 2026-08-03..2026-08-09",
			views[0].WeekStart, views[0].WeekEnd)
	}
}

// On the first day of the week, yesterday falls in the *previous* week. It comes
// from the day map rather than the grid, which is the whole reason
// billing.FetchWindow stretches the request back a day.
func TestYesterdayWorksOnTheFirstDayOfTheWeek(t *testing.T) {
	t.Parallel()

	monday := time.Date(2026, 8, 3, 10, 0, 0, 0, time.Local)

	fetcher := &stubFetcher{entries: []pinestem.BillingEntry{
		entry("2026-08-03", 4001, 782, 120, 0), // today, this week
		entry("2026-08-02", 4001, 782, 480, 0), // yesterday, last week
	}}
	svc, _ := newService(t, fetcher, monday)

	if _, err := svc.Save(t.Context(), billingBoard()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	views, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	row := views[0].Rows[0]
	if row.YesterdayMinutes != 480 {
		t.Errorf("yesterday = %d, want 480 from the previous week", row.YesterdayMinutes)
	}
	if row.TodayMinutes != 120 {
		t.Errorf("today = %d, want 120", row.TodayMinutes)
	}
	// Yesterday is outside this week and must not inflate the week total.
	if row.WeekMinutes != 120 {
		t.Errorf("week = %d, want 120 -- last week's hours leaked in", row.WeekMinutes)
	}
}

// The picker opens before any project is chosen, so "everyone" is a real ask
// and must not go through the empty-codes guard that returns nothing.
func TestMembersWithNoProjectsAsksForTheWholeCompany(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{
		companyRoster: []pinestem.Member{{ID: 4001, Name: "Sample Member"}},
		members:       []pinestem.Member{{ID: 4002, Name: "Second Member"}},
	}
	svc, _ := newService(t, fetcher, pinnedNow)

	all, err := svc.Members(t.Context(), nil)
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	if fetcher.companyCalls != 1 || len(all) != 1 || all[0].ID != 4001 {
		t.Errorf("company roster = %+v after %d calls", all, fetcher.companyCalls)
	}

	scoped, err := svc.Members(t.Context(), []string{"RES"})
	if err != nil {
		t.Fatalf("Members (scoped): %v", err)
	}
	if fetcher.memberCalls != 1 || len(scoped) != 1 || scoped[0].ID != 4002 {
		t.Errorf("scoped members = %+v after %d calls", scoped, fetcher.memberCalls)
	}
}
