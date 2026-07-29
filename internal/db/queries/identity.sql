-- name: GetProfile :one
SELECT * FROM app_profile WHERE id = 1;

-- name: UpsertProfile :exec
INSERT INTO app_profile (id, display_name, created_at, updated_at)
VALUES (1, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    display_name = excluded.display_name,
    updated_at   = excluded.updated_at;

-- name: DeleteProfile :exec
DELETE FROM app_profile WHERE id = 1;

-- name: GetPinestemAccount :one
SELECT * FROM pinestem_account WHERE id = 1;

-- name: UpsertPinestemAccount :exec
INSERT INTO pinestem_account (
    id, user_id, user_name, first_name, last_name,
    company_id, company_name, role_id,
    is_project_manager, is_team_lead,
    account_type, time_zone, date_time_format,
    authenticated_at, token_fallback
) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    user_id            = excluded.user_id,
    user_name          = excluded.user_name,
    first_name         = excluded.first_name,
    last_name          = excluded.last_name,
    company_id         = excluded.company_id,
    company_name       = excluded.company_name,
    role_id            = excluded.role_id,
    is_project_manager = excluded.is_project_manager,
    is_team_lead       = excluded.is_team_lead,
    account_type       = excluded.account_type,
    time_zone          = excluded.time_zone,
    date_time_format   = excluded.date_time_format,
    authenticated_at   = excluded.authenticated_at,
    token_fallback     = excluded.token_fallback;

-- name: DeletePinestemAccount :exec
DELETE FROM pinestem_account WHERE id = 1;
