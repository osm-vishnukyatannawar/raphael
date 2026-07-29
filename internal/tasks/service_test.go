package tasks_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/osm-vishnukyatannawar/raphael/internal/db"
	"github.com/osm-vishnukyatannawar/raphael/internal/identity"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
	"github.com/osm-vishnukyatannawar/raphael/internal/settings"
	"github.com/osm-vishnukyatannawar/raphael/internal/tasks"
)

type stubFetcher struct {
	projects []pinestem.Project
	tasks    []pinestem.Task
	err      error

	gotUserID       int64
	gotCompanyID    int64
	gotProjectCodes []string
	calls           int
}

func (s *stubFetcher) ListProjects(_ context.Context, _ string, companyID int64) ([]pinestem.Project, error) {
	s.gotCompanyID = companyID
	if s.err != nil {
		return nil, s.err
	}

	return s.projects, nil
}

func (s *stubFetcher) ListReviewTasks(
	_ context.Context, _ string, _, userID int64, codes []string,
) ([]pinestem.Task, error) {
	s.calls++
	s.gotUserID = userID
	s.gotProjectCodes = codes
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

func newService(t *testing.T, fetcher tasks.Fetcher) (*tasks.Service, *sql.DB) {
	t.Helper()

	conn, err := db.OpenAt(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	creds := stubCreds{creds: &identity.Credentials{Token: "tok", CompanyID: 453, UserID: 2286}}

	return tasks.New(conn, fetcher, creds, settings.New(conn)), conn
}

func sample() ([]pinestem.Project, []pinestem.Task) {
	projects := []pinestem.Project{
		{ProjectID: 782, Code: "RES", Name: "Research and Development", StatusID: 2},
		{ProjectID: 851, Code: "AMP", Name: "Amphenol Website", StatusID: 2},
	}
	list := []pinestem.Task{
		{
			TaskID: 110780, ShortCode: "REST-2408", Name: "HRMS UI issue",
			ProjectCode: "RES", ProjectName: "Research and Development",
			Priority: "Medium", StatusType: "3. In review", StatusColor: "#e68fac",
			DueDate: "2026-07-30 00:00:00", ModifiedOn: "2026-07-28 23:01:42",
			SprintName: "2026 - Q3", CompetencyName: "Angular",
		},
		{
			TaskID: 110701, ShortCode: "REST-2402", Name: "Nodejs conversion",
			ProjectCode: "RES", ProjectName: "Research and Development",
			Priority: "Medium", ModifiedOn: "2026-07-28 21:56:44",
		},
		{
			TaskID: 110600, ShortCode: "REST-2397", Name: "Escalation module",
			ProjectCode: "RES", ProjectName: "Research and Development",
			Priority: "Medium", ModifiedOn: "2026-07-28 18:02:18",
		},
	}

	return projects, list
}

func TestCachedIsEmptyBeforeAnyRefresh(t *testing.T) {
	t.Parallel()

	svc, _ := newService(t, &stubFetcher{})

	got, err := svc.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d tasks, want 0", len(got))
	}
}

func TestRefreshPassesProjectCodesAndUserID(t *testing.T) {
	t.Parallel()

	projects, list := sample()
	fetcher := &stubFetcher{projects: projects, tasks: list}
	svc, _ := newService(t, fetcher)

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// AssignedTo must be the per-company UserId from the stored credentials.
	if fetcher.gotUserID != 2286 {
		t.Errorf("userID = %d, want 2286", fetcher.gotUserID)
	}
	if fetcher.gotCompanyID != 453 {
		t.Errorf("companyID = %d, want 453", fetcher.gotCompanyID)
	}
	// Every project from the live list feeds the task query — that is the whole
	// reason projects are fetched rather than hardcoded.
	if len(fetcher.gotProjectCodes) != 2 {
		t.Errorf("project codes = %v, want both", fetcher.gotProjectCodes)
	}
}

func TestRefreshCachesNewestModifiedFirst(t *testing.T) {
	t.Parallel()

	projects, list := sample()
	// Hand them over out of order; the cache must still come back sorted.
	shuffled := []pinestem.Task{list[2], list[0], list[1]}
	svc, _ := newService(t, &stubFetcher{projects: projects, tasks: shuffled})

	got, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	want := []string{"REST-2408", "REST-2402", "REST-2397"}
	for i, code := range want {
		if got[i].ShortCode != code {
			t.Errorf("position %d = %s, want %s", i, got[i].ShortCode, code)
		}
	}

	if got[0].StatusColor != "#e68fac" || got[0].CompetencyName != "Angular" {
		t.Errorf("metadata lost through the cache: %+v", got[0])
	}
}

// A task leaving "In review" simply stops appearing; it must not linger.
func TestRefreshDropsTasksThatLeftTheQueue(t *testing.T) {
	t.Parallel()

	projects, list := sample()
	fetcher := &stubFetcher{projects: projects, tasks: list}
	svc, _ := newService(t, fetcher)

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}

	fetcher.tasks = list[:1] // two tasks moved on
	got, err := svc.Refresh(t.Context())
	if err != nil {
		t.Fatalf("second Refresh: %v", err)
	}

	if len(got) != 1 || got[0].ShortCode != "REST-2408" {
		t.Errorf("got %d tasks (%+v), want only REST-2408", len(got), got)
	}
}

// A failed refresh must leave the previous cache readable rather than blanking it.
func TestRefreshFailureKeepsPreviousCache(t *testing.T) {
	t.Parallel()

	projects, list := sample()
	fetcher := &stubFetcher{projects: projects, tasks: list}
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
	if len(cached) != 3 {
		t.Errorf("cache has %d tasks after a failed refresh, want the previous 3", len(cached))
	}
}

func TestRefreshRecordsSyncTime(t *testing.T) {
	t.Parallel()

	projects, list := sample()
	svc, conn := newService(t, &stubFetcher{projects: projects, tasks: list})
	set := settings.New(conn)

	before, err := set.Get(t.Context())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if before.TasksSyncedAt != "" {
		t.Errorf("TasksSyncedAt = %q before any refresh, want empty", before.TasksSyncedAt)
	}

	if _, err := svc.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	after, err := set.Get(t.Context())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if after.TasksSyncedAt == "" {
		t.Error("TasksSyncedAt still empty after a refresh")
	}
	// The default must survive a refresh writing the sync timestamp.
	if after.RefreshIntervalSeconds != settings.DefaultRefreshSeconds {
		t.Errorf("interval = %d, want the %d default",
			after.RefreshIntervalSeconds, settings.DefaultRefreshSeconds)
	}
}
