-- +goose Up
-- modify "program_archives" table
ALTER TABLE "program_archives" ADD COLUMN "video_resolution" text NULL;
-- modify "programs" table
ALTER TABLE "programs" ADD COLUMN "video_resolution" text NULL;
-- modify "recordings" table
ALTER TABLE "recordings" ADD COLUMN "video_resolution" text NULL;

-- +goose Down
-- reverse: modify "recordings" table
ALTER TABLE "recordings" DROP COLUMN "video_resolution";
-- reverse: modify "programs" table
ALTER TABLE "programs" DROP COLUMN "video_resolution";
-- reverse: modify "program_archives" table
ALTER TABLE "program_archives" DROP COLUMN "video_resolution";
