-- +goose Up
-- modify "recording_files" table
ALTER TABLE "recording_files" ADD COLUMN "transcode_frame_rate" integer NULL;

-- +goose Down
-- reverse: modify "recording_files" table
ALTER TABLE "recording_files" DROP COLUMN "transcode_frame_rate";
