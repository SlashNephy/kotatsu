-- +goose Up
-- modify "notification_events" table
ALTER TABLE "notification_events" DROP CONSTRAINT "notification_events_kind_check", ADD CONSTRAINT "notification_events_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text, 'drama_recorded'::text, 'drama_record_failed'::text, 'scrambled_recordings'::text, 'lineup_changed'::text]));
-- modify "notification_preferences" table
ALTER TABLE "notification_preferences" DROP CONSTRAINT "notification_preferences_kind_check", ADD CONSTRAINT "notification_preferences_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text, 'drama_recorded'::text, 'drama_record_failed'::text, 'scrambled_recordings'::text, 'lineup_changed'::text]));
-- create "drama_record_apps" table
CREATE TABLE "drama_record_apps" ("destination" text NOT NULL, "client_id" text NOT NULL, "client_secret_encrypted" bytea NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("destination"), CONSTRAINT "drama_record_apps_destination_check" CHECK (destination = 'simkl'::text));
-- create "drama_record_authorizations" table
CREATE TABLE "drama_record_authorizations" ("id" uuid NOT NULL, "state_hash" bytea NOT NULL, "session_id" uuid NOT NULL, "user_id" uuid NOT NULL, "destination" text NOT NULL, "code_verifier" text NOT NULL, "redirect_uri" text NOT NULL, "created_at" timestamptz NOT NULL, "expires_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "drama_record_authorizations_state_hash_key" UNIQUE ("state_hash"), CONSTRAINT "drama_record_authorizations_session_id_fkey" FOREIGN KEY ("session_id") REFERENCES "sessions" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "drama_record_authorizations_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "drama_record_authorizations_destination_check" CHECK (destination = 'simkl'::text));
-- create index "drama_record_authorizations_expires_at_idx" to table: "drama_record_authorizations"
CREATE INDEX "drama_record_authorizations_expires_at_idx" ON "drama_record_authorizations" ("expires_at");
-- create "drama_record_connections" table
CREATE TABLE "drama_record_connections" ("user_id" uuid NOT NULL, "destination" text NOT NULL, "access_token_encrypted" bytea NOT NULL, "refresh_token_encrypted" bytea NOT NULL, "access_token_expires_at" timestamptz NOT NULL, "account_name" text NOT NULL, "connected_at" timestamptz NOT NULL, "reauthorization_required" boolean NOT NULL DEFAULT false, "watchlist_activity" text NULL, "watchlist_removed_activity" text NULL, "watchlist_checked_at" timestamptz NULL, "watchlist_imported_at" timestamptz NULL, "watchlist_error" text NULL, "watchlist_retry_at" timestamptz NULL, PRIMARY KEY ("user_id", "destination"), CONSTRAINT "drama_record_connections_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "drama_record_connections_destination_check" CHECK (destination = 'simkl'::text));
-- create "drama_record_settings" table
CREATE TABLE "drama_record_settings" ("user_id" uuid NOT NULL, "threshold_percent" smallint NOT NULL DEFAULT 20, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("user_id"), CONSTRAINT "drama_record_settings_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "drama_record_settings_threshold_percent_check" CHECK ((threshold_percent >= 1) AND (threshold_percent <= 100)));
-- create "drama_records" table
CREATE TABLE "drama_records" ("id" uuid NOT NULL, "user_id" uuid NOT NULL, "service_id" bigint NOT NULL, "start_at" timestamptz NOT NULL, "destination" text NOT NULL, "tmdb_series_id" integer NOT NULL, "season_number" integer NOT NULL, "episode_number" integer NOT NULL, "program_name" text NOT NULL, "watched_at" timestamptz NOT NULL, "state" text NOT NULL, "url" text NULL, "error" text NULL, "created_at" timestamptz NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "drama_records_user_id_service_id_start_at_destination_tmdb__key" UNIQUE ("user_id", "service_id", "start_at", "destination", "tmdb_series_id", "season_number", "episode_number"), CONSTRAINT "drama_records_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "drama_records_destination_check" CHECK (destination = 'simkl'::text), CONSTRAINT "drama_records_episode_number_check" CHECK (episode_number > 0), CONSTRAINT "drama_records_season_number_check" CHECK (season_number >= 0), CONSTRAINT "drama_records_state_check" CHECK (state = ANY (ARRAY['pending'::text, 'recorded'::text, 'failed'::text])), CONSTRAINT "drama_records_tmdb_series_id_check" CHECK (tmdb_series_id > 0));
-- create "drama_watchlist_imports" table
CREATE TABLE "drama_watchlist_imports" ("user_id" uuid NOT NULL, "destination" text NOT NULL, "tmdb_series_id" integer NOT NULL, "simkl_id" bigint NULL, "status" text NOT NULL, "applied" boolean NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("user_id", "destination", "tmdb_series_id"), CONSTRAINT "drama_watchlist_imports_user_id_destination_fkey" FOREIGN KEY ("user_id", "destination") REFERENCES "drama_record_connections" ("user_id", "destination") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "drama_watchlist_imports_destination_check" CHECK (destination = 'simkl'::text), CONSTRAINT "drama_watchlist_imports_status_check" CHECK (status = ANY (ARRAY['watching'::text, 'plantowatch'::text, 'hold'::text, 'completed'::text, 'dropped'::text])), CONSTRAINT "drama_watchlist_imports_tmdb_series_id_check" CHECK (tmdb_series_id > 0));
-- create index "drama_watchlist_imports_pending_idx" to table: "drama_watchlist_imports"
CREATE INDEX "drama_watchlist_imports_pending_idx" ON "drama_watchlist_imports" ("user_id", "destination") WHERE (NOT applied);

-- +goose Down
-- reverse: create index "drama_watchlist_imports_pending_idx" to table: "drama_watchlist_imports"
DROP INDEX "drama_watchlist_imports_pending_idx";
-- reverse: create "drama_watchlist_imports" table
DROP TABLE "drama_watchlist_imports";
-- reverse: create "drama_records" table
DROP TABLE "drama_records";
-- reverse: create "drama_record_settings" table
DROP TABLE "drama_record_settings";
-- reverse: create "drama_record_connections" table
DROP TABLE "drama_record_connections";
-- reverse: create index "drama_record_authorizations_expires_at_idx" to table: "drama_record_authorizations"
DROP INDEX "drama_record_authorizations_expires_at_idx";
-- reverse: create "drama_record_authorizations" table
DROP TABLE "drama_record_authorizations";
-- reverse: create "drama_record_apps" table
DROP TABLE "drama_record_apps";
-- reverse: modify "notification_preferences" table
ALTER TABLE "notification_preferences" DROP CONSTRAINT "notification_preferences_kind_check", ADD CONSTRAINT "notification_preferences_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text, 'scrambled_recordings'::text, 'lineup_changed'::text]));
-- reverse: modify "notification_events" table
ALTER TABLE "notification_events" DROP CONSTRAINT "notification_events_kind_check", ADD CONSTRAINT "notification_events_kind_check" CHECK (kind = ANY (ARRAY['recording_started'::text, 'recording_completed'::text, 'recording_failed'::text, 'reservation_conflict'::text, 'storage_low'::text, 'tuner_server_unreachable'::text, 'watch_reminder'::text, 'anime_recorded'::text, 'anime_record_failed'::text, 'scrambled_recordings'::text, 'lineup_changed'::text]));
