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
	// DefaultRefreshSeconds is the auto-refresh cadence on a fresh install.
	DefaultRefreshSeconds = 60

	// MinRefreshSeconds is the floor for a non-zero interval. Anything shorter
	// would hammer Pinestem — two requests per cycle — for no practical gain.
	MinRefreshSeconds = 15

	// RefreshDisabled turns automatic refresh off entirely.
	RefreshDisabled = 0
)

// Settings is the app's preferences, as handed to the frontend.
type Settings struct {
	RefreshIntervalSeconds int64  `json:"refreshIntervalSeconds"`
	TasksSyncedAt          string `json:"tasksSyncedAt"`
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
		return &Settings{RefreshIntervalSeconds: DefaultRefreshSeconds}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("settings: read: %w", err)
	}

	out := &Settings{RefreshIntervalSeconds: row.RefreshIntervalSeconds}
	if row.TasksSyncedAt != nil {
		out.TasksSyncedAt = *row.TasksSyncedAt
	}

	return out, nil
}

// SetRefreshInterval stores the auto-refresh cadence.
//
// The value is clamped rather than rejected: a mistyped 2 becomes the 15s floor
// instead of erroring, and negatives collapse to "disabled". Callers get back
// what was actually stored so the UI can show the corrected value.
func (s *Service) SetRefreshInterval(ctx context.Context, seconds int64) (int64, error) {
	seconds = ClampInterval(seconds)

	if err := s.queries.SetRefreshInterval(ctx, seconds); err != nil {
		return 0, fmt.Errorf("settings: save refresh interval: %w", err)
	}

	return seconds, nil
}

// MarkTasksSynced records when the task cache was last refreshed.
func (s *Service) MarkTasksSynced(ctx context.Context, at time.Time) error {
	stamp := at.UTC().Format(time.RFC3339)
	if err := s.queries.SetTasksSyncedAt(ctx, &stamp); err != nil {
		return fmt.Errorf("settings: record sync time: %w", err)
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
