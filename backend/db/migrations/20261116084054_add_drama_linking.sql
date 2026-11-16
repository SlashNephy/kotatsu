-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "drama_linking_enabled" boolean NOT NULL DEFAULT false;
-- create "drama_match_cursor" table
CREATE TABLE "drama_match_cursor" ("id" smallint NOT NULL, "epg_version" bigint NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "drama_match_cursor_id_check" CHECK (id = 1));
-- create "drama_title_overrides" table
CREATE TABLE "drama_title_overrides" ("title_key" text NOT NULL, "tmdb_series_id" integer NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("title_key"), CONSTRAINT "drama_title_overrides_title_key_check" CHECK (title_key <> ''::text), CONSTRAINT "drama_title_overrides_tmdb_series_id_check" CHECK (tmdb_series_id > 0));
-- create "tmdb_searches" table
CREATE TABLE "tmdb_searches" ("query" text NOT NULL, "results" jsonb NOT NULL, "fetched_at" timestamptz NOT NULL, PRIMARY KEY ("query"));
-- create "tmdb_seasons" table
CREATE TABLE "tmdb_seasons" ("series_id" integer NOT NULL, "season_number" integer NOT NULL, "episodes" jsonb NOT NULL, "fetched_at" timestamptz NOT NULL, PRIMARY KEY ("series_id", "season_number"), CONSTRAINT "tmdb_seasons_season_number_check" CHECK (season_number >= 0));
-- create "tmdb_series" table
CREATE TABLE "tmdb_series" ("id" integer NOT NULL, "name" text NOT NULL, "original_name" text NOT NULL, "first_air_date" date NULL, "seasons" jsonb NOT NULL, "fetched_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "tmdb_series_id_check" CHECK (id > 0));
-- create "program_drama_matches" table
CREATE TABLE "program_drama_matches" ("service_id" bigint NOT NULL, "start_at" timestamptz NOT NULL, "event_id" bigint NOT NULL, "title_key" text NOT NULL, "tmdb_series_id" integer NULL, "season_number" integer NULL, "episode_number" integer NULL, "series_source" text NULL, "episode_source" text NULL, "matched_at" timestamptz NOT NULL, "finalized_at" timestamptz NULL, PRIMARY KEY ("service_id", "start_at"), CONSTRAINT "program_drama_matches_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "program_drama_matches_check" CHECK ((tmdb_series_id IS NULL) = (series_source IS NULL)), CONSTRAINT "program_drama_matches_check1" CHECK ((episode_number IS NULL) = (episode_source IS NULL)), CONSTRAINT "program_drama_matches_check2" CHECK ((episode_number IS NULL) OR ((tmdb_series_id IS NOT NULL) AND (season_number IS NOT NULL))), CONSTRAINT "program_drama_matches_episode_number_check" CHECK (episode_number > 0), CONSTRAINT "program_drama_matches_episode_source_check" CHECK (episode_source = ANY (ARRAY['air_date'::text, 'title'::text])), CONSTRAINT "program_drama_matches_season_number_check" CHECK (season_number >= 0), CONSTRAINT "program_drama_matches_series_source_check" CHECK (series_source = ANY (ARRAY['search'::text, 'override'::text])), CONSTRAINT "program_drama_matches_tmdb_series_id_check" CHECK (tmdb_series_id > 0));
-- create index "program_drama_matches_service_id_event_id_idx" to table: "program_drama_matches"
CREATE INDEX "program_drama_matches_service_id_event_id_idx" ON "program_drama_matches" ("service_id", "event_id");
-- create index "program_drama_matches_title_key_idx" to table: "program_drama_matches"
CREATE INDEX "program_drama_matches_title_key_idx" ON "program_drama_matches" ("title_key");
-- create index "program_drama_matches_tmdb_series_id_idx" to table: "program_drama_matches"
CREATE INDEX "program_drama_matches_tmdb_series_id_idx" ON "program_drama_matches" ("tmdb_series_id") WHERE (tmdb_series_id IS NOT NULL);
-- create "recording_drama_matches" table
CREATE TABLE "recording_drama_matches" ("recording_id" uuid NOT NULL, "tmdb_series_id" integer NULL, "season_number" integer NULL, "episode_number" integer NULL, "series_source" text NULL, "episode_source" text NULL, "matched_at" timestamptz NOT NULL, PRIMARY KEY ("recording_id"), CONSTRAINT "recording_drama_matches_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recordings" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "recording_drama_matches_check" CHECK ((tmdb_series_id IS NULL) = (series_source IS NULL)), CONSTRAINT "recording_drama_matches_check1" CHECK ((episode_number IS NULL) = (episode_source IS NULL)), CONSTRAINT "recording_drama_matches_check2" CHECK ((episode_number IS NULL) OR ((tmdb_series_id IS NOT NULL) AND (season_number IS NOT NULL))), CONSTRAINT "recording_drama_matches_episode_number_check" CHECK (episode_number > 0), CONSTRAINT "recording_drama_matches_episode_source_check" CHECK (episode_source = ANY (ARRAY['air_date'::text, 'title'::text])), CONSTRAINT "recording_drama_matches_season_number_check" CHECK (season_number >= 0), CONSTRAINT "recording_drama_matches_series_source_check" CHECK (series_source = ANY (ARRAY['search'::text, 'override'::text])), CONSTRAINT "recording_drama_matches_tmdb_series_id_check" CHECK (tmdb_series_id > 0));
-- create index "recording_drama_matches_tmdb_series_id_idx" to table: "recording_drama_matches"
CREATE INDEX "recording_drama_matches_tmdb_series_id_idx" ON "recording_drama_matches" ("tmdb_series_id") WHERE (tmdb_series_id IS NOT NULL);

-- +goose Down
-- reverse: create index "recording_drama_matches_tmdb_series_id_idx" to table: "recording_drama_matches"
DROP INDEX "recording_drama_matches_tmdb_series_id_idx";
-- reverse: create "recording_drama_matches" table
DROP TABLE "recording_drama_matches";
-- reverse: create index "program_drama_matches_tmdb_series_id_idx" to table: "program_drama_matches"
DROP INDEX "program_drama_matches_tmdb_series_id_idx";
-- reverse: create index "program_drama_matches_title_key_idx" to table: "program_drama_matches"
DROP INDEX "program_drama_matches_title_key_idx";
-- reverse: create index "program_drama_matches_service_id_event_id_idx" to table: "program_drama_matches"
DROP INDEX "program_drama_matches_service_id_event_id_idx";
-- reverse: create "program_drama_matches" table
DROP TABLE "program_drama_matches";
-- reverse: create "tmdb_series" table
DROP TABLE "tmdb_series";
-- reverse: create "tmdb_seasons" table
DROP TABLE "tmdb_seasons";
-- reverse: create "tmdb_searches" table
DROP TABLE "tmdb_searches";
-- reverse: create "drama_title_overrides" table
DROP TABLE "drama_title_overrides";
-- reverse: create "drama_match_cursor" table
DROP TABLE "drama_match_cursor";
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "drama_linking_enabled";
