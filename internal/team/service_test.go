package team_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/db"
	"github.com/osm-vishnukyatannawar/raphael/internal/identity"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
	"github.com/osm-vishnukyatannawar/raphael/internal/settings"
	"github.com/osm-vishnukyatannawar/raphael/internal/team"
)

// stubFetcher records what was asked of the API. Everything is mutex-guarded:
// task boards fetch members concurrently, so an unguarded counter here would
// only ever fail under -race, which is exactly the bug this package could grow.
type stubFetcher struct {
	mu sync.Mutex

	tasksByMember map[int64][]pinestem.Task
	entries       []pinestem.BillingEntry
	members       []pinestem.Member
	companyRoster []pinestem.Member
	statuses      []pinestem.TaskStatus

	taskErr error

	askedFor          []int64
	gotExcludeInform  []bool
	billingCalls      int
	gotBillingEmpID   int64
	gotBillingProject []int64
	memberCalls       int
	companyCalls      int
}

func (s *stubFetcher) ListTaskStatuses(
	context.Context, string, int64, []string,
) ([]pinestem.TaskStatus, error) {
	return s.statuses, nil
}

func (s *stubFetcher) ListProjectMembers(
	_ context.Context, _ string, _ int64, _ []string,
) ([]pinestem.Member, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memberCalls++

	return s.members, nil
}

func (s *stubFetcher) ListCompanyMembers(
	_ context.Context, _ string, _ int64,
) ([]pinestem.Member, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.companyCalls++

	return s.companyRoster, nil
}

func (s *stubFetcher) ListTasksAssignedTo(
	_ context.Context, _ string, _, empID int64,
	_ []string, _ []int64, excludeInformTo bool,
) ([]pinestem.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.askedFor = append(s.askedFor, empID)
	s.gotExcludeInform = append(s.gotExcludeInform, excludeInformTo)

	if s.taskErr != nil {
		return nil, s.taskErr
	}

	return s.tasksByMember[empID], nil
}

func (s *stubFetcher) BillingEntries(
	_ context.Context, req pinestem.BillingRequest,
) ([]pinestem.BillingEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.billingCalls++
	s.gotBillingEmpID = req.EmpID
	s.gotBillingProject = req.ProjectIDs

	return s.entries, nil
}

type stubCreds struct{}

func (stubCreds) Credentials(context.Context) (*identity.Credentials, error) {
	return &identity.Credentials{Token: "tok", CompanyID: 453, UserID: 2286}, nil
}

// pinnedNow is a Wednesday, so "yesterday" sits inside a Monday-start week.
var pinnedNow = time.Date(2026, 8, 5, 14, 0, 0, 0, time.Local)

func newService(t *testing.T, fetcher team.Fetcher, now time.Time) (*team.Service, *sql.DB) {
	t.Helper()

	conn, err := db.OpenAt(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	svc := team.New(
		conn, fetcher, stubCreds{}, settings.New(conn),
		team.WithClock(func() time.Time { return now }),
	)

	return svc, conn
}

func taskBoard() team.Board {
	return team.Board{
		Kind:     team.KindTasks,
		Name:     "Backend",
		Projects: []team.ProjectRef{{ProjectID: 782, Code: "RES", Name: "Research"}},
		Members: []team.MemberRef{
			{EmpID: 4001, Name: "Sample Member"},
			{EmpID: 4002, Name: "Second Member"},
		},
		Statuses: []team.StatusRef{{StatusID: 4063, Name: "3. In review"}},
	}
}

func billingBoard() team.Board {
	return team.Board{
		Kind:     team.KindBilling,
		Name:     "Backend hours",
		Projects: []team.ProjectRef{{ProjectID: 782, Code: "RES", Name: "Research"}},
		Members: []team.MemberRef{
			{EmpID: 4001, Name: "Sample Member"},
			{EmpID: 4002, Name: "Second Member"},
		},
	}
}

func TestSaveRoundTripsAndReplacesChildren(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t, &stubFetcher{}, pinnedNow)

	saved, err := svc.Save(t.Context(), taskBoard())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.ID == 0 || saved.Kind != team.KindTasks || len(saved.Members) != 2 {
		t.Fatalf("saved = %+v", saved)
	}

	// Editing submits the whole configuration; the old children must not linger.
	edit := *saved
	edit.Name = "Backend renamed"
	edit.Members = []team.MemberRef{{EmpID: 4002, Name: "Second Member"}}

	updated, err := svc.Save(t.Context(), edit)
	if err != nil {
		t.Fatalf("Save (update): %v", err)
	}
	if updated.Name != "Backend renamed" {
		t.Errorf("name = %q", updated.Name)
	}
	if len(updated.Members) != 1 || updated.Members[0].EmpID != 4002 {
		t.Errorf("members = %+v, want only 4002", updated.Members)
	}

	boards, err := svc.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(boards) != 1 {
		t.Errorf("got %d boards, want 1 (the update created a second)", len(boards))
	}
}

func TestDeleteCascadesToCachedRows(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{tasksByMember: map[int64][]pinestem.Task{
		4001: {{TaskID: 1, ShortCode: "RES-1", Name: "One", AssignedToEmpID: 4001}},
	}}
	svc, conn := newService(t, fetcher, pinnedNow)

	saved, err := svc.Save(t.Context(), taskBoard())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if err := svc.Delete(t.Context(), saved.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var count int
	row := conn.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM team_board_task")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count cached tasks: %v", err)
	}
	if count != 0 {
		t.Errorf("%d cached tasks survived the board being deleted", count)
	}
}

// The endpoint has no working multi-assignee filter, so the cost of a board is
// exactly one request per member. A regression here is a silent API hammering.
func TestTaskRefreshFetchesOncePerMember(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{tasksByMember: map[int64][]pinestem.Task{
		4001: {{TaskID: 1, ShortCode: "RES-1", Name: "One", AssignedToEmpID: 4001}},
		4002: {{TaskID: 2, ShortCode: "RES-2", Name: "Two", AssignedToEmpID: 4002}},
	}}
	svc, _ := newService(t, fetcher, pinnedNow)

	if _, err := svc.Save(t.Context(), taskBoard()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	views, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if len(fetcher.askedFor) != 2 {
		t.Errorf("made %d task calls for 2 members: %v", len(fetcher.askedFor), fetcher.askedFor)
	}
	for _, exclude := range fetcher.gotExcludeInform {
		if !exclude {
			t.Error("a team board asked for informed-on tasks; a member's column would fill with other people's work")
		}
	}

	if len(views) != 1 || len(views[0].Groups) != 2 {
		t.Fatalf("views = %+v", views)
	}
	if views[0].Groups[0].EmpID != 4001 || len(views[0].Groups[0].Tasks) != 1 {
		t.Errorf("first group = %+v", views[0].Groups[0])
	}
	if !views[0].Configured {
		t.Error("a fully configured board reported itself unconfigured")
	}
}

// A member with nothing assigned still gets a column: "nobody gave them
// anything" is the useful answer, not a missing name.
func TestEmptyMemberStillGetsAColumn(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{tasksByMember: map[int64][]pinestem.Task{
		4001: {{TaskID: 1, ShortCode: "RES-1", Name: "One", AssignedToEmpID: 4001}},
	}}
	svc, _ := newService(t, fetcher, pinnedNow)

	if _, err := svc.Save(t.Context(), taskBoard()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	views, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(views[0].Groups) != 2 {
		t.Fatalf("groups = %+v, want both members", views[0].Groups)
	}
	if got := views[0].Groups[1]; got.EmpID != 4002 || len(got.Tasks) != 0 {
		t.Errorf("second group = %+v, want an empty column for 4002", got)
	}
}

// Every filter is mandatory: Pinestem reads an omitted one as "everything", so
// an under-configured board would pull each member's entire history.
func TestUnconfiguredBoardFetchesNothing(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		muts  func(*team.Board)
		kind  string
		calls func(*stubFetcher) int
	}{
		{
			"tasks board with no statuses", func(b *team.Board) { b.Statuses = nil }, team.KindTasks,
			func(f *stubFetcher) int { return len(f.askedFor) },
		},
		{
			"tasks board with no members", func(b *team.Board) { b.Members = nil }, team.KindTasks,
			func(f *stubFetcher) int { return len(f.askedFor) },
		},
		{
			"tasks board with no projects", func(b *team.Board) { b.Projects = nil }, team.KindTasks,
			func(f *stubFetcher) int { return len(f.askedFor) },
		},
		{
			"billing board with no members", func(b *team.Board) { b.Members = nil }, team.KindBilling,
			func(f *stubFetcher) int { return f.billingCalls },
		},
		{
			"billing board with no projects", func(b *team.Board) { b.Projects = nil }, team.KindBilling,
			func(f *stubFetcher) int { return f.billingCalls },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fetcher := &stubFetcher{}
			svc, _ := newService(t, fetcher, pinnedNow)

			in := taskBoard()
			if tc.kind == team.KindBilling {
				in = billingBoard()
			}
			tc.muts(&in)

			if _, err := svc.Save(t.Context(), in); err != nil {
				t.Fatalf("Save: %v", err)
			}

			views, err := svc.Refresh(t.Context())
			if err != nil {
				t.Fatalf("Refresh: %v", err)
			}
			if got := tc.calls(fetcher); got != 0 {
				t.Errorf("made %d API calls for an unconfigured board", got)
			}
			if views[0].Configured {
				t.Error("an under-configured board reported itself configured")
			}
		})
	}
}

// A board emptied in the editor must not keep painting the old configuration's
// rows.
func TestClearingAConfigurationDropsCachedRows(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{tasksByMember: map[int64][]pinestem.Task{
		4001: {{TaskID: 1, ShortCode: "RES-1", Name: "One", AssignedToEmpID: 4001}},
	}}
	svc, _ := newService(t, fetcher, pinnedNow)

	saved, err := svc.Save(t.Context(), taskBoard())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	stripped := *saved
	stripped.Statuses = nil
	if _, err := svc.Save(t.Context(), stripped); err != nil {
		t.Fatalf("Save (stripped): %v", err)
	}

	views, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh (stripped): %v", err)
	}
	for _, g := range views[0].Groups {
		if len(g.Tasks) != 0 {
			t.Errorf("%s still shows %d cached tasks", g.EmpName, len(g.Tasks))
		}
	}
}

// The cache is replaced wholesale, so a task that vanished upstream vanishes
// here too rather than lingering forever.
func TestRefreshDropsTasksThatDisappearedUpstream(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{tasksByMember: map[int64][]pinestem.Task{
		4001: {
			{TaskID: 1, ShortCode: "RES-1", Name: "One", AssignedToEmpID: 4001},
			{TaskID: 2, ShortCode: "RES-2", Name: "Two", AssignedToEmpID: 4001},
		},
	}}
	svc, _ := newService(t, fetcher, pinnedNow)

	if _, err := svc.Save(t.Context(), taskBoard()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	fetcher.tasksByMember[4001] = []pinestem.Task{
		{TaskID: 2, ShortCode: "RES-2", Name: "Two", AssignedToEmpID: 4001},
	}

	views, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh (second): %v", err)
	}
	tasks := views[0].Groups[0].Tasks
	if len(tasks) != 1 || tasks[0].TaskID != 2 {
		t.Errorf("tasks = %+v, want only task 2", tasks)
	}
}

// A failed refresh must leave the last complete picture rather than blanking
// the board or showing half a team.
func TestFailedRefreshKeepsTheCache(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{tasksByMember: map[int64][]pinestem.Task{
		4001: {{TaskID: 1, ShortCode: "RES-1", Name: "One", AssignedToEmpID: 4001}},
	}}
	svc, _ := newService(t, fetcher, pinnedNow)

	if _, err := svc.Save(t.Context(), taskBoard()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	fetcher.taskErr = errors.New("network is down")
	if _, err := svc.Refresh(t.Context()); err == nil {
		t.Fatal("Refresh succeeded despite the fetcher failing")
	}

	views, err := svc.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}
	if len(views[0].Groups[0].Tasks) != 1 {
		t.Errorf("cached tasks were lost: %+v", views[0].Groups[0])
	}
}

// A row that comes back under someone we did not ask about is not theirs.
func TestTasksOwnedBySomeoneElseAreDropped(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{tasksByMember: map[int64][]pinestem.Task{
		4001: {
			{TaskID: 1, ShortCode: "RES-1", Name: "Mine", AssignedToEmpID: 4001},
			{TaskID: 9, ShortCode: "RES-9", Name: "Someone else's", AssignedToEmpID: 6277},
		},
	}}
	svc, _ := newService(t, fetcher, pinnedNow)

	if _, err := svc.Save(t.Context(), taskBoard()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	views, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	tasks := views[0].Groups[0].Tasks
	if len(tasks) != 1 || tasks[0].TaskID != 1 {
		t.Errorf("tasks = %+v, want only the row owned by 4001", tasks)
	}
}

// Undated tasks sort last; everything else by due date. Same order as the
// my-tasks list so the two read alike.
func TestTasksSortByDueDateWithUndatedLast(t *testing.T) {
	t.Parallel()

	fetcher := &stubFetcher{tasksByMember: map[int64][]pinestem.Task{
		4001: {
			{TaskID: 1, ShortCode: "RES-1", Name: "No due date", AssignedToEmpID: 4001},
			{TaskID: 2, ShortCode: "RES-2", Name: "Later", DueDate: "2026-08-20 00:00:00", AssignedToEmpID: 4001},
			{TaskID: 3, ShortCode: "RES-3", Name: "Sooner", DueDate: "2026-08-06 00:00:00", AssignedToEmpID: 4001},
		},
	}}
	svc, _ := newService(t, fetcher, pinnedNow)

	if _, err := svc.Save(t.Context(), taskBoard()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	views, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	var got []int64
	for _, task := range views[0].Groups[0].Tasks {
		got = append(got, task.TaskID)
	}
	if len(got) != 3 || got[0] != 3 || got[1] != 2 || got[2] != 1 {
		t.Errorf("order = %v, want [3 2 1]", got)
	}
}
