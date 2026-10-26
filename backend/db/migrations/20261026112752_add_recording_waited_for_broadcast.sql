-- +goose Up
-- modify "recordings" table
ALTER TABLE "recordings" ADD COLUMN "waited_for_broadcast" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "recordings" table
ALTER TABLE "recordings" DROP COLUMN "waited_for_broadcast";
