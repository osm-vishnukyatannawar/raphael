-- Identity: who the user is locally, and the Pinestem session we act on their behalf with.
--
-- Both tables are single-row (CHECK id = 1). Raphael is a per-user desktop app;
-- there is exactly one profile and one Pinestem session per install.
--
-- No password column exists here by design. Secrets live in the OS keyring
-- (see internal/secret). token_fallback is the one exception — populated only
-- when no keyring is available, so the app still works headless.

-- +goose Up
CREATE TABLE app_profile (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    display_name TEXT    NOT NULL,
    created_at   TEXT    NOT NULL,
    updated_at   TEXT    NOT NULL
);

CREATE TABLE pinestem_account (
    id                 INTEGER PRIMARY KEY CHECK (id = 1),
    user_id            INTEGER NOT NULL,
    user_name          TEXT    NOT NULL,
    first_name         TEXT    NOT NULL,
    last_name          TEXT    NOT NULL,
    -- Arrives as a string ("453") in the auth payload but is an integer in every
    -- other endpoint; parsed once on the way in.
    company_id         INTEGER NOT NULL,
    company_name       TEXT    NOT NULL,
    role_id            INTEGER NOT NULL DEFAULT 0,
    is_project_manager INTEGER NOT NULL DEFAULT 0,
    is_team_lead       INTEGER NOT NULL DEFAULT 0,
    account_type       TEXT    NOT NULL DEFAULT '',
    time_zone          TEXT    NOT NULL DEFAULT '',
    date_time_format   TEXT    NOT NULL DEFAULT '',
    authenticated_at   TEXT    NOT NULL,
    -- NULL whenever the keyring is working. Non-NULL means we degraded.
    token_fallback     TEXT
);

-- +goose Down
DROP TABLE pinestem_account;
DROP TABLE app_profile;
