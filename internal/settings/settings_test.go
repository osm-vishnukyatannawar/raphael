package settings_test

import (
	"path/filepath"
	"testing"

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
	if got.TasksSyncedAt != "" {
		t.Errorf("TasksSyncedAt = %q, want empty", got.TasksSyncedAt)
	}
}

// Clamping rather than rejecting: a mistyped interval should be corrected, and
// the caller told what was actually stored, not handed an error.
func TestSetRefreshIntervalClamps(t *testing.T) {
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

			stored, err := svc.SetRefreshInterval(t.Context(), tc.in)
			if err != nil {
				t.Fatalf("SetRefreshInterval: %v", err)
			}
			if stored != tc.want {
				t.Errorf("returned %d, want %d", stored, tc.want)
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
