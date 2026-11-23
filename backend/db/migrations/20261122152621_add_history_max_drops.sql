-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD CONSTRAINT "system_settings_history_max_drops_check" CHECK ((history_max_drops >= 0) AND (history_max_drops <= 1000)), ADD COLUMN "history_max_drops" integer NOT NULL DEFAULT 0;

-- +goose Down
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "history_max_drops", DROP CONSTRAINT "system_settings_history_max_drops_check";
