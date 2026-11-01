-- +goose Up
-- modify "recordings" table
ALTER TABLE "recordings" ADD COLUMN "name_normalized" text NULL, ADD COLUMN "description_normalized" text NULL;

-- +goose Down
-- reverse: modify "recordings" table
ALTER TABLE "recordings" DROP COLUMN "description_normalized", DROP COLUMN "name_normalized";
