package team

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/osm-vishnukyatannawar/raphael/internal/db/sqlc"
	"github.com/osm-vishnukyatannawar/raphael/internal/identity"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
)

// memberFetchLimit bounds how many members are fetched at once.
//
// Pinestem has no multi-assignee filter, so a board is one paginated request per
// member and a ten-person board would otherwise open ten concurrent connections
// against an API that is already the slow part of every refresh.
const memberFetchLimit = 4

// refreshTaskBoard replaces one board's cached tasks.
//
// A failure anywhere aborts before the cache is touched, so a board keeps the
// last complete picture rather than showing a partial team.
func (s *Service) refreshTaskBoard(
	ctx context.Context, creds *identity.Credentials, b Board,
) error {
	codes := b.projectCodes()
	statusIDs := b.statusIDs()

	perMember := make([][]pinestem.Task, len(b.Members))

	group, gctx := errgroup.WithContext(ctx)
	group.SetLimit(memberFetchLimit)

	for i, m := range b.Members {
		group.Go(func() error {
			tasks, err := s.fetcher.ListTasksAssignedTo(
				gctx, creds.Token, creds.CompanyID, m.EmpID, codes, statusIDs, true,
			)
			if err != nil {
				return fmt.Errorf("team: tasks for %s: %w", m.Name, err)
			}
			perMember[i] = tasks

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return fmt.Errorf("team: refresh board %q: %w", b.Name, err)
	}

	return s.replaceBoardTasks(ctx, b, perMember)
}

// replaceBoardTasks swaps a board's whole task cache inside one transaction, so
// a task that disappeared upstream disappears here too and a failed write
// leaves the previous rows intact.
func (s *Service) replaceBoardTasks(
	ctx context.Context, b Board, perMember [][]pinestem.Task,
) error {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("team: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)
	syncedAt := s.now().UTC().Format(time.RFC3339)

	if err := qtx.DeleteTeamBoardTasks(ctx, b.ID); err != nil {
		return fmt.Errorf("team: clear tasks for board %d: %w", b.ID, err)
	}

	for i, m := range b.Members {
		seen := map[int64]bool{}

		for _, t := range perMember[i] {
			// ExcludeInformTo should already have done this, but the row carries
			// its own owner and it is cheap to believe that over the question we
			// asked. Zero means the field was absent, which is not evidence of
			// anything.
			if t.AssignedToEmpID != 0 && t.AssignedToEmpID != m.EmpID {
				continue
			}
			// The API can repeat a task across pages when rows shift under
			// pagination; the primary key would reject the duplicate and fail
			// the whole refresh.
			if seen[t.TaskID] {
				continue
			}
			seen[t.TaskID] = true

			err := qtx.InsertTeamBoardTask(ctx, sqlc.InsertTeamBoardTaskParams{
				BoardID:        b.ID,
				EmpID:          m.EmpID,
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
				return fmt.Errorf("team: cache task %d: %w", t.TaskID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("team: commit tasks for board %d: %w", b.ID, err)
	}

	return nil
}

func (s *Service) cachedTasks(ctx context.Context) (map[int64]map[int64][]Task, error) {
	rows, err := s.queries.ListTeamBoardTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("team: read cached tasks: %w", err)
	}

	// board -> member -> tasks. The query already returns them in display order.
	out := map[int64]map[int64][]Task{}
	for _, r := range rows {
		byMember, ok := out[r.BoardID]
		if !ok {
			byMember = map[int64][]Task{}
			out[r.BoardID] = byMember
		}

		byMember[r.EmpID] = append(byMember[r.EmpID], Task{
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
		})
	}

	return out, nil
}

// groupTasks builds one column per configured member, in the board's own member
// order. A member with nothing gets an empty column rather than vanishing --
// "nobody assigned them anything" is the useful answer, not a missing name.
func groupTasks(b Board, byMember map[int64][]Task) []MemberTasks {
	out := make([]MemberTasks, 0, len(b.Members))
	for _, m := range b.Members {
		tasks := byMember[m.EmpID]
		if tasks == nil {
			tasks = []Task{}
		}

		out = append(out, MemberTasks{EmpID: m.EmpID, EmpName: m.Name, Tasks: tasks})
	}

	return out
}
