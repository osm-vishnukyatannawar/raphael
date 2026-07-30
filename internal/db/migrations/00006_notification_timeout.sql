-- How long a new-task notification stays on screen.

-- +goose Up
-- Seconds. 0 means "until dismissed", mirroring the freedesktop Notify
-- expire_timeout convention this maps onto. Note that 0 is *not* "off" here,
-- unlike the refresh intervals — turning notifications off is notify_new_tasks.
ALTER TABLE app_settings ADD COLUMN notification_timeout_seconds INTEGER NOT NULL DEFAULT 10;

-- +goose Down
ALTER TABLE app_settings DROP COLUMN notification_timeout_seconds;
