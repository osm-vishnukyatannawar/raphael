-- Alerting when a task arrives in review.

-- +goose Up

-- seen_task is deliberately NOT part of the wholesale task-cache swap in
-- internal/tasks. A task row is deleted and reinserted on every refresh, so
-- storing acknowledgement on it would need carry-forward code; keeping it in a
-- side table means it just survives. Rows whose task_id is absent from a refresh
-- are pruned, so a task that leaves review and later returns alerts again.
CREATE TABLE seen_task (
    task_id       INTEGER PRIMARY KEY,
    first_seen_at TEXT    NOT NULL,
    acknowledged  INTEGER NOT NULL DEFAULT 0
);

-- Seed from whatever is already cached, marked as seen.
--
-- Without this, upgrading an install that already has a task cache makes every
-- open review look brand new on the next refresh: the tasks are in `task` but
-- absent from `seen_task`, which is exactly the "arrived since last time" test.
-- The result is an alert per task and the window yanked to the front on first
-- launch. On a fresh install `task` is empty and this does nothing.
INSERT INTO seen_task (task_id, first_seen_at, acknowledged)
SELECT task_id, synced_at, 1 FROM task;

ALTER TABLE app_settings ADD COLUMN notify_new_tasks INTEGER NOT NULL DEFAULT 1;
ALTER TABLE app_settings ADD COLUMN focus_on_new_task INTEGER NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE app_settings DROP COLUMN focus_on_new_task;
ALTER TABLE app_settings DROP COLUMN notify_new_tasks;
DROP TABLE seen_task;
