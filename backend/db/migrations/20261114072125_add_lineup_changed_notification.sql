-- +goose Up
-- modify "notification_events" table
ALTER TABLE "notification_events" DROP CONSTRAINT "notification_events_kind_check", ADD CONSTRAINT "notification_events_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text, 'scrambled_recordings'::text, 'lineup_changed'::text]));
-- modify "notification_preferences" table
ALTER TABLE "notification_preferences" DROP CONSTRAINT "notification_preferences_kind_check", ADD CONSTRAINT "notification_preferences_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text, 'scrambled_recordings'::text, 'lineup_changed'::text]));

-- +goose Down
-- reverse: modify "notification_preferences" table
ALTER TABLE "notification_preferences" DROP CONSTRAINT "notification_preferences_kind_check", ADD CONSTRAINT "notification_preferences_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text, 'scrambled_recordings'::text]));
-- reverse: modify "notification_events" table
ALTER TABLE "notification_events" DROP CONSTRAINT "notification_events_kind_check", ADD CONSTRAINT "notification_events_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text, 'scrambled_recordings'::text]));
