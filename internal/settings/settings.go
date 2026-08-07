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

	// DefaultMyTasksRefreshSeconds is the cadence for the wider assigned-task
	// list. It has its own interval rather than sharing the review queue's:
	// everything assigned to you turns over far more slowly than the subset
	// actively waiting on your review.
	DefaultMyTasksRefreshSeconds = 300

	// DefaultTeamRefreshSeconds is the cadence for team boards, and the slowest
	// of the lot on purpose. Other people's queues turn over more slowly than
	// your own, nothing here notifies, and a task board costs one paginated
	// request *per member* -- so a short interval is far more expensive than the
	// number suggests.
	DefaultTeamRefreshSeconds = 600

	// MinRefreshSeconds is the floor for a non-zero interval. Anything shorter
	// would hammer Pinestem for no practical gain.
	MinRefreshSeconds = 15

	// RefreshDisabled turns automatic refresh off entirely.
	RefreshDisabled = 0

	// DefaultWeekStartDay is Monday (ISO-8601).
	DefaultWeekStartDay = int64(time.Monday)

	// DefaultNotificationSeconds is how long a new-task notification stays up.
	DefaultNotificationSeconds = 10

	// NotificationUntilDismissed keeps a notification on screen indefinitely.
	// It is 0 because that is what the freedesktop Notify expire_timeout
	// argument uses, and this value is passed straight through to it.
	NotificationUntilDismissed = 0

	// MaxNotificationSeconds is an upper bound so a typo can't wedge a
	// notification on screen for a day. Use 0 for "until dismissed" instead.
	MaxNotificationSeconds = 3600
)

// Settings is the app's preferences, as handed to the frontend.
type Settings struct {
	RefreshIntervalSeconds        int64 `json:"refreshIntervalSeconds"`
	BillingRefreshIntervalSeconds int64 `json:"billingRefreshIntervalSeconds"`
	MyTasksRefreshIntervalSeconds int64 `json:"myTasksRefreshIntervalSeconds"`
	TeamRefreshIntervalSeconds    int64 `json:"teamRefreshIntervalSeconds"`
	// WeekStartDay is 0=Sunday … 6=Saturday, matching Go's time.Weekday.
	WeekStartDay   int64 `json:"weekStartDay"`
	NotifyNewTasks bool  `json:"notifyNewTasks"`
	FocusOnNewTask bool  `json:"focusOnNewTask"`
	// NotifyNewMyTasks alerts for the wider assigned list. There is deliberately
	// no focus counterpart: a task landing in your backlog is worth a
	// notification, not worth pulling the window over whatever you are doing.
	NotifyNewMyTasks bool `json:"notifyNewMyTasks"`
	// NotificationTimeoutSeconds is 0 for "until dismissed". Unlike the refresh
	// intervals, 0 here does not mean "off".
	NotificationTimeoutSeconds int64  `json:"notificationTimeoutSeconds"`
	TasksSyncedAt              string `json:"tasksSyncedAt"`
	BillingSyncedAt            string `json:"billingSyncedAt"`
	MonitorsSyncedAt           string `json:"monitorsSyncedAt"`
	MyTasksSyncedAt            string `json:"myTasksSyncedAt"`
	TeamSyncedAt               string `json:"teamSyncedAt"`
}

// NotificationTimeout is the configured duration, and whether it should stay up
// until the user dismisses it.
func (s Settings) NotificationTimeout() (d time.Duration, untilDismissed bool) {
	if s.NotificationTimeoutSeconds == NotificationUntilDismissed {
		return 0, true
	}

	return time.Duration(s.NotificationTimeoutSeconds) * time.Second, false
}

// Defaults is what a fresh install starts with.
func Defaults() Settings {
	return Settings{
		RefreshIntervalSeconds:        DefaultRefreshSeconds,
		BillingRefreshIntervalSeconds: DefaultBillingRefreshSeconds,
		MyTasksRefreshIntervalSeconds: DefaultMyTasksRefreshSeconds,
		TeamRefreshIntervalSeconds:    DefaultTeamRefreshSeconds,
		WeekStartDay:                  DefaultWeekStartDay,
		NotifyNewTasks:                true,
		FocusOnNewTask:                true,
		NotifyNewMyTasks:              true,
		NotificationTimeoutSeconds:    DefaultNotificationSeconds,
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
		MyTasksRefreshIntervalSeconds: row.MyTasksRefreshIntervalSeconds,
		TeamRefreshIntervalSeconds:    row.TeamRefreshIntervalSeconds,
		WeekStartDay:                  row.WeekStartDay,
		NotifyNewTasks:                row.NotifyNewTasks == 1,
		FocusOnNewTask:                row.FocusOnNewTask == 1,
		NotifyNewMyTasks:              row.NotifyNewMyTasks == 1,
		NotificationTimeoutSeconds:    row.NotificationTimeoutSeconds,
	}
	if row.TasksSyncedAt != nil {
		out.TasksSyncedAt = *row.TasksSyncedAt
	}
	if row.BillingSyncedAt != nil {
		out.BillingSyncedAt = *row.BillingSyncedAt
	}
	if row.MonitorsSyncedAt != nil {
		out.MonitorsSyncedAt = *row.MonitorsSyncedAt
	}
	if row.MyTasksSyncedAt != nil {
		out.MyTasksSyncedAt = *row.MyTasksSyncedAt
	}
	if row.TeamSyncedAt != nil {
		out.TeamSyncedAt = *row.TeamSyncedAt
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
		MyTasksRefreshIntervalSeconds: ClampInterval(in.MyTasksRefreshIntervalSeconds),
		TeamRefreshIntervalSeconds:    ClampInterval(in.TeamRefreshIntervalSeconds),
		WeekStartDay:                  ClampWeekStartDay(in.WeekStartDay),
		NotifyNewTasks:                boolToInt(in.NotifyNewTasks),
		FocusOnNewTask:                boolToInt(in.FocusOnNewTask),
		NotifyNewMyTasks:              boolToInt(in.NotifyNewMyTasks),
		NotificationTimeoutSeconds:    ClampNotificationTimeout(in.NotificationTimeoutSeconds),
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

// MarkTeamSynced records when the team board caches were last refreshed.
func (s *Service) MarkTeamSynced(ctx context.Context, at time.Time) error {
	stamp := at.UTC().Format(time.RFC3339)
	if err := s.queries.SetTeamSyncedAt(ctx, &stamp); err != nil {
		return fmt.Errorf("settings: record team sync time: %w", err)
	}

	return nil
}

// MarkMyTasksSynced records when the assigned-task cache was last refreshed.
func (s *Service) MarkMyTasksSynced(ctx context.Context, at time.Time) error {
	stamp := at.UTC().Format(time.RFC3339)
	if err := s.queries.SetMyTasksSyncedAt(ctx, &stamp); err != nil {
		return fmt.Errorf("settings: record my-task sync time: %w", err)
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

// ClampNotificationTimeout keeps the timeout sane. A negative value is a typo
// rather than an intent, so it falls back to the default; 0 is left alone
// because it is the deliberate "until dismissed" setting.
func ClampNotificationTimeout(seconds int64) int64 {
	switch {
	case seconds == NotificationUntilDismissed:
		return NotificationUntilDismissed
	case seconds < 0:
		return DefaultNotificationSeconds
	case seconds > MaxNotificationSeconds:
		return MaxNotificationSeconds
	default:
		return seconds
	}
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}

	return 0
}
