-- +goose Up
-- modify "notification_events" table
ALTER TABLE "notification_events" DROP CONSTRAINT "notification_events_kind_check", ADD CONSTRAINT "notification_events_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text]));
-- modify "notification_preferences" table
ALTER TABLE "notification_preferences" DROP CONSTRAINT "notification_preferences_kind_check", ADD CONSTRAINT "notification_preferences_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text]));
-- modify "user_settings" table
ALTER TABLE "user_settings" ADD CONSTRAINT "user_settings_watch_reminder_minutes_check" CHECK ((watch_reminder_minutes >= 0) AND (watch_reminder_minutes <= 60)), ADD COLUMN "watch_reminder_minutes" integer NOT NULL DEFAULT 5;
-- create "watch_reservations" table
CREATE TABLE "watch_reservations" ("user_id" uuid NOT NULL, "service_id" bigint NOT NULL, "event_id" bigint NOT NULL, "created_at" timestamptz NOT NULL, PRIMARY KEY ("user_id", "service_id", "event_id"), CONSTRAINT "watch_reservations_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "watch_reservations_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "watch_reservations_service_id_event_id_idx" to table: "watch_reservations"
CREATE INDEX "watch_reservations_service_id_event_id_idx" ON "watch_reservations" ("service_id", "event_id");

-- +goose Down
-- reverse: create index "watch_reservations_service_id_event_id_idx" to table: "watch_reservations"
DROP INDEX "watch_reservations_service_id_event_id_idx";
-- reverse: create "watch_reservations" table
DROP TABLE "watch_reservations";
-- reverse: modify "user_settings" table
ALTER TABLE "user_settings" DROP CONSTRAINT "user_settings_watch_reminder_minutes_check", DROP COLUMN "watch_reminder_minutes";
-- reverse: modify "notification_preferences" table
ALTER TABLE "notification_preferences" DROP CONSTRAINT "notification_preferences_kind_check", ADD CONSTRAINT "notification_preferences_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text]));
-- reverse: modify "notification_events" table
ALTER TABLE "notification_events" DROP CONSTRAINT "notification_events_kind_check", ADD CONSTRAINT "notification_events_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text]));
