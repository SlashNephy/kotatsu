-- +goose Up
-- modify "programs" table
ALTER TABLE "programs" ADD COLUMN "extended_normalized" text NULL;

-- +goose Down
-- reverse: modify "programs" table
ALTER TABLE "programs" DROP COLUMN "extended_normalized";
