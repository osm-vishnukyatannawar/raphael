// Package monitor tracks billing targets for a saved group of projects and
// people, and reports how far ahead or behind that group is this month.
package monitor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/db/sqlc"
	"github.com/osm-vishnukyatannawar/raphael/internal/identity"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
)

// AllProjects is the project_id sentinel meaning "this person, across every
// project in the monitor". Zero is never a real Pinestem project ID.
const AllProjects int64 = 0

// ErrNotFound is returned for an unknown monitor ID.
var ErrNotFound = errors.New("monitor: not found")

// Fetcher is the slice of the Pinestem client this package needs.
type Fetcher interface {
	BillingEntries(ctx context.Context, req pinestem.BillingRequest) ([]pinestem.BillingEntry, error)
}

// CredentialSource hands out the stored Pinestem session.
type CredentialSource interface {
	Credentials(ctx context.Context) (*identity.Credentials, error)
}

// Project is one project inside a monitor.
type Project struct {
	ProjectID int64  `json:"projectId"`
	Code      string `json:"code"`
	Name      string `json:"name"`
}

// Target is one person's commitment, either across the whole monitor
// (ProjectID == AllProjects) or on one specific project.
type Target struct {
	EmpID         int64  `json:"empId"`
	EmpName       string `json:"empName"`
	ProjectID     int64  `json:"projectId"`
	TargetMinutes int64  `json:"targetMinutes"`
}

// Monitor is the saved configuration.
type Monitor struct {
	ID       int64     `json:"id"`
	Name     string    `json:"name"`
	Projects []Project `json:"projects"`
	Targets  []Target  `json:"targets"`
}

// RowProgress is one target measured against what was actually billed.
type RowProgress struct {
	EmpID   int64  `json:"empId"`
	EmpName string `json:"empName"`
	// ProjectID is AllProjects when the target spans the whole monitor.
	ProjectID            int64  `json:"projectId"`
	ProjectName          string `json:"projectName"`
	TargetMinutes        int64  `json:"targetMinutes"`
	BillableMinutes      int64  `json:"billableMinutes"`
	NonBillableMinutes   int64  `json:"nonBillableMinutes"`
	ShortfallMinutes     int64  `json:"shortfallMinutes"`
	ExpectedByNowMinutes int64  `json:"expectedByNowMinutes"`
	NeededPerDayMinutes  int64  `json:"neededPerDayMinutes"`
	OnTrack              bool   `json:"onTrack"`
}

// Progress is a monitor's totals plus a row per target.
type Progress struct {
	MonitorID            int64         `json:"monitorId"`
	Name                 string        `json:"name"`
	PeriodStart          string        `json:"periodStart"`
	PeriodEnd            string        `json:"periodEnd"`
	TargetMinutes        int64         `json:"targetMinutes"`
	BillableMinutes      int64         `json:"billableMinutes"`
	NonBillableMinutes   int64         `json:"nonBillableMinutes"`
	ShortfallMinutes     int64         `json:"shortfallMinutes"`
	ExpectedByNowMinutes int64         `json:"expectedByNowMinutes"`
	RemainingWorkingDays int           `json:"remainingWorkingDays"`
	NeededPerDayMinutes  int64         `json:"neededPerDayMinutes"`
	OnTrack              bool          `json:"onTrack"`
	Projects             []Project     `json:"projects"`
	Rows                 []RowProgress `json:"rows"`
}

type Service struct {
	database *sql.DB
	queries  *sqlc.Queries
	fetcher  Fetcher
	creds    CredentialSource

	now func() time.Time
}

// Option customises a Service.
type Option func(*Service)

// WithClock replaces the source of "now". Tests only.
func WithClock(f func() time.Time) Option {
	return func(s *Service) { s.now = f }
}

func New(
	database *sql.DB, fetcher Fetcher, creds CredentialSource, opts ...Option,
) *Service {
	s := &Service{
		database: database,
		queries:  sqlc.New(database),
		fetcher:  fetcher,
		creds:    creds,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}

	return s
}

// List returns every saved monitor's configuration.
func (s *Service) List(ctx context.Context) ([]Monitor, error) {
	rows, err := s.queries.ListMonitors(ctx)
	if err != nil {
		return nil, fmt.Errorf("monitor: list: %w", err)
	}

	projects, targets, err := s.childrenByMonitor(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]Monitor, 0, len(rows))
	for _, r := range rows {
		out = append(out, Monitor{
			ID:       r.ID,
			Name:     r.Name,
			Projects: projects[r.ID],
			Targets:  targets[r.ID],
		})
	}

	return out, nil
}

// Get returns one monitor's configuration.
func (s *Service) Get(ctx context.Context, id int64) (*Monitor, error) {
	row, err := s.queries.GetMonitor(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("monitor: get %d: %w", id, err)
	}

	projects, targets, err := s.childrenByMonitor(ctx)
	if err != nil {
		return nil, err
	}

	return &Monitor{
		ID:       row.ID,
		Name:     row.Name,
		Projects: projects[row.ID],
		Targets:  targets[row.ID],
	}, nil
}

// Save creates or updates a monitor. An ID of zero creates.
//
// Children are replaced rather than diffed: the editor submits the whole
// configuration, so reconciling row by row would be more code for the same end
// state.
func (s *Service) Save(ctx context.Context, in Monitor) (*Monitor, error) {
	if in.Name == "" {
		return nil, errors.New("monitor: name is required")
	}

	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("monitor: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)
	now := s.now().UTC().Format(time.RFC3339)

	id := in.ID
	if id == 0 {
		id, err = qtx.InsertMonitor(ctx, sqlc.InsertMonitorParams{
			Name: in.Name, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return nil, fmt.Errorf("monitor: insert: %w", err)
		}
	} else {
		err = qtx.UpdateMonitor(ctx, sqlc.UpdateMonitorParams{
			Name: in.Name, UpdatedAt: now, ID: id,
		})
		if err != nil {
			return nil, fmt.Errorf("monitor: update %d: %w", id, err)
		}
	}

	if err := qtx.DeleteMonitorProjects(ctx, id); err != nil {
		return nil, fmt.Errorf("monitor: clear projects: %w", err)
	}
	for _, p := range in.Projects {
		err := qtx.InsertMonitorProject(ctx, sqlc.InsertMonitorProjectParams{
			MonitorID:   id,
			ProjectID:   p.ProjectID,
			ProjectCode: p.Code,
			ProjectName: p.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("monitor: add project %s: %w", p.Code, err)
		}
	}

	if err := qtx.DeleteMonitorTargets(ctx, id); err != nil {
		return nil, fmt.Errorf("monitor: clear targets: %w", err)
	}
	for _, t := range in.Targets {
		err := qtx.InsertMonitorTarget(ctx, sqlc.InsertMonitorTargetParams{
			MonitorID:     id,
			EmpID:         t.EmpID,
			EmpName:       t.EmpName,
			ProjectID:     t.ProjectID,
			TargetMinutes: t.TargetMinutes,
		})
		if err != nil {
			return nil, fmt.Errorf("monitor: add target for %s: %w", t.EmpName, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("monitor: commit: %w", err)
	}

	return s.Get(ctx, id)
}

// Delete removes a monitor and, by cascade, its projects, targets and actuals.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.queries.DeleteMonitor(ctx, id); err != nil {
		return fmt.Errorf("monitor: delete %d: %w", id, err)
	}

	return nil
}

// Cached reports progress from stored actuals, without any network.
func (s *Service) Cached(ctx context.Context) ([]Progress, error) {
	monitors, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.queries.ListMonitorActuals(ctx)
	if err != nil {
		return nil, fmt.Errorf("monitor: read actuals: %w", err)
	}

	actuals := make(map[actualKey]actual, len(rows))
	for _, r := range rows {
		actuals[actualKey{r.MonitorID, r.EmpID, r.ProjectID}] = actual{
			billable:    r.BillableMinutes,
			nonBillable: r.NonBillableMinutes,
		}
	}

	return s.project(monitors, actuals), nil
}

// Refresh pulls this month's billing for every monitored project in one pass and
// recomputes progress.
//
// One fetch covers all monitors: the rows carry EmpID and ProjectID, so they can
// be sliced per monitor locally. Adding a monitor therefore costs no extra API
// calls — only widening the set of projects does.
func (s *Service) Refresh(ctx context.Context) ([]Progress, error) {
	monitors, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(monitors) == 0 {
		return []Progress{}, nil
	}

	creds, err := s.creds.Credentials(ctx)
	if err != nil {
		return nil, err
	}

	projectIDs := unionProjectIDs(monitors)
	if len(projectIDs) == 0 {
		// Monitors exist but none names a project; nothing to ask the API for.
		return s.project(monitors, map[actualKey]actual{}), nil
	}

	start, end := MonthBounds(s.now())

	entries, err := s.fetcher.BillingEntries(ctx, pinestem.BillingRequest{
		Token:     creds.Token,
		CompanyID: creds.CompanyID,
		CallerID:  creds.UserID,
		// EmpID deliberately left zero: this view is the whole team's hours on
		// the monitored projects, not just the caller's.
		EmpID:            0,
		ProjectIDs:       projectIDs,
		RoleID:           creds.RoleID,
		IsProjectManager: creds.IsProjectManager,
		TimeZone:         creds.TimeZone,
		Start:            start,
		End:              end,
	})
	if err != nil {
		return nil, err
	}

	actuals := attribute(monitors, entries)
	if err := s.replaceActuals(ctx, actuals); err != nil {
		return nil, err
	}

	return s.project(monitors, actuals), nil
}

type actualKey struct {
	monitorID, empID, projectID int64
}

type actual struct {
	billable, nonBillable int64
}

// unionProjectIDs is every distinct project across all monitors, so one request
// covers them all.
func unionProjectIDs(monitors []Monitor) []int64 {
	seen := map[int64]bool{}

	var ids []int64
	for _, m := range monitors {
		for _, p := range m.Projects {
			if !seen[p.ProjectID] {
				seen[p.ProjectID] = true
				ids = append(ids, p.ProjectID)
			}
		}
	}

	return ids
}

// attribute assigns each billing entry to every monitor that watches its
// project. An entry can count towards more than one monitor — two monitors may
// legitimately track the same project — so this is not a partition.
func attribute(monitors []Monitor, entries []pinestem.BillingEntry) map[actualKey]actual {
	out := map[actualKey]actual{}

	for _, m := range monitors {
		watched := map[int64]bool{}
		for _, p := range m.Projects {
			watched[p.ProjectID] = true
		}

		for _, e := range entries {
			if !watched[e.ProjectID] {
				continue
			}

			key := actualKey{m.ID, e.EmpID, e.ProjectID}
			got := out[key]
			got.billable += e.BillableMinutes
			got.nonBillable += e.NonBillableMinutes
			out[key] = got
		}
	}

	return out
}

// project turns stored actuals into progress for each monitor.
func (s *Service) project(monitors []Monitor, actuals map[actualKey]actual) []Progress {
	now := s.now()
	start, end := MonthBounds(now)

	totalDays := WorkingDays(start, end)
	elapsed := ElapsedWorkingDays(start, now)
	remaining := RemainingWorkingDays(now, end)

	out := make([]Progress, 0, len(monitors))
	for _, m := range monitors {
		p := Progress{
			MonitorID:            m.ID,
			Name:                 m.Name,
			PeriodStart:          start.Format(time.DateOnly),
			PeriodEnd:            end.Format(time.DateOnly),
			RemainingWorkingDays: remaining,
			Projects:             m.Projects,
			Rows:                 make([]RowProgress, 0, len(m.Targets)),
		}

		names := map[int64]string{}
		for _, pr := range m.Projects {
			names[pr.ProjectID] = pr.Name
		}

		for _, t := range m.Targets {
			row := RowProgress{
				EmpID:         t.EmpID,
				EmpName:       t.EmpName,
				ProjectID:     t.ProjectID,
				ProjectName:   names[t.ProjectID],
				TargetMinutes: t.TargetMinutes,
			}

			if t.ProjectID == AllProjects {
				// Sum this person across every project the monitor watches.
				for _, pr := range m.Projects {
					a := actuals[actualKey{m.ID, t.EmpID, pr.ProjectID}]
					row.BillableMinutes += a.billable
					row.NonBillableMinutes += a.nonBillable
				}
			} else {
				a := actuals[actualKey{m.ID, t.EmpID, t.ProjectID}]
				row.BillableMinutes = a.billable
				row.NonBillableMinutes = a.nonBillable
			}

			row.ShortfallMinutes = shortfall(row.TargetMinutes, row.BillableMinutes)
			row.ExpectedByNowMinutes = prorate(row.TargetMinutes, elapsed, totalDays)
			row.NeededPerDayMinutes = perDay(row.ShortfallMinutes, remaining)
			row.OnTrack = row.BillableMinutes >= row.ExpectedByNowMinutes

			p.Rows = append(p.Rows, row)
			p.TargetMinutes += row.TargetMinutes
			p.BillableMinutes += row.BillableMinutes
			p.NonBillableMinutes += row.NonBillableMinutes
		}

		p.ShortfallMinutes = shortfall(p.TargetMinutes, p.BillableMinutes)
		p.ExpectedByNowMinutes = prorate(p.TargetMinutes, elapsed, totalDays)
		p.NeededPerDayMinutes = perDay(p.ShortfallMinutes, remaining)
		p.OnTrack = p.BillableMinutes >= p.ExpectedByNowMinutes

		out = append(out, p)
	}

	return out
}

// shortfall floors at zero: being ahead is not a negative shortfall, and the UI
// reads this as "how much is still owed".
func shortfall(target, billed int64) int64 {
	if billed >= target {
		return 0
	}

	return target - billed
}

// prorate is the share of the target that should have been billed by now.
func prorate(target int64, elapsed, total int) int64 {
	if total <= 0 {
		return 0
	}

	return target * int64(elapsed) / int64(total)
}

// perDay spreads a shortfall over the working days left.
//
// Zero when no days remain: the alternative is dividing by zero, and reporting
// the whole shortfall as "today's" figure would imply it is still achievable.
// Callers distinguish the two cases by RemainingWorkingDays.
func perDay(shortfallMinutes int64, remaining int) int64 {
	if remaining <= 0 || shortfallMinutes <= 0 {
		return 0
	}

	// Round up: rounding down would leave the target just short.
	return (shortfallMinutes + int64(remaining) - 1) / int64(remaining)
}

func (s *Service) replaceActuals(ctx context.Context, actuals map[actualKey]actual) error {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("monitor: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)

	if err := qtx.DeleteAllMonitorActuals(ctx); err != nil {
		return fmt.Errorf("monitor: clear actuals: %w", err)
	}
	for key, a := range actuals {
		err := qtx.InsertMonitorActual(ctx, sqlc.InsertMonitorActualParams{
			MonitorID:          key.monitorID,
			EmpID:              key.empID,
			ProjectID:          key.projectID,
			BillableMinutes:    a.billable,
			NonBillableMinutes: a.nonBillable,
		})
		if err != nil {
			return fmt.Errorf("monitor: cache actual: %w", err)
		}
	}

	stamp := s.now().UTC().Format(time.RFC3339)
	if err := qtx.SetMonitorsSyncedAt(ctx, &stamp); err != nil {
		return fmt.Errorf("monitor: record sync time: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("monitor: commit actuals: %w", err)
	}

	return nil
}

func (s *Service) childrenByMonitor(
	ctx context.Context,
) (map[int64][]Project, map[int64][]Target, error) {
	projectRows, err := s.queries.ListMonitorProjects(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("monitor: read projects: %w", err)
	}
	targetRows, err := s.queries.ListMonitorTargets(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("monitor: read targets: %w", err)
	}

	projects := map[int64][]Project{}
	for _, r := range projectRows {
		projects[r.MonitorID] = append(projects[r.MonitorID], Project{
			ProjectID: r.ProjectID,
			Code:      r.ProjectCode,
			Name:      r.ProjectName,
		})
	}

	targets := map[int64][]Target{}
	for _, r := range targetRows {
		targets[r.MonitorID] = append(targets[r.MonitorID], Target{
			EmpID:         r.EmpID,
			EmpName:       r.EmpName,
			ProjectID:     r.ProjectID,
			TargetMinutes: r.TargetMinutes,
		})
	}

	return projects, targets, nil
}
