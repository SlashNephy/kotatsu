-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD CONSTRAINT "system_settings_notification_retention_days_check" CHECK (notification_retention_days >= 0), ADD COLUMN "notification_retention_days" integer NOT NULL DEFAULT 90;

-- +goose Down
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "notification_retention_days", DROP CONSTRAINT "system_settings_notification_retention_days_check";
