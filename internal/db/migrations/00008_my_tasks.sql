-- Every task assigned to the user, not just the ones waiting on review.
--
-- A separate table rather than a status column on `task`: the two lists have
-- different refresh cadences, different alerting and different filters, and the
-- review cache is swapped wholesale on its own schedule. Sharing one table would
-- mean each refresh had to avoid deleting the other list's rows.

-- +goose Up

CREATE TABLE my_task (
    task_id         INTEGER PRIMARY KEY,
    short_code      TEXT    NOT NULL,
    name            TEXT    NOT NULL,
    project_code    TEXT    NOT NULL,
    project_name    TEXT    NOT NULL,
    priority        TEXT    NOT NULL DEFAULT '',
    status_id       INTEGER NOT NULL DEFAULT 0,
    status_type     TEXT    NOT NULL DEFAULT '',
    status_color    TEXT    NOT NULL DEFAULT '',
    -- Pinestem timestamps, verbatim as "YYYY-MM-DD HH:MM:SS" — that format sorts
    -- correctly as text, so ORDER BY needs no conversion. An absent due date is
    -- '' rather than NULL so the "no due date last" sort is a plain CASE.
    due_date        TEXT    NOT NULL DEFAULT '',
    modified_on     TEXT    NOT NULL DEFAULT '',
    sprint_name     TEXT    NOT NULL DEFAULT '',
    competency_name TEXT    NOT NULL DEFAULT '',
    synced_at       TEXT    NOT NULL
);

CREATE INDEX idx_my_task_due_date ON my_task (due_date);

-- Same side-table trick as seen_task: the row in my_task is deleted and
-- reinserted on every refresh, so acknowledgement has to live outside it.
CREATE TABLE seen_my_task (
    task_id       INTEGER PRIMARY KEY,
    first_seen_at TEXT    NOT NULL,
    acknowledged  INTEGER NOT NULL DEFAULT 0
);

-- Hidden tasks, unlike seen ones, are NOT pruned on refresh. Hiding is a lasting
-- "stop showing me this", and a task that briefly drops out of the filtered set
-- must not come back unhidden. The rows are cheap; nothing prunes them.
CREATE TABLE hidden_my_task (
    task_id   INTEGER PRIMARY KEY,
    -- Denormalised so the "show hidden" list can still name a task that no
    -- longer comes back from the API at all.
    short_code TEXT NOT NULL DEFAULT '',
    name       TEXT NOT NULL DEFAULT '',
    hidden_at  TEXT NOT NULL
);

-- The filters. An empty table means "no filter" — every active project, and
-- every status that is not done. That is what a fresh install gets, so the list
-- is useful before anyone opens the filter dialog.
CREATE TABLE my_task_project_filter (
    project_code TEXT PRIMARY KEY,
    project_name TEXT NOT NULL DEFAULT ''
);

CREATE TABLE my_task_status_filter (
    status_id   INTEGER PRIMARY KEY,
    status_name TEXT NOT NULL DEFAULT ''
);

-- Its own cadence, deliberately not shared with the review queue: this list is
-- everything assigned to you and moves far more slowly than a review queue other
-- people push into.
ALTER TABLE app_settings ADD COLUMN my_tasks_refresh_interval_seconds INTEGER NOT NULL DEFAULT 300;
ALTER TABLE app_settings ADD COLUMN notify_new_my_tasks INTEGER NOT NULL DEFAULT 1;
ALTER TABLE app_settings ADD COLUMN my_tasks_synced_at TEXT;

-- +goose Down
ALTER TABLE app_settings DROP COLUMN my_tasks_synced_at;
ALTER TABLE app_settings DROP COLUMN notify_new_my_tasks;
ALTER TABLE app_settings DROP COLUMN my_tasks_refresh_interval_seconds;
DROP TABLE my_task_status_filter;
DROP TABLE my_task_project_filter;
DROP TABLE hidden_my_task;
DROP TABLE seen_my_task;
DROP INDEX idx_my_task_due_date;
DROP TABLE my_task;
