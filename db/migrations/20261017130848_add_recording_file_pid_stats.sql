-- +goose Up
-- modify "recording_files" table
ALTER TABLE "recording_files" ADD COLUMN "pid_stats" jsonb NULL;

-- +goose Down
-- reverse: modify "recording_files" table
ALTER TABLE "recording_files" DROP COLUMN "pid_stats";
