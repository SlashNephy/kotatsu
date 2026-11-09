-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD CONSTRAINT "system_settings_watch_progress_retention_days_check" CHECK (watch_progress_retention_days >= 0), ADD COLUMN "watch_progress_retention_days" integer NOT NULL DEFAULT 365;
-- create index "watch_progress_updated_at_idx" to table: "watch_progress"
CREATE INDEX "watch_progress_updated_at_idx" ON "watch_progress" ("updated_at");

-- +goose Down
-- reverse: create index "watch_progress_updated_at_idx" to table: "watch_progress"
DROP INDEX "watch_progress_updated_at_idx";
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "watch_progress_retention_days", DROP CONSTRAINT "system_settings_watch_progress_retention_days_check";
