-- +goose Up
-- modify "recording_assignments" table
ALTER TABLE "recording_assignments" ADD COLUMN "stream_generation" bigint NOT NULL DEFAULT 0;

-- +goose Down
-- reverse: modify "recording_assignments" table
ALTER TABLE "recording_assignments" DROP COLUMN "stream_generation";
