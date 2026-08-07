// Package team backs the team boards: named, user-created views of other
// people's tasks and logged hours.
//
// One package rather than two despite the two board kinds, because everything
// except the refresh path is shared -- a name, a set of projects, a set of
// people, and the same wholesale-replace cache discipline. Only Refresh
// branches on kind.
//
// Nothing here notifies. A team board is a look-when-you-want view of somebody
// else's queue, so there is no first-seen tracking, no acknowledgement and no
// alert text, which is the main way this differs from internal/mytasks.
package team

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/billing"
	"github.com/osm-vishnukyatannawar/raphael/internal/db/sqlc"
	"github.com/osm-vishnukyatannawar/raphael/internal/identity"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
	"github.com/osm-vishnukyatannawar/raphael/internal/settings"
)

// Board kinds. Stored as text and constrained by the schema so a typo fails at
// the database rather than silently producing a board that never refreshes.
const (
	KindTasks   = "tasks"
	KindBilling = "billing"
)

// ErrNotFound is returned for an unknown board ID.
var ErrNotFound = errors.New("team: board not found")

// Fetcher is the slice of the Pinestem client this package needs.
type Fetcher interface {
	ListTaskStatuses(
		ctx context.Context, token string, companyID int64, projectCodes []string,
	) ([]pinestem.TaskStatus, error)
	ListProjectMembers(
		ctx context.Context, token string, companyID int64, projectCodes []string,
	) ([]pinestem.Member, error)
	ListCompanyMembers(
		ctx context.Context, token string, companyID int64,
	) ([]pinestem.Member, error)
	ListTasksAssignedTo(
		ctx context.Context, token string, companyID, empID int64,
		projectCodes []string, statusIDs []int64, excludeInformTo bool,
	) ([]pinestem.Task, error)
	BillingEntries(
		ctx context.Context, req pinestem.BillingRequest,
	) ([]pinestem.BillingEntry, error)
}

// CredentialSource hands out the stored Pinestem session.
type CredentialSource interface {
	Credentials(ctx context.Context) (*identity.Credentials, error)
}

// ProjectRef is one project on a board. Code and name are stored with the board
// rather than joined from the project cache, which a task refresh wipes.
type ProjectRef struct {
	ProjectID int64  `json:"projectId"`
	Code      string `json:"code"`
	Name      string `json:"name"`
}

// MemberRef is one person on a board. EmpID is the identifier shared by the
// members dropdown, the billing rows and a task's AssignedToEmpID.
type MemberRef struct {
	EmpID int64  `json:"empId"`
	Name  string `json:"name"`
}

// StatusRef is one task status on a board. Empty on billing boards.
type StatusRef struct {
	StatusID int64  `json:"statusId"`
	Name     string `json:"name"`
}

// Board is the saved configuration.
type Board struct {
	ID       int64        `json:"id"`
	Kind     string       `json:"kind"`
	Name     string       `json:"name"`
	Position int64        `json:"position"`
	Projects []ProjectRef `json:"projects"`
	Members  []MemberRef  `json:"members"`
	Statuses []StatusRef  `json:"statuses"`
}

// Configured reports whether the board has enough filters to be worth fetching.
//
// Every filter is mandatory, and not out of caution: Pinestem reads an omitted
// ProjectCode as "every project" and an omitted TaskStatusID as "every status",
// so an under-configured task board would pull each member's entire history --
// multiplied by the number of members.
func (b Board) Configured() bool {
	if len(b.Projects) == 0 || len(b.Members) == 0 {
		return false
	}
	if b.Kind == KindTasks && len(b.Statuses) == 0 {
		return false
	}

	return true
}

func (b Board) projectCodes() []string {
	out := make([]string, 0, len(b.Projects))
	for _, p := range b.Projects {
		out = append(out, p.Code)
	}

	return out
}

func (b Board) projectIDs() []int64 {
	out := make([]int64, 0, len(b.Projects))
	for _, p := range b.Projects {
		out = append(out, p.ProjectID)
	}

	return out
}

func (b Board) statusIDs() []int64 {
	out := make([]int64, 0, len(b.Statuses))
	for _, st := range b.Statuses {
		out = append(out, st.StatusID)
	}

	return out
}

// Task is a task as a board displays it.
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
}

// MemberTasks is one person's column on a task board.
type MemberTasks struct {
	EmpID   int64  `json:"empId"`
	EmpName string `json:"empName"`
	Tasks   []Task `json:"tasks"`
}

// MemberHours is one person's row on a billing board. Days always holds exactly
// seven entries, oldest first and zero-filled, matching billing.Summary so the
// week grid never has to handle gaps.
type MemberHours struct {
	EmpID                  int64              `json:"empId"`
	EmpName                string             `json:"empName"`
	TodayMinutes           int64              `json:"todayMinutes"`
	YesterdayMinutes       int64              `json:"yesterdayMinutes"`
	WeekMinutes            int64              `json:"weekMinutes"`
	WeekBillableMinutes    int64              `json:"weekBillableMinutes"`
	WeekNonBillableMinutes int64              `json:"weekNonBillableMinutes"`
	Days                   []billing.DayTotal `json:"days"`
}

// BoardView is a board plus whatever its kind renders. Groups is populated for
// KindTasks and Rows for KindBilling; the other is an empty slice.
type BoardView struct {
	Board  Board         `json:"board"`
	Groups []MemberTasks `json:"groups"`
	Rows   []MemberHours `json:"rows"`
	// WeekStart and WeekEnd label a billing board's grid.
	WeekStart string `json:"weekStart"`
	WeekEnd   string `json:"weekEnd"`
	// Configured mirrors Board.Configured so the UI can tell "nobody has any
	// tasks" apart from "this board was never set up".
	Configured bool `json:"configured"`
}

type Service struct {
	database *sql.DB
	queries  *sqlc.Queries
	fetcher  Fetcher
	creds    CredentialSource
	settings *settings.Service

	// now is injectable so tests can pin "today" without waiting for one.
	now func() time.Time
}

// Option customises a Service.
type Option func(*Service)

// WithClock replaces the source of "now". Tests only.
func WithClock(f func() time.Time) Option {
	return func(s *Service) { s.now = f }
}

func New(
	database *sql.DB, fetcher Fetcher, creds CredentialSource,
	set *settings.Service, opts ...Option,
) *Service {
	s := &Service{
		database: database,
		queries:  sqlc.New(database),
		fetcher:  fetcher,
		creds:    creds,
		settings: set,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}

	return s
}

// List returns every saved board's configuration, in strip order.
func (s *Service) List(ctx context.Context) ([]Board, error) {
	rows, err := s.queries.ListTeamBoards(ctx)
	if err != nil {
		return nil, fmt.Errorf("team: list boards: %w", err)
	}

	children, err := s.childrenByBoard(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]Board, 0, len(rows))
	for _, r := range rows {
		out = append(out, board(r, children))
	}

	return out, nil
}

// Get returns one board's configuration.
func (s *Service) Get(ctx context.Context, id int64) (*Board, error) {
	row, err := s.queries.GetTeamBoard(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("team: get board %d: %w", id, err)
	}

	children, err := s.childrenByBoard(ctx)
	if err != nil {
		return nil, err
	}

	out := board(row, children)

	return &out, nil
}

// Save creates or updates a board. An ID of zero creates.
//
// Children are replaced rather than diffed: the editor submits the whole
// configuration, so reconciling row by row would be more code for the same end
// state.
//
// Kind is only honoured on create. Switching an existing board's kind would
// leave the wrong cache populated and the wrong filters saved, so the editor
// offers delete-and-recreate instead.
func (s *Service) Save(ctx context.Context, in Board) (*Board, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, errors.New("team: board name is required")
	}
	if in.ID == 0 && in.Kind != KindTasks && in.Kind != KindBilling {
		return nil, fmt.Errorf("team: unknown board kind %q", in.Kind)
	}

	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("team: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)
	now := s.now().UTC().Format(time.RFC3339)

	id := in.ID
	if id == 0 {
		id, err = qtx.InsertTeamBoard(ctx, sqlc.InsertTeamBoardParams{
			Kind: in.Kind, Name: in.Name, Position: in.Position,
			CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return nil, fmt.Errorf("team: insert board: %w", err)
		}
	} else {
		err = qtx.UpdateTeamBoard(ctx, sqlc.UpdateTeamBoardParams{
			Name: in.Name, Position: in.Position, UpdatedAt: now, ID: id,
		})
		if err != nil {
			return nil, fmt.Errorf("team: update board %d: %w", id, err)
		}
	}

	if err := replaceChildren(ctx, qtx, id, in); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("team: commit: %w", err)
	}

	return s.Get(ctx, id)
}

func replaceChildren(ctx context.Context, qtx *sqlc.Queries, id int64, in Board) error {
	if err := qtx.DeleteTeamBoardProjects(ctx, id); err != nil {
		return fmt.Errorf("team: clear projects: %w", err)
	}
	for _, p := range in.Projects {
		err := qtx.InsertTeamBoardProject(ctx, sqlc.InsertTeamBoardProjectParams{
			BoardID:     id,
			ProjectID:   p.ProjectID,
			ProjectCode: p.Code,
			ProjectName: p.Name,
		})
		if err != nil {
			return fmt.Errorf("team: add project %s: %w", p.Code, err)
		}
	}

	if err := qtx.DeleteTeamBoardMembers(ctx, id); err != nil {
		return fmt.Errorf("team: clear members: %w", err)
	}
	for _, m := range in.Members {
		err := qtx.InsertTeamBoardMember(ctx, sqlc.InsertTeamBoardMemberParams{
			BoardID: id, EmpID: m.EmpID, EmpName: m.Name,
		})
		if err != nil {
			return fmt.Errorf("team: add member %d: %w", m.EmpID, err)
		}
	}

	if err := qtx.DeleteTeamBoardStatuses(ctx, id); err != nil {
		return fmt.Errorf("team: clear statuses: %w", err)
	}
	for _, st := range in.Statuses {
		err := qtx.InsertTeamBoardStatus(ctx, sqlc.InsertTeamBoardStatusParams{
			BoardID: id, StatusID: st.StatusID, StatusName: st.Name,
		})
		if err != nil {
			return fmt.Errorf("team: add status %d: %w", st.StatusID, err)
		}
	}

	return nil
}

// Delete removes a board and, by cascade, its filters and cached rows.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.queries.DeleteTeamBoard(ctx, id); err != nil {
		return fmt.Errorf("team: delete board %d: %w", id, err)
	}

	return nil
}

// Reorder rewrites the strip order from the given sequence of board IDs.
func (s *Service) Reorder(ctx context.Context, ids []int64) error {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("team: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)
	now := s.now().UTC().Format(time.RFC3339)

	for i, id := range ids {
		err := qtx.SetTeamBoardPosition(ctx, sqlc.SetTeamBoardPositionParams{
			Position: int64(i), UpdatedAt: now, ID: id,
		})
		if err != nil {
			return fmt.Errorf("team: reorder board %d: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("team: commit reorder: %w", err)
	}

	return nil
}

// Members offers the people a board can be built from.
//
// No project codes means the whole company, which is a legitimate ask here --
// the picker opens before any project is chosen. That is why it calls
// ListCompanyMembers explicitly rather than relying on the client's
// empty-codes behaviour, which returns nothing on purpose.
func (s *Service) Members(ctx context.Context, projectCodes []string) ([]pinestem.Member, error) {
	creds, err := s.creds.Credentials(ctx)
	if err != nil {
		return nil, err
	}

	if len(projectCodes) == 0 {
		return s.fetcher.ListCompanyMembers(ctx, creds.Token, creds.CompanyID)
	}

	return s.fetcher.ListProjectMembers(ctx, creds.Token, creds.CompanyID, projectCodes)
}

// Statuses offers the task statuses defined across the given projects.
func (s *Service) Statuses(
	ctx context.Context, projectCodes []string,
) ([]pinestem.TaskStatus, error) {
	creds, err := s.creds.Credentials(ctx)
	if err != nil {
		return nil, err
	}

	return s.fetcher.ListTaskStatuses(ctx, creds.Token, creds.CompanyID, projectCodes)
}

// Cached builds every board's view from stored rows, without any network.
func (s *Service) Cached(ctx context.Context) ([]BoardView, error) {
	boards, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	tasksByBoard, err := s.cachedTasks(ctx)
	if err != nil {
		return nil, err
	}

	daysByBoard, err := s.cachedDays(ctx)
	if err != nil {
		return nil, err
	}

	weekStartDay, err := s.weekStartDay(ctx)
	if err != nil {
		return nil, err
	}
	weekStart, weekEnd := billing.WeekBounds(s.now(), weekStartDay)

	out := make([]BoardView, 0, len(boards))
	for _, b := range boards {
		view := BoardView{
			Board:      b,
			Groups:     []MemberTasks{},
			Rows:       []MemberHours{},
			Configured: b.Configured(),
		}

		switch b.Kind {
		case KindTasks:
			view.Groups = groupTasks(b, tasksByBoard[b.ID])
		case KindBilling:
			view.WeekStart = weekStart.Format(time.DateOnly)
			view.WeekEnd = weekEnd.Format(time.DateOnly)
			view.Rows = s.summariseHours(b, daysByBoard[b.ID], weekStart)
		}

		out = append(out, view)
	}

	return out, nil
}

// Refresh pulls live data for every board and returns the rebuilt views.
//
// Task boards cost one paginated call per member, so they are fetched per board
// and bounded; billing boards share a single call across all of them. The views
// are then read straight back out of SQLite so that cached and freshly-synced
// output travel one code path and cannot drift apart.
func (s *Service) Refresh(ctx context.Context) ([]BoardView, error) {
	boards, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(boards) == 0 {
		return []BoardView{}, nil
	}

	creds, err := s.creds.Credentials(ctx)
	if err != nil {
		return nil, err
	}

	var billingBoards []Board

	for _, b := range boards {
		if !b.Configured() {
			// Clear whatever a previous, fuller configuration left behind, so a
			// board emptied in the editor does not keep showing old rows.
			if err := s.clearBoard(ctx, b); err != nil {
				return nil, err
			}

			continue
		}

		switch b.Kind {
		case KindTasks:
			if err := s.refreshTaskBoard(ctx, creds, b); err != nil {
				return nil, err
			}
		case KindBilling:
			billingBoards = append(billingBoards, b)
		}
	}

	if err := s.refreshBillingBoards(ctx, creds, billingBoards); err != nil {
		return nil, err
	}

	if err := s.settings.MarkTeamSynced(ctx, time.Now()); err != nil {
		return nil, err
	}

	return s.Cached(ctx)
}

func (s *Service) clearBoard(ctx context.Context, b Board) error {
	switch b.Kind {
	case KindTasks:
		if err := s.queries.DeleteTeamBoardTasks(ctx, b.ID); err != nil {
			return fmt.Errorf("team: clear tasks for board %d: %w", b.ID, err)
		}
	case KindBilling:
		if err := s.queries.DeleteTeamBoardDays(ctx, b.ID); err != nil {
			return fmt.Errorf("team: clear hours for board %d: %w", b.ID, err)
		}
	}

	return nil
}

func (s *Service) weekStartDay(ctx context.Context) (time.Weekday, error) {
	current, err := s.settings.Get(ctx)
	if err != nil {
		return time.Monday, err
	}

	return time.Weekday(current.WeekStartDay), nil
}

func (s *Service) childrenByBoard(ctx context.Context) (*children, error) {
	projects, err := s.queries.ListTeamBoardProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("team: list board projects: %w", err)
	}

	members, err := s.queries.ListTeamBoardMembers(ctx)
	if err != nil {
		return nil, fmt.Errorf("team: list board members: %w", err)
	}

	statuses, err := s.queries.ListTeamBoardStatuses(ctx)
	if err != nil {
		return nil, fmt.Errorf("team: list board statuses: %w", err)
	}

	out := &children{
		projects: map[int64][]ProjectRef{},
		members:  map[int64][]MemberRef{},
		statuses: map[int64][]StatusRef{},
	}
	for _, p := range projects {
		out.projects[p.BoardID] = append(out.projects[p.BoardID], ProjectRef{
			ProjectID: p.ProjectID, Code: p.ProjectCode, Name: p.ProjectName,
		})
	}
	for _, m := range members {
		out.members[m.BoardID] = append(out.members[m.BoardID], MemberRef{
			EmpID: m.EmpID, Name: m.EmpName,
		})
	}
	for _, st := range statuses {
		out.statuses[st.BoardID] = append(out.statuses[st.BoardID], StatusRef{
			StatusID: st.StatusID, Name: st.StatusName,
		})
	}

	return out, nil
}

type children struct {
	projects map[int64][]ProjectRef
	members  map[int64][]MemberRef
	statuses map[int64][]StatusRef
}

func board(row sqlc.TeamBoard, c *children) Board {
	out := Board{
		ID:       row.ID,
		Kind:     row.Kind,
		Name:     row.Name,
		Position: row.Position,
		Projects: c.projects[row.ID],
		Members:  c.members[row.ID],
		Statuses: c.statuses[row.ID],
	}
	if out.Projects == nil {
		out.Projects = []ProjectRef{}
	}
	if out.Members == nil {
		out.Members = []MemberRef{}
	}
	if out.Statuses == nil {
		out.Statuses = []StatusRef{}
	}

	return out
}
