package settings_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/db"
	"github.com/osm-vishnukyatannawar/raphael/internal/settings"
)

func newService(t *testing.T) *settings.Service {
	t.Helper()

	conn, err := db.OpenAt(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return settings.New(conn)
}

func TestGetReturnsDefaultsOnFreshInstall(t *testing.T) {
	t.Parallel()

	got, err := newService(t).Get(t.Context())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.RefreshIntervalSeconds != settings.DefaultRefreshSeconds {
		t.Errorf("interval = %d, want %d", got.RefreshIntervalSeconds, settings.DefaultRefreshSeconds)
	}
	if got.BillingRefreshIntervalSeconds != settings.DefaultBillingRefreshSeconds {
		t.Errorf("billing interval = %d, want %d",
			got.BillingRefreshIntervalSeconds, settings.DefaultBillingRefreshSeconds)
	}
	// Monday: ISO-8601, and the chosen default.
	if got.WeekStartDay != int64(time.Monday) {
		t.Errorf("WeekStartDay = %d, want %d (Monday)", got.WeekStartDay, time.Monday)
	}
	// Alerts default on — an assistant that stays silent until configured is
	// indistinguishable from a broken one.
	if !got.NotifyNewTasks || !got.FocusOnNewTask {
		t.Errorf("alert defaults = notify:%v focus:%v, want both true",
			got.NotifyNewTasks, got.FocusOnNewTask)
	}
	if got.TasksSyncedAt != "" || got.BillingSyncedAt != "" {
		t.Errorf("sync stamps = %q/%q, want empty", got.TasksSyncedAt, got.BillingSyncedAt)
	}
	// The assigned-task list has its own cadence and its own alert toggle; it
	// must not inherit the review queue's.
	if got.MyTasksRefreshIntervalSeconds != settings.DefaultMyTasksRefreshSeconds {
		t.Errorf("my-tasks interval = %d, want %d",
			got.MyTasksRefreshIntervalSeconds, settings.DefaultMyTasksRefreshSeconds)
	}
	if !got.NotifyNewMyTasks {
		t.Error("NotifyNewMyTasks defaults off")
	}
	if got.MyTasksSyncedAt != "" {
		t.Errorf("MyTasksSyncedAt = %q, want empty", got.MyTasksSyncedAt)
	}
}

// The three intervals are independent: changing one must not move the others.
func TestSaveKeepsIntervalsIndependent(t *testing.T) {
	t.Parallel()

	svc := newService(t)

	stored, err := svc.Save(t.Context(), settings.Settings{
		RefreshIntervalSeconds:        30,
		BillingRefreshIntervalSeconds: 600,
		MyTasksRefreshIntervalSeconds: 900,
		WeekStartDay:                  int64(time.Monday),
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if stored.RefreshIntervalSeconds != 30 ||
		stored.BillingRefreshIntervalSeconds != 600 ||
		stored.MyTasksRefreshIntervalSeconds != 900 {
		t.Errorf("intervals = %d/%d/%d, want 30/600/900",
			stored.RefreshIntervalSeconds,
			stored.BillingRefreshIntervalSeconds,
			stored.MyTasksRefreshIntervalSeconds)
	}

	// And the my-tasks interval takes the same floor as the other two.
	clamped, err := svc.Save(t.Context(), settings.Settings{
		MyTasksRefreshIntervalSeconds: 3,
		WeekStartDay:                  int64(time.Monday),
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if clamped.MyTasksRefreshIntervalSeconds != settings.MinRefreshSeconds {
		t.Errorf("my-tasks interval = %d, want the %ds floor",
			clamped.MyTasksRefreshIntervalSeconds, settings.MinRefreshSeconds)
	}
}

func TestMarkMyTasksSyncedIsSeparateFromTheOthers(t *testing.T) {
	t.Parallel()

	svc := newService(t)

	if err := svc.MarkMyTasksSynced(t.Context(), time.Now()); err != nil {
		t.Fatalf("MarkMyTasksSynced: %v", err)
	}

	got, err := svc.Get(t.Context())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.MyTasksSyncedAt == "" {
		t.Error("MyTasksSyncedAt was not recorded")
	}
	if got.TasksSyncedAt != "" || got.BillingSyncedAt != "" {
		t.Errorf("other stamps moved: %q/%q", got.TasksSyncedAt, got.BillingSyncedAt)
	}
}

// Clamping rather than rejecting: a mistyped interval should be corrected, and
// the caller told what was actually stored, not handed an error.
func TestSaveClampsIntervals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   int64
		want int64
	}{
		{"zero disables", 0, settings.RefreshDisabled},
		{"negative disables", -30, settings.RefreshDisabled},
		{"below floor is raised", 2, settings.MinRefreshSeconds},
		{"floor itself is kept", settings.MinRefreshSeconds, settings.MinRefreshSeconds},
		{"normal value is kept", 300, 300},
		{"large value is kept", 86400, 86400},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := newService(t)

			stored, err := svc.Save(t.Context(), settings.Settings{
				RefreshIntervalSeconds:        tc.in,
				BillingRefreshIntervalSeconds: tc.in,
				WeekStartDay:                  int64(time.Monday),
			})
			if err != nil {
				t.Fatalf("Save: %v", err)
			}
			if stored.RefreshIntervalSeconds != tc.want {
				t.Errorf("tasks interval returned %d, want %d",
					stored.RefreshIntervalSeconds, tc.want)
			}
			// Both intervals share the floor; they are independent values, not
			// independent rules.
			if stored.BillingRefreshIntervalSeconds != tc.want {
				t.Errorf("billing interval returned %d, want %d",
					stored.BillingRefreshIntervalSeconds, tc.want)
			}

			got, err := svc.Get(t.Context())
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.RefreshIntervalSeconds != tc.want {
				t.Errorf("persisted %d, want %d", got.RefreshIntervalSeconds, tc.want)
			}
		})
	}
}

// An out-of-range weekday would make WeekBounds produce a nonsense window, so it
// is corrected to the default rather than stored.
func TestSaveClampsWeekStartDay(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want int64
	}{
		{-1, int64(time.Monday)},
		{7, int64(time.Monday)},
		{99, int64(time.Monday)},
		{0, int64(time.Sunday)},
		{6, int64(time.Saturday)},
		{3, int64(time.Wednesday)},
	}

	for _, tc := range cases {
		svc := newService(t)

		stored, err := svc.Save(t.Context(), settings.Settings{WeekStartDay: tc.in})
		if err != nil {
			t.Fatalf("Save(%d): %v", tc.in, err)
		}
		if stored.WeekStartDay != tc.want {
			t.Errorf("WeekStartDay(%d) = %d, want %d", tc.in, stored.WeekStartDay, tc.want)
		}
	}
}

func TestSaveRoundTripsToggles(t *testing.T) {
	t.Parallel()

	svc := newService(t)

	if _, err := svc.Save(t.Context(), settings.Settings{
		RefreshIntervalSeconds:        90,
		BillingRefreshIntervalSeconds: 600,
		WeekStartDay:                  int64(time.Sunday),
		NotifyNewTasks:                false,
		FocusOnNewTask:                true,
		NotifyNewMyTasks:              true,
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := svc.Get(t.Context())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.NotifyNewTasks {
		t.Error("NotifyNewTasks did not persist as false")
	}
	if !got.FocusOnNewTask {
		t.Error("FocusOnNewTask did not persist as true")
	}
	// Independent of the review-queue toggle above, which is off here.
	if !got.NotifyNewMyTasks {
		t.Error("NotifyNewMyTasks did not persist as true")
	}
	if got.WeekStartDay != int64(time.Sunday) || got.BillingRefreshIntervalSeconds != 600 {
		t.Errorf("round trip lost values: %+v", got)
	}
}

// Saving settings must not rewind the sync stamps — they are written by the
// refresh path and share the same single row.
func TestSaveDoesNotClearSyncStamps(t *testing.T) {
	t.Parallel()

	svc := newService(t)
	at := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)

	if err := svc.MarkTasksSynced(t.Context(), at); err != nil {
		t.Fatalf("MarkTasksSynced: %v", err)
	}
	if err := svc.MarkBillingSynced(t.Context(), at); err != nil {
		t.Fatalf("MarkBillingSynced: %v", err)
	}

	if _, err := svc.Save(t.Context(), settings.Settings{RefreshIntervalSeconds: 45}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := svc.Get(t.Context())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TasksSyncedAt == "" || got.BillingSyncedAt == "" {
		t.Errorf("sync stamps cleared by a settings save: %+v", got)
	}
}

// 0 is "until dismissed" here, not "off" — the opposite of what 0 means for the
// refresh intervals in the same struct. Worth pinning so a future tidy-up that
// unifies the clamps doesn't silently disable sticky notifications.
func TestNotificationTimeoutClamping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   int64
		want int64
	}{
		{"zero stays zero (until dismissed)", 0, settings.NotificationUntilDismissed},
		{"negative falls back to the default", -5, settings.DefaultNotificationSeconds},
		{"normal value is kept", 30, 30},
		{"one second is allowed", 1, 1},
		{"absurd value is capped", 99999, settings.MaxNotificationSeconds},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			svc := newService(t)

			stored, err := svc.Save(t.Context(), settings.Settings{
				NotificationTimeoutSeconds: tc.in,
			})
			if err != nil {
				t.Fatalf("Save: %v", err)
			}
			if stored.NotificationTimeoutSeconds != tc.want {
				t.Errorf("stored %d, want %d", stored.NotificationTimeoutSeconds, tc.want)
			}
		})
	}
}

func TestNotificationTimeoutAccessor(t *testing.T) {
	t.Parallel()

	d, sticky := settings.Settings{NotificationTimeoutSeconds: 0}.NotificationTimeout()
	if !sticky || d != 0 {
		t.Errorf("0 gave %v/%v, want 0/true", d, sticky)
	}

	d, sticky = settings.Settings{NotificationTimeoutSeconds: 12}.NotificationTimeout()
	if sticky || d != 12*time.Second {
		t.Errorf("12 gave %v/%v, want 12s/false", d, sticky)
	}
}

// The team interval is slower than the rest by default, and clamps like the
// others: a mistyped value must land on the floor rather than hammer Pinestem
// once per member per member-board.
func TestTeamRefreshIntervalDefaultsAndClamps(t *testing.T) {
	t.Parallel()

	svc := newService(t)

	got, err := svc.Get(t.Context())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TeamRefreshIntervalSeconds != settings.DefaultTeamRefreshSeconds {
		t.Errorf("default team interval = %d, want %d",
			got.TeamRefreshIntervalSeconds, settings.DefaultTeamRefreshSeconds)
	}

	in := settings.Defaults()
	in.TeamRefreshIntervalSeconds = 2

	saved, err := svc.Save(t.Context(), in)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.TeamRefreshIntervalSeconds != settings.MinRefreshSeconds {
		t.Errorf("clamped team interval = %d, want %d",
			saved.TeamRefreshIntervalSeconds, settings.MinRefreshSeconds)
	}
}

// A settings save shares the app_settings row with the sync stamps and must not
// rewind them.
func TestSaveDoesNotRewindTeamSyncStamp(t *testing.T) {
	t.Parallel()

	svc := newService(t)

	if err := svc.MarkTeamSynced(t.Context(), time.Now()); err != nil {
		t.Fatalf("MarkTeamSynced: %v", err)
	}

	saved, err := svc.Save(t.Context(), settings.Defaults())
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.TeamSyncedAt == "" {
		t.Error("TeamSyncedAt was cleared by an unrelated settings save")
	}
}
