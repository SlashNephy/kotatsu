-- +goose Up
-- modify "captures" table
ALTER TABLE "captures" ADD COLUMN "caption_normalized" text NULL;

-- +goose Down
-- reverse: modify "captures" table
ALTER TABLE "captures" DROP COLUMN "caption_normalized";
