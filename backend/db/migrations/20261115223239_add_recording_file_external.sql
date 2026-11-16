-- +goose Up
-- modify "recording_files" table
ALTER TABLE "recording_files" ADD COLUMN "external" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "recording_files" table
ALTER TABLE "recording_files" DROP COLUMN "external";
