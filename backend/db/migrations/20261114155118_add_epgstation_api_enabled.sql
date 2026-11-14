-- +goose Up
-- modify "system_settings" table
ALTER TABLE "system_settings" ADD COLUMN "epgstation_api_enabled" boolean NOT NULL DEFAULT false;

-- +goose Down
-- reverse: modify "system_settings" table
ALTER TABLE "system_settings" DROP COLUMN "epgstation_api_enabled";
