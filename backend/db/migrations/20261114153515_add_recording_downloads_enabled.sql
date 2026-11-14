-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "recording_downloads_enabled" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "recording_downloads_enabled";
