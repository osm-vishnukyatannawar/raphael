-- Named, user-created views of *other* people's work.
--
-- Two kinds share one table because everything except the refresh path is the
-- same: a name, a set of projects, a set of people. A 'tasks' board also carries
-- statuses; a 'billing' board leaves team_board_status empty.
--
-- There is deliberately no seen_/acknowledged/hidden side table here, unlike
-- my_task. Team boards never notify — they are a look-when-you-want view of
-- someone else's queue, and a task landing on a colleague's plate is not an
-- event worth interrupting the user for. Without alerting there is nothing to
-- acknowledge, so the whole first-seen machinery is absent on purpose.

-- +goose Up

CREATE TABLE team_board (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    kind TEXT    NOT NULL CHECK (kind IN ('tasks', 'billing')),
    name TEXT    NOT NULL,
    -- Order in the board strip. Ties break by id so the order is total even
    -- before anyone drags anything.
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT    NOT NULL,
    updated_at TEXT    NOT NULL
);

-- code and name are denormalised for the same reason as monitor_project: the
-- `project` table is a cache wiped and rebuilt on every task refresh, so joining
-- to it would make a saved board's labels depend on whether the last sync
-- happened to succeed.
CREATE TABLE team_board_project (
    board_id     INTEGER NOT NULL REFERENCES team_board (id) ON DELETE CASCADE,
    project_id   INTEGER NOT NULL,
    project_code TEXT    NOT NULL,
    project_name TEXT    NOT NULL,
    PRIMARY KEY (board_id, project_id)
);

-- emp_name denormalised likewise: members come from a dropdown endpoint that is
-- never cached, so a saved board must be able to label its own people offline.
CREATE TABLE team_board_member (
    board_id INTEGER NOT NULL REFERENCES team_board (id) ON DELETE CASCADE,
    emp_id   INTEGER NOT NULL,
    emp_name TEXT    NOT NULL,
    PRIMARY KEY (board_id, emp_id)
);

CREATE TABLE team_board_status (
    board_id    INTEGER NOT NULL REFERENCES team_board (id) ON DELETE CASCADE,
    status_id   INTEGER NOT NULL,
    status_name TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (board_id, status_id)
);

-- Cache, replaced wholesale per refresh like every other cache in the app.
--
-- The primary key includes board_id and emp_id, not just task_id: the same task
-- can legitimately appear on two boards, and Pinestem can return a task under a
-- member who is only informed on it.
--
-- Columns mirror my_task: Pinestem timestamps stored verbatim as
-- "YYYY-MM-DD HH:MM:SS" because that format sorts correctly as text, and an
-- absent due date is '' rather than NULL so "undated last" is a plain CASE.
CREATE TABLE team_board_task (
    board_id        INTEGER NOT NULL REFERENCES team_board (id) ON DELETE CASCADE,
    emp_id          INTEGER NOT NULL,
    task_id         INTEGER NOT NULL,
    short_code      TEXT    NOT NULL,
    name            TEXT    NOT NULL,
    project_code    TEXT    NOT NULL,
    project_name    TEXT    NOT NULL,
    priority        TEXT    NOT NULL DEFAULT '',
    status_id       INTEGER NOT NULL DEFAULT 0,
    status_type     TEXT    NOT NULL DEFAULT '',
    status_color    TEXT    NOT NULL DEFAULT '',
    due_date        TEXT    NOT NULL DEFAULT '',
    modified_on     TEXT    NOT NULL DEFAULT '',
    sprint_name     TEXT    NOT NULL DEFAULT '',
    competency_name TEXT    NOT NULL DEFAULT '',
    synced_at       TEXT    NOT NULL,
    PRIMARY KEY (board_id, emp_id, task_id)
);

CREATE INDEX idx_team_board_task_due_date ON team_board_task (board_id, due_date);

-- One row per member per day in the fetched window, replaced wholesale.
CREATE TABLE team_board_day (
    board_id             INTEGER NOT NULL REFERENCES team_board (id) ON DELETE CASCADE,
    emp_id               INTEGER NOT NULL,
    day                  TEXT    NOT NULL,
    billable_minutes     INTEGER NOT NULL DEFAULT 0,
    non_billable_minutes INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (board_id, emp_id, day)
);

-- One interval for every board rather than one each. Slower than any of the
-- others by default: other people's queues turn over far less often than your
-- own, and a task board costs one paginated request *per member*, so a short
-- interval here is much more expensive than it looks.
ALTER TABLE app_settings ADD COLUMN team_refresh_interval_seconds INTEGER NOT NULL DEFAULT 600;
ALTER TABLE app_settings ADD COLUMN team_synced_at TEXT;

-- +goose Down
ALTER TABLE app_settings DROP COLUMN team_synced_at;
ALTER TABLE app_settings DROP COLUMN team_refresh_interval_seconds;
DROP TABLE team_board_day;
DROP INDEX idx_team_board_task_due_date;
DROP TABLE team_board_task;
DROP TABLE team_board_status;
DROP TABLE team_board_member;
DROP TABLE team_board_project;
DROP TABLE team_board;
