-- +goose Up
-- modify "recording_history" table
ALTER TABLE "recording_history" ADD COLUMN "description" text NOT NULL DEFAULT '', ADD COLUMN "series_episode" integer NULL, ADD COLUMN "syobocal_tid" integer NULL, ADD COLUMN "anime_count" double precision NULL, ADD COLUMN "imported" boolean NOT NULL DEFAULT false;
-- ほかの録画ソフトから引き継いだ履歴は、取り込みの記録 (import_sources) から見分ける (#1498)。
UPDATE "recording_history" SET "imported" = true
WHERE "id" IN (SELECT "target_id" FROM "import_sources" WHERE "kind" = 'recorded_history');
-- modify "rules" table
ALTER TABLE "rules" ADD COLUMN "bundle_same_episodes" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "rules" table
ALTER TABLE "rules" DROP COLUMN "bundle_same_episodes";
-- reverse: modify "recording_history" table
ALTER TABLE "recording_history" DROP COLUMN "imported", DROP COLUMN "anime_count", DROP COLUMN "syobocal_tid", DROP COLUMN "series_episode", DROP COLUMN "description";
