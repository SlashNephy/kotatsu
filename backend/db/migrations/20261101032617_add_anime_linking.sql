-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "anime_linking_enabled" boolean NOT NULL DEFAULT false;
-- create "anime_channel_definitions" table
CREATE TABLE "anime_channel_definitions" ("channel_type" text NOT NULL, "network_id" bigint NOT NULL, "sid" bigint NOT NULL, "name" text NOT NULL, "syobocal_channel_id" integer NULL, "annict_channel_id" integer NULL, PRIMARY KEY ("channel_type", "network_id", "sid"));
-- create "anime_imports" table
CREATE TABLE "anime_imports" ("source" text NOT NULL, "etag" text NULL, "checked_at" timestamptz NOT NULL, "imported_at" timestamptz NULL, "item_count" integer NOT NULL, "error" text NULL, PRIMARY KEY ("source"), CONSTRAINT "anime_imports_source_check" CHECK (source = ANY (ARRAY['saya_definitions'::text, 'arm'::text, 'arm_supplementary'::text])));
-- create "anime_works" table
CREATE TABLE "anime_works" ("source" text NOT NULL, "syobocal_tid" integer NOT NULL, "position" smallint NOT NULL, "annict_work_id" integer NULL, "anilist_id" integer NULL, "mal_id" integer NULL, PRIMARY KEY ("source", "syobocal_tid", "position"), CONSTRAINT "anime_works_source_check" CHECK (source = ANY (ARRAY['arm'::text, 'arm_supplementary'::text])));
-- create index "anime_works_syobocal_tid_idx" to table: "anime_works"
CREATE INDEX "anime_works_syobocal_tid_idx" ON "anime_works" ("syobocal_tid");
-- create "anime_channel_overrides" table
CREATE TABLE "anime_channel_overrides" ("service_id" bigint NOT NULL, "syobocal_channel_id" integer NULL, "annict_channel_id" integer NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("service_id"), CONSTRAINT "anime_channel_overrides_service_id_fkey" FOREIGN KEY ("service_id") REFERENCES "services" ("id") ON UPDATE NO ACTION ON DELETE CASCADE, CONSTRAINT "anime_channel_overrides_annict_channel_id_check" CHECK (annict_channel_id > 0), CONSTRAINT "anime_channel_overrides_check" CHECK ((syobocal_channel_id IS NOT NULL) OR (annict_channel_id IS NOT NULL)), CONSTRAINT "anime_channel_overrides_syobocal_channel_id_check" CHECK (syobocal_channel_id > 0));

-- +goose Down
-- reverse: create "anime_channel_overrides" table
DROP TABLE "anime_channel_overrides";
-- reverse: create index "anime_works_syobocal_tid_idx" to table: "anime_works"
DROP INDEX "anime_works_syobocal_tid_idx";
-- reverse: create "anime_works" table
DROP TABLE "anime_works";
-- reverse: create "anime_imports" table
DROP TABLE "anime_imports";
-- reverse: create "anime_channel_definitions" table
DROP TABLE "anime_channel_definitions";
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "anime_linking_enabled";
