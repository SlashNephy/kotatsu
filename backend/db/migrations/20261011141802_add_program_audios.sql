-- +goose Up
-- modify "program_archives" table
ALTER TABLE "program_archives" ADD COLUMN "audios" jsonb NOT NULL DEFAULT '[]';
-- modify "programs" table
ALTER TABLE "programs" ADD COLUMN "audios" jsonb NOT NULL DEFAULT '[]';
-- modify "recordings" table
ALTER TABLE "recordings" ADD COLUMN "audios" jsonb NOT NULL DEFAULT '[]';

-- +goose Down
-- reverse: modify "recordings" table
ALTER TABLE "recordings" DROP COLUMN "audios";
-- reverse: modify "programs" table
ALTER TABLE "programs" DROP COLUMN "audios";
-- reverse: modify "program_archives" table
ALTER TABLE "program_archives" DROP COLUMN "audios";
