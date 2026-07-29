-- Baseline migration.
--
-- There is no schema yet. This exists because goose fails with "no migration
-- files found" against an empty directory, and because it gives the chain a
-- version 1 for later migrations to build on. Real tables land in 00002_*.sql.

-- +goose Up
SELECT 1;

-- +goose Down
SELECT 1;
