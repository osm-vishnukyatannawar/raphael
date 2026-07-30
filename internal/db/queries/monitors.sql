-- name: ListMonitors :many
SELECT * FROM monitor ORDER BY name;

-- name: GetMonitor :one
SELECT * FROM monitor WHERE id = ?;

-- name: InsertMonitor :one
INSERT INTO monitor (name, created_at, updated_at) VALUES (?, ?, ?)
RETURNING id;

-- name: UpdateMonitor :exec
UPDATE monitor SET name = ?, updated_at = ? WHERE id = ?;

-- name: DeleteMonitor :exec
DELETE FROM monitor WHERE id = ?;

-- name: ListMonitorProjects :many
SELECT * FROM monitor_project ORDER BY monitor_id, project_code;

-- name: DeleteMonitorProjects :exec
DELETE FROM monitor_project WHERE monitor_id = ?;

-- name: InsertMonitorProject :exec
INSERT INTO monitor_project (monitor_id, project_id, project_code, project_name)
VALUES (?, ?, ?, ?);

-- name: ListMonitorTargets :many
SELECT * FROM monitor_target ORDER BY monitor_id, emp_name, project_id;

-- name: DeleteMonitorTargets :exec
DELETE FROM monitor_target WHERE monitor_id = ?;

-- name: InsertMonitorTarget :exec
INSERT INTO monitor_target (monitor_id, emp_id, emp_name, project_id, target_minutes)
VALUES (?, ?, ?, ?, ?);

-- name: ListMonitorActuals :many
SELECT * FROM monitor_actual;

-- name: DeleteAllMonitorActuals :exec
DELETE FROM monitor_actual;

-- name: InsertMonitorActual :exec
INSERT INTO monitor_actual (
    monitor_id, emp_id, project_id, billable_minutes, non_billable_minutes
) VALUES (?, ?, ?, ?, ?);

-- name: SetMonitorsSyncedAt :exec
INSERT INTO app_settings (id, monitors_synced_at)
VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET monitors_synced_at = excluded.monitors_synced_at;
