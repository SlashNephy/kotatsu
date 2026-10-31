-- +goose Up
-- modify "recording_failure_counts" table
ALTER TABLE "recording_failure_counts" ADD COLUMN "reset_at" timestamptz NULL;

-- +goose Down
-- reverse: modify "recording_failure_counts" table
ALTER TABLE "recording_failure_counts" DROP COLUMN "reset_at";
