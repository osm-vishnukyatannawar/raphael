// Package settings stores app-level preferences in SQLite.
package settings

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/db/sqlc"
)

const (
	// DefaultRefreshSeconds is the task auto-refresh cadence on a fresh install.
	DefaultRefreshSeconds = 60

	// DefaultBillingRefreshSeconds is the billing cadence. Slower than tasks on
	// purpose: hours you logged yourself change far less often than a review
	// queue other people push into.
	DefaultBillingRefreshSeconds = 300

	// MinRefreshSeconds is the floor for a non-zero interval. Anything shorter
	// would hammer Pinestem for no practical gain.
	MinRefreshSeconds = 15

	// RefreshDisabled turns automatic refresh off entirely.
	RefreshDisabled = 0

	// DefaultWeekStartDay is Monday (ISO-8601).
	DefaultWeekStartDay = int64(time.Monday)
)

// Settings is the app's preferences, as handed to the frontend.
type Settings struct {
	RefreshIntervalSeconds        int64 `json:"refreshIntervalSeconds"`
	BillingRefreshIntervalSeconds int64 `json:"billingRefreshIntervalSeconds"`
	// WeekStartDay is 0=Sunday … 6=Saturday, matching Go's time.Weekday.
	WeekStartDay    int64  `json:"weekStartDay"`
	NotifyNewTasks  bool   `json:"notifyNewTasks"`
	FocusOnNewTask  bool   `json:"focusOnNewTask"`
	TasksSyncedAt   string `json:"tasksSyncedAt"`
	BillingSyncedAt string `json:"billingSyncedAt"`
}

// Defaults is what a fresh install starts with.
func Defaults() Settings {
	return Settings{
		RefreshIntervalSeconds:        DefaultRefreshSeconds,
		BillingRefreshIntervalSeconds: DefaultBillingRefreshSeconds,
		WeekStartDay:                  DefaultWeekStartDay,
		NotifyNewTasks:                true,
		FocusOnNewTask:                true,
	}
}

type Service struct {
	queries *sqlc.Queries
}

func New(database *sql.DB) *Service {
	return &Service{queries: sqlc.New(database)}
}

// Get returns stored settings, falling back to defaults on a fresh install.
func (s *Service) Get(ctx context.Context) (*Settings, error) {
	row, err := s.queries.GetSettings(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		out := Defaults()

		return &out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("settings: read: %w", err)
	}

	out := Settings{
		RefreshIntervalSeconds:        row.RefreshIntervalSeconds,
		BillingRefreshIntervalSeconds: row.BillingRefreshIntervalSeconds,
		WeekStartDay:                  row.WeekStartDay,
		NotifyNewTasks:                row.NotifyNewTasks == 1,
		FocusOnNewTask:                row.FocusOnNewTask == 1,
	}
	if row.TasksSyncedAt != nil {
		out.TasksSyncedAt = *row.TasksSyncedAt
	}
	if row.BillingSyncedAt != nil {
		out.BillingSyncedAt = *row.BillingSyncedAt
	}

	return &out, nil
}

// Save stores the user-editable preferences and returns them as persisted.
//
// Values are clamped rather than rejected: a mistyped 2 becomes the 15s floor
// instead of erroring. The returned value is therefore the source of truth for
// what the UI should display, not whatever was submitted.
//
// The sync stamps are deliberately not written here — they share the same row
// but belong to the refresh path, and a settings save must not rewind them.
func (s *Service) Save(ctx context.Context, in Settings) (*Settings, error) {
	err := s.queries.SaveSettings(ctx, sqlc.SaveSettingsParams{
		RefreshIntervalSeconds:        ClampInterval(in.RefreshIntervalSeconds),
		BillingRefreshIntervalSeconds: ClampInterval(in.BillingRefreshIntervalSeconds),
		WeekStartDay:                  ClampWeekStartDay(in.WeekStartDay),
		NotifyNewTasks:                boolToInt(in.NotifyNewTasks),
		FocusOnNewTask:                boolToInt(in.FocusOnNewTask),
	})
	if err != nil {
		return nil, fmt.Errorf("settings: save: %w", err)
	}

	return s.Get(ctx)
}

// MarkTasksSynced records when the task cache was last refreshed.
func (s *Service) MarkTasksSynced(ctx context.Context, at time.Time) error {
	stamp := at.UTC().Format(time.RFC3339)
	if err := s.queries.SetTasksSyncedAt(ctx, &stamp); err != nil {
		return fmt.Errorf("settings: record task sync time: %w", err)
	}

	return nil
}

// MarkBillingSynced records when the billing cache was last refreshed.
func (s *Service) MarkBillingSynced(ctx context.Context, at time.Time) error {
	stamp := at.UTC().Format(time.RFC3339)
	if err := s.queries.SetBillingSyncedAt(ctx, &stamp); err != nil {
		return fmt.Errorf("settings: record billing sync time: %w", err)
	}

	return nil
}

// ClampInterval normalises an interval to 0 (disabled) or at least MinRefreshSeconds.
func ClampInterval(seconds int64) int64 {
	switch {
	case seconds <= 0:
		return RefreshDisabled
	case seconds < MinRefreshSeconds:
		return MinRefreshSeconds
	default:
		return seconds
	}
}

// ClampWeekStartDay keeps the value a real weekday. Out of range falls back to
// the default rather than erroring — an impossible weekday would otherwise make
// billing.WeekBounds compute a nonsense window.
func ClampWeekStartDay(day int64) int64 {
	if day < int64(time.Sunday) || day > int64(time.Saturday) {
		return DefaultWeekStartDay
	}

	return day
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}

	return 0
}
