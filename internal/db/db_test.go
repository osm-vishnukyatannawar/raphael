package db_test

import (
	"path/filepath"
	"testing"

	"github.com/osm-vishnukyatannawar/raphael/internal/db"
)

// Uses a temp file rather than :memory: — an in-memory database gives each
// pooled connection its own empty schema, so migrations appear to vanish.
func TestOpenAtRunsMigrations(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "test.db")

	conn, err := db.OpenAt(t.Context(), path)
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})

	var fk int
	if err := conn.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("read foreign_keys pragma: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}

	// goose records applied migrations here; its existence proves migrate() ran.
	var name string
	err = conn.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='goose_db_version'",
	).Scan(&name)
	if err != nil {
		t.Fatalf("goose version table missing: %v", err)
	}
}

// Reopening must be a no-op rather than re-applying or erroring, since every
// app launch calls Open on an existing database.
func TestOpenAtIsIdempotent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "test.db")

	for i := range 2 {
		conn, err := db.OpenAt(t.Context(), path)
		if err != nil {
			t.Fatalf("OpenAt call %d: %v", i+1, err)
		}
		if err := conn.Close(); err != nil {
			t.Fatalf("close call %d: %v", i+1, err)
		}
	}
}
