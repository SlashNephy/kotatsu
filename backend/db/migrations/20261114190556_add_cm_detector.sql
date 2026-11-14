-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "cm_detector" text NOT NULL DEFAULT 'ffmpeg';

-- +goose Down
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "cm_detector";
