-- +goose Up
-- modify "notification_events" table
ALTER TABLE "notification_events" DROP CONSTRAINT "notification_events_kind_check", ADD CONSTRAINT "notification_events_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text]));
-- modify "notification_preferences" table
ALTER TABLE "notification_preferences" DROP CONSTRAINT "notification_preferences_kind_check", ADD CONSTRAINT "notification_preferences_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text]));
-- create "anime_record_apps" table
CREATE TABLE "anime_record_apps" ("destination" text NOT NULL, "client_id" text NOT NULL, "client_secret_encrypted" bytea NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("destination"), CONSTRAINT "anime_record_apps_destination_check" CHECK (destination = ANY (ARRAY['annict'::text, 'anilist'::text])));
-- create "anime_record_authorizations" table
CREATE TABLE "anime_record_authorizations" ("id" uuid NOT NULL, "state_hash" bytea NOT NULL, "session_id" uuid NOT NULL, "user_id" uuid NOT NULL, "destination" text NOT NULL, "code_verifier" text NOT NULL, "redirect_uri" text NOT NULL, "created_at" timestamptz NOT NULL, "expires_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "anime_record_authorizations_state_hash_key" UNIQUE ("state_hash"), CONSTRAINT "anime_record_authorizations_session_id_fkey" FOREIGN KEY ("session_id") REFERENCES "sessions" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "anime_record_authorizations_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "anime_record_authorizations_destination_check" CHECK (destination = ANY (ARRAY['annict'::text, 'anilist'::text])));
-- create index "anime_record_authorizations_expires_at_idx" to table: "anime_record_authorizations"
CREATE INDEX "anime_record_authorizations_expires_at_idx" ON "anime_record_authorizations" ("expires_at");
-- create "anime_record_connections" table
CREATE TABLE "anime_record_connections" ("user_id" uuid NOT NULL, "destination" text NOT NULL, "access_token_encrypted" bytea NOT NULL, "account_name" text NOT NULL, "connected_at" timestamptz NOT NULL, PRIMARY KEY ("user_id", "destination"), CONSTRAINT "anime_record_connections_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "anime_record_connections_destination_check" CHECK (destination = ANY (ARRAY['annict'::text, 'anilist'::text])));
-- create "anime_record_settings" table
CREATE TABLE "anime_record_settings" ("user_id" uuid NOT NULL, "threshold_percent" smallint NOT NULL DEFAULT 20, "watching_on_first" boolean NOT NULL DEFAULT false, "watching_on_episode" boolean NOT NULL DEFAULT false, "watched_on_last" boolean NOT NULL DEFAULT false, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("user_id"), CONSTRAINT "anime_record_settings_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "anime_record_settings_threshold_percent_check" CHECK ((threshold_percent >= 1) AND (threshold_percent <= 100)));
-- create "anime_records" table
CREATE TABLE "anime_records" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "service_id" bigint NOT NULL, "start_at" timestamptz NOT NULL, "destination" text NOT NULL, "syobocal_tid" integer NOT NULL, "episode_number" double precision NULL, "subtitle" text NULL, "is_first" boolean NOT NULL, "is_last" boolean NOT NULL, "program_name" text NOT NULL, "state" text NOT NULL, "url" text NULL, "error" text NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "anime_records_user_id_service_id_start_at_destination_syobo_key" UNIQUE NULLS NOT DISTINCT ("user_id", "service_id", "start_at", "destination", "syobocal_tid", "episode_number"), CONSTRAINT "anime_records_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "anime_records_destination_check" CHECK (destination = ANY (ARRAY['annict'::text, 'anilist'::text])), CONSTRAINT "anime_records_state_check" CHECK (state = ANY (ARRAY['pending'::text, 'recorded'::text, 'failed'::text])));
-- create "anime_watch_progress" table
CREATE TABLE "anime_watch_progress" ("user_id" uuid NOT NULL, "service_id" bigint NOT NULL, "start_at" timestamptz NOT NULL, "played" jsonb NOT NULL, "played_ms" bigint NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("user_id", "service_id", "start_at"), CONSTRAINT "anime_watch_progress_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "anime_watch_progress_updated_at_idx" to table: "anime_watch_progress"
CREATE INDEX "anime_watch_progress_updated_at_idx" ON "anime_watch_progress" ("updated_at");

-- +goose Down
-- reverse: create index "anime_watch_progress_updated_at_idx" to table: "anime_watch_progress"
DROP INDEX "anime_watch_progress_updated_at_idx";
-- reverse: create "anime_watch_progress" table
DROP TABLE "anime_watch_progress";
-- reverse: create "anime_records" table
DROP TABLE "anime_records";
-- reverse: create "anime_record_settings" table
DROP TABLE "anime_record_settings";
-- reverse: create "anime_record_connections" table
DROP TABLE "anime_record_connections";
-- reverse: create index "anime_record_authorizations_expires_at_idx" to table: "anime_record_authorizations"
DROP INDEX "anime_record_authorizations_expires_at_idx";
-- reverse: create "anime_record_authorizations" table
DROP TABLE "anime_record_authorizations";
-- reverse: create "anime_record_apps" table
DROP TABLE "anime_record_apps";
-- reverse: modify "notification_preferences" table
ALTER TABLE "notification_preferences" DROP CONSTRAINT "notification_preferences_kind_check", ADD CONSTRAINT "notification_preferences_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text]));
-- reverse: modify "notification_events" table
ALTER TABLE "notification_events" DROP CONSTRAINT "notification_events_kind_check", ADD CONSTRAINT "notification_events_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text]));
