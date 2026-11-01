-- +goose Up
-- create "anime_match_cursor" table
CREATE TABLE "anime_match_cursor" ("id" smallint NOT NULL, "epg_version" bigint NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "anime_match_cursor_id_check" CHECK (id = 1));
-- create "syobocal_lookups" table
CREATE TABLE "syobocal_lookups" ("channel_id" integer NOT NULL, "day_start" timestamptz NOT NULL, "fetched_at" timestamptz NOT NULL, PRIMARY KEY ("channel_id", "day_start"));
-- create "syobocal_programs" table
CREATE TABLE "syobocal_programs" ("pid" integer NOT NULL, "tid" integer NOT NULL, "channel_id" integer NOT NULL, "start_at" timestamptz NOT NULL, "end_at" timestamptz NOT NULL, "offset_seconds" integer NOT NULL, "count" text NOT NULL, "sub_title" text NOT NULL, "st_sub_title" text NOT NULL, "flag" integer NOT NULL, "deleted" boolean NOT NULL, "fetched_at" timestamptz NOT NULL, PRIMARY KEY ("pid"));
-- create index "syobocal_programs_channel_id_start_at_idx" to table: "syobocal_programs"
CREATE INDEX "syobocal_programs_channel_id_start_at_idx" ON "syobocal_programs" ("channel_id", "start_at");
-- create index "syobocal_programs_end_at_idx" to table: "syobocal_programs"
CREATE INDEX "syobocal_programs_end_at_idx" ON "syobocal_programs" ("end_at");
-- create "program_anime_matches" table
CREATE TABLE "program_anime_matches" ("service_id" bigint NOT NULL, "start_at" timestamptz NOT NULL, "event_id" bigint NOT NULL, "matched_at" timestamptz NOT NULL, "finalized_at" timestamptz NULL, PRIMARY KEY ("service_id", "start_at"), CONSTRAINT "program_anime_matches_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create index "program_anime_matches_service_id_event_id_idx" to table: "program_anime_matches"
CREATE INDEX "program_anime_matches_service_id_event_id_idx" ON "program_anime_matches" ("service_id", "event_id");
-- create "program_anime_episodes" table
CREATE TABLE "program_anime_episodes" ("service_id" bigint NOT NULL, "start_at" timestamptz NOT NULL, "position" smallint NOT NULL, "source" text NOT NULL, "syobocal_tid" integer NULL, "syobocal_pid" integer NULL, "count_start" double precision NULL, "count_end" double precision NULL, "subtitle" text NULL, "is_first" boolean NOT NULL, "is_last" boolean NOT NULL, PRIMARY KEY ("service_id", "start_at", "position"), CONSTRAINT "program_anime_episodes_service_id_start_at_fkey" FOREIGN KEY ("service_id", "start_at") REFERENCES "program_anime_matches" ("service_id", "start_at") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "program_anime_episodes_source_check" CHECK (source = 'syobocal'::text));

-- +goose Down
-- reverse: create "program_anime_episodes" table
DROP TABLE "program_anime_episodes";
-- reverse: create index "program_anime_matches_service_id_event_id_idx" to table: "program_anime_matches"
DROP INDEX "program_anime_matches_service_id_event_id_idx";
-- reverse: create "program_anime_matches" table
DROP TABLE "program_anime_matches";
-- reverse: create index "syobocal_programs_end_at_idx" to table: "syobocal_programs"
DROP INDEX "syobocal_programs_end_at_idx";
-- reverse: create index "syobocal_programs_channel_id_start_at_idx" to table: "syobocal_programs"
DROP INDEX "syobocal_programs_channel_id_start_at_idx";
-- reverse: create "syobocal_programs" table
DROP TABLE "syobocal_programs";
-- reverse: create "syobocal_lookups" table
DROP TABLE "syobocal_lookups";
-- reverse: create "anime_match_cursor" table
DROP TABLE "anime_match_cursor";
