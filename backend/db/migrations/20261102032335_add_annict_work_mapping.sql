-- +goose Up
-- modify "anime_imports" table
ALTER TABLE "anime_imports" DROP CONSTRAINT "anime_imports_source_check", ADD CONSTRAINT "anime_imports_source_check" CHECK (source = ANY (ARRAY['saya_definitions'::text, 'arm'::text, 'syobocal_titles'::text, 'annict_works'::text]));
-- modify "anime_works" table
ALTER TABLE "anime_works" DROP CONSTRAINT "anime_works_source_check", ADD CONSTRAINT "anime_works_source_check" CHECK (source = ANY (ARRAY['annict'::text, 'arm'::text]));
-- create "anime_import_tokens" table
CREATE TABLE "anime_import_tokens" ("id" smallint NOT NULL, "annict_token_encrypted" bytea NOT NULL, "updated_at" timestamptz NOT NULL, PRIMARY KEY ("id"), CONSTRAINT "anime_import_tokens_id_check" CHECK (id = 1));
-- create "annict_work_sync" table
CREATE TABLE "annict_work_sync" ("id" smallint NOT NULL, "seasonal_at" timestamptz NULL, "full_cursor" text NULL, "full_completed_at" timestamptz NULL, PRIMARY KEY ("id"), CONSTRAINT "annict_work_sync_id_check" CHECK (id = 1));
-- create "annict_works" table
CREATE TABLE "annict_works" ("annict_id" integer NOT NULL, "syobocal_tid" integer NULL, "mal_anime_id" integer NULL, "anilist_id" integer NULL, "anilist_checked_at" timestamptz NULL, "fetched_at" timestamptz NOT NULL, PRIMARY KEY ("annict_id"));
-- create index "annict_works_anilist_checked_at_idx" to table: "annict_works"
CREATE INDEX "annict_works_anilist_checked_at_idx" ON "annict_works" ("anilist_checked_at" NULLS FIRST) WHERE (mal_anime_id IS NOT NULL);

-- +goose Down
-- reverse: create index "annict_works_anilist_checked_at_idx" to table: "annict_works"
DROP INDEX "annict_works_anilist_checked_at_idx";
-- reverse: create "annict_works" table
DROP TABLE "annict_works";
-- reverse: create "annict_work_sync" table
DROP TABLE "annict_work_sync";
-- reverse: create "anime_import_tokens" table
DROP TABLE "anime_import_tokens";
-- reverse: modify "anime_works" table
ALTER TABLE "anime_works" DROP CONSTRAINT "anime_works_source_check", ADD CONSTRAINT "anime_works_source_check" CHECK (source = 'arm'::text);
-- reverse: modify "anime_imports" table
ALTER TABLE "anime_imports" DROP CONSTRAINT "anime_imports_source_check", ADD CONSTRAINT "anime_imports_source_check" CHECK (source = ANY (ARRAY['saya_definitions'::text, 'arm'::text, 'syobocal_titles'::text]));
