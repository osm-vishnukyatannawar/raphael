-- Saved billing targets for a group of projects and people.

-- +goose Up
CREATE TABLE monitor (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- code and name are denormalised deliberately. The `project` table is a cache
-- wiped and rebuilt on every task refresh, so joining to it would make a saved
-- monitor's labels depend on whether the last sync happened to succeed — and on
-- whether the project is still in the narrow status filter the task list uses.
CREATE TABLE monitor_project (
    monitor_id   INTEGER NOT NULL REFERENCES monitor (id) ON DELETE CASCADE,
    project_id   INTEGER NOT NULL,
    project_code TEXT    NOT NULL,
    project_name TEXT    NOT NULL,
    PRIMARY KEY (monitor_id, project_id)
);

-- project_id = 0 means "this person, across every project in the monitor".
-- A sentinel rather than NULL so it can sit in the primary key: NULLs are
-- distinct from each other in SQLite, which would allow duplicate rows for the
-- same person.
CREATE TABLE monitor_target (
    monitor_id     INTEGER NOT NULL REFERENCES monitor (id) ON DELETE CASCADE,
    emp_id         INTEGER NOT NULL,
    emp_name       TEXT    NOT NULL,
    project_id     INTEGER NOT NULL DEFAULT 0,
    target_minutes INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (monitor_id, emp_id, project_id)
);

-- Actuals for the current period only, replaced wholesale per refresh like the
-- other caches. Keyed by the real project_id (never the 0 sentinel) so one row
-- can serve both a per-project target and an across-projects one.
CREATE TABLE monitor_actual (
    monitor_id           INTEGER NOT NULL REFERENCES monitor (id) ON DELETE CASCADE,
    emp_id               INTEGER NOT NULL,
    project_id           INTEGER NOT NULL,
    billable_minutes     INTEGER NOT NULL DEFAULT 0,
    non_billable_minutes INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (monitor_id, emp_id, project_id)
);

ALTER TABLE app_settings ADD COLUMN monitors_synced_at TEXT;

-- +goose Down
ALTER TABLE app_settings DROP COLUMN monitors_synced_at;
DROP TABLE monitor_actual;
DROP TABLE monitor_target;
DROP TABLE monitor_project;
DROP TABLE monitor;
