-- +goose Up
-- create index "notification_events_dispatched_idx" to table: "notification_events"
CREATE INDEX "notification_events_dispatched_idx" ON "notification_events" ("occurred_at") WHERE (dispatched_at IS NOT NULL);
-- create index "notifications_event_id_idx" to table: "notifications"
CREATE INDEX "notifications_event_id_idx" ON "notifications" ("event_id");
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD CONSTRAINT "system_settings_notification_retention_days_check" CHECK (notification_retention_days >= 0), ADD COLUMN "notification_retention_days" integer NOT NULL DEFAULT 90;

-- +goose Down
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP CONSTRAINT "system_settings_notification_retention_days_check", DROP COLUMN "notification_retention_days";
-- reverse: create index "notifications_event_id_idx" to table: "notifications"
DROP INDEX "notifications_event_id_idx";
-- reverse: create index "notification_events_dispatched_idx" to table: "notification_events"
DROP INDEX "notification_events_dispatched_idx";
