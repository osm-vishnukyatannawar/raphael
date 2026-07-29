-- Cached Pinestem data plus app-level settings.
--
-- project and task are caches, not sources of truth: they are replaced wholesale
-- on every refresh so a task leaving "In review" disappears rather than lingering.

-- +goose Up
CREATE TABLE project (
    project_id INTEGER PRIMARY KEY,
    code       TEXT    NOT NULL,
    name       TEXT    NOT NULL,
    status_id  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE task (
    task_id         INTEGER PRIMARY KEY,
    short_code      TEXT    NOT NULL,
    name            TEXT    NOT NULL,
    project_code    TEXT    NOT NULL,
    project_name    TEXT    NOT NULL,
    priority        TEXT    NOT NULL DEFAULT '',
    status_type     TEXT    NOT NULL DEFAULT '',
    status_color    TEXT    NOT NULL DEFAULT '',
    -- Pinestem timestamps, stored verbatim as "YYYY-MM-DD HH:MM:SS". Kept as text
    -- rather than parsed because that format sorts correctly lexicographically,
    -- so ORDER BY works without a conversion.
    due_date        TEXT    NOT NULL DEFAULT '',
    modified_on     TEXT    NOT NULL DEFAULT '',
    sprint_name     TEXT    NOT NULL DEFAULT '',
    competency_name TEXT    NOT NULL DEFAULT '',
    synced_at       TEXT    NOT NULL
);

CREATE INDEX idx_task_modified_on ON task (modified_on DESC);

CREATE TABLE app_settings (
    id                       INTEGER PRIMARY KEY CHECK (id = 1),
    -- 0 disables automatic refresh.
    refresh_interval_seconds INTEGER NOT NULL DEFAULT 60,
    tasks_synced_at          TEXT
);

-- +goose Down
DROP TABLE app_settings;
DROP INDEX idx_task_modified_on;
DROP TABLE task;
DROP TABLE project;
