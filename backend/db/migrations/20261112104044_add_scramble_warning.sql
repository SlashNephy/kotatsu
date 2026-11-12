-- +goose Up
-- modify "notification_events" table
ALTER TABLE "notification_events" DROP CONSTRAINT "notification_events_kind_check", ADD CONSTRAINT "notification_events_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text, 'scrambled_recordings'::text]));
-- modify "notification_preferences" table
ALTER TABLE "notification_preferences" DROP CONSTRAINT "notification_preferences_kind_check", ADD CONSTRAINT "notification_preferences_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text, 'scrambled_recordings'::text]));
-- modify "recordings" table
ALTER TABLE "recordings" ADD COLUMN "scrambled_ratio" double precision NULL, ADD COLUMN "scramble_warning" boolean NOT NULL DEFAULT false;
-- create "scramble_alerts" table
CREATE TABLE "scramble_alerts" ("id" boolean NOT NULL DEFAULT true, "consecutive" bigint NOT NULL, "notified_at" timestamptz NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "scramble_alerts_id_check" CHECK (id));

-- +goose Down
-- reverse: create "scramble_alerts" table
DROP TABLE "scramble_alerts";
-- reverse: modify "recordings" table
ALTER TABLE "recordings" DROP COLUMN "scramble_warning", DROP COLUMN "scrambled_ratio";
-- reverse: modify "notification_preferences" table
ALTER TABLE "notification_preferences" DROP CONSTRAINT "notification_preferences_kind_check", ADD CONSTRAINT "notification_preferences_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text]));
-- reverse: modify "notification_events" table
ALTER TABLE "notification_events" DROP CONSTRAINT "notification_events_kind_check", ADD CONSTRAINT "notification_events_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text]));
