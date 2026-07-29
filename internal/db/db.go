// Package db owns the SQLite connection and schema migrations.
//
// The driver is modernc.org/sqlite (pure Go, no cgo) so that the database layer
// adds no cross-compilation constraints beyond the ones Wails already imposes
// for the webview.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/osm-vishnukyatannawar/raphael/internal/config"
)

// migrationsFS carries the schema into the binary, so a released build needs no
// files on disk to bring a fresh database up to date.
//
//go:embed all:migrations
var migrationsFS embed.FS

// Open connects to the user's database and applies any pending migrations.
func Open(ctx context.Context) (*sql.DB, error) {
	path, err := config.DatabasePath()
	if err != nil {
		return nil, err
	}

	return OpenAt(ctx, path)
}

// OpenAt is Open against an explicit path. Tests use it with a temp file.
func OpenAt(ctx context.Context, path string) (*sql.DB, error) {
	// _pragma args are modernc-specific; foreign keys are off by default in
	// SQLite and busy_timeout avoids SQLITE_BUSY when the sync engine and the
	// UI touch the database concurrently.
	dsn := path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}

	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ping sqlite %q: %w", path, err)
	}

	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

func migrate(conn *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.Up(conn, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
