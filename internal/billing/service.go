// Package billing fetches the caller's logged hours from Pinestem and caches
// per-day totals locally so the UI can paint immediately on launch.
package billing

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

// dateOnly is how Pinestem stamps entries and how days are keyed here.
const dateOnly = time.DateOnly

// Fetcher is the slice of the Pinestem client this package needs.
type Fetcher interface {
	BillingEntries(ctx context.Context, req pinestem.BillingRequest) ([]pinestem.BillingEntry, error)
}

// CredentialSource hands out the stored Pinestem session.
type CredentialSource interface {
	Credentials(ctx context.Context) (*identity.Credentials, error)
}

// DayTotal is one calendar day's logged time.
type DayTotal struct {
	Day                string `json:"day"`
	BillableMinutes    int64  `json:"billableMinutes"`
	NonBillableMinutes int64  `json:"nonBillableMinutes"`
}

// Minutes is the day's total.
func (d DayTotal) Minutes() int64 {
	return d.BillableMinutes + d.NonBillableMinutes
}

// Summary is what the UI renders. Days always holds exactly seven entries,
// oldest first and zero-filled, so the week grid never has to handle gaps.
type Summary struct {
	TodayMinutes           int64      `json:"todayMinutes"`
	YesterdayMinutes       int64      `json:"yesterdayMinutes"`
	WeekMinutes            int64      `json:"weekMinutes"`
	WeekBillableMinutes    int64      `json:"weekBillableMinutes"`
	WeekNonBillableMinutes int64      `json:"weekNonBillableMinutes"`
	WeekStart              string     `json:"weekStart"`
	WeekEnd                string     `json:"weekEnd"`
	Days                   []DayTotal `json:"days"`
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

// Cached builds the summary from stored per-day totals, without any network.
func (s *Service) Cached(ctx context.Context) (*Summary, error) {
	rows, err := s.queries.ListBillingDays(ctx)
	if err != nil {
		return nil, fmt.Errorf("billing: read cache: %w", err)
	}

	byDay := make(map[string]DayTotal, len(rows))
	for _, r := range rows {
		byDay[r.Day] = DayTotal{
			Day:                r.Day,
			BillableMinutes:    r.BillableMinutes,
			NonBillableMinutes: r.NonBillableMinutes,
		}
	}

	weekStart, err := s.weekStartDay(ctx)
	if err != nil {
		return nil, err
	}

	return s.summarise(byDay, weekStart), nil
}

// Refresh pulls live entries covering the current week plus yesterday, replaces
// the cache with their per-day totals, and returns the new summary.
func (s *Service) Refresh(ctx context.Context) (*Summary, error) {
	creds, err := s.creds.Credentials(ctx)
	if err != nil {
		return nil, err
	}

	weekStartDay, err := s.weekStartDay(ctx)
	if err != nil {
		return nil, err
	}

	now := s.now()
	start, end := fetchWindow(now, weekStartDay)

	entries, err := s.fetcher.BillingEntries(ctx, requestFor(creds, start, end))
	if err != nil {
		return nil, err
	}

	byDay := totalsByDay(entries)
	if err := s.replaceCache(ctx, byDay); err != nil {
		return nil, err
	}

	if err := s.settings.MarkBillingSynced(ctx, time.Now()); err != nil {
		return nil, err
	}

	return s.summarise(byDay, weekStartDay), nil
}

// ForDate answers a one-off lookup for any date.
//
// Deliberately not cached: the cache models "the current week", and writing an
// arbitrary historical day into it would make Cached's week total wrong.
func (s *Service) ForDate(ctx context.Context, day string) (*DayTotal, error) {
	parsed, err := time.ParseInLocation(dateOnly, day, time.Local)
	if err != nil {
		return nil, fmt.Errorf("billing: %q is not a YYYY-MM-DD date: %w", day, err)
	}

	creds, err := s.creds.Credentials(ctx)
	if err != nil {
		return nil, err
	}

	entries, err := s.fetcher.BillingEntries(
		ctx, requestFor(creds, StartOfDay(parsed), EndOfDay(parsed)),
	)
	if err != nil {
		return nil, err
	}

	total := DayTotal{Day: day}
	for _, e := range entries {
		total.BillableMinutes += e.BillableMinutes
		total.NonBillableMinutes += e.NonBillableMinutes
	}

	return &total, nil
}

// fetchWindow is the range to request: the configured week, stretched back to
// include yesterday. On the first day of the week yesterday falls in the
// previous one, and without this the "Yesterday" figure would silently read
// zero rather than being fetched.
func fetchWindow(now time.Time, startDay time.Weekday) (time.Time, time.Time) {
	weekStart, _ := WeekBounds(now, startDay)

	start := weekStart
	if yesterday := StartOfDay(now).AddDate(0, 0, -1); yesterday.Before(start) {
		start = yesterday
	}

	// End at today rather than the end of the week: future days hold nothing,
	// and a narrower window is a cheaper query.
	return start, EndOfDay(now)
}

func requestFor(creds *identity.Credentials, start, end time.Time) pinestem.BillingRequest {
	return pinestem.BillingRequest{
		Token:     creds.Token,
		CompanyID: creds.CompanyID,
		// EmpID is the caller's own per-company UserID today. It is a distinct
		// field so that reporting on a colleague later needs no new plumbing.
		EmpID:            creds.UserID,
		RoleID:           creds.RoleID,
		IsProjectManager: creds.IsProjectManager,
		TimeZone:         creds.TimeZone,
		Start:            start,
		End:              end,
	}
}

func totalsByDay(entries []pinestem.BillingEntry) map[string]DayTotal {
	byDay := make(map[string]DayTotal)

	for _, e := range entries {
		// Entry dates are "YYYY-MM-DD HH:MM:SS"; the first ten characters are the
		// calendar day, no parsing required.
		if len(e.Date) < len(dateOnly) {
			continue
		}
		day := e.Date[:len(dateOnly)]

		total := byDay[day]
		total.Day = day
		total.BillableMinutes += e.BillableMinutes
		total.NonBillableMinutes += e.NonBillableMinutes
		byDay[day] = total
	}

	return byDay
}

// summarise projects per-day totals onto the current week.
//
// Today and yesterday are read from the same map, so they cost nothing extra
// and stay consistent with the week grid.
func (s *Service) summarise(byDay map[string]DayTotal, startDay time.Weekday) *Summary {
	now := s.now()
	weekStart, weekEnd := WeekBounds(now, startDay)

	out := &Summary{
		WeekStart:        weekStart.Format(dateOnly),
		WeekEnd:          weekEnd.Format(dateOnly),
		TodayMinutes:     byDay[now.Format(dateOnly)].Minutes(),
		YesterdayMinutes: byDay[now.AddDate(0, 0, -1).Format(dateOnly)].Minutes(),
		Days:             make([]DayTotal, 0, 7),
	}

	for i := range 7 {
		key := weekStart.AddDate(0, 0, i).Format(dateOnly)

		day, ok := byDay[key]
		if !ok {
			day = DayTotal{Day: key}
		}

		out.Days = append(out.Days, day)
		out.WeekBillableMinutes += day.BillableMinutes
		out.WeekNonBillableMinutes += day.NonBillableMinutes
	}
	out.WeekMinutes = out.WeekBillableMinutes + out.WeekNonBillableMinutes

	return out
}

// replaceCache swaps the whole cache inside one transaction, for the same
// reason internal/tasks does: a deleted Pinestem entry must make its day shrink,
// and a mid-refresh failure must leave the previous numbers intact.
func (s *Service) replaceCache(ctx context.Context, byDay map[string]DayTotal) error {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("billing: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	qtx := s.queries.WithTx(tx)

	if err := qtx.DeleteAllBillingDays(ctx); err != nil {
		return fmt.Errorf("billing: clear cache: %w", err)
	}
	for _, d := range byDay {
		err := qtx.InsertBillingDay(ctx, sqlc.InsertBillingDayParams{
			Day:                d.Day,
			BillableMinutes:    d.BillableMinutes,
			NonBillableMinutes: d.NonBillableMinutes,
		})
		if err != nil {
			return fmt.Errorf("billing: cache %s: %w", d.Day, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("billing: commit cache: %w", err)
	}

	return nil
}

func (s *Service) weekStartDay(ctx context.Context) (time.Weekday, error) {
	current, err := s.settings.Get(ctx)
	if err != nil {
		return time.Monday, err
	}

	return time.Weekday(settings.ClampWeekStartDay(current.WeekStartDay)), nil
}
