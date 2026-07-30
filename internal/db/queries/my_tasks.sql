-- name: ListMyTasks :many
-- Soonest due first, undated last, newest-modified breaking ties. Both flags are
-- COALESCEd to the safe default: an unknown task is "already seen" (never a
-- surprise highlight) and not hidden.
SELECT
    t.*,
    COALESCE(s.acknowledged, 1) AS acknowledged,
    CASE WHEN h.task_id IS NULL THEN 0 ELSE 1 END AS hidden
FROM my_task t
LEFT JOIN seen_my_task s ON s.task_id = t.task_id
LEFT JOIN hidden_my_task h ON h.task_id = t.task_id
ORDER BY
    CASE WHEN t.due_date = '' THEN 1 ELSE 0 END,
    t.due_date,
    t.modified_on DESC;

-- name: DeleteAllMyTasks :exec
DELETE FROM my_task;

-- name: InsertMyTask :exec
INSERT INTO my_task (
    task_id, short_code, name, project_code, project_name,
    priority, status_id, status_type, status_color,
    due_date, modified_on, sprint_name, competency_name, synced_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListSeenMyTasks :many
SELECT * FROM seen_my_task;

-- name: DeleteAllSeenMyTasks :exec
DELETE FROM seen_my_task;

-- name: InsertSeenMyTask :exec
INSERT INTO seen_my_task (task_id, first_seen_at, acknowledged) VALUES (?, ?, ?);

-- name: AcknowledgeMyTask :exec
UPDATE seen_my_task SET acknowledged = 1 WHERE task_id = ?;

-- name: AcknowledgeAllMyTasks :exec
UPDATE seen_my_task SET acknowledged = 1;

-- name: ListHiddenMyTasks :many
SELECT * FROM hidden_my_task ORDER BY hidden_at DESC;

-- name: HideMyTask :exec
-- Re-hiding an already hidden task refreshes its labels rather than failing.
INSERT INTO hidden_my_task (task_id, short_code, name, hidden_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(task_id) DO UPDATE SET
    short_code = excluded.short_code,
    name       = excluded.name,
    hidden_at  = excluded.hidden_at;

-- name: UnhideMyTask :exec
DELETE FROM hidden_my_task WHERE task_id = ?;

-- name: UnhideAllMyTasks :exec
DELETE FROM hidden_my_task;

-- name: ListMyTaskProjectFilter :many
SELECT * FROM my_task_project_filter ORDER BY project_code;

-- name: DeleteMyTaskProjectFilter :exec
DELETE FROM my_task_project_filter;

-- name: InsertMyTaskProjectFilter :exec
INSERT INTO my_task_project_filter (project_code, project_name) VALUES (?, ?);

-- name: ListMyTaskStatusFilter :many
SELECT * FROM my_task_status_filter ORDER BY status_name;

-- name: DeleteMyTaskStatusFilter :exec
DELETE FROM my_task_status_filter;

-- name: InsertMyTaskStatusFilter :exec
INSERT INTO my_task_status_filter (status_id, status_name) VALUES (?, ?);

-- name: SetMyTasksSyncedAt :exec
INSERT INTO app_settings (id, my_tasks_synced_at)
VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET my_tasks_synced_at = excluded.my_tasks_synced_at;
