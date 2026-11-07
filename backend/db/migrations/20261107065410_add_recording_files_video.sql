-- +goose Up
-- modify "recording_files" table
ALTER TABLE "recording_files" ADD COLUMN "video" jsonb NULL;

-- +goose Down
-- reverse: modify "recording_files" table
ALTER TABLE "recording_files" DROP COLUMN "video";
