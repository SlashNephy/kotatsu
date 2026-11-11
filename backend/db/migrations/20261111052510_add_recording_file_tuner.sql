-- +goose Up
-- modify "recording_files" table
ALTER TABLE "recording_files" ADD COLUMN "tuner_index" bigint NULL, ADD COLUMN "tuner_name" text NULL;

-- +goose Down
-- reverse: modify "recording_files" table
ALTER TABLE "recording_files" DROP COLUMN "tuner_name", DROP COLUMN "tuner_index";
