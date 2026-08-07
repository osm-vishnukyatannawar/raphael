package team

import (
	"context"
	"fmt"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/billing"
	"github.com/osm-vishnukyatannawar/raphael/internal/db/sqlc"
	"github.com/osm-vishnukyatannawar/raphael/internal/identity"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
)

// dayKey identifies one person's work on one project on one day.
//
// Project is part of the key because a single fetch covers the union of every
// billing board's projects. Without it, a board scoped to one project would be
// credited with hours its members logged on somebody else's board.
type dayKey struct {
	empID, projectID int64
	day              string
}

// refreshBillingBoards replaces the cached hours for every billing board.
//
// One API call covers them all: omitting EmpID returns the whole team and each
// row carries EmpID and ProjectID, so the rows can be sliced per board locally.
// Adding a board therefore costs no extra requests -- only widening the set of
// projects does. This is the same trick internal/monitor uses.
func (s *Service) refreshBillingBoards(
	ctx context.Context, creds *identity.Credentials, boards []Board,
) error {
	if len(boards) == 0 {
		return nil
	}

	weekStartDay, err := s.weekStartDay(ctx)
	if err != nil {
		return err
	}

	// billing.FetchWindow, not a hand-rolled week: it stretches back to include
	// yesterday, which on the first day of a week falls in the previous one and
	// would otherwise read as zero.
	start, end := billing.FetchWindow(s.now(), weekStartDay)

	entries, err := s.fetcher.BillingEntries(ctx, pinestem.BillingRequest{
		Token:     creds.Token,
		CompanyID: creds.CompanyID,
		CallerID:  creds.UserID,
		// EmpID deliberately left zero: these boards are other people's hours,
		// so the filter has to be off and the slicing done here.
		EmpID:            0,
		ProjectIDs:       unionProjectIDs(boards),
		RoleID:           creds.RoleID,
		IsProjectManager: creds.IsProjectManager,
		TimeZone:         creds.TimeZone,
		Start:            start,
		End:              end,
	})
	if err != nil {
		return err
	}

	totals := totalsByKey(entries)

	for _, b := range boards {
		if err := s.replaceBoardDays(ctx, b, totals); err != nil {
			return err
		}
	}

	return nil
}

func unionProjectIDs(boards []Board) []int64 {
	seen := map[int64]bool{}
	out := []int64{}

	for _, b := range boards {
		for _, id := range b.projectIDs() {
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
	}

	return out
}

func totalsByKey(entries []pinestem.BillingEntry) map[dayKey]billing.DayTotal {
	out := map[dayKey]billing.DayTotal{}

	for _, e := range entries {
		// Entry dates are "YYYY-MM-DD HH:MM:SS"; the first ten characters are
		// the calendar day, no parsing required.
		if len(e.Date) < len(time.DateOnly) {
			continue
		}
		day := e.Date[:len(time.DateOnly)]

		key := dayKey{empID: e.EmpID, projectID: e.ProjectID, day: day}
		total := out[key]
		total.Day = day
		total.BillableMinutes += e.BillableMinutes
		total.NonBillableMinutes += e.NonBillableMinutes
		out[key] = total
	}

	return out
}

// replaceBoardDays swaps a board's whole hours cache inside one transaction.
func (s *Service) replaceBoardDays(
	ctx context.Context, b Board, totals map[dayKey]billing.DayTotal,
) error {
	// Collapse the shared fetch down to this board's people and projects.
	perMemberDay := map[dayKey]billing.DayTotal{}
	members := map[int64]bool{}
	projects := map[int64]bool{}

	for _, m := range b.Members {
		members[m.EmpID] = true
	}
	for _, id := range b.projectIDs() {
		projects[id] = true
	}

	for key, total := range totals {
		if !members[key.empID] || !projects[key.projectID] {
			continue
		}

		// projectID drops out here: a board reports a person's hours across all
		// of its projects, not one row per project.
		collapsed := dayKey{empID: key.empID, day: key.day}
		running := perMemberDay[collapsed]
		running.Day = key.day
		running.BillableMinutes += total.BillableMinutes
		running.NonBillableMinutes += total.NonBillableMinutes
		perMemberDay[collapsed] = running
	}

	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("team: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)

	if err := qtx.DeleteTeamBoardDays(ctx, b.ID); err != nil {
		return fmt.Errorf("team: clear hours for board %d: %w", b.ID, err)
	}

	for key, total := range perMemberDay {
		err := qtx.InsertTeamBoardDay(ctx, sqlc.InsertTeamBoardDayParams{
			BoardID:            b.ID,
			EmpID:              key.empID,
			Day:                key.day,
			BillableMinutes:    total.BillableMinutes,
			NonBillableMinutes: total.NonBillableMinutes,
		})
		if err != nil {
			return fmt.Errorf("team: cache hours for board %d: %w", b.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("team: commit hours for board %d: %w", b.ID, err)
	}

	return nil
}

func (s *Service) cachedDays(ctx context.Context) (map[int64]map[int64]map[string]billing.DayTotal, error) {
	rows, err := s.queries.ListTeamBoardDays(ctx)
	if err != nil {
		return nil, fmt.Errorf("team: read cached hours: %w", err)
	}

	out := map[int64]map[int64]map[string]billing.DayTotal{}
	for _, r := range rows {
		byMember, ok := out[r.BoardID]
		if !ok {
			byMember = map[int64]map[string]billing.DayTotal{}
			out[r.BoardID] = byMember
		}

		byDay, ok := byMember[r.EmpID]
		if !ok {
			byDay = map[string]billing.DayTotal{}
			byMember[r.EmpID] = byDay
		}

		byDay[r.Day] = billing.DayTotal{
			Day:                r.Day,
			BillableMinutes:    r.BillableMinutes,
			NonBillableMinutes: r.NonBillableMinutes,
		}
	}

	return out, nil
}

// summariseHours projects the cached days onto the current week, one row per
// configured member and in the board's own member order.
//
// Today and yesterday are read from the same map as the week grid, so they cost
// nothing extra and cannot disagree with it. Mirrors billing.summarise.
func (s *Service) summariseHours(
	b Board, byMember map[int64]map[string]billing.DayTotal, weekStart time.Time,
) []MemberHours {
	now := s.now()
	today := billing.StartOfDay(now).Format(time.DateOnly)
	yesterday := billing.StartOfDay(now).AddDate(0, 0, -1).Format(time.DateOnly)

	out := make([]MemberHours, 0, len(b.Members))
	for _, m := range b.Members {
		byDay := byMember[m.EmpID]

		row := MemberHours{
			EmpID:   m.EmpID,
			EmpName: m.Name,
			Days:    make([]billing.DayTotal, 0, 7),
		}

		// Exactly seven zero-filled days so the grid never handles gaps.
		for i := range 7 {
			day := weekStart.AddDate(0, 0, i).Format(time.DateOnly)

			total, ok := byDay[day]
			if !ok {
				total = billing.DayTotal{Day: day}
			}
			row.Days = append(row.Days, total)
			row.WeekBillableMinutes += total.BillableMinutes
			row.WeekNonBillableMinutes += total.NonBillableMinutes
		}
		row.WeekMinutes = row.WeekBillableMinutes + row.WeekNonBillableMinutes

		// Today and yesterday come from the map, not the grid: on the first day
		// of a week yesterday is outside it.
		row.TodayMinutes = byDay[today].Minutes()
		row.YesterdayMinutes = byDay[yesterday].Minutes()

		out = append(out, row)
	}

	return out
}
