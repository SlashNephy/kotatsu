-- +goose Up
-- modify "recordings" table
ALTER TABLE "recordings" ADD COLUMN "gaps" jsonb NOT NULL DEFAULT '[]', ADD COLUMN "program_lost" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "recordings" table
ALTER TABLE "recordings" DROP COLUMN "program_lost", DROP COLUMN "gaps";
