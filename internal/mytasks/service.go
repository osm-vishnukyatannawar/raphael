// Package mytasks fetches every Pinestem task assigned to the caller — not just
// the ones waiting on review — and caches them locally.
//
// It is deliberately a separate package from internal/tasks rather than a mode
// of it. The two lists differ in almost everything that matters: cadence, sort
// order, which statuses they ask for, whether the user can filter them, and
// whether a new arrival is worth stealing the screen for. Sharing a service
// would mean threading that difference through every method.
package mytasks

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/db/sqlc"
	"github.com/osm-vishnukyatannawar/raphael/internal/identity"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
	"github.com/osm-vishnukyatannawar/raphael/internal/settings"
)

// Fetcher is the slice of the Pinestem client this package needs.
type Fetcher interface {
	ListProjects(
		ctx context.Context, token string, companyID int64, statuses []int,
	) ([]pinestem.Project, error)
	ListTaskStatuses(
		ctx context.Context, token string, companyID int64, projectCodes []string,
	) ([]pinestem.TaskStatus, error)
	ListAssignedTasks(
		ctx context.Context,
		token string,
		companyID, userID int64,
		projectCodes []string,
		statusIDs []int64,
	) ([]pinestem.Task, error)
}

// CredentialSource hands out the stored Pinestem session.
type CredentialSource interface {
	Credentials(ctx context.Context) (*identity.Credentials, error)
}

// Task is a cached task as the frontend sees it.
type Task struct {
	TaskID         int64  `json:"taskId"`
	ShortCode      string `json:"shortCode"`
	Name           string `json:"name"`
	ProjectCode    string `json:"projectCode"`
	ProjectName    string `json:"projectName"`
	Priority       string `json:"priority"`
	StatusID       int64  `json:"statusId"`
	StatusType     string `json:"statusType"`
	StatusColor    string `json:"statusColor"`
	DueDate        string `json:"dueDate"`
	ModifiedOn     string `json:"modifiedOn"`
	SprintName     string `json:"sprintName"`
	CompetencyName string `json:"competencyName"`
	// IsNew drives the highlight: the task appeared and has not been opened or
	// dismissed yet. It survives restarts.
	IsNew bool `json:"isNew"`
	// Hidden means the user asked not to see this one. The row is still cached
	// and still returned — the frontend decides where to put it.
	Hidden bool `json:"hidden"`
}

// HiddenTask is an entry in the hide list, with the labels it had when hidden.
//
// Those labels are stored rather than joined because the hide list must be able
// to name — and therefore unhide — a task that no longer comes back from the
// API at all, which is exactly what happens when it leaves the filtered set.
type HiddenTask struct {
	TaskID    int64  `json:"taskId"`
	ShortCode string `json:"shortCode"`
	Name      string `json:"name"`
	HiddenAt  string `json:"hiddenAt"`
}

// ProjectFilter is one chosen project. The code is what the API filters on; the
// name is kept so the filter dialog can label it without a lookup.
type ProjectFilter struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// StatusFilter is one chosen task status.
type StatusFilter struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Filter is the saved selection. An empty list on either side means "no filter":
// every active project, and every status that is not done. That is what a fresh
// install gets, so the list is useful before anyone opens the dialog.
type Filter struct {
	Projects []ProjectFilter `json:"projects"`
	Statuses []StatusFilter  `json:"statuses"`
}

// Options is what the filter dialog picks from, fetched live.
type Options struct {
	Projects []pinestem.Project    `json:"projects"`
	Statuses []pinestem.TaskStatus `json:"statuses"`
}

// Result is a refresh outcome. New holds only tasks that were not in the list
// before and are not hidden — hidden tasks are silent by definition.
type Result struct {
	Tasks  []Task       `json:"tasks"`
	New    []Task       `json:"new"`
	Hidden []HiddenTask `json:"hidden"`
}

type Service struct {
	database *sql.DB
	queries  *sqlc.Queries
	fetcher  Fetcher
	creds    CredentialSource
	settings *settings.Service
}

func New(
	database *sql.DB, fetcher Fetcher, creds CredentialSource, set *settings.Service,
) *Service {
	return &Service{
		database: database,
		queries:  sqlc.New(database),
		fetcher:  fetcher,
		creds:    creds,
		settings: set,
	}
}

// Cached returns the locally stored tasks: soonest due first, undated last.
func (s *Service) Cached(ctx context.Context) ([]Task, error) {
	rows, err := s.queries.ListMyTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("mytasks: read cache: %w", err)
	}

	out := make([]Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, Task{
			TaskID:         r.TaskID,
			ShortCode:      r.ShortCode,
			Name:           r.Name,
			ProjectCode:    r.ProjectCode,
			ProjectName:    r.ProjectName,
			Priority:       r.Priority,
			StatusID:       r.StatusID,
			StatusType:     r.StatusType,
			StatusColor:    r.StatusColor,
			DueDate:        r.DueDate,
			ModifiedOn:     r.ModifiedOn,
			SprintName:     r.SprintName,
			CompetencyName: r.CompetencyName,
			IsNew:          r.Acknowledged == 0,
			Hidden:         r.Hidden != 0,
		})
	}

	return out, nil
}

// Hidden returns the hide list, most recently hidden first.
func (s *Service) Hidden(ctx context.Context) ([]HiddenTask, error) {
	rows, err := s.queries.ListHiddenMyTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("mytasks: read hidden: %w", err)
	}

	out := make([]HiddenTask, 0, len(rows))
	for _, r := range rows {
		out = append(out, HiddenTask{
			TaskID:    r.TaskID,
			ShortCode: r.ShortCode,
			Name:      r.Name,
			HiddenAt:  r.HiddenAt,
		})
	}

	return out, nil
}

// Refresh resolves the filter, pulls the matching tasks and replaces the cache.
//
// A fully configured filter costs one API call. An unconfigured one costs three:
// the project list, then the status list for those projects, then the tasks.
func (s *Service) Refresh(ctx context.Context) (*Result, error) {
	creds, err := s.creds.Credentials(ctx)
	if err != nil {
		return nil, err
	}

	// A first-ever sync seeds silently. Every assigned task is "new" when the
	// cache is empty, and a fresh install firing a notification for each one is
	// noise, not news.
	current, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	seeding := current.MyTasksSyncedAt == ""

	filter, err := s.Filter(ctx)
	if err != nil {
		return nil, err
	}

	codes, err := s.projectCodes(ctx, creds, filter)
	if err != nil {
		return nil, err
	}

	statusIDs, err := s.statusIDs(ctx, creds, filter, codes)
	if err != nil {
		return nil, err
	}

	fetched, err := s.fetcher.ListAssignedTasks(
		ctx, creds.Token, creds.CompanyID, creds.UserID, codes, statusIDs,
	)
	if err != nil {
		return nil, err
	}

	newIDs, err := s.replaceCache(ctx, fetched, seeding)
	if err != nil {
		return nil, err
	}

	if err := s.settings.MarkMyTasksSynced(ctx, time.Now()); err != nil {
		return nil, err
	}

	return s.result(ctx, newIDs)
}

// projectCodes is the saved selection, or every active project when nothing is
// selected.
func (s *Service) projectCodes(
	ctx context.Context, creds *identity.Credentials, filter *Filter,
) ([]string, error) {
	if len(filter.Projects) > 0 {
		codes := make([]string, 0, len(filter.Projects))
		for _, p := range filter.Projects {
			codes = append(codes, p.Code)
		}

		return codes, nil
	}

	projects, err := s.fetcher.ListProjects(
		ctx, creds.Token, creds.CompanyID, pinestem.ActiveProjectStatuses,
	)
	if err != nil {
		return nil, err
	}

	codes := make([]string, 0, len(projects))
	for _, p := range projects {
		codes = append(codes, p.Code)
	}

	return codes, nil
}

// statusIDs is the saved selection, or every status that is not done.
//
// Dropping the done status is the whole reason this needs a lookup: without a
// status filter the endpoint returns every task the user has ever finished.
func (s *Service) statusIDs(
	ctx context.Context, creds *identity.Credentials, filter *Filter, codes []string,
) ([]int64, error) {
	if len(filter.Statuses) > 0 {
		ids := make([]int64, 0, len(filter.Statuses))
		for _, st := range filter.Statuses {
			ids = append(ids, st.ID)
		}

		return ids, nil
	}

	statuses, err := s.fetcher.ListTaskStatuses(ctx, creds.Token, creds.CompanyID, codes)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(statuses))
	for _, st := range statuses {
		if st.IsDone {
			continue
		}
		ids = append(ids, st.ID)
	}

	return ids, nil
}

// Options is the live project and status list the filter dialog picks from.
//
// Projects use the wide status range, like the monitor editor: a project can
// still hold assigned tasks after it leaves the active statuses.
func (s *Service) Options(ctx context.Context) (*Options, error) {
	creds, err := s.creds.Credentials(ctx)
	if err != nil {
		return nil, err
	}

	projects, err := s.fetcher.ListProjects(
		ctx, creds.Token, creds.CompanyID, pinestem.AllProjectStatuses,
	)
	if err != nil {
		return nil, err
	}

	codes := make([]string, 0, len(projects))
	for _, p := range projects {
		codes = append(codes, p.Code)
	}

	statuses, err := s.fetcher.ListTaskStatuses(ctx, creds.Token, creds.CompanyID, codes)
	if err != nil {
		return nil, err
	}

	return &Options{Projects: projects, Statuses: statuses}, nil
}

// Filter returns the saved selection.
func (s *Service) Filter(ctx context.Context) (*Filter, error) {
	projectRows, err := s.queries.ListMyTaskProjectFilter(ctx)
	if err != nil {
		return nil, fmt.Errorf("mytasks: read project filter: %w", err)
	}
	statusRows, err := s.queries.ListMyTaskStatusFilter(ctx)
	if err != nil {
		return nil, fmt.Errorf("mytasks: read status filter: %w", err)
	}

	out := Filter{
		Projects: make([]ProjectFilter, 0, len(projectRows)),
		Statuses: make([]StatusFilter, 0, len(statusRows)),
	}
	for _, r := range projectRows {
		out.Projects = append(out.Projects, ProjectFilter{Code: r.ProjectCode, Name: r.ProjectName})
	}
	for _, r := range statusRows {
		out.Statuses = append(out.Statuses, StatusFilter{ID: r.StatusID, Name: r.StatusName})
	}

	return &out, nil
}

// SaveFilter replaces the saved selection with the submitted one.
//
// Replacement rather than a merge: the dialog submits the whole filter, so
// clearing a selection has to be expressible.
func (s *Service) SaveFilter(ctx context.Context, in Filter) (*Filter, error) {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mytasks: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)

	if err := qtx.DeleteMyTaskProjectFilter(ctx); err != nil {
		return nil, fmt.Errorf("mytasks: clear project filter: %w", err)
	}
	for _, p := range in.Projects {
		err := qtx.InsertMyTaskProjectFilter(ctx, sqlc.InsertMyTaskProjectFilterParams{
			ProjectCode: p.Code,
			ProjectName: p.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("mytasks: save project filter %s: %w", p.Code, err)
		}
	}

	if err := qtx.DeleteMyTaskStatusFilter(ctx); err != nil {
		return nil, fmt.Errorf("mytasks: clear status filter: %w", err)
	}
	for _, st := range in.Statuses {
		err := qtx.InsertMyTaskStatusFilter(ctx, sqlc.InsertMyTaskStatusFilterParams{
			StatusID:   st.ID,
			StatusName: st.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("mytasks: save status filter %d: %w", st.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("mytasks: commit filter: %w", err)
	}

	return s.Filter(ctx)
}

// Hide takes a task out of the visible list for good.
//
// The labels are copied from the cache if the task is there. Hiding a task that
// has not arrived yet is allowed and records blank labels — the next refresh
// fills them in, and it means a known-unwanted task can be silenced ahead of
// time.
func (s *Service) Hide(ctx context.Context, taskID int64) error {
	shortCode, name := "", ""

	cached, err := s.Cached(ctx)
	if err != nil {
		return err
	}
	for _, task := range cached {
		if task.TaskID == taskID {
			shortCode, name = task.ShortCode, task.Name

			break
		}
	}

	err = s.queries.HideMyTask(ctx, sqlc.HideMyTaskParams{
		TaskID:    taskID,
		ShortCode: shortCode,
		Name:      name,
		HiddenAt:  time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("mytasks: hide %d: %w", taskID, err)
	}

	return nil
}

// Unhide puts a task back in the visible list.
func (s *Service) Unhide(ctx context.Context, taskID int64) error {
	if err := s.queries.UnhideMyTask(ctx, taskID); err != nil {
		return fmt.Errorf("mytasks: unhide %d: %w", taskID, err)
	}

	return nil
}

// UnhideAll empties the hide list.
func (s *Service) UnhideAll(ctx context.Context) error {
	if err := s.queries.UnhideAllMyTasks(ctx); err != nil {
		return fmt.Errorf("mytasks: unhide all: %w", err)
	}

	return nil
}

// Acknowledge clears one task's highlight. Called when the task is opened.
func (s *Service) Acknowledge(ctx context.Context, taskID int64) error {
	if err := s.queries.AcknowledgeMyTask(ctx, taskID); err != nil {
		return fmt.Errorf("mytasks: acknowledge %d: %w", taskID, err)
	}

	return nil
}

// AcknowledgeAll clears every highlight at once.
func (s *Service) AcknowledgeAll(ctx context.Context) error {
	if err := s.queries.AcknowledgeAllMyTasks(ctx); err != nil {
		return fmt.Errorf("mytasks: acknowledge all: %w", err)
	}

	return nil
}

// result assembles what both Refresh and the bound methods hand to the frontend.
func (s *Service) result(ctx context.Context, newIDs map[int64]bool) (*Result, error) {
	list, err := s.Cached(ctx)
	if err != nil {
		return nil, err
	}

	hidden, err := s.Hidden(ctx)
	if err != nil {
		return nil, err
	}

	out := &Result{Tasks: list, New: []Task{}, Hidden: hidden}
	for _, task := range list {
		// Hidden tasks are silent even on first sight; that is what hiding is.
		if newIDs[task.TaskID] && !task.Hidden {
			out.New = append(out.New, task)
		}
	}

	return out, nil
}

// Snapshot is Cached plus the hide list, with nothing flagged as newly arrived.
func (s *Service) Snapshot(ctx context.Context) (*Result, error) {
	return s.result(ctx, nil)
}

// replaceCache swaps the whole cache inside one transaction and returns the IDs
// of tasks that were not in the list before.
//
// Same shape as internal/tasks: wholesale replacement so a task that leaves the
// filtered set disappears, inside a transaction so a mid-refresh failure leaves
// the previous cache intact. hidden_my_task is untouched — hiding outlives any
// individual refresh.
func (s *Service) replaceCache(
	ctx context.Context, list []pinestem.Task, seeding bool,
) (map[int64]bool, error) {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mytasks: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)
	syncedAt := time.Now().UTC().Format(time.RFC3339)

	previous, err := qtx.ListSeenMyTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("mytasks: read seen tasks: %w", err)
	}
	seen := make(map[int64]sqlc.SeenMyTask, len(previous))
	for _, p := range previous {
		seen[p.TaskID] = p
	}

	if err := qtx.DeleteAllMyTasks(ctx); err != nil {
		return nil, fmt.Errorf("mytasks: clear tasks: %w", err)
	}
	if err := qtx.DeleteAllSeenMyTasks(ctx); err != nil {
		return nil, fmt.Errorf("mytasks: clear seen tasks: %w", err)
	}

	newIDs := make(map[int64]bool)

	for _, t := range list {
		err := qtx.InsertMyTask(ctx, sqlc.InsertMyTaskParams{
			TaskID:         t.TaskID,
			ShortCode:      t.ShortCode,
			Name:           t.Name,
			ProjectCode:    t.ProjectCode,
			ProjectName:    t.ProjectName,
			Priority:       t.Priority,
			StatusID:       t.StatusID,
			StatusType:     t.StatusType,
			StatusColor:    t.StatusColor,
			DueDate:        t.DueDate,
			ModifiedOn:     t.ModifiedOn,
			SprintName:     t.SprintName,
			CompetencyName: t.CompetencyName,
			SyncedAt:       syncedAt,
		})
		if err != nil {
			return nil, fmt.Errorf("mytasks: cache task %s: %w", t.ShortCode, err)
		}

		entry, known := seen[t.TaskID]
		switch {
		case known:
			// Carry the previous acknowledgement and first-seen stamp forward.
		case seeding:
			entry = sqlc.SeenMyTask{FirstSeenAt: syncedAt, Acknowledged: 1}
		default:
			entry = sqlc.SeenMyTask{FirstSeenAt: syncedAt, Acknowledged: 0}
			newIDs[t.TaskID] = true
		}

		err = qtx.InsertSeenMyTask(ctx, sqlc.InsertSeenMyTaskParams{
			TaskID:       t.TaskID,
			FirstSeenAt:  entry.FirstSeenAt,
			Acknowledged: entry.Acknowledged,
		})
		if err != nil {
			return nil, fmt.Errorf("mytasks: record seen task %s: %w", t.ShortCode, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("mytasks: commit cache: %w", err)
	}

	return newIDs, nil
}
