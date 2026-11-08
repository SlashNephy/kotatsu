-- +goose Up
-- modify "program_archives" table
ALTER TABLE "program_archives" ADD COLUMN "series_episode" integer NULL, ADD COLUMN "series_repeat" boolean NULL;
-- modify "programs" table
ALTER TABLE "programs" ADD COLUMN "series_episode" integer NULL, ADD COLUMN "series_repeat" boolean NULL;

-- +goose Down
-- reverse: modify "programs" table
ALTER TABLE "programs" DROP COLUMN "series_repeat", DROP COLUMN "series_episode";
-- reverse: modify "program_archives" table
ALTER TABLE "program_archives" DROP COLUMN "series_repeat", DROP COLUMN "series_episode";
