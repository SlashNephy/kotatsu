-- +goose Up
-- modify "program_archives" table
ALTER TABLE "program_archives" ADD COLUMN "extended_normalized" text NULL;

-- +goose Down
-- reverse: modify "program_archives" table
ALTER TABLE "program_archives" DROP COLUMN "extended_normalized";
