-- Cached billing hours.

-- +goose Up

-- One row per calendar day, in local time. Replaced wholesale per refresh like
-- the task cache. Pinestem reports minutes, and they are stored as minutes:
-- converting to hours is a display concern, and 0.1h is not exact in binary
-- floating point.
CREATE TABLE billing_day (
    day                  TEXT    PRIMARY KEY,
    billable_minutes     INTEGER NOT NULL DEFAULT 0,
    non_billable_minutes INTEGER NOT NULL DEFAULT 0
);

-- Billing gets its own cadence: hours you logged yourself change far less often
-- than a review queue other people push into.
ALTER TABLE app_settings ADD COLUMN billing_refresh_interval_seconds INTEGER NOT NULL DEFAULT 300;
-- 0=Sunday … 6=Saturday, matching Go's time.Weekday. Default 1 (Monday, ISO-8601).
ALTER TABLE app_settings ADD COLUMN week_start_day INTEGER NOT NULL DEFAULT 1;
ALTER TABLE app_settings ADD COLUMN billing_synced_at TEXT;

-- +goose Down
ALTER TABLE app_settings DROP COLUMN billing_synced_at;
ALTER TABLE app_settings DROP COLUMN week_start_day;
ALTER TABLE app_settings DROP COLUMN billing_refresh_interval_seconds;
DROP TABLE billing_day;
