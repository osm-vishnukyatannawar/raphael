// Package tasks fetches the caller's in-review Pinestem tasks and caches them
// locally so the UI can paint immediately on launch.
package tasks

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
	ListProjects(ctx context.Context, token string, companyID int64) ([]pinestem.Project, error)
	ListReviewTasks(
		ctx context.Context, token string, companyID, userID int64, projectCodes []string,
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
	StatusType     string `json:"statusType"`
	StatusColor    string `json:"statusColor"`
	DueDate        string `json:"dueDate"`
	ModifiedOn     string `json:"modifiedOn"`
	SprintName     string `json:"sprintName"`
	CompetencyName string `json:"competencyName"`
	// IsNew drives the highlight: the task appeared in the queue and has not
	// been opened or dismissed yet. It survives restarts.
	IsNew bool `json:"isNew"`
}

// Result is a refresh outcome. New holds only tasks that were not in the queue
// before, which is what alerting keys off — Tasks is the full list either way.
type Result struct {
	Tasks []Task `json:"tasks"`
	New   []Task `json:"new"`
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

// Cached returns the locally stored tasks, newest-modified first.
//
// Pinestem timestamps are "YYYY-MM-DD HH:MM:SS", which sorts correctly as text,
// so the ORDER BY needs no date parsing.
func (s *Service) Cached(ctx context.Context) ([]Task, error) {
	rows, err := s.queries.ListTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("tasks: read cache: %w", err)
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
			StatusType:     r.StatusType,
			StatusColor:    r.StatusColor,
			DueDate:        r.DueDate,
			ModifiedOn:     r.ModifiedOn,
			SprintName:     r.SprintName,
			CompetencyName: r.CompetencyName,
			IsNew:          r.Acknowledged == 0,
		})
	}

	return out, nil
}

// Refresh pulls the live project list, then the in-review tasks across all of
// those projects, and replaces the cache with the result.
func (s *Service) Refresh(ctx context.Context) (*Result, error) {
	creds, err := s.creds.Credentials(ctx)
	if err != nil {
		return nil, err
	}

	// A first-ever sync seeds silently. Every task in the queue is "new" when
	// the cache is empty, and a fresh install firing a notification per open
	// review is noise, not news.
	current, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	seeding := current.TasksSyncedAt == ""

	projects, err := s.fetcher.ListProjects(ctx, creds.Token, creds.CompanyID)
	if err != nil {
		return nil, err
	}

	codes := make([]string, 0, len(projects))
	for _, p := range projects {
		codes = append(codes, p.Code)
	}

	fetched, err := s.fetcher.ListReviewTasks(
		ctx, creds.Token, creds.CompanyID, creds.UserID, codes,
	)
	if err != nil {
		return nil, err
	}

	newIDs, err := s.replaceCache(ctx, projects, fetched, seeding)
	if err != nil {
		return nil, err
	}

	if err := s.settings.MarkTasksSynced(ctx, time.Now()); err != nil {
		return nil, err
	}

	list, err := s.Cached(ctx)
	if err != nil {
		return nil, err
	}

	result := &Result{Tasks: list, New: []Task{}}
	for _, t := range list {
		if newIDs[t.TaskID] {
			result.New = append(result.New, t)
		}
	}

	return result, nil
}

// Acknowledge clears one task's highlight. Called when the task is opened.
func (s *Service) Acknowledge(ctx context.Context, taskID int64) error {
	if err := s.queries.AcknowledgeTask(ctx, taskID); err != nil {
		return fmt.Errorf("tasks: acknowledge %d: %w", taskID, err)
	}

	return nil
}

// AcknowledgeAll clears every highlight at once.
func (s *Service) AcknowledgeAll(ctx context.Context) error {
	if err := s.queries.AcknowledgeAllTasks(ctx); err != nil {
		return fmt.Errorf("tasks: acknowledge all: %w", err)
	}

	return nil
}

// replaceCache swaps the whole cache inside one transaction and returns the IDs
// of tasks that were not in the queue before.
//
// Wholesale replacement rather than a row-by-row merge: a task that leaves "In
// review" simply stops appearing in the response, and diffing to discover that
// is more code for the same outcome. The transaction means a mid-refresh
// failure leaves the previous cache intact instead of an empty list.
//
// seen_task is rebuilt alongside it rather than left alone, so that a task which
// leaves the queue and later returns is treated as new again. Acknowledgement of
// tasks that are still present is carried across.
func (s *Service) replaceCache(
	ctx context.Context, projects []pinestem.Project, list []pinestem.Task, seeding bool,
) (map[int64]bool, error) {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("tasks: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)
	syncedAt := time.Now().UTC().Format(time.RFC3339)

	previous, err := qtx.ListSeenTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("tasks: read seen tasks: %w", err)
	}
	seen := make(map[int64]sqlc.SeenTask, len(previous))
	for _, p := range previous {
		seen[p.TaskID] = p
	}

	if err := qtx.DeleteAllProjects(ctx); err != nil {
		return nil, fmt.Errorf("tasks: clear projects: %w", err)
	}
	for _, p := range projects {
		err := qtx.InsertProject(ctx, sqlc.InsertProjectParams{
			ProjectID: p.ProjectID,
			Code:      p.Code,
			Name:      p.Name,
			StatusID:  p.StatusID,
		})
		if err != nil {
			return nil, fmt.Errorf("tasks: cache project %s: %w", p.Code, err)
		}
	}

	if err := qtx.DeleteAllTasks(ctx); err != nil {
		return nil, fmt.Errorf("tasks: clear tasks: %w", err)
	}
	if err := qtx.DeleteAllSeenTasks(ctx); err != nil {
		return nil, fmt.Errorf("tasks: clear seen tasks: %w", err)
	}

	newIDs := make(map[int64]bool)

	for _, t := range list {
		err := qtx.InsertTask(ctx, sqlc.InsertTaskParams{
			TaskID:         t.TaskID,
			ShortCode:      t.ShortCode,
			Name:           t.Name,
			ProjectCode:    t.ProjectCode,
			ProjectName:    t.ProjectName,
			Priority:       t.Priority,
			StatusType:     t.StatusType,
			StatusColor:    t.StatusColor,
			DueDate:        t.DueDate,
			ModifiedOn:     t.ModifiedOn,
			SprintName:     t.SprintName,
			CompetencyName: t.CompetencyName,
			SyncedAt:       syncedAt,
		})
		if err != nil {
			return nil, fmt.Errorf("tasks: cache task %s: %w", t.ShortCode, err)
		}

		entry, known := seen[t.TaskID]
		switch {
		case known:
			// Carry the previous acknowledgement and first-seen stamp forward.
		case seeding:
			entry = sqlc.SeenTask{FirstSeenAt: syncedAt, Acknowledged: 1}
		default:
			entry = sqlc.SeenTask{FirstSeenAt: syncedAt, Acknowledged: 0}
			newIDs[t.TaskID] = true
		}

		err = qtx.InsertSeenTask(ctx, sqlc.InsertSeenTaskParams{
			TaskID:       t.TaskID,
			FirstSeenAt:  entry.FirstSeenAt,
			Acknowledged: entry.Acknowledged,
		})
		if err != nil {
			return nil, fmt.Errorf("tasks: record seen task %s: %w", t.ShortCode, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("tasks: commit cache: %w", err)
	}

	return newIDs, nil
}
