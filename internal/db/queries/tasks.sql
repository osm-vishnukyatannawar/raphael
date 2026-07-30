-- name: ListTasks :many
-- COALESCE defaults to 1 ("already seen"): a task row without a seen_task row
-- must never render as a new-task highlight.
SELECT t.*, COALESCE(s.acknowledged, 1) AS acknowledged
FROM task t
LEFT JOIN seen_task s ON s.task_id = t.task_id
ORDER BY t.modified_on DESC;

-- name: DeleteAllTasks :exec
DELETE FROM task;

-- name: InsertTask :exec
INSERT INTO task (
    task_id, short_code, name, project_code, project_name,
    priority, status_type, status_color,
    due_date, modified_on, sprint_name, competency_name, synced_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: DeleteAllProjects :exec
DELETE FROM project;

-- name: InsertProject :exec
INSERT INTO project (project_id, code, name, status_id)
VALUES (?, ?, ?, ?);

-- name: ListProjects :many
SELECT * FROM project ORDER BY code;

-- name: ListSeenTasks :many
SELECT * FROM seen_task;

-- name: DeleteAllSeenTasks :exec
DELETE FROM seen_task;

-- name: InsertSeenTask :exec
INSERT INTO seen_task (task_id, first_seen_at, acknowledged) VALUES (?, ?, ?);

-- name: AcknowledgeTask :exec
UPDATE seen_task SET acknowledged = 1 WHERE task_id = ?;

-- name: AcknowledgeAllTasks :exec
UPDATE seen_task SET acknowledged = 1;

-- name: GetSettings :one
SELECT * FROM app_settings WHERE id = 1;

-- name: SaveSettings :exec
-- Only the user-editable fields. The sync stamps have their own setters so a
-- settings save never rewinds them.
INSERT INTO app_settings (
    id, refresh_interval_seconds, billing_refresh_interval_seconds,
    my_tasks_refresh_interval_seconds, week_start_day,
    notify_new_tasks, focus_on_new_task, notify_new_my_tasks,
    notification_timeout_seconds
) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    refresh_interval_seconds          = excluded.refresh_interval_seconds,
    billing_refresh_interval_seconds  = excluded.billing_refresh_interval_seconds,
    my_tasks_refresh_interval_seconds = excluded.my_tasks_refresh_interval_seconds,
    week_start_day                    = excluded.week_start_day,
    notify_new_tasks                  = excluded.notify_new_tasks,
    focus_on_new_task                 = excluded.focus_on_new_task,
    notify_new_my_tasks               = excluded.notify_new_my_tasks,
    notification_timeout_seconds      = excluded.notification_timeout_seconds;

-- name: SetTasksSyncedAt :exec
INSERT INTO app_settings (id, tasks_synced_at)
VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET tasks_synced_at = excluded.tasks_synced_at;

-- name: SetBillingSyncedAt :exec
INSERT INTO app_settings (id, billing_synced_at)
VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET billing_synced_at = excluded.billing_synced_at;

-- name: ListBillingDays :many
SELECT * FROM billing_day ORDER BY day;

-- name: DeleteAllBillingDays :exec
DELETE FROM billing_day;

-- name: InsertBillingDay :exec
INSERT INTO billing_day (day, billable_minutes, non_billable_minutes)
VALUES (?, ?, ?);
