-- name: ListTeamBoards :many
SELECT * FROM team_board ORDER BY position, id;

-- name: GetTeamBoard :one
SELECT * FROM team_board WHERE id = ?;

-- name: InsertTeamBoard :one
INSERT INTO team_board (kind, name, position, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
RETURNING id;

-- name: UpdateTeamBoard :exec
-- kind is not updatable: switching a board's kind would leave the wrong cache
-- table populated and the wrong filters saved. Delete and recreate instead.
UPDATE team_board SET name = ?, position = ?, updated_at = ? WHERE id = ?;

-- name: SetTeamBoardPosition :exec
UPDATE team_board SET position = ?, updated_at = ? WHERE id = ?;

-- name: DeleteTeamBoard :exec
DELETE FROM team_board WHERE id = ?;

-- name: ListTeamBoardProjects :many
SELECT * FROM team_board_project ORDER BY board_id, project_code;

-- name: DeleteTeamBoardProjects :exec
DELETE FROM team_board_project WHERE board_id = ?;

-- name: InsertTeamBoardProject :exec
INSERT INTO team_board_project (board_id, project_id, project_code, project_name)
VALUES (?, ?, ?, ?);

-- name: ListTeamBoardMembers :many
SELECT * FROM team_board_member ORDER BY board_id, emp_name, emp_id;

-- name: DeleteTeamBoardMembers :exec
DELETE FROM team_board_member WHERE board_id = ?;

-- name: InsertTeamBoardMember :exec
INSERT INTO team_board_member (board_id, emp_id, emp_name) VALUES (?, ?, ?);

-- name: ListTeamBoardStatuses :many
SELECT * FROM team_board_status ORDER BY board_id, status_name, status_id;

-- name: DeleteTeamBoardStatuses :exec
DELETE FROM team_board_status WHERE board_id = ?;

-- name: InsertTeamBoardStatus :exec
INSERT INTO team_board_status (board_id, status_id, status_name) VALUES (?, ?, ?);

-- name: ListTeamBoardTasks :many
-- Soonest due first, undated last, newest-modified breaking ties: the same
-- order as ListMyTasks, so a board reads like the my-tasks list does.
--
-- Keep this file pure ASCII. sqlc's star expansion computes the offset of the
-- `*` in bytes but slices the query in runes, so a single non-ASCII character
-- anywhere above shifts the splice and emits nonsense like "SELECboard_id".
SELECT * FROM team_board_task
ORDER BY
    board_id,
    emp_id,
    CASE WHEN due_date = '' THEN 1 ELSE 0 END,
    due_date,
    modified_on DESC;

-- name: DeleteTeamBoardTasks :exec
DELETE FROM team_board_task WHERE board_id = ?;

-- name: InsertTeamBoardTask :exec
INSERT INTO team_board_task (
    board_id, emp_id, task_id, short_code, name, project_code, project_name,
    priority, status_id, status_type, status_color,
    due_date, modified_on, sprint_name, competency_name, synced_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListTeamBoardDays :many
SELECT * FROM team_board_day ORDER BY board_id, emp_id, day;

-- name: DeleteTeamBoardDays :exec
DELETE FROM team_board_day WHERE board_id = ?;

-- name: InsertTeamBoardDay :exec
INSERT INTO team_board_day (
    board_id, emp_id, day, billable_minutes, non_billable_minutes
) VALUES (?, ?, ?, ?, ?);

-- name: SetTeamSyncedAt :exec
INSERT INTO app_settings (id, team_synced_at)
VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET team_synced_at = excluded.team_synced_at;
