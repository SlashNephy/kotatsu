-- +goose Up
-- modify "anime_imports" table
ALTER TABLE "anime_imports" DROP CONSTRAINT "anime_imports_source_check", ADD CONSTRAINT "anime_imports_source_check" CHECK (source = ANY (ARRAY['saya_definitions'::text, 'arm'::text, 'arm_supplementary'::text, 'syobocal_titles'::text]));
-- modify "program_anime_episodes" table
ALTER TABLE "program_anime_episodes" DROP CONSTRAINT "program_anime_episodes_source_check", ADD CONSTRAINT "program_anime_episodes_source_check" CHECK (source = ANY (ARRAY['syobocal'::text, 'title'::text]));
-- create "syobocal_titles" table
CREATE TABLE "syobocal_titles" ("tid" integer NOT NULL, "title" text NOT NULL, "title_key" text NOT NULL, "last_update" timestamptz NULL, PRIMARY KEY ("tid"));
-- create index "syobocal_titles_title_key_idx" to table: "syobocal_titles"
CREATE INDEX "syobocal_titles_title_key_idx" ON "syobocal_titles" ("title_key");
-- create "recording_anime_matches" table
CREATE TABLE "recording_anime_matches" ("recording_id" uuid NOT NULL, "matched_at" timestamptz NOT NULL, PRIMARY KEY ("recording_id"), CONSTRAINT "recording_anime_matches_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recordings" ("id") ON UPDATE NO ACTION ON DELETE CASCADE);
-- create "recording_anime_episodes" table
CREATE TABLE "recording_anime_episodes" ("recording_id" uuid NOT NULL, "position" smallint NOT NULL, "source" text NOT NULL, "syobocal_tid" integer NULL, "syobocal_pid" integer NULL, "count_start" double precision NULL, "count_end" double precision NULL, "subtitle" text NULL, "is_first" boolean NOT NULL, "is_last" boolean NOT NULL, PRIMARY KEY ("recording_id", "position"), CONSTRAINT "recording_anime_episodes_recording_id_fkey" FOREIGN KEY ("recording_id") REFERENCES "recording_anime_matches" ("recording_id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "recording_anime_episodes_source_check" CHECK (source = ANY (ARRAY['syobocal'::text, 'title'::text])));

-- +goose Down
-- reverse: create "recording_anime_episodes" table
DROP TABLE "recording_anime_episodes";
-- reverse: create "recording_anime_matches" table
DROP TABLE "recording_anime_matches";
-- reverse: create index "syobocal_titles_title_key_idx" to table: "syobocal_titles"
DROP INDEX "syobocal_titles_title_key_idx";
-- reverse: create "syobocal_titles" table
DROP TABLE "syobocal_titles";
-- reverse: modify "program_anime_episodes" table
ALTER TABLE "program_anime_episodes" DROP CONSTRAINT "program_anime_episodes_source_check", ADD CONSTRAINT "program_anime_episodes_source_check" CHECK (source = 'syobocal'::text);
-- reverse: modify "anime_imports" table
ALTER TABLE "anime_imports" DROP CONSTRAINT "anime_imports_source_check", ADD CONSTRAINT "anime_imports_source_check" CHECK (source = ANY (ARRAY['saya_definitions'::text, 'arm'::text, 'arm_supplementary'::text]));
