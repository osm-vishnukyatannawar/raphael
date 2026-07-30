package mytasks_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/osm-vishnukyatannawar/raphael/internal/db"
	"github.com/osm-vishnukyatannawar/raphael/internal/identity"
	"github.com/osm-vishnukyatannawar/raphael/internal/mytasks"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
	"github.com/osm-vishnukyatannawar/raphael/internal/settings"
)

type stubFetcher struct {
	projects []pinestem.Project
	statuses []pinestem.TaskStatus
	tasks    []pinestem.Task
	err      error

	gotUserID       int64
	gotProjectCodes []string
	gotStatusIDs    []int64
	projectCalls    int
	statusCalls     int
	taskCalls       int
}

func (s *stubFetcher) ListProjects(
	_ context.Context, _ string, _ int64, _ []int,
) ([]pinestem.Project, error) {
	s.projectCalls++
	if s.err != nil {
		return nil, s.err
	}

	return s.projects, nil
}

func (s *stubFetcher) ListTaskStatuses(
	_ context.Context, _ string, _ int64, _ []string,
) ([]pinestem.TaskStatus, error) {
	s.statusCalls++
	if s.err != nil {
		return nil, s.err
	}

	return s.statuses, nil
}

func (s *stubFetcher) ListAssignedTasks(
	_ context.Context, _ string, _, userID int64, codes []string, statusIDs []int64,
) ([]pinestem.Task, error) {
	s.taskCalls++
	s.gotUserID = userID
	s.gotProjectCodes = codes
	s.gotStatusIDs = statusIDs
	if s.err != nil {
		return nil, s.err
	}

	return s.tasks, nil
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

func newService(t *testing.T, fetcher mytasks.Fetcher) (*mytasks.Service, *sql.DB) {
	t.Helper()

	conn, err := db.OpenAt(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	creds := stubCreds{creds: &identity.Credentials{Token: "tok", CompanyID: 453, UserID: 2286}}

	return mytasks.New(conn, fetcher, creds, settings.New(conn)), conn
}

func sample() *stubFetcher {
	return &stubFetcher{
		projects: []pinestem.Project{
			{ProjectID: 782, Code: "RES", Name: "Research and Development", StatusID: 2},
			{ProjectID: 851, Code: "AMP", Name: "Amphenol Website", StatusID: 2},
		},
		statuses: []pinestem.TaskStatus{
			{ID: 1824, Name: "1. Planned"},
			{ID: 1825, Name: "2. Development /In Progress"},
			{ID: 4063, Name: "3. In review"},
			{ID: 1823, Name: "9. Done", IsDone: true},
		},
		tasks: []pinestem.Task{
			{
				TaskID: 109731, ShortCode: "REST-2376", Name: "Project Management",
				ProjectCode: "RES", ProjectName: "Research and Development",
				StatusID: 1825, StatusType: "2. Development /In Progress",
				DueDate: "2026-09-30 00:00:00", ModifiedOn: "2026-07-30 12:10:38",
			},
			{
				TaskID: 110780, ShortCode: "REST-2408", Name: "HRMS UI issue",
				ProjectCode: "RES", ProjectName: "Research and Development",
				StatusID: 4063, StatusType: "3. In review",
				DueDate: "2026-07-30 00:00:00", ModifiedOn: "2026-07-28 23:01:42",
			},
			{
				TaskID: 110701, ShortCode: "REST-2402", Name: "Nodejs conversion",
				ProjectCode: "AMP", ProjectName: "Amphenol Website",
				StatusID: 1824, StatusType: "1. Planned",
				ModifiedOn: "2026-07-28 21:56:44",
			},
		},
	}
}

// refresh twice: the first sync seeds silently, so tests that care about "new"
// need a second one.
func refreshTwice(t *testing.T, svc *mytasks.Service) *mytasks.Result {
	t.Helper()

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}

	result, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("second Refresh: %v", err)
	}

	return result
}

func TestCachedIsEmptyBeforeAnyRefresh(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t, sample())

	got, err := svc.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d tasks, want 0", len(got))
	}
}

// With nothing configured the list still has to be useful: every active project,
// every status that is not the terminal "done" one.
func TestRefreshWithNoFilterUsesEveryProjectAndOpenStatuses(t *testing.T) {
	t.Parallel()

	fetcher := sample()
	svc, _ := newService(t, fetcher)

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if fetcher.gotUserID != 2286 {
		t.Errorf("userID = %d, want 2286", fetcher.gotUserID)
	}
	if len(fetcher.gotProjectCodes) != 2 {
		t.Errorf("project codes = %v, want both", fetcher.gotProjectCodes)
	}
	// 1823 is IsDone and must be dropped; the other three stay.
	if len(fetcher.gotStatusIDs) != 3 {
		t.Fatalf("status IDs = %v, want the three open ones", fetcher.gotStatusIDs)
	}
	for _, id := range fetcher.gotStatusIDs {
		if id == 1823 {
			t.Errorf("the done status 1823 was queried: %v", fetcher.gotStatusIDs)
		}
	}
}

// A configured filter is used verbatim, and costs neither the project lookup nor
// the status lookup — a fully configured list is one API call per refresh.
func TestRefreshWithFilterSkipsTheLookups(t *testing.T) {
	t.Parallel()

	fetcher := sample()
	svc, _ := newService(t, fetcher)

	saved, err := svc.SaveFilter(t.Context(), mytasks.Filter{
		Projects: []mytasks.ProjectFilter{{Code: "AMP", Name: "Amphenol Website"}},
		Statuses: []mytasks.StatusFilter{{ID: 4063, Name: "3. In review"}},
	})
	if err != nil {
		t.Fatalf("SaveFilter: %v", err)
	}
	if len(saved.Projects) != 1 || len(saved.Statuses) != 1 {
		t.Fatalf("saved filter = %+v", saved)
	}

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if len(fetcher.gotProjectCodes) != 1 || fetcher.gotProjectCodes[0] != "AMP" {
		t.Errorf("project codes = %v, want [AMP]", fetcher.gotProjectCodes)
	}
	if len(fetcher.gotStatusIDs) != 1 || fetcher.gotStatusIDs[0] != 4063 {
		t.Errorf("status IDs = %v, want [4063]", fetcher.gotStatusIDs)
	}
	if fetcher.projectCalls != 0 || fetcher.statusCalls != 0 {
		t.Errorf("lookups called %d/%d times, want none",
			fetcher.projectCalls, fetcher.statusCalls)
	}
}

// Soonest due first, undated last. A task due next week matters more than one
// modified five minutes ago, which is the opposite of the review queue's order.
func TestCachedSortsByDueDateWithUndatedLast(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t, sample())

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	got, err := svc.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}

	want := []string{"REST-2408", "REST-2376", "REST-2402"}
	for i, code := range want {
		if got[i].ShortCode != code {
			t.Fatalf("order = %s, %s, %s; want %v",
				got[0].ShortCode, got[1].ShortCode, got[2].ShortCode, want)
		}
	}
}

// A fresh install must not fire an alert per open task.
func TestFirstRefreshSeedsSilently(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t, sample())

	result, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if len(result.New) != 0 {
		t.Errorf("new = %d on the seeding refresh, want 0", len(result.New))
	}
	for _, task := range result.Tasks {
		if task.IsNew {
			t.Errorf("%s is highlighted after seeding", task.ShortCode)
		}
	}
}

func TestRefreshReportsTasksThatArrivedSinceLastTime(t *testing.T) {
	t.Parallel()

	fetcher := sample()
	svc, _ := newService(t, fetcher)

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	fetcher.tasks = append(fetcher.tasks, pinestem.Task{
		TaskID: 110900, ShortCode: "REST-2411", Name: "New assignment",
		ProjectCode: "RES", ProjectName: "Research and Development",
		StatusID: 1824, ModifiedOn: "2026-07-30 14:00:00",
	})

	result, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if len(result.New) != 1 || result.New[0].ShortCode != "REST-2411" {
		t.Fatalf("new = %+v, want just REST-2411", result.New)
	}
}

// Hiding is a lasting "stop showing me this": it must survive the wholesale
// cache swap that every refresh performs.
func TestHideSurvivesRefreshAndUnhideRestores(t *testing.T) {
	t.Parallel()

	fetcher := sample()
	svc, _ := newService(t, fetcher)

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if err := svc.Hide(t.Context(), 109731); err != nil {
		t.Fatalf("Hide: %v", err)
	}

	result, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if !findTask(t, result.Tasks, "REST-2376").Hidden {
		t.Error("REST-2376 came back unhidden after a refresh")
	}
	if findTask(t, result.Tasks, "REST-2408").Hidden {
		t.Error("REST-2408 was hidden without being asked")
	}

	// The hidden list keeps its own labels so a task that stops coming back from
	// the API can still be named — and unhidden.
	hidden, err := svc.Hidden(t.Context())
	if err != nil {
		t.Fatalf("Hidden: %v", err)
	}
	if len(hidden) != 1 || hidden[0].ShortCode != "REST-2376" {
		t.Fatalf("hidden = %+v, want REST-2376", hidden)
	}

	if err := svc.Unhide(t.Context(), 109731); err != nil {
		t.Fatalf("Unhide: %v", err)
	}

	list, err := svc.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}
	if findTask(t, list, "REST-2376").Hidden {
		t.Error("REST-2376 is still hidden after Unhide")
	}
}

// The point of hiding is silence: a hidden task must not alert, even the first
// time it appears.
func TestHiddenTasksNeverCountAsNew(t *testing.T) {
	t.Parallel()

	fetcher := sample()
	svc, _ := newService(t, fetcher)

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Hide a task that has not arrived yet, then let it arrive.
	if err := svc.Hide(t.Context(), 110900); err != nil {
		t.Fatalf("Hide: %v", err)
	}
	fetcher.tasks = append(fetcher.tasks, pinestem.Task{
		TaskID: 110900, ShortCode: "REST-2411", Name: "Scrum call",
		ProjectCode: "RES", ProjectName: "Research and Development",
		StatusID: 1824, ModifiedOn: "2026-07-30 14:00:00",
	})

	result, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if len(result.New) != 0 {
		t.Errorf("new = %+v, want none — the task is hidden", result.New)
	}
}

func TestAcknowledgeClearsTheHighlight(t *testing.T) {
	t.Parallel()

	fetcher := sample()
	svc, _ := newService(t, fetcher)

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	fetcher.tasks = append(fetcher.tasks, pinestem.Task{
		TaskID: 110900, ShortCode: "REST-2411", Name: "New assignment",
		ProjectCode: "RES", ModifiedOn: "2026-07-30 14:00:00",
	})
	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if err := svc.Acknowledge(t.Context(), 110900); err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}

	list, err := svc.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}
	if findTask(t, list, "REST-2411").IsNew {
		t.Error("REST-2411 is still highlighted after Acknowledge")
	}
}

func TestAcknowledgeAllClearsEveryHighlight(t *testing.T) {
	t.Parallel()

	fetcher := sample()
	svc, _ := newService(t, fetcher)

	result := refreshTwice(t, svc)
	if len(result.Tasks) == 0 {
		t.Fatal("no tasks cached")
	}

	if err := svc.AcknowledgeAll(t.Context()); err != nil {
		t.Fatalf("AcknowledgeAll: %v", err)
	}

	list, err := svc.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}
	for _, task := range list {
		if task.IsNew {
			t.Errorf("%s is still highlighted", task.ShortCode)
		}
	}
}

// A failed refresh must leave the previous list on screen rather than blanking
// it, which means the cache is untouched.
func TestFailedRefreshKeepsTheCache(t *testing.T) {
	t.Parallel()

	fetcher := sample()
	svc, _ := newService(t, fetcher)

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	fetcher.err = errors.New("network is down")
	if _, err := svc.Refresh(t.Context()); err == nil {
		t.Fatal("expected an error")
	}

	list, err := svc.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("cached %d tasks after a failed refresh, want 3", len(list))
	}
}

func TestFilterRoundTrips(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t, sample())

	empty, err := svc.Filter(t.Context())
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(empty.Projects) != 0 || len(empty.Statuses) != 0 {
		t.Errorf("fresh filter = %+v, want empty", empty)
	}

	if _, err := svc.SaveFilter(t.Context(), mytasks.Filter{
		Projects: []mytasks.ProjectFilter{
			{Code: "RES", Name: "Research and Development"},
			{Code: "AMP", Name: "Amphenol Website"},
		},
		Statuses: []mytasks.StatusFilter{{ID: 4063, Name: "3. In review"}},
	}); err != nil {
		t.Fatalf("SaveFilter: %v", err)
	}

	// Saving replaces rather than merges: the editor submits the whole filter.
	got, err := svc.SaveFilter(t.Context(), mytasks.Filter{
		Projects: []mytasks.ProjectFilter{{Code: "AMP", Name: "Amphenol Website"}},
	})
	if err != nil {
		t.Fatalf("SaveFilter: %v", err)
	}
	if len(got.Projects) != 1 || got.Projects[0].Code != "AMP" {
		t.Errorf("projects = %+v, want just AMP", got.Projects)
	}
	if len(got.Statuses) != 0 {
		t.Errorf("statuses = %+v, want cleared", got.Statuses)
	}
}

func TestRefreshRequiresOnboarding(t *testing.T) {
	t.Parallel()

	conn, err := db.OpenAt(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	svc := mytasks.New(
		conn, sample(),
		stubCreds{err: identity.ErrNotOnboarded},
		settings.New(conn),
	)

	if _, err := svc.Refresh(t.Context()); !errors.Is(err, identity.ErrNotOnboarded) {
		t.Fatalf("Refresh error = %v, want ErrNotOnboarded", err)
	}
}

func findTask(t *testing.T, list []mytasks.Task, shortCode string) mytasks.Task {
	t.Helper()

	for _, task := range list {
		if task.ShortCode == shortCode {
			return task
		}
	}

	t.Fatalf("%s is not in the list", shortCode)

	return mytasks.Task{}
}
