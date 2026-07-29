-- name: ListTasks :many
SELECT * FROM task ORDER BY modified_on DESC;

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

-- name: GetSettings :one
SELECT * FROM app_settings WHERE id = 1;

-- name: UpsertSettings :exec
INSERT INTO app_settings (id, refresh_interval_seconds, tasks_synced_at)
VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    refresh_interval_seconds = excluded.refresh_interval_seconds,
    tasks_synced_at          = excluded.tasks_synced_at;

-- name: SetRefreshInterval :exec
INSERT INTO app_settings (id, refresh_interval_seconds)
VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET refresh_interval_seconds = excluded.refresh_interval_seconds;

-- name: SetTasksSyncedAt :exec
INSERT INTO app_settings (id, tasks_synced_at)
VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET tasks_synced_at = excluded.tasks_synced_at;
